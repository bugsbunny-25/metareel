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

type fakeJustWatch struct {
	byID   map[string]*client.MovieOrShowFragment
	search []client.MovieOrShowFragment
	err    error
	calls  int
}

func (f *fakeJustWatch) GetTitleByID(_ context.Context, id, _, _ string) (*client.MovieOrShowFragment, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.byID[id], nil
}

func (f *fakeJustWatch) GetTitlesByTopSearchPopular(_ context.Context, _ string, _ int, _, _ string, _ client.ObjectType, _ []client.Package) (*client.GetTitlesByTopSearchPopularQuery, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	var out client.GetTitlesByTopSearchPopularQuery
	for _, frag := range f.search {
		out.PoupularTitles.Edges = append(out.PoupularTitles.Edges, struct {
			Cursor string
			Node   client.MovieOrShowFragment
		}{Node: frag})
	}
	return &out, nil
}

type fakeMDBList struct {
	media *client.MDBListMedia
	err   error
	calls int
}

func (f *fakeMDBList) GetByTmdb(context.Context, string, string) (*client.MDBListMedia, error) {
	f.calls++
	return f.media, f.err
}

type fakeRT struct {
	bySlug      map[string]*client.RTRating
	search      *client.RTRating
	err         error
	slugCalls   []string
	searchCalls []string
}

func (f *fakeRT) BySlug(_ context.Context, slug string) (*client.RTRating, error) {
	f.slugCalls = append(f.slugCalls, slug)
	return f.bySlug[slug], f.err
}

func (f *fakeRT) Search(_ context.Context, _ client.RTKind, name string, _ int) (*client.RTRating, error) {
	f.searchCalls = append(f.searchCalls, name)
	return f.search, f.err
}

func i64(v int64) *int64     { return &v }
func f64(v float64) *float64 { return &v }
func str(v string) *string   { return &v }
func boolp(v bool) *bool     { return &v }

func bugoniaFragment() client.MovieOrShowFragment {
	var f client.MovieOrShowFragment
	f.Id = "tm1504418"
	f.Content.Title = "Bugonia"
	year := 2025
	f.Content.OriginalReleaseYear = &year
	f.Content.ExternalIds.TmdbId = str("701387")
	f.Content.ExternalIds.ImdbId = str("tt12300742")
	votes, meter, certified := 228706.0, 87.0, true
	f.Content.Scoring.ImdbScore = f64(7.4)
	f.Content.Scoring.ImdbVotes = &votes
	f.Content.Scoring.TomatoMeter = &meter
	f.Content.Scoring.CertifiedFresh = &certified
	f.Content.Scoring.TmdbScore = f64(7.3)
	return f
}

func bugoniaMDBList() *client.MDBListMedia {
	return &client.MDBListMedia{
		Title:  "Bugonia",
		Year:   2025,
		ImdbID: "tt12300742",
		Score:  f64(78),
		Ratings: []client.MDBListRating{
			{Source: "imdb", Value: f64(7.5), Votes: f64(230000)},
			{Source: "tomatoes", Value: f64(86), URL: "https://www.rottentomatoes.com/m/bugonia"},
			{Source: "tomatoesaudience", Value: f64(71)},
			{Source: "metacritic", Value: f64(73)},
			{Source: "letterboxd", Value: nil},
		},
	}
}

type ratingsFixture struct {
	svc   *RatingsService
	repo  *repository.FlixPatrolRepository
	jw    *fakeJustWatch
	mdb   *fakeMDBList
	rt    *fakeRT
	clock time.Time
}

func newRatingsFixture(t *testing.T) *ratingsFixture {
	t.Helper()
	db := newTestDB(t)
	flixRepo := repository.NewFlixPatrolRepository(db)
	f := &ratingsFixture{
		repo:  flixRepo,
		jw:    &fakeJustWatch{search: []client.MovieOrShowFragment{bugoniaFragment()}},
		mdb:   &fakeMDBList{media: bugoniaMDBList()},
		rt:    &fakeRT{bySlug: map[string]*client.RTRating{"m/bugonia": {Slug: "m/bugonia", Title: "Bugonia", Year: 2025, CriticsScore: i64(88), AudienceScore: i64(72), CertifiedFresh: boolp(true)}}},
		clock: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC),
	}
	f.svc = &RatingsService{
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		repo:      repository.NewRatingsRepository(db),
		titles:    flixRepo,
		justWatch: f.jw,
		mdblist:   f.mdb,
		rt:        f.rt,
		ttl:       12 * time.Hour,
		preferred: ProviderJustWatch,
		now:       func() time.Time { return f.clock },
	}
	return f
}

var bugonia = TmdbRef{Kind: "movie", ID: "701387"}

func TestRatings_RefreshThenCache(t *testing.T) {
	f := newRatingsFixture(t)
	ctx := context.Background()
	if _, err := seedOne(t, f.repo, seedTitle{Slug: "bugonia", Name: "Bugonia", Kind: "movie", TmdbID: "701387"}); err != nil {
		t.Fatal(err)
	}

	out, err := f.svc.GetRatings(ctx, bugonia, false)
	if err != nil {
		t.Fatalf("GetRatings: %v", err)
	}
	if out.Cached || out.Stale {
		t.Fatalf("first call should refresh, got cached=%v stale=%v", out.Cached, out.Stale)
	}
	if out.IMDb == nil || out.IMDb.Rating != 7.4 || out.IMDb.Source != ProviderJustWatch || *out.IMDb.Votes != 228706 {
		t.Fatalf("imdb = %+v, want justwatch 7.4", out.IMDb)
	}
	rt := out.RottenTomatoes
	if rt == nil || *rt.CriticsScore != 88 || *rt.CriticsSource != ProviderRottenTomatoes || *rt.AudienceScore != 72 ||
		*rt.CriticsRating != "Certified Fresh" || *rt.AudienceRating != "Upright" || *rt.URL != "https://www.rottentomatoes.com/m/bugonia" {
		t.Fatalf("rotten_tomatoes = %+v", rt)
	}
	// MDBList's RT link supplied the slug, so no fuzzy search was needed.
	if len(f.rt.slugCalls) != 1 || f.rt.slugCalls[0] != "m/bugonia" || len(f.rt.searchCalls) != 0 {
		t.Fatalf("rt calls: slug=%v search=%v", f.rt.slugCalls, f.rt.searchCalls)
	}
	// Every provider's ratings are stored, e.g. MDBList's Metacritic.
	got := map[string]float64{}
	for _, s := range out.Sources {
		got[s.Provider+"/"+s.Source] = s.Value
	}
	for key, want := range map[string]float64{
		"justwatch/imdb": 7.4, "justwatch/tomatoes": 87, "justwatch/tmdb": 7.3,
		"mdblist/imdb": 7.5, "mdblist/popcorn": 71, "mdblist/metacritic": 73, "mdblist/mdblist": 78,
		"rottentomatoes/tomatoes": 88, "rottentomatoes/popcorn": 72, "rottentomatoes/tomatoes_certified": 1,
	} {
		if got[key] != want {
			t.Errorf("source %s = %v, want %v", key, got[key], want)
		}
	}
	if _, ok := got["mdblist/letterboxd"]; ok {
		t.Errorf("null MDBList rating should not be stored")
	}

	// IDs found while rating are back-filled onto the scraped title.
	title, err := f.repo.GetTitleBySlug(ctx, "bugonia")
	if err != nil {
		t.Fatal(err)
	}
	if title.ImdbID.String != "tt12300742" || title.RtUrl.String != "m/bugonia" {
		t.Fatalf("title ids not back-filled: imdb=%q rt=%q", title.ImdbID.String, title.RtUrl.String)
	}

	// Within the TTL: served from the database without calling providers.
	jwCalls, mdbCalls := f.jw.calls, f.mdb.calls
	f.clock = f.clock.Add(11 * time.Hour)
	out, err = f.svc.GetRatings(ctx, bugonia, false)
	if err != nil {
		t.Fatalf("GetRatings (cached): %v", err)
	}
	if !out.Cached || f.jw.calls != jwCalls || f.mdb.calls != mdbCalls || out.IMDb.Rating != 7.4 {
		t.Fatalf("expected cached response without provider calls, cached=%v jw=%d mdb=%d", out.Cached, f.jw.calls-jwCalls, f.mdb.calls-mdbCalls)
	}

	// refresh=true bypasses the cache, and the stored JustWatch ID is used
	// instead of searching again.
	frag := bugoniaFragment()
	frag.Content.Scoring.ImdbScore = f64(7.6)
	f.jw.byID = map[string]*client.MovieOrShowFragment{"tm1504418": &frag}
	f.jw.search = nil
	out, err = f.svc.GetRatings(ctx, bugonia, true)
	if err != nil || out.Cached || out.IMDb.Rating != 7.6 {
		t.Fatalf("forced refresh: err=%v cached=%v imdb=%+v", err, out != nil && out.Cached, out.IMDb)
	}
}

func TestRatings_RTFallsBackToProviders(t *testing.T) {
	f := newRatingsFixture(t)
	f.rt.bySlug = nil // RT search index has no match
	f.svc.preferred = ProviderMDBList

	out, err := f.svc.GetRatings(context.Background(), bugonia, false)
	if err != nil {
		t.Fatalf("GetRatings: %v", err)
	}
	if out.IMDb.Source != ProviderMDBList || out.IMDb.Rating != 7.5 {
		t.Fatalf("imdb = %+v, want preferred mdblist", out.IMDb)
	}
	rt := out.RottenTomatoes
	if rt == nil || *rt.CriticsScore != 86 || *rt.CriticsSource != ProviderMDBList || *rt.AudienceScore != 71 || *rt.AudienceSource != ProviderMDBList {
		t.Fatalf("rotten_tomatoes = %+v, want mdblist fallback", rt)
	}
	// MDBList has no Certified Fresh flag, so it comes from JustWatch.
	if rt.CertifiedFresh == nil || !*rt.CertifiedFresh || *rt.CriticsRating != "Certified Fresh" {
		t.Fatalf("certified fresh = %v rating=%v", rt.CertifiedFresh, *rt.CriticsRating)
	}
	// Slug lookup missed, so RT was searched by name.
	if len(f.rt.searchCalls) != 1 || f.rt.searchCalls[0] != "Bugonia" {
		t.Fatalf("rt search calls = %v", f.rt.searchCalls)
	}
}

func TestRatings_SearchFindsSlugWithoutMDBList(t *testing.T) {
	f := newRatingsFixture(t)
	f.svc.mdblist = nil
	f.rt.bySlug = nil
	f.rt.search = &client.RTRating{Slug: "m/bugonia", Title: "Bugonia", Year: 2025, CriticsScore: i64(88), AudienceScore: i64(72), CertifiedFresh: boolp(true)}
	ctx := context.Background()
	if _, err := seedOne(t, f.repo, seedTitle{Slug: "bugonia", Name: "Bugonia", Kind: "movie", TmdbID: "701387"}); err != nil {
		t.Fatal(err)
	}

	out, err := f.svc.GetRatings(ctx, bugonia, false)
	if err != nil {
		t.Fatalf("GetRatings: %v", err)
	}
	if *out.RottenTomatoes.CriticsSource != ProviderRottenTomatoes || len(f.rt.searchCalls) != 1 {
		t.Fatalf("rt = %+v search=%v", out.RottenTomatoes, f.rt.searchCalls)
	}

	// Next refresh uses the stored slug instead of searching.
	f.rt.bySlug = map[string]*client.RTRating{"m/bugonia": f.rt.search}
	f.clock = f.clock.Add(13 * time.Hour)
	if _, err := f.svc.GetRatings(ctx, bugonia, false); err != nil {
		t.Fatalf("GetRatings: %v", err)
	}
	if len(f.rt.searchCalls) != 1 || len(f.rt.slugCalls) == 0 || f.rt.slugCalls[len(f.rt.slugCalls)-1] != "m/bugonia" {
		t.Fatalf("second refresh: slug=%v search=%v", f.rt.slugCalls, f.rt.searchCalls)
	}
}

func TestRatings_StaleWhenRefreshFails(t *testing.T) {
	f := newRatingsFixture(t)
	ctx := context.Background()
	if _, err := f.svc.GetRatings(ctx, bugonia, false); err != nil {
		t.Fatalf("GetRatings: %v", err)
	}

	boom := errors.New("upstream down")
	f.jw.err, f.mdb.err, f.rt.err = boom, boom, boom
	f.clock = f.clock.Add(13 * time.Hour)
	out, err := f.svc.GetRatings(ctx, bugonia, false)
	if err != nil {
		t.Fatalf("GetRatings: %v", err)
	}
	if !out.Stale || !out.Cached || out.IMDb.Rating != 7.4 {
		t.Fatalf("want stale cached ratings, got stale=%v cached=%v imdb=%+v", out.Stale, out.Cached, out.IMDb)
	}
}

func TestRatings_PartialFailureKeepsProviderData(t *testing.T) {
	f := newRatingsFixture(t)
	ctx := context.Background()
	if _, err := f.svc.GetRatings(ctx, bugonia, false); err != nil {
		t.Fatalf("GetRatings: %v", err)
	}

	// MDBList fails on the next refresh: its previously stored ratings stay.
	f.mdb.err = errors.New("rate limited")
	f.clock = f.clock.Add(13 * time.Hour)
	out, err := f.svc.GetRatings(ctx, bugonia, false)
	if err != nil {
		t.Fatalf("GetRatings: %v", err)
	}
	found := false
	for _, s := range out.Sources {
		if s.Provider == ProviderMDBList && s.Source == "metacritic" {
			found = true
		}
	}
	if out.Stale || !found {
		t.Fatalf("stale=%v, mdblist metacritic kept=%v", out.Stale, found)
	}
}

func TestRatings_Errors(t *testing.T) {
	t.Run("unknown title", func(t *testing.T) {
		f := newRatingsFixture(t)
		f.mdb.media = nil
		f.jw.search = nil
		_, err := f.svc.GetRatings(context.Background(), TmdbRef{Kind: "movie", ID: "1"}, false)
		if !errors.Is(err, ErrTitleNotFound) {
			t.Fatalf("err = %v, want ErrTitleNotFound", err)
		}
	})
	t.Run("all providers down, nothing cached", func(t *testing.T) {
		f := newRatingsFixture(t)
		boom := errors.New("upstream down")
		f.jw.err, f.mdb.err, f.rt.err = boom, boom, boom
		_, err := f.svc.GetRatings(context.Background(), bugonia, false)
		if !errors.Is(err, ErrRatingsUnavailable) {
			t.Fatalf("err = %v, want ErrRatingsUnavailable", err)
		}
	})
}

func TestRtSlugFromURL(t *testing.T) {
	for raw, want := range map[string]string{
		"https://www.rottentomatoes.com/m/bugonia":   "m/bugonia",
		"https://www.rottentomatoes.com/tv/beef/s01": "tv/beef",
		"/m/apex_2026": "m/apex_2026",
		"https://www.rottentomatoes.com/celebrity/someone": "",
		"": "",
	} {
		if got := rtSlugFromURL(raw); got != want {
			t.Errorf("rtSlugFromURL(%q) = %q, want %q", raw, got, want)
		}
	}
}
