package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

func offer(country, monetization string, id int, name string) client.TMDBWatchProvider {
	return client.TMDBWatchProvider{Country: country, Monetization: monetization, ProviderID: id, ProviderName: name}
}

// TestWatchProviderEvents: the first refresh is the baseline (no events);
// later refreshes record offers that appeared or disappeared.
func TestWatchProviderEvents(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	meta := repository.NewMetadataRepository(db)
	svc := NewHistoryService(repository.NewHistoryRepository(db), meta)
	ref := TmdbRef{Kind: "tv", ID: "100"}

	save := func(offers ...client.TMDBWatchProvider) {
		t.Helper()
		if err := meta.SaveDetails(ctx, &client.TMDBFullDetails{Kind: "tv", ID: "100", Title: "Show", WatchProviders: offers}, false); err != nil {
			t.Fatal(err)
		}
	}
	save(offer("US", "flatrate", 8, "Netflix"), offer("GB", "flatrate", 8, "Netflix"))
	got, err := svc.TitleAvailabilityHistory(ctx, ref, "")
	if err != nil || len(got.Events) != 0 {
		t.Fatalf("after first refresh: %+v, %v; want no events", got, err)
	}

	// US: Netflix leaves, Prime arrives; GB unchanged; DE rent appears.
	save(offer("US", "flatrate", 9, "Prime Video"), offer("GB", "flatrate", 8, "Netflix"), offer("DE", "rent", 2, "Apple TV"))
	got, err = svc.TitleAvailabilityHistory(ctx, ref, "")
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	for _, e := range got.Events {
		if e.PreviousCheckAt == "" || e.ObservedAt == "" {
			t.Errorf("event %+v lacks check times", e)
		}
		seen = append(seen, fmt.Sprintf("%s %s %s %d", e.Event, e.Country, e.Monetization, e.ProviderID))
	}
	want := map[string]bool{"added DE rent 2": true, "added US flatrate 9": true, "removed US flatrate 8": true}
	if len(seen) != len(want) {
		t.Fatalf("events = %v, want %v", seen, want)
	}
	for _, s := range seen {
		if !want[s] {
			t.Fatalf("unexpected event %q (all: %v)", s, seen)
		}
	}

	us, err := svc.TitleAvailabilityHistory(ctx, ref, "us")
	if err != nil || len(us.Events) != 2 {
		t.Fatalf("US events = %+v, %v; want 2", us, err)
	}

	feed, err := svc.AvailabilityChanges(ctx, AvailabilityChangesQuery{Country: "US", Event: "added"})
	if err != nil {
		t.Fatal(err)
	}
	if len(feed.Events) != 1 || feed.Events[0].ProviderName != "Prime Video" || feed.Events[0].Title != "Show" {
		t.Fatalf("feed = %+v, want Prime Video added to Show", feed.Events)
	}
	future := time.Now().Add(time.Hour)
	if feed, err = svc.AvailabilityChanges(ctx, AvailabilityChangesQuery{Since: &future}); err != nil || len(feed.Events) != 0 {
		t.Fatalf("feed since future = %+v, %v; want none", feed, err)
	}

	if _, err := svc.TitleAvailabilityHistory(ctx, TmdbRef{Kind: "tv", ID: "999"}, ""); !errors.Is(err, ErrTitleNotFound) {
		t.Fatalf("unknown title: err = %v, want ErrTitleNotFound", err)
	}
}

func TestAvailabilityChangesValidation(t *testing.T) {
	svc := NewHistoryService(repository.NewHistoryRepository(newTestDB(t)), nil)
	for _, q := range []AvailabilityChangesQuery{
		{Event: "gone"}, {Monetization: "lease"}, {ProviderID: "x"}, {ProviderID: "-1"}, {Country: "ZZ"},
	} {
		var v *ValidationError
		if _, err := svc.AvailabilityChanges(context.Background(), q); !errors.As(err, &v) {
			t.Errorf("%+v: err = %v, want ValidationError", q, err)
		}
	}
}

// TestTitleHistory: rating series keep a point only when the value changed
// and start with the value in effect at `from`; popularity is per day.
func TestTitleHistory(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	q := sqlc.New(db)
	meta := repository.NewMetadataRepository(db)
	if err := meta.SaveDetails(ctx, &client.TMDBFullDetails{Kind: "movie", ID: "5", Title: "Film", IMDbID: "tt0000005"}, false); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct {
		date   string
		rating float64
		votes  int64
	}{
		{"2026-01-01", 7.0, 100},
		{"2026-01-02", 7.0, 100}, // unchanged: not stored
		{"2026-03-01", 7.1, 150},
		{"2026-03-02", 7.1, 160},
	} {
		if err := q.RecordIMDbRating(ctx, sqlc.RecordIMDbRatingParams{ImdbID: "tt0000005", CapturedOn: r.date, AverageRating: r.rating, NumVotes: r.votes}); err != nil {
			t.Fatal(err)
		}
	}
	ratings := repository.NewRatingsRepository(db)
	for i, fetched := range []time.Time{day("2026-02-01"), day("2026-02-10"), day("2026-03-05")} {
		value := 90.0
		if i == 2 {
			value = 88
		}
		if _, _, err := ratings.SaveTitleRatings(ctx, repository.TitleRatings{TmdbKind: "movie", TmdbID: "5", RefreshedAt: fetched},
			map[string][]repository.RatingSource{"rottentomatoes": {{Source: "tomatoes", Value: value, FetchedAt: fetched}}}); err != nil {
			t.Fatal(err)
		}
	}
	hist := repository.NewHistoryRepository(db)
	if err := hist.SavePopularity(ctx, []repository.PopularityPoint{
		{Ref: TmdbRef{Kind: "movie", ID: "5"}, Date: "2026-02-28", Popularity: 3},
		{Ref: TmdbRef{Kind: "movie", ID: "5"}, Date: "2026-03-01", Popularity: 4.5},
		{Ref: TmdbRef{Kind: "movie", ID: "5"}, Date: "2026-03-01", Popularity: 5}, // same day: replaced
		{Ref: TmdbRef{Kind: "tv", ID: "5"}, Date: "2026-03-01", Popularity: 99},   // other namespace
	}); err != nil {
		t.Fatal(err)
	}

	svc := NewHistoryService(hist, meta)
	got, err := svc.TitleHistory(ctx, TmdbRef{Kind: "movie", ID: "5"}, TitleHistoryQuery{From: timePtr(day("2026-02-15")), To: timePtr(day("2026-03-31"))})
	if err != nil {
		t.Fatal(err)
	}
	dates := func(ps []repository.RatingPoint) string {
		var s []string
		for _, p := range ps {
			s = append(s, fmt.Sprintf("%s=%g", p.Date, p.Value))
		}
		return strings.Join(s, ",")
	}
	if got.IMDb == nil || dates(got.IMDb.Points) != "2026-01-01=7,2026-03-01=7.1,2026-03-02=7.1" {
		t.Fatalf("imdb = %+v", got.IMDb)
	}
	if len(got.Ratings) != 1 || dates(got.Ratings[0].Points) != "2026-02-01=90,2026-03-05=88" {
		t.Fatalf("ratings = %+v", got.Ratings)
	}
	if len(got.Popularity) != 2 || got.Popularity[1].Popularity != 5 {
		t.Fatalf("popularity = %+v", got.Popularity)
	}

	var v *ValidationError
	if _, err := svc.TitleHistory(ctx, TmdbRef{Kind: "movie", ID: "5"}, TitleHistoryQuery{From: timePtr(day("2026-04-01")), To: timePtr(day("2026-03-01"))}); !errors.As(err, &v) {
		t.Fatalf("from after to: err = %v", err)
	}
	if _, err := svc.TitleHistory(ctx, TmdbRef{Kind: "movie", ID: "5"}, TitleHistoryQuery{From: timePtr(day("2024-01-01")), To: timePtr(day("2026-03-01"))}); !errors.As(err, &v) {
		t.Fatalf("range too long: err = %v", err)
	}
	if _, err := svc.TitleHistory(ctx, TmdbRef{Kind: "movie", ID: "6"}, TitleHistoryQuery{}); !errors.Is(err, ErrTitleNotFound) {
		t.Fatalf("unknown title: err = %v", err)
	}
}

func gzLines(t *testing.T, lines ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write([]byte(strings.Join(lines, "\n") + "\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestTMDBPopularityImport: known titles only, yesterday's export while
// today's is missing, and unchanged files skipped.
func TestTMDBPopularityImport(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	meta := repository.NewMetadataRepository(db)
	for _, d := range []client.TMDBFullDetails{{Kind: "movie", ID: "1"}, {Kind: "tv", ID: "2"}} {
		if err := meta.SaveDetails(ctx, &d, false); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string][]byte{
		"/movie_ids_11_05_2026.json.gz":     gzLines(t, `{"id":1,"original_title":"A","popularity":12.5}`, `{"id":3,"popularity":1}`, `not json`),
		"/tv_series_ids_11_06_2026.json.gz": gzLines(t, `{"id":2,"original_name":"B","popularity":7.25}`, `{"id":1,"popularity":4}`),
	}
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("ETag", `"`+r.URL.Path+`"`)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	hist := repository.NewHistoryRepository(db)
	imp := NewTMDBPopularityImporter(nil, client.NewDownloader(time.Minute, ""), repository.NewImportsRepository(db), hist)
	imp.now = func() time.Time { return time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC) }
	imp.url = srv.URL + "/%s_ids_%s.json.gz"

	rep, err := imp.Run(ctx, false, nil)
	if err != nil {
		t.Fatalf("run: %v (%+v)", err, rep)
	}
	if rep.Files[0].Status != "imported" || rep.Files[0].Rows != 1 || rep.Files[0].Skipped != 1 ||
		rep.Files[1].Status != "imported" || rep.Files[1].Rows != 1 {
		t.Fatalf("report = %+v", rep.Files)
	}
	movie, _ := hist.PopularityHistory(ctx, TmdbRef{Kind: "movie", ID: "1"}, "2026-01-01", "2026-12-31")
	tv, _ := hist.PopularityHistory(ctx, TmdbRef{Kind: "tv", ID: "2"}, "2026-01-01", "2026-12-31")
	if len(movie) != 1 || movie[0].Date != "2026-11-05" || movie[0].Popularity != 12.5 {
		t.Fatalf("movie popularity = %+v (from yesterday's export)", movie)
	}
	if len(tv) != 1 || tv[0].Date != "2026-11-06" || tv[0].Popularity != 7.25 {
		t.Fatalf("tv popularity = %+v", tv)
	}

	rep, err = imp.Run(ctx, false, nil)
	if err != nil || rep.Files[0].Status != "unchanged" || rep.Files[1].Status != "unchanged" {
		t.Fatalf("second run = %+v, %v; want unchanged", rep, err)
	}

	delete(files, "/movie_ids_11_05_2026.json.gz")
	if rep, err = imp.Run(ctx, false, nil); err == nil || rep.Files[0].Status != "failed" {
		t.Fatalf("no movie export: %+v, %v; want failure", rep, err)
	}
	if len(requests) == 0 {
		t.Fatal("no requests made")
	}
}

func timePtr(t time.Time) *time.Time { return &t }
