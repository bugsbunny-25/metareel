package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// TMDBPersonDetails is a person's TMDB profile with every credit they have:
// /person/{id} with combined_credits and external_ids appended.
type TMDBPersonDetails struct {
	ID           int
	Name         string
	Biography    string
	Birthday     string // YYYY-MM-DD, "" if unknown
	Deathday     string
	PlaceOfBirth string
	ProfilePath  string
	KnownFor     string // known_for_department
	Homepage     string
	IMDbID       string
	AlsoKnownAs  []string
	Credits      []TMDBPersonCredit
}

// TMDBPersonCredit is one of a person's credits: a cast role (Department
// "Acting", Character set) or a crew job.
type TMDBPersonCredit struct {
	Kind         string // movie | tv
	ID           int
	Title        string
	Date         string // release date or first air date, "" if none yet
	PosterPath   string
	Character    string
	Job          string
	Department   string
	EpisodeCount int
	Popularity   float64
	VoteAverage  float64
	VoteCount    int
}

type tmdbPersonCredit struct {
	ID           int     `json:"id"`
	MediaType    string  `json:"media_type"`
	Title        string  `json:"title"`
	Name         string  `json:"name"`
	ReleaseDate  string  `json:"release_date"`
	FirstAirDate string  `json:"first_air_date"`
	PosterPath   string  `json:"poster_path"`
	Character    string  `json:"character"`
	Job          string  `json:"job"`
	Department   string  `json:"department"`
	EpisodeCount int     `json:"episode_count"`
	Popularity   float64 `json:"popularity"`
	VoteAverage  float64 `json:"vote_average"`
	VoteCount    int     `json:"vote_count"`
}

type tmdbPersonResp struct {
	ID              int      `json:"id"`
	Name            string   `json:"name"`
	Biography       string   `json:"biography"`
	Birthday        string   `json:"birthday"`
	Deathday        string   `json:"deathday"`
	PlaceOfBirth    string   `json:"place_of_birth"`
	ProfilePath     string   `json:"profile_path"`
	KnownFor        string   `json:"known_for_department"`
	Homepage        string   `json:"homepage"`
	AlsoKnownAs     []string `json:"also_known_as"`
	CombinedCredits struct {
		Cast []tmdbPersonCredit `json:"cast"`
		Crew []tmdbPersonCredit `json:"crew"`
	} `json:"combined_credits"`
	ExternalIDs struct {
		IMDbID string `json:"imdb_id"`
	} `json:"external_ids"`
}

// Person fetches a person's profile and every movie and TV credit in one
// request. It returns nil, nil when TMDB has no such person or no API key is
// configured.
func (c *TMDBClient) Person(ctx context.Context, id int) (*TMDBPersonDetails, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if id <= 0 {
		return nil, fmt.Errorf("invalid tmdb person id")
	}
	// A prolific actor's combined credits run to thousands of rows; 8 MB is plenty.
	resp, err := c.http.R().SetContext(ctx).
		SetQueryParams(map[string]string{"api_key": c.apiKey, "append_to_response": "combined_credits,external_ids"}).
		SetResponseBodyLimit(8 << 20).
		Get(fmt.Sprintf("https://api.themoviedb.org/3/person/%d", id))
	if err != nil {
		return nil, fmt.Errorf("tmdb person: %w", redactErr(err))
	}
	if resp.StatusCode() == 404 {
		return nil, nil
	}
	if resp.IsError() {
		return nil, fmt.Errorf("tmdb person status %d", resp.StatusCode())
	}
	var p tmdbPersonResp
	if err := json.Unmarshal(resp.Body(), &p); err != nil {
		return nil, fmt.Errorf("tmdb person: decode: %w", err)
	}
	clean := strings.TrimSpace
	out := &TMDBPersonDetails{
		ID: p.ID, Name: clean(p.Name), Biography: clean(p.Biography), Birthday: p.Birthday, Deathday: p.Deathday,
		PlaceOfBirth: clean(p.PlaceOfBirth), ProfilePath: p.ProfilePath, KnownFor: p.KnownFor,
		Homepage: clean(p.Homepage), IMDbID: p.ExternalIDs.IMDbID, AlsoKnownAs: p.AlsoKnownAs,
	}
	add := func(x tmdbPersonCredit, cast bool) {
		if x.MediaType != "movie" && x.MediaType != "tv" {
			return
		}
		cr := TMDBPersonCredit{
			Kind: x.MediaType, ID: x.ID, Title: clean(x.Title), Date: x.ReleaseDate, PosterPath: x.PosterPath,
			Job: clean(x.Job), Department: x.Department, EpisodeCount: x.EpisodeCount,
			Popularity: x.Popularity, VoteAverage: x.VoteAverage, VoteCount: x.VoteCount,
		}
		if x.MediaType == "tv" {
			cr.Title, cr.Date = clean(x.Name), x.FirstAirDate
		}
		if cast {
			cr.Department, cr.Character, cr.Job = "Acting", clean(x.Character), ""
		}
		out.Credits = append(out.Credits, cr)
	}
	for _, x := range p.CombinedCredits.Cast {
		add(x, true)
	}
	for _, x := range p.CombinedCredits.Crew {
		add(x, false)
	}
	return out, nil
}
