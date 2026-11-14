package repository

import (
	"context"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// ShowRefreshCandidate is one of a user's shows and when its episodes were
// last cached ("" = never).
type ShowRefreshCandidate struct {
	TmdbID    string
	FetchedAt string
}

// ShowRefreshCandidates lists the user's shows, never-fetched first, then by
// how long ago their episodes were cached.
func (r *TrackingRepository) ShowRefreshCandidates(ctx context.Context, userID int64) ([]ShowRefreshCandidate, error) {
	rows, err := r.q.ListShowRefreshCandidates(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("show refresh candidates: %w", err)
	}
	out := make([]ShowRefreshCandidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, ShowRefreshCandidate{TmdbID: row.TmdbID, FetchedAt: row.FetchedAt})
	}
	return out, nil
}

// QueueEpisode is an episode in the show queue or the upcoming list.
type QueueEpisode struct {
	SeasonNumber  int64   `json:"season_number"`
	EpisodeNumber int64   `json:"episode_number"`
	Name          string  `json:"name"`
	AirDate       *string `json:"air_date,omitempty"`
	Runtime       *int64  `json:"runtime,omitempty"`
	EpisodeType   *string `json:"episode_type,omitempty"`
}

// ShowQueueRow is one show in a user's library with their progress through
// it. Counts leave specials (season 0) out.
type ShowQueueRow struct {
	TmdbID            string
	Status            string
	Favorite          bool
	WatchlistPosition *int64
	LastWatchedAt     *string
	CreatedAt         string
	UpdatedAt         string
	Title             string
	PosterPath        string
	TotalEpisodes     int64
	AiredEpisodes     int64
	WatchedEpisodes   int64
	MinutesLeft       int64
	NextAirDate       *string
	// NextEpisode is the first aired episode they have not seen, if any.
	NextEpisode *QueueEpisode
}

// ShowQueue returns every show in the user's library (not the ones they are
// not interested in) with its progress as of today (YYYY-MM-DD, UTC).
func (r *TrackingRepository) ShowQueue(ctx context.Context, userID int64, today string) ([]ShowQueueRow, error) {
	rows, err := r.q.ListShowQueue(ctx, sqlc.ListShowQueueParams{UserID: userID, Today: today})
	if err != nil {
		return nil, fmt.Errorf("show queue: %w", err)
	}
	out := make([]ShowQueueRow, 0, len(rows))
	for _, row := range rows {
		q := ShowQueueRow{
			TmdbID: row.TmdbID, Status: row.Status, Favorite: row.Favorite != 0,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			Title: row.Title, PosterPath: row.PosterPath,
			TotalEpisodes: row.TotalEpisodes, AiredEpisodes: row.AiredEpisodes,
			WatchedEpisodes: row.WatchedEpisodes, MinutesLeft: row.MinutesLeft,
			LastWatchedAt: nonEmptyPtr(row.LastWatchedAt), NextAirDate: nonEmptyPtr(row.NextAirDate),
		}
		if row.WatchlistPosition >= 0 {
			p := row.WatchlistPosition
			q.WatchlistPosition = &p
		}
		if row.NextSeason >= 0 {
			q.NextEpisode = &QueueEpisode{
				SeasonNumber: row.NextSeason, EpisodeNumber: row.NextEpisode, Name: row.NextName,
				AirDate: nonEmptyPtr(row.NextEpisodeAirDate), EpisodeType: nonEmptyPtr(row.NextEpisodeType),
			}
			if row.NextRuntime > 0 {
				rt := row.NextRuntime
				q.NextEpisode.Runtime = &rt
			}
		}
		out = append(out, q)
	}
	return out, nil
}

// UpcomingEpisode is an episode that has not aired yet, of a show in the
// user's library.
type UpcomingEpisode struct {
	TmdbID     string `json:"tmdb_id"`
	ShowTitle  string `json:"show_title"`
	PosterPath string `json:"poster_path,omitempty"`
	Status     string `json:"status"`
	QueueEpisode
}

// UpcomingEpisodes lists episodes airing after `after` and up to `until`
// (both YYYY-MM-DD) of the user's shows, except those on hold, dropped or
// not wanted; soonest first.
func (r *TrackingRepository) UpcomingEpisodes(ctx context.Context, userID int64, after, until string, limit int64) ([]UpcomingEpisode, error) {
	rows, err := r.q.ListUpcomingEpisodes(ctx, sqlc.ListUpcomingEpisodesParams{
		UserID: userID, After: after, Until: until, Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("upcoming episodes: %w", err)
	}
	out := make([]UpcomingEpisode, 0, len(rows))
	for _, row := range rows {
		airDate := row.AirDate
		out = append(out, UpcomingEpisode{
			TmdbID: row.TmdbID, ShowTitle: row.ShowTitle, PosterPath: row.PosterPath, Status: row.Status,
			QueueEpisode: QueueEpisode{
				SeasonNumber: row.SeasonNumber, EpisodeNumber: row.EpisodeNumber, Name: row.Name,
				AirDate: &airDate, Runtime: nullInt64Ptr(row.Runtime), EpisodeType: nullStringPtr(row.EpisodeType),
			},
		})
	}
	return out, nil
}

// nonEmptyPtr is nil for "", else a pointer to a copy of s.
func nonEmptyPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
