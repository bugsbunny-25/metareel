package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// WatchFormat is one premium format in the taxonomy admins maintain
// (IMAX, Dolby Cinema, 4DX, 70mm, Atmos, ...). category separates the
// independent axes a cinema format mixes, so "all my 70mm screenings" and
// "IMAX hours" stay answerable.
type WatchFormat struct {
	ID        int64   `json:"id"`
	Slug      string  `json:"slug"`
	Label     string  `json:"label"`
	Category  string  `json:"category"`
	Note      *string `json:"note,omitempty"`
	SortOrder int64   `json:"sort_order"`
	Active    bool    `json:"active"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

func formatFrom(row sqlc.WatchFormat) WatchFormat {
	return WatchFormat{
		ID: row.ID, Slug: row.Slug, Label: row.Label, Category: row.Category,
		Note: nullStringPtr(row.Note), SortOrder: row.SortOrder, Active: row.Active != 0,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// ListFormats returns the taxonomy, lowest sort order first.
func (r *TrackingRepository) ListFormats(ctx context.Context, activeOnly bool) ([]WatchFormat, error) {
	rows, err := r.q.ListWatchFormats(ctx, boolInt(activeOnly))
	if err != nil {
		return nil, fmt.Errorf("list watch formats: %w", err)
	}
	out := make([]WatchFormat, 0, len(rows))
	for _, row := range rows {
		out = append(out, formatFrom(row))
	}
	return out, nil
}

// Format returns one format by id, or sql.ErrNoRows.
func (r *TrackingRepository) Format(ctx context.Context, id int64) (WatchFormat, error) {
	row, err := r.q.GetWatchFormat(ctx, id)
	if err != nil {
		return WatchFormat{}, err
	}
	return formatFrom(row), nil
}

// FormatBySlug returns one format by slug, or sql.ErrNoRows.
func (r *TrackingRepository) FormatBySlug(ctx context.Context, slug string) (WatchFormat, error) {
	row, err := r.q.GetWatchFormatBySlug(ctx, slug)
	if err != nil {
		return WatchFormat{}, err
	}
	return formatFrom(row), nil
}

// CreateFormat adds a format to the taxonomy.
func (r *TrackingRepository) CreateFormat(ctx context.Context, f WatchFormat) (WatchFormat, error) {
	row, err := r.q.CreateWatchFormat(ctx, sqlc.CreateWatchFormatParams{
		Slug: f.Slug, Label: f.Label, Category: f.Category, Note: toNullString(f.Note),
		SortOrder: f.SortOrder, Active: boolInt(f.Active),
	})
	if err != nil {
		return WatchFormat{}, fmt.Errorf("create watch format %q: %w", f.Slug, err)
	}
	return formatFrom(row), nil
}

// UpdateFormat edits a format. Its slug never changes, because watches
// reference it.
func (r *TrackingRepository) UpdateFormat(ctx context.Context, f WatchFormat) (bool, error) {
	n, err := r.q.UpdateWatchFormat(ctx, sqlc.UpdateWatchFormatParams{
		ID: f.ID, Label: f.Label, Category: f.Category, Note: toNullString(f.Note),
		SortOrder: f.SortOrder, Active: boolInt(f.Active),
	})
	if err != nil {
		return false, fmt.Errorf("update watch format %d: %w", f.ID, err)
	}
	return n > 0, nil
}

// formatsFor hydrates a page of watches with their formats.
func (r *TrackingRepository) formatsFor(ctx context.Context, userID int64, ids []int64) (map[int64][]WatchFormat, error) {
	rows, err := r.q.ListFormatsForWatchEntries(ctx, sqlc.ListFormatsForWatchEntriesParams{
		UserID: userID, EntryIds: ids})
	if err != nil {
		return nil, fmt.Errorf("list formats for watches: %w", err)
	}
	out := make(map[int64][]WatchFormat, len(ids))
	for _, row := range rows {
		out[row.EntryID] = append(out[row.EntryID], WatchFormat{
			ID: row.FormatID, Slug: row.Slug, Label: row.Label, Category: row.Category, Active: true})
	}
	return out, nil
}

// WatchContext is where and how a watch happened. Everything is optional:
// a watch with none of it is still a watch.
type WatchContext struct {
	VenueID          *int64  `json:"venue_id,omitempty"`
	ScreenID         *int64  `json:"screen_id,omitempty"`
	VenueNameAtWatch *string `json:"venue_name_at_watch,omitempty"`
	VenueCityAtWatch *string `json:"venue_city_at_watch,omitempty"`
	Seat             *string `json:"seat,omitempty"`
	ScreeningType    *string `json:"screening_type,omitempty"`
	EventName        *string `json:"event_name,omitempty"`
	WithQA           bool    `json:"with_qa"`

	PriceMinor       *int64  `json:"price_minor,omitempty"`
	FeesMinor        *int64  `json:"fees_minor,omitempty"`
	ConcessionsMinor *int64  `json:"concessions_minor,omitempty"`
	Currency         *string `json:"currency,omitempty"`
	TicketsCount     *int64  `json:"tickets_count,omitempty"`
	BookingSource    *string `json:"booking_source,omitempty"`

	ProviderID   *int64  `json:"provider_id,omitempty"`
	ProviderName *string `json:"provider_name,omitempty"`
	Monetization *string `json:"monetization,omitempty"`
	Media        *string `json:"media,omitempty"`
	Edition      *string `json:"edition,omitempty"`
	Device       *string `json:"device,omitempty"`

	Dimension        *string  `json:"dimension,omitempty"`
	Resolution       *string  `json:"resolution,omitempty"`
	HDR              *string  `json:"hdr,omitempty"`
	AudioFormat      *string  `json:"audio_format,omitempty"`
	FrameRate        *float64 `json:"frame_rate,omitempty"`
	AudioLanguage    *string  `json:"audio_language,omitempty"`
	SubtitleLanguage *string  `json:"subtitle_language,omitempty"`

	ContentAspectRatioText *string  `json:"content_aspect_ratio_text,omitempty"`
	ContentAspectRatio     *float64 `json:"content_aspect_ratio,omitempty"`
	ContentAspectVariable  bool     `json:"content_aspect_variable"`
	ScreenAspectRatio      *float64 `json:"screen_aspect_ratio,omitempty"`
	ScreenWidth            *float64 `json:"screen_width,omitempty"`
	ScreenHeight           *float64 `json:"screen_height,omitempty"`
	ScreenDimUnit          *string  `json:"screen_dim_unit,omitempty"`
	ScreenWidthM           *float64 `json:"screen_width_m,omitempty"`
	ScreenHeightM          *float64 `json:"screen_height_m,omitempty"`
}

// setContext writes a watch's where-and-how columns inside a transaction.
func setContext(ctx context.Context, q *sqlc.Queries, userID, entryID int64, c *WatchContext) error {
	if c == nil {
		c = &WatchContext{}
	}
	if _, err := q.SetWatchEntryContext(ctx, sqlc.SetWatchEntryContextParams{
		ID: entryID, UserID: userID,
		VenueID: toNullInt64(c.VenueID), ScreenID: toNullInt64(c.ScreenID),
		VenueNameAtWatch: toNullString(c.VenueNameAtWatch), VenueCityAtWatch: toNullString(c.VenueCityAtWatch),
		Seat: toNullString(c.Seat), ScreeningType: toNullString(c.ScreeningType),
		EventName: toNullString(c.EventName), WithQa: boolInt(c.WithQA),
		PriceMinor: toNullInt64(c.PriceMinor), FeesMinor: toNullInt64(c.FeesMinor),
		ConcessionsMinor: toNullInt64(c.ConcessionsMinor), Currency: toNullString(c.Currency),
		TicketsCount: toNullInt64(c.TicketsCount), BookingSource: toNullString(c.BookingSource),
		ProviderID: toNullInt64(c.ProviderID), ProviderName: toNullString(c.ProviderName),
		Monetization: toNullString(c.Monetization), Media: toNullString(c.Media),
		Edition: toNullString(c.Edition), Device: toNullString(c.Device),
		Dimension: toNullString(c.Dimension), Resolution: toNullString(c.Resolution),
		Hdr: toNullString(c.HDR), AudioFormat: toNullString(c.AudioFormat),
		FrameRate: floatPtrNull(c.FrameRate), AudioLanguage: toNullString(c.AudioLanguage),
		SubtitleLanguage:       toNullString(c.SubtitleLanguage),
		ContentAspectRatioText: toNullString(c.ContentAspectRatioText),
		ContentAspectRatio:     floatPtrNull(c.ContentAspectRatio),
		ContentAspectVariable:  boolInt(c.ContentAspectVariable),
		ScreenAspectRatio:      floatPtrNull(c.ScreenAspectRatio),
		ScreenWidth:            floatPtrNull(c.ScreenWidth), ScreenHeight: floatPtrNull(c.ScreenHeight),
		ScreenDimUnit: toNullString(c.ScreenDimUnit),
		ScreenWidthM:  floatPtrNull(c.ScreenWidthM), ScreenHeightM: floatPtrNull(c.ScreenHeightM),
	}); err != nil {
		return fmt.Errorf("set context of watch %d: %w", entryID, err)
	}
	return nil
}

// setFormats replaces a watch's formats.
func setFormats(ctx context.Context, q *sqlc.Queries, entryID int64, ids []int64) error {
	if err := q.ClearWatchEntryFormats(ctx, entryID); err != nil {
		return fmt.Errorf("clear formats of watch %d: %w", entryID, err)
	}
	for _, id := range ids {
		if err := q.AddWatchEntryFormat(ctx, sqlc.AddWatchEntryFormatParams{EntryID: entryID, FormatID: id}); err != nil {
			return fmt.Errorf("add format %d to watch %d: %w", id, entryID, err)
		}
	}
	return nil
}

// contextFrom reads the where-and-how columns off a stored watch.
func contextFrom(row sqlc.WatchEntry) WatchContext {
	return WatchContext{
		VenueID: nullInt64Ptr(row.VenueID), ScreenID: nullInt64Ptr(row.ScreenID),
		VenueNameAtWatch: nullStringPtr(row.VenueNameAtWatch), VenueCityAtWatch: nullStringPtr(row.VenueCityAtWatch),
		Seat: nullStringPtr(row.Seat), ScreeningType: nullStringPtr(row.ScreeningType),
		EventName: nullStringPtr(row.EventName), WithQA: row.WithQa != 0,
		PriceMinor: nullInt64Ptr(row.PriceMinor), FeesMinor: nullInt64Ptr(row.FeesMinor),
		ConcessionsMinor: nullInt64Ptr(row.ConcessionsMinor), Currency: nullStringPtr(row.Currency),
		TicketsCount: nullInt64Ptr(row.TicketsCount), BookingSource: nullStringPtr(row.BookingSource),
		ProviderID: nullInt64Ptr(row.ProviderID), ProviderName: nullStringPtr(row.ProviderName),
		Monetization: nullStringPtr(row.Monetization), Media: nullStringPtr(row.Media),
		Edition: nullStringPtr(row.Edition), Device: nullStringPtr(row.Device),
		Dimension: nullStringPtr(row.Dimension), Resolution: nullStringPtr(row.Resolution),
		HDR: nullStringPtr(row.Hdr), AudioFormat: nullStringPtr(row.AudioFormat),
		FrameRate: nullFloatPtr(row.FrameRate), AudioLanguage: nullStringPtr(row.AudioLanguage),
		SubtitleLanguage:       nullStringPtr(row.SubtitleLanguage),
		ContentAspectRatioText: nullStringPtr(row.ContentAspectRatioText),
		ContentAspectRatio:     nullFloatPtr(row.ContentAspectRatio),
		ContentAspectVariable:  row.ContentAspectVariable != 0,
		ScreenAspectRatio:      nullFloatPtr(row.ScreenAspectRatio),
		ScreenWidth:            nullFloatPtr(row.ScreenWidth), ScreenHeight: nullFloatPtr(row.ScreenHeight),
		ScreenDimUnit: nullStringPtr(row.ScreenDimUnit),
		ScreenWidthM:  nullFloatPtr(row.ScreenWidthM), ScreenHeightM: nullFloatPtr(row.ScreenHeightM),
	}
}

// Venue is a cinema. Venues are shared reference data, not per-user rows: a
// cinema is a place, not an opinion.
type Venue struct {
	ID                int64    `json:"id"`
	Name              string   `json:"name"`
	Chain             *string  `json:"chain,omitempty"`
	Address           *string  `json:"address,omitempty"`
	City              *string  `json:"city,omitempty"`
	Region            *string  `json:"region,omitempty"`
	Country           *string  `json:"country,omitempty"`
	Latitude          *float64 `json:"latitude,omitempty"`
	Longitude         *float64 `json:"longitude,omitempty"`
	Timezone          *string  `json:"timezone,omitempty"`
	OpenedOn          *string  `json:"opened_on,omitempty"`
	ClosedOn          *string  `json:"closed_on,omitempty"`
	Status            string   `json:"status"`
	ReplacedByVenueID *int64   `json:"replaced_by_venue_id,omitempty"`
	Note              *string  `json:"note,omitempty"`
	CreatedBy         *int64   `json:"created_by,omitempty"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
	ScreenCount       int64    `json:"screen_count"`
}

// CreateVenue adds a cinema and returns its id.
func (r *TrackingRepository) CreateVenue(ctx context.Context, v Venue, createdBy int64) (int64, error) {
	id, err := r.q.CreateVenue(ctx, sqlc.CreateVenueParams{
		Name: v.Name, Chain: toNullString(v.Chain), Address: toNullString(v.Address),
		City: toNullString(v.City), Region: toNullString(v.Region), Country: toNullString(v.Country),
		Latitude: floatPtrNull(v.Latitude), Longitude: floatPtrNull(v.Longitude),
		Timezone: toNullString(v.Timezone), OpenedOn: toNullString(v.OpenedOn),
		ClosedOn: toNullString(v.ClosedOn), Status: v.Status, Note: toNullString(v.Note),
		CreatedBy: sql.NullInt64{Int64: createdBy, Valid: createdBy > 0},
	})
	if err != nil {
		return 0, fmt.Errorf("create venue %q: %w", v.Name, err)
	}
	return id, nil
}

// UpdateVenue edits a cinema.
func (r *TrackingRepository) UpdateVenue(ctx context.Context, v Venue) (bool, error) {
	n, err := r.q.UpdateVenue(ctx, sqlc.UpdateVenueParams{
		ID: v.ID, Name: v.Name, Chain: toNullString(v.Chain), Address: toNullString(v.Address),
		City: toNullString(v.City), Region: toNullString(v.Region), Country: toNullString(v.Country),
		Latitude: floatPtrNull(v.Latitude), Longitude: floatPtrNull(v.Longitude),
		Timezone: toNullString(v.Timezone), OpenedOn: toNullString(v.OpenedOn),
		ClosedOn: toNullString(v.ClosedOn), Status: v.Status,
		ReplacedByVenueID: toNullInt64(v.ReplacedByVenueID), Note: toNullString(v.Note),
	})
	if err != nil {
		return false, fmt.Errorf("update venue %d: %w", v.ID, err)
	}
	return n > 0, nil
}

// Venue returns one cinema, or sql.ErrNoRows.
func (r *TrackingRepository) Venue(ctx context.Context, id int64) (Venue, error) {
	row, err := r.q.GetVenue(ctx, id)
	if err != nil {
		return Venue{}, err
	}
	return Venue{
		ID: row.ID, Name: row.Name, Chain: nullStringPtr(row.Chain), Address: nullStringPtr(row.Address),
		City: nullStringPtr(row.City), Region: nullStringPtr(row.Region), Country: nullStringPtr(row.Country),
		Latitude: nullFloatPtr(row.Latitude), Longitude: nullFloatPtr(row.Longitude),
		Timezone: nullStringPtr(row.Timezone), OpenedOn: nullStringPtr(row.OpenedOn),
		ClosedOn: nullStringPtr(row.ClosedOn), Status: row.Status,
		ReplacedByVenueID: nullInt64Ptr(row.ReplacedByVenueID), Note: nullStringPtr(row.Note),
		CreatedBy: nullInt64Ptr(row.CreatedBy), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}

// VenueFilter searches the shared venue list.
type VenueFilter struct {
	Query         string
	Country       string
	IncludeClosed bool
	Limit         int64
	Offset        int64
}

// ListVenues searches cinemas by name, chain or city.
func (r *TrackingRepository) ListVenues(ctx context.Context, f VenueFilter) ([]Venue, error) {
	rows, err := r.q.ListVenues(ctx, sqlc.ListVenuesParams{
		Query: f.Query, Country: f.Country, IncludeClosed: boolInt(f.IncludeClosed),
		Limit: f.Limit, Offset: f.Offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list venues: %w", err)
	}
	out := make([]Venue, 0, len(rows))
	for _, row := range rows {
		out = append(out, Venue{
			ID: row.ID, Name: row.Name, Chain: nullStringPtr(row.Chain), Address: nullStringPtr(row.Address),
			City: nullStringPtr(row.City), Region: nullStringPtr(row.Region), Country: nullStringPtr(row.Country),
			Latitude: nullFloatPtr(row.Latitude), Longitude: nullFloatPtr(row.Longitude),
			Timezone: nullStringPtr(row.Timezone), OpenedOn: nullStringPtr(row.OpenedOn),
			ClosedOn: nullStringPtr(row.ClosedOn), Status: row.Status,
			ReplacedByVenueID: nullInt64Ptr(row.ReplacedByVenueID), Note: nullStringPtr(row.Note),
			CreatedBy: nullInt64Ptr(row.CreatedBy), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			ScreenCount: row.ScreenCount,
		})
	}
	return out, nil
}

// VenueWatchCount is how many watches point at a venue.
func (r *TrackingRepository) VenueWatchCount(ctx context.Context, id int64) (int64, error) {
	n, err := r.q.CountVenueWatches(ctx, sql.NullInt64{Int64: id, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("count watches at venue %d: %w", id, err)
	}
	return n, nil
}

// DeleteVenue removes a cinema. Callers check VenueWatchCount first.
func (r *TrackingRepository) DeleteVenue(ctx context.Context, id int64) (bool, error) {
	n, err := r.q.DeleteVenue(ctx, id)
	if err != nil {
		return false, fmt.Errorf("delete venue %d: %w", id, err)
	}
	return n > 0, nil
}

// VenueAlias is a name a venue used to have.
type VenueAlias struct {
	ID       int64   `json:"id"`
	VenueID  int64   `json:"venue_id"`
	Name     string  `json:"name"`
	FromDate *string `json:"from_date,omitempty"`
	ToDate   *string `json:"to_date,omitempty"`
}

// AddVenueAlias records a former name.
func (r *TrackingRepository) AddVenueAlias(ctx context.Context, a VenueAlias) (VenueAlias, error) {
	row, err := r.q.AddVenueAlias(ctx, sqlc.AddVenueAliasParams{
		VenueID: a.VenueID, Name: a.Name,
		FromDate: toNullString(a.FromDate), ToDate: toNullString(a.ToDate)})
	if err != nil {
		return VenueAlias{}, fmt.Errorf("add alias to venue %d: %w", a.VenueID, err)
	}
	return VenueAlias{ID: row.ID, VenueID: row.VenueID, Name: row.Name,
		FromDate: nullStringPtr(row.FromDate), ToDate: nullStringPtr(row.ToDate)}, nil
}

// VenueAliases lists a venue's former names.
func (r *TrackingRepository) VenueAliases(ctx context.Context, venueID int64) ([]VenueAlias, error) {
	rows, err := r.q.ListVenueAliases(ctx, venueID)
	if err != nil {
		return nil, fmt.Errorf("list aliases of venue %d: %w", venueID, err)
	}
	out := make([]VenueAlias, 0, len(rows))
	for _, row := range rows {
		out = append(out, VenueAlias{ID: row.ID, VenueID: row.VenueID, Name: row.Name,
			FromDate: nullStringPtr(row.FromDate), ToDate: nullStringPtr(row.ToDate)})
	}
	return out, nil
}

// DeleteVenueAlias removes a former name.
func (r *TrackingRepository) DeleteVenueAlias(ctx context.Context, venueID, id int64) (bool, error) {
	n, err := r.q.DeleteVenueAlias(ctx, sqlc.DeleteVenueAliasParams{ID: id, VenueID: venueID})
	if err != nil {
		return false, fmt.Errorf("delete alias %d: %w", id, err)
	}
	return n > 0, nil
}

// VenueRef is a venue's id in an outside source.
type VenueRef struct {
	Source string `json:"source"`
	Ref    string `json:"ref"`
}

// SaveVenueRef records a venue's id in an outside source.
func (r *TrackingRepository) SaveVenueRef(ctx context.Context, venueID int64, ref VenueRef) error {
	if err := r.q.UpsertVenueRef(ctx, sqlc.UpsertVenueRefParams{
		VenueID: venueID, Source: ref.Source, Ref: ref.Ref}); err != nil {
		return fmt.Errorf("save %s ref of venue %d: %w", ref.Source, venueID, err)
	}
	return nil
}

// VenueRefs lists a venue's outside ids.
func (r *TrackingRepository) VenueRefs(ctx context.Context, venueID int64) ([]VenueRef, error) {
	rows, err := r.q.ListVenueRefs(ctx, venueID)
	if err != nil {
		return nil, fmt.Errorf("list refs of venue %d: %w", venueID, err)
	}
	out := make([]VenueRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, VenueRef{Source: row.Source, Ref: row.Ref})
	}
	return out, nil
}

// VenueByRef finds a venue already stored under an outside id, so the same
// cinema is not added twice.
func (r *TrackingRepository) VenueByRef(ctx context.Context, source, ref string) (int64, error) {
	id, err := r.q.FindVenueByRef(ctx, sqlc.FindVenueByRefParams{Source: source, Ref: ref})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// VenueScreen is one auditorium. Geometry belongs here rather than on each
// visit, so it is entered once and screens can be compared.
type VenueScreen struct {
	ID            int64    `json:"id"`
	VenueID       int64    `json:"venue_id"`
	Name          string   `json:"name"`
	Number        *int64   `json:"number,omitempty"`
	Seats         *int64   `json:"seats,omitempty"`
	Width         *float64 `json:"width,omitempty"`
	Height        *float64 `json:"height,omitempty"`
	DimUnit       *string  `json:"dim_unit,omitempty"`
	WidthM        *float64 `json:"width_m,omitempty"`
	HeightM       *float64 `json:"height_m,omitempty"`
	AspectRatio   *float64 `json:"aspect_ratio,omitempty"`
	EffectiveFrom *string  `json:"effective_from,omitempty"`
	EffectiveTo   *string  `json:"effective_to,omitempty"`
	Note          *string  `json:"note,omitempty"`
	CreatedAt     string   `json:"created_at,omitempty"`
	UpdatedAt     string   `json:"updated_at,omitempty"`
}

func screenParams(s VenueScreen) (sql.NullInt64, sql.NullInt64, sql.NullFloat64, sql.NullFloat64,
	sql.NullString, sql.NullFloat64, sql.NullFloat64, sql.NullFloat64, sql.NullString, sql.NullString, sql.NullString) {
	return toNullInt64(s.Number), toNullInt64(s.Seats), floatPtrNull(s.Width), floatPtrNull(s.Height),
		toNullString(s.DimUnit), floatPtrNull(s.WidthM), floatPtrNull(s.HeightM), floatPtrNull(s.AspectRatio),
		toNullString(s.EffectiveFrom), toNullString(s.EffectiveTo), toNullString(s.Note)
}

// CreateVenueScreen adds an auditorium.
func (r *TrackingRepository) CreateVenueScreen(ctx context.Context, s VenueScreen) (int64, error) {
	number, seats, width, height, unit, widthM, heightM, ratio, from, to, note := screenParams(s)
	id, err := r.q.CreateVenueScreen(ctx, sqlc.CreateVenueScreenParams{
		VenueID: s.VenueID, Name: s.Name, Number: number, Seats: seats,
		Width: width, Height: height, DimUnit: unit, WidthM: widthM, HeightM: heightM,
		AspectRatio: ratio, EffectiveFrom: from, EffectiveTo: to, Note: note,
	})
	if err != nil {
		return 0, fmt.Errorf("create screen at venue %d: %w", s.VenueID, err)
	}
	return id, nil
}

// UpdateVenueScreen edits an auditorium.
func (r *TrackingRepository) UpdateVenueScreen(ctx context.Context, s VenueScreen) (bool, error) {
	number, seats, width, height, unit, widthM, heightM, ratio, from, to, note := screenParams(s)
	n, err := r.q.UpdateVenueScreen(ctx, sqlc.UpdateVenueScreenParams{
		ID: s.ID, VenueID: s.VenueID, Name: s.Name, Number: number, Seats: seats,
		Width: width, Height: height, DimUnit: unit, WidthM: widthM, HeightM: heightM,
		AspectRatio: ratio, EffectiveFrom: from, EffectiveTo: to, Note: note,
	})
	if err != nil {
		return false, fmt.Errorf("update screen %d: %w", s.ID, err)
	}
	return n > 0, nil
}

func screenFromRow(id, venueID int64, name string, number, seats sql.NullInt64,
	width, height sql.NullFloat64, unit sql.NullString, widthM, heightM, ratio sql.NullFloat64,
	from, to, note sql.NullString, createdAt, updatedAt string) VenueScreen {
	return VenueScreen{
		ID: id, VenueID: venueID, Name: name, Number: nullInt64Ptr(number), Seats: nullInt64Ptr(seats),
		Width: nullFloatPtr(width), Height: nullFloatPtr(height), DimUnit: nullStringPtr(unit),
		WidthM: nullFloatPtr(widthM), HeightM: nullFloatPtr(heightM), AspectRatio: nullFloatPtr(ratio),
		EffectiveFrom: nullStringPtr(from), EffectiveTo: nullStringPtr(to), Note: nullStringPtr(note),
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
}

// VenueScreens lists a venue's auditoriums.
func (r *TrackingRepository) VenueScreens(ctx context.Context, venueID int64) ([]VenueScreen, error) {
	rows, err := r.q.ListVenueScreens(ctx, venueID)
	if err != nil {
		return nil, fmt.Errorf("list screens of venue %d: %w", venueID, err)
	}
	out := make([]VenueScreen, 0, len(rows))
	for _, row := range rows {
		out = append(out, screenFromRow(row.ID, row.VenueID, row.Name, row.Number, row.Seats,
			row.Width, row.Height, row.DimUnit, row.WidthM, row.HeightM, row.AspectRatio,
			row.EffectiveFrom, row.EffectiveTo, row.Note, row.CreatedAt, row.UpdatedAt))
	}
	return out, nil
}

// VenueScreen returns one auditorium, or sql.ErrNoRows.
func (r *TrackingRepository) VenueScreen(ctx context.Context, id int64) (VenueScreen, error) {
	row, err := r.q.GetVenueScreen(ctx, id)
	if err != nil {
		return VenueScreen{}, err
	}
	return screenFromRow(row.ID, row.VenueID, row.Name, row.Number, row.Seats,
		row.Width, row.Height, row.DimUnit, row.WidthM, row.HeightM, row.AspectRatio,
		row.EffectiveFrom, row.EffectiveTo, row.Note, row.CreatedAt, row.UpdatedAt), nil
}

// DeleteVenueScreen removes an auditorium.
func (r *TrackingRepository) DeleteVenueScreen(ctx context.Context, venueID, id int64) (bool, error) {
	n, err := r.q.DeleteVenueScreen(ctx, sqlc.DeleteVenueScreenParams{ID: id, VenueID: venueID})
	if err != nil {
		return false, fmt.Errorf("delete screen %d: %w", id, err)
	}
	return n > 0, nil
}
