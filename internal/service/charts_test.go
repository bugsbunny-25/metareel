package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

func newChartsFixture(t *testing.T) (*ChartsService, *TitleOverviewService) {
	t.Helper()
	db := newTestDB(t)
	flix := repository.NewFlixPatrolRepository(db)
	seed(t, flix,
		[]seedTitle{
			{Slug: "a", Name: "A", Kind: "movie", TmdbID: "1", ImdbID: "tt0000001"},
			{Slug: "a-2026", Name: "A", Kind: "movie", TmdbID: "1"}, // same TMDB title, other slug
			{Slug: "b", Name: "B", Kind: "movie", TmdbID: "2"},
			{Slug: "c", Name: "C", Kind: "movie"},
			{Slug: "d", Name: "D", Kind: "movie", TmdbID: "4"},
		},
		[]seedRanking{
			{"a", "2026-10-01", "US", "netflix", "movies", 1},
			{"b", "2026-10-01", "US", "netflix", "movies", 2},
			{"d", "2026-10-01", "US", "netflix", "movies", 3},
			{"b", "2026-10-02", "US", "netflix", "movies", 1},
			{"a", "2026-10-02", "US", "netflix", "movies", 2},
			{"c", "2026-10-02", "US", "netflix", "movies", 3},
			{"b", "2026-10-03", "US", "netflix", "movies", 1},
			{"d", "2026-10-03", "US", "netflix", "movies", 2},
			{"c", "2026-10-03", "US", "netflix", "movies", 3},
			{"a-2026", "2026-10-03", "IN", "netflix", "movies", 1},
		})
	analytics := repository.NewAnalyticsRepository(db)
	charts := NewChartsService(analytics)
	charts.now = func() time.Time { return time.Date(2026, 10, 3, 13, 0, 0, 0, time.UTC) }
	overview := NewTitleOverviewService(analytics, flix, repository.NewMetadataRepository(db), nil)
	return charts, overview
}

func TestMovers(t *testing.T) {
	charts, _ := newChartsFixture(t)
	out, err := charts.Movers(context.Background(), MoversQuery{Country: "us", Provider: "netflix"})
	if err != nil {
		t.Fatal(err)
	}
	m := out.Charts[0]
	if m.Date != "2026-10-03" || m.PreviousDate == nil || *m.PreviousDate != "2026-10-02" {
		t.Fatalf("dates = %s / %v", m.Date, m.PreviousDate)
	}
	names := func(ms []Mover) string {
		var s []string
		for _, x := range ms {
			s = append(s, x.Slug)
		}
		return strings.Join(s, ",")
	}
	// b stayed #1, c stayed #3; d re-entered (charted 10-01); a exited.
	if names(m.Climbers) != "" || names(m.Fallers) != "" || names(m.ReEntries) != "d" || names(m.Debuts) != "" || names(m.Exits) != "a" {
		t.Fatalf("climbers=%s fallers=%s re=%s debuts=%s exits=%s", names(m.Climbers), names(m.Fallers), names(m.ReEntries), names(m.Debuts), names(m.Exits))
	}
	if _, err := charts.Movers(context.Background(), MoversQuery{Country: "US"}); err == nil {
		t.Fatal("provider is required")
	}
}

func TestLeaderboardMergesTmdbTitles(t *testing.T) {
	charts, _ := newChartsFixture(t)
	out, err := charts.Leaderboard(context.Background(), LeaderboardQuery{Period: "week"})
	if err != nil {
		t.Fatal(err)
	}
	if out.From != "2026-09-27" || out.To != "2026-10-03" {
		t.Fatalf("range = %s..%s", out.From, out.To)
	}
	top := out.Items[0]
	// b: 9+10+10 = 29; a (both slugs): 10+9+10 = 29, best rank 1 too → order by points then rank, stable.
	if top.Points != 29 || len(out.Items) != 4 {
		t.Fatalf("items = %+v", out.Items)
	}
	for _, it := range out.Items {
		if it.TmdbID != nil && *it.TmdbID == "1" {
			if len(it.Slugs) != 2 || strings.Join(it.Countries, ",") != "IN,US" || it.DaysOnCharts != 3 {
				t.Fatalf("merged entry = %+v", it)
			}
		}
	}
	day, err := charts.Leaderboard(context.Background(), LeaderboardQuery{Period: "day", Country: "US"})
	if err != nil || len(day.Items) != 3 || day.Items[0].Slug != "b" {
		t.Fatalf("day = %+v err=%v", day, err)
	}
	if _, err := charts.Leaderboard(context.Background(), LeaderboardQuery{Period: "year"}); err == nil {
		t.Fatal("bad period accepted")
	}
}

func TestHistoryAndCatalog(t *testing.T) {
	charts, _ := newChartsFixture(t)
	ctx := context.Background()
	h, err := charts.History(ctx, HistoryQuery{Country: "US", Provider: "netflix", Category: "movies"})
	if err != nil || len(h.Days) != 3 || len(h.Days[2].Items) != 3 || h.Days[2].Items[0].Slug != "b" {
		t.Fatalf("history = %+v err=%v", h, err)
	}
	cat, err := charts.Catalog(ctx, "", "")
	if err != nil || len(cat.Items) != 2 {
		t.Fatalf("catalog = %+v err=%v", cat, err)
	}
	for _, c := range cat.Items {
		if c.Country == "US" && (c.Dates != 3 || c.LatestDate != "2026-10-03" || c.Stale) {
			t.Fatalf("US chart = %+v", c)
		}
	}
	var sb strings.Builder
	if err := charts.Export(ctx, ExportQuery{Country: "US", Format: "csv"}, &sb); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(sb.String(), "\n"); lines != 10 { // header + 9 rows
		t.Fatalf("csv lines = %d\n%s", lines, sb.String())
	}
}

func TestTitleStatsAndLookup(t *testing.T) {
	_, overview := newChartsFixture(t)
	ctx := context.Background()
	st, err := overview.Stats(ctx, TmdbRef{Kind: "movie", ID: "1"}, RankingHistoryQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if st.TotalPoints != 29 || *st.PeakRank != 1 || st.DaysAtNumber1 != 2 || st.Debut.Date != "2026-10-01" || st.LongestStreak != 2 {
		t.Fatalf("stats = %+v", st.TitleStats)
	}
	if len(st.Spread) != 2 || st.Spread[1].Country != "IN" || st.Spread[1].DaysAfter != 2 {
		t.Fatalf("spread = %+v", st.Spread)
	}
	res, err := overview.Lookup(ctx, "tt0000001", "", "")
	if err != nil || len(res.Refs) != 1 || res.Refs[0].TmdbID != "1" {
		t.Fatalf("lookup = %+v err=%v", res, err)
	}
	if _, err := overview.Lookup(ctx, "", "", "nope"); err != ErrTitleNotFound {
		t.Fatalf("unknown slug err = %v", err)
	}
	if _, err := overview.Lookup(ctx, "tt1", "movie:1", ""); err == nil {
		t.Fatal("two ids accepted")
	}
}
