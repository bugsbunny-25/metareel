package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTMDBFullDetailsMapping(t *testing.T) {
	const body = `{
	  "title": "Bugonia", "original_title": "Bugonia", "original_language": "en", "release_date": "2025-10-24",
	  "runtime": 118, "genres": [{"id": 1, "name": "Comedy"}, {"id": 2, "name": "Science Fiction"}],
	  "production_countries": [{"iso_3166_1": "US"}, {"iso_3166_1": "IE"}],
	  "production_companies": [{"name": "Element Pictures"}], "popularity": 50.5, "vote_average": 7.3, "vote_count": 900,
	  "imdb_id": "tt12300742",
	  "external_ids": {"imdb_id": "tt12300742", "wikidata_id": "Q125971387"},
	  "watch/providers": {"results": {"US": {"link": "https://www.themoviedb.org/movie/701387/watch?locale=US",
	    "flatrate": [{"logo_path": "/p.jpg", "provider_id": 9, "provider_name": "Amazon Prime Video", "display_priority": 1}],
	    "rent": [{"logo_path": "/a.jpg", "provider_id": 2, "provider_name": "Apple TV", "display_priority": 3}]}}},
	  "release_dates": {"results": [{"iso_3166_1": "US", "release_dates": [
	    {"release_date": "2025-10-24T00:00:00.000Z", "type": 3},
	    {"release_date": "2025-12-09T00:00:00.000Z", "type": 4},
	    {"release_date": "2025-12-01T00:00:00.000Z", "type": 4}]}]}
	}`
	var r tmdbFullResp
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	d := r.toDetails("movie", "701387")
	if d.Title != "Bugonia" || d.Runtime != 118 || d.IMDbID != "tt12300742" || d.WikidataID != "Q125971387" {
		t.Fatalf("details = %+v", d)
	}
	if strings.Join(d.Genres, ",") != "Comedy,Science Fiction" || strings.Join(d.OriginCountries, ",") != "US,IE" {
		t.Fatalf("genres/countries = %v %v", d.Genres, d.OriginCountries)
	}
	if d.DigitalReleaseDates["US"] != "2025-12-01" {
		t.Fatalf("digital release = %v (want earliest type-4 date)", d.DigitalReleaseDates)
	}
	if len(d.WatchProviders) != 2 || d.WatchProviders[0].Country != "US" {
		t.Fatalf("providers = %+v", d.WatchProviders)
	}

	var tv tmdbFullResp
	_ = json.Unmarshal([]byte(`{"name": "BEEF", "first_air_date": "2023-04-06", "episode_run_time": [35], "origin_country": ["US"], "networks": [{"name": "Netflix"}]}`), &tv)
	dt := tv.toDetails("tv", "154385")
	if dt.Title != "BEEF" || dt.ReleaseDate != "2023-04-06" || dt.Runtime != 35 || dt.Networks[0] != "Netflix" {
		t.Fatalf("tv details = %+v", dt)
	}
}

func TestWikidataSPARQLLookup(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		query = r.Form.Get("query")
		if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "metareel/") {
			t.Errorf("user agent = %q", ua)
		}
		_, _ = io.WriteString(w, `{"results": {"bindings": [
		  {"tmdb": {"value": "701387"}, "item": {"value": "http://www.wikidata.org/entity/Q125971387"},
		   "imdb": {"value": "tt12300742"}, "rt": {"value": "m/bugonia"}, "mc": {"value": "movie/bugonia"}, "lb": {"value": "bugonia"}}]}}`)
	}))
	defer srv.Close()
	c := NewWikidataSPARQL(time.Second)
	c.endpoint = srv.URL

	got, err := c.LookupByTMDB(context.Background(), "movie", []string{"701387", "550", "x\" } DROP"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "wdt:P4947") || strings.Contains(query, "DROP") {
		t.Fatalf("query = %s", query)
	}
	w := got["701387"]
	if w.WikidataID != "Q125971387" || w.RottenTomato != "m/bugonia" || w.Metacritic != "movie/bugonia" || w.Letterboxd != "bugonia" {
		t.Fatalf("ids = %+v", w)
	}
	if _, ok := got["550"]; ok {
		t.Fatal("unknown title should be absent")
	}
}

func TestDownloaderSkipsUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", "Tue, 06 Oct 2026 18:55:55 GMT")
		_, _ = io.WriteString(w, "week\tcategory\n")
	}))
	defer srv.Close()
	d := NewDownloader(time.Second, "")
	ctx := context.Background()

	dl, err := d.Open(ctx, srv.URL, FileVersion{})
	if err != nil || dl == nil {
		t.Fatalf("first open: %v %v", dl, err)
	}
	dl.Body.Close()
	again, err := d.Open(ctx, srv.URL, dl.Version)
	if err != nil || again != nil {
		t.Fatalf("unchanged file should return nil, got %v %v", again, err)
	}
}
