package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-resty/resty/v2"
)

func TestTMDBCredits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/movie/1/credits":
			fmt.Fprint(w, `{"cast": [{"id": 10, "name": " Ryan Gosling ", "character": "Ryland Grace", "order": 0, "known_for_department": "Acting", "profile_path": "/r.jpg"}],
			  "crew": [{"id": 20, "name": "Phil Lord", "department": "Directing", "job": "Director"}, {"id": 21, "name": "Drew", "department": "Writing", "job": "Screenplay"}]}`)
		case "/3/tv/2":
			if r.URL.Query().Get("append_to_response") != "aggregate_credits" {
				t.Errorf("tv append = %q", r.URL.Query().Get("append_to_response"))
			}
			fmt.Fprint(w, `{"name": "BEEF", "created_by": [{"id": 30, "name": "Lee Sung Jin"}],
			  "aggregate_credits": {"cast": [{"id": 40, "name": "Steven Yeun", "order": 0, "total_episode_count": 10,
			    "roles": [{"character": "Cameo", "episode_count": 1}, {"character": "Danny Cho", "episode_count": 10}]}],
			  "crew": [{"id": 30, "name": "Lee Sung Jin", "department": "Directing", "jobs": [{"job": "Director", "episode_count": 3}, {"job": "Writer", "episode_count": 10}]}]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	c := NewTMDB(resty.New().SetTransport(toServer{base}), "k")
	ctx := context.Background()

	m, err := c.Credits(ctx, "movie", "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Cast) != 1 || m.Cast[0] != (TMDBCredit{PersonID: 10, Name: "Ryan Gosling", ProfilePath: "/r.jpg", KnownFor: "Acting", Department: "Acting", Character: "Ryland Grace"}) ||
		len(m.Crew) != 2 || m.Crew[1].Job != "Screenplay" || m.Crew[1].Order != 1 {
		t.Fatalf("movie credits = %+v", m)
	}
	tv, err := c.Credits(ctx, "tv", "2")
	if err != nil {
		t.Fatal(err)
	}
	if len(tv.Cast) != 1 || tv.Cast[0].Character != "Danny Cho" || tv.Cast[0].Episodes != 10 {
		t.Fatalf("tv cast = %+v", tv.Cast)
	}
	// The creator first, then each crew job.
	if len(tv.Crew) != 3 || tv.Crew[0].Job != "Creator" || tv.Crew[1].Job != "Director" || tv.Crew[1].Episodes != 3 || tv.Crew[2].Job != "Writer" {
		t.Fatalf("tv crew = %+v", tv.Crew)
	}
	if got, err := c.Credits(ctx, "movie", "404"); got != nil || err != nil {
		t.Fatalf("unknown = %+v, %v", got, err)
	}
	if _, err := c.Credits(ctx, "movie", "1/../x"); err == nil {
		t.Fatal("non-numeric id accepted")
	}
	if _, err := c.Credits(ctx, "person", "1"); err == nil {
		t.Fatal("bad kind accepted")
	}
}
