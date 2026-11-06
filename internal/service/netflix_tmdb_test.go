package service

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/client"
)

// fakeNetflixTMDB answers searches by name and kind; onNetflix lists
// "kind:id" streaming on Netflix; alts holds alternative titles.
type fakeNetflixTMDB struct {
	movies, tv map[string][]client.TMDBSearchResult
	onNetflix  map[string]bool
	alts       map[string][]string
	calls      int
}

func (f *fakeNetflixTMDB) SearchMovie(_ context.Context, q string) ([]client.TMDBSearchResult, error) {
	f.calls++
	return f.movies[q], nil
}

func (f *fakeNetflixTMDB) SearchTV(_ context.Context, q string) ([]client.TMDBSearchResult, error) {
	f.calls++
	return f.tv[q], nil
}

func (f *fakeNetflixTMDB) WatchProviderCountries(_ context.Context, kind, id string, providers []int) ([]string, error) {
	f.calls++
	if len(providers) == 0 || providers[0] != 8 {
		panic("expected Netflix provider IDs")
	}
	if f.onNetflix[kind+":"+id] {
		return []string{"TH", "US"}, nil
	}
	return nil, nil
}

func (f *fakeNetflixTMDB) AlternativeTitles(_ context.Context, kind, id string) ([]string, error) {
	f.calls++
	return f.alts[kind+":"+id], nil
}

func hit(id, year int, title string) client.TMDBSearchResult {
	return client.TMDBSearchResult{ID: id, Title: title, OriginalTitle: title, Year: year}
}

func TestNetflixTMDBMatch(t *testing.T) {
	f := &fakeNetflixTMDB{
		movies: map[string][]client.TMDBSearchResult{
			// Several films share the name; one is on Netflix.
			"Turbo": {hit(1001, 2013, "Turbo"), hit(1002, 1986, "Turbo"), hit(1003, 2016, "Turbo Kid 2")},
			// A local film under its English name.
			"Pee Nak 5": {{ID: 2001, Title: "Pee Nak 5", OriginalTitle: "พี่นาค 5", Year: 2026}},
			// Two on Netflix, one a recent release.
			"The Hunt": {hit(3001, 2020, "The Hunt"), hit(3002, 2012, "The Hunt"), hit(3003, 2026, "The Hunt")},
			// Two old ones on Netflix: ambiguous.
			"Black and Blue": {hit(4001, 2019, "Black and Blue"), hit(4002, 2014, "Black and Blue")},
			// Only an alternative title matches.
			"Murder Suspect": {hit(5001, 2025, "Murder: Suspect No.1")},
			// A stand-up special: a TV list, a TMDB movie.
			"Derrick Stroup: Nostalgic": {hit(6001, 2026, "Derrick Stroup: Nostalgic")},
			// Not on Netflix anywhere.
			"The Pact": {hit(7001, 2012, "The Pact")},
			// Released after it charted: impossible.
			"Never Let Go": {hit(8001, 2027, "Never Let Go"), hit(8002, 2024, "Never Let Go")},
		},
		tv: map[string][]client.TMDBSearchResult{
			"Upstairs, Downstairs": {hit(9001, 1971, "Upstairs, Downstairs"), hit(9002, 2010, "Upstairs Downstairs")},
		},
		onNetflix: map[string]bool{
			"movie:1001": true, "movie:2001": true, "movie:3001": true, "movie:3003": true,
			"movie:4001": true, "movie:4002": true, "movie:5001": true, "movie:6001": true,
			"movie:8001": true, "movie:8002": true, "tv:9002": true,
		},
		alts: map[string][]string{"movie:5001": {"Murder Suspect", "Asesinato"}},
	}
	m := &netflixTMDBMatcher{tmdb: f, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	cases := []struct {
		name, list string
		year       int
		wantID     int
		wantKind   string
		wantSource string
	}{
		{"Turbo", "movie", 2026, 1001, "movie", MatchSourceTMDBNetflix},
		{"Pee Nak 5", "movie", 2026, 2001, "movie", MatchSourceTMDBNetflix},
		{"The Hunt", "movie", 2026, 3003, "movie", MatchSourceTMDBNetflix},
		{"Black and Blue", "movie", 2026, 0, "", ""},
		{"Murder Suspect", "movie", 2026, 5001, "movie", MatchSourceTMDBNetflixAltTitle},
		{"Derrick Stroup: Nostalgic", "tv_show", 2026, 6001, "movie", MatchSourceTMDBNetflix},
		{"The Pact", "movie", 2026, 0, "", ""},
		{"Never Let Go", "movie", 2026, 8002, "movie", MatchSourceTMDBNetflix},
		{"Upstairs, Downstairs", "tv_show", 2026, 9002, "tv", MatchSourceTMDBNetflix},
		{"Unknown Name", "movie", 2026, 0, "", ""},
	}
	for _, c := range cases {
		got, kind, ok := m.match(context.Background(), c.name, c.list, c.year)
		if c.wantID == 0 {
			if ok {
				t.Errorf("%s: matched %+v (%s), want no match", c.name, got, kind)
			}
			continue
		}
		if !ok || got.TmdbID != strconv.Itoa(c.wantID) || kind != c.wantKind || got.Source != c.wantSource {
			t.Errorf("%s: got %+v kind %q ok %v, want %d %s via %s", c.name, got, kind, ok, c.wantID, c.wantKind, c.wantSource)
		}
	}
	if f.calls == 0 {
		t.Fatal("no TMDB calls")
	}
}
