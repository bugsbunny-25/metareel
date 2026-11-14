package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// posterFixture seeds one TV chart covering every poster case:
//   - "show":    TV title with a poster (tmdb_kind 'tv' must be derived from kind tv_show)
//   - "special": a movie on the TV chart with a poster (looked up in the movie namespace)
//   - "blank":   matched TV title whose metadata has an empty poster_path
//   - "bare":    matched TV title with no tmdb_titles row at all
//   - "nomatch": unmatched title (no tmdb_id)
//
// A decoy tv/100 row with another poster makes a wrong-namespace lookup
// for "special" (movie/100) visible.
func posterFixture(t *testing.T) *sql.DB {
	t.Helper()
	db := newTestDB(t)
	seed(t, db,
		[]seedTitle{
			{Slug: "show", Name: "Show", Kind: "tv_show", TmdbID: "200"},
			{Slug: "special", Name: "Special", Kind: "movie", TmdbID: "100"},
			{Slug: "blank", Name: "Blank", Kind: "tv_show", TmdbID: "300"},
			{Slug: "bare", Name: "Bare", Kind: "tv_show", TmdbID: "400"},
			{Slug: "nomatch", Name: "No Match", Kind: "tv_show"},
		},
		[]seedRanking{
			{"show", "2026-10-02", "US", "netflix", "tv_shows", 1},
			{"special", "2026-10-02", "US", "netflix", "tv_shows", 2},
			{"blank", "2026-10-02", "US", "netflix", "tv_shows", 3},
			{"bare", "2026-10-02", "US", "netflix", "tv_shows", 4},
			{"nomatch", "2026-10-02", "US", "netflix", "tv_shows", 5},
			{"show", "2026-10-03", "US", "netflix", "tv_shows", 2},
			{"special", "2026-10-03", "US", "netflix", "tv_shows", 1},
			{"blank", "2026-10-03", "US", "netflix", "tv_shows", 3},
			{"bare", "2026-10-03", "US", "netflix", "tv_shows", 4},
			{"nomatch", "2026-10-03", "US", "netflix", "tv_shows", 5},
		})
	// Matching may already have created bare rows; upsert only the poster.
	if _, err := db.Exec(`
		INSERT INTO tmdb_titles (tmdb_kind, tmdb_id, title, poster_path) VALUES
		  ('tv', '200', 'Show', '/show.jpg'),
		  ('movie', '100', 'Special', '/special.jpg'),
		  ('tv', '100', 'Decoy', '/decoy.jpg'),
		  ('tv', '300', 'Blank', '')
		ON CONFLICT (tmdb_kind, tmdb_id) DO UPDATE SET poster_path = excluded.poster_path`); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM tmdb_titles WHERE tmdb_kind = 'tv' AND tmdb_id = '400'`); err != nil {
		t.Fatalf("drop bare metadata: %v", err)
	}
	return db
}

var wantPosters = map[string]string{"show": "/show.jpg", "special": "/special.jpg", "blank": "", "bare": "", "nomatch": ""}

func checkPoster(t *testing.T, where, slug string, got *string) {
	t.Helper()
	want, ok := wantPosters[slug]
	if !ok {
		t.Fatalf("%s: unexpected slug %q", where, slug)
	}
	gotS := "<none>"
	if got != nil {
		gotS = *got
	}
	if (want == "" && got != nil) || (want != "" && gotS != want) {
		t.Errorf("%s %s: poster_path = %q, want %q", where, slug, gotS, map[bool]string{true: "<none>", false: want}[want == ""])
	}
}

func TestTop10_PosterPath(t *testing.T) {
	db := posterFixture(t)
	top := NewTop10ReadService(repository.NewFlixPatrolRepository(db)).WithMetadata(repository.NewMetadataRepository(db))
	ctx := context.Background()

	for name, get := range map[string]func(context.Context, Top10Query) (*Top10Response, error){
		"by provider":   top.GetTVShowsByProvider,
		"all providers": top.GetTVShowsAllProviders,
	} {
		out, err := get(ctx, Top10Query{CountryCode: "US", Provider: "netflix"})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(out.Items) != 5 {
			t.Fatalf("%s: %d items, want 5", name, len(out.Items))
		}
		for _, it := range out.Items {
			checkPoster(t, "top10 "+name, it.Slug, it.PosterPath)
		}
	}

	// Without metadata wired in, no poster (and no error).
	plain := NewTop10ReadService(repository.NewFlixPatrolRepository(db))
	out, err := plain.GetTVShowsByProvider(ctx, Top10Query{CountryCode: "US", Provider: "netflix"})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range out.Items {
		if it.PosterPath != nil {
			t.Errorf("no metadata service: %s poster_path = %q", it.Slug, *it.PosterPath)
		}
	}
}

func TestChartTitle_PosterPath(t *testing.T) {
	db := posterFixture(t)
	charts := NewChartsService(repository.NewAnalyticsRepository(db))
	charts.now = func() time.Time { return time.Date(2026, 10, 3, 13, 0, 0, 0, time.UTC) }
	ctx := context.Background()

	h, err := charts.History(ctx, HistoryQuery{Country: "US", Provider: "netflix", Category: "tv_shows"})
	if err != nil || len(h.Days) != 2 {
		t.Fatalf("history = %+v err=%v", h, err)
	}
	for _, d := range h.Days {
		for _, it := range d.Items {
			checkPoster(t, "history "+d.Date, it.Slug, it.PosterPath)
		}
	}

	m, err := charts.Movers(ctx, MoversQuery{Country: "US", Provider: "netflix"})
	if err != nil || len(m.Charts) != 1 {
		t.Fatalf("movers = %+v err=%v", m, err)
	}
	// special climbed 2 → 1, show fell 1 → 2: both carry their poster.
	mc := m.Charts[0]
	if len(mc.Climbers) != 1 || len(mc.Fallers) != 1 {
		t.Fatalf("climbers=%+v fallers=%+v", mc.Climbers, mc.Fallers)
	}
	checkPoster(t, "movers climber", mc.Climbers[0].Slug, mc.Climbers[0].PosterPath)
	checkPoster(t, "movers faller", mc.Fallers[0].Slug, mc.Fallers[0].PosterPath)
}
