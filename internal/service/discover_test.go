package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

func TestDiscover(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC)
	today := flixpatrol.EffectiveTop10Date(now)
	d := func(offset int) string { return repository.FormatDate(today.AddDate(0, 0, offset)) }

	meta := repository.NewMetadataRepository(db)
	offer := func(country, monetization string, id int, name string) client.TMDBWatchProvider {
		return client.TMDBWatchProvider{Country: country, Monetization: monetization, ProviderID: id, ProviderName: name}
	}
	save := func(kind, id, title, imdb string, offers ...client.TMDBWatchProvider) {
		t.Helper()
		if err := meta.SaveDetails(ctx, &client.TMDBFullDetails{Kind: kind, ID: id, Title: title, ReleaseDate: "2024-05-01", IMDbID: imdb,
			WatchProviders: offers, PosterPath: "/" + id + ".jpg"}, false); err != nil {
			t.Fatal(err)
		}
	}
	netflix := func(c string) client.TMDBWatchProvider { return offer(c, "flatrate", 8, "Netflix") }
	save("movie", "101", "A", "tt101", netflix("US"))                                                        // gem
	save("movie", "102", "B", "tt102", netflix("US"), offer("US", "ads", 1796, "Netflix Standard with Ads")) // few votes
	save("tv", "103", "C", "tt103", offer("US", "flatrate", 9, "Prime Video"))                               // charts in the US
	save("movie", "104", "D", "tt104", offer("US", "rent", 2, "Apple TV"))                                   // rent only
	save("movie", "105", "E", "tt105", netflix("US"), offer("US", "flatrate", 257, "fuboTV"))                // low rating (fuboTV: a TV bundle)
	save("movie", "106", "F", "tt106", offer("US", "rent", 2, "Apple TV"), netflix("GB"))                    // elsewhere only
	for imdb, r := range map[string][2]float64{"tt101": {8.6, 200000}, "tt102": {9.0, 300}, "tt103": {8.0, 50000}, "tt104": {8.2, 40000},
		"tt105": {6.0, 90000}, "tt106": {7.1, 10000}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO imdb_ratings (imdb_id, average_rating, num_votes) VALUES (?, ?, ?)`, imdb, r[0], int64(r[1])); err != nil {
			t.Fatal(err)
		}
	}
	seed(t, db, []seedTitle{
		{Slug: "a", Name: "A", Kind: "movie", TmdbID: "101"}, {Slug: "c", Name: "C", Kind: "tv_show", TmdbID: "103"},
		{Slug: "f", Name: "F", Kind: "movie", TmdbID: "106"}, {Slug: "g", Name: "G", Kind: "movie", TmdbID: "107"},
	}, []seedRanking{
		{"a", d(-2), "GB", "netflix", "movies", 3},
		{"f", d(-1), "GB", "netflix", "movies", 1}, {"f", d(0), "GB", "netflix", "movies", 2},
		{"g", d(0), "GB", "netflix", "movies", 4}, // no TMDB details: availability unknown
		{"c", d(-3), "US", "amazon-prime", "tv_shows", 1}, {"c", d(-2), "US", "amazon-prime", "tv_shows", 1}, {"c", d(-1), "US", "amazon-prime", "tv_shows", 1},
	})
	// Netflix US: A arrived this month; Prime US: C has an episode coming.
	if _, err := db.ExecContext(ctx, `INSERT INTO watch_provider_events (tmdb_kind, tmdb_id, country, monetization, provider_id, provider_name, event, observed_at)
		VALUES ('movie', '101', 'US', 'flatrate', 8, 'Netflix', 'added', ?)`, d(-5)+" 04:00:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO tmdb_episodes (tmdb_id, season_number, episode_number, name, air_date) VALUES ('103', 2, 1, 'Pilot', ?)`,
		now.AddDate(0, 0, 3).Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}

	svc := NewDiscoverService(repository.NewDiscoverRepository(db), repository.NewAnalyticsRepository(db), repository.NewEpisodesRepository(db), meta)
	svc.now = func() time.Time { return now }

	// Hidden gems: A (rated, streamable, not in the US Top 10), not C
	// (charts here), D (rent only), E (rating), B (votes).
	gems, err := svc.HiddenGems(ctx, HiddenGemsQuery{Country: "us"})
	if err != nil {
		t.Fatal(err)
	}
	if gems.Total != 1 || gems.Considered != 2 || gems.Items[0].TmdbID != "101" || gems.Items[0].PointsElsewhere != 8 ||
		len(gems.Items[0].ChartedIn) != 1 || gems.Items[0].StreamingHere[0].ProviderName != "Netflix" || *gems.Items[0].PosterPath != "/101.jpg" {
		t.Fatalf("gems = %+v", gems)
	}
	if s := gems.Items[0].Score; s < 8.2 || s > 8.4 {
		t.Fatalf("A score = %v (8.6 pulled toward 6.5)", s)
	}
	gems, _ = svc.HiddenGems(ctx, HiddenGemsQuery{Country: "US", MinVotes: 100})
	if gems.Total != 2 || gems.Items[1].TmdbID != "102" || gems.Items[1].Score > 6.6 {
		t.Fatalf("few votes rank low: %+v", gems.Items)
	}
	if gems, _ = svc.HiddenGems(ctx, HiddenGemsQuery{Country: "US", Kind: "tv"}); gems.Total != 0 {
		t.Fatalf("kind=tv: %+v", gems.Items)
	}
	if gems, _ = svc.HiddenGems(ctx, HiddenGemsQuery{Country: "US", ProviderID: 9}); gems.Considered != 1 || gems.Total != 0 {
		t.Fatalf("provider 9: %+v", gems)
	}

	// Elsewhere (US): F charts in GB and only rents in the US.
	el, err := svc.Elsewhere(ctx, ElsewhereQuery{Country: "US"})
	if err != nil {
		t.Fatal(err)
	}
	if el.Total != 1 || el.Items[0].TmdbID != "106" || el.Items[0].Points != 19 || el.Items[0].Countries[0].Country != "GB" ||
		len(el.Items[0].RentBuyHere) != 1 || *el.Items[0].IMDbRating != 7.1 {
		t.Fatalf("elsewhere = %+v", el)
	}
	if el.Skipped["available_here"] != 1 || el.Skipped["availability_unknown"] != 1 || el.Skipped["charting_here"] != 1 ||
		len(el.CountriesChecked) != 1 || el.CountriesChecked[0] != "GB" {
		t.Fatalf("skipped = %v, checked %v", el.Skipped, el.CountriesChecked)
	}
	// GB: A streams there? no (US only): A, C (no GB offers) count as elsewhere; F streams in GB.
	if gb, err := svc.Elsewhere(ctx, ElsewhereQuery{Country: "GB"}); err != nil || gb.Total != 1 || gb.Items[0].TmdbID != "103" {
		t.Fatalf("GB = %+v, %v", gb, err)
	}

	// Rotation (US): Prime (30 chart points + 10 for C's episodes + 1 top
	// rated) ahead of Netflix (an arrival + A top rated).
	rot, err := svc.Rotation(ctx, RotationQuery{Country: "US", Slots: 1, MinTitles: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(rot.Services) != 2 || rot.ChartBasis != "country" {
		t.Fatalf("rotation = %+v", rot)
	}
	prime, nf := rot.Services[0], rot.Services[1]
	if prime.ProviderID != 9 || prime.Score != 30+10+1 || prime.Recommendation != "subscribe" || prime.UpcomingShow != 1 || prime.Charting != 1 {
		t.Fatalf("prime = %+v", prime)
	}
	if nf.ProviderID != 8 || nf.Score != 3+1 || nf.Arrivals != 1 || nf.Recommendation != "skip" || nf.Titles != 3 || len(nf.ProviderIDs) != 2 {
		t.Fatalf("netflix (its ad plan counted in) = %+v", nf)
	}
	// Other providers (here a live-TV bundle) only on request.
	if all, _ := svc.Rotation(ctx, RotationQuery{Country: "US", MinTitles: 1, IncludeOther: true}); len(all.Services) != 3 {
		t.Fatalf("include_other = %+v", all.Services)
	}
	rot, _ = svc.Rotation(ctx, RotationQuery{Country: "US", Slots: 1, MinTitles: 1, Subscribed: []int64{1796}}) // the ad plan
	if rot.Services[0].Recommendation != "add" || rot.Services[1].Recommendation != "pause" {
		t.Fatalf("with subscriptions = %+v", rot.Services)
	}
	// A country without charts falls back to every country's.
	if fr, _ := svc.Rotation(ctx, RotationQuery{Country: "FR", MinTitles: 1}); fr.ChartBasis != "all_countries" || len(fr.Services) != 0 {
		t.Fatalf("FR = %+v", fr)
	}

	var v *ValidationError
	for _, err := range []error{
		func() error { _, err := svc.HiddenGems(ctx, HiddenGemsQuery{Country: "USA"}); return err }(),
		func() error { _, err := svc.HiddenGems(ctx, HiddenGemsQuery{Country: "US", MinRating: 11}); return err }(),
		func() error {
			_, err := svc.HiddenGems(ctx, HiddenGemsQuery{Country: "US", MinRating: math.NaN()})
			return err
		}(),
		func() error { _, err := svc.HiddenGems(ctx, HiddenGemsQuery{Country: "US", Days: 181}); return err }(),
		func() error { _, err := svc.HiddenGems(ctx, HiddenGemsQuery{Country: "US", Kind: "show"}); return err }(),
		func() error { _, err := svc.Elsewhere(ctx, ElsewhereQuery{Country: "US", Days: 31}); return err }(),
		func() error { _, err := svc.Rotation(ctx, RotationQuery{Country: "US", Slots: 11}); return err }(),
		func() error {
			_, err := svc.Rotation(ctx, RotationQuery{Country: "US", Subscribed: make([]int64, 21)})
			return err
		}(),
	} {
		if !errors.As(err, &v) {
			t.Errorf("err = %v, want ValidationError", err)
		}
	}
}
