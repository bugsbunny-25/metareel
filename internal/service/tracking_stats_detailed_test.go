package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDetailedStats(t *testing.T) {
	s, _, alice, bob := newTrackingEnv(t)
	ctx := context.Background()
	// Saturday 14 November 2026, noon UTC.
	s.now = func() time.Time { return time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC) }

	log := func(user int64, in WatchInput) {
		t.Helper()
		if _, _, err := s.LogWatch(ctx, user, in); err != nil {
			t.Fatalf("log %+v: %v", in, err)
		}
	}
	ep := func(season, episode int64, at string) WatchInput {
		return WatchInput{Kind: "tv", TmdbID: "1396", SeasonNumber: tp(season), EpisodeNumber: tp(episode), EndedAt: at}
	}
	// Three episodes on 13 Nov (a binge), one on 2 Nov, a rewatch of S1E1 on 14 Nov.
	log(alice, ep(1, 1, "2026-11-13T20:00:00Z"))
	log(alice, ep(1, 2, "2026-11-13T21:00:00Z"))
	log(alice, ep(1, 3, "2026-11-13T22:00:00Z"))
	log(alice, ep(2, 1, "2026-11-02T20:00:00Z"))
	log(alice, ep(1, 1, "2026-11-14T10:00:00Z"))
	// Films: Inception twice (rated 8 and 10), The Dark Knight once last year.
	r8, r10 := int64(8), int64(10)
	log(alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-10T22:00:00Z", Rating: &r8})
	log(alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-12T22:00:00Z", Rating: &r10})
	log(alice, WatchInput{Kind: "movie", TmdbID: "155", EndedAt: "2025-06-01T22:00:00Z"})
	// Bob's watch never shows up in Alice's stats.
	log(bob, ep(1, 1, "2026-11-13T20:00:00Z"))

	all, err := s.DetailedStats(ctx, alice, "")
	if err != nil {
		t.Fatal(err)
	}
	if all.Watches != 8 || all.Episodes != 5 || all.Movies != 3 || all.Shows != 1 || all.Films != 2 || all.Rewatches != 2 {
		t.Fatalf("totals = %+v", all)
	}
	if all.ThisYearEpisodes != 5 || all.ThisYearMovies != 2 {
		t.Errorf("this year = %d episodes, %d movies", all.ThisYearEpisodes, all.ThisYearMovies)
	}
	// 13 and 14 Nov in a row; 12 Nov too (Inception) makes three.
	if all.Streak.Current != 3 {
		t.Errorf("current streak = %d, want 3", all.Streak.Current)
	}

	tv, err := s.DetailedStats(ctx, alice, "tv")
	if err != nil {
		t.Fatal(err)
	}
	if tv.Watches != 5 || tv.Movies != 0 || len(tv.TopTitles) != 1 || tv.TopTitles[0].Title != "Breaking Bad" || tv.TopTitles[0].Watches != 5 {
		t.Fatalf("tv = %+v", tv)
	}
	if len(tv.Binges) != 1 || tv.Binges[0].Day != "2026-11-13" || tv.Binges[0].Episodes != 3 {
		t.Errorf("binges = %+v", tv.Binges)
	}
	// The week of Monday 9 Nov holds the four latest episodes; the week of 2 Nov one.
	weeks := tv.Trend.Weeks
	if len(weeks) != detailTrendWeeks || weeks[len(weeks)-1].Label != "2026-11-09" || weeks[len(weeks)-1].Watches != 4 || weeks[len(weeks)-2].Watches != 1 {
		t.Errorf("weeks = %+v", weeks[len(weeks)-2:])
	}
	if tv.ThisWeek.Watches != 4 || tv.ThisMonth.Watches != 5 {
		t.Errorf("this week %d, this month %d", tv.ThisWeek.Watches, tv.ThisMonth.Watches)
	}
	if len(tv.ShowsByStage) == 0 || tv.MinutesLeft == nil {
		t.Fatalf("show stats missing: %+v", tv.ShowsByStage)
	}
	var watching int64
	for _, st := range tv.ShowsByStage {
		if st.Key == StageWatching {
			watching = st.Count
		}
	}
	if watching != 1 {
		t.Errorf("watching = %d, want 1 (%+v)", watching, tv.ShowsByStage)
	}
	if len(tv.UpcomingByMonth) != detailUpcomingMon || tv.UpcomingByMonth[0].Label != "2026-11" {
		t.Errorf("upcoming = %+v", tv.UpcomingByMonth)
	}

	mv, err := s.DetailedStats(ctx, alice, "movie")
	if err != nil {
		t.Fatal(err)
	}
	if mv.Watches != 3 || mv.Ratings.Rated != 2 || mv.Ratings.Average == nil || *mv.Ratings.Average != 9 || mv.ShowsByStage != nil {
		t.Fatalf("movie = %+v", mv)
	}
	if mv.TopTitles[0].Title != "Inception" || mv.TopTitles[0].Minutes != 296 {
		t.Errorf("top film = %+v, want Inception, 2 x 148 minutes", mv.TopTitles[0])
	}
	if len(mv.Trend.Years) != 2 || mv.Trend.Years[0].Label != "2025" {
		t.Errorf("years = %+v", mv.Trend.Years)
	}

	var ve *ValidationError
	if _, err := s.DetailedStats(ctx, alice, "books"); !errors.As(err, &ve) {
		t.Fatalf("bad kind err = %v, want a ValidationError", err)
	}
}
