package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

type RatingsRepository struct {
	db *sql.DB
	q  *sqlc.Queries
}

func NewRatingsRepository(db *sql.DB) *RatingsRepository {
	return &RatingsRepository{db: db, q: sqlc.New(db)}
}

// TitleRatings is the identity of a rated TMDB title: the IDs used to look it
// up again at each provider. Nil pointers mean unknown.
type TitleRatings struct {
	TmdbKind    string // "movie" | "tv"
	TmdbID      string
	Title       *string
	Year        *int64
	ImdbID      *string
	JustWatchID *string
	RTSlug      *string // "m/<vanity>" or "tv/<vanity>"
	RefreshedAt time.Time
}

// RatingSource is one rating as a provider reported it.
type RatingSource struct {
	Provider  string // "justwatch" | "mdblist" | "rottentomatoes"
	Source    string // e.g. "imdb", "tomatoes", "popcorn", "metacritic"
	Value     float64
	Votes     *int64
	URL       *string
	FetchedAt time.Time
}

// GetTitleRatings returns the cached identity and all stored rating sources,
// or (nil, nil, nil) if the title was never rated.
func (r *RatingsRepository) GetTitleRatings(ctx context.Context, tmdbKind, tmdbID string) (*TitleRatings, []RatingSource, error) {
	row, err := r.q.GetTitleRatings(ctx, sqlc.GetTitleRatingsParams{TmdbKind: tmdbKind, TmdbID: tmdbID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get title ratings: %w", err)
	}
	sources, err := r.listSources(ctx, r.q, tmdbKind, tmdbID)
	if err != nil {
		return nil, nil, err
	}
	return ratingsFromRow(row), sources, nil
}

// SaveTitleRatings upserts the title identity and, for each provider in
// sourcesByProvider, replaces that provider's stored ratings. Providers not
// in the map (e.g. ones that failed this refresh) keep their previous
// ratings. Returns the saved identity and all stored sources.
func (r *RatingsRepository) SaveTitleRatings(ctx context.Context, in TitleRatings, sourcesByProvider map[string][]RatingSource) (*TitleRatings, []RatingSource, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("begin save ratings: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)

	row, err := q.UpsertTitleRatings(ctx, sqlc.UpsertTitleRatingsParams{
		TmdbKind:    in.TmdbKind,
		TmdbID:      in.TmdbID,
		Title:       toNullString(in.Title),
		Year:        toNullInt64(in.Year),
		ImdbID:      toNullString(in.ImdbID),
		JustwatchID: toNullString(in.JustWatchID),
		RtSlug:      toNullString(in.RTSlug),
		RefreshedAt: in.RefreshedAt.UTC(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("upsert title ratings: %w", err)
	}

	for provider, sources := range sourcesByProvider {
		if err := q.DeleteTitleRatingSourcesByProvider(ctx, sqlc.DeleteTitleRatingSourcesByProviderParams{
			TmdbKind: in.TmdbKind, TmdbID: in.TmdbID, Provider: provider,
		}); err != nil {
			return nil, nil, fmt.Errorf("delete %s rating sources: %w", provider, err)
		}
		for _, s := range sources {
			if err := q.InsertTitleRatingSource(ctx, sqlc.InsertTitleRatingSourceParams{
				TmdbKind:  in.TmdbKind,
				TmdbID:    in.TmdbID,
				Provider:  provider,
				Source:    s.Source,
				Value:     s.Value,
				Votes:     toNullInt64(s.Votes),
				Url:       toNullString(s.URL),
				FetchedAt: s.FetchedAt.UTC(),
			}); err != nil {
				return nil, nil, fmt.Errorf("insert %s rating source %s: %w", provider, s.Source, err)
			}
		}
	}

	sources, err := r.listSources(ctx, q, in.TmdbKind, in.TmdbID)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit save ratings: %w", err)
	}
	return ratingsFromRow(row), sources, nil
}

func (r *RatingsRepository) listSources(ctx context.Context, q *sqlc.Queries, tmdbKind, tmdbID string) ([]RatingSource, error) {
	rows, err := q.ListTitleRatingSources(ctx, sqlc.ListTitleRatingSourcesParams{TmdbKind: tmdbKind, TmdbID: tmdbID})
	if err != nil {
		return nil, fmt.Errorf("list title rating sources: %w", err)
	}
	out := make([]RatingSource, 0, len(rows))
	for _, row := range rows {
		out = append(out, RatingSource{
			Provider:  row.Provider,
			Source:    row.Source,
			Value:     row.Value,
			Votes:     nullInt64Ptr(row.Votes),
			URL:       nullStringPtr(row.Url),
			FetchedAt: row.FetchedAt,
		})
	}
	return out, nil
}

// FillTitleExternalIDs sets imdb_id / rt_url on scraped titles with this TMDB
// ID and kind ("movie" | "tv_show") where they are still empty.
func (r *RatingsRepository) FillTitleExternalIDs(ctx context.Context, titleKind, tmdbID string, imdbID, rtURL *string) error {
	if imdbID == nil && rtURL == nil {
		return nil
	}
	err := r.q.FillTitleExternalIDs(ctx, sqlc.FillTitleExternalIDsParams{
		ImdbID: toNullString(imdbID),
		RtUrl:  toNullString(rtURL),
		TmdbID: sql.NullString{String: tmdbID, Valid: true},
		Kind:   titleKind,
	})
	if err != nil {
		return fmt.Errorf("fill title external ids: %w", err)
	}
	return nil
}

func ratingsFromRow(row sqlc.TitleRating) *TitleRatings {
	return &TitleRatings{
		TmdbKind:    row.TmdbKind,
		TmdbID:      row.TmdbID,
		Title:       nullStringPtr(row.Title),
		Year:        nullInt64Ptr(row.Year),
		ImdbID:      nullStringPtr(row.ImdbID),
		JustWatchID: nullStringPtr(row.JustwatchID),
		RTSlug:      nullStringPtr(row.RtSlug),
		RefreshedAt: row.RefreshedAt,
	}
}

func toNullString(p *string) sql.NullString {
	if p == nil || *p == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

func toNullInt64(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}
