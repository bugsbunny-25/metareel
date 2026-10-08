package service

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// newTestTop10Service returns a Top10ReadService backed by a migrated,
// file-based SQLite database, plus the repository for seeding.
func newTestTop10Service(t *testing.T) (*Top10ReadService, *repository.FlixPatrolRepository) {
	t.Helper()
	repo := repository.NewFlixPatrolRepository(newTestDB(t))
	return NewTop10ReadService(repo), repo
}

// newTestDB returns a migrated, file-based SQLite database.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "test.db") + "?_pragma=foreign_keys(1)"
	db, err := repository.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := repository.Migrate(db, "../../db/migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func day(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

type seedRanking struct {
	slug     string
	date     string
	country  string
	provider string
	category string
	rank     int64
}

func seed(t *testing.T, repo *repository.FlixPatrolRepository, titles []repository.UpsertTitleInput, rankings []seedRanking) {
	t.Helper()
	ctx := context.Background()
	ids := map[string]int64{}
	for _, in := range titles {
		row, err := repo.UpsertTitle(ctx, in)
		if err != nil {
			t.Fatalf("upsert title %s: %v", in.Slug, err)
		}
		ids[in.Slug] = row.ID
	}
	for _, r := range rankings {
		if _, err := repo.UpsertRanking(ctx, repository.UpsertRankingInput{
			TitleID:           ids[r.slug],
			RankedOn:          day(r.date),
			Country:           r.country,
			StreamingProvider: r.provider,
			Category:          r.category,
			Rank:              r.rank,
		}); err != nil {
			t.Fatalf("upsert ranking %+v: %v", r, err)
		}
	}
}

func TestTop10_PreviousRankAndDaysInTop10(t *testing.T) {
	svc, repo := newTestTop10Service(t)
	seed(t, repo,
		[]repository.UpsertTitleInput{
			{Slug: "a", Name: "A", Kind: "movie"},
			{Slug: "b", Name: "B", Kind: "movie"},
			{Slug: "c", Name: "C", Kind: "movie"},
		},
		[]seedRanking{
			{"a", "2026-10-01", "US", "netflix", "movies", 1},
			{"b", "2026-10-01", "US", "netflix", "movies", 2},
			// 2026-10-02 was never scraped.
			{"b", "2026-10-03", "US", "netflix", "movies", 1},
			{"a", "2026-10-03", "US", "netflix", "movies", 3},
			{"c", "2026-10-03", "US", "netflix", "movies", 2},
			// Another chart must not leak into the netflix one.
			{"c", "2026-10-01", "US", "hbo-max", "movies", 1},
		})

	tests := []struct {
		name     string
		date     time.Time
		wantDate string
	}{
		{name: "explicit date", date: day("2026-10-03"), wantDate: "2026-10-03"},
		{name: "omitted date means latest", date: time.Time{}, wantDate: "2026-10-03"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := svc.GetMoviesByProvider(context.Background(), Top10Query{Date: tt.date, CountryCode: "US", Provider: "netflix"})
			if err != nil {
				t.Fatalf("GetMoviesByProvider: %v", err)
			}
			if out.Date != tt.wantDate {
				t.Fatalf("date = %s, want %s", out.Date, tt.wantDate)
			}
			want := map[string]struct {
				rank, days int64
				prev       *int64
			}{
				"b": {rank: 1, prev: ptr(2), days: 2}, // previous scraped date is 10-01, despite the gap
				"c": {rank: 2, prev: nil, days: 1},    // new on netflix; its hbo-max rank must not count
				"a": {rank: 3, prev: ptr(1), days: 2},
			}
			if len(out.Items) != len(want) {
				t.Fatalf("got %d items, want %d", len(out.Items), len(want))
			}
			for _, it := range out.Items {
				w := want[it.Slug]
				if it.Rank != w.rank || it.DaysInTop10 != w.days || !equalPtr(it.PreviousRank, w.prev) || it.Date != "2026-10-03" {
					t.Errorf("%s: rank=%d prev=%v days=%d date=%s, want rank=%d prev=%v days=%d", it.Slug, it.Rank, deref(it.PreviousRank), it.DaysInTop10, it.Date, w.rank, deref(w.prev), w.days)
				}
			}
		})
	}
}

func TestTop10_AllProvidersLatestIsPerProvider(t *testing.T) {
	svc, repo := newTestTop10Service(t)
	seed(t, repo,
		[]repository.UpsertTitleInput{{Slug: "a", Name: "A", Kind: "tv_show"}},
		[]seedRanking{
			{"a", "2026-10-02", "US", "netflix", "tv_shows", 1},
			{"a", "2026-10-03", "US", "netflix", "tv_shows", 1},
			// hbo-max hasn't been scraped for 10-03 yet.
			{"a", "2026-10-02", "US", "hbo-max", "tv_shows", 4},
		})

	out, err := svc.GetTVShowsAllProviders(context.Background(), Top10Query{CountryCode: "US"})
	if err != nil {
		t.Fatalf("GetTVShowsAllProviders: %v", err)
	}
	if out.Date != "2026-10-03" {
		t.Fatalf("date = %s, want newest item date 2026-10-03", out.Date)
	}
	got := map[string]string{}
	for _, it := range out.Items {
		got[it.Provider] = it.Date
	}
	if got["netflix"] != "2026-10-03" || got["hbo-max"] != "2026-10-02" {
		t.Fatalf("per-provider dates = %v", got)
	}
}

func TestTop10_NotFound(t *testing.T) {
	svc, _ := newTestTop10Service(t)
	_, err := svc.GetMoviesByProvider(context.Background(), Top10Query{CountryCode: "US", Provider: "netflix"})
	if !errors.Is(err, ErrTop10NotFound) {
		t.Fatalf("err = %v, want ErrTop10NotFound", err)
	}
}

func TestTitleRankings(t *testing.T) {
	svc, repo := newTestTop10Service(t)
	seed(t, repo,
		[]repository.UpsertTitleInput{
			// Two FlixPatrol slugs mapped to the same TMDB movie.
			{Slug: "ice-age", Name: "Ice Age", Kind: "movie", TmdbID: "425", ImdbID: "tt0268380"},
			{Slug: "ice-age-2002", Name: "Ice Age", Kind: "movie", TmdbID: "425"},
			// Same numeric ID in the TV namespace is a different title.
			{Slug: "some-show", Name: "Some Show", Kind: "tv_show", TmdbID: "425"},
			// A special charting as TV but mapped as a movie.
			{Slug: "special", Name: "Special", Kind: "movie", TmdbID: "999"},
		},
		[]seedRanking{
			{"ice-age", "2026-10-01", "US", "netflix", "movies", 5},
			{"ice-age", "2026-10-02", "US", "netflix", "movies", 3},
			{"ice-age-2002", "2026-10-03", "US", "netflix", "movies", 4},
			{"ice-age", "2026-10-02", "IN", "netflix", "movies", 7},
			{"some-show", "2026-10-02", "US", "netflix", "tv_shows", 1},
			{"special", "2026-10-02", "US", "netflix", "tv_shows", 2},
		})
	ctx := context.Background()

	t.Run("single", func(t *testing.T) {
		out, err := svc.GetTitleRankings(ctx, TmdbRef{Kind: "movie", ID: "425"}, RankingHistoryQuery{})
		if err != nil {
			t.Fatalf("GetTitleRankings: %v", err)
		}
		if len(out.Titles) != 2 || len(out.Rankings) != 4 {
			t.Fatalf("titles=%d rankings=%d, want 2 and 4", len(out.Titles), len(out.Rankings))
		}
		if len(out.Charts) != 2 {
			t.Fatalf("charts = %+v, want IN and US netflix movies", out.Charts)
		}
		us := out.Charts[1]
		if us.Country != "US" || us.DaysInTop10 != 3 || us.BestRank != 3 || us.FirstDate != "2026-10-01" || us.LastDate != "2026-10-03" || us.LastRank != 4 {
			t.Fatalf("US summary = %+v", us)
		}
	})

	t.Run("filters", func(t *testing.T) {
		from, to := day("2026-10-02"), day("2026-10-02")
		out, err := svc.GetTitleRankings(ctx, TmdbRef{Kind: "movie", ID: "425"}, RankingHistoryQuery{CountryCode: "us", From: &from, To: &to})
		if err != nil {
			t.Fatalf("GetTitleRankings: %v", err)
		}
		if len(out.Rankings) != 1 || out.Rankings[0].Rank != 3 || out.Rankings[0].Country != "US" {
			t.Fatalf("rankings = %+v", out.Rankings)
		}
	})

	t.Run("kind separates namespaces", func(t *testing.T) {
		out, err := svc.GetTitleRankings(ctx, TmdbRef{Kind: "tv", ID: "425"}, RankingHistoryQuery{})
		if err != nil {
			t.Fatalf("GetTitleRankings: %v", err)
		}
		if len(out.Titles) != 1 || out.Titles[0].Slug != "some-show" || len(out.Rankings) != 1 {
			t.Fatalf("got %+v", out)
		}
	})

	t.Run("special charting as tv is found as movie", func(t *testing.T) {
		out, err := svc.GetTitleRankings(ctx, TmdbRef{Kind: "movie", ID: "999"}, RankingHistoryQuery{})
		if err != nil {
			t.Fatalf("GetTitleRankings: %v", err)
		}
		if len(out.Rankings) != 1 || out.Rankings[0].Category != "tv_shows" {
			t.Fatalf("rankings = %+v", out.Rankings)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := svc.GetTitleRankings(ctx, TmdbRef{Kind: "tv", ID: "999"}, RankingHistoryQuery{})
		if !errors.Is(err, ErrTitleNotFound) {
			t.Fatalf("err = %v, want ErrTitleNotFound", err)
		}
	})

	t.Run("batch", func(t *testing.T) {
		refs, err := ParseTmdbRefList("tv:425, movie:425,movie:1,movie:425")
		if err != nil {
			t.Fatalf("ParseTmdbRefList: %v", err)
		}
		out, err := svc.GetTitleRankingsBatch(ctx, refs, RankingHistoryQuery{})
		if err != nil {
			t.Fatalf("GetTitleRankingsBatch: %v", err)
		}
		if len(out.Items) != 2 || out.Items[0].Kind != "tv" || out.Items[1].Kind != "movie" {
			t.Fatalf("items = %+v", out.Items)
		}
		if len(out.NotFound) != 1 || out.NotFound[0] != "movie:1" {
			t.Fatalf("not_found = %v", out.NotFound)
		}
	})

	t.Run("validation", func(t *testing.T) {
		from, to := day("2026-10-03"), day("2026-10-01")
		for _, q := range []RankingHistoryQuery{{CountryCode: "XX"}, {From: &from, To: &to}} {
			var vErr *ValidationError
			if _, err := svc.GetTitleRankings(ctx, TmdbRef{Kind: "movie", ID: "425"}, q); !errors.As(err, &vErr) {
				t.Errorf("query %+v: err = %v, want ValidationError", q, err)
			}
		}
		for _, raw := range []string{"425", "film:425", "movie:abc"} {
			var vErr *ValidationError
			if _, err := ParseTmdbRefList(raw); !errors.As(err, &vErr) {
				t.Errorf("ParseTmdbRefList(%q): err = %v, want ValidationError", raw, err)
			}
		}
	})
}

func ptr(v int64) *int64 { return &v }

func equalPtr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func deref(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}
