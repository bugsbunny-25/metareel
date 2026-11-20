package client

import (
	"context"
	"fmt"
	"sort"
	"strconv"
)

// TMDBCollection is a TMDB movie collection (a franchise: "The Dark Knight
// Collection") with its films, oldest first. Box sets usually match one.
type TMDBCollection struct {
	ID         int
	Name       string
	PosterPath string
	Parts      []TMDBSearchResult
}

// SearchCollections finds collections by name; Parts is empty here.
func (c *TMDBClient) SearchCollections(ctx context.Context, query string) ([]TMDBCollection, error) {
	if !c.Enabled() {
		return nil, nil
	}
	var out struct {
		Results []struct {
			ID         int    `json:"id"`
			Name       string `json:"name"`
			PosterPath string `json:"poster_path"`
		} `json:"results"`
	}
	resp, err := c.http.R().SetContext(ctx).
		SetQueryParams(map[string]string{"api_key": c.apiKey, "query": query}).
		SetResult(&out).
		Get("https://api.themoviedb.org/3/search/collection")
	if err != nil {
		return nil, fmt.Errorf("tmdb search collection: %w", redactErr(err))
	}
	if resp.IsError() {
		return nil, fmt.Errorf("tmdb search collection status %d", resp.StatusCode())
	}
	res := make([]TMDBCollection, 0, len(out.Results))
	for _, r := range out.Results {
		res = append(res, TMDBCollection{ID: r.ID, Name: r.Name, PosterPath: r.PosterPath})
	}
	return res, nil
}

// Collection returns a collection with its films, oldest first (unreleased
// and undated films last).
func (c *TMDBClient) Collection(ctx context.Context, id int) (TMDBCollection, error) {
	if !c.Enabled() {
		return TMDBCollection{}, nil
	}
	var out struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`
		PosterPath string `json:"poster_path"`
		Parts      []struct {
			ID            int    `json:"id"`
			Title         string `json:"title"`
			OriginalTitle string `json:"original_title"`
			ReleaseDate   string `json:"release_date"`
			PosterPath    string `json:"poster_path"`
		} `json:"parts"`
	}
	resp, err := c.http.R().SetContext(ctx).
		SetQueryParam("api_key", c.apiKey).
		SetResult(&out).
		Get("https://api.themoviedb.org/3/collection/" + strconv.Itoa(id))
	if err != nil {
		return TMDBCollection{}, fmt.Errorf("tmdb collection %d: %w", id, redactErr(err))
	}
	if resp.IsError() {
		return TMDBCollection{}, fmt.Errorf("tmdb collection %d status %d", id, resp.StatusCode())
	}
	col := TMDBCollection{ID: out.ID, Name: out.Name, PosterPath: out.PosterPath}
	for _, p := range out.Parts {
		date := p.ReleaseDate
		if len(date) != 10 {
			date = ""
		}
		col.Parts = append(col.Parts, TMDBSearchResult{
			ID: p.ID, Title: p.Title, OriginalTitle: p.OriginalTitle, Year: yearFromDate(date), ReleaseDate: date, PosterPath: p.PosterPath,
		})
	}
	sort.SliceStable(col.Parts, func(i, j int) bool {
		a, b := col.Parts[i].ReleaseDate, col.Parts[j].ReleaseDate
		if (a == "") != (b == "") {
			return b == ""
		}
		return a < b
	})
	return col, nil
}
