package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// MetadataRepository stores TMDB metadata, watch providers and Wikidata IDs
// per TMDB title.
type MetadataRepository struct {
	db *sql.DB
	q  *sqlc.Queries
}

func NewMetadataRepository(db *sql.DB) *MetadataRepository {
	return &MetadataRepository{db: db, q: sqlc.New(db)}
}

// TmdbRef identifies a TMDB title: Kind is "movie" or "tv".
type TmdbRef struct {
	Kind string
	ID   string
}

func (r TmdbRef) String() string { return r.Kind + ":" + r.ID }

// SaveDetails stores a title's TMDB details and replaces its watch providers.
func (r *MetadataRepository) SaveDetails(ctx context.Context, d *client.TMDBFullDetails) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)

	var digital sql.NullString
	if len(d.DigitalReleaseDates) > 0 {
		digital = jsonString(d.DigitalReleaseDates)
	}
	if err := q.UpsertTmdbTitleDetails(ctx, sqlc.UpsertTmdbTitleDetailsParams{
		TmdbKind: d.Kind, TmdbID: d.ID,
		Title: nullIfEmpty(d.Title), OriginalTitle: nullIfEmpty(d.OriginalTitle), OriginalLanguage: nullIfEmpty(d.OriginalLanguage),
		Overview: nullIfEmpty(d.Overview), Status: nullIfEmpty(d.Status), ReleaseDate: nullIfEmpty(d.ReleaseDate),
		LastAirDate: nullIfEmpty(d.LastAirDate), Runtime: nullIfZero(d.Runtime),
		NumberOfSeasons: nullIfZero(d.NumberOfSeasons), NumberOfEpisodes: nullIfZero(d.NumberOfEpisodes),
		Genres: jsonList(d.Genres), OriginCountries: jsonList(d.OriginCountries),
		ProductionCompanies: jsonList(d.ProductionCompanies), Networks: jsonList(d.Networks),
		PosterPath: nullIfEmpty(d.PosterPath), BackdropPath: nullIfEmpty(d.BackdropPath),
		Popularity: sql.NullFloat64{Float64: d.Popularity, Valid: true}, VoteAverage: sql.NullFloat64{Float64: d.VoteAverage, Valid: true},
		VoteCount: sql.NullInt64{Int64: int64(d.VoteCount), Valid: true}, DigitalReleaseDates: digital,
		ImdbID: nullIfEmpty(d.IMDbID), WikidataID: nullIfEmpty(d.WikidataID),
	}); err != nil {
		return fmt.Errorf("upsert tmdb title: %w", err)
	}
	if err := q.DeleteTmdbWatchProviders(ctx, sqlc.DeleteTmdbWatchProvidersParams{TmdbKind: d.Kind, TmdbID: d.ID}); err != nil {
		return fmt.Errorf("delete watch providers: %w", err)
	}
	for _, p := range d.WatchProviders {
		if err := q.InsertTmdbWatchProvider(ctx, sqlc.InsertTmdbWatchProviderParams{
			TmdbKind: d.Kind, TmdbID: d.ID, Country: p.Country, Monetization: p.Monetization,
			ProviderID: int64(p.ProviderID), ProviderName: p.ProviderName, LogoPath: nullIfEmpty(p.LogoPath),
			DisplayPriority: sql.NullInt64{Int64: int64(p.DisplayPriority), Valid: true}, Link: nullIfEmpty(p.Link),
		}); err != nil {
			return fmt.Errorf("insert watch provider: %w", err)
		}
	}
	return tx.Commit()
}

// SaveWikidata stores IDs found on Wikidata (or records an empty lookup).
func (r *MetadataRepository) SaveWikidata(ctx context.Context, ref TmdbRef, ids client.WikidataIDs) error {
	return r.q.UpsertTmdbTitleWikidata(ctx, sqlc.UpsertTmdbTitleWikidataParams{
		TmdbKind: ref.Kind, TmdbID: ref.ID, ImdbID: nullIfEmpty(ids.IMDbID), WikidataID: nullIfEmpty(ids.WikidataID),
		RtID: nullIfEmpty(ids.RottenTomato), MetacriticID: nullIfEmpty(ids.Metacritic), LetterboxdID: nullIfEmpty(ids.Letterboxd),
	})
}

// TmdbMetadata is stored TMDB metadata, decoded.
type TmdbMetadata struct {
	Kind                string            `json:"tmdb_kind"`
	ID                  string            `json:"tmdb_id"`
	Title               *string           `json:"title"`
	OriginalTitle       *string           `json:"original_title"`
	OriginalLanguage    *string           `json:"original_language"`
	Overview            *string           `json:"overview"`
	Status              *string           `json:"status"`
	ReleaseDate         *string           `json:"release_date"`
	LastAirDate         *string           `json:"last_air_date,omitempty"`
	Runtime             *int64            `json:"runtime"`
	NumberOfSeasons     *int64            `json:"number_of_seasons,omitempty"`
	NumberOfEpisodes    *int64            `json:"number_of_episodes,omitempty"`
	Genres              []string          `json:"genres"`
	OriginCountries     []string          `json:"origin_countries"`
	ProductionCompanies []string          `json:"production_companies"`
	Networks            []string          `json:"networks,omitempty"`
	PosterPath          *string           `json:"poster_path"`
	BackdropPath        *string           `json:"backdrop_path"`
	Popularity          *float64          `json:"popularity"`
	VoteAverage         *float64          `json:"vote_average"`
	VoteCount           *int64            `json:"vote_count"`
	DigitalReleaseDates map[string]string `json:"digital_release_dates,omitempty"`
	IMDbID              *string           `json:"imdb_id"`
	WikidataID          *string           `json:"wikidata_id"`
	RottenTomatoesID    *string           `json:"rotten_tomatoes_id"`
	MetacriticID        *string           `json:"metacritic_id"`
	LetterboxdID        *string           `json:"letterboxd_id"`
	DetailsFetchedAt    *time.Time        `json:"details_fetched_at"`
}

func toTmdbMetadata(t sqlc.TmdbTitle) *TmdbMetadata {
	m := &TmdbMetadata{
		Kind: t.TmdbKind, ID: t.TmdbID,
		Title: nullStringPtr(t.Title), OriginalTitle: nullStringPtr(t.OriginalTitle), OriginalLanguage: nullStringPtr(t.OriginalLanguage),
		Overview: nullStringPtr(t.Overview), Status: nullStringPtr(t.Status), ReleaseDate: nullStringPtr(t.ReleaseDate),
		LastAirDate: nullStringPtr(t.LastAirDate), Runtime: nullInt64Ptr(t.Runtime),
		NumberOfSeasons: nullInt64Ptr(t.NumberOfSeasons), NumberOfEpisodes: nullInt64Ptr(t.NumberOfEpisodes),
		Genres: decodeList(t.Genres), OriginCountries: decodeList(t.OriginCountries),
		ProductionCompanies: decodeList(t.ProductionCompanies), Networks: decodeList(t.Networks),
		PosterPath: nullStringPtr(t.PosterPath), BackdropPath: nullStringPtr(t.BackdropPath),
		Popularity: nullFloatPtr(t.Popularity), VoteAverage: nullFloatPtr(t.VoteAverage), VoteCount: nullInt64Ptr(t.VoteCount),
		IMDbID: nullStringPtr(t.ImdbID), WikidataID: nullStringPtr(t.WikidataID), RottenTomatoesID: nullStringPtr(t.RtID),
		MetacriticID: nullStringPtr(t.MetacriticID), LetterboxdID: nullStringPtr(t.LetterboxdID),
	}
	if t.DigitalReleaseDates.Valid {
		_ = json.Unmarshal([]byte(t.DigitalReleaseDates.String), &m.DigitalReleaseDates)
	}
	if t.DetailsFetchedAt.Valid {
		v := t.DetailsFetchedAt.Time.UTC()
		m.DetailsFetchedAt = &v
	}
	return m
}

// Get returns a title's stored metadata, or nil if there is none.
func (r *MetadataRepository) Get(ctx context.Context, ref TmdbRef) (*TmdbMetadata, error) {
	t, err := r.q.GetTmdbTitle(ctx, sqlc.GetTmdbTitleParams{TmdbKind: ref.Kind, TmdbID: ref.ID})
	if IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tmdb title: %w", err)
	}
	return toTmdbMetadata(t), nil
}

// GetMany returns stored metadata keyed by ref.String().
func (r *MetadataRepository) GetMany(ctx context.Context, refs []TmdbRef) (map[string]*TmdbMetadata, error) {
	out := map[string]*TmdbMetadata{}
	if len(refs) == 0 {
		return out, nil
	}
	want := make(map[string]bool, len(refs))
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		want[ref.String()] = true
		ids = append(ids, ref.ID)
	}
	rows, err := r.q.ListTmdbTitlesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list tmdb titles: %w", err)
	}
	for _, row := range rows {
		m := toTmdbMetadata(row)
		if key := (TmdbRef{Kind: m.Kind, ID: m.ID}).String(); want[key] {
			out[key] = m
		}
	}
	return out, nil
}

// WatchProvider is one way to watch a title in a country.
type WatchProvider struct {
	Country         string    `json:"country"`
	Monetization    string    `json:"monetization"`
	ProviderID      int64     `json:"provider_id"`
	ProviderName    string    `json:"provider_name"`
	LogoPath        *string   `json:"logo_path"`
	DisplayPriority *int64    `json:"display_priority"`
	Link            *string   `json:"link"`
	FetchedAt       time.Time `json:"fetched_at"`
}

// WatchProviders lists where a title can be watched (country "" = all).
func (r *MetadataRepository) WatchProviders(ctx context.Context, ref TmdbRef, country string) ([]WatchProvider, error) {
	rows, err := r.q.ListTmdbWatchProviders(ctx, sqlc.ListTmdbWatchProvidersParams{TmdbKind: ref.Kind, TmdbID: ref.ID, Country: strings.ToUpper(country)})
	if err != nil {
		return nil, fmt.Errorf("list watch providers: %w", err)
	}
	out := make([]WatchProvider, 0, len(rows))
	for _, row := range rows {
		out = append(out, WatchProvider{
			Country: row.Country, Monetization: row.Monetization, ProviderID: row.ProviderID, ProviderName: row.ProviderName,
			LogoPath: nullStringPtr(row.LogoPath), DisplayPriority: nullInt64Ptr(row.DisplayPriority), Link: nullStringPtr(row.Link),
			FetchedAt: row.FetchedAt.UTC(),
		})
	}
	return out, nil
}

// RefsDueForDetails lists matched TMDB titles whose details are missing or
// stale: older than recentWindow for titles charting in the last 30 days,
// else older than staleWindow (SQLite modifiers such as "-3 days").
func (r *MetadataRepository) RefsDueForDetails(ctx context.Context, recentWindow, staleWindow string, limit int64) ([]TmdbRef, error) {
	rows, err := r.q.ListTmdbRefsDueForDetails(ctx, sqlc.ListTmdbRefsDueForDetailsParams{RecentWindow: recentWindow, StaleWindow: staleWindow, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("list tmdb refs due for details: %w", err)
	}
	out := make([]TmdbRef, 0, len(rows))
	for _, row := range rows {
		if row.TmdbID.Valid && row.TmdbID.String != "" {
			out = append(out, TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID.String})
		}
	}
	return out, nil
}

// RefsDueForWikidata lists matched TMDB titles not looked up on Wikidata
// within window (a SQLite modifier such as "-30 days").
func (r *MetadataRepository) RefsDueForWikidata(ctx context.Context, window string, limit int64) ([]TmdbRef, error) {
	rows, err := r.q.ListTmdbRefsDueForWikidata(ctx, sqlc.ListTmdbRefsDueForWikidataParams{Window: window, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("list tmdb refs due for wikidata: %w", err)
	}
	out := make([]TmdbRef, 0, len(rows))
	for _, row := range rows {
		if row.TmdbID.Valid && row.TmdbID.String != "" {
			out = append(out, TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID.String})
		}
	}
	return out, nil
}

// MetadataCounts summarises stored metadata.
type MetadataCounts struct {
	Titles       int64 `json:"titles"`
	WithDetails  int64 `json:"with_details"`
	WithWikidata int64 `json:"with_wikidata"`
}

func (r *MetadataRepository) Counts(ctx context.Context) (MetadataCounts, error) {
	row, err := r.q.CountTmdbTitles(ctx)
	if err != nil {
		return MetadataCounts{}, fmt.Errorf("count tmdb titles: %w", err)
	}
	return MetadataCounts{Titles: row.Titles, WithDetails: row.WithDetails, WithWikidata: row.WithWikidata}, nil
}

func jsonList(v []string) sql.NullString {
	if len(v) == 0 {
		return sql.NullString{}
	}
	return jsonString(v)
}

func jsonString(v any) sql.NullString {
	b, err := json.Marshal(v)
	if err != nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(b), Valid: true}
}

func decodeList(s sql.NullString) []string {
	out := []string{}
	if s.Valid {
		_ = json.Unmarshal([]byte(s.String), &out)
	}
	return out
}

func nullIfZero(v int) sql.NullInt64 { return sql.NullInt64{Int64: int64(v), Valid: v != 0} }

func nullFloatPtr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}
