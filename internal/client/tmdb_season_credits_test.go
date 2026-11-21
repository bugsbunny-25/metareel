package client

import "testing"

func TestParseShowSeasonCredits(t *testing.T) {
	body := []byte(`{"name": "Show", "poster_path": "/show.jpg",
	  "seasons": [{"season_number": 2, "name": "Season 2", "air_date": "2021-01-01", "episode_count": 8, "poster_path": "/s2.jpg"},
	              {"season_number": 1, "name": "Season 1", "air_date": "2020-01-01", "episode_count": 10}],
	  "season/1/aggregate_credits": {"cast": [{"id": 7, "roles": [{"character": " Eleven ", "episode_count": 8}]}],
	                                 "crew": [{"id": 9, "department": "Writing", "jobs": [{"job": "Writer", "episode_count": 2}]}]},
	  "season/2/aggregate_credits": {"cast": [{"id": 7, "roles": [{"character": "Eleven", "episode_count": 9}]}], "crew": []}}`)
	got, err := parseShowSeasonCredits(5, body, []int{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Show" || len(got.Seasons) != 2 || got.Seasons[0].Number != 1 || got.Seasons[1].PosterPath != "/s2.jpg" {
		t.Fatalf("show = %+v", got)
	}
	if p := got.People[7]; len(p) != 2 || p[0].Character != "Eleven" || p[0].Department != "Acting" || p[1].EpisodeCount != 9 {
		t.Fatalf("cast = %+v", p)
	}
	if p := got.People[9]; len(p) != 1 || p[0].Job != "Writer" || p[0].Department != "Writing" || p[0].Season != 1 {
		t.Fatalf("crew = %+v", p)
	}
}
