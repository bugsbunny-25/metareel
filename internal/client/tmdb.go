package client

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
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
		ID            int    `json:"id"`
		Title         string `json:"title"`          // movies
		OriginalTitle string `json:"original_title"` // movies
		ReleaseDate   string `json:"release_date"`   // movies, YYYY-MM-DD
		Name          string `json:"name"`           // tv
		OriginalName  string `json:"original_name"`  // tv
		FirstAirDate  string `json:"first_air_date"` // tv, YYYY-MM-DD
	} `json:"results"`
}

// TMDBSearchResult is a single movie or TV search hit.
type TMDBSearchResult struct {
	ID            int
	Title         string
	OriginalTitle string
	Year          int    // 0 if TMDB has no release / first-air date
	ReleaseDate   string // YYYY-MM-DD release / first-air date, "" if none
}

// SearchMovie returns TMDB movie search results for query, most relevant first.
func (c *TMDBClient) SearchMovie(ctx context.Context, query string) ([]TMDBSearchResult, error) {
	return c.search(ctx, "movie", query)
}

// SearchTV returns TMDB TV search results for query, most relevant first.
func (c *TMDBClient) SearchTV(ctx context.Context, query string) ([]TMDBSearchResult, error) {
	return c.search(ctx, "tv", query)
}

func (c *TMDBClient) search(ctx context.Context, kind string, query string) ([]TMDBSearchResult, error) {
	if !c.Enabled() {
		return nil, nil
	}
	var out tmdbSearchResp
	resp, err := c.http.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"api_key": c.apiKey,
			"query":   query,
		}).
		SetResult(&out).
		Get("https://api.themoviedb.org/3/search/" + kind)
	if err != nil {
		return nil, fmt.Errorf("tmdb search %s: %w", kind, redactErr(err))
	}
	if resp.IsError() {
		return nil, fmt.Errorf("tmdb search %s status %d", kind, resp.StatusCode())
	}

	results := make([]TMDBSearchResult, 0, len(out.Results))
	for _, r := range out.Results {
		res := TMDBSearchResult{ID: r.ID, Title: r.Title, OriginalTitle: r.OriginalTitle, Year: yearFromDate(r.ReleaseDate), ReleaseDate: r.ReleaseDate}
		if kind == "tv" {
			res.Title, res.OriginalTitle, res.Year, res.ReleaseDate = r.Name, r.OriginalName, yearFromDate(r.FirstAirDate), r.FirstAirDate
		}
		if len(res.ReleaseDate) != 10 {
			res.ReleaseDate = ""
		}
		results = append(results, res)
	}
	return results, nil
}

func yearFromDate(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return y
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
		return "", "", fmt.Errorf("tmdb movie external ids: %w", redactErr(err))
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
		return "", "", fmt.Errorf("tmdb tv external ids: %w", redactErr(err))
	}
	if resp.IsError() {
		return "", "", fmt.Errorf("tmdb tv external ids status %d", resp.StatusCode())
	}
	return strings.TrimSpace(out.IMDbID), strings.TrimSpace(out.WikidataID), nil
}

// Details returns the title and release / first-air year of a TMDB movie or
// TV show; kind is "movie" or "tv".
func (c *TMDBClient) Details(ctx context.Context, kind string, tmdbID string) (string, int, error) {
	if !c.Enabled() {
		return "", 0, nil
	}
	if kind != "movie" && kind != "tv" {
		return "", 0, fmt.Errorf("invalid tmdb kind %q", kind)
	}
	var out struct {
		Title        string `json:"title"`
		Name         string `json:"name"`
		ReleaseDate  string `json:"release_date"`
		FirstAirDate string `json:"first_air_date"`
	}
	resp, err := c.http.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{"api_key": c.apiKey}).
		SetResult(&out).
		Get(fmt.Sprintf("https://api.themoviedb.org/3/%s/%s", kind, tmdbID))
	if err != nil {
		return "", 0, fmt.Errorf("tmdb %s details: %w", kind, redactErr(err))
	}
	if resp.StatusCode() == 404 {
		return "", 0, nil
	}
	if resp.IsError() {
		return "", 0, fmt.Errorf("tmdb %s details status %d", kind, resp.StatusCode())
	}
	if kind == "tv" {
		return out.Name, yearFromDate(out.FirstAirDate), nil
	}
	return out.Title, yearFromDate(out.ReleaseDate), nil
}

// TMDBFound is a TMDB title found by an external ID.
type TMDBFound struct {
	Kind  string // movie | tv
	ID    string
	Title string
	Year  int
}

// FindByExternalID returns the TMDB titles with an IMDb (tt…) or Wikidata
// (Q…) ID, via /find. Other IDs are rejected before any request.
func (c *TMDBClient) FindByExternalID(ctx context.Context, id string) ([]TMDBFound, error) {
	if !c.Enabled() {
		return nil, nil
	}
	var source string
	switch {
	case imdbTitleRe.MatchString(id):
		source = "imdb_id"
	case WikidataQIDRe.MatchString(id):
		source = "wikidata_id"
	default:
		return nil, fmt.Errorf("unsupported external id %q", id)
	}
	var out struct {
		Movies []struct {
			ID          int    `json:"id"`
			Title       string `json:"title"`
			ReleaseDate string `json:"release_date"`
		} `json:"movie_results"`
		TV []struct {
			ID           int    `json:"id"`
			Name         string `json:"name"`
			FirstAirDate string `json:"first_air_date"`
		} `json:"tv_results"`
	}
	resp, err := c.http.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{"api_key": c.apiKey, "external_source": source}).
		SetResult(&out).
		Get("https://api.themoviedb.org/3/find/" + id)
	if err != nil {
		return nil, fmt.Errorf("tmdb find: %w", redactErr(err))
	}
	if resp.IsError() {
		return nil, fmt.Errorf("tmdb find status %d", resp.StatusCode())
	}
	found := make([]TMDBFound, 0, len(out.Movies)+len(out.TV))
	for _, m := range out.Movies {
		found = append(found, TMDBFound{Kind: "movie", ID: strconv.Itoa(m.ID), Title: m.Title, Year: yearFromDate(m.ReleaseDate)})
	}
	for _, t := range out.TV {
		found = append(found, TMDBFound{Kind: "tv", ID: strconv.Itoa(t.ID), Title: t.Name, Year: yearFromDate(t.FirstAirDate)})
	}
	return found, nil
}

var imdbTitleRe = regexp.MustCompile(`^tt\d{5,10}$`)

// TMDB watch provider IDs of Netflix (the standard plan and "Netflix basic
// with Ads"), as in watch/providers and JustWatch package IDs.
var TMDBNetflixProviderIDs = []int{8, 1796}

// WatchProviderCountries returns the countries where any of providerIDs
// streams a title (flatrate, free or ads), from TMDB's watch providers
// (data by JustWatch). Nil when TMDB has no such title.
func (c *TMDBClient) WatchProviderCountries(ctx context.Context, kind, tmdbID string, providerIDs []int) ([]string, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if kind != "movie" && kind != "tv" {
		return nil, fmt.Errorf("invalid tmdb kind %q", kind)
	}
	type offer struct {
		ProviderID int `json:"provider_id"`
	}
	var out struct {
		Results map[string]struct {
			Flatrate []offer `json:"flatrate"`
			Free     []offer `json:"free"`
			Ads      []offer `json:"ads"`
		} `json:"results"`
	}
	resp, err := c.http.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{"api_key": c.apiKey}).
		SetResult(&out).
		Get(fmt.Sprintf("https://api.themoviedb.org/3/%s/%s/watch/providers", kind, tmdbID))
	if err != nil {
		return nil, fmt.Errorf("tmdb %s watch providers: %w", kind, redactErr(err))
	}
	if resp.StatusCode() == 404 {
		return nil, nil
	}
	if resp.IsError() {
		return nil, fmt.Errorf("tmdb %s watch providers status %d", kind, resp.StatusCode())
	}
	want := map[int]bool{}
	for _, id := range providerIDs {
		want[id] = true
	}
	var countries []string
	for country, r := range out.Results {
		for _, list := range [][]offer{r.Flatrate, r.Free, r.Ads} {
			found := false
			for _, o := range list {
				if want[o.ProviderID] {
					found = true
					break
				}
			}
			if found {
				countries = append(countries, country)
				break
			}
		}
	}
	sort.Strings(countries)
	return countries, nil
}

// AlternativeTitles returns a title's alternative titles on TMDB (other
// countries' and languages' names). Nil when TMDB has no such title.
func (c *TMDBClient) AlternativeTitles(ctx context.Context, kind, tmdbID string) ([]string, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if kind != "movie" && kind != "tv" {
		return nil, fmt.Errorf("invalid tmdb kind %q", kind)
	}
	type alt struct {
		Title string `json:"title"`
	}
	var out struct {
		Titles  []alt `json:"titles"`  // movies
		Results []alt `json:"results"` // tv
	}
	resp, err := c.http.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{"api_key": c.apiKey}).
		SetResult(&out).
		Get(fmt.Sprintf("https://api.themoviedb.org/3/%s/%s/alternative_titles", kind, tmdbID))
	if err != nil {
		return nil, fmt.Errorf("tmdb %s alternative titles: %w", kind, redactErr(err))
	}
	if resp.StatusCode() == 404 {
		return nil, nil
	}
	if resp.IsError() {
		return nil, fmt.Errorf("tmdb %s alternative titles status %d", kind, resp.StatusCode())
	}
	var titles []string
	for _, a := range append(out.Titles, out.Results...) {
		if t := strings.TrimSpace(a.Title); t != "" {
			titles = append(titles, t)
		}
	}
	return titles, nil
}
