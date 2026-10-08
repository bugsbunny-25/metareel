package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
)

type newTitlesCall struct {
	pageType client.NewPageType
	date     string
	pack     string
	after    string
}

type fakeReleasesJW struct {
	mu           sync.Mutex // the service queries days concurrently
	packages     []client.JWPackage
	packageCalls int
	// pages[pageType+date] is the list of pages returned in order of cursor.
	pages map[string][]client.NewTitlesPage
	calls []newTitlesCall
	err   error
}

func (f *fakeReleasesJW) GetPackages(context.Context, string) ([]client.JWPackage, error) {
	f.packageCalls++
	return f.packages, nil
}

func (f *fakeReleasesJW) GetNewTitles(_ context.Context, pageType client.NewPageType, _, _, date, pack string, _ []client.ObjectType, after string, _ int) (*client.NewTitlesPage, error) {
	f.mu.Lock()
	f.calls = append(f.calls, newTitlesCall{pageType, date, pack, after})
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	key := string(pageType)
	if pageType == client.NewPageTypeNew {
		key += date
	}
	pages := f.pages[key]
	idx := 0
	if after != "" {
		idx = int(after[0] - '0')
	}
	if idx >= len(pages) {
		return &client.NewTitlesPage{}, nil
	}
	return &pages[idx], nil
}

func edgeFromJSON(t *testing.T, raw string) client.NewTitleEdge {
	t.Helper()
	var e client.NewTitleEdge
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("bad edge fixture: %v", err)
	}
	return e
}

const movieEdge = `{"NewOffer":{"MonetizationType":"FLATRATE","PresentationType":"SD","StandardWebURL":"https://www.netflix.com/title/1"},
 "Node":{"MovieOrSeason":{"ID":"tm1","ObjectType":"MOVIE","Content":{"Title":"Who Is Martin Mull?","ShortDescription":"A doc.","FullPath":"/us/movie/who-is-martin-mull",
 "OriginalReleaseYear":2026,"OriginalReleaseDate":"2026-10-07","Runtime":95,"PosterURL":"/poster/1/s592/m.jpg",
 "ExternalIds":{"TmdbId":"1756138","ImdbId":"tt1"},"Scoring":{"ImdbScore":7.1,"ImdbVotes":120,"TomatoMeter":90,"CertifiedFresh":false},
 "Genres":[{"ShortName":"doc","Translation":"Documentary"}],"Season":{},"UpcomingReleases":[]}}}}`

const seasonEdge = `{"NewOffer":{"MonetizationType":"FLATRATE","PresentationType":"HD","StandardWebURL":"https://www.netflix.com/title/2"},
 "Node":{"MovieOrSeason":{"ID":"tss2","ObjectType":"SHOW_SEASON","Content":{"Title":"Season 2","ShortDescription":"","FullPath":"/us/tv-show/the-show/season-2",
 "OriginalReleaseYear":2026,"Runtime":0,"ExternalIds":{"TmdbId":"290233:2"},"Scoring":{},"Genres":[],"Season":{"SeasonNumber":2},
 "UpcomingReleases":[{"ReleaseDate":"2026-10-09","ReleaseType":"THEATRICAL","Package":{"ShortName":"tmd"}},
                     {"ReleaseDate":"2026-10-12","ReleaseType":"DIGITAL","Package":{"ShortName":"nfx"}},
                     {"ReleaseDate":"2026-10-10","ReleaseType":"PHYSICAL","Package":{"ShortName":"nfx"}}]},
 "Season":{"Show":{"ID":"ts2","Content":{"Title":"The Show","ShortDescription":"Show description.","PosterURL":"/poster/2/s592/show.jpg",
 "ExternalIds":{"ImdbId":"tt2"},"Scoring":{"ImdbScore":8.2,"ImdbVotes":5000},"Genres":[{"ShortName":"drm","Translation":"Drama"}]}}}}}}`

const undatedEdge = `{"Node":{"MovieOrSeason":{"ID":"tm3","ObjectType":"MOVIE","Content":{"Title":"Someday","ExternalIds":{"TmdbId":"3"},"Scoring":{},"Genres":[],"Season":{},
 "UpcomingReleases":[{"ReleaseDate":"2026-11-01","ReleaseType":"DIGITAL","Package":{"ShortName":"dnp"}}]}}}}`

const soonEdge = `{"Node":{"MovieOrSeason":{"ID":"tm4","ObjectType":"MOVIE","Content":{"Title":"Animals","ExternalIds":{"TmdbId":"4"},"Scoring":{},"Genres":[],"Season":{},
 "UpcomingReleases":[{"ReleaseDate":"2026-10-09","ReleaseType":"DIGITAL","Package":{"ShortName":"nfx"}}]}}}}`

func newReleasesFixture(t *testing.T) (*ReleasesService, *fakeReleasesJW) {
	t.Helper()
	jw := &fakeReleasesJW{
		packages: []client.JWPackage{
			{ShortName: "nfx", ClearName: "Netflix", TechnicalName: "netflix"},
			{ShortName: "atp", ClearName: "Apple TV", TechnicalName: "appletvplus"},
			{ShortName: "ppa", ClearName: "Paramount Plus Apple TV channel", TechnicalName: "appletvparamountplus"},
			{ShortName: "prv", ClearName: "Amazon Prime Video", TechnicalName: "amazonprimevideo"},
			{ShortName: "ppp", ClearName: "Paramount Plus Premium", TechnicalName: "paramountpluspremium"},
		},
		pages: map[string][]client.NewTitlesPage{},
	}
	svc := &ReleasesService{
		log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		jw:       jw,
		now:      func() time.Time { return time.Date(2026, 10, 8, 3, 0, 0, 0, time.UTC) },
		packages: map[string]packagesCacheEntry{},
	}
	return svc, jw
}

func TestReleases_ResolveService(t *testing.T) {
	svc, jw := newReleasesFixture(t)
	ctx := context.Background()
	for in, want := range map[string]string{"netflix": "nfx", "NFX": "nfx", "apple-tv": "atp", "appletvparamountplus": "ppa", "ppa": "ppa",
		// Country-specific codes: no "amp" / "pmp" here, so the next candidate wins.
		"amazon-prime": "prv", "paramount-plus": "ppp"} {
		p, err := svc.resolveService(ctx, "US", in)
		if err != nil || p.ShortName != want {
			t.Errorf("resolveService(%q) = %q, %v; want %q", in, p.ShortName, err, want)
		}
	}
	var vErr *ValidationError
	if _, err := svc.resolveService(ctx, "US", "hbo-max"); !errors.As(err, &vErr) {
		t.Errorf("service missing in country: err = %v, want ValidationError", err)
	}
	if jw.packageCalls != 1 {
		t.Errorf("packages fetched %d times, want 1 (cached)", jw.packageCalls)
	}
}

func TestReleases_GetNew(t *testing.T) {
	svc, jw := newReleasesFixture(t)
	jw.pages["NEW2026-10-07"] = []client.NewTitlesPage{
		{Edges: []client.NewTitleEdge{edgeFromJSON(t, movieEdge)}, PageInfo: client.PageInfo{HasNextPage: true, EndCursor: "1"}},
		{Edges: []client.NewTitleEdge{edgeFromJSON(t, seasonEdge)}},
	}
	jw.pages["NEW2026-10-05"] = []client.NewTitlesPage{{Edges: []client.NewTitleEdge{edgeFromJSON(t, seasonEdge)}}}

	out, err := svc.GetNew(context.Background(), ReleasesQuery{CountryCode: "us", Service: "netflix"})
	if err != nil {
		t.Fatalf("GetNew: %v", err)
	}
	if *out.From != "2026-10-02" || *out.To != "2026-10-08" || out.Service.JustWatchPackage != "nfx" || out.Service.Name != "Netflix" {
		t.Fatalf("range/service = %v..%v %+v, want last 7 days on nfx", *out.From, *out.To, out.Service)
	}
	var dates []string
	for _, c := range jw.calls {
		if c.after == "" {
			dates = append(dates, c.date)
		}
	}
	sort.Strings(dates)
	if len(dates) != 7 || dates[0] != "2026-10-02" || dates[6] != "2026-10-08" {
		t.Fatalf("queried days = %v", dates)
	}
	if out.Count != 3 || len(out.Items) != 3 {
		t.Fatalf("count = %d, want 3 (2 on 10-07 across two pages, 1 on 10-05)", out.Count)
	}
	if *out.Items[0].Date != "2026-10-07" || *out.Items[2].Date != "2026-10-05" {
		t.Fatalf("items not newest day first: %v, %v", *out.Items[0].Date, *out.Items[2].Date)
	}

	movie := out.Items[0]
	if movie.Kind != "movie" || movie.Title != "Who Is Martin Mull?" || *movie.TmdbID != "1756138" || *movie.ImdbID != "tt1" ||
		*movie.JustWatchURL != "https://www.justwatch.com/us/movie/who-is-martin-mull" || *movie.PosterURL != "https://images.justwatch.com/poster/1/s592/m.jpg" ||
		*movie.RuntimeMinutes != 95 || movie.Genres[0] != "Documentary" || *movie.IMDbRating != 7.1 || *movie.Tomatometer != 90 ||
		*movie.Offer.URL != "https://www.netflix.com/title/1" || *movie.Offer.MonetizationType != "FLATRATE" || movie.SeasonNumber != nil {
		t.Fatalf("movie = %+v", movie)
	}

	season := out.Items[1]
	if season.Kind != "tv" || season.Title != "The Show" || *season.SeasonNumber != 2 || *season.SeasonTitle != "Season 2" ||
		*season.TmdbID != "290233" || *season.ImdbID != "tt2" || *season.Description != "Show description." ||
		*season.PosterURL != "https://images.justwatch.com/poster/2/s592/show.jpg" || season.Genres[0] != "Drama" ||
		*season.IMDbRating != 8.2 || season.RuntimeMinutes != nil || season.ReleaseType != nil {
		t.Fatalf("season = %+v", season)
	}

	tv, err := svc.GetNew(context.Background(), ReleasesQuery{CountryCode: "US", Service: "nfx", Kind: "tv"})
	if err != nil || tv.Count != 2 {
		t.Fatalf("type=tv: count=%d err=%v, want 2 seasons", tv.Count, err)
	}
}

func TestReleases_GetNewValidation(t *testing.T) {
	svc, _ := newReleasesFixture(t)
	ctx := context.Background()
	from, to := day("2026-09-01"), day("2026-10-08")
	cases := []ReleasesQuery{
		{CountryCode: "US", Service: "netflix", From: &from, To: &to}, // > 31 days
		{CountryCode: "US", Service: "netflix", From: &to, To: &from}, // reversed
		{CountryCode: "XX", Service: "netflix"},                       // country
		{CountryCode: "US", Service: "netflix", Kind: "anime"},        // type
		{CountryCode: "US", Service: "nope"},                          // service
		{CountryCode: "US", Service: "netflix", Language: "english"},  // language
	}
	for _, q := range cases {
		var vErr *ValidationError
		if _, err := svc.GetNew(ctx, q); !errors.As(err, &vErr) {
			t.Errorf("GetNew(%+v): err = %v, want ValidationError", q, err)
		}
	}
}

func TestReleases_GetUpcoming(t *testing.T) {
	svc, jw := newReleasesFixture(t)
	jw.pages["UPCOMING"] = []client.NewTitlesPage{{Edges: []client.NewTitleEdge{
		edgeFromJSON(t, seasonEdge), edgeFromJSON(t, undatedEdge), edgeFromJSON(t, soonEdge),
	}}}
	ctx := context.Background()

	out, err := svc.GetUpcoming(ctx, ReleasesQuery{CountryCode: "US", Service: "netflix"})
	if err != nil {
		t.Fatalf("GetUpcoming: %v", err)
	}
	if out.Count != 3 || out.From != nil || out.To != nil {
		t.Fatalf("count=%d from=%v to=%v", out.Count, out.From, out.To)
	}
	// Soonest first; the date is the Netflix (digital) release, not the
	// theatrical one; titles with no Netflix date come last.
	got := []string{out.Items[0].Title, out.Items[1].Title, out.Items[2].Title}
	if got[0] != "Animals" || got[1] != "The Show" || got[2] != "Someday" {
		t.Fatalf("order = %v", got)
	}
	if *out.Items[1].Date != "2026-10-12" || *out.Items[1].ReleaseType != "DIGITAL" || out.Items[2].Date != nil || out.Items[0].Offer != nil {
		t.Fatalf("dates = %+v / %+v", out.Items[1], out.Items[2])
	}

	from, to := day("2026-10-10"), day("2026-10-31")
	ranged, err := svc.GetUpcoming(ctx, ReleasesQuery{CountryCode: "US", Service: "netflix", From: &from, To: &to})
	if err != nil {
		t.Fatalf("GetUpcoming ranged: %v", err)
	}
	if ranged.Count != 1 || ranged.Items[0].Title != "The Show" || *ranged.From != "2026-10-10" {
		t.Fatalf("ranged = %+v", ranged.Items)
	}
}

func TestReleases_UpstreamError(t *testing.T) {
	svc, jw := newReleasesFixture(t)
	jw.err = errors.New("boom")
	_, err := svc.GetUpcoming(context.Background(), ReleasesQuery{CountryCode: "US", Service: "netflix"})
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}
