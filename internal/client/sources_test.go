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
	_ = json.Unmarshal([]byte(`{"name": "BEEF", "first_air_date": "2023-04-06", "episode_run_time": [35], "origin_country": ["US"], "networks": [{"name": "Netflix"}],
	  "seasons": [{"season_number": 0, "name": "Specials", "episode_count": 2},
	              {"season_number": 1, "name": " Season 1 ", "air_date": "2023-04-06", "episode_count": 10}]}`), &tv)
	dt := tv.toDetails("tv", "154385")
	if dt.Title != "BEEF" || dt.ReleaseDate != "2023-04-06" || dt.Runtime != 35 || dt.Networks[0] != "Netflix" {
		t.Fatalf("tv details = %+v", dt)
	}
	if len(dt.Seasons) != 2 || dt.Seasons[1] != (TMDBSeason{Number: 1, Name: "Season 1", AirDate: "2023-04-06", EpisodeCount: 10}) {
		t.Fatalf("tv seasons = %+v", dt.Seasons)
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
		   "imdb": {"value": "tt12300742"}, "rt": {"value": "m/bugonia"}, "mc": {"value": "movie/bugonia"}, "lb": {"value": "bugonia"},
		   "nf": {"value": "81700000"}}]}}`)
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
	if w.WikidataID != "Q125971387" || w.RottenTomato != "m/bugonia" || w.Metacritic != "movie/bugonia" || w.Letterboxd != "bugonia" || w.NetflixID != "81700000" {
		t.Fatalf("ids = %+v", w)
	}
	if _, ok := got["550"]; ok {
		t.Fatal("unknown title should be absent")
	}
}

func TestWikidataFindNetflixTitles(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		query = r.Form.Get("query")
		_, _ = io.WriteString(w, `{"results": {"bindings": [
		  {"name": {"value": "Arcane"}, "item": {"value": "http://www.wikidata.org/entity/Q85518571"}, "nf": {"value": "81435684"},
		   "tmdbtv": {"value": "94605"}, "imdb": {"value": "tt11126994"}, "year": {"value": "2021"}},
		  {"name": {"value": "Arcane"}, "item": {"value": "http://www.wikidata.org/entity/Q85518571"}, "nf": {"value": "81435684"},
		   "tmdbtv": {"value": "94605"}, "imdb": {"value": "tt99"}, "year": {"value": "2021"}},
		  {"name": {"value": "Raw"}, "item": {"value": "http://www.wikidata.org/entity/Q21427355"}, "nf": {"value": "80115405"},
		   "tmdbm": {"value": "393519"}, "year": {"value": "2016"}},
		  {"name": {"value": "Odd"}, "item": {"value": "http://www.wikidata.org/entity/Q1"}, "nf": {"value": "x"}, "tmdbm": {"value": "1"}}]}}`)
	}))
	defer srv.Close()
	c := NewWikidataSPARQL(time.Second)
	c.endpoint = srv.URL

	got, err := c.FindNetflixTitles(context.Background(), []string{"Arcane", "Raw", `Bad" } DROP`, "Odd"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, `"Arcane"@en`) || !strings.Contains(query, "wdt:P1874") || strings.Contains(query, "DROP") {
		t.Fatalf("query = %s", query)
	}
	if a := got["Arcane"]; len(a) != 1 || a[0].TmdbKind != "tv" || a[0].TmdbID != "94605" || a[0].NetflixID != "81435684" || a[0].WikidataID != "Q85518571" || a[0].Year != 2021 {
		t.Fatalf("arcane = %+v", a)
	}
	if r := got["Raw"]; len(r) != 1 || r[0].TmdbKind != "movie" || r[0].Year != 2016 {
		t.Fatalf("raw = %+v", r)
	}
	if len(got["Odd"]) != 0 {
		t.Fatalf("non-numeric netflix id accepted: %+v", got["Odd"])
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

func TestNetflixTop10Search(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/graphql":
			b, _ := io.ReadAll(r.Body)
			body = string(b)
			if r.Header.Get("Cookie") != "" {
				t.Error("sent cookies")
			}
			_, _ = io.WriteString(w, `{"data":{"tudumTop10Search":{"items":[
			  {"title":"Wednesday: Season 2","titleId":81435630,"category":"TV",
			   "globalData":{"weeks":[{"endDate":"2025-08-10"}]},"countryData":[{"country":"US","weeks":[{"endDate":"2025-08-17"}]}]},
			  {"title":"Bad","titleId":"x","category":"FILMS","globalData":{"weeks":[]},"countryData":[]}]}}}`)
		case r.URL.Path == "/title/81435630":
			http.Redirect(w, r, "https://www.netflix.com/title/81231974", http.StatusMovedPermanently)
		case r.URL.Path == "/title/81757047":
			_, _ = io.WriteString(w, "<html></html>")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := NewNetflixTop10Search(time.Second, "qid", 7)
	c.endpoint, c.titleBase = srv.URL+"/graphql", srv.URL+"/title/"

	items, err := c.Search(context.Background(), "Wednesday")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"searchTerm":"Wednesday"`) || !strings.Contains(body, `"id":"qid"`) || !strings.Contains(body, `"version":7`) {
		t.Fatalf("request = %s", body)
	}
	if len(items) != 1 || items[0].NetflixID != "81435630" || items[0].Category != "TV" || strings.Join(items[0].Weeks(), ",") != "2025-08-10,2025-08-17" {
		t.Fatalf("items = %+v", items)
	}
	if id, err := c.ShowID(context.Background(), "81435630"); err != nil || id != "81231974" {
		t.Fatalf("season → show = %q %v", id, err)
	}
	if id, err := c.ShowID(context.Background(), "81757047"); err != nil || id != "81757047" {
		t.Fatalf("film = %q %v", id, err)
	}
	if _, err := c.ShowID(context.Background(), "1/../x"); err == nil {
		t.Fatal("non-numeric id accepted")
	}
}
