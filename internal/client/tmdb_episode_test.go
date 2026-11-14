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

func TestTMDBTVEpisode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/3/tv/100/season/1/episode/2" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("api_key") != "k" {
			t.Errorf("missing api key")
		}
		var guests []string
		for i := 0; i < 15; i++ {
			guests = append(guests, fmt.Sprintf(`{"id": %d, "name": "Guest %d", "character": "Role", "profile_path": "/g.jpg"}`, i, i))
		}
		fmt.Fprintf(w, `{"id": 9, "name": " Two Wolves ", "overview": " A plot. ", "air_date": "2024-11-24", "episode_type": "standard",
		  "runtime": 63, "still_path": "/s.jpg", "vote_average": 7.4, "vote_count": 120,
		  "crew": [{"job": "Director", "name": "Anna Foerster"}, {"job": "Writer", "name": "Alison Schapker"},
		           {"job": "Writer", "name": "Alison Schapker"}, {"job": "Editor", "name": "Someone"}, {"job": "Teleplay", "name": "Diane Ademu-John"}],
		  "guest_stars": [%s]}`, strings.Join(guests, ","))
	}))
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	c := NewTMDB(resty.New().SetTransport(toServer{base}), "k")

	e, err := c.TVEpisode(context.Background(), "100", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "Two Wolves" || e.Overview != "A plot." || e.SeasonNumber != 1 || e.EpisodeNumber != 2 || e.Runtime != 63 || e.StillPath != "/s.jpg" || e.VoteAverage != 7.4 {
		t.Fatalf("episode = %+v", e)
	}
	if strings.Join(e.Directors, ",") != "Anna Foerster" || strings.Join(e.Writers, ",") != "Alison Schapker,Diane Ademu-John" {
		t.Fatalf("crew = %v / %v", e.Directors, e.Writers)
	}
	if len(e.GuestStars) != maxGuestStars || e.GuestStars[0].Name != "Guest 0" {
		t.Fatalf("guests = %d", len(e.GuestStars))
	}
	if got, err := c.TVEpisode(context.Background(), "100", 9, 9); got != nil || err != nil {
		t.Fatalf("unknown episode = %v, %v; want nil, nil", got, err)
	}
	if _, err := c.TVEpisode(context.Background(), "x1", 1, 1); err == nil {
		t.Fatal("want an error for a bad id")
	}
}
