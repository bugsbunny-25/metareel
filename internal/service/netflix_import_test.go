package service

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestNetflixImportParsesAndMatches(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	flix := repository.NewFlixPatrolRepository(db)
	imports := repository.NewImportsRepository(db)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	n := NewNetflixImporter(log, nil, imports, flix, repository.NewTaskScheduleRepository(db), nil, nil, []string{"US"})

	// A FlixPatrol title that charted on Netflix gives the match.
	seed(t, flix, []seedTitle{{Slug: "unabomber", Name: "UNABOMBER", Kind: "movie", TmdbID: "999"}},
		[]seedRanking{{"unabomber", "2026-10-01", "US", "netflix", "movies", 1}})

	global := "week\tcategory\tweekly_rank\tshow_title\tseason_title\tweekly_hours_viewed\truntime\tweekly_views\tcumulative_weeks_in_top_10\n" +
		"2026-10-04\tFilms (English)\t1\tUNABOMBER\tN/A\t41500000\t1.6667\t24900000\t2\n" +
		"2026-10-04\tTV (English)\t1\tWednesday\tWednesday: Season 2\t1000\t\t\t3\n" +
		"bad-week\tTV (English)\t2\tX\tN/A\t1\t1\t1\t1\n"
	rows, skipped, err := n.importGlobal(ctx, strings.NewReader(global))
	if err != nil || rows != 2 || skipped != 1 {
		t.Fatalf("global rows=%d skipped=%d err=%v", rows, skipped, err)
	}
	countries := "country_name\tcountry_iso2\tweek\tcategory\tweekly_rank\tshow_title\tseason_title\tcumulative_weeks_in_top_10\n" +
		"United States\tUS\t2026-10-04\tFilms\t1\tUNABOMBER\tN/A\t2\n" +
		"Argentina\tAR\t2026-10-04\tFilms\t1\tUNABOMBER\tN/A\t2\n"
	rows, _, err = n.importCountries(ctx, strings.NewReader(countries), map[string]bool{"US": true})
	if err != nil || rows != 1 {
		t.Fatalf("country rows=%d err=%v (AR must be filtered out)", rows, err)
	}
	if _, _, err := n.importGlobal(ctx, strings.NewReader("week\tshow_title\n")); err == nil {
		t.Fatal("missing columns should fail")
	}

	rep := &ImportReport{}
	if err := n.matchTitles(ctx, rep, func(string, string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	if rep.Matched != 1 || rep.Unmatched != 1 {
		t.Fatalf("matched=%d unmatched=%d", rep.Matched, rep.Unmatched)
	}
	var tmdb, source string
	if err := db.QueryRow(`SELECT tmdb_id, match_source FROM netflix_titles WHERE show_title = 'UNABOMBER'`).Scan(&tmdb, &source); err != nil {
		t.Fatal(err)
	}
	if tmdb != "999" || source != "flixpatrol_title" {
		t.Fatalf("netflix match = %s via %s", tmdb, source)
	}
}

func TestPickRecentRelease(t *testing.T) {
	hits := []titleCandidate{{Title: "The Breadwinner", Year: 2026, TmdbID: "1"}, {Title: "The Breadwinner", Year: 2017, TmdbID: "2"}}
	if c, ok := pickRecentRelease(hits, 2026); !ok || c.TmdbID != "1" || c.Source != "justwatch_recent_release" {
		t.Fatalf("got %+v %v", c, ok)
	}
	if _, ok := pickRecentRelease(hits, 2022); ok {
		t.Fatal("no recent release should not match")
	}
	twin := []titleCandidate{{Year: 2025, TmdbID: "1"}, {Year: 2026, TmdbID: "2"}}
	if _, ok := pickRecentRelease(twin, 2026); ok {
		t.Fatal("two recent releases are ambiguous")
	}
}
