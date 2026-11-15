package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// TMDBEpisodeDetail is one episode with what a details screen shows: the
// synopsis, a still, TMDB's score, who directed and wrote it, and its guest
// cast.
type TMDBEpisodeDetail struct {
	ID            int             `json:"id"`
	SeasonNumber  int             `json:"season_number"`
	EpisodeNumber int             `json:"episode_number"`
	Name          string          `json:"name"`
	Overview      string          `json:"overview"`
	AirDate       string          `json:"air_date"`
	EpisodeType   string          `json:"episode_type"`
	Runtime       int             `json:"runtime"`
	StillPath     string          `json:"still_path"`
	VoteAverage   float64         `json:"vote_average"`
	VoteCount     int             `json:"vote_count"`
	Directors     []string        `json:"directors"`
	Writers       []string        `json:"writers"`
	GuestStars    []TMDBGuestStar `json:"guest_stars"`
}

// TMDBGuestStar is one guest on an episode.
type TMDBGuestStar struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character"`
	ProfilePath string `json:"profile_path"`
}

// maxGuestStars caps the guest cast kept per episode (a details screen shows a row).
const maxGuestStars = 12

// TVEpisode fetches one episode of a show. It returns nil, nil when TMDB has
// no such episode or no API key is configured.
func (c *TMDBClient) TVEpisode(ctx context.Context, tvID string, season, episode int) (*TMDBEpisodeDetail, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if !isDigits(tvID) || season < 0 || episode < 1 {
		return nil, fmt.Errorf("invalid tmdb episode")
	}
	var raw struct {
		ID          int             `json:"id"`
		Name        string          `json:"name"`
		Overview    string          `json:"overview"`
		AirDate     string          `json:"air_date"`
		EpisodeType string          `json:"episode_type"`
		Runtime     int             `json:"runtime"`
		StillPath   string          `json:"still_path"`
		VoteAverage float64         `json:"vote_average"`
		VoteCount   int             `json:"vote_count"`
		GuestStars  []TMDBGuestStar `json:"guest_stars"`
		Crew        []struct {
			Name string `json:"name"`
			Job  string `json:"job"`
		} `json:"crew"`
	}
	resp, err := c.http.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{"api_key": c.apiKey}).
		Get(fmt.Sprintf("https://api.themoviedb.org/3/tv/%s/season/%d/episode/%d", tvID, season, episode))
	if err != nil {
		return nil, fmt.Errorf("tmdb tv episode: %w", redactErr(err))
	}
	if resp.StatusCode() == 404 {
		return nil, nil
	}
	if resp.IsError() {
		return nil, fmt.Errorf("tmdb tv episode status %d", resp.StatusCode())
	}
	if err := json.Unmarshal(resp.Body(), &raw); err != nil {
		return nil, fmt.Errorf("decode tmdb tv %s episode: %w", tvID, err)
	}
	d := TMDBEpisodeDetail{
		ID: raw.ID, SeasonNumber: season, EpisodeNumber: episode,
		Name: strings.TrimSpace(raw.Name), Overview: strings.TrimSpace(raw.Overview),
		AirDate: raw.AirDate, EpisodeType: raw.EpisodeType, Runtime: raw.Runtime, StillPath: raw.StillPath,
		VoteAverage: raw.VoteAverage, VoteCount: raw.VoteCount,
		Directors: []string{}, Writers: []string{},
	}
	seen := map[string]bool{}
	for _, m := range raw.Crew {
		key := m.Job + "|" + m.Name
		if seen[key] || strings.TrimSpace(m.Name) == "" {
			continue
		}
		seen[key] = true
		switch m.Job {
		case "Director":
			d.Directors = append(d.Directors, m.Name)
		case "Writer", "Teleplay", "Story", "Screenplay":
			d.Writers = append(d.Writers, m.Name)
		}
	}
	d.GuestStars = raw.GuestStars
	if d.GuestStars == nil {
		d.GuestStars = []TMDBGuestStar{}
	}
	if len(d.GuestStars) > maxGuestStars {
		d.GuestStars = d.GuestStars[:maxGuestStars]
	}
	return &d, nil
}
