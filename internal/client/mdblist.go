package client

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// MDBListClient reads ratings from the MDBList API (https://api.mdblist.com/docs/).
// The free tier allows 1,000 requests per day.
type MDBListClient struct {
	http   *resty.Client
	apiKey string
}

// NewMDBList returns nil when apiKey is empty.
func NewMDBList(apiKey string, timeout time.Duration) *MDBListClient {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil
	}
	return &MDBListClient{
		http:   resty.New().SetTimeout(timeout).SetBaseURL("https://api.mdblist.com").SetHeader("Accept", "application/json"),
		apiKey: apiKey,
	}
}

// MDBListMedia is the subset of GET /tmdb/{movie|show}/{id} we use.
type MDBListMedia struct {
	Title        string          `json:"title"`
	Year         int             `json:"year"`
	ImdbID       string          `json:"imdb_id"`
	Score        *float64        `json:"score"`         // MDBList's own score, 0-100
	ScoreAverage *float64        `json:"score_average"` // average of all sources, 0-100
	Ratings      []MDBListRating `json:"ratings"`
}

// MDBListRating is one rating source. Value is on the source's own scale:
// imdb 0-10, tomatoes (critics) and popcorn / tomatoesaudience 0-100.
type MDBListRating struct {
	Source string   `json:"source"`
	Value  *float64 `json:"value"`
	Votes  *float64 `json:"votes"`
	URL    string   `json:"url"`
}

// Rating returns the first rating from any of sources, or nil.
func (m *MDBListMedia) Rating(sources ...string) *MDBListRating {
	for _, s := range sources {
		for i := range m.Ratings {
			if m.Ratings[i].Source == s && m.Ratings[i].Value != nil {
				return &m.Ratings[i]
			}
		}
	}
	return nil
}

// GetByTmdb looks a title up by TMDB ID; kind is "movie" or "tv". Returns nil
// if MDBList does not know the title.
func (c *MDBListClient) GetByTmdb(ctx context.Context, kind string, tmdbID string) (*MDBListMedia, error) {
	mediaType := "movie"
	if kind == "tv" {
		mediaType = "show"
	}
	var out MDBListMedia
	var apiErr struct {
		Error string `json:"error"`
	}
	resp, err := c.http.R().
		SetContext(ctx).
		SetQueryParam("apikey", c.apiKey).
		SetResult(&out).
		SetError(&apiErr).
		Get(fmt.Sprintf("/tmdb/%s/%s", mediaType, tmdbID))
	if err != nil {
		return nil, fmt.Errorf("mdblist get %s %s: %w", mediaType, tmdbID, redactErr(err))
	}
	switch {
	case resp.StatusCode() == http.StatusNotFound:
		return nil, nil
	case resp.StatusCode() == http.StatusTooManyRequests:
		return nil, fmt.Errorf("mdblist rate limited (%s), retry after %ss", apiErr.Error, resp.Header().Get("Retry-After"))
	case resp.IsError():
		return nil, fmt.Errorf("mdblist get %s %s status %d: %s", mediaType, tmdbID, resp.StatusCode(), apiErr.Error)
	}
	return &out, nil
}
