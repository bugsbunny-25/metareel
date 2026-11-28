package client

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"strconv"
	"strings"
)

// TMDBListItem is one title on a TMDB list (trending, now playing): enough to
// draw a poster row and open the title.
type TMDBListItem struct {
	Kind         string  `json:"tmdb_kind"` // movie | tv
	ID           string  `json:"tmdb_id"`   // as everywhere else in the API
	Title        string  `json:"title"`
	PosterPath   string  `json:"poster_path,omitempty"`
	BackdropPath string  `json:"backdrop_path,omitempty"`
	ReleaseDate  string  `json:"release_date,omitempty"` // first air date for TV
	Overview     string  `json:"overview,omitempty"`
	VoteAverage  float64 `json:"vote_average"`
	VoteCount    int     `json:"vote_count"`
	// SeasonCount (TV) comes from metareel's stored TMDB details, so it is
	// set only for shows metareel knows; the list itself has no seasons.
	SeasonCount *int64 `json:"season_count,omitempty"`
	// LogoPath and Certification (In cinemas: the country's) also come from
	// stored details, for titles metareel knows.
	LogoPath      *string `json:"logo_path,omitempty"`
	Certification *string `json:"certification,omitempty"`
}

type tmdbListResp struct {
	Results []struct {
		ID           int     `json:"id"`
		MediaType    string  `json:"media_type"` // trending/all only
		Title        string  `json:"title"`
		Name         string  `json:"name"`
		PosterPath   string  `json:"poster_path"`
		BackdropPath string  `json:"backdrop_path"`
		Overview     string  `json:"overview"`
		ReleaseDate  string  `json:"release_date"`
		FirstAirDate string  `json:"first_air_date"`
		VoteAverage  float64 `json:"vote_average"`
		VoteCount    int     `json:"vote_count"`
		Adult        bool    `json:"adult"`
	} `json:"results"`
}

var tmdbRegionRe = regexp.MustCompile(`^[A-Z]{2}$`)

// Trending returns TMDB's trending titles: kind all | movie | tv, window day | week.
// People in "all" are dropped. Nil, nil without an API key.
func (c *TMDBClient) Trending(ctx context.Context, kind, window string) ([]TMDBListItem, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if kind != "all" && kind != "movie" && kind != "tv" {
		return nil, fmt.Errorf("invalid trending kind %q", kind)
	}
	if window != "day" && window != "week" {
		return nil, fmt.Errorf("invalid trending window %q", window)
	}
	return c.list(ctx, "trending", fmt.Sprintf("https://api.themoviedb.org/3/trending/%s/%s", kind, window), kind, nil)
}

// NowPlaying returns the films TMDB lists as in cinemas in a country (ISO
// 3166-1 region), first page. Nil, nil without an API key.
func (c *TMDBClient) NowPlaying(ctx context.Context, region string) ([]TMDBListItem, error) {
	if !c.Enabled() {
		return nil, nil
	}
	region = strings.ToUpper(region)
	if !tmdbRegionRe.MatchString(region) {
		return nil, fmt.Errorf("invalid region %q", region)
	}
	return c.list(ctx, "now playing", "https://api.themoviedb.org/3/movie/now_playing", "movie", map[string]string{"region": region})
}

func (c *TMDBClient) list(ctx context.Context, what, url, kind string, params map[string]string) ([]TMDBListItem, error) {
	q := map[string]string{"api_key": c.apiKey}
	maps.Copy(q, params)
	resp, err := c.http.R().SetContext(ctx).SetQueryParams(q).Get(url)
	if err != nil {
		return nil, fmt.Errorf("tmdb %s: %w", what, redactErr(err))
	}
	if resp.IsError() {
		return nil, fmt.Errorf("tmdb %s status %d", what, resp.StatusCode())
	}
	var raw tmdbListResp
	if err := json.Unmarshal(resp.Body(), &raw); err != nil {
		return nil, fmt.Errorf("decode tmdb %s: %w", what, err)
	}
	out := make([]TMDBListItem, 0, len(raw.Results))
	for _, r := range raw.Results {
		k := kind
		if k == "all" {
			k = r.MediaType
		}
		if (k != "movie" && k != "tv") || r.Adult || r.ID == 0 {
			continue
		}
		it := TMDBListItem{Kind: k, ID: strconv.Itoa(r.ID), PosterPath: r.PosterPath, BackdropPath: r.BackdropPath, Overview: strings.TrimSpace(r.Overview), VoteAverage: r.VoteAverage, VoteCount: r.VoteCount}
		if k == "movie" {
			it.Title, it.ReleaseDate = strings.TrimSpace(r.Title), r.ReleaseDate
		} else {
			it.Title, it.ReleaseDate = strings.TrimSpace(r.Name), r.FirstAirDate
		}
		out = append(out, it)
	}
	return out, nil
}
