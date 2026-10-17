package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

type fakeJustWatchOffers struct {
	offers   map[string]map[string][]client.JWOffer // node → country → offers
	search   map[string][]client.MovieOrShowFragment
	err      error
	searches int
	fetched  []string
}

func (f *fakeJustWatchOffers) GetOffers(_ context.Context, nodeID string, _ []string) (map[string][]client.JWOffer, bool, error) {
	f.fetched = append(f.fetched, nodeID)
	if f.err != nil {
		return nil, false, f.err
	}
	o, ok := f.offers[nodeID]
	return o, ok, nil
}

func (f *fakeJustWatchOffers) GetTitlesByTopSearchPopular(_ context.Context, q string, _ int, _, _ string, _ client.ObjectType, _ []client.Package) (*client.GetTitlesByTopSearchPopularQuery, error) {
	f.searches++
	res := &client.GetTitlesByTopSearchPopularQuery{}
	for _, frag := range f.search[q] {
		res.PoupularTitles.Edges = append(res.PoupularTitles.Edges, struct {
			Cursor string
			Node   client.MovieOrShowFragment
		}{Node: frag})
	}
	return res, nil
}

func jwHit(id, tmdbID string) client.MovieOrShowFragment {
	var f client.MovieOrShowFragment
	f.Id = id
	f.Content.ExternalIds.TmdbId = &tmdbID
	return f
}

func wednesdayDetails() *client.TMDBFullDetails {
	link := "https://www.themoviedb.org/tv/119051-wednesday/watch?locale=US"
	return &client.TMDBFullDetails{Kind: "tv", ID: "119051", Title: "Wednesday", Seasons: []client.TMDBSeason{},
		WatchProviders: []client.TMDBWatchProvider{
			{Country: "US", Monetization: "flatrate", ProviderID: 8, ProviderName: "Netflix", Link: link},
			{Country: "US", Monetization: "ads", ProviderID: 1796, ProviderName: "Netflix Standard with Ads", Link: link},
			{Country: "US", Monetization: "buy", ProviderID: 3, ProviderName: "Google Play Movies", Link: link},
			{Country: "IN", Monetization: "flatrate", ProviderID: 8, ProviderName: "Netflix"},
		}}
}

func urlsByKey(t *testing.T, meta *repository.MetadataRepository, ref repository.TmdbRef) map[string]string {
	t.Helper()
	ps, err := meta.WatchProviders(context.Background(), ref, "")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, p := range ps {
		u := ""
		if p.URL != nil {
			u = *p.URL
		}
		out[p.Country+"/"+p.Monetization+"/"+p.ProviderName] = u
	}
	return out
}

func TestMetadataOfferURLs(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	meta := repository.NewMetadataRepository(db)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ref := repository.TmdbRef{Kind: "tv", ID: "119051"}
	seed(t, db, []seedTitle{{Slug: "wednesday", Name: "Wednesday", Kind: "tv_show", TmdbID: "119051"}},
		[]seedRanking{{"wednesday", "2026-10-10", "US", "netflix", "tv_shows", 1}})

	const nfx = "https://www.netflix.com/title/81231974"
	jw := &fakeJustWatchOffers{
		search: map[string][]client.MovieOrShowFragment{"Wednesday": {jwHit("ts9", "999"), jwHit("ts274631", "119051")}},
		offers: map[string]map[string][]client.JWOffer{"ts274631": {
			"US": {{Monetization: "flatrate", PackageID: 8, URL: nfx}, {Monetization: "flatrate", PackageID: 1796, URL: nfx + "?ads"}},
			"IN": {{Monetization: "flatrate", PackageID: 8, URL: nfx + "?in"}},
		}},
	}
	svc := &TitleMetadataService{log: log, repo: meta, justWatch: jw}

	// Before any lookup the title is due (providers stored without offers).
	if err := meta.SaveDetails(ctx, wednesdayDetails(), false); err != nil {
		t.Fatal(err)
	}
	if due, _ := meta.RefsDueForDetails(ctx, "-3 days", "-30 days", 10); len(due) != 1 {
		t.Fatalf("due before offers lookup = %v", due)
	}

	// No stored JustWatch ID: found by search (TMDB ID must match), ID saved.
	if err := svc.saveDetails(ctx, ref, wednesdayDetails()); err != nil {
		t.Fatal(err)
	}
	got := urlsByKey(t, meta, ref)
	want := map[string]string{
		"US/flatrate/Netflix":              nfx,
		"US/ads/Netflix Standard with Ads": nfx + "?ads", // same provider, other monetization
		"US/buy/Google Play Movies":        "",           // JustWatch has no such offer
		"IN/flatrate/Netflix":              nfx + "?in",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s url = %q, want %q", k, got[k], v)
		}
	}
	m, _ := meta.Get(ctx, ref)
	if m == nil || m.JustWatchID == nil || *m.JustWatchID != "ts274631" {
		t.Fatalf("justwatch id = %+v", m)
	}
	if due, _ := meta.RefsDueForDetails(ctx, "-3 days", "-30 days", 10); len(due) != 0 {
		t.Fatalf("due after offers lookup = %v", due)
	}

	// Stored ID: no search.
	if err := svc.saveDetails(ctx, ref, wednesdayDetails()); err != nil {
		t.Fatal(err)
	}
	if jw.searches != 1 || jw.fetched[len(jw.fetched)-1] != "ts274631" {
		t.Fatalf("searches = %d, fetched = %v", jw.searches, jw.fetched)
	}

	// JustWatch failing keeps the URLs of the last refresh.
	jw.err = errors.New("justwatch down")
	if err := svc.saveDetails(ctx, ref, wednesdayDetails()); err != nil {
		t.Fatal(err)
	}
	if got := urlsByKey(t, meta, ref); got["US/flatrate/Netflix"] != nfx || got["IN/flatrate/Netflix"] != nfx+"?in" {
		t.Fatalf("urls after failure = %v", got)
	}

	// The availability response carries each provider's url.
	ov := &TitleOverviewService{metadata: meta}
	av, err := ov.Availability(ctx, ref, "US")
	if err != nil {
		t.Fatal(err)
	}
	us := av.Countries["US"]
	if us == nil || len(us.Flatrate) != 1 || us.Flatrate[0].URL == nil || *us.Flatrate[0].URL != nfx || us.Link == nil {
		t.Fatalf("availability US = %+v", us)
	}
}

func TestMetadataOfferURLsUnknownTitle(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	meta := repository.NewMetadataRepository(db)
	ref := repository.TmdbRef{Kind: "tv", ID: "119051"}
	jw := &fakeJustWatchOffers{search: map[string][]client.MovieOrShowFragment{"Wednesday": {jwHit("ts9", "999")}}}
	svc := &TitleMetadataService{log: slog.New(slog.NewTextHandler(io.Discard, nil)), repo: meta, justWatch: jw}
	if err := svc.saveDetails(ctx, ref, wednesdayDetails()); err != nil {
		t.Fatal(err)
	}
	if got := urlsByKey(t, meta, ref); got["US/flatrate/Netflix"] != "" {
		t.Fatalf("urls = %v", got)
	}
	if len(jw.fetched) != 0 {
		t.Fatalf("fetched offers of %v for a title JustWatch doesn't know", jw.fetched)
	}
	var fetchedAt *string
	if err := db.QueryRow(`SELECT offers_fetched_at FROM tmdb_titles WHERE tmdb_kind = 'tv' AND tmdb_id = '119051'`).Scan(&fetchedAt); err != nil || fetchedAt == nil {
		t.Fatalf("offers_fetched_at = %v, err = %v (a lookup that found nothing still counts)", fetchedAt, err)
	}
}
