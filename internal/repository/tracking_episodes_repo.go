package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// TrackedEpisode is one episode of a tracked show.
type TrackedEpisode struct {
	SeasonNumber  int64   `json:"season_number"`
	EpisodeNumber int64   `json:"episode_number"`
	TmdbEpisodeID *string `json:"tmdb_episode_id,omitempty"`
	Name          string  `json:"name"`
	AirDate       *string `json:"air_date,omitempty"`
	EpisodeType   *string `json:"episode_type,omitempty"`
	Runtime       *int64  `json:"runtime,omitempty"`
	// Filled by show reads.
	WatchCount    int64   `json:"watch_count"`
	LastWatchedAt *string `json:"last_watched_at,omitempty"`
}

// SaveShowEpisodes replaces the cached episodes of the given seasons of a
// show and marks the show fetched, in one transaction. Seasons that are not
// in the map keep what was stored.
func (r *TrackingRepository) SaveShowEpisodes(ctx context.Context, tmdbID string, seasons map[int64][]TrackedEpisode) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)

	for season, episodes := range seasons {
		if err := q.DeleteTrackedSeason(ctx, sqlc.DeleteTrackedSeasonParams{
			TmdbID: tmdbID, SeasonNumber: season}); err != nil {
			return fmt.Errorf("clear cached season %d of tv %s: %w", season, tmdbID, err)
		}
		for _, e := range episodes {
			if err := q.UpsertTrackedEpisode(ctx, sqlc.UpsertTrackedEpisodeParams{
				TmdbID: tmdbID, SeasonNumber: season, EpisodeNumber: e.EpisodeNumber,
				TmdbEpisodeID: toNullString(e.TmdbEpisodeID), Name: e.Name,
				AirDate: toNullString(e.AirDate), EpisodeType: toNullString(e.EpisodeType),
				Runtime: toNullInt64(e.Runtime),
			}); err != nil {
				return fmt.Errorf("cache episode S%dE%d of tv %s: %w", season, e.EpisodeNumber, tmdbID, err)
			}
		}
	}
	if err := q.SetTrackedEpisodesFetched(ctx, tmdbID); err != nil {
		return fmt.Errorf("mark episodes fetched for tv %s: %w", tmdbID, err)
	}
	return tx.Commit()
}

// EpisodesFetchedAt is when a show's episodes were last cached ("" = never).
func (r *TrackingRepository) EpisodesFetchedAt(ctx context.Context, tmdbID string) (string, error) {
	at, err := r.q.GetTrackedEpisodesFetchedAt(ctx, tmdbID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("episodes fetched at for tv %s: %w", tmdbID, err)
	}
	return at, nil
}

// Episode returns one cached episode, or sql.ErrNoRows.
func (r *TrackingRepository) Episode(ctx context.Context, tmdbID string, season, episode int64) (TrackedEpisode, error) {
	row, err := r.q.GetTrackedEpisode(ctx, sqlc.GetTrackedEpisodeParams{
		TmdbID: tmdbID, SeasonNumber: season, EpisodeNumber: episode})
	if err != nil {
		return TrackedEpisode{}, err
	}
	return TrackedEpisode{
		SeasonNumber: row.SeasonNumber, EpisodeNumber: row.EpisodeNumber,
		TmdbEpisodeID: nullStringPtr(row.TmdbEpisodeID), Name: row.Name,
		AirDate: nullStringPtr(row.AirDate), EpisodeType: nullStringPtr(row.EpisodeType),
		Runtime: nullInt64Ptr(row.Runtime),
	}, nil
}

// ShowEpisodes lists a show's episodes with the user's watch count on each.
func (r *TrackingRepository) ShowEpisodes(ctx context.Context, userID int64, tmdbID string, includeSpecials bool) ([]TrackedEpisode, error) {
	rows, err := r.q.ListShowEpisodes(ctx, sqlc.ListShowEpisodesParams{
		UserID: userID, TmdbID: tmdbID, IncludeSpecials: boolInt(includeSpecials)})
	if err != nil {
		return nil, fmt.Errorf("list episodes of tv %s: %w", tmdbID, err)
	}
	out := make([]TrackedEpisode, 0, len(rows))
	for _, row := range rows {
		e := TrackedEpisode{
			SeasonNumber: row.SeasonNumber, EpisodeNumber: row.EpisodeNumber,
			TmdbEpisodeID: nullStringPtr(row.TmdbEpisodeID), Name: row.Name,
			AirDate: nullStringPtr(row.AirDate), EpisodeType: nullStringPtr(row.EpisodeType),
			Runtime: nullInt64Ptr(row.Runtime), WatchCount: row.WatchCount,
		}
		if row.LastWatchedAt != "" {
			at := row.LastWatchedAt
			e.LastWatchedAt = &at
		}
		out = append(out, e)
	}
	return out, nil
}

// EpisodeScope picks the episodes a bulk mark or unmark applies to.
type EpisodeScope struct {
	// Scope is one of: episode, season, up_to, all.
	Scope           string
	SeasonNumber    int64
	EpisodeNumber   int64
	IncludeSpecials bool
	// AiredOnly leaves out episodes TMDB has no air date for, or that have
	// not aired yet. Marking uses it; unmarking does not.
	AiredOnly bool
	// OnlyUnwatched leaves out episodes the user has already seen, so
	// marking a season does not pile rewatches onto the ones they have.
	OnlyUnwatched bool
	// Today is the UTC day aired-ness is measured against (YYYY-MM-DD).
	Today string
}

// MarkableEpisodes lists the episodes a scope selects.
func (r *TrackingRepository) MarkableEpisodes(ctx context.Context, userID int64, tmdbID string, sc EpisodeScope) ([]TrackedEpisode, error) {
	rows, err := r.q.ListMarkableEpisodes(ctx, sqlc.ListMarkableEpisodesParams{
		UserID: userID, TmdbID: tmdbID,
		Scope: sc.Scope, SeasonNumber: sc.SeasonNumber, EpisodeNumber: sc.EpisodeNumber,
		IncludeSpecials: boolInt(sc.IncludeSpecials), AiredOnly: boolInt(sc.AiredOnly),
		OnlyUnwatched: boolInt(sc.OnlyUnwatched), Today: sc.Today,
	})
	if err != nil {
		return nil, fmt.Errorf("list markable episodes of tv %s: %w", tmdbID, err)
	}
	out := make([]TrackedEpisode, 0, len(rows))
	for _, row := range rows {
		out = append(out, TrackedEpisode{
			SeasonNumber: row.SeasonNumber, EpisodeNumber: row.EpisodeNumber,
			TmdbEpisodeID: nullStringPtr(row.TmdbEpisodeID), Name: row.Name,
			Runtime: nullInt64Ptr(row.Runtime),
		})
	}
	return out, nil
}

// MarkEpisodes records a watch for each episode in one transaction and
// refreshes the show's roll-up. Every episode gets the same time, because a
// bulk mark says "I have seen these", not when each one was watched.
func (r *TrackingRepository) MarkEpisodes(ctx context.Context, userID int64, tmdbID string, episodes []TrackedEpisode, w NewWatch) (int, error) {
	if len(episodes) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)

	liked := int64(0)
	if w.Liked {
		liked = 1
	}
	for _, e := range episodes {
		season, episode := e.SeasonNumber, e.EpisodeNumber
		number, err := q.NextWatchNumber(ctx, sqlc.NextWatchNumberParams{
			UserID: userID, TmdbKind: "tv", TmdbID: tmdbID,
			SeasonNumber:  sql.NullInt64{Int64: season, Valid: true},
			EpisodeNumber: sql.NullInt64{Int64: episode, Valid: true}})
		if err != nil {
			return 0, fmt.Errorf("next watch number for S%dE%d: %w", season, episode, err)
		}
		rewatch := int64(0)
		if number > 1 {
			rewatch = 1
		}
		// runtime_minutes_used records the runtime a start was derived
		// from. An imprecise mark derives no start, so it stays empty
		// rather than implying one.
		runtime := w.RuntimeMinutesUsed
		runtimeSource := w.RuntimeSource
		if w.TimePrecision == "minute" && e.Runtime != nil && *e.Runtime > 0 {
			runtime = e.Runtime
			src := "tmdb_episode"
			runtimeSource = &src
		}
		if _, err := q.InsertWatchEntry(ctx, sqlc.InsertWatchEntryParams{
			UserID: userID, ClientKey: sql.NullString{}, SubjectKind: "episode",
			TmdbKind: "tv", TmdbID: tmdbID,
			SeasonNumber:  sql.NullInt64{Int64: season, Valid: true},
			EpisodeNumber: sql.NullInt64{Int64: episode, Valid: true},
			TmdbEpisodeID: toNullString(e.TmdbEpisodeID),
			EndedAt:       w.EndedAt, StartedAt: toNullString(w.StartedAt),
			RuntimeMinutesUsed: toNullInt64(runtime), RuntimeSource: toNullString(runtimeSource),
			Tz: toNullString(w.Tz), UtcOffsetMinutes: toNullInt64(w.UTCOffsetMinutes),
			WatchedOn: w.WatchedOn, TimePrecision: w.TimePrecision,
			Rating: toNullInt64(w.Rating), Liked: liked, IsRewatch: rewatch, WatchNumber: number,
			Notes: toNullString(w.Notes), VenueKind: toNullString(w.VenueKind), Source: w.Source,
		}); err != nil {
			return 0, fmt.Errorf("mark S%dE%d of tv %s: %w", season, episode, tmdbID, err)
		}
	}
	if err := rollup(ctx, q, userID, "tv", tmdbID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit episode marks: %w", err)
	}
	return len(episodes), nil
}

// UnmarkEpisodes deletes every watch of the episodes a scope selects and
// refreshes the show's roll-up.
func (r *TrackingRepository) UnmarkEpisodes(ctx context.Context, userID int64, tmdbID string, sc EpisodeScope) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)

	n, err := q.DeleteEpisodeWatches(ctx, sqlc.DeleteEpisodeWatchesParams{
		UserID: userID, TmdbID: tmdbID, Scope: sc.Scope,
		SeasonNumber:    sql.NullInt64{Int64: sc.SeasonNumber, Valid: true},
		EpisodeNumber:   sql.NullInt64{Int64: sc.EpisodeNumber, Valid: true},
		IncludeSpecials: boolInt(sc.IncludeSpecials),
	})
	if err != nil {
		return 0, fmt.Errorf("unmark episodes of tv %s: %w", tmdbID, err)
	}
	if err := rollup(ctx, q, userID, "tv", tmdbID); err != nil {
		return 0, err
	}
	if _, err := q.DeleteEmptyUserTitle(ctx, sqlc.DeleteEmptyUserTitleParams{
		UserID: userID, TmdbKind: "tv", TmdbID: tmdbID}); err != nil {
		return 0, fmt.Errorf("prune empty user title: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit episode unmarks: %w", err)
	}
	return n, nil
}

// ShowProgressCounts is how far a user is through a show. The denominator
// moves as episodes air, so it is counted rather than stored.
type ShowProgressCounts struct {
	TotalEpisodes   int64  `json:"total_episodes"`
	AiredEpisodes   int64  `json:"aired_episodes"`
	WatchedEpisodes int64  `json:"watched_episodes"`
	NextAirDate     string `json:"next_air_date,omitempty"`
}

// ShowProgress counts a user's progress through one show.
func (r *TrackingRepository) ShowProgress(ctx context.Context, userID int64, tmdbID, today string, includeSpecials bool) (ShowProgressCounts, error) {
	row, err := r.q.ShowProgress(ctx, sqlc.ShowProgressParams{
		UserID: userID, TmdbID: tmdbID, Today: today, IncludeSpecials: boolInt(includeSpecials)})
	if err != nil {
		return ShowProgressCounts{}, fmt.Errorf("show progress of tv %s: %w", tmdbID, err)
	}
	return ShowProgressCounts{
		TotalEpisodes: row.TotalEpisodes, AiredEpisodes: row.AiredEpisodes,
		WatchedEpisodes: row.WatchedEpisodes, NextAirDate: row.NextAirDate,
	}, nil
}

// UpNextEpisode is the next episode to watch of a show in progress.
type UpNextEpisode struct {
	TmdbID        string  `json:"tmdb_id"`
	ShowTitle     string  `json:"show_title"`
	PosterPath    string  `json:"poster_path,omitempty"`
	SeasonNumber  int64   `json:"season_number"`
	EpisodeNumber int64   `json:"episode_number"`
	Name          string  `json:"name"`
	AirDate       *string `json:"air_date,omitempty"`
	EpisodeType   *string `json:"episode_type,omitempty"`
	Runtime       *int64  `json:"runtime,omitempty"`
}

// UpNext is the next aired, unwatched episode of each show the user is part
// way through, most recently aired first.
func (r *TrackingRepository) UpNext(ctx context.Context, userID int64, today string, limit int64) ([]UpNextEpisode, error) {
	rows, err := r.q.UpNext(ctx, sqlc.UpNextParams{UserID: userID, Today: today, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("up next: %w", err)
	}
	out := make([]UpNextEpisode, 0, len(rows))
	for _, row := range rows {
		out = append(out, UpNextEpisode{
			TmdbID: row.TmdbID, ShowTitle: row.ShowTitle, PosterPath: row.PosterPath,
			SeasonNumber: row.SeasonNumber, EpisodeNumber: row.EpisodeNumber, Name: row.Name,
			AirDate: nullStringPtr(row.AirDate), EpisodeType: nullStringPtr(row.EpisodeType),
			Runtime: nullInt64Ptr(row.Runtime),
		})
	}
	return out, nil
}
