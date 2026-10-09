package client

import (
	"context"
	"fmt"
	"strings"
)

// TMDBFullDetails is a TMDB movie or TV show with its external IDs, watch
// providers (all countries) and, for movies, digital release dates.
type TMDBFullDetails struct {
	Kind                string // movie | tv
	ID                  string
	Title               string
	OriginalTitle       string
	OriginalLanguage    string
	Overview            string
	Status              string
	ReleaseDate         string // movie release date / TV first air date, YYYY-MM-DD
	LastAirDate         string
	Runtime             int // minutes; TV: first episode_run_time
	NumberOfSeasons     int
	NumberOfEpisodes    int
	Genres              []string
	OriginCountries     []string
	ProductionCompanies []string
	Networks            []string
	PosterPath          string
	BackdropPath        string
	Popularity          float64
	VoteAverage         float64
	VoteCount           int
	IMDbID              string
	WikidataID          string
	// DigitalReleaseDates maps country → first digital (type 4) release date.
	DigitalReleaseDates map[string]string
	WatchProviders      []TMDBWatchProvider
}

// TMDBWatchProvider is one way to watch a title in one country.
type TMDBWatchProvider struct {
	Country         string
	Monetization    string // flatrate | free | ads | rent | buy
	ProviderID      int
	ProviderName    string
	LogoPath        string
	DisplayPriority int
	Link            string
}

type tmdbNamed struct {
	Name string `json:"name"`
}

type tmdbProviderEntry struct {
	LogoPath        string `json:"logo_path"`
	ProviderID      int    `json:"provider_id"`
	ProviderName    string `json:"provider_name"`
	DisplayPriority int    `json:"display_priority"`
}

type tmdbFullResp struct {
	Title               string      `json:"title"`
	Name                string      `json:"name"`
	OriginalTitle       string      `json:"original_title"`
	OriginalName        string      `json:"original_name"`
	OriginalLanguage    string      `json:"original_language"`
	Overview            string      `json:"overview"`
	Status              string      `json:"status"`
	ReleaseDate         string      `json:"release_date"`
	FirstAirDate        string      `json:"first_air_date"`
	LastAirDate         string      `json:"last_air_date"`
	Runtime             int         `json:"runtime"`
	EpisodeRunTime      []int       `json:"episode_run_time"`
	NumberOfSeasons     int         `json:"number_of_seasons"`
	NumberOfEpisodes    int         `json:"number_of_episodes"`
	Genres              []tmdbNamed `json:"genres"`
	OriginCountry       []string    `json:"origin_country"`
	ProductionCountries []struct {
		ISO string `json:"iso_3166_1"`
	} `json:"production_countries"`
	ProductionCompanies []tmdbNamed `json:"production_companies"`
	Networks            []tmdbNamed `json:"networks"`
	PosterPath          string      `json:"poster_path"`
	BackdropPath        string      `json:"backdrop_path"`
	Popularity          float64     `json:"popularity"`
	VoteAverage         float64     `json:"vote_average"`
	VoteCount           int         `json:"vote_count"`
	IMDbID              string      `json:"imdb_id"` // movies carry it at the top level too
	ExternalIDs         struct {
		IMDbID     string `json:"imdb_id"`
		WikidataID string `json:"wikidata_id"`
	} `json:"external_ids"`
	WatchProviders struct {
		Results map[string]struct {
			Link     string              `json:"link"`
			Flatrate []tmdbProviderEntry `json:"flatrate"`
			Free     []tmdbProviderEntry `json:"free"`
			Ads      []tmdbProviderEntry `json:"ads"`
			Rent     []tmdbProviderEntry `json:"rent"`
			Buy      []tmdbProviderEntry `json:"buy"`
		} `json:"results"`
	} `json:"watch/providers"`
	ReleaseDates struct {
		Results []struct {
			ISO          string `json:"iso_3166_1"`
			ReleaseDates []struct {
				ReleaseDate string `json:"release_date"`
				Type        int    `json:"type"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates"`
}

// FullDetails fetches a title with external IDs and watch providers (and
// release dates for movies) in one request. It returns nil, nil when TMDB has
// no such title or no API key is configured.
func (c *TMDBClient) FullDetails(ctx context.Context, kind string, tmdbID string) (*TMDBFullDetails, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if kind != "movie" && kind != "tv" {
		return nil, fmt.Errorf("invalid tmdb kind %q", kind)
	}
	appends := "external_ids,watch/providers"
	if kind == "movie" {
		appends += ",release_dates"
	}
	var out tmdbFullResp
	resp, err := c.http.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{"api_key": c.apiKey, "append_to_response": appends}).
		SetResult(&out).
		Get(fmt.Sprintf("https://api.themoviedb.org/3/%s/%s", kind, tmdbID))
	if err != nil {
		return nil, fmt.Errorf("tmdb %s details: %w", kind, redactErr(err))
	}
	if resp.StatusCode() == 404 {
		return nil, nil
	}
	if resp.IsError() {
		return nil, fmt.Errorf("tmdb %s details status %d", kind, resp.StatusCode())
	}
	return out.toDetails(kind, tmdbID), nil
}

func (r *tmdbFullResp) toDetails(kind, id string) *TMDBFullDetails {
	d := &TMDBFullDetails{
		Kind: kind, ID: id,
		Title: r.Title, OriginalTitle: r.OriginalTitle, OriginalLanguage: r.OriginalLanguage,
		Overview: r.Overview, Status: r.Status, ReleaseDate: r.ReleaseDate, LastAirDate: r.LastAirDate,
		Runtime: r.Runtime, NumberOfSeasons: r.NumberOfSeasons, NumberOfEpisodes: r.NumberOfEpisodes,
		PosterPath: r.PosterPath, BackdropPath: r.BackdropPath,
		Popularity: r.Popularity, VoteAverage: r.VoteAverage, VoteCount: r.VoteCount,
		IMDbID: strings.TrimSpace(r.ExternalIDs.IMDbID), WikidataID: strings.TrimSpace(r.ExternalIDs.WikidataID),
		OriginCountries: r.OriginCountry,
	}
	if kind == "tv" {
		d.Title, d.OriginalTitle, d.ReleaseDate = r.Name, r.OriginalName, r.FirstAirDate
		if len(r.EpisodeRunTime) > 0 {
			d.Runtime = r.EpisodeRunTime[0]
		}
	}
	if d.IMDbID == "" {
		d.IMDbID = strings.TrimSpace(r.IMDbID)
	}
	if len(d.OriginCountries) == 0 {
		for _, c := range r.ProductionCountries {
			d.OriginCountries = append(d.OriginCountries, c.ISO)
		}
	}
	for _, g := range r.Genres {
		d.Genres = append(d.Genres, g.Name)
	}
	for _, c := range r.ProductionCompanies {
		d.ProductionCompanies = append(d.ProductionCompanies, c.Name)
	}
	for _, n := range r.Networks {
		d.Networks = append(d.Networks, n.Name)
	}
	for country, p := range r.WatchProviders.Results {
		for _, group := range []struct {
			kind    string
			entries []tmdbProviderEntry
		}{{"flatrate", p.Flatrate}, {"free", p.Free}, {"ads", p.Ads}, {"rent", p.Rent}, {"buy", p.Buy}} {
			for _, e := range group.entries {
				d.WatchProviders = append(d.WatchProviders, TMDBWatchProvider{
					Country: country, Monetization: group.kind, ProviderID: e.ProviderID, ProviderName: e.ProviderName,
					LogoPath: e.LogoPath, DisplayPriority: e.DisplayPriority, Link: p.Link,
				})
			}
		}
	}
	for _, rd := range r.ReleaseDates.Results {
		for _, x := range rd.ReleaseDates {
			if x.Type != 4 || len(x.ReleaseDate) < 10 {
				continue
			}
			date := x.ReleaseDate[:10]
			if d.DigitalReleaseDates == nil {
				d.DigitalReleaseDates = map[string]string{}
			}
			if cur, ok := d.DigitalReleaseDates[rd.ISO]; !ok || date < cur {
				d.DigitalReleaseDates[rd.ISO] = date
			}
		}
	}
	return d
}
