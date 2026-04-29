package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-resty/resty/v2"
)

// TMDBClient is a small wrapper around the TMDB API, used to map scraped titles
// to TMDB + IMDb IDs.
type TMDBClient struct {
	http   *resty.Client
	apiKey string
}

func NewTMDB(httpClient *resty.Client, apiKey string) *TMDBClient {
	return &TMDBClient{http: httpClient, apiKey: strings.TrimSpace(apiKey)}
}

func (c *TMDBClient) Enabled() bool { return c != nil && c.apiKey != "" }

type tmdbSearchResp struct {
	Results []struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
	} `json:"results"`
}

func (c *TMDBClient) SearchMovie(ctx context.Context, query string) (int, string, error) {
	if !c.Enabled() {
		return 0, "", nil
	}
	var out tmdbSearchResp
	resp, err := c.http.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"api_key": c.apiKey,
			"query":   query,
		}).
		SetResult(&out).
		Get("https://api.themoviedb.org/3/search/movie")
	if err != nil {
		return 0, "", fmt.Errorf("tmdb search movie: %w", err)
	}
	if resp.IsError() {
		return 0, "", fmt.Errorf("tmdb search movie status %d", resp.StatusCode())
	}
	if len(out.Results) == 0 {
		return 0, "", nil
	}
	return out.Results[0].ID, out.Results[0].Title, nil
}

func (c *TMDBClient) SearchTV(ctx context.Context, query string) (int, string, error) {
	if !c.Enabled() {
		return 0, "", nil
	}
	var out tmdbSearchResp
	resp, err := c.http.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"api_key": c.apiKey,
			"query":   query,
		}).
		SetResult(&out).
		Get("https://api.themoviedb.org/3/search/tv")
	if err != nil {
		return 0, "", fmt.Errorf("tmdb search tv: %w", err)
	}
	if resp.IsError() {
		return 0, "", fmt.Errorf("tmdb search tv status %d", resp.StatusCode())
	}
	if len(out.Results) == 0 {
		return 0, "", nil
	}
	return out.Results[0].ID, out.Results[0].Title, nil
}

type tmdbExternalIDs struct {
	IMDbID     string `json:"imdb_id"`
	WikidataID string `json:"wikidata_id"`
}

func (c *TMDBClient) MovieExternalIDs(ctx context.Context, tmdbID int) (string, string, error) {
	if !c.Enabled() || tmdbID == 0 {
		return "", "", nil
	}
	var out tmdbExternalIDs
	resp, err := c.http.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{"api_key": c.apiKey}).
		SetResult(&out).
		Get(fmt.Sprintf("https://api.themoviedb.org/3/movie/%d/external_ids", tmdbID))
	if err != nil {
		return "", "", fmt.Errorf("tmdb movie external ids: %w", err)
	}
	if resp.IsError() {
		return "", "", fmt.Errorf("tmdb movie external ids status %d", resp.StatusCode())
	}
	return strings.TrimSpace(out.IMDbID), strings.TrimSpace(out.WikidataID), nil
}

func (c *TMDBClient) TVExternalIDs(ctx context.Context, tmdbID int) (string, string, error) {
	if !c.Enabled() || tmdbID == 0 {
		return "", "", nil
	}
	var out tmdbExternalIDs
	resp, err := c.http.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{"api_key": c.apiKey}).
		SetResult(&out).
		Get(fmt.Sprintf("https://api.themoviedb.org/3/tv/%d/external_ids", tmdbID))
	if err != nil {
		return "", "", fmt.Errorf("tmdb tv external ids: %w", err)
	}
	if resp.IsError() {
		return "", "", fmt.Errorf("tmdb tv external ids status %d", resp.StatusCode())
	}
	return strings.TrimSpace(out.IMDbID), strings.TrimSpace(out.WikidataID), nil
}
