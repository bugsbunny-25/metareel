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

func TestTMDBPerson(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/person/31":
			if r.URL.Query().Get("append_to_response") != "combined_credits,external_ids" {
				t.Errorf("append = %q", r.URL.Query().Get("append_to_response"))
			}
			fmt.Fprint(w, `{"id": 31, "name": " Tom Hanks ", "biography": "Actor.\n", "birthday": "1956-07-09", "deathday": null,
			  "place_of_birth": "Concord, California, USA", "profile_path": "/t.jpg", "known_for_department": "Acting", "homepage": null,
			  "also_known_as": ["Thomas Jeffrey Hanks"], "external_ids": {"imdb_id": "nm0000158"},
			  "combined_credits": {
			    "cast": [{"id": 13, "media_type": "movie", "title": "Forrest Gump", "release_date": "1994-06-23", "character": "Forrest", "poster_path": "/f.jpg", "vote_average": 8.5, "vote_count": 27000, "popularity": 40},
			             {"id": 4, "media_type": "tv", "name": "Band of Brothers", "first_air_date": "2001-09-09", "character": "Self", "episode_count": 1, "department": "Crew"},
			             {"id": 9, "media_type": "person", "name": "odd"}],
			    "crew": [{"id": 4, "media_type": "tv", "name": "Band of Brothers", "first_air_date": "2001-09-09", "department": "Production", "job": "Executive Producer", "episode_count": 10}]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	c := NewTMDB(resty.New().SetTransport(toServer{base}), "k")
	ctx := context.Background()

	p, err := c.Person(ctx, 31)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Tom Hanks" || p.Biography != "Actor." || p.Birthday != "1956-07-09" || p.Deathday != "" || p.IMDbID != "nm0000158" ||
		p.KnownFor != "Acting" || len(p.AlsoKnownAs) != 1 || len(p.Credits) != 3 {
		t.Fatalf("person = %+v", p)
	}
	if m := p.Credits[0]; m != (TMDBPersonCredit{Kind: "movie", ID: 13, Title: "Forrest Gump", Date: "1994-06-23", PosterPath: "/f.jpg", Character: "Forrest",
		Department: "Acting", Popularity: 40, VoteAverage: 8.5, VoteCount: 27000}) {
		t.Fatalf("movie credit = %+v", m)
	}
	if tv := p.Credits[1]; tv.Kind != "tv" || tv.Title != "Band of Brothers" || tv.Date != "2001-09-09" || tv.Department != "Acting" || tv.EpisodeCount != 1 {
		t.Fatalf("tv cast credit = %+v", tv)
	}
	if crew := p.Credits[2]; crew.Job != "Executive Producer" || crew.Department != "Production" || crew.Character != "" {
		t.Fatalf("crew credit = %+v", crew)
	}
	if got, err := c.Person(ctx, 404); got != nil || err != nil {
		t.Fatalf("unknown = %+v, %v", got, err)
	}
	if got, err := NewTMDB(resty.New(), "").Person(ctx, 31); got != nil || err != nil {
		t.Fatalf("no key = %+v, %v", got, err)
	}
}
