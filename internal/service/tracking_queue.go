package service

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Where a show stands for the user. The queue lists them in this order.
const (
	StageWatching   = "watching"    // part way through, watched recently
	StageStalled    = "stalled"     // part way through, nothing watched for stale_days
	StageNotStarted = "not_started" // on the watchlist (or watching) with no episode seen
	StageOnHold     = "on_hold"     // the user put it on hold
	StageDropped    = "dropped"     // the user stopped watching
	StageCaughtUp   = "caught_up"   // every aired episode seen
)

var stageOrder = []string{StageWatching, StageStalled, StageNotStarted, StageOnHold, StageDropped, StageCaughtUp}

const (
	defaultStaleDays    = 30
	minStaleDays        = 7
	maxStaleDays        = 365
	defaultUpcomingDays = 90
	maxUpcomingDays     = 365
	maxUpcomingEpisodes = 500
	// Episode lists refreshed per queue request; the rest wait for the next one.
	maxShowRefreshes   = 12
	showRefreshWorkers = 4
)

// ShowQueueItem is one show in the queue.
type ShowQueueItem struct {
	TmdbID            string                   `json:"tmdb_id"`
	Title             string                   `json:"title"`
	PosterPath        string                   `json:"poster_path,omitempty"`
	Status            string                   `json:"status"`
	Favorite          bool                     `json:"favorite"`
	Stage             string                   `json:"stage"`
	TotalEpisodes     int64                    `json:"total_episodes"`
	AiredEpisodes     int64                    `json:"aired_episodes"`
	WatchedEpisodes   int64                    `json:"watched_episodes"`
	RemainingEpisodes int64                    `json:"remaining_episodes"`
	MinutesLeft       int64                    `json:"minutes_left"`
	Percent           int64                    `json:"percent"`
	LastWatchedAt     *string                  `json:"last_watched_at,omitempty"`
	AddedAt           string                   `json:"added_at"`
	WatchlistPosition *int64                   `json:"watchlist_position,omitempty"`
	NextEpisode       *repository.QueueEpisode `json:"next_episode,omitempty"`
	NextAirDate       *string                  `json:"next_air_date,omitempty"`
	updatedAt         string
}

// refreshMu makes concurrent queue requests wait for one refresh rather than
// fetch the same shows twice.
var refreshMu sync.Mutex

// refreshShowEpisodes re-caches the episode lists of the user's shows that
// were never fetched or are older than the cache TTL, at most
// maxShowRefreshes per call. Failures are logged and skipped: a TMDB outage
// leaves the queue on cached data rather than failing it.
func (s *TrackingService) refreshShowEpisodes(ctx context.Context, userID int64) {
	if s.titles == nil {
		return
	}
	refreshMu.Lock()
	defer refreshMu.Unlock()

	cands, err := s.read.ShowRefreshCandidates(ctx, userID)
	if err != nil {
		s.log.Warn("list shows to refresh", "user_id", userID, "error", err)
		return
	}
	now := s.now().UTC()
	due := make([]string, 0, maxShowRefreshes)
	for _, c := range cands {
		if len(due) == maxShowRefreshes {
			break
		}
		if c.FetchedAt != "" {
			if at, err := repository.ParseTimestamp(c.FetchedAt); err == nil && now.Sub(at) <= trackedEpisodesTTL {
				continue
			}
		}
		due = append(due, c.TmdbID)
	}

	sem := make(chan struct{}, showRefreshWorkers)
	var wg sync.WaitGroup
	for _, id := range due {
		wg.Add(1)
		sem <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := s.titles.ResolveEpisodes(ctx, id, false); err != nil {
				s.log.Warn("refresh show episodes", "tmdb_id", id, "error", err)
			}
		}(id)
	}
	wg.Wait()
}

// ShowQueue sorts every show in the user's library into stages (see
// stageOrder), each with progress and the next episode to watch. staleDays
// (7-365) is how long without a watch moves a show part way through from
// "watching" to "stalled"; 0 means the user's preference, else 30.
func (s *TrackingService) ShowQueue(ctx context.Context, userID int64, staleDays int) (map[string]any, error) {
	if staleDays == 0 {
		staleDays = defaultStaleDays
		if p, _, err := s.preferences(ctx, userID); err == nil && p.StaleDays != nil {
			staleDays = *p.StaleDays
		}
	}
	if staleDays < minStaleDays || staleDays > maxStaleDays {
		return nil, &ValidationError{Message: "stale_days must be between 7 and 365"}
	}
	s.refreshShowEpisodes(ctx, userID)

	rows, err := s.read.ShowQueue(ctx, userID, s.today())
	if err != nil {
		return nil, err
	}
	staleBefore := s.now().UTC().AddDate(0, 0, -staleDays)
	counts := make(map[string]int, len(stageOrder))
	for _, st := range stageOrder {
		counts[st] = 0
	}
	shows := make([]ShowQueueItem, 0, len(rows))
	for _, r := range rows {
		it := ShowQueueItem{
			TmdbID: r.TmdbID, Title: r.Title, PosterPath: r.PosterPath, Status: r.Status, Favorite: r.Favorite,
			TotalEpisodes: r.TotalEpisodes, AiredEpisodes: r.AiredEpisodes, WatchedEpisodes: r.WatchedEpisodes,
			RemainingEpisodes: max(r.AiredEpisodes-r.WatchedEpisodes, 0), MinutesLeft: r.MinutesLeft,
			Percent: percentOf(r.WatchedEpisodes, r.AiredEpisodes), LastWatchedAt: r.LastWatchedAt,
			AddedAt: r.CreatedAt, WatchlistPosition: r.WatchlistPosition, NextEpisode: r.NextEpisode,
			NextAirDate: r.NextAirDate, updatedAt: r.UpdatedAt,
		}
		it.Stage = stageOf(r, staleBefore)
		counts[it.Stage]++
		shows = append(shows, it)
	}
	sortQueue(shows)
	return map[string]any{"stale_days": staleDays, "today": s.today(), "counts": counts, "shows": shows}, nil
}

// stageOf decides where a show stands. A status the user chose (on hold,
// dropped) wins; otherwise the episodes decide, so a "completed" show with
// a new episode out is back to watching.
func stageOf(r repository.ShowQueueRow, staleBefore time.Time) string {
	switch r.Status {
	case StatusOnHold:
		return StageOnHold
	case StatusDropped:
		return StageDropped
	}
	if r.WatchedEpisodes == 0 {
		if r.Status == StatusCompleted {
			return StageCaughtUp
		}
		return StageNotStarted
	}
	if r.NextEpisode == nil {
		return StageCaughtUp
	}
	if r.LastWatchedAt != nil {
		if at, err := repository.ParseTimestamp(*r.LastWatchedAt); err == nil && at.Before(staleBefore) {
			return StageStalled
		}
	}
	return StageWatching
}

// sortQueue orders shows by stage, then within a stage: most recently
// watched first while in progress, watchlist order (then newest added) for
// not started, soonest next episode for caught up, latest change otherwise.
func sortQueue(shows []ShowQueueItem) {
	rank := make(map[string]int, len(stageOrder))
	for i, st := range stageOrder {
		rank[st] = i
	}
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	sort.SliceStable(shows, func(i, j int) bool {
		a, b := shows[i], shows[j]
		if a.Stage != b.Stage {
			return rank[a.Stage] < rank[b.Stage]
		}
		switch a.Stage {
		case StageWatching, StageStalled:
			if x, y := str(a.LastWatchedAt), str(b.LastWatchedAt); x != y {
				return x > y
			}
		case StageNotStarted:
			pa, pb := a.WatchlistPosition, b.WatchlistPosition
			if (pa == nil) != (pb == nil) {
				return pa != nil
			}
			if pa != nil && *pa != *pb {
				return *pa < *pb
			}
			if a.AddedAt != b.AddedAt {
				return a.AddedAt > b.AddedAt
			}
		case StageCaughtUp:
			x, y := str(a.NextAirDate), str(b.NextAirDate)
			if (x == "") != (y == "") {
				return x != ""
			}
			if x != y {
				return x < y
			}
			if x, y := str(a.LastWatchedAt), str(b.LastWatchedAt); x != y {
				return x > y
			}
		default:
			if a.updatedAt != b.updatedAt {
				return a.updatedAt > b.updatedAt
			}
		}
		return a.Title < b.Title
	})
}

// Upcoming lists the episodes airing in the next `days` days (1-365,
// default 90) of the user's shows, leaving out shows on hold, dropped or not
// wanted. Today's episodes count as aired and are in the queue instead.
func (s *TrackingService) Upcoming(ctx context.Context, userID int64, days int) (map[string]any, error) {
	if days == 0 {
		days = defaultUpcomingDays
	}
	if days < 1 || days > maxUpcomingDays {
		return nil, &ValidationError{Message: "days must be between 1 and 365"}
	}
	s.refreshShowEpisodes(ctx, userID)

	today := s.now().UTC()
	until := today.AddDate(0, 0, days).Format("2006-01-02")
	eps, err := s.read.UpcomingEpisodes(ctx, userID, today.Format("2006-01-02"), until, maxUpcomingEpisodes)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"from": today.AddDate(0, 0, 1).Format("2006-01-02"), "to": until, "days": days,
		"episodes": eps, "truncated": len(eps) == maxUpcomingEpisodes,
	}, nil
}
