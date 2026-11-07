package service

import (
	"context"
	"errors"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestReleaseWindows(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	meta := repository.NewMetadataRepository(db)
	rel := func(country string, typ int, date string) client.TMDBReleaseDate {
		return client.TMDBReleaseDate{Country: country, Type: typ, Date: date}
	}
	save := func(id, title string, dates []client.TMDBReleaseDate, offers ...client.TMDBWatchProvider) {
		t.Helper()
		if err := meta.SaveDetails(ctx, &client.TMDBFullDetails{Kind: "movie", ID: id, Title: title, ReleaseDate: "2026-01-01",
			ReleaseDates: dates, ReleaseDatesFetched: true, WatchProviders: offers}, false); err != nil {
			t.Fatal(err)
		}
	}
	// Replaced on the next refresh: this GB date is gone afterwards.
	save("501", "M1", []client.TMDBReleaseDate{rel("GB", client.ReleaseTV, "2026-01-02")})
	save("501", "M1", []client.TMDBReleaseDate{
		rel("US", client.ReleaseTheatricalLimited, "2026-03-01"), rel("US", client.ReleaseTheatrical, "2026-03-06"),
		rel("US", client.ReleaseDigital, "2026-03-24"), rel("GB", client.ReleaseTheatrical, "2026-03-13"),
	})
	save("502", "M2", []client.TMDBReleaseDate{rel("US", client.ReleaseTheatricalLimited, "2026-04-10"), rel("US", client.ReleaseDigital, "2026-05-01")},
		client.TMDBWatchProvider{Country: "US", Monetization: "flatrate", ProviderID: 9, ProviderName: "Amazon Prime Video"})
	save("503", "M3", []client.TMDBReleaseDate{rel("US", client.ReleaseTheatrical, "2026-05-01")})
	// A refresh without a release_dates block keeps the stored dates.
	if err := meta.SaveDetails(ctx, &client.TMDBFullDetails{Kind: "movie", ID: "503", Title: "M3"}, false); err != nil {
		t.Fatal(err)
	}
	save("504", "M4", []client.TMDBReleaseDate{rel("US", client.ReleaseTheatrical, "2025-01-01"), rel("US", client.ReleaseDigital, "2025-02-01")})
	save("505", "M5", []client.TMDBReleaseDate{rel("US", client.ReleaseTheatrical, "2026-02-01")})
	save("600", "Show", nil) // no release dates

	event := func(id, monetization, provider, observed string) {
		if _, err := db.ExecContext(ctx, `INSERT INTO watch_provider_events (tmdb_kind, tmdb_id, country, monetization, provider_id, provider_name, event, observed_at)
			VALUES ('movie', ?, 'US', ?, 8, ?, 'added', ?)`, id, monetization, provider, observed); err != nil {
			t.Fatal(err)
		}
	}
	event("501", "flatrate", "Netflix", "2026-06-20 05:00:00")
	event("501", "flatrate", "Netflix", "2026-07-20 05:00:00") // later arrival again: the first counts
	event("501", "rent", "Netflix", "2026-03-24 05:00:00")     // not streaming
	event("505", "flatrate", "Netflix", "2026-01-15 05:00:00") // before the theatrical release: ignored

	// Netflix US movies stored daily from 06-01; m1 first charts on 06-18.
	var titles []seedTitle
	var ranks []seedRanking
	titles = append(titles, seedTitle{Slug: "f", Name: "F", Kind: "movie"}, seedTitle{Slug: "m1", Name: "M1", Kind: "movie", TmdbID: "501"},
		seedTitle{Slug: "m5", Name: "M5", Kind: "movie", TmdbID: "505"})
	for d := day("2026-06-01"); !d.After(day("2026-06-25")); d = d.AddDate(0, 0, 1) {
		ds := repository.FormatDate(d)
		ranks = append(ranks, seedRanking{"f", ds, "US", "netflix", "movies", 1})
		if ds >= "2026-06-18" {
			ranks = append(ranks, seedRanking{"m1", ds, "US", "netflix", "movies", 2})
		}
	}
	// m5 on a chart's first stored day: no streaming evidence.
	ranks = append(ranks, seedRanking{"m5", "2026-03-01", "US", "disney", "movies", 1})
	seed(t, db, titles, ranks)

	svc := NewReleaseWindowsService(repository.NewReleaseDatesRepository(db), meta, repository.NewAnalyticsRepository(db))
	from, to := day("2026-01-01"), day("2026-10-31")
	out, err := svc.Windows(ctx, ReleaseWindowsQuery{From: &from, To: &to})
	if err != nil {
		t.Fatal(err)
	}
	if out.Country != "US" || out.Movies != 4 || out.ItemsTotal != 4 {
		t.Fatalf("movies = %d, items %d", out.Movies, out.ItemsTotal)
	}
	var order []string
	for _, it := range out.Items {
		order = append(order, it.TmdbID)
	}
	if got := joinLines(order); got != joinLines([]string{"503", "502", "501", "505"}) {
		t.Fatalf("order = %v", order)
	}
	td := out.TheatricalToDigital
	if td.Titles != 2 || td.Missing != 2 || *td.Median != 19.5 || td.WithinDays["17"] != 0 || td.WithinDays["31"] != 2 {
		t.Fatalf("theatrical to digital = %+v", td)
	}
	ts := out.TheatricalToStreaming
	if ts.Titles != 1 || ts.Missing != 3 || *ts.Median != 104 || ts.WithinDays["90"] != 0 {
		t.Fatalf("theatrical to streaming = %+v", ts)
	}
	if ds := out.DigitalToStreaming; ds.Titles != 1 || *ds.Median != 86 || ds.Missing != 1 {
		t.Fatalf("digital to streaming = %+v", ds)
	}
	if len(out.StreamingByProvider) != 1 || out.StreamingByProvider[0].Provider != "netflix" || out.StreamingByProvider[0].Basis != "first_chart" {
		t.Fatalf("by provider = %+v", out.StreamingByProvider)
	}
	m1, m2, m5 := out.Items[2], out.Items[1], out.Items[3]
	if m1.Streaming == nil || m1.Streaming.Date != "2026-06-18" || *m1.Windows.TheatricalBasis != "wide" || *m1.Windows.TheatricalToDigital != 18 {
		t.Fatalf("m1 = %+v (streaming %+v)", m1, m1.Streaming)
	}
	if m2.Streaming != nil || len(m2.StreamingNow) != 1 || m2.StreamingNow[0].ProviderName != "Amazon Prime Video" ||
		*m2.Windows.TheatricalBasis != "limited" || *m2.Windows.TheatricalToDigital != 21 {
		t.Fatalf("m2 = %+v", m2)
	}
	if m5.Streaming != nil {
		t.Fatalf("m5 streaming = %+v (early offer and an uncovered chart day must not count)", m5.Streaming)
	}

	// One movie, every country.
	tr, err := svc.TitleReleases(ctx, TmdbRef{Kind: "movie", ID: "501"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Countries) != 2 || tr.Countries[0].Country != "GB" || tr.Countries[0].TV != nil || *tr.Countries[0].Theatrical != "2026-03-13" ||
		tr.Countries[1].Streaming == nil || *tr.Countries[1].Windows.TheatricalToStreaming != 104 || *tr.Title != "M1" {
		t.Fatalf("title releases = %+v", tr)
	}
	// A country filter (any case).
	if tr, err = svc.TitleReleases(ctx, TmdbRef{Kind: "movie", ID: "501"}, "us"); err != nil || len(tr.Countries) != 1 {
		t.Fatalf("country=us: %+v, %v", tr, err)
	}
	var v *ValidationError
	if _, err := svc.TitleReleases(ctx, TmdbRef{Kind: "tv", ID: "600"}, ""); !errors.As(err, &v) {
		t.Fatalf("tv: %v", err)
	}
	if _, err := svc.TitleReleases(ctx, TmdbRef{Kind: "movie", ID: "999"}, ""); !errors.Is(err, ErrTitleNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := svc.Windows(ctx, ReleaseWindowsQuery{Country: "USA"}); !errors.As(err, &v) {
		t.Fatalf("bad country: %v", err)
	}
	if _, err := svc.Windows(ctx, ReleaseWindowsQuery{Limit: 501}); !errors.As(err, &v) {
		t.Fatalf("bad limit: %v", err)
	}
}
