package service

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestTitleFactsFillTop10(t *testing.T) {
	db := posterFixture(t)
	ctx := context.Background()
	if _, err := db.Exec(`
		UPDATE tmdb_titles SET overview = 'A show.', backdrop_path = '/show-bd.jpg', release_date = '2024-03-01',
		  runtime = 48, number_of_seasons = 3, genres = '["Drama","Crime"]',
		  certifications = '{"US":"TV-MA","GB":"15"}', logo_path = '/show-logo.png'
		WHERE tmdb_kind = 'tv' AND tmdb_id = '200';
		UPDATE tmdb_titles SET overview = 'A special.', release_date = '2025-12-24', runtime = 95, number_of_seasons = 2,
		  certifications = '{"GB":"12"}'
		WHERE tmdb_kind = 'movie' AND tmdb_id = '100';
		INSERT INTO title_ratings (tmdb_kind, tmdb_id, refreshed_at) VALUES ('tv', '200', '2026-10-01 00:00:00');
		INSERT INTO title_rating_sources (tmdb_kind, tmdb_id, provider, source, value, fetched_at)
		VALUES ('tv', '200', 'rottentomatoes', 'tomatoes', 91, '2026-10-01 00:00:00')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	meta := repository.NewMetadataRepository(db)
	top := NewTop10ReadService(repository.NewFlixPatrolRepository(db)).WithMetadata(meta)
	ratings := &RatingsService{
		log: slog.New(slog.NewTextHandler(io.Discard, nil)), repo: repository.NewRatingsRepository(db),
		ttl: 12 * time.Hour, preferred: ProviderJustWatch,
		now: func() time.Time { return time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC) },
	}
	facts := NewTitleFactsService(meta, ratings)

	get := func(withRatings bool) map[string]Top10Result {
		out, err := top.GetTVShowsByProvider(ctx, Top10Query{CountryCode: "US", Provider: "netflix"})
		if err != nil {
			t.Fatal(err)
		}
		if err := facts.Fill(ctx, out, withRatings); err != nil {
			t.Fatal(err)
		}
		bySlug := map[string]Top10Result{}
		for _, it := range out.Items {
			bySlug[it.Slug] = it
		}
		return bySlug
	}

	rows := get(false)
	show := rows["show"]
	if show.TmdbKind != "tv" || derefStr(show.Overview) != "A show." || derefStr(show.BackdropPath) != "/show-bd.jpg" ||
		show.Year == nil || *show.Year != 2024 || show.Runtime == nil || *show.Runtime != 48 ||
		show.SeasonCount == nil || *show.SeasonCount != 3 || len(show.Genres) != 2 || show.Ratings != nil {
		t.Errorf("show facts = %+v", show.TitleFacts)
	}
	// The chart's country picks the certification.
	if derefStr(show.Certification) != "TV-MA" || derefStr(show.LogoPath) != "/show-logo.png" {
		t.Errorf("show certification/logo = %v / %v", show.Certification, show.LogoPath)
	}
	// A movie on the TV chart: movie namespace, never a season count; no US
	// certification, no logo.
	if sp := rows["special"]; sp.TmdbKind != "movie" || derefStr(sp.Overview) != "A special." || sp.SeasonCount != nil ||
		sp.Certification != nil || sp.LogoPath != nil {
		t.Errorf("special facts = %+v", sp.TitleFacts)
	}
	// Matched without metadata: only the kind. Unmatched: nothing.
	if b := rows["bare"]; b.TmdbKind != "tv" || b.Overview != nil || b.BackdropPath != nil {
		t.Errorf("bare facts = %+v", b.TitleFacts)
	}
	if n := rows["nomatch"]; n.TmdbKind != "" {
		t.Errorf("nomatch facts = %+v", n.TitleFacts)
	}

	rows = get(true)
	r := rows["show"].Ratings
	if r == nil || r.RottenTomatoes == nil || deref64(r.RottenTomatoes.CriticsScore) != 91 || !r.Stale {
		t.Fatalf("show ratings = %+v", r)
	}
	if rows["special"].Ratings != nil {
		t.Errorf("special has no stored ratings, got %+v", rows["special"].Ratings)
	}
}

func TestTitleFactsFillChartResponses(t *testing.T) {
	db, charts, _ := newChartsFixtureDB(t)
	if _, err := db.Exec(`INSERT INTO tmdb_titles (tmdb_kind, tmdb_id, title, overview) VALUES ('movie', '2', 'B', 'Bee.')
		ON CONFLICT (tmdb_kind, tmdb_id) DO UPDATE SET overview = excluded.overview`); err != nil {
		t.Fatal(err)
	}
	facts := NewTitleFactsService(repository.NewMetadataRepository(db), nil)
	ctx := context.Background()

	movers, err := charts.Movers(ctx, MoversQuery{Country: "us", Provider: "netflix"})
	if err != nil {
		t.Fatal(err)
	}
	board, err := charts.Leaderboard(ctx, LeaderboardQuery{Period: "week"})
	if err != nil {
		t.Fatal(err)
	}
	hist, err := charts.History(ctx, HistoryQuery{Country: "us", Provider: "netflix", Category: "movies"})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []FactsHolder{movers, board, hist} {
		if err := facts.Fill(ctx, h, true); err != nil {
			t.Fatal(err)
		}
		seen := false
		for _, tg := range h.FactsTargets() {
			if tg.TmdbID != nil && *tg.TmdbID == "2" {
				seen = true
				if derefStr(tg.Facts.Overview) != "Bee." || tg.Facts.TmdbKind != "movie" {
					t.Errorf("%T: b facts = %+v", h, *tg.Facts)
				}
			}
		}
		if _, ok := h.(*MoversResponse); !ok && !seen {
			t.Errorf("%T: title b missing", h)
		}
	}
}

func TestParseFactsInclude(t *testing.T) {
	for raw, want := range map[string]bool{"": false, "ratings": true, " Ratings ,": true} {
		got, err := ParseFactsInclude(raw)
		if err != nil || got != want {
			t.Errorf("%q: got %v, %v", raw, got, err)
		}
	}
	if _, err := ParseFactsInclude("ratings,timeline"); err == nil {
		t.Error("unknown include should fail")
	}
}

func TestTrendingSeasonCounts(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec(`INSERT INTO tmdb_titles (tmdb_kind, tmdb_id, title, number_of_seasons) VALUES ('tv', '7', 'Seven', 4)`); err != nil {
		t.Fatal(err)
	}
	lister := &listOf{items: []client.TMDBListItem{
		{Kind: "tv", ID: "7", Title: "Seven", Overview: "Story."},
		{Kind: "tv", ID: "8", Title: "Unknown"},
		{Kind: "movie", ID: "7", Title: "Film"},
	}}
	s := NewTitleListsService(lister).WithMetadata(repository.NewMetadataRepository(db))
	out, err := s.Trending(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if c := out.Items[0].SeasonCount; c == nil || *c != 4 || out.Items[0].Overview != "Story." {
		t.Errorf("known show = %+v", out.Items[0])
	}
	if out.Items[1].SeasonCount != nil || out.Items[2].SeasonCount != nil {
		t.Errorf("unknown show / movie got a season count: %+v", out.Items[1:])
	}
	// The cached list itself is never changed.
	if lister.items[0].SeasonCount != nil {
		t.Error("cached item was modified")
	}
}

type listOf struct{ items []client.TMDBListItem }

func (l *listOf) Enabled() bool { return true }
func (l *listOf) Trending(context.Context, string, string) ([]client.TMDBListItem, error) {
	return l.items, nil
}
func (l *listOf) NowPlaying(context.Context, string) ([]client.TMDBListItem, error) {
	return l.items, nil
}

func deref64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func TestOverviewExtraSections(t *testing.T) {
	_, _, overview := newChartsFixtureDB(t)
	ctx := context.Background()
	ref := TmdbRef{Kind: "movie", ID: "1"}

	def, err := ParseIncludes("")
	if err != nil {
		t.Fatal(err)
	}
	out, err := overview.Overview(ctx, ref, OverviewQuery{Include: def})
	if err != nil {
		t.Fatal(err)
	}
	if out.Rankings != nil || out.Timeline != nil || out.Releases != nil {
		t.Fatal("rankings, timeline and releases must be opt-in")
	}

	inc, err := ParseIncludes("rankings")
	if err != nil {
		t.Fatal(err)
	}
	all, err := overview.Overview(ctx, ref, OverviewQuery{Include: inc})
	if err != nil {
		t.Fatal(err)
	}
	in, err := overview.Overview(ctx, ref, OverviewQuery{Include: inc, Countries: []string{"in", "IN"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Rankings) != 3 || len(in.Rankings) != 1 || in.Rankings[0].Country != "IN" {
		t.Fatalf("rankings: all=%d in=%+v", len(all.Rankings), in.Rankings)
	}
	if _, err := overview.Overview(ctx, ref, OverviewQuery{Include: inc, Countries: []string{"US", "IN", "GB", "DE", "FR", "ES"}}); err == nil {
		t.Error("more than 5 countries should fail")
	}
	if _, err := overview.Overview(ctx, ref, OverviewQuery{Include: inc, Countries: []string{"XX"}}); err == nil {
		t.Error("an invalid country should fail")
	}
	if _, err := ParseIncludes("timeline,releases,nope"); err == nil {
		t.Error("unknown include should fail")
	}
}

func TestChartProviderIDsAndLogos(t *testing.T) {
	if ChartProviderOf(1796) != "netflix" || ChartProviderOf(119) != "amazon-prime" || ChartProviderOf(1825) != "" {
		t.Fatalf("ChartProviderOf: %q %q %q", ChartProviderOf(1796), ChartProviderOf(119), ChartProviderOf(1825))
	}
	charts, _ := newChartsFixture(t)
	db := newTestDB(t)
	if _, err := db.Exec(`
		INSERT INTO tmdb_titles (tmdb_kind, tmdb_id, title) VALUES ('movie', '1', 'A');
		INSERT INTO tmdb_watch_providers (tmdb_kind, tmdb_id, country, monetization, provider_id, provider_name, logo_path)
		VALUES ('movie', '1', 'US', 'flatrate', 8, 'Netflix', '/nfx.png'), ('movie', '1', 'US', 'flatrate', 1796, 'Netflix with Ads', '');`); err != nil {
		t.Fatal(err)
	}
	logos := NewProviderLogos(repository.NewMetadataRepository(db))
	got, err := logos.Get(context.Background())
	if err != nil || got[8] != "/nfx.png" || len(got) != 1 {
		t.Fatalf("logos = %v, %v", got, err)
	}
	charts.WithProviderLogos(logos)
	providers, err := charts.Providers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range providers {
		if p.Slug == "netflix" {
			if p.TMDBProviderID != 8 || len(p.TMDBProviderIDs) != 3 || derefStr(p.LogoPath) != "/nfx.png" {
				t.Errorf("netflix provider = %+v", p)
			}
			return
		}
	}
	t.Fatal("netflix missing from providers")
}

func TestListsLogoAndCinemaCertification(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec(`INSERT INTO tmdb_titles (tmdb_kind, tmdb_id, title, logo_path, certifications)
		VALUES ('movie', '7', 'Film', '/film-logo.png', '{"US":"PG"}')`); err != nil {
		t.Fatal(err)
	}
	lister := &listOf{items: []client.TMDBListItem{{Kind: "movie", ID: "7", Title: "Film"}}}
	s := NewTitleListsService(lister).WithMetadata(repository.NewMetadataRepository(db))
	ctx := context.Background()
	cin, err := s.InCinemas(ctx, "us")
	if err != nil {
		t.Fatal(err)
	}
	if it := cin.Items[0]; derefStr(it.LogoPath) != "/film-logo.png" || derefStr(it.Certification) != "PG" {
		t.Errorf("in cinemas item = %+v", it)
	}
	tr, err := s.Trending(ctx, "movie")
	if err != nil {
		t.Fatal(err)
	}
	if it := tr.Items[0]; derefStr(it.LogoPath) != "/film-logo.png" || it.Certification != nil {
		t.Errorf("trending item = %+v (no country, so no certification)", it)
	}
}

func TestSaveDetailsStoresCertificationsAndLogo(t *testing.T) {
	db := newTestDB(t)
	meta := repository.NewMetadataRepository(db)
	ctx := context.Background()
	d := &client.TMDBFullDetails{Kind: "tv", ID: "9", Title: "Nine", Certifications: map[string]string{"US": "TV-14"}, LogoPath: "/nine.png"}
	if err := meta.SaveDetails(ctx, d, false); err != nil {
		t.Fatal(err)
	}
	m, err := meta.Get(ctx, repository.TmdbRef{Kind: "tv", ID: "9"})
	if err != nil || m == nil {
		t.Fatalf("get: %v %v", m, err)
	}
	if m.Certifications["US"] != "TV-14" || derefStr(m.LogoPath) != "/nine.png" {
		t.Errorf("stored = %v / %v", m.Certifications, m.LogoPath)
	}
	// A refresh without them clears both (TMDB dropped them).
	d.Certifications, d.LogoPath = nil, ""
	if err := meta.SaveDetails(ctx, d, false); err != nil {
		t.Fatal(err)
	}
	if m, _ = meta.Get(ctx, repository.TmdbRef{Kind: "tv", ID: "9"}); m.Certifications != nil || m.LogoPath != nil {
		t.Errorf("after refresh = %v / %v", m.Certifications, m.LogoPath)
	}
}
