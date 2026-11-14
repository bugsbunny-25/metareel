package service

import (
	"context"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Glance is the small summary the mobile app's widgets show (and refresh in
// the background): what to watch next, what airs next, the cinema plan's
// month and the last thing watched. Read-only; built from the same reads as
// /me/up-next, /me/upcoming, /me/subscriptions/savings and /me/watches.
type Glance struct {
	GeneratedAt  string                      `json:"generated_at"`
	UpNext       []repository.UpNextEpisode  `json:"up_next"`
	NextAiring   *repository.UpcomingEpisode `json:"next_airing"`
	Subscription *GlanceSubscription         `json:"subscription"`
	LastWatch    *GlanceWatch                `json:"last_watch"`
}

// GlanceSubscription is the active cinema plan's current month.
type GlanceSubscription struct {
	ID           int64   `json:"id"`
	Plan         string  `json:"plan"`
	Chain        *string `json:"chain,omitempty"`
	Currency     string  `json:"currency"`
	Period       string  `json:"period"`
	Month        string  `json:"month"`                        // YYYY-MM
	Watches      int     `json:"watches_this_month"`           // covered watches this month
	BreakEven    *int    `json:"break_even_watches,omitempty"` // covered watches a period needs to pay off
	SavedMinor   int64   `json:"saved_minor"`                  // over the plan's whole life
	TotalWatches int     `json:"watches"`                      // covered watches over the plan's life
}

// GlanceWatch is the most recent diary entry.
type GlanceWatch struct {
	ID            int64  `json:"id"`
	Kind          string `json:"tmdb_kind"`
	TmdbID        string `json:"tmdb_id"`
	Title         string `json:"title"`
	PosterPath    string `json:"poster_path,omitempty"`
	SeasonNumber  *int64 `json:"season_number,omitempty"`
	EpisodeNumber *int64 `json:"episode_number,omitempty"`
	WatchedOn     string `json:"watched_on"`
	EndedAt       string `json:"ended_at"`
	Rating        *int64 `json:"rating,omitempty"`
}

const (
	glanceUpNext     = 3
	glanceAiringDays = 60
)

// Glance gathers the widget summary for a user.
func (s *TrackingService) Glance(ctx context.Context, userID int64) (*Glance, error) {
	now := s.now().UTC()
	g := &Glance{GeneratedAt: now.Format(time.RFC3339), UpNext: []repository.UpNextEpisode{}}

	up, err := s.read.UpNext(ctx, userID, s.today(), glanceUpNext)
	if err != nil {
		return nil, err
	}
	if up != nil {
		g.UpNext = up
	}

	airing, err := s.read.UpcomingEpisodes(ctx, userID, now.Format(time.DateOnly), now.AddDate(0, 0, glanceAiringDays).Format(time.DateOnly), 1)
	if err != nil {
		return nil, err
	}
	if len(airing) > 0 {
		g.NextAiring = &airing[0]
	}

	sub, err := s.glanceSubscription(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	g.Subscription = sub

	watches, err := s.read.ListWatches(ctx, userID, repository.DiaryFilter{Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(watches) > 0 {
		w := watches[0]
		g.LastWatch = &GlanceWatch{ID: w.ID, Kind: w.Kind, TmdbID: w.TmdbID, Title: w.Title, PosterPath: w.PosterPath,
			SeasonNumber: w.SeasonNumber, EpisodeNumber: w.EpisodeNumber, WatchedOn: w.WatchedOn, EndedAt: w.EndedAt, Rating: w.Rating}
	}
	return g, nil
}

// glanceSubscription picks the active plan with the most covered watches
// (ties: the newest) and reports its current month.
func (s *TrackingService) glanceSubscription(ctx context.Context, userID int64, now time.Time) (*GlanceSubscription, error) {
	subs, err := s.read.CinemaSubscriptions(ctx, userID)
	if err != nil || len(subs) == 0 {
		return nil, err
	}
	watches, err := s.read.CinemaWatchCosts(ctx, userID)
	if err != nil {
		return nil, err
	}
	today := now.Format(time.DateOnly)
	month := today[:7]
	var best *SubscriptionSavings
	for _, sub := range subs {
		p := planSavings(sub, watches, today)
		if !p.Active {
			continue
		}
		if best == nil || p.Watches > best.Watches || (p.Watches == best.Watches && p.StartedOn > best.StartedOn) {
			best = &p
		}
	}
	if best == nil {
		return nil, nil
	}
	out := &GlanceSubscription{ID: best.ID, Plan: best.Plan, Chain: best.Chain, Currency: best.Currency, Period: best.Period,
		Month: month, BreakEven: best.BreakEven, SavedMinor: best.SavedMinor, TotalWatches: best.Watches}
	for _, m := range best.Months {
		if m.Month == month {
			out.Watches = m.Watches
		}
	}
	return out, nil
}
