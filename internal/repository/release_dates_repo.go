package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// ReleaseDatesRepository reads TMDB release dates of movies and the
// observations of when they reached streaming.
type ReleaseDatesRepository struct {
	q *sqlc.Queries
}

func NewReleaseDatesRepository(db *sql.DB) *ReleaseDatesRepository {
	return &ReleaseDatesRepository{q: sqlc.New(db)}
}

// releaseIDBatch: movie IDs per IN (...) list.
const releaseIDBatch = 500

// saveReleaseDates replaces a movie's release dates (inside SaveDetails'
// transaction); a response without release dates keeps the stored ones.
func saveReleaseDates(ctx context.Context, q *sqlc.Queries, ref TmdbRef, fetched bool, dates []client.TMDBReleaseDate) error {
	if ref.Kind != "movie" || !fetched {
		return nil
	}
	if err := q.DeleteReleaseDates(ctx, sqlc.DeleteReleaseDatesParams{TmdbKind: ref.Kind, TmdbID: ref.ID}); err != nil {
		return fmt.Errorf("delete release dates: %w", err)
	}
	for _, d := range dates {
		if err := q.InsertReleaseDate(ctx, sqlc.InsertReleaseDateParams{TmdbKind: ref.Kind, TmdbID: ref.ID, Country: d.Country,
			ReleaseType: int64(d.Type), ReleaseDate: d.Date, Note: nullIfEmpty(d.Note)}); err != nil {
			return fmt.Errorf("insert release date: %w", err)
		}
	}
	return nil
}

// ReleaseDate is one stored release of a movie.
type ReleaseDate struct {
	TmdbID     string
	Title      string
	PosterPath string
	Country    string
	Type       int
	Date       string // YYYY-MM-DD
	Note       string
}

// MovieReleaseDates lists a movie's release dates in every country.
func (r *ReleaseDatesRepository) MovieReleaseDates(ctx context.Context, tmdbID string) ([]ReleaseDate, error) {
	rows, err := r.q.ListMovieReleaseDates(ctx, tmdbID)
	if err != nil {
		return nil, fmt.Errorf("list release dates: %w", err)
	}
	out := make([]ReleaseDate, 0, len(rows))
	for _, row := range rows {
		out = append(out, ReleaseDate{TmdbID: tmdbID, Country: row.Country, Type: int(row.ReleaseType), Date: row.ReleaseDate, Note: row.Note})
	}
	return out, nil
}

// TheatricalReleaseDates lists every release date in a country of the
// movies released in theaters there (limited or wide) between two dates.
func (r *ReleaseDatesRepository) TheatricalReleaseDates(ctx context.Context, country, from, to string) ([]ReleaseDate, error) {
	rows, err := r.q.ListTheatricalReleaseDates(ctx, sqlc.ListTheatricalReleaseDatesParams{Country: country, FromDate: from, ToDate: to})
	if err != nil {
		return nil, fmt.Errorf("list theatrical release dates: %w", err)
	}
	out := make([]ReleaseDate, 0, len(rows))
	for _, row := range rows {
		out = append(out, ReleaseDate{TmdbID: row.TmdbID, Title: row.Title, PosterPath: row.PosterPath, Country: country,
			Type: int(row.ReleaseType), Date: row.ReleaseDate, Note: row.Note})
	}
	return out, nil
}

// StreamingOffer is a subscription / free / ads offer of a movie: current,
// or the first time it was seen arriving (FirstSeen).
type StreamingOffer struct {
	TmdbID       string
	Country      string
	ProviderID   int64
	ProviderName string
	Monetization string
	FirstSeen    string // YYYY-MM-DD; "" for current offers
}

// FirstStreamingOffers returns the first arrival of each streaming offer of
// the movies (country "" = every country).
func (r *ReleaseDatesRepository) FirstStreamingOffers(ctx context.Context, country string, ids []string) ([]StreamingOffer, error) {
	var out []StreamingOffer
	for _, batch := range batches(ids, releaseIDBatch) {
		rows, err := r.q.ListFirstStreamingOffers(ctx, sqlc.ListFirstStreamingOffersParams{Country: country, Ids: batch})
		if err != nil {
			return nil, fmt.Errorf("list first streaming offers: %w", err)
		}
		for _, row := range rows {
			out = append(out, StreamingOffer{TmdbID: row.TmdbID, Country: row.Country, ProviderID: row.ProviderID,
				ProviderName: row.ProviderName, Monetization: row.Monetization, FirstSeen: row.FirstSeen})
		}
	}
	return out, nil
}

// CurrentStreamingOffers returns the movies' current streaming offers
// (country "" = every country).
func (r *ReleaseDatesRepository) CurrentStreamingOffers(ctx context.Context, country string, ids []string) ([]StreamingOffer, error) {
	var out []StreamingOffer
	for _, batch := range batches(ids, releaseIDBatch) {
		rows, err := r.q.ListMovieStreamingOffers(ctx, sqlc.ListMovieStreamingOffersParams{Country: country, Ids: batch})
		if err != nil {
			return nil, fmt.Errorf("list streaming offers: %w", err)
		}
		for _, row := range rows {
			out = append(out, StreamingOffer{TmdbID: row.TmdbID, Country: row.Country, ProviderID: row.ProviderID,
				ProviderName: row.ProviderName, Monetization: row.Monetization})
		}
	}
	return out, nil
}

// FirstChart is a movie's first FlixPatrol chart date in a country on a
// provider, and on how many of the 14 days before it that chart was stored.
type FirstChart struct {
	TmdbID, Country, Provider, Date string
	StoredBefore                    int
}

// FirstMovieCharts returns the movies' first chart dates (country "" =
// every country).
func (r *ReleaseDatesRepository) FirstMovieCharts(ctx context.Context, country string, ids []string) ([]FirstChart, error) {
	var out []FirstChart
	for _, batch := range batches(ids, releaseIDBatch) {
		nb := make([]sql.NullString, len(batch))
		for i, id := range batch {
			nb[i] = sql.NullString{String: id, Valid: true}
		}
		rows, err := r.q.ListFirstMovieCharts(ctx, sqlc.ListFirstMovieChartsParams{Country: country, Ids: nb})
		if err != nil {
			return nil, fmt.Errorf("list first movie charts: %w", err)
		}
		for _, row := range rows {
			out = append(out, FirstChart{TmdbID: row.TmdbID, Country: row.Country, Provider: row.StreamingProvider, Date: row.FirstChart,
				StoredBefore: int(row.StoredBefore)})
		}
	}
	return out, nil
}

func batches(ids []string, n int) [][]string {
	var out [][]string
	for len(ids) > 0 {
		k := min(n, len(ids))
		out = append(out, ids[:k])
		ids = ids[k:]
	}
	return out
}
