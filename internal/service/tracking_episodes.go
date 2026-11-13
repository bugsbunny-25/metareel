package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Scopes a bulk mark or unmark can apply to.
const (
	ScopeEpisode = "episode" // just this one
	ScopeSeason  = "season"  // every episode of this season
	ScopeUpTo    = "up_to"   // everything up to and including this one
	ScopeAll     = "all"     // the whole show
)

var episodeScopes = map[string]bool{
	ScopeEpisode: true, ScopeSeason: true, ScopeUpTo: true, ScopeAll: true,
}

const maxUpNext = 100

// episodeFor resolves the episode a watch points at, caching the show's
// episodes first. It returns nil for a movie.
func (s *TrackingService) episodeFor(ctx context.Context, in WatchInput) (*repository.TrackedEpisode, error) {
	if in.Kind != "tv" || in.SeasonNumber == nil || in.EpisodeNumber == nil {
		return nil, nil
	}
	if *in.SeasonNumber < 0 {
		return nil, &ValidationError{Message: "season_number must be 0 or more (0 is specials)"}
	}
	if *in.EpisodeNumber < 1 {
		return nil, &ValidationError{Message: "episode_number must be 1 or more"}
	}
	if err := s.titles.ResolveEpisodes(ctx, in.TmdbID, false); err != nil {
		return nil, err
	}
	ep, err := s.write.Episode(ctx, in.TmdbID, *in.SeasonNumber, *in.EpisodeNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &ValidationError{Message: fmt.Sprintf("TMDB has no season %d episode %d of this show", *in.SeasonNumber, *in.EpisodeNumber)}
	}
	if err != nil {
		return nil, err
	}
	return &ep, nil
}

// MarkEpisodesInput marks a run of episodes watched in one go.
//
// The time applies to every episode marked, because a bulk mark says "I have
// seen these", not when each one was watched; that is why the precision
// defaults to a whole day rather than a minute. Log episodes one at a time
// (POST /api/v1/me/watches) when the exact times matter.
type MarkEpisodesInput struct {
	Scope           string  `json:"scope"`
	SeasonNumber    *int64  `json:"season_number"`
	EpisodeNumber   *int64  `json:"episode_number"`
	IncludeSpecials bool    `json:"include_specials"`
	EndedAt         string  `json:"ended_at"`
	TZ              *string `json:"tz"`
	OffsetMinutes   *int64  `json:"utc_offset_minutes"`
	Precision       string  `json:"time_precision"`
	Rating          *int64  `json:"rating"`
	Liked           *bool   `json:"liked"`
	Notes           *string `json:"notes"`
	VenueKind       *string `json:"venue_kind"`
}

// MarkEpisodes records a watch for every episode the scope selects that the
// user has not already seen, so marking a season does not pile rewatches
// onto the episodes they have. Episodes that have not aired are left alone.
func (s *TrackingService) MarkEpisodes(ctx context.Context, userID int64, tmdbID string, in MarkEpisodesInput) (map[string]any, error) {
	sc, err := s.scopeFor(in.Scope, in.SeasonNumber, in.EpisodeNumber, in.IncludeSpecials)
	if err != nil {
		return nil, err
	}
	sc.AiredOnly, sc.OnlyUnwatched = true, true

	snap, err := s.titles.Resolve(ctx, "tv", tmdbID)
	if err != nil {
		return nil, err
	}
	if err := s.titles.ResolveEpisodes(ctx, tmdbID, false); err != nil {
		return nil, err
	}

	precision := in.Precision
	if precision == "" {
		precision = PrecisionDay
	}
	endedAt := in.EndedAt
	if endedAt == "" {
		endedAt = s.now().UTC().Format(time.RFC3339)
	}
	base, err := s.buildWatch(WatchInput{
		Kind: "tv", TmdbID: tmdbID, EndedAt: endedAt, TZ: in.TZ, OffsetMinutes: in.OffsetMinutes,
		Precision: precision, Rating: in.Rating, Liked: in.Liked, Notes: in.Notes, VenueKind: in.VenueKind,
	}, snap, nil)
	if err != nil {
		return nil, err
	}

	episodes, err := s.write.MarkableEpisodes(ctx, userID, tmdbID, sc)
	if err != nil {
		return nil, err
	}
	marked, err := s.write.MarkEpisodes(ctx, userID, tmdbID, episodes, base)
	if err != nil {
		return nil, err
	}
	if err := s.advanceShowStatus(ctx, userID, tmdbID, sc.IncludeSpecials); err != nil {
		return nil, err
	}
	return s.showPayload(ctx, userID, tmdbID, sc.IncludeSpecials, map[string]any{"marked": marked})
}

// UnmarkEpisodes deletes every watch of the episodes the scope selects. The
// deletes are permanent, like any other watch delete.
func (s *TrackingService) UnmarkEpisodes(ctx context.Context, userID int64, tmdbID, scope string, season, episode *int64, includeSpecials bool) (map[string]any, error) {
	sc, err := s.scopeFor(scope, season, episode, includeSpecials)
	if err != nil {
		return nil, err
	}
	removed, err := s.write.UnmarkEpisodes(ctx, userID, tmdbID, sc)
	if err != nil {
		return nil, err
	}
	if err := s.advanceShowStatus(ctx, userID, tmdbID, includeSpecials); err != nil {
		return nil, err
	}
	return s.showPayload(ctx, userID, tmdbID, includeSpecials, map[string]any{"removed": removed})
}

// GetShow returns a show with the user's progress through it: its seasons and
// episodes, how often each was watched, and the overall counts.
func (s *TrackingService) GetShow(ctx context.Context, userID int64, tmdbID string, includeSpecials bool) (map[string]any, error) {
	if err := ValidateRef("tv", tmdbID); err != nil {
		return nil, err
	}
	if err := s.titles.ResolveEpisodes(ctx, tmdbID, false); err != nil {
		return nil, err
	}
	return s.showPayload(ctx, userID, tmdbID, includeSpecials, nil)
}

// UpNext is the next episode to watch of each show the user is part way
// through, most recently aired first.
func (s *TrackingService) UpNext(ctx context.Context, userID int64, limit int64) (map[string]any, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > maxUpNext {
		limit = maxUpNext
	}
	rows, err := s.read.UpNext(ctx, userID, s.today(), limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{"episodes": rows, "limit": limit}, nil
}

// scopeFor validates a bulk scope and the coordinates it needs.
func (s *TrackingService) scopeFor(scope string, season, episode *int64, includeSpecials bool) (repository.EpisodeScope, error) {
	if scope == "" {
		scope = ScopeEpisode
	}
	if !episodeScopes[scope] {
		return repository.EpisodeScope{}, &ValidationError{Message: "scope must be one of episode, season, up_to, all"}
	}
	sc := repository.EpisodeScope{Scope: scope, IncludeSpecials: includeSpecials, Today: s.today()}
	if scope == ScopeAll {
		return sc, nil
	}
	if season == nil {
		return repository.EpisodeScope{}, &ValidationError{Message: "season_number is required for this scope"}
	}
	if *season < 0 {
		return repository.EpisodeScope{}, &ValidationError{Message: "season_number must be 0 or more (0 is specials)"}
	}
	sc.SeasonNumber = *season
	if scope == ScopeSeason {
		return sc, nil
	}
	if episode == nil {
		return repository.EpisodeScope{}, &ValidationError{Message: "episode_number is required for this scope"}
	}
	if *episode < 1 {
		return repository.EpisodeScope{}, &ValidationError{Message: "episode_number must be 1 or more"}
	}
	sc.EpisodeNumber = *episode
	return sc, nil
}

// advanceShowStatus moves a show between "watching" and "completed" as
// episodes are marked and unmarked, so up-next works without the user
// setting a status by hand. A status they chose themselves (dropped, on
// hold, not interested) is left alone.
func (s *TrackingService) advanceShowStatus(ctx context.Context, userID int64, tmdbID string, includeSpecials bool) error {
	st, err := s.write.UserTitle(ctx, userID, "tv", tmdbID)
	if err != nil || st == nil {
		return err
	}
	switch st.Status {
	case StatusNone, StatusWatchlist, StatusWatching, StatusCompleted:
	default:
		return nil
	}
	counts, err := s.write.ShowProgress(ctx, userID, tmdbID, s.today(), includeSpecials)
	if err != nil {
		return err
	}
	want := st.Status
	switch {
	case counts.WatchedEpisodes == 0:
		if st.Status == StatusWatching || st.Status == StatusCompleted {
			want = StatusNone
		}
	case counts.AiredEpisodes > 0 && counts.WatchedEpisodes >= counts.AiredEpisodes:
		want = StatusCompleted
	default:
		want = StatusWatching
	}
	if want == st.Status {
		return nil
	}
	return s.write.SaveUserTitleState(ctx, userID, "tv", tmdbID, repository.UserTitleState{
		Status: want, Favorite: st.Favorite, WatchlistNote: st.WatchlistNote,
		WatchlistPosition: watchlistPositionFor(want, st.WatchlistPosition),
	})
}

func watchlistPositionFor(status string, current *int64) *int64 {
	if status == StatusWatchlist {
		return current
	}
	return nil
}

// showPayload is the show page: its metadata, the user's state, the counts
// and the seasons with their episodes.
func (s *TrackingService) showPayload(ctx context.Context, userID int64, tmdbID string, includeSpecials bool, extra map[string]any) (map[string]any, error) {
	snap, err := s.read.TitleSnapshot(ctx, "tv", tmdbID)
	if err != nil {
		return nil, err
	}
	state, err := s.read.UserTitle(ctx, userID, "tv", tmdbID)
	if err != nil {
		return nil, err
	}
	counts, err := s.read.ShowProgress(ctx, userID, tmdbID, s.today(), includeSpecials)
	if err != nil {
		return nil, err
	}
	episodes, err := s.read.ShowEpisodes(ctx, userID, tmdbID, includeSpecials)
	if err != nil {
		return nil, err
	}

	type season struct {
		SeasonNumber    int64                       `json:"season_number"`
		TotalEpisodes   int                         `json:"total_episodes"`
		WatchedEpisodes int                         `json:"watched_episodes"`
		Episodes        []repository.TrackedEpisode `json:"episodes"`
	}
	seasons := make([]season, 0, 8)
	index := map[int64]int{}
	for _, e := range episodes {
		i, ok := index[e.SeasonNumber]
		if !ok {
			seasons = append(seasons, season{SeasonNumber: e.SeasonNumber, Episodes: []repository.TrackedEpisode{}})
			i = len(seasons) - 1
			index[e.SeasonNumber] = i
		}
		seasons[i].Episodes = append(seasons[i].Episodes, e)
		seasons[i].TotalEpisodes++
		if e.WatchCount > 0 {
			seasons[i].WatchedEpisodes++
		}
	}

	progress := map[string]any{
		"total_episodes":   counts.TotalEpisodes,
		"aired_episodes":   counts.AiredEpisodes,
		"watched_episodes": counts.WatchedEpisodes,
		"percent":          percentOf(counts.WatchedEpisodes, counts.AiredEpisodes),
		"include_specials": includeSpecials,
	}
	if counts.NextAirDate != "" {
		progress["next_air_date"] = counts.NextAirDate
	}

	out := map[string]any{
		"title": titlePayload(snap), "state": state, "progress": progress, "seasons": seasons,
	}
	for k, v := range extra {
		out[k] = v
	}
	return out, nil
}

// percentOf is progress against the episodes that have aired, rounded down.
// The denominator moves as a show airs, so 100% today can be 96% next week.
func percentOf(watched, aired int64) int64 {
	if aired <= 0 {
		return 0
	}
	if watched > aired {
		watched = aired
	}
	return watched * 100 / aired
}

// today is the UTC day aired-ness is measured against.
func (s *TrackingService) today() string { return s.now().UTC().Format("2006-01-02") }
