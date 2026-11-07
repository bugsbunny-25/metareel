package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// EpisodesRepository stores TMDB episodes of TV shows and reads them for the
// episode calendar and release-pattern analytics.
type EpisodesRepository struct {
	db *sql.DB
	q  *sqlc.Queries
}

func NewEpisodesRepository(db *sql.DB) *EpisodesRepository {
	return &EpisodesRepository{db: db, q: sqlc.New(db)}
}

// EpisodeTarget is a TV show whose episodes may be fetched.
type EpisodeTarget struct {
	TmdbID    string
	Status    string       // TMDB status: Returning Series, Ended, ...
	Seasons   []TmdbSeason // from the show's TMDB details
	FetchedAt string       // "YYYY-MM-DD HH:MM:SS" UTC, empty = never
	Airing    bool         // a stored episode airs within the window asked for
}

// EpisodeTargets lists TV shows with a season list; Airing marks those with
// a stored episode dated between airingFrom and airingTo (YYYY-MM-DD).
func (r *EpisodesRepository) EpisodeTargets(ctx context.Context, airingFrom, airingTo string) ([]EpisodeTarget, error) {
	rows, err := r.q.ListEpisodeTargets(ctx, sqlc.ListEpisodeTargetsParams{AiringFrom: airingFrom, AiringTo: airingTo})
	if err != nil {
		return nil, fmt.Errorf("list episode targets: %w", err)
	}
	out := make([]EpisodeTarget, 0, len(rows))
	for _, row := range rows {
		out = append(out, EpisodeTarget{TmdbID: row.TmdbID, Status: row.Status, Seasons: DecodeTmdbSeasons(row.Seasons),
			FetchedAt: row.EpisodesFetchedAt, Airing: row.Airing != 0})
	}
	return out, nil
}

// SaveEpisodes replaces the episodes of the given seasons of a show and marks
// the show fetched, in one transaction. Seasons missing from episodes keep
// what was stored.
func (r *EpisodesRepository) SaveEpisodes(ctx context.Context, tmdbID string, episodes map[int][]client.TMDBEpisode) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)
	for season, eps := range episodes {
		if err := q.DeleteSeasonEpisodes(ctx, sqlc.DeleteSeasonEpisodesParams{TmdbID: tmdbID, SeasonNumber: int64(season)}); err != nil {
			return fmt.Errorf("delete episodes of tv %s season %d: %w", tmdbID, season, err)
		}
		for _, e := range eps {
			if err := q.InsertEpisode(ctx, sqlc.InsertEpisodeParams{
				TmdbID: tmdbID, SeasonNumber: int64(season), EpisodeNumber: int64(e.EpisodeNumber),
				Name: nullIfEmpty(e.Name), AirDate: nullIfEmpty(e.AirDate), EpisodeType: nullIfEmpty(e.EpisodeType),
				Runtime: nullIfZero(e.Runtime),
			}); err != nil {
				return fmt.Errorf("insert episode %d of tv %s season %d: %w", e.EpisodeNumber, tmdbID, season, err)
			}
		}
	}
	if err := q.SetEpisodesFetched(ctx, tmdbID); err != nil {
		return fmt.Errorf("mark episodes fetched of tv %s: %w", tmdbID, err)
	}
	return tx.Commit()
}

// CalendarEpisode is an episode of a show available in a country.
type CalendarEpisode struct {
	TmdbID        string
	Title         string
	PosterPath    string
	SeasonNumber  int64
	EpisodeNumber int64
	Name          string
	AirDate       string
	EpisodeType   string
	Runtime       *int64
}

// CalendarEpisodes lists episodes airing between two dates (YYYY-MM-DD) of
// shows with a subscription, free or ad-supported offer in a country
// (providerID 0 = any provider).
func (r *EpisodesRepository) CalendarEpisodes(ctx context.Context, country, from, to string, providerID int64) ([]CalendarEpisode, error) {
	rows, err := r.q.ListCalendarEpisodes(ctx, sqlc.ListCalendarEpisodesParams{FromDate: from, ToDate: to, Country: country, ProviderID: providerID})
	if err != nil {
		return nil, fmt.Errorf("list calendar episodes: %w", err)
	}
	out := make([]CalendarEpisode, 0, len(rows))
	for _, row := range rows {
		out = append(out, CalendarEpisode{TmdbID: row.TmdbID, Title: row.Title, PosterPath: row.PosterPath, SeasonNumber: row.SeasonNumber,
			EpisodeNumber: row.EpisodeNumber, Name: row.Name, AirDate: row.AirDate, EpisodeType: row.EpisodeType, Runtime: nullInt64Ptr(row.Runtime)})
	}
	return out, nil
}

// CalendarOffer is a show's offer in a country.
type CalendarOffer struct {
	ProviderID   int64  `json:"provider_id"`
	ProviderName string `json:"provider_name"`
	Monetization string `json:"monetization"`
	URL          string `json:"url,omitempty"`
}

// CalendarOffers returns, per show with an episode airing between two
// dates, its subscription, free and ad-supported offers in a country.
func (r *EpisodesRepository) CalendarOffers(ctx context.Context, country, from, to string) (map[string][]CalendarOffer, error) {
	rows, err := r.q.ListCalendarProviders(ctx, sqlc.ListCalendarProvidersParams{Country: country, FromDate: from, ToDate: to})
	if err != nil {
		return nil, fmt.Errorf("list calendar providers: %w", err)
	}
	out := map[string][]CalendarOffer{}
	for _, row := range rows {
		out[row.TmdbID] = append(out[row.TmdbID], CalendarOffer{ProviderID: row.ProviderID, ProviderName: row.ProviderName,
			Monetization: row.Monetization, URL: row.Url})
	}
	return out, nil
}

// SeasonEpisode is an episode's air date (empty = TBA).
type SeasonEpisode struct {
	TmdbID        string
	Title         string
	SeasonNumber  int64
	EpisodeNumber int64
	AirDate       string
}

// SeasonEpisodes lists every episode of the regular seasons whose first
// episode aired between two dates, ordered by show, season and episode.
func (r *EpisodesRepository) SeasonEpisodes(ctx context.Context, from, to string) ([]SeasonEpisode, error) {
	rows, err := r.q.ListSeasonEpisodes(ctx, sqlc.ListSeasonEpisodesParams{FromDate: from, ToDate: to})
	if err != nil {
		return nil, fmt.Errorf("list season episodes: %w", err)
	}
	out := make([]SeasonEpisode, 0, len(rows))
	for _, row := range rows {
		out = append(out, SeasonEpisode{TmdbID: row.TmdbID, Title: row.Title, SeasonNumber: row.SeasonNumber,
			EpisodeNumber: row.EpisodeNumber, AirDate: row.AirDate})
	}
	return out, nil
}

// SeasonPremieres returns the first air date of every regular season, per
// show, of the shows with an episode on or after from.
func (r *EpisodesRepository) SeasonPremieres(ctx context.Context, from string) (map[string]map[int64]string, error) {
	rows, err := r.q.ListShowSeasonPremieres(ctx, from)
	if err != nil {
		return nil, fmt.Errorf("list season premieres: %w", err)
	}
	out := map[string]map[int64]string{}
	for _, row := range rows {
		if out[row.TmdbID] == nil {
			out[row.TmdbID] = map[int64]string{}
		}
		out[row.TmdbID][row.SeasonNumber] = row.Premiere
	}
	return out, nil
}
