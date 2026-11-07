package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

type fakeReviewSearch struct {
	hits  map[string][]MappingCandidate // "kind|name"
	fail  map[string]bool               // providers that fail
	calls int
}

func (f *fakeReviewSearch) ReviewSearch(_ context.Context, q CandidateQuery) ([]MappingCandidate, map[string]string) {
	f.calls++
	var errs map[string]string
	for p := range f.fail {
		if errs == nil {
			errs = map[string]string{}
		}
		errs[p] = "search failed"
	}
	return f.hits[q.Kind+"|"+q.Name], errs
}

type fakeReviewOffers map[string][]string // "kind:id" -> countries

func (f fakeReviewOffers) WatchProviderCountries(_ context.Context, kind, id string, providers []int) ([]string, error) {
	if len(providers) == 0 || providers[0] != 9 {
		panic("expected Prime Video provider IDs")
	}
	return f[kind+":"+id], nil
}

func TestReviewSuggestions(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	d1 := time.Now().UTC().AddDate(0, 0, -10).Format("2006-01-02")
	seed(t, db, []seedTitle{{Slug: "family-man", Name: "The Family Man", Kind: "tv_show", TmdbID: "1111"}},
		[]seedRanking{{"family-man", d1, "IN", "amazon-prime", "tv_shows", 1}})
	if err := repository.NewMetadataRepository(db).SaveDetails(ctx, &client.TMDBFullDetails{Kind: "tv", ID: "1111", Title: "The Family Man",
		ReleaseDate: "1990-09-21"}, false); err != nil {
		t.Fatal(err)
	}
	id := func(s string) *string { return &s }
	year := func(y int) *int { return &y }
	chartYear := time.Now().UTC().AddDate(0, 0, -10).Year()
	search := &fakeReviewSearch{hits: map[string][]MappingCandidate{
		"tv_show|The Family Man": {
			{Title: "The Family Man", Year: year(1990), Kind: "tv_show", TmdbID: id("1111")}, // the current mapping
			{Title: "The Family Man", Year: year(2019), Kind: "tv_show", TmdbID: id("93352")},
			{Title: "The Family Man Show", Year: year(2001), Kind: "tv_show", TmdbID: id("5")},
			{Title: "The Family Man", Year: year(chartYear + 3), Kind: "tv_show", TmdbID: id("6")}, // not out yet
			{Title: "No TMDB", Kind: "tv_show"},
			{Title: "The Family Man", Kind: "tv_show", TmdbID: id("../search")}, // not a TMDB ID
			// Dated: out 5 days after it first charted / 100 days before.
			{Title: "The Family Man", Kind: "tv_show", TmdbID: id("7"), ReleaseDate: id(time.Now().UTC().AddDate(0, 0, -5).Format("2006-01-02"))},
			{Title: "The Family Man", Kind: "tv_show", TmdbID: id("8"), ReleaseDate: id(time.Now().UTC().AddDate(0, 0, -110).Format("2006-01-02"))},
		},
		"movie|The Family Man": {{Title: "The Family Man", Year: year(2000), Kind: "movie", TmdbID: id("9")},
			{Title: "The Family Man", Year: year(2019), Kind: "tv_show", TmdbID: id("93352")}}, // seen already
	}}
	svc := NewReviewFixService(slog.New(slog.NewTextHandler(io.Discard, nil)), repository.NewMappingRepository(db), search,
		fakeReviewOffers{"tv:93352": {"IN", "US"}})
	svc.gap = 0

	out, err := svc.Suggestions(ctx, "flixpatrol", 1)
	if err != nil {
		t.Fatal(err)
	}
	if out.TmdbID != "1111" || out.Name != "The Family Man" || len(out.Providers) != 1 || search.calls != 2 {
		t.Fatalf("subject = %+v (calls %d)", out, search.calls)
	}
	var order []string
	for _, s := range out.Suggestions {
		order = append(order, s.TmdbKind+":"+s.TmdbID)
	}
	// Five at most: tv:7 (released after it charted, tied with tv:6) is cut.
	if got := joinLines(order); got != joinLines([]string{"tv:93352", "tv:8", "movie:9", "tv:5", "tv:6"}) {
		t.Fatalf("order = %v", order)
	}
	top := out.Suggestions[0]
	if top.Score != 40+10+35 || top.OnProvider == nil || !*top.OnProvider || len(top.Reasons) != 3 {
		t.Fatalf("top = %+v", top)
	}
	if m := out.Suggestions[2]; m.OnProvider == nil || *m.OnProvider || m.Score != 40 {
		t.Fatalf("movie = %+v", m)
	}
	if late := out.Suggestions[4]; late.Score != 40+10-50 {
		t.Fatalf("not released yet = %+v", late)
	}
	if before := out.Suggestions[1]; before.Score != 40+10+15 {
		t.Fatalf("released shortly before = %+v", before)
	}

	// Kept for a few minutes: no new searches.
	if again, err := svc.Suggestions(ctx, "flixpatrol", 1); err != nil || again != out || search.calls != 2 {
		t.Fatalf("cached = %v, calls %d", err, search.calls)
	}
	svc.now = func() time.Time { return time.Now().Add(reviewCacheTTL) }

	// Search failures are reported per provider without detail (and not
	// cached).
	search.fail = map[string]bool{"tmdb": true}
	if out, err = svc.Suggestions(ctx, "flixpatrol", 1); err != nil || out.Errors["tmdb"] != "search failed" {
		t.Fatalf("errors = %+v, %v", out, err)
	}
	calls := search.calls
	if _, err = svc.Suggestions(ctx, "flixpatrol", 1); err != nil || search.calls == calls {
		t.Fatalf("a result with errors was cached (calls %d)", search.calls)
	}
	if _, err := svc.Suggestions(ctx, "netflix", 1); !errors.Is(err, ErrReviewSubjectNotFound) {
		t.Fatalf("not in queue: %v", err)
	}
	var v *ValidationError
	if _, err := svc.Suggestions(ctx, "imdb", 1); !errors.As(err, &v) {
		t.Fatalf("bad subject: %v", err)
	}
}

func TestDecodeEntities(t *testing.T) {
	for in, want := range map[string]string{
		"Big John&#39;s Bosh! Family": "Big John's Bosh! Family",
		"Don&#x27;t":                  "Don't",
		"Tom &amp; Jerry":             "Tom & Jerry",
		"Bread&nothing":               "Bread&nothing", // not "&not" + "hing"
		"Me&times":                    "Me&times",
		"Fast & Furious":              "Fast & Furious",
	} {
		if got := decodeEntities(in); got != want {
			t.Errorf("decodeEntities(%q) = %q, want %q", in, got, want)
		}
	}
	if normalizeTitle("Big John&#39;s") != normalizeTitle("Big John's") || normalizeTitle("Bread&nothing") != "breadandnothing" {
		t.Fatal("normalizeTitle does not decode entities the same way")
	}
}
