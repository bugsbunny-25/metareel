package service

import (
	"context"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestDecayAndSimilarity(t *testing.T) {
	db := newTestDB(t)
	flix := repository.NewFlixPatrolRepository(db)
	seed(t, flix,
		[]seedTitle{{Slug: "a", Name: "A", Kind: "movie", TmdbID: "1"}, {Slug: "b", Name: "B", Kind: "movie", TmdbID: "2"}},
		[]seedRanking{
			{"b", "2026-09-25", "US", "netflix", "movies", 1}, // the chart's first stored date: no debuts counted
			{"a", "2026-10-01", "US", "netflix", "movies", 1},
			{"a", "2026-10-02", "US", "netflix", "movies", 3},
			{"b", "2026-10-03", "US", "netflix", "movies", 1}, // a dropped off on day 2
			{"a", "2026-10-01", "GB", "netflix", "movies", 2},
			{"b", "2026-10-01", "GB", "netflix", "movies", 1},
		})
	svc := NewAnalyticsService(repository.NewAnalyticsRepository(db), repository.NewMetadataRepository(db), repository.NewImportsRepository(db), repository.NewRatingsRepository(db))
	now := func() time.Time { return time.Date(2026, 10, 3, 13, 0, 0, 0, time.UTC) }
	svc.now, svc.charts.now = now, now
	ctx := context.Background()
	from, to := day("2026-10-01"), day("2026-10-03")

	d, err := svc.Decay(ctx, AnalyticsQuery{From: &from, To: &to, Country: "US"})
	if err != nil {
		t.Fatal(err)
	}
	// US debuts in range: a (10-01). b charted on 09-25, so 10-03 is not a debut.
	if d.Debuts != 1 || d.Curve[0].Titles != 1 || d.Curve[0].Charting != 1 || *d.Curve[0].AvgRank != 1 {
		t.Fatalf("day 0 = %+v (debuts %d)", d.Curve[0], d.Debuts)
	}
	if d.Curve[2].Titles != 1 || d.Curve[2].Charting != 0 || d.Curve[2].Retention != 0 {
		t.Fatalf("day 2 = %+v", d.Curve[2])
	}

	sim, err := svc.CountrySimilarity(ctx, AnalyticsQuery{From: &from, To: &to})
	if err != nil {
		t.Fatal(err)
	}
	if len(sim.Pairs) != 1 || sim.Pairs[0].Jaccard != 1 || sim.Pairs[0].Shared != 2 {
		t.Fatalf("pairs = %+v", sim.Pairs)
	}

	if r := pearson([]float64{1, 2, 3}, []float64{2, 4, 6}); r == nil || *r != 1 {
		t.Fatalf("pearson = %v", r)
	}
	if pearson([]float64{1, 1, 1}, []float64{1, 2, 3}) != nil {
		t.Fatal("no variance should be nil")
	}
}
