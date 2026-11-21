package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// TMDBShowSeasons is a TV show's seasons with who is credited in each:
// /tv/{id} with season/{n}/aggregate_credits appended (guest stars included).
type TMDBShowSeasons struct {
	ID         int
	Name       string
	PosterPath string
	Seasons    []TMDBShowSeason // every season TMDB lists, by number
	// People maps a person ID to their credits per season (seasons fetched
	// only; see Fetched).
	People map[int64][]TMDBSeasonCredit
	// Fetched are the season numbers whose credits were asked for.
	Fetched []int
}

// TMDBShowSeason is one season of a show.
type TMDBShowSeason struct {
	Number       int
	Name         string
	AirDate      string // YYYY-MM-DD, "" if not announced
	EpisodeCount int
	PosterPath   string
}

// TMDBSeasonCredit is what one person did in one season: cast roles
// (Character set) and crew jobs, each with its episode count.
type TMDBSeasonCredit struct {
	Season       int
	Character    string // cast; "" for crew
	Job          string // crew; "" for cast
	Department   string // "Acting" for cast
	EpisodeCount int
}

// maxSeasonAppends is TMDB's append_to_response limit (20 items).
const maxSeasonAppends = 20

type tmdbAggregateCredits struct {
	Cast []struct {
		ID    int `json:"id"`
		Roles []struct {
			Character    string `json:"character"`
			EpisodeCount int    `json:"episode_count"`
		} `json:"roles"`
	} `json:"cast"`
	Crew []struct {
		ID         int    `json:"id"`
		Department string `json:"department"`
		Jobs       []struct {
			Job          string `json:"job"`
			EpisodeCount int    `json:"episode_count"`
		} `json:"jobs"`
	} `json:"crew"`
}

// ShowSeasonCredits fetches a show's season list and the aggregate credits
// of the given seasons (at most 20; seasons TMDB does not have are skipped).
// It returns nil, nil when TMDB has no such show or no API key is configured.
func (c *TMDBClient) ShowSeasonCredits(ctx context.Context, tvID int, seasons []int) (*TMDBShowSeasons, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if tvID <= 0 {
		return nil, fmt.Errorf("invalid tmdb tv id")
	}
	if len(seasons) == 0 || len(seasons) > maxSeasonAppends {
		return nil, fmt.Errorf("ask for 1-%d seasons", maxSeasonAppends)
	}
	appends := make([]string, 0, len(seasons))
	for _, n := range seasons {
		appends = append(appends, fmt.Sprintf("season/%d/aggregate_credits", n))
	}
	// A long-running show's season credits are large (every guest star).
	resp, err := c.http.R().SetContext(ctx).
		SetQueryParams(map[string]string{"api_key": c.apiKey, "append_to_response": strings.Join(appends, ",")}).
		SetResponseBodyLimit(16 << 20).
		Get(fmt.Sprintf("https://api.themoviedb.org/3/tv/%d", tvID))
	if err != nil {
		return nil, fmt.Errorf("tmdb show season credits: %w", redactErr(err))
	}
	if resp.StatusCode() == 404 {
		return nil, nil
	}
	if resp.IsError() {
		return nil, fmt.Errorf("tmdb show season credits status %d", resp.StatusCode())
	}
	return parseShowSeasonCredits(tvID, resp.Body(), seasons)
}

func parseShowSeasonCredits(tvID int, body []byte, seasons []int) (*TMDBShowSeasons, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("tmdb show season credits: decode: %w", err)
	}
	var base struct {
		Name       string `json:"name"`
		PosterPath string `json:"poster_path"`
		Seasons    []struct {
			Number       int    `json:"season_number"`
			Name         string `json:"name"`
			AirDate      string `json:"air_date"`
			EpisodeCount int    `json:"episode_count"`
			PosterPath   string `json:"poster_path"`
		} `json:"seasons"`
	}
	if err := json.Unmarshal(body, &base); err != nil {
		return nil, fmt.Errorf("tmdb show season credits: decode: %w", err)
	}
	out := &TMDBShowSeasons{ID: tvID, Name: strings.TrimSpace(base.Name), PosterPath: base.PosterPath,
		People: map[int64][]TMDBSeasonCredit{}, Fetched: append([]int(nil), seasons...)}
	for _, s := range base.Seasons {
		out.Seasons = append(out.Seasons, TMDBShowSeason{Number: s.Number, Name: strings.TrimSpace(s.Name), AirDate: s.AirDate,
			EpisodeCount: s.EpisodeCount, PosterPath: s.PosterPath})
	}
	sort.Slice(out.Seasons, func(i, j int) bool { return out.Seasons[i].Number < out.Seasons[j].Number })
	for _, n := range seasons {
		msg, ok := raw["season/"+strconv.Itoa(n)+"/aggregate_credits"]
		if !ok {
			continue
		}
		var agg tmdbAggregateCredits
		if err := json.Unmarshal(msg, &agg); err != nil {
			return nil, fmt.Errorf("tmdb season %d credits: decode: %w", n, err)
		}
		for _, p := range agg.Cast {
			for _, r := range p.Roles {
				out.People[int64(p.ID)] = append(out.People[int64(p.ID)], TMDBSeasonCredit{Season: n, Character: strings.TrimSpace(r.Character),
					Department: "Acting", EpisodeCount: r.EpisodeCount})
			}
		}
		for _, p := range agg.Crew {
			for _, j := range p.Jobs {
				out.People[int64(p.ID)] = append(out.People[int64(p.ID)], TMDBSeasonCredit{Season: n, Job: strings.TrimSpace(j.Job),
					Department: p.Department, EpisodeCount: j.EpisodeCount})
			}
		}
	}
	return out, nil
}
