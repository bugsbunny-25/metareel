package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// TMDBCredit is one person's credit on a title: a cast role, or a crew job.
type TMDBCredit struct {
	PersonID    int
	Name        string
	ProfilePath string
	KnownFor    string // known_for_department
	Department  string // Acting for cast
	Job         string // "" for cast
	Character   string // cast
	Order       int    // cast billing order; crew: TMDB's order
	Episodes    int    // TV: episodes credited (0 for movies)
}

// TMDBCredits is a title's cast and crew.
type TMDBCredits struct {
	Cast []TMDBCredit
	Crew []TMDBCredit
}

type tmdbPerson struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	ProfilePath string `json:"profile_path"`
	KnownFor    string `json:"known_for_department"`
}

type tmdbMovieCredits struct {
	Cast []struct {
		tmdbPerson
		Character string `json:"character"`
		Order     int    `json:"order"`
	} `json:"cast"`
	Crew []struct {
		tmdbPerson
		Department string `json:"department"`
		Job        string `json:"job"`
	} `json:"crew"`
}

type tmdbTVCredits struct {
	CreatedBy        []tmdbPerson `json:"created_by"`
	AggregateCredits struct {
		Cast []struct {
			tmdbPerson
			Order int `json:"order"`
			Roles []struct {
				Character    string `json:"character"`
				EpisodeCount int    `json:"episode_count"`
			} `json:"roles"`
			TotalEpisodes int `json:"total_episode_count"`
		} `json:"cast"`
		Crew []struct {
			tmdbPerson
			Department string `json:"department"`
			Jobs       []struct {
				Job          string `json:"job"`
				EpisodeCount int    `json:"episode_count"`
			} `json:"jobs"`
		} `json:"crew"`
	} `json:"aggregate_credits"`
}

// Credits fetches a title's cast and crew: /movie/{id}/credits, or for TV
// the show's aggregate credits (every season) with its creators (job
// "Creator"). It returns nil, nil when TMDB has no such title or no API key
// is configured.
func (c *TMDBClient) Credits(ctx context.Context, kind, tmdbID string) (*TMDBCredits, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if !isDigits(tmdbID) {
		return nil, fmt.Errorf("invalid tmdb id")
	}
	var url string
	params := map[string]string{"api_key": c.apiKey}
	switch kind {
	case "movie":
		url = fmt.Sprintf("https://api.themoviedb.org/3/movie/%s/credits", tmdbID)
	case "tv":
		url = fmt.Sprintf("https://api.themoviedb.org/3/tv/%s", tmdbID)
		params["append_to_response"] = "aggregate_credits"
	default:
		return nil, fmt.Errorf("invalid tmdb kind %q", kind)
	}
	// A long-running show's aggregate credits can be large; 8 MB is plenty.
	resp, err := c.http.R().SetContext(ctx).SetQueryParams(params).SetResponseBodyLimit(8 << 20).Get(url)
	if err != nil {
		return nil, fmt.Errorf("tmdb %s credits: %w", kind, redactErr(err))
	}
	if resp.StatusCode() == 404 {
		return nil, nil
	}
	if resp.IsError() {
		return nil, fmt.Errorf("tmdb %s credits status %d", kind, resp.StatusCode())
	}
	out := &TMDBCredits{}
	clean := func(s string) string { return strings.TrimSpace(s) }
	if kind == "movie" {
		var m tmdbMovieCredits
		if err := json.Unmarshal(resp.Body(), &m); err != nil {
			return nil, fmt.Errorf("tmdb movie credits: decode: %w", err)
		}
		for _, x := range m.Cast {
			out.Cast = append(out.Cast, TMDBCredit{PersonID: x.ID, Name: clean(x.Name), ProfilePath: x.ProfilePath, KnownFor: x.KnownFor,
				Department: "Acting", Character: clean(x.Character), Order: x.Order})
		}
		for i, x := range m.Crew {
			out.Crew = append(out.Crew, TMDBCredit{PersonID: x.ID, Name: clean(x.Name), ProfilePath: x.ProfilePath, KnownFor: x.KnownFor,
				Department: x.Department, Job: x.Job, Order: i})
		}
		return out, nil
	}
	var t tmdbTVCredits
	if err := json.Unmarshal(resp.Body(), &t); err != nil {
		return nil, fmt.Errorf("tmdb tv credits: decode: %w", err)
	}
	for i, p := range t.CreatedBy {
		out.Crew = append(out.Crew, TMDBCredit{PersonID: p.ID, Name: clean(p.Name), ProfilePath: p.ProfilePath, KnownFor: p.KnownFor,
			Department: "Writing", Job: "Creator", Order: i})
	}
	for _, x := range t.AggregateCredits.Cast {
		cr := TMDBCredit{PersonID: x.ID, Name: clean(x.Name), ProfilePath: x.ProfilePath, KnownFor: x.KnownFor, Department: "Acting",
			Order: x.Order, Episodes: x.TotalEpisodes}
		// The role with the most episodes names the character.
		best := -1
		for _, r := range x.Roles {
			if r.EpisodeCount > best {
				best, cr.Character = r.EpisodeCount, clean(r.Character)
			}
		}
		out.Cast = append(out.Cast, cr)
	}
	for i, x := range t.AggregateCredits.Crew {
		for _, j := range x.Jobs {
			out.Crew = append(out.Crew, TMDBCredit{PersonID: x.ID, Name: clean(x.Name), ProfilePath: x.ProfilePath, KnownFor: x.KnownFor,
				Department: x.Department, Job: j.Job, Order: len(t.CreatedBy) + i, Episodes: j.EpisodeCount})
		}
	}
	return out, nil
}
