package service

import (
	"context"
	"errors"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestOriginsAnalyticsAndChartOrigin(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	seed(t, db,
		[]seedTitle{
			{Slug: "wednesday", Name: "Wednesday", Kind: "tv_show", TmdbID: "119051"},
			{Slug: "minions", Name: "Minions", Kind: "movie", TmdbID: "438148"},
			{Slug: "apex", Name: "Apex", Kind: "movie", TmdbID: "1"},
			{Slug: "unmatched", Name: "Unmatched", Kind: "movie"},
			{Slug: "chicago-fire", Name: "Chicago Fire", Kind: "tv_show", TmdbID: "44006"},
		},
		[]seedRanking{
			// Week of 2026-10-05 (Mon): Netflix US.
			{"wednesday", "2026-10-06", "US", "netflix", "tv_shows", 1}, // 10 original
			{"minions", "2026-10-06", "US", "netflix", "movies", 2},     // 9 licensed (nbcu)
			{"apex", "2026-10-06", "US", "netflix", "movies", 1},        // 10 original (distributor)
			{"unmatched", "2026-10-06", "US", "netflix", "movies", 3},   // 8 unknown
			// Next week.
			{"minions", "2026-10-13", "US", "netflix", "movies", 1}, // 10 licensed
			// Peacock: an NBC show is in-house.
			{"chicago-fire", "2026-10-13", "US", "peacock", "tv_shows", 1},
		})
	meta := repository.NewMetadataRepository(db)
	for _, d := range []client.TMDBFullDetails{
		{Kind: "tv", ID: "119051", Networks: []string{"Netflix"}, ProductionCompanies: []string{"MGM Television"}},
		{Kind: "movie", ID: "438148", ReleaseDate: "2022-06-29", ProductionCompanies: []string{"Universal Pictures", "Illumination"}},
		{Kind: "movie", ID: "1", ReleaseDate: "2026-04-24", ProductionCompanies: []string{"Chernin Entertainment"}},
		{Kind: "tv", ID: "44006", Networks: []string{"NBC"}, ProductionCompanies: []string{"Universal Television"}},
	} {
		if err := meta.SaveDetails(ctx, &d, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := meta.SaveDistributors(ctx, "1", []string{"Netflix"}); err != nil {
		t.Fatal(err)
	}

	svc := NewAnalyticsService(repository.NewAnalyticsRepository(db), meta, repository.NewImportsRepository(db), repository.NewRatingsRepository(db))
	got, err := svc.Origins(ctx, AnalyticsQuery{From: timePtr(day("2026-10-01")), To: timePtr(day("2026-10-31"))})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Providers) != 2 || got.Providers[0].Provider != "netflix" || got.Providers[1].Provider != "peacock" {
		t.Fatalf("providers = %+v", got.Providers)
	}
	nf := got.Providers[0]
	if nf.Points != 47 {
		t.Fatalf("netflix points = %d, want 47", nf.Points)
	}
	want := map[string]int64{OriginOriginal: 20, OriginLicensed: 19, OriginUnknown: 8, OriginInHouse: 0}
	for o, p := range want {
		if nf.Origins[o].Points != p {
			t.Errorf("netflix %s points = %d, want %d (%+v)", o, nf.Origins[o].Points, p, nf.Origins[o])
		}
	}
	if nf.Origins[OriginOriginal].Share != 0.426 || nf.Origins[OriginOriginal].Titles != 2 {
		t.Errorf("original share = %+v", nf.Origins[OriginOriginal])
	}
	if len(nf.Series) != 2 || nf.Series[0].Period != "2026-10-05" || nf.Series[0].Points != 37 || nf.Series[1].Shares[OriginLicensed] != 1 {
		t.Errorf("series = %+v", nf.Series)
	}
	if len(nf.Suppliers) != 1 || nf.Suppliers[0].Owner != "nbcu" || nf.Suppliers[0].Name != "NBCUniversal" || nf.Suppliers[0].Share != 1 {
		t.Errorf("suppliers = %+v", nf.Suppliers)
	}
	if top := nf.Top[OriginOriginal]; len(top) != 2 || top[0].Name != "Apex" || top[0].Reason != "distributor: Netflix" {
		t.Errorf("top originals = %+v", top)
	}
	if pk := got.Providers[1]; pk.Origins[OriginInHouse].Points != 10 || pk.Origins[OriginInHouse].Share != 1 {
		t.Errorf("peacock = %+v", pk.Origins)
	}

	month, err := svc.Origins(ctx, AnalyticsQuery{From: timePtr(day("2026-10-01")), To: timePtr(day("2026-10-31")), Interval: "month", Provider: "netflix", Kind: "movie"})
	if err != nil {
		t.Fatal(err)
	}
	if len(month.Providers) != 1 || len(month.Providers[0].Series) != 1 || month.Providers[0].Series[0].Period != "2026-10-01" || month.Providers[0].Points != 37 {
		t.Errorf("monthly movies = %+v", month.Providers)
	}
	var v *ValidationError
	if _, err := svc.Origins(ctx, AnalyticsQuery{Interval: "day"}); !errors.As(err, &v) {
		t.Errorf("interval day: err = %v", err)
	}

	// Chart entries carry the origin too.
	top := NewTop10ReadService(repository.NewFlixPatrolRepository(db)).WithMetadata(meta)
	chart, err := top.GetMoviesByProvider(ctx, Top10Query{Date: day("2026-10-06"), CountryCode: "US", Provider: "netflix"})
	if err != nil {
		t.Fatal(err)
	}
	gotOrigins := map[string]string{}
	for _, it := range chart.Items {
		if it.Origin == nil {
			t.Fatalf("%s: no origin", it.Name)
		}
		gotOrigins[it.Name] = it.Origin.Origin
	}
	if gotOrigins["Apex"] != OriginOriginal || gotOrigins["Minions"] != OriginLicensed || gotOrigins["Unmatched"] != OriginUnknown {
		t.Errorf("chart origins = %v", gotOrigins)
	}
}
