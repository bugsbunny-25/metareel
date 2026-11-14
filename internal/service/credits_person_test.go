package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

type fakePeople struct {
	enabled bool
	people  map[int]*client.TMDBPersonDetails
	credits map[string]*client.TMDBCredits
	fail    bool
	calls   atomic.Int32
}

func (f *fakePeople) SearchPeople(_ context.Context, q string) ([]client.TMDBPersonResult, error) {
	if f.fail {
		return nil, errors.New("tmdb search person status 503")
	}
	var out []client.TMDBPersonResult
	for id, p := range f.people {
		if strings.Contains(strings.ToLower(p.Name), strings.ToLower(q)) {
			out = append(out, client.TMDBPersonResult{ID: id, Name: p.Name, KnownFor: p.KnownFor, KnownTitles: []string{}})
		}
	}
	return out, nil
}

func (f *fakePeople) Credits(_ context.Context, kind, id string) (*client.TMDBCredits, error) {
	f.calls.Add(1)
	if f.fail {
		return nil, errors.New("tmdb credits status 503")
	}
	return f.credits[kind+":"+id], nil
}

func (f *fakePeople) Enabled() bool { return f.enabled }
func (f *fakePeople) Person(_ context.Context, id int) (*client.TMDBPersonDetails, error) {
	f.calls.Add(1)
	if f.fail {
		return nil, errors.New("tmdb person status 503")
	}
	return f.people[id], nil
}

func TestFilmography(t *testing.T) {
	got := filmography([]client.TMDBPersonCredit{
		{Kind: "movie", ID: 1, Title: "Old", Date: "1999-05-01", Department: "Acting", Character: "Neo"},
		{Kind: "movie", ID: 1, Title: "Old", Date: "1999-05-01", Department: "Directing", Job: "Director"},
		{Kind: "movie", ID: 1, Title: "Old", Date: "1999-05-01", Department: "Directing", Job: "Director"}, // duplicate
		{Kind: "tv", ID: 1, Title: "Same ID, other kind", Date: "2020-01-01", Department: "Acting", EpisodeCount: 3},
		{Kind: "tv", ID: 1, Title: "Same ID, other kind", Date: "2020-01-01", Department: "Production", Job: "Producer", EpisodeCount: 8},
		{Kind: "movie", ID: 5, Title: "Announced", Department: "Writing", Job: "Screenplay", Popularity: 2},
		{Kind: "movie", ID: 6, Title: "Rumoured", Department: "Acting", Popularity: 9},
		{Kind: "movie", ID: 7, Title: "", Department: "Acting"}, // no title: dropped
	})
	if len(got) != 4 {
		t.Fatalf("titles = %+v", got)
	}
	// Undated first (most popular first), then newest.
	if got[0].TmdbID != "6" || got[1].TmdbID != "5" || got[2].Kind != "tv" || got[3].TmdbID != "1" {
		t.Fatalf("order = %+v", got)
	}
	if r := got[3].Roles; len(r) != 2 || r[0] != "as Neo" || r[1] != "Director" || len(got[3].Departments) != 2 || *got[3].Year != 1999 {
		t.Fatalf("merged = %+v", got[3])
	}
	if got[2].Episodes == nil || *got[2].Episodes != 8 || got[2].Roles[0] != "Actor" {
		t.Fatalf("tv = %+v", got[2])
	}
	if got[0].Date != nil || got[0].Year != nil {
		t.Fatalf("undated = %+v", got[0])
	}
}

func TestPersonWithTMDB(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	meta := repository.NewMetadataRepository(db)
	if err := meta.SaveDetails(ctx, &client.TMDBFullDetails{Kind: "movie", ID: "1", Title: "Heist", ReleaseDate: "2026-03-01"}, false); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewCreditsRepository(db)
	r := NewCreditsRefresher(slog.New(slog.NewTextHandler(io.Discard, nil)), fakeCredits{
		"movie:1": {Cast: []client.TMDBCredit{{PersonID: 10, Name: "Ana", Department: "Acting", Character: "Lead"}}},
	}, repo)
	r.gap = 0
	if _, err := r.Run(ctx, false, nil); err != nil {
		t.Fatal(err)
	}
	src := &fakePeople{enabled: true, people: map[int]*client.TMDBPersonDetails{
		10: {ID: 10, Name: "Ana Real", Biography: "Born somewhere.", Birthday: "1980-02-03", KnownFor: "Acting", IMDbID: "nm1", ProfilePath: "/ana.jpg",
			Credits: []client.TMDBPersonCredit{{Kind: "movie", ID: 1, Title: "Heist", Date: "2026-03-01", Department: "Acting", Character: "Lead"},
				{Kind: "tv", ID: 9, Title: "Early Show", Date: "2001-01-01", Department: "Acting", Character: "Guest", EpisodeCount: 1}}},
		50: {ID: 50, Name: "Only On TMDB", KnownFor: "Directing",
			Credits: []client.TMDBPersonCredit{{Kind: "movie", ID: 3, Title: "Debut", Date: "2010-01-01", Department: "Directing", Job: "Director"}}},
	}}
	now := time.Now()
	svc := NewCreditsService(repo, meta).WithTMDB(src)
	svc.now = func() time.Time { return now }

	ana, err := svc.Person(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if ana.Name != "Ana" || len(ana.Titles) != 1 || ana.DetailsStatus != PersonDetailsOK || ana.Details == nil ||
		*ana.Details.Biography != "Born somewhere." || *ana.Details.IMDbID != "nm1" || *ana.ProfilePath != "/ana.jpg" ||
		len(ana.Filmography) != 2 || ana.Filmography[0].TmdbID != "1" || ana.Details.AlsoKnownAs == nil {
		t.Fatalf("ana = %+v", ana)
	}

	// Cached: no second TMDB call until the TTL passes.
	if _, err := svc.Person(ctx, 10); err != nil || src.calls.Load() != 1 {
		t.Fatalf("calls = %d, err %v", src.calls.Load(), err)
	}
	now = now.Add(personCacheTTL + time.Minute)
	if _, err := svc.Person(ctx, 10); err != nil || src.calls.Load() != 2 {
		t.Fatalf("after ttl calls = %d, err %v", src.calls.Load(), err)
	}

	// Known only to TMDB.
	only, err := svc.Person(ctx, 50)
	if err != nil || only.Name != "Only On TMDB" || len(only.Titles) != 0 || only.Titles == nil || len(only.Filmography) != 1 || *only.KnownFor != "Directing" {
		t.Fatalf("tmdb only = %+v, %v", only, err)
	}

	// Unknown everywhere; the miss is cached for an hour.
	calls := src.calls.Load()
	for range 2 {
		if _, err := svc.Person(ctx, 404); !errors.Is(err, ErrPersonNotFound) {
			t.Fatalf("unknown: %v", err)
		}
	}
	if src.calls.Load() != calls+1 {
		t.Fatalf("miss not cached: %d calls", src.calls.Load()-calls)
	}

	// TMDB failing: a charted person still answers; an unknown one is a 502.
	src.fail = true
	now = now.Add(personCacheTTL + time.Minute)
	ana, err = svc.Person(ctx, 10)
	if err != nil || ana.DetailsStatus != PersonDetailsUnavailable || ana.Details != nil || ana.Filmography == nil || len(ana.Titles) != 1 {
		t.Fatalf("tmdb down = %+v, %v", ana, err)
	}
	if _, err := svc.Person(ctx, 77); !errors.Is(err, ErrPersonUnavailable) {
		t.Fatalf("unknown while tmdb down: %v", err)
	}

	// No TMDB key.
	plain := NewCreditsService(repo, meta).WithTMDB(&fakePeople{})
	if ana, err := plain.Person(ctx, 10); err != nil || ana.DetailsStatus != PersonDetailsNotConfigured || ana.Name != "Ana" {
		t.Fatalf("not configured = %+v, %v", ana, err)
	}
	if _, err := plain.Person(ctx, 50); !errors.Is(err, ErrPersonNotFound) {
		t.Fatalf("not configured, unknown: %v", err)
	}
}

func TestSearchPeople(t *testing.T) {
	src := &fakePeople{enabled: true, people: map[int]*client.TMDBPersonDetails{1: {Name: "Ana Real", KnownFor: "Acting"}, 2: {Name: "Ben"}}}
	svc := NewCreditsService(nil, nil).WithTMDB(src)
	ctx := context.Background()
	got, err := svc.SearchPeople(ctx, "  ana ", 0)
	if err != nil || len(got) != 1 || got[0].PersonID != 1 || *got[0].KnownFor != "Acting" || got[0].ProfilePath != nil {
		t.Fatalf("search = %+v, %v", got, err)
	}
	var verr *ValidationError
	if _, err := svc.SearchPeople(ctx, "a", 0); !errors.As(err, &verr) {
		t.Fatalf("one letter: %v", err)
	}
	src.fail = true
	if _, err := svc.SearchPeople(ctx, "ana", 0); !errors.Is(err, ErrPersonUnavailable) {
		t.Fatalf("tmdb down: %v", err)
	}
	if _, err := NewCreditsService(nil, nil).SearchPeople(ctx, "ana", 0); !errors.Is(err, ErrPersonUnavailable) {
		t.Fatalf("no tmdb: %v", err)
	}
}

func TestTMDBCacheBounded(t *testing.T) {
	c := newTMDBCache[int64, *client.TMDBPersonDetails](personCacheMax)
	now := time.Now()
	for i := range personCacheMax + 5 {
		c.put(int64(i), nil, true, now.Add(time.Duration(i)*time.Second))
	}
	if c.len() != personCacheMax {
		t.Fatalf("cache size = %d", c.len())
	}
	if _, _, ok := c.get(0, now.Add(time.Minute)); ok {
		t.Fatal("oldest entry kept")
	}
	if _, miss, ok := c.get(personCacheMax+4, now.Add(personCacheMax*time.Second)); !ok || !miss {
		t.Fatal("newest miss not cached")
	}
	if _, _, ok := c.get(personCacheMax+4, now.Add(personCacheMax*time.Second+personMissTTL+time.Minute)); ok {
		t.Fatal("miss kept past its TTL")
	}
}

func TestTitleCreditsLive(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	meta := repository.NewMetadataRepository(db)
	if err := meta.SaveDetails(ctx, &client.TMDBFullDetails{Kind: "movie", ID: "1", Title: "Catalogued"}, false); err != nil {
		t.Fatal(err)
	}
	src := &fakePeople{enabled: true, credits: map[string]*client.TMDBCredits{
		"movie:1": {Cast: []client.TMDBCredit{{PersonID: 10, Name: "Ana", Character: "Lead", Department: "Acting"}}},
		"tv:7": {Cast: []client.TMDBCredit{{PersonID: 11, Name: "Ben", Character: "Host", Order: 1, Episodes: 12}, {PersonID: 12, Name: "Cy", Order: 0}},
			Crew: []client.TMDBCredit{{PersonID: 20, Name: "Dee", Job: "Creator", Department: "Writing"}, {PersonID: 21, Name: "Gaffer", Job: "Gaffer"}}},
	}}
	repo := repository.NewCreditsRepository(db)
	svc := NewCreditsService(repo, meta).WithTMDB(src)

	// Catalogued, credits job not run yet: live, with the stored title.
	c, err := svc.TitleCredits(ctx, TmdbRef{Kind: "movie", ID: "1"})
	if err != nil || c.Source != "tmdb" || *c.Title != "Catalogued" || len(c.Cast) != 1 || c.Cast[0].Name != "Ana" {
		t.Fatalf("catalogued = %+v, %v", c, err)
	}
	// Unknown to metareel: live, billing order, kept jobs only.
	c, err = svc.TitleCredits(ctx, TmdbRef{Kind: "tv", ID: "7"})
	if err != nil || c.Source != "tmdb" || c.Title != nil || len(c.Cast) != 2 || c.Cast[0].Name != "Cy" || *c.Cast[1].Episodes != 12 ||
		len(c.Crew) != 1 || c.Crew[0].Job != "Creator" {
		t.Fatalf("unknown title = %+v, %v", c, err)
	}
	calls := src.calls.Load()
	if _, err := svc.TitleCredits(ctx, TmdbRef{Kind: "tv", ID: "7"}); err != nil || src.calls.Load() != calls {
		t.Fatalf("not cached: %d calls", src.calls.Load()-calls)
	}
	// Unknown everywhere.
	if _, err := svc.TitleCredits(ctx, TmdbRef{Kind: "movie", ID: "404"}); !errors.Is(err, ErrTitleNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	// TMDB failing: a catalogued title answers with nothing; an unknown one is a 502.
	src.fail = true
	if err := meta.SaveDetails(ctx, &client.TMDBFullDetails{Kind: "movie", ID: "2", Title: "Other"}, false); err != nil {
		t.Fatal(err)
	}
	if c, err := svc.TitleCredits(ctx, TmdbRef{Kind: "movie", ID: "2"}); err != nil || c.Source != "none" || len(c.Cast) != 0 {
		t.Fatalf("catalogued while down = %+v, %v", c, err)
	}
	if _, err := svc.TitleCredits(ctx, TmdbRef{Kind: "movie", ID: "3"}); !errors.Is(err, ErrCreditsUnavailable) {
		t.Fatalf("unknown while down: %v", err)
	}
	// Without TMDB the old behaviour holds.
	plain := NewCreditsService(repo, meta)
	if c, err := plain.TitleCredits(ctx, TmdbRef{Kind: "movie", ID: "1"}); err != nil || c.Source != "none" {
		t.Fatalf("no tmdb = %+v, %v", c, err)
	}
	if _, err := plain.TitleCredits(ctx, TmdbRef{Kind: "tv", ID: "7"}); !errors.Is(err, ErrTitleNotFound) {
		t.Fatalf("no tmdb, unknown: %v", err)
	}
}
