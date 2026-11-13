package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// VenueInput is a cinema as a client sends it.
//
// Venues are shared reference data, not per-user rows: a cinema is a place,
// not an opinion, and one row per cinema is what keeps a diary from filling
// with near-duplicates. Any signed-in user may add or correct one.
type VenueInput struct {
	Name              string   `json:"name"`
	Chain             *string  `json:"chain"`
	Address           *string  `json:"address"`
	City              *string  `json:"city"`
	Region            *string  `json:"region"`
	Country           *string  `json:"country"`
	Latitude          *float64 `json:"latitude"`
	Longitude         *float64 `json:"longitude"`
	Timezone          *string  `json:"timezone"`
	OpenedOn          *string  `json:"opened_on"`
	ClosedOn          *string  `json:"closed_on"`
	Status            *string  `json:"status"`
	ReplacedByVenueID *int64   `json:"replaced_by_venue_id"`
	Note              *string  `json:"note"`
	// Refs are this cinema's ids in outside sources, keyed by source
	// (google_place_id, osm_id, wikidata_id, fsq_id, overture_id). Only
	// Google's place id may be stored indefinitely; its other place data
	// may not, which is why nothing else from Google is kept here.
	Refs map[string]string `json:"refs"`
}

// ScreenInput is one auditorium as a client sends it.
type ScreenInput struct {
	Name          string   `json:"name"`
	Number        *int64   `json:"number"`
	Seats         *int64   `json:"seats"`
	Width         *float64 `json:"width"`
	Height        *float64 `json:"height"`
	DimUnit       *string  `json:"dim_unit"`
	AspectRatio   *string  `json:"aspect_ratio"`
	EffectiveFrom *string  `json:"effective_from"`
	EffectiveTo   *string  `json:"effective_to"`
	Note          *string  `json:"note"`
}

// ListVenues searches the shared venue list.
func (s *TrackingService) ListVenues(ctx context.Context, query, country string, includeClosed bool, limit, offset int64) (map[string]any, error) {
	if country != "" && !countryRe.MatchString(country) {
		return nil, &ValidationError{Message: "country must be a 2-letter ISO 3166-1 code"}
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > maxVenueListLimit {
		limit = maxVenueListLimit
	}
	venues, err := s.read.ListVenues(ctx, repository.VenueFilter{
		Query: strings.TrimSpace(query), Country: strings.ToUpper(country),
		IncludeClosed: includeClosed, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"venues": venues, "limit": limit, "offset": offset}, nil
}

// GetVenue returns a cinema with its screens, former names and outside ids.
func (s *TrackingService) GetVenue(ctx context.Context, id int64) (map[string]any, error) {
	venue, err := s.read.Venue(ctx, id)
	if err != nil {
		return nil, err
	}
	screens, err := s.read.VenueScreens(ctx, id)
	if err != nil {
		return nil, err
	}
	aliases, err := s.read.VenueAliases(ctx, id)
	if err != nil {
		return nil, err
	}
	refs, err := s.read.VenueRefs(ctx, id)
	if err != nil {
		return nil, err
	}
	watches, err := s.read.VenueWatchCount(ctx, id)
	if err != nil {
		return nil, err
	}
	venue.ScreenCount = int64(len(screens))
	return map[string]any{
		"venue": venue, "screens": screens, "aliases": aliases, "refs": refs, "watch_count": watches,
	}, nil
}

// CreateVenue adds a cinema, or returns the one already stored under the
// same outside id so the same place is not added twice.
func (s *TrackingService) CreateVenue(ctx context.Context, userID int64, in VenueInput) (map[string]any, error) {
	venue, err := s.venueFrom(in, repository.Venue{Status: "open"})
	if err != nil {
		return nil, err
	}
	for source, ref := range in.Refs {
		if existing, err := s.write.VenueByRef(ctx, source, ref); err == nil {
			return nil, &ConflictError{Message: fmt.Sprintf("that cinema is already stored as venue %d", existing)}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	id, err := s.write.CreateVenue(ctx, venue, userID)
	if err != nil {
		return nil, err
	}
	if err := s.saveRefs(ctx, id, in.Refs); err != nil {
		return nil, err
	}
	return s.GetVenue(ctx, id)
}

// UpdateVenue corrects a cinema.
func (s *TrackingService) UpdateVenue(ctx context.Context, id int64, in VenueInput) (map[string]any, error) {
	current, err := s.write.Venue(ctx, id)
	if err != nil {
		return nil, err
	}
	venue, err := s.venueFrom(in, current)
	if err != nil {
		return nil, err
	}
	venue.ID = id
	if venue.ReplacedByVenueID != nil && *venue.ReplacedByVenueID == id {
		return nil, &ValidationError{Message: "a venue cannot replace itself"}
	}
	ok, err := s.write.UpdateVenue(ctx, venue)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, sql.ErrNoRows
	}
	if err := s.saveRefs(ctx, id, in.Refs); err != nil {
		return nil, err
	}
	return s.GetVenue(ctx, id)
}

// DeleteVenue removes a cinema no watch points at. One that is in use is
// closed instead, so the diary keeps its meaning.
func (s *TrackingService) DeleteVenue(ctx context.Context, id int64) error {
	n, err := s.write.VenueWatchCount(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		watches := "watches"
		if n == 1 {
			watches = "watch"
		}
		return &ConflictError{Message: fmt.Sprintf(
			"%d %s happened at this venue; set its status to closed instead of deleting it", n, watches)}
	}
	ok, err := s.write.DeleteVenue(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return sql.ErrNoRows
	}
	return nil
}

// AddVenueAlias records a name a cinema used to have.
func (s *TrackingService) AddVenueAlias(ctx context.Context, venueID int64, name string, from, to *string) (repository.VenueAlias, error) {
	if _, err := s.write.Venue(ctx, venueID); err != nil {
		return repository.VenueAlias{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return repository.VenueAlias{}, &ValidationError{Message: "name is required"}
	}
	for field, value := range map[string]*string{"from_date": from, "to_date": to} {
		if err := validateOptionalDate(field, value); err != nil {
			return repository.VenueAlias{}, err
		}
	}
	return s.write.AddVenueAlias(ctx, repository.VenueAlias{
		VenueID: venueID, Name: name, FromDate: trimmedPtr(from), ToDate: trimmedPtr(to)})
}

// DeleteVenueAlias removes a former name.
func (s *TrackingService) DeleteVenueAlias(ctx context.Context, venueID, id int64) error {
	ok, err := s.write.DeleteVenueAlias(ctx, venueID, id)
	if err != nil {
		return err
	}
	if !ok {
		return sql.ErrNoRows
	}
	return nil
}

// CreateScreen adds an auditorium to a cinema.
func (s *TrackingService) CreateScreen(ctx context.Context, venueID int64, in ScreenInput) (repository.VenueScreen, error) {
	if _, err := s.write.Venue(ctx, venueID); err != nil {
		return repository.VenueScreen{}, err
	}
	screen, err := s.screenFrom(in)
	if err != nil {
		return repository.VenueScreen{}, err
	}
	screen.VenueID = venueID
	id, err := s.write.CreateVenueScreen(ctx, screen)
	if err != nil {
		return repository.VenueScreen{}, err
	}
	return s.write.VenueScreen(ctx, id)
}

// UpdateScreen edits an auditorium.
func (s *TrackingService) UpdateScreen(ctx context.Context, venueID, id int64, in ScreenInput) (repository.VenueScreen, error) {
	screen, err := s.screenFrom(in)
	if err != nil {
		return repository.VenueScreen{}, err
	}
	screen.ID, screen.VenueID = id, venueID
	ok, err := s.write.UpdateVenueScreen(ctx, screen)
	if err != nil {
		return repository.VenueScreen{}, err
	}
	if !ok {
		return repository.VenueScreen{}, sql.ErrNoRows
	}
	return s.write.VenueScreen(ctx, id)
}

// DeleteScreen removes an auditorium.
func (s *TrackingService) DeleteScreen(ctx context.Context, venueID, id int64) error {
	ok, err := s.write.DeleteVenueScreen(ctx, venueID, id)
	if err != nil {
		return err
	}
	if !ok {
		return sql.ErrNoRows
	}
	return nil
}

// venueFrom validates a venue body against the row it is replacing.
func (s *TrackingService) venueFrom(in VenueInput, current repository.Venue) (repository.Venue, error) {
	out := current
	if name := strings.TrimSpace(in.Name); name != "" {
		out.Name = name
	}
	if out.Name == "" {
		return repository.Venue{}, &ValidationError{Message: "name is required"}
	}
	out.Chain, out.Address = trimmedPtr(in.Chain), trimmedPtr(in.Address)
	out.City, out.Region, out.Note = trimmedPtr(in.City), trimmedPtr(in.Region), trimmedPtr(in.Note)
	out.ReplacedByVenueID = in.ReplacedByVenueID

	if c := trimmedPtr(in.Country); c != nil {
		if !countryRe.MatchString(*c) {
			return repository.Venue{}, &ValidationError{Message: "country must be a 2-letter ISO 3166-1 code"}
		}
		up := strings.ToUpper(*c)
		out.Country = &up
	} else {
		out.Country = nil
	}
	if in.Latitude != nil && (*in.Latitude < -90 || *in.Latitude > 90) {
		return repository.Venue{}, &ValidationError{Message: "latitude must be between -90 and 90"}
	}
	if in.Longitude != nil && (*in.Longitude < -180 || *in.Longitude > 180) {
		return repository.Venue{}, &ValidationError{Message: "longitude must be between -180 and 180"}
	}
	if (in.Latitude == nil) != (in.Longitude == nil) {
		return repository.Venue{}, &ValidationError{Message: "latitude and longitude go together"}
	}
	out.Latitude, out.Longitude = in.Latitude, in.Longitude

	if tz := trimmedPtr(in.Timezone); tz != nil {
		if _, err := time.LoadLocation(*tz); err != nil {
			return repository.Venue{}, &ValidationError{Message: "timezone must be an IANA name, e.g. Asia/Kolkata"}
		}
		out.Timezone = tz
	} else {
		out.Timezone = nil
	}
	for field, value := range map[string]*string{"opened_on": in.OpenedOn, "closed_on": in.ClosedOn} {
		if err := validateOptionalDate(field, value); err != nil {
			return repository.Venue{}, err
		}
	}
	out.OpenedOn, out.ClosedOn = trimmedPtr(in.OpenedOn), trimmedPtr(in.ClosedOn)

	if status := trimmedPtr(in.Status); status != nil {
		if !trackingVenueStatus[*status] {
			return repository.Venue{}, &ValidationError{Message: "status must be one of open, closed, renamed, rebuilt"}
		}
		out.Status = *status
	}
	if out.Status == "" {
		out.Status = "open"
	}
	return out, nil
}

// screenFrom validates an auditorium body and normalises its measurements.
func (s *TrackingService) screenFrom(in ScreenInput) (repository.VenueScreen, error) {
	out := repository.VenueScreen{
		Name: strings.TrimSpace(in.Name), Number: in.Number, Seats: in.Seats,
		Note: trimmedPtr(in.Note),
	}
	if in.Seats != nil && *in.Seats < 0 {
		return repository.VenueScreen{}, &ValidationError{Message: "seats must not be negative"}
	}
	if in.Width != nil || in.Height != nil {
		unit := trimmedPtr(in.DimUnit)
		if unit == nil {
			return repository.VenueScreen{}, &ValidationError{Message: "dim_unit (m or ft) is required with a screen size"}
		}
		if !trackingDimUnits[*unit] {
			return repository.VenueScreen{}, &ValidationError{Message: "dim_unit must be m or ft"}
		}
		for field, v := range map[string]*float64{"width": in.Width, "height": in.Height} {
			if v != nil && *v <= 0 {
				return repository.VenueScreen{}, &ValidationError{Message: field + " must be greater than zero"}
			}
		}
		out.DimUnit, out.Width, out.Height = unit, in.Width, in.Height
		out.WidthM, out.HeightM = toMetres(in.Width, *unit), toMetres(in.Height, *unit)
	}
	if ratio := trimmedPtr(in.AspectRatio); ratio != nil {
		value, err := parseAspectRatio(*ratio)
		if err != nil {
			return repository.VenueScreen{}, err
		}
		out.AspectRatio = &value
	}
	for field, value := range map[string]*string{"effective_from": in.EffectiveFrom, "effective_to": in.EffectiveTo} {
		if err := validateOptionalDate(field, value); err != nil {
			return repository.VenueScreen{}, err
		}
	}
	out.EffectiveFrom, out.EffectiveTo = trimmedPtr(in.EffectiveFrom), trimmedPtr(in.EffectiveTo)
	return out, nil
}

// saveRefs stores a venue's ids in outside sources.
func (s *TrackingService) saveRefs(ctx context.Context, venueID int64, refs map[string]string) error {
	for source, ref := range refs {
		source, ref = strings.TrimSpace(source), strings.TrimSpace(ref)
		if source == "" || ref == "" {
			continue
		}
		if !venueRefSources[source] {
			return &ValidationError{Message: "unknown ref source " + source + "; expected one of " + allowedValues(venueRefSources)}
		}
		if err := s.write.SaveVenueRef(ctx, venueID, repository.VenueRef{Source: source, Ref: ref}); err != nil {
			return err
		}
	}
	return nil
}

// venueRefSources are the outside catalogues a venue may carry an id from.
var venueRefSources = map[string]bool{
	"google_place_id": true, "osm_id": true, "wikidata_id": true,
	"fsq_id": true, "overture_id": true,
}

func validateOptionalDate(field string, value *string) error {
	v := trimmedPtr(value)
	if v == nil {
		return nil
	}
	if _, err := time.Parse("2006-01-02", *v); err != nil {
		return &ValidationError{Message: field + " must be YYYY-MM-DD"}
	}
	return nil
}
