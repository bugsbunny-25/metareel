package service

import (
	"context"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// queueEnv is alice with one show in every stage, on a fixed clock
// (2026-11-13 12:00 UTC).
func queueEnv(t *testing.T) (*TrackingService, *fakeTitleSource, int64, int64) {
	t.Helper()
	s, src, alice, bob := newTrackingEnv(t)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	s.titles.now = s.now

	show := func(id, title string, eps ...client.TMDBEpisode) {
		src.details["tv/"+id] = client.TMDBTitleDetails{Title: title, Found: true, NumberOfSeasons: 1,
			Seasons: []client.TMDBSeason{{Number: 1}}}
		for i := range eps {
			eps[i].SeasonNumber, eps[i].EpisodeNumber = 1, i+1
		}
		src.episodes[id] = map[int][]client.TMDBEpisode{1: eps}
	}
	ep := func(name, airDate string) client.TMDBEpisode {
		return client.TMDBEpisode{Name: name, AirDate: airDate, Runtime: 30}
	}
	show("2001", "Stale Show", ep("One", "2026-01-01"), ep("Two", "2026-01-08"))
	show("2002", "Wishlist Show", ep("Premiere", "2026-12-01"))
	show("2003", "Paused Show", ep("One", "2026-01-01"), ep("Two", "2026-01-08"), ep("Later", "2026-11-20"))
	show("2004", "Dropped Show", ep("One", "2026-01-01"), ep("Later", "2026-11-21"))
	show("2005", "Caught Up Show", ep("One", "2026-11-06"), ep("Today", "2026-11-13"),
		ep("Next", "2026-11-20"), ep("Far", "2027-01-30"))
	show("2006", "Unwanted Show", ep("One", "2026-01-01"), ep("Later", "2026-11-22"))

	ctx := context.Background()
	mark := func(id string, scope string, season, episode *int64, endedAt string) {
		t.Helper()
		if _, err := s.MarkEpisodes(ctx, alice, id, MarkEpisodesInput{Scope: scope,
			SeasonNumber: season, EpisodeNumber: episode, EndedAt: endedAt}); err != nil {
			t.Fatalf("mark %s: %v", id, err)
		}
	}
	status := func(id, st string) {
		t.Helper()
		if _, err := s.SetTitleState(ctx, alice, "tv", id, TitleStateInput{Status: tp(st)}); err != nil {
			t.Fatalf("status %s %s: %v", id, st, err)
		}
	}
	mark("1396", ScopeUpTo, tp(int64(1)), tp(int64(2)), "2026-11-12T20:00:00Z")
	mark("2001", ScopeEpisode, tp(int64(1)), tp(int64(1)), "2026-09-01T20:00:00Z")
	status("2002", StatusWatchlist)
	mark("2003", ScopeEpisode, tp(int64(1)), tp(int64(1)), "2026-11-01T20:00:00Z")
	status("2003", StatusOnHold)
	status("2004", StatusDropped)
	mark("2005", ScopeAll, nil, nil, "2026-11-13T10:00:00Z")
	status("2006", StatusNotInterested)
	return s, src, alice, bob
}

func TestShowQueueStages(t *testing.T) {
	s, _, alice, bob := queueEnv(t)
	ctx := context.Background()

	out, err := s.ShowQueue(ctx, alice, 0)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	shows := out["shows"].([]ShowQueueItem)
	got := make([]string, 0, len(shows))
	for _, sh := range shows {
		got = append(got, sh.TmdbID+":"+sh.Stage)
	}
	want := []string{"1396:watching", "2001:stalled", "2002:not_started", "2003:on_hold", "2004:dropped", "2005:caught_up"}
	if len(got) != len(want) {
		t.Fatalf("queue = %v, want %v (not interested left out)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("queue = %v, want %v", got, want)
		}
	}
	if c := out["counts"].(map[string]int); c[StageWatching] != 1 || c[StageCaughtUp] != 1 || c[StageDropped] != 1 {
		t.Errorf("counts = %v", c)
	}

	bb := shows[0]
	// Breaking Bad: 5 aired episodes outside specials (S2E3 has no date), 2 seen.
	if bb.WatchedEpisodes != 2 || bb.AiredEpisodes != 5 || bb.RemainingEpisodes != 3 || bb.Percent != 40 {
		t.Errorf("progress = %d/%d (%d left, %d%%), want 2/5, 3 left, 40%%",
			bb.WatchedEpisodes, bb.AiredEpisodes, bb.RemainingEpisodes, bb.Percent)
	}
	if bb.NextEpisode == nil || bb.NextEpisode.SeasonNumber != 1 || bb.NextEpisode.EpisodeNumber != 3 {
		t.Errorf("next = %+v, want S1E3", bb.NextEpisode)
	}
	if bb.MinutesLeft != 48+47+47 {
		t.Errorf("minutes_left = %d, want %d", bb.MinutesLeft, 48+47+47)
	}

	// A show not started yet has its episodes fetched on the way, so its
	// next air date is known; the caught-up show's is the next unaired one.
	if ws := shows[2]; ws.WatchedEpisodes != 0 || ws.NextAirDate == nil || *ws.NextAirDate != "2026-12-01" {
		t.Errorf("wishlist show = %+v", ws)
	}
	if cu := shows[5]; cu.NextEpisode != nil || cu.NextAirDate == nil || *cu.NextAirDate != "2026-11-20" || cu.Percent != 100 {
		t.Errorf("caught-up show = %+v, want no next episode, next air 2026-11-20, 100%%", cu)
	}

	// A wider stale window puts the stalled show back in progress.
	out, err = s.ShowQueue(ctx, alice, 90)
	if err != nil {
		t.Fatalf("queue 90: %v", err)
	}
	if st := out["shows"].([]ShowQueueItem)[1]; st.TmdbID != "2001" || st.Stage != StageWatching {
		t.Errorf("with stale_days=90 second show = %s %s, want 2001 watching", st.TmdbID, st.Stage)
	}

	for _, bad := range []int{3, 400, -1} {
		if _, err := s.ShowQueue(ctx, alice, bad); !isValidation(err) {
			t.Errorf("stale_days=%d: err = %v, want a validation error", bad, err)
		}
	}

	out, err = s.ShowQueue(ctx, bob, 0)
	if err != nil {
		t.Fatalf("bob queue: %v", err)
	}
	if n := len(out["shows"].([]ShowQueueItem)); n != 0 {
		t.Errorf("bob sees %d shows, want 0", n)
	}
}

// A completed show with a new episode out is back in progress.
func TestShowQueueNewEpisodeReopensACompletedShow(t *testing.T) {
	s, src, alice, _ := queueEnv(t)
	ctx := context.Background()
	later := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return later }
	s.titles.now = s.now
	_ = src

	out, err := s.ShowQueue(ctx, alice, 0)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	for _, sh := range out["shows"].([]ShowQueueItem) {
		if sh.TmdbID == "2005" {
			if sh.Stage != StageWatching || sh.NextEpisode == nil || sh.NextEpisode.EpisodeNumber != 3 {
				t.Errorf("caught-up show after a new episode = %s next %+v, want watching S1E3", sh.Stage, sh.NextEpisode)
			}
			return
		}
	}
	t.Fatal("show 2005 missing from the queue")
}

func TestUpcoming(t *testing.T) {
	s, _, alice, _ := queueEnv(t)
	ctx := context.Background()

	list := func(days int) []string {
		t.Helper()
		out, err := s.Upcoming(ctx, alice, days)
		if err != nil {
			t.Fatalf("upcoming %d: %v", days, err)
		}
		var got []string
		for _, e := range out["episodes"].([]repository.UpcomingEpisode) {
			got = append(got, e.TmdbID+"@"+*e.AirDate)
		}
		return got
	}
	// Not today's episode (aired), not the shows on hold, dropped or unwanted.
	want := []string{"2005@2026-11-20", "2002@2026-12-01", "2005@2027-01-30"}
	if got := list(0); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("upcoming = %v, want %v", got, want)
	}
	if got := list(10); len(got) != 1 || got[0] != "2005@2026-11-20" {
		t.Errorf("upcoming 10 days = %v, want just 2005@2026-11-20", got)
	}
	for _, bad := range []int{-1, 366} {
		if _, err := s.Upcoming(ctx, alice, bad); !isValidation(err) {
			t.Errorf("days=%d: err = %v, want a validation error", bad, err)
		}
	}
}
