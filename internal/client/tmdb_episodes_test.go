package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-resty/resty/v2"
)

// toServer sends every request to a test server, keeping path and query.
type toServer struct{ base *url.URL }

func (t toServer) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Scheme, r2.URL.Host = t.base.Scheme, t.base.Host
	return http.DefaultTransport.RoundTrip(r2)
}

func TestTMDBTVSeasonEpisodes(t *testing.T) {
	var appends []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/3/tv/404" || (r.URL.Path == "/3/tv/405" && strings.Contains(r.URL.Query().Get("append_to_response"), "season/21")) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("api_key") != "k" {
			t.Errorf("missing api key")
		}
		a := r.URL.Query().Get("append_to_response")
		appends = append(appends, a)
		var parts []string
		for _, s := range strings.Split(a, ",") {
			n := strings.TrimPrefix(s, "season/")
			if n == "3" {
				continue // TMDB leaves out seasons it doesn't have
			}
			parts = append(parts, fmt.Sprintf(`"season/%s": {"episodes": [{"episode_number": 1, "season_number": 99, "name": " Pilot ",
			  "air_date": "2026-01-0%s", "episode_type": "standard", "runtime": 50}, {"episode_number": 2, "air_date": null, "episode_type": "finale"}]}`, n, "1"))
		}
		fmt.Fprintf(w, `{"name": "Show", %s}`, strings.Join(parts, ","))
	}))
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	c := NewTMDB(resty.New().SetTransport(toServer{base}), "k")

	seasons := make([]int, 0, 22)
	for n := 1; n <= 22; n++ {
		seasons = append(seasons, n)
	}
	got, err := c.TVSeasonEpisodes(context.Background(), "100", seasons)
	if err != nil {
		t.Fatal(err)
	}
	if len(appends) != 2 || strings.Count(appends[0], "season/") != 20 || appends[1] != "season/21,season/22" {
		t.Fatalf("requests = %v, want 20 seasons then 2", appends)
	}
	if len(got) != 21 || got[3] != nil {
		t.Fatalf("seasons = %d (season 3: %v), want 21 without season 3", len(got), got[3])
	}
	e := got[2]
	if len(e) != 2 || e[0] != (TMDBEpisode{SeasonNumber: 2, EpisodeNumber: 1, Name: "Pilot", AirDate: "2026-01-01", EpisodeType: "standard", Runtime: 50}) ||
		e[1].AirDate != "" || e[1].EpisodeType != "finale" {
		t.Fatalf("season 2 = %+v", e)
	}
	if got, err := c.TVSeasonEpisodes(context.Background(), "404", []int{1}); got != nil || err != nil {
		t.Fatalf("unknown show = %v, %v; want nil, nil", got, err)
	}
	// A 404 on a later batch keeps the seasons already read.
	if got, err := c.TVSeasonEpisodes(context.Background(), "405", seasons); err != nil || len(got) != 19 {
		t.Fatalf("404 on the second batch = %d seasons, %v; want the first batch's 19", len(got), err)
	}
	if _, err := c.TVSeasonEpisodes(context.Background(), "1/../x", []int{1}); err == nil {
		t.Fatal("non-numeric id accepted")
	}
}
