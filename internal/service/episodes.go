package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

const (
	// A show is "active" (episodes refreshed daily) when a stored episode
	// airs within activePastDays before / activeAheadDays after today, or a
	// season starts within activeSeasonDays of today.
	episodesActivePastDays   = 14
	episodesActiveAheadDays  = 60
	episodesActiveSeasonDays = 30
	// Refresh intervals: active shows, returning shows, the rest (ended).
	episodesActiveEvery    = 24 * time.Hour
	episodesReturningEvery = 7 * 24 * time.Hour
	episodesEndedEvery     = 30 * 24 * time.Hour
	// Slack so a daily schedule doesn't skip a show fetched a little later
	// the day before.
	episodesDueSlack = 3 * time.Hour
	// Seasons that aired within this many years are fetched (plus the
	// latest season and seasons without a date).
	episodesRecentYears = 3
	// Shows fetched per run at most (the rest wait for the next run), and the
	// pause between TMDB requests.
	episodesMaxShows      = 1500
	episodesGap           = 250 * time.Millisecond
	episodesMaxFailStreak = 20
)

// TMDB statuses of shows that may get new seasons.
var returningStatuses = map[string]bool{"Returning Series": true, "In Production": true, "Planned": true, "Pilot": true}

type episodeSource interface {
	Enabled() bool
	TVSeasonEpisodes(ctx context.Context, tvID string, seasons []int) (map[int][]client.TMDBEpisode, error)
}

// EpisodesRefresher stores TMDB episodes of the TV shows metareel knows (the
// tmdb.episodes.refresh job): recent, upcoming and latest seasons, active
// shows daily, returning shows weekly, ended shows monthly.
type EpisodesRefresher struct {
	log      *slog.Logger
	source   episodeSource
	episodes *repository.EpisodesRepository
	now      func() time.Time
	gap      time.Duration
}

func NewEpisodesRefresher(log *slog.Logger, source episodeSource, episodes *repository.EpisodesRepository) *EpisodesRefresher {
	return &EpisodesRefresher{log: log, source: source, episodes: episodes, now: time.Now, gap: episodesGap}
}

// EpisodesReport summarises a run.
type EpisodesReport struct {
	Shows    int    `json:"shows"`    // TV shows with a TMDB season list
	Due      int    `json:"due"`      // due for a refresh
	Fetched  int    `json:"fetched"`  // shows fetched
	Seasons  int    `json:"seasons"`  // seasons stored
	Episodes int    `json:"episodes"` // episodes stored
	Missing  int    `json:"missing"`  // unknown to TMDB
	Failed   int    `json:"failed"`
	Deferred int    `json:"deferred"` // due, left for the next run (per-run cap)
	Skipped  string `json:"skipped,omitempty"`
}

// Run refreshes the episodes of due shows; force refreshes every show.
func (r *EpisodesRefresher) Run(ctx context.Context, force bool, runLog func(level, msg string)) (*EpisodesReport, error) {
	logf := func(level, format string, args ...any) {
		if runLog != nil {
			runLog(level, fmt.Sprintf(format, args...))
		}
	}
	rep := &EpisodesReport{}
	if r.source == nil || !r.source.Enabled() {
		rep.Skipped = "TMDB_API_KEY is not set"
		logf("warn", "TMDB episodes: skipped, TMDB_API_KEY is not set")
		return rep, nil
	}
	now := r.now().UTC()
	today := now.Truncate(24 * time.Hour)
	targets, err := r.episodes.EpisodeTargets(ctx, repository.FormatDate(today.AddDate(0, 0, -episodesActivePastDays)),
		repository.FormatDate(today.AddDate(0, 0, episodesActiveAheadDays)))
	if err != nil {
		return nil, err
	}
	rep.Shows = len(targets)
	var due []repository.EpisodeTarget
	for _, t := range targets {
		if force || episodesDue(t, now) {
			due = append(due, t)
		}
	}
	rep.Due = len(due)
	if len(due) > episodesMaxShows {
		rep.Deferred = len(due) - episodesMaxShows
		due = due[:episodesMaxShows] // never-fetched and oldest first (query order)
	}
	logf("info", "TMDB episodes: %d TV shows, %d due", rep.Shows, rep.Due)
	streak := 0
	for i, t := range due {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		if i > 0 && r.gap > 0 {
			if err := sleepCtx(ctx, r.gap); err != nil {
				return rep, err
			}
		}
		seasons := seasonsToFetch(t.Seasons, today)
		var eps map[int][]client.TMDBEpisode
		if len(seasons) > 0 {
			eps, err = r.source.TVSeasonEpisodes(ctx, t.TmdbID, seasons)
			if err != nil {
				rep.Failed++
				streak++
				if rep.Failed <= 5 {
					logf("warn", "tv %s: %v", t.TmdbID, err)
				}
				r.log.Warn("tmdb episodes fetch failed", slog.String("tmdb_id", t.TmdbID), slog.Any("err", err))
				if streak >= episodesMaxFailStreak {
					return rep, fmt.Errorf("tmdb episodes: %d requests in a row failed, last: %w", streak, err)
				}
				continue
			}
			if eps == nil {
				rep.Missing++
			}
		}
		streak = 0
		// Marked fetched even when TMDB has nothing, so it waits for its
		// next interval.
		if err := r.episodes.SaveEpisodes(ctx, t.TmdbID, eps); err != nil {
			return rep, err
		}
		rep.Fetched++
		rep.Seasons += len(eps)
		for _, e := range eps {
			rep.Episodes += len(e)
		}
	}
	logf("info", "TMDB episodes: %d shows fetched (%d seasons, %d episodes), %d unknown to TMDB, %d failed, %d left for the next run",
		rep.Fetched, rep.Seasons, rep.Episodes, rep.Missing, rep.Failed, rep.Deferred)
	if rep.Failed > 0 && rep.Fetched == 0 {
		return rep, fmt.Errorf("every TMDB episodes request failed (%d)", rep.Failed)
	}
	return rep, nil
}

// episodesDue reports whether a show's episodes need fetching at now.
func episodesDue(t repository.EpisodeTarget, now time.Time) bool {
	if t.FetchedAt == "" {
		return true
	}
	fetched, err := time.Parse("2006-01-02 15:04:05", t.FetchedAt)
	if err != nil {
		return true
	}
	every := episodesEndedEvery
	switch {
	case t.Airing || seasonStartsNear(t.Seasons, now.Truncate(24*time.Hour)):
		every = episodesActiveEvery
	case returningStatuses[t.Status]:
		every = episodesReturningEvery
	}
	return now.Sub(fetched) >= every-episodesDueSlack
}

// seasonStartsNear reports whether a regular season starts within
// episodesActiveSeasonDays of today.
func seasonStartsNear(seasons []repository.TmdbSeason, today time.Time) bool {
	lo := repository.FormatDate(today.AddDate(0, 0, -episodesActiveSeasonDays))
	hi := repository.FormatDate(today.AddDate(0, 0, episodesActiveSeasonDays))
	for _, s := range seasons {
		if s.Number >= 1 && s.AirDate != "" && s.AirDate >= lo && s.AirDate <= hi {
			return true
		}
	}
	return false
}

// seasonsToFetch picks the regular seasons that aired in the last three
// years or have no date yet, plus the latest season, in order.
func seasonsToFetch(seasons []repository.TmdbSeason, today time.Time) []int {
	cutoff := repository.FormatDate(today.AddDate(-episodesRecentYears, 0, 0))
	latest := 0
	pick := map[int]bool{}
	for _, s := range seasons {
		if s.Number < 1 {
			continue
		}
		latest = max(latest, s.Number)
		if s.AirDate == "" || s.AirDate >= cutoff {
			pick[s.Number] = true
		}
	}
	if latest > 0 {
		pick[latest] = true
	}
	out := make([]int, 0, len(pick))
	for n := range pick {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// --- calendar ---------------------------------------------------------------------

const (
	defaultCalendarDays = 14
	maxCalendarDays     = 62
)

// CalendarService lists upcoming episodes of shows streamable in a country.
type CalendarService struct {
	episodes *repository.EpisodesRepository
	now      func() time.Time
}

func NewCalendarService(episodes *repository.EpisodesRepository) *CalendarService {
	return &CalendarService{episodes: episodes, now: time.Now}
}

// CalendarQuery: dates default to today and the next 13 days.
type CalendarQuery struct {
	Country    string
	From, To   *time.Time
	ProviderID string // TMDB watch provider ID
}

// CalendarResponse lists episode releases, by date then show.
type CalendarResponse struct {
	Country    string          `json:"country"`
	From       string          `json:"from"`
	To         string          `json:"to"`
	ProviderID *int64          `json:"provider_id"`
	Items      []CalendarEntry `json:"items"`
}

// CalendarEntry is one show's episodes of one season on one date (a binge
// release is one entry with every episode).
type CalendarEntry struct {
	AirDate      string                     `json:"air_date"`
	TmdbID       string                     `json:"tmdb_id"`
	Title        string                     `json:"title"`
	PosterPath   *string                    `json:"poster_path"`
	SeasonNumber int64                      `json:"season_number"`
	Episodes     []CalendarEpisode          `json:"episodes"`
	Premiere     bool                       `json:"season_premiere"` // includes episode 1
	Finale       bool                       `json:"season_finale"`   // includes TMDB's finale episode
	Offers       []repository.CalendarOffer `json:"offers"`          // subscription, free and ad-supported offers in the country
}

// CalendarEpisode is one episode in a calendar entry.
type CalendarEpisode struct {
	EpisodeNumber int64  `json:"episode_number"`
	Name          string `json:"name"`
	EpisodeType   string `json:"episode_type,omitempty"`
	Runtime       *int64 `json:"runtime,omitempty"`
}

// Calendar returns the episodes airing in a date range of the shows with a
// subscription, free or ad-supported offer in the country.
func (s *CalendarService) Calendar(ctx context.Context, q CalendarQuery) (*CalendarResponse, error) {
	country, err := validCountry(q.Country, true)
	if err != nil {
		return nil, err
	}
	from := s.now().UTC().Truncate(24 * time.Hour)
	if q.From != nil {
		from = q.From.UTC()
	}
	to := from.AddDate(0, 0, defaultCalendarDays-1)
	if q.To != nil {
		to = q.To.UTC()
	}
	if from.After(to) {
		return nil, &ValidationError{Message: "from must not be after to"}
	}
	if to.Sub(from) >= maxCalendarDays*24*time.Hour {
		return nil, &ValidationError{Message: fmt.Sprintf("range must be at most %d days", maxCalendarDays)}
	}
	var providerID int64
	if raw := strings.TrimSpace(q.ProviderID); raw != "" {
		if providerID, err = strconv.ParseInt(raw, 10, 64); err != nil || providerID <= 0 {
			return nil, &ValidationError{Message: fmt.Sprintf("invalid provider_id %q", q.ProviderID)}
		}
	}
	fromS, toS := repository.FormatDate(from), repository.FormatDate(to)
	eps, err := s.episodes.CalendarEpisodes(ctx, country, fromS, toS, providerID)
	if err != nil {
		return nil, err
	}
	offers, err := s.episodes.CalendarOffers(ctx, country, fromS, toS)
	if err != nil {
		return nil, err
	}
	out := &CalendarResponse{Country: country, From: fromS, To: toS, Items: []CalendarEntry{}}
	if providerID != 0 {
		out.ProviderID = &providerID
	}
	for _, e := range eps {
		n := len(out.Items)
		if n == 0 || out.Items[n-1].AirDate != e.AirDate || out.Items[n-1].TmdbID != e.TmdbID || out.Items[n-1].SeasonNumber != e.SeasonNumber {
			entry := CalendarEntry{AirDate: e.AirDate, TmdbID: e.TmdbID, Title: e.Title, PosterPath: optStr(e.PosterPath),
				SeasonNumber: e.SeasonNumber, Episodes: []CalendarEpisode{}, Offers: offers[e.TmdbID]}
			if entry.Offers == nil {
				entry.Offers = []repository.CalendarOffer{}
			}
			out.Items = append(out.Items, entry)
			n++
		}
		entry := &out.Items[n-1]
		entry.Episodes = append(entry.Episodes, CalendarEpisode{EpisodeNumber: e.EpisodeNumber, Name: e.Name, EpisodeType: e.EpisodeType, Runtime: e.Runtime})
		if e.EpisodeNumber == 1 {
			entry.Premiere = true
		}
		if e.EpisodeType == "finale" {
			entry.Finale = true
		}
	}
	return out, nil
}
