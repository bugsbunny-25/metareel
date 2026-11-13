package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Enums validated here rather than by CHECK constraints, so a new value
// needs no table rebuild.
var (
	trackingScreeningTypes = map[string]bool{
		"regular": true, "preview": true, "premiere": true, "festival": true,
		"press": true, "rerelease": true, "marathon": true,
	}
	trackingMedia = map[string]bool{
		"bluray_4k": true, "bluray": true, "dvd": true, "vhs": true,
		"digital_file": true, "35mm": true, "70mm": true,
	}
	trackingDimensions   = map[string]bool{"2d": true, "3d": true, "3d_hfr": true}
	trackingResolutions  = map[string]bool{"sd": true, "hd": true, "fhd": true, "4k": true, "8k": true}
	trackingHDR          = map[string]bool{"none": true, "hdr10": true, "hdr10_plus": true, "dolby_vision": true, "hlg": true}
	trackingMonetization = map[string]bool{"flatrate": true, "free": true, "ads": true, "rent": true, "buy": true}
	trackingVenueStatus  = map[string]bool{"open": true, "closed": true, "renamed": true, "rebuilt": true}
	trackingDimUnits     = map[string]bool{"m": true, "ft": true}
	trackingFormatCats   = map[string]bool{
		"brand": true, "projection": true, "sound": true, "motion": true, "dimension": true,
	}
)

var (
	currencyRe   = regexp.MustCompile(`^[A-Z]{3}$`)
	formatSlugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	countryRe    = regexp.MustCompile(`^[A-Za-z]{2}$`)
)

// feetToMetres converts a screen measured in feet.
const feetToMetres = 0.3048

const maxVenueListLimit = 200

// WatchContextInput is where and how a watch happened, as a client sends it.
// It is embedded in WatchInput, so these fields sit at the top level of the
// request body and match the shape a watch is returned in.
type WatchContextInput struct {
	VenueID       *int64  `json:"venue_id"`
	ScreenID      *int64  `json:"screen_id"`
	Seat          *string `json:"seat"`
	ScreeningType *string `json:"screening_type"`
	EventName     *string `json:"event_name"`
	WithQA        *bool   `json:"with_qa"`

	// Money is in minor units (paise, cents) so it is never a float.
	PriceMinor       *int64  `json:"price_minor"`
	FeesMinor        *int64  `json:"fees_minor"`
	ConcessionsMinor *int64  `json:"concessions_minor"`
	Currency         *string `json:"currency"`
	TicketsCount     *int64  `json:"tickets_count"`
	BookingSource    *string `json:"booking_source"`

	ProviderID   *int64  `json:"provider_id"`
	ProviderName *string `json:"provider_name"`
	Monetization *string `json:"monetization"`
	Media        *string `json:"media"`
	Edition      *string `json:"edition"`
	Device       *string `json:"device"`

	Dimension        *string  `json:"dimension"`
	Resolution       *string  `json:"resolution"`
	HDR              *string  `json:"hdr"`
	AudioFormat      *string  `json:"audio_format"`
	FrameRate        *float64 `json:"frame_rate"`
	AudioLanguage    *string  `json:"audio_language"`
	SubtitleLanguage *string  `json:"subtitle_language"`

	// ContentAspectRatio is what the film looked like, as written
	// ("1.90:1", "2.39:1", "16:9"). TMDB has no aspect-ratio data, so this
	// is the viewer's own; it is stored as written and as a number.
	ContentAspectRatio    *string  `json:"content_aspect_ratio"`
	ContentAspectVariable *bool    `json:"content_aspect_variable"`
	ScreenAspectRatio     *string  `json:"screen_aspect_ratio"`
	ScreenWidth           *float64 `json:"screen_width"`
	ScreenHeight          *float64 `json:"screen_height"`
	ScreenDimUnit         *string  `json:"screen_dim_unit"`
}

// resolveContext validates a watch's context and fills in what the venue and
// screen already know. It also returns the venue's time zone, which a watch
// there defaults to.
func (s *TrackingService) resolveContext(ctx context.Context, in WatchContextInput, formats []string) (*repository.WatchContext, []int64, *string, error) {
	c := &repository.WatchContext{
		Seat: trimmedPtr(in.Seat), EventName: trimmedPtr(in.EventName),
		BookingSource: trimmedPtr(in.BookingSource), ProviderName: trimmedPtr(in.ProviderName),
		Edition: trimmedPtr(in.Edition), Device: trimmedPtr(in.Device),
		AudioFormat: trimmedPtr(in.AudioFormat), AudioLanguage: trimmedPtr(in.AudioLanguage),
		SubtitleLanguage: trimmedPtr(in.SubtitleLanguage),
		PriceMinor:       in.PriceMinor, FeesMinor: in.FeesMinor, ConcessionsMinor: in.ConcessionsMinor,
		TicketsCount: in.TicketsCount, ProviderID: in.ProviderID, FrameRate: in.FrameRate,
	}
	if in.WithQA != nil {
		c.WithQA = *in.WithQA
	}
	if in.ContentAspectVariable != nil {
		c.ContentAspectVariable = *in.ContentAspectVariable
	}

	for _, e := range []struct {
		name  string
		value *string
		set   map[string]bool
		out   **string
	}{
		{"screening_type", in.ScreeningType, trackingScreeningTypes, &c.ScreeningType},
		{"media", in.Media, trackingMedia, &c.Media},
		{"dimension", in.Dimension, trackingDimensions, &c.Dimension},
		{"resolution", in.Resolution, trackingResolutions, &c.Resolution},
		{"hdr", in.HDR, trackingHDR, &c.HDR},
		{"monetization", in.Monetization, trackingMonetization, &c.Monetization},
	} {
		v := trimmedPtr(e.value)
		if v == nil {
			continue
		}
		if !e.set[*v] {
			return nil, nil, nil, &ValidationError{Message: e.name + " is not one of the values this API accepts: " + allowedValues(e.set)}
		}
		*e.out = v
	}

	if cur := trimmedPtr(in.Currency); cur != nil {
		up := strings.ToUpper(*cur)
		if !currencyRe.MatchString(up) {
			return nil, nil, nil, &ValidationError{Message: "currency must be a 3-letter ISO 4217 code, e.g. INR or USD"}
		}
		c.Currency = &up
	}
	for name, amount := range map[string]*int64{
		"price_minor": in.PriceMinor, "fees_minor": in.FeesMinor, "concessions_minor": in.ConcessionsMinor,
	} {
		if amount != nil && *amount < 0 {
			return nil, nil, nil, &ValidationError{Message: name + " must not be negative"}
		}
	}
	if (in.PriceMinor != nil || in.FeesMinor != nil || in.ConcessionsMinor != nil) && c.Currency == nil {
		return nil, nil, nil, &ValidationError{Message: "currency is required when an amount is given"}
	}
	if in.TicketsCount != nil && *in.TicketsCount < 1 {
		return nil, nil, nil, &ValidationError{Message: "tickets_count must be 1 or more"}
	}

	if ratio := trimmedPtr(in.ContentAspectRatio); ratio != nil {
		value, err := parseAspectRatio(*ratio)
		if err != nil {
			return nil, nil, nil, err
		}
		c.ContentAspectRatioText, c.ContentAspectRatio = ratio, &value
	}
	if ratio := trimmedPtr(in.ScreenAspectRatio); ratio != nil {
		value, err := parseAspectRatio(*ratio)
		if err != nil {
			return nil, nil, nil, err
		}
		c.ScreenAspectRatio = &value
	}

	unit := trimmedPtr(in.ScreenDimUnit)
	if in.ScreenWidth != nil || in.ScreenHeight != nil {
		if unit == nil {
			return nil, nil, nil, &ValidationError{Message: "screen_dim_unit (m or ft) is required with a screen size"}
		}
		if !trackingDimUnits[*unit] {
			return nil, nil, nil, &ValidationError{Message: "screen_dim_unit must be m or ft"}
		}
		c.ScreenDimUnit = unit
		c.ScreenWidth, c.ScreenHeight = in.ScreenWidth, in.ScreenHeight
		c.ScreenWidthM, c.ScreenHeightM = toMetres(in.ScreenWidth, *unit), toMetres(in.ScreenHeight, *unit)
	}

	var venueTZ *string
	if in.VenueID != nil {
		venue, err := s.write.Venue(ctx, *in.VenueID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil, &ValidationError{Message: "no such venue"}
		}
		if err != nil {
			return nil, nil, nil, err
		}
		c.VenueID = &venue.ID
		// Snapshot the venue as it is now, so a later rename or closure
		// cannot rewrite what the diary says you went to.
		name := venue.Name
		c.VenueNameAtWatch = &name
		c.VenueCityAtWatch = venue.City
		venueTZ = venue.Timezone

		if in.ScreenID != nil {
			screen, err := s.write.VenueScreen(ctx, *in.ScreenID)
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil, nil, &ValidationError{Message: "no such screen"}
			}
			if err != nil {
				return nil, nil, nil, err
			}
			if screen.VenueID != venue.ID {
				return nil, nil, nil, &ValidationError{Message: "that screen belongs to another venue"}
			}
			c.ScreenID = &screen.ID
			// The screen's own geometry fills in anything not given.
			if c.ScreenAspectRatio == nil {
				c.ScreenAspectRatio = screen.AspectRatio
			}
			if c.ScreenWidth == nil && c.ScreenHeight == nil {
				c.ScreenWidth, c.ScreenHeight = screen.Width, screen.Height
				c.ScreenDimUnit, c.ScreenWidthM, c.ScreenHeightM = screen.DimUnit, screen.WidthM, screen.HeightM
			}
		}
	} else if in.ScreenID != nil {
		return nil, nil, nil, &ValidationError{Message: "screen_id needs the venue_id it belongs to"}
	}

	ids, err := s.formatIDs(ctx, formats)
	if err != nil {
		return nil, nil, nil, err
	}
	return c, ids, venueTZ, nil
}

// formatIDs turns format slugs into ids, refusing unknown or retired ones.
func (s *TrackingService) formatIDs(ctx context.Context, slugs []string) ([]int64, error) {
	if len(slugs) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool, len(slugs))
	ids := make([]int64, 0, len(slugs))
	for _, raw := range slugs {
		slug := strings.ToLower(strings.TrimSpace(raw))
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		f, err := s.write.FormatBySlug(ctx, slug)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &ValidationError{Message: fmt.Sprintf("unknown format %q; GET /api/v1/me/formats lists them", slug)}
		}
		if err != nil {
			return nil, err
		}
		if !f.Active {
			return nil, &ValidationError{Message: fmt.Sprintf("the format %q is no longer offered", slug)}
		}
		ids = append(ids, f.ID)
	}
	return ids, nil
}

// ListFormats returns the premium-format taxonomy.
func (s *TrackingService) ListFormats(ctx context.Context, activeOnly bool) (map[string]any, error) {
	formats, err := s.read.ListFormats(ctx, activeOnly)
	if err != nil {
		return nil, err
	}
	return map[string]any{"formats": formats}, nil
}

// FormatInput is a format an admin adds or edits.
type FormatInput struct {
	Slug      string  `json:"slug"`
	Label     string  `json:"label"`
	Category  string  `json:"category"`
	Note      *string `json:"note"`
	SortOrder *int64  `json:"sort_order"`
	Active    *bool   `json:"active"`
}

// CreateFormat adds a format to the taxonomy.
func (s *TrackingService) CreateFormat(ctx context.Context, in FormatInput) (repository.WatchFormat, error) {
	f := repository.WatchFormat{
		Slug: strings.ToLower(strings.TrimSpace(in.Slug)), Label: strings.TrimSpace(in.Label),
		Category: strings.TrimSpace(in.Category), Note: trimmedPtr(in.Note), Active: true,
	}
	if !formatSlugRe.MatchString(f.Slug) {
		return repository.WatchFormat{}, &ValidationError{Message: "slug must be lowercase words joined by hyphens, e.g. imax-70mm"}
	}
	if f.Label == "" {
		return repository.WatchFormat{}, &ValidationError{Message: "label is required"}
	}
	if !trackingFormatCats[f.Category] {
		return repository.WatchFormat{}, &ValidationError{Message: "category must be one of brand, projection, sound, motion, dimension"}
	}
	if in.SortOrder != nil {
		f.SortOrder = *in.SortOrder
	}
	if in.Active != nil {
		f.Active = *in.Active
	}
	if _, err := s.write.FormatBySlug(ctx, f.Slug); err == nil {
		return repository.WatchFormat{}, &ConflictError{Message: "a format with that slug already exists"}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return repository.WatchFormat{}, err
	}
	return s.write.CreateFormat(ctx, f)
}

// UpdateFormat edits a format. The slug never changes, because watches
// reference it; retiring one is done by setting active to false, never by
// deleting it.
func (s *TrackingService) UpdateFormat(ctx context.Context, id int64, in FormatInput) (repository.WatchFormat, error) {
	current, err := s.write.Format(ctx, id)
	if err != nil {
		return repository.WatchFormat{}, err
	}
	if in.Label != "" {
		current.Label = strings.TrimSpace(in.Label)
	}
	if in.Category != "" {
		if !trackingFormatCats[strings.TrimSpace(in.Category)] {
			return repository.WatchFormat{}, &ValidationError{Message: "category must be one of brand, projection, sound, motion, dimension"}
		}
		current.Category = strings.TrimSpace(in.Category)
	}
	if in.Note != nil {
		current.Note = trimmedPtr(in.Note)
	}
	if in.SortOrder != nil {
		current.SortOrder = *in.SortOrder
	}
	if in.Active != nil {
		current.Active = *in.Active
	}
	ok, err := s.write.UpdateFormat(ctx, current)
	if err != nil {
		return repository.WatchFormat{}, err
	}
	if !ok {
		return repository.WatchFormat{}, sql.ErrNoRows
	}
	return s.write.Format(ctx, id)
}

// parseAspectRatio reads "1.90:1", "2.39:1", "16:9" or a plain number.
func parseAspectRatio(raw string) (float64, error) {
	bad := &ValidationError{Message: `aspect ratio must look like "1.90:1", "16:9" or "1.9"`}
	raw = strings.TrimSpace(raw)
	width, height, hasColon := strings.Cut(raw, ":")
	w, err := strconv.ParseFloat(strings.TrimSpace(width), 64)
	if err != nil || w <= 0 {
		return 0, bad
	}
	if !hasColon {
		return round2(w), nil
	}
	h, err := strconv.ParseFloat(strings.TrimSpace(height), 64)
	if err != nil || h <= 0 {
		return 0, bad
	}
	return round2(w / h), nil
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

// toMetres normalises a screen measurement so screens compare.
func toMetres(v *float64, unit string) *float64 {
	if v == nil {
		return nil
	}
	out := *v
	if unit == "ft" {
		out *= feetToMetres
	}
	out = float64(int64(out*1000+0.5)) / 1000
	return &out
}

// allowedValues lists a value set for an error message.
func allowedValues(set map[string]bool) string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
