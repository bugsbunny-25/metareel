package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

func reasonCodes(it ReviewItem) string {
	var codes []string
	for _, r := range it.Reasons {
		codes = append(codes, r.Code)
	}
	return strings.Join(codes, ",")
}

func TestScoreMapping(t *testing.T) {
	good := repository.ReviewCandidate{Subject: "flixpatrol", Name: "The Boys: Season 5", Kind: "tv_show", TmdbKind: "tv", TmdbID: "76479",
		MatchSource: "justwatch_path", TmdbTitle: "The Boys", TmdbReleaseDate: "2019-07-25", HasDetails: true,
		FirstCharted: "2026-04-20", Providers: []string{"amazon-prime"},
		LastCharted: map[string]string{"amazon-prime": "2026-04-28"}, DetailsFetchedAt: "2026-05-02 04:00:00"}
	onPrime := map[int64]bool{9: true, 119: true}
	if it := scoreMapping(good, onPrime); it.Risk != 0 || len(it.Reasons) != 0 {
		t.Fatalf("good mapping: %+v", it)
	}

	// A sitcom of the same name charting on Prime Video India (a real
	// mis-match): not on Prime anywhere, the name agrees.
	wrong := good
	wrong.Name, wrong.TmdbTitle, wrong.TmdbReleaseDate, wrong.MatchSource = "The Family Man", "The Family Man", "1990-09-21", "justwatch_search"
	it := scoreMapping(wrong, map[int64]bool{8: true})
	if reasonCodes(it) != ReviewNotOnProvider || it.Risk != 45 {
		t.Fatalf("not on provider: %+v", it)
	}
	// Charted there long before the offers were fetched: it may have left.
	stale := wrong
	stale.DetailsFetchedAt = "2026-11-06 04:00:00"
	if it := scoreMapping(stale, map[int64]bool{8: true}); len(it.Reasons) != 0 {
		t.Fatalf("stale chart flagged: %+v", it)
	}

	// Weak source, released after it charted, a different name, another
	// IMDb ID: capped at 100.
	bad := repository.ReviewCandidate{Subject: "netflix", Name: "Spectre", Kind: "movie", TmdbKind: "movie", TmdbID: "1",
		MatchSource: "justwatch_unique_name", TmdbTitle: "Specter of the Past", TmdbReleaseDate: "2027-01-01", HasDetails: true,
		TmdbIMDbID: "tt1", EvidenceIMDbID: "tt2", FirstCharted: "2026-03-01", Providers: []string{"netflix"},
		LastCharted: map[string]string{"netflix": "2026-03-01"}, DetailsFetchedAt: "2026-03-02 01:00:00"}
	it = scoreMapping(bad, map[int64]bool{8: true})
	if got := reasonCodes(it); got != "weak_source,released_later,name_differs,imdb_mismatch" || it.Risk != 100 {
		t.Fatalf("bad mapping: %s risk %d", got, it.Risk)
	}

	// Films: FlixPatrol's premiere year far from TMDB's; no offers at all.
	film := repository.ReviewCandidate{Subject: "flixpatrol", Name: "Retribution", Kind: "movie", TmdbKind: "movie", TmdbID: "2",
		MatchSource: "tmdb_search", TmdbTitle: "Retribution", TmdbReleaseDate: "2006-06-01", HasDetails: true,
		FPPremiereDate: "2023-08-23", FirstCharted: "2026-01-01", Providers: []string{"netflix"}}
	if got := reasonCodes(scoreMapping(film, nil)); got != "weak_source,no_offers,year_mismatch" {
		t.Fatalf("film: %s", got)
	}

	// No details yet: only that (and the source) can be said.
	film.HasDetails = false
	if got := reasonCodes(scoreMapping(film, nil)); got != "weak_source,no_details" {
		t.Fatalf("no details: %s", got)
	}

	// A provider without known offer IDs is not checked.
	other := good
	other.Providers = []string{"zee5"}
	if it := scoreMapping(other, map[int64]bool{232: true}); it.Risk != 0 {
		t.Fatalf("unknown provider: %+v", it)
	}
}

func TestNamesAgree(t *testing.T) {
	for _, c := range []struct {
		chart, tmdb string
		want        bool
	}{
		{"Wednesday: Season 2", "Wednesday", true},
		{"Stranger Things 5", "Stranger Things", true},
		{"Upstairs, Downstairs", "Upstairs Downstairs", true},
		{"Fallout", "Fallout", true},
		{"Murder Suspect", "Murder: Suspect No.1", true}, // containment is accepted
		{"The Witches", "Hocus Pocus", false},
		{"Spectre", "Specter", false},
		{"Bridgerton - Part 1", "Bridgerton", true},
	} {
		if got := namesAgree(c.chart, c.tmdb); got != c.want {
			t.Errorf("namesAgree(%q, %q) = %v", c.chart, c.tmdb, got)
		}
	}
}

func TestMappingReviewQueue(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	d1 := time.Now().UTC().AddDate(0, 0, -10).Format("2006-01-02")
	d2 := time.Now().UTC().AddDate(0, 0, -9).Format("2006-01-02")
	seed(t, db,
		[]seedTitle{
			{Slug: "the-boys", Name: "The Boys", Kind: "tv_show", TmdbID: "76479"},
			{Slug: "family-man", Name: "The Family Man", Kind: "tv_show", TmdbID: "1111"},
		},
		[]seedRanking{
			{"the-boys", d1, "US", "amazon-prime", "tv_shows", 1},
			{"family-man", d1, "IN", "amazon-prime", "tv_shows", 2},
			{"family-man", d2, "IN", "amazon-prime", "tv_shows", 1},
		})
	meta := repository.NewMetadataRepository(db)
	for _, d := range []client.TMDBFullDetails{
		{Kind: "tv", ID: "76479", Title: "The Boys", ReleaseDate: "2019-07-25",
			WatchProviders: []client.TMDBWatchProvider{{Country: "US", Monetization: "flatrate", ProviderID: 9, ProviderName: "Prime Video"}}},
		{Kind: "tv", ID: "1111", Title: "The Family Man", ReleaseDate: "1990-09-21",
			WatchProviders: []client.TMDBWatchProvider{{Country: "US", Monetization: "buy", ProviderID: 2, ProviderName: "Apple TV"}}},
	} {
		if err := meta.SaveDetails(ctx, &d, false); err != nil {
			t.Fatal(err)
		}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewTitleMappingService(log, repository.NewMappingRepository(db), testWriter(db), meta, nil, nil)

	got, err := svc.Review(ctx, MappingReviewQuery{}, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	// seed's matches have source "test" (unknown: 15), so The Boys scores 15
	// and stays below the default threshold.
	if got.Checked != 2 || got.Total != 1 || got.Items[0].Name != "The Family Man" || got.Items[0].Points != 19 {
		t.Fatalf("queue = %+v", got)
	}
	if got.Reasons[ReviewNotOnProvider] != 1 || got.Items[0].Risk != 50 || got.Items[0].FirstSeen != d1 {
		t.Fatalf("family man = %+v reasons %v", got.Items[0], got.Reasons)
	}

	all, err := svc.Review(ctx, MappingReviewQuery{MinRisk: 0, MinGiven: true}, 50, 0)
	if err != nil || all.Total != 2 {
		t.Fatalf("min_risk 0: %+v, %v", all, err)
	}
	if page, _ := svc.Review(ctx, MappingReviewQuery{MinRisk: 0, MinGiven: true}, 1, 1); len(page.Items) != 1 || page.Items[0].Name != "The Boys" {
		t.Fatalf("second page = %+v", page)
	}
	if none, _ := svc.Review(ctx, MappingReviewQuery{Reason: ReviewIMDbMismatch}, 50, 0); none.Total != 0 {
		t.Fatalf("reason filter = %+v", none)
	}
	if nf, _ := svc.Review(ctx, MappingReviewQuery{Subject: "netflix"}, 50, 0); nf.Checked != 0 {
		t.Fatalf("netflix only = %+v", nf)
	}
	var v *ValidationError
	for _, q := range []MappingReviewQuery{{Subject: "imdb"}, {Reason: "vibes"}, {MinRisk: 101, MinGiven: true}} {
		if _, err := svc.Review(ctx, q, 50, 0); !errors.As(err, &v) {
			t.Errorf("%+v: err = %v", q, err)
		}
	}
}
