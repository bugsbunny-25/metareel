package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// seedCharts stores Netflix TV charts keyed "date|country", titles in rank
// order.
func seedCharts(t *testing.T, seedFn func([]seedTitle, []seedRanking), charts map[string][]string) {
	t.Helper()
	known := map[string]bool{}
	var titles []seedTitle
	var ranks []seedRanking
	for key, slugs := range charts {
		date, country, _ := strings.Cut(key, "|")
		for i, s := range slugs {
			if !known[s] {
				known[s] = true
				titles = append(titles, seedTitle{Slug: s, Name: strings.ToUpper(s), Kind: "tv_show", TmdbID: tmdbOf(s)})
			}
			ranks = append(ranks, seedRanking{s, date, country, "netflix", "tv_shows", int64(i + 1)})
		}
	}
	seedFn(titles, ranks)
}

// tmdbOf gives titles named x1/x2 the same TMDB show (one title, two
// slugs); the others have none.
func tmdbOf(slug string) string {
	if strings.HasPrefix(slug, "x") {
		return "900"
	}
	return ""
}

func TestBreakouts(t *testing.T) {
	db := newTestDB(t)
	svc := NewBreakoutService(repository.NewAnalyticsRepository(db))
	ctx := context.Background()
	seedCharts(t, func(ti []seedTitle, r []seedRanking) { seed(t, db, ti, r) }, map[string][]string{
		// US: a enters at #2; b enters at #10.
		"2026-11-07|US": {"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9"},
		"2026-11-10|US": {"f1", "a", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "b"},
		// GB: a climbs #8 -> #3 (5 places); c climbs #6 -> #4 (2, under min_climb).
		"2026-11-07|GB": {"g1", "g2", "g3", "g4", "g5", "c", "g7", "a"},
		"2026-11-10|GB": {"g1", "g2", "a", "c", "g5", "g7"},
		// IN: chart stored a day late (still current); a and x1 (the TMDB
		// show also charting as x2) climb, b stays put.
		"2026-11-06|IN": {"i1", "i2", "i3", "i4", "i5", "a", "b", "i8", "x1"},
		"2026-11-09|IN": {"a", "i1", "i2", "i3", "b", "x2"},
		// DE: stale (latest chart 3 days before the date).
		"2026-11-03|DE": {"d1"},
		"2026-11-07|DE": {"d1", "a"},
	})
	date := day("2026-11-10")
	out, err := svc.Breakouts(ctx, BreakoutQuery{Date: &date})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(out.CountriesChecked, ","); got != "GB,IN,US" {
		t.Fatalf("countries checked = %s", got)
	}
	if out.Total != 1 || len(out.Items) != 1 {
		t.Fatalf("items = %+v", out.Items)
	}
	a := out.Items[0]
	if a.Name != "A" || a.RisingCountries != 3 || a.Score != 9+5+5 || strings.Join(a.ChartingCountries, ",") != "GB,IN,US" {
		t.Fatalf("a = %+v", a)
	}
	// Biggest gain first: US entered at #2 (9 points), then GB/IN by country.
	us, gb, in := a.Countries[0], a.Countries[1], a.Countries[2]
	if us.Country != "US" || us.Movement != "entered" || us.BaseRank != nil || us.Rank != 2 || us.BaseDate != "2026-11-07" || us.Gain != 9 {
		t.Fatalf("us = %+v", us)
	}
	if gb.Country != "GB" || gb.Movement != "climbed" || *gb.BaseRank != 8 || gb.Rank != 3 || gb.Gain != 5 {
		t.Fatalf("gb = %+v", gb)
	}
	if in.Country != "IN" || in.Date != "2026-11-09" || in.BaseDate != "2026-11-06" || *in.BaseRank != 6 || in.Rank != 1 {
		t.Fatalf("in = %+v", in)
	}

	// One country is enough with min_countries=1: b (US entry), x (IN, one
	// TMDB show across two slugs: best rank 9 -> 6); c's 2-place climb is not.
	out, err = svc.Breakouts(ctx, BreakoutQuery{Date: &date, MinCountries: 1})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, b := range out.Items {
		names = append(names, b.Name)
	}
	if got := strings.Join(names, ","); got != "A,X2,B" && got != "A,X1,B" {
		t.Fatalf("min_countries=1: %s", got)
	}
	// A 2-place climb counts with min_climb=2.
	out, _ = svc.Breakouts(ctx, BreakoutQuery{Date: &date, MinCountries: 1, MinClimb: 2, Limit: 2})
	if out.Total != 4 || len(out.Items) != 2 {
		t.Fatalf("min_climb=2: total %d, items %d", out.Total, len(out.Items))
	}
	// Movies only: nothing.
	if out, _ = svc.Breakouts(ctx, BreakoutQuery{Date: &date, Kind: "movie"}); out.Total != 0 || out.Items == nil {
		t.Fatalf("kind=movie: %+v", out)
	}
	// A wider window compares with the previous stored date within slack.
	if out, _ = svc.Breakouts(ctx, BreakoutQuery{Date: &date, Window: 14}); out.Total != 0 {
		t.Fatalf("window=14 (no base date): %+v", out.Items)
	}

	var v *ValidationError
	for _, q := range []BreakoutQuery{{Window: 15}, {MinClimb: 10}, {MinCountries: -1}, {Limit: 201}, {Category: "x"}, {Kind: "show"}} {
		if _, err := svc.Breakouts(ctx, q); !errors.As(err, &v) {
			t.Errorf("%+v: err = %v", q, err)
		}
	}
}

func TestWebhookBreakoutEvents(t *testing.T) {
	db := newTestDB(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := repository.NewWebhookRepository(db)
	top := NewTop10ReadService(repository.NewFlixPatrolRepository(db)).WithMetadata(repository.NewMetadataRepository(db))
	svc := NewWebhookService(log, repo, repository.NewOpsRepository(db), top, WebhookOptions{AllowLoopback: true}).
		WithBreakouts(NewBreakoutService(repository.NewAnalyticsRepository(db)))
	now := time.Date(2026, 11, 11, 15, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	today := flixpatrol.EffectiveTop10Date(now)
	d := func(offset int) string { return repository.FormatDate(today.AddDate(0, 0, offset)) }
	store := func(charts map[string][]string) {
		seedCharts(t, func(ti []seedTitle, r []seedRanking) { seed(t, db, ti, r) }, charts)
	}
	ctx := context.Background()
	rc := &receiver{}
	srv := httptest.NewServer(rc)
	defer srv.Close()
	if _, err := svc.Create(ctx, WebhookInput{Name: "b", URL: srv.URL, Events: []string{EventTitleBreakout}}); err != nil {
		t.Fatal(err)
	}
	// Base charts (and an old breakout outside the event days) exist before
	// following starts.
	store(map[string][]string{
		d(-4) + "|US": {"u1", "u2"}, d(-4) + "|GB": {"g1", "g2"},
	})
	svc.Tick(ctx)

	// Yesterday: a enters in both countries -> one event. An old date's
	// backfill (5 days ago) raises none.
	store(map[string][]string{
		d(-1) + "|US": {"a", "u1"}, d(-1) + "|GB": {"g1", "a"},
		d(-8) + "|US": {"o1"}, d(-8) + "|GB": {"o1"}, d(-5) + "|US": {"o1", "p"}, d(-5) + "|GB": {"o1", "p"},
	})
	svc.Tick(ctx)
	// Today: a still rising (no new event), b enters in both (new event).
	store(map[string][]string{
		d(0) + "|US": {"a", "b", "u1"}, d(0) + "|GB": {"b", "a", "g1"},
	})
	svc.Tick(ctx)
	svc.Tick(ctx)

	var got []string
	rc.mu.Lock()
	for _, body := range rc.bodies {
		var env struct {
			ID   string
			Type string
			Data BreakoutEventData
		}
		if err := json.Unmarshal([]byte(body), &env); err != nil {
			t.Fatal(err)
		}
		if env.Type != EventTitleBreakout || env.Data.RisingCountries != 2 || env.Data.Window != 3 || len(env.Data.Countries) != 2 {
			t.Errorf("event %s", body)
		}
		got = append(got, env.Data.Name+"@"+env.Data.Date)
	}
	rc.mu.Unlock()
	if want := "A@" + d(-1) + ",B@" + d(0); strings.Join(got, ",") != want {
		t.Fatalf("breakout events = %v, want %s", got, want)
	}
}
