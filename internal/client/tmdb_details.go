package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
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
	// ReleaseDates: the earliest date of each release type per country
	// (movies only; nil when TMDB lists none).
	ReleaseDates []TMDBReleaseDate
	// ReleaseDatesFetched: the response had a release_dates block (an empty
	// one means none); without it stored release dates are kept.
	ReleaseDatesFetched bool
	WatchProviders      []TMDBWatchProvider
	// Seasons of a TV show (season 0 = specials); nil for movies.
	Seasons []TMDBSeason
}

// TMDB release types.
const (
	ReleasePremiere          = 1
	ReleaseTheatricalLimited = 2
	ReleaseTheatrical        = 3
	ReleaseDigital           = 4
	ReleasePhysical          = 5
	ReleaseTV                = 6
)

// TMDBReleaseDate is a movie's earliest release of one type in a country.
type TMDBReleaseDate struct {
	Country string
	Type    int    // ReleasePremiere … ReleaseTV
	Date    string // YYYY-MM-DD
	Note    string
}

// TMDBSeason is one season of a TV show.
type TMDBSeason struct {
	Number       int    `json:"season_number"`
	Name         string `json:"name"`
	AirDate      string `json:"air_date,omitempty"` // YYYY-MM-DD
	EpisodeCount int    `json:"episode_count"`
}

// TMDBWatchProvider is one way to watch a title in one country.
type TMDBWatchProvider struct {
	Country         string
	Monetization    string // flatrate | free | ads | rent | buy
	ProviderID      int
	ProviderName    string
	LogoPath        string
	DisplayPriority int
	Link            string // TMDB's watch page for the country
	URL             string // the service's page for the title (from JustWatch; "" = unknown)
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
	ProductionCompanies []tmdbNamed  `json:"production_companies"`
	Networks            []tmdbNamed  `json:"networks"`
	Seasons             []TMDBSeason `json:"seasons"`
	PosterPath          string       `json:"poster_path"`
	BackdropPath        string       `json:"backdrop_path"`
	Popularity          float64      `json:"popularity"`
	VoteAverage         float64      `json:"vote_average"`
	VoteCount           int          `json:"vote_count"`
	IMDbID              string       `json:"imdb_id"` // movies carry it at the top level too
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
				Note        string `json:"note"`
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
		d.Seasons = make([]TMDBSeason, 0, len(r.Seasons))
		for _, se := range r.Seasons {
			se.Name = strings.TrimSpace(se.Name)
			d.Seasons = append(d.Seasons, se)
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
	d.ReleaseDatesFetched = r.ReleaseDates.Results != nil
	type releaseKey struct {
		country string
		typ     int
	}
	earliest := map[releaseKey]int{} // index into d.ReleaseDates
	for _, rd := range r.ReleaseDates.Results {
		country := strings.ToUpper(strings.TrimSpace(rd.ISO))
		if len(country) != 2 || country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z' {
			continue
		}
		for _, x := range rd.ReleaseDates {
			if x.Type < ReleasePremiere || x.Type > ReleaseTV || len(x.ReleaseDate) < 10 {
				continue
			}
			date := x.ReleaseDate[:10]
			if _, err := time.Parse("2006-01-02", date); err != nil {
				continue
			}
			k := releaseKey{country, x.Type}
			note := cleanNote(x.Note)
			if i, ok := earliest[k]; ok {
				if date < d.ReleaseDates[i].Date {
					d.ReleaseDates[i].Date, d.ReleaseDates[i].Note = date, note
				}
			} else {
				earliest[k] = len(d.ReleaseDates)
				d.ReleaseDates = append(d.ReleaseDates, TMDBReleaseDate{Country: country, Type: x.Type, Date: date, Note: note})
			}
			if x.Type == ReleaseDigital {
				if d.DigitalReleaseDates == nil {
					d.DigitalReleaseDates = map[string]string{}
				}
				if cur, ok := d.DigitalReleaseDates[country]; !ok || date < cur {
					d.DigitalReleaseDates[country] = date
				}
			}
		}
	}
	sort.Slice(d.ReleaseDates, func(i, j int) bool {
		a, b := d.ReleaseDates[i], d.ReleaseDates[j]
		if a.Country != b.Country {
			return a.Country < b.Country
		}
		return a.Type < b.Type
	})
	return d
}

// TMDBEpisode is one episode of a TV season.
type TMDBEpisode struct {
	SeasonNumber  int    `json:"season_number"`
	EpisodeNumber int    `json:"episode_number"`
	Name          string `json:"name"`
	AirDate       string `json:"air_date"`     // YYYY-MM-DD, empty = TBA
	EpisodeType   string `json:"episode_type"` // standard | finale | mid_season
	Runtime       int    `json:"runtime"`      // minutes, 0 = unknown
}

// tmdbMaxAppends is how many sub-requests TMDB accepts in append_to_response.
const tmdbMaxAppends = 20

// TVSeasonEpisodes fetches the episodes of a TV show's seasons, up to 20
// seasons per request (append_to_response=season/N). It returns nil, nil when
// the show is unknown to TMDB or no API key is configured; seasons TMDB does
// not return are left out of the map.
func (c *TMDBClient) TVSeasonEpisodes(ctx context.Context, tvID string, seasons []int) (map[int][]TMDBEpisode, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if !isDigits(tvID) {
		return nil, fmt.Errorf("invalid tmdb tv id")
	}
	out := make(map[int][]TMDBEpisode, len(seasons))
	for start := 0; start < len(seasons); start += tmdbMaxAppends {
		batch := seasons[start:min(start+tmdbMaxAppends, len(seasons))]
		appends := make([]string, 0, len(batch))
		for _, n := range batch {
			appends = append(appends, fmt.Sprintf("season/%d", n))
		}
		var raw map[string]json.RawMessage
		resp, err := c.http.R().
			SetContext(ctx).
			SetQueryParams(map[string]string{"api_key": c.apiKey, "append_to_response": strings.Join(appends, ",")}).
			Get(fmt.Sprintf("https://api.themoviedb.org/3/tv/%s", tvID))
		if err != nil {
			return nil, fmt.Errorf("tmdb tv seasons: %w", redactErr(err))
		}
		if resp.StatusCode() == 404 {
			if start == 0 {
				return nil, nil // unknown show
			}
			break // gone between two batches: keep what was read
		}
		if resp.IsError() {
			return nil, fmt.Errorf("tmdb tv seasons status %d", resp.StatusCode())
		}
		if err := json.Unmarshal(resp.Body(), &raw); err != nil {
			return nil, fmt.Errorf("decode tmdb tv %s seasons: %w", tvID, err)
		}
		for _, n := range batch {
			body, ok := raw[fmt.Sprintf("season/%d", n)]
			if !ok || string(body) == "null" {
				continue
			}
			var season struct {
				Episodes []TMDBEpisode `json:"episodes"`
			}
			if err := json.Unmarshal(body, &season); err != nil {
				return nil, fmt.Errorf("decode tmdb season %d of tv %s: %w", n, tvID, err)
			}
			for i := range season.Episodes {
				season.Episodes[i].Name = strings.TrimSpace(season.Episodes[i].Name)
				season.Episodes[i].SeasonNumber = n
			}
			out[n] = season.Episodes
		}
	}
	return out, nil
}

// cleanNote makes TMDB's free-text release note safe to store: no control
// or invalid characters, at most 200 bytes, cut on a character boundary.
func cleanNote(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		s = s[:200]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1] // drop a character cut in half
		}
	}
	return strings.TrimSpace(s)
}
