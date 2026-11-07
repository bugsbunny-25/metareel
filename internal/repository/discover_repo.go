package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// DiscoverRepository reads ratings, offers and offer changes for the
// discovery endpoints.
type DiscoverRepository struct {
	q *sqlc.Queries
}

func NewDiscoverRepository(db *sql.DB) *DiscoverRepository {
	return &DiscoverRepository{q: sqlc.New(db)}
}

// RatedTitle is a TMDB title with its IMDb rating (IMDb's dataset).
type RatedTitle struct {
	Ref         TmdbRef
	Title       string
	ReleaseDate string
	PosterPath  string
	Genres      []string
	Rating      float64
	Votes       int64
}

// IMDbRatedTitles lists every TMDB title with an IMDb rating.
func (r *DiscoverRepository) IMDbRatedTitles(ctx context.Context) ([]RatedTitle, error) {
	rows, err := r.q.ListIMDbRatedTitles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rated titles: %w", err)
	}
	out := make([]RatedTitle, 0, len(rows))
	for _, row := range rows {
		out = append(out, RatedTitle{Ref: TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID}, Title: row.Title, ReleaseDate: row.ReleaseDate,
			PosterPath: row.PosterPath, Genres: decodeList(row.Genres), Rating: row.AverageRating, Votes: row.NumVotes})
	}
	return out, nil
}

// CountryOffer is one offer of a title in a country.
type CountryOffer struct {
	Ref          TmdbRef
	ProviderID   int64
	ProviderName string
	Monetization string // flatrate | free | ads | rent | buy
}

// CountryOffers lists every offer in a country.
func (r *DiscoverRepository) CountryOffers(ctx context.Context, country string) ([]CountryOffer, error) {
	rows, err := r.q.ListCountryOffers(ctx, country)
	if err != nil {
		return nil, fmt.Errorf("list country offers: %w", err)
	}
	out := make([]CountryOffer, 0, len(rows))
	for _, row := range rows {
		out = append(out, CountryOffer{Ref: TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID}, ProviderID: row.ProviderID,
			ProviderName: row.ProviderName, Monetization: row.Monetization})
	}
	return out, nil
}

// OfferChange is a streaming offer that arrived or left.
type OfferChange struct {
	CountryOffer
	Event      string // added | removed
	ObservedAt string // 'YYYY-MM-DD HH:MM:SS' UTC
	Title      string
}

// CountryOfferChanges lists subscription / free / ads offer changes in a
// country since a time ('YYYY-MM-DD HH:MM:SS' UTC), newest first.
func (r *DiscoverRepository) CountryOfferChanges(ctx context.Context, country, since string) ([]OfferChange, error) {
	rows, err := r.q.ListCountryOfferEvents(ctx, sqlc.ListCountryOfferEventsParams{Country: country, Since: since})
	if err != nil {
		return nil, fmt.Errorf("list country offer events: %w", err)
	}
	out := make([]OfferChange, 0, len(rows))
	for _, row := range rows {
		out = append(out, OfferChange{CountryOffer: CountryOffer{Ref: TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID}, ProviderID: row.ProviderID,
			ProviderName: row.ProviderName, Monetization: row.Monetization}, Event: row.Event, ObservedAt: row.ObservedAt, Title: row.Title})
	}
	return out, nil
}
