package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// fakeFetcher serves the saved US Netflix chart for chart URLs whose path
// contains a key of pages, and errors for everything else.
type fakeFetcher struct {
	html   string
	pages  map[string]bool // URL substring → serve the chart
	health error
	calls  []string
}

func (f *fakeFetcher) Fetch(_ context.Context, pageURL, _ string, _ bool) (*goquery.Document, error) {
	f.calls = append(f.calls, pageURL)
	for k, ok := range f.pages {
		if ok && strings.Contains(pageURL, k) {
			return goquery.NewDocumentFromReader(strings.NewReader(f.html))
		}
	}
	return nil, errors.New("unexpected status 500")
}

func (f *fakeFetcher) Health(context.Context) error { return f.health }

func newJobFixture(t *testing.T, pages map[string]bool) (*FlixPatrolJob, *fakeFetcher, *repository.FlixPatrolRepository) {
	t.Helper()
	html, err := os.ReadFile("../scraper/flixpatrol/testdata/netflix_united_states_2026-04-28.html")
	if err != nil {
		t.Fatal(err)
	}
	repo := repository.NewFlixPatrolRepository(newTestDB(t))
	f := &fakeFetcher{html: string(html), pages: pages}
	job := NewFlixPatrolJob(slog.New(slog.NewTextHandler(io.Discard, nil)), f, repo)
	// 2026-04-28 13:00 UTC: the current chart date is 2026-04-28.
	job.now = func() time.Time { return time.Date(2026, 4, 28, 13, 0, 0, 0, time.UTC) }
	return job, f, repo
}

func TestFlixPatrolJobContinuesPastFailedTargets(t *testing.T) {
	job, _, repo := newJobFixture(t, map[string]bool{"/netflix/united-states/": true})
	ctx := context.Background()
	targets := []FlixPatrolScrapeTarget{
		{CountrySlug: "united-states", Provider: flixpatrol.ProviderHBOMax}, // fails
		{CountrySlug: "united-states", Provider: flixpatrol.ProviderNetflix},
	}
	rep, err := job.Run(ctx, targets, FlixPatrolRunOptions{AcceptUnchanged: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Count(TargetFailed) != 1 || rep.Count(TargetSucceeded) != 1 {
		t.Fatalf("results = %+v", rep.Results)
	}
	if len(rep.NewTitleIDs) == 0 {
		t.Fatal("expected new titles to match")
	}
	stored, err := repo.ChartSnapshotsForDate(ctx, time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC), "US", "netflix")
	if err != nil || len(stored) != 2 {
		t.Fatalf("snapshots = %v err=%v", stored, err)
	}

	// A retry skips the stored chart and only fetches the failed one.
	rep, err = job.Run(ctx, targets, FlixPatrolRunOptions{AcceptUnchanged: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Count(TargetSkipped) != 1 || rep.Count(TargetFailed) != 1 {
		t.Fatalf("retry results = %+v", rep.Results)
	}
}

func TestFlixPatrolJobBackfillAndFreshness(t *testing.T) {
	job, f, _ := newJobFixture(t, map[string]bool{"/netflix/united-states/": true})
	ctx := context.Background()
	targets := []FlixPatrolScrapeTarget{{CountrySlug: "united-states", Provider: flixpatrol.ProviderNetflix}}

	// The fake serves the same chart for every date, so after filling the
	// gaps (oldest first) today's chart looks unpublished.
	rep, err := job.Run(ctx, targets, FlixPatrolRunOptions{BackfillDays: 2})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range rep.Results {
		got = append(got, r.Date+":"+r.Status)
	}
	want := "2026-04-26:succeeded 2026-04-27:succeeded 2026-04-28:not_fresh"
	if strings.Join(got, " ") != want {
		t.Fatalf("results = %v, want %s", got, want)
	}
	if len(f.calls) != 3 || !strings.HasSuffix(f.calls[0], "/2026-04-26/") {
		t.Fatalf("fetches = %v", f.calls)
	}
	if nf := rep.NotFresh(); len(nf) != 1 || nf[0].Provider != flixpatrol.ProviderNetflix {
		t.Fatalf("not fresh = %+v", nf)
	}

	// Once re-checking is over, the unchanged chart is stored; gaps are filled.
	rep, err = job.Run(ctx, targets, FlixPatrolRunOptions{BackfillDays: 2, AcceptUnchanged: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Results) != 1 || rep.Results[0].Status != TargetSucceeded {
		t.Fatalf("second run = %+v", rep.Results)
	}
}

func TestFlixPatrolJobFetcherDown(t *testing.T) {
	job, f, _ := newJobFixture(t, nil)
	f.health = errors.New("connection refused")
	rep, err := job.Run(context.Background(), []FlixPatrolScrapeTarget{{CountrySlug: "united-states", Provider: flixpatrol.ProviderNetflix}}, FlixPatrolRunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Count(TargetFailed) != 1 || len(f.calls) != 0 {
		t.Fatalf("results = %+v calls = %v", rep.Results, f.calls)
	}
}

func TestFlixPatrolJobStopsAfterRepeatedFetchFailures(t *testing.T) {
	job, f, _ := newJobFixture(t, nil)
	var targets []FlixPatrolScrapeTarget
	for _, p := range []flixpatrol.Provider{"netflix", "hbo-max", "disney", "peacock", "apple-tv"} {
		targets = append(targets, FlixPatrolScrapeTarget{CountrySlug: "united-states", Provider: p})
	}
	rep, err := job.Run(context.Background(), targets, FlixPatrolRunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Count(TargetFailed) != 5 || len(f.calls) != maxConsecutiveFetchFailures {
		t.Fatalf("failed=%d fetches=%d", rep.Count(TargetFailed), len(f.calls))
	}
}
