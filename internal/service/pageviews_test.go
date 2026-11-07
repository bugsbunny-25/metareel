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

// fakePageviews returns a view count per day (day of month x 10) for known
// articles and records the requested ranges.
type fakePageviews struct {
	articles map[string]bool
	fail     bool
	calls    []string
}

func (f *fakePageviews) Pageviews(_ context.Context, project, article string, from, to time.Time) ([]client.DailyViews, error) {
	f.calls = append(f.calls, article+" "+from.Format("2006-01-02")+".."+to.Format("2006-01-02"))
	if f.fail {
		return nil, errors.New("boom")
	}
	if project != "en.wikipedia" || !f.articles[article] {
		return nil, nil
	}
	var out []client.DailyViews
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		out = append(out, client.DailyViews{Day: d.Format("2006-01-02"), Views: int64(d.Day() * 10)})
	}
	return out, nil
}

func TestPageviewsImport(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	meta := repository.NewMetadataRepository(db)
	for _, d := range []client.TMDBFullDetails{{Kind: "tv", ID: "1", Title: "Show"}, {Kind: "movie", ID: "2", Title: "Film"}, {Kind: "movie", ID: "3", Title: "No article"}} {
		if err := meta.SaveDetails(ctx, &d, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := meta.SaveEnwiki(ctx, TmdbRef{Kind: "tv", ID: "1"}, "Show (TV series)"); err != nil {
		t.Fatal(err)
	}
	if err := meta.SaveEnwiki(ctx, TmdbRef{Kind: "movie", ID: "2"}, "Renamed film"); err != nil {
		t.Fatal(err)
	}
	if err := meta.SaveEnwiki(ctx, TmdbRef{Kind: "movie", ID: "3"}, ""); err != nil {
		t.Fatal(err)
	}
	hist := repository.NewHistoryRepository(db)
	src := &fakePageviews{articles: map[string]bool{"Show (TV series)": true}}
	imp := NewPageviewsImporter(slog.New(slog.NewTextHandler(io.Discard, nil)), src, hist)
	imp.gap = 0
	imp.now = func() time.Time { return time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC) }

	rep, err := imp.Run(ctx, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	// First import: the last 90 days up to yesterday.
	if rep.Titles != 2 || rep.Fetched != 1 || rep.Missing != 1 || rep.Days != 90 || src.calls[0] != "Renamed film 2026-08-08..2026-11-05" && src.calls[1] != "Renamed film 2026-08-08..2026-11-05" {
		t.Fatalf("first run = %+v, calls %v", rep, src.calls)
	}

	// Next day: only the new day; a run on the same day fetches nothing.
	imp.now = func() time.Time { return time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC) }
	src.calls = nil
	rep, _ = imp.Run(ctx, false, nil)
	if rep.Days != 1 || len(src.calls) != 2 || (src.calls[0] != "Show (TV series) 2026-11-06..2026-11-06" && src.calls[1] != "Show (TV series) 2026-11-06..2026-11-06") {
		t.Fatalf("second run = %+v, calls %v", rep, src.calls)
	}
	src.calls = nil
	if rep, _ = imp.Run(ctx, false, nil); rep.UpToDate != 1 || rep.Fetched != 0 {
		t.Fatalf("same-day run = %+v", rep)
	}
	// Force refetches the last week.
	if rep, _ = imp.Run(ctx, true, nil); rep.Days != 7 {
		t.Fatalf("forced run = %+v", rep)
	}

	points, err := hist.Pageviews(ctx, TmdbRef{Kind: "tv", ID: "1"}, "en.wikipedia", "2026-11-01", "2026-11-30")
	if err != nil || len(points) != 6 || points[0].Date != "2026-11-01" || points[0].Views != 10 || points[5].Date != "2026-11-06" {
		t.Fatalf("stored = %+v, %v", points, err)
	}

	// The title history carries them, with the article's URL.
	got, err := NewHistoryService(hist, meta).TitleHistory(ctx, TmdbRef{Kind: "tv", ID: "1"}, TitleHistoryQuery{From: timePtr(day("2026-11-01")), To: timePtr(day("2026-11-06"))})
	if err != nil {
		t.Fatal(err)
	}
	if got.Wikipedia == nil || got.Wikipedia.URL != "https://en.wikipedia.org/wiki/Show_%28TV_series%29" || len(got.Wikipedia.Points) != 6 {
		t.Fatalf("history wikipedia = %+v", got.Wikipedia)
	}
	if none, _ := NewHistoryService(hist, meta).TitleHistory(ctx, TmdbRef{Kind: "movie", ID: "3"}, TitleHistoryQuery{}); none.Wikipedia != nil {
		t.Fatalf("title without article: %+v", none.Wikipedia)
	}

	// A rate limit is waited out and the title retried.
	limited := &rateLimitedOnce{fakePageviews: src}
	imp.source = limited
	imp.now = func() time.Time { return time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC) }
	if rep, err = imp.Run(ctx, false, nil); err != nil || rep.Failed != 0 || rep.Fetched != 1 || limited.limited != 1 {
		t.Fatalf("rate-limited run = %+v, %v (limited %d)", rep, err, limited.limited)
	}
	imp.source = src

	// Every request failing is an error.
	src.fail = true
	imp.now = func() time.Time { return time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC) }
	if _, err := imp.Run(ctx, false, nil); err == nil {
		t.Fatal("all failures: no error")
	}
}

// rateLimitedOnce answers the first request with a 429.
type rateLimitedOnce struct {
	*fakePageviews
	limited int
}

func (r *rateLimitedOnce) Pageviews(ctx context.Context, project, article string, from, to time.Time) ([]client.DailyViews, error) {
	if r.limited == 0 {
		r.limited++
		return nil, &client.RateLimitedError{RetryAfter: time.Millisecond}
	}
	return r.fakePageviews.Pageviews(ctx, project, article, from, to)
}
