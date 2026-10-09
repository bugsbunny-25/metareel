package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WikidataSPARQL batch-looks-up external IDs on Wikidata's query service
// (https://query.wikidata.org), replacing one REST call per title.
type WikidataSPARQL struct {
	http      *http.Client
	endpoint  string
	userAgent string
}

// wikidataUserAgent identifies us as Wikimedia's User-Agent policy asks.
const wikidataUserAgent = "metareel/0.1 (https://github.com/bugsbunny-25/metareel)"

func NewWikidataSPARQL(timeout time.Duration) *WikidataSPARQL {
	if timeout < 30*time.Second {
		timeout = 30 * time.Second
	}
	return &WikidataSPARQL{
		http:      &http.Client{Timeout: timeout},
		endpoint:  "https://query.wikidata.org/sparql",
		userAgent: wikidataUserAgent,
	}
}

// WikidataIDs are the IDs Wikidata holds for one TMDB title.
type WikidataIDs struct {
	WikidataID   string // e.g. Q125971387
	IMDbID       string // P345
	RottenTomato string // P1258, e.g. m/bugonia or tv/beef
	Metacritic   string // P1712, e.g. movie/bugonia
	Letterboxd   string // P6127 (films)
}

// WikidataBatchSize is the most TMDB IDs LookupByTMDB sends in one query.
const WikidataBatchSize = 100

// LookupByTMDB returns Wikidata's IDs for TMDB titles of one kind ("movie"
// or "tv"), keyed by TMDB ID. Titles Wikidata doesn't know are absent.
func (c *WikidataSPARQL) LookupByTMDB(ctx context.Context, kind string, tmdbIDs []string) (map[string]WikidataIDs, error) {
	prop := "P4947" // TMDB movie ID
	if kind == "tv" {
		prop = "P4983" // TMDB TV series ID
	} else if kind != "movie" {
		return nil, fmt.Errorf("invalid tmdb kind %q", kind)
	}
	out := map[string]WikidataIDs{}
	for start := 0; start < len(tmdbIDs); start += WikidataBatchSize {
		end := min(start+WikidataBatchSize, len(tmdbIDs))
		if err := c.lookupBatch(ctx, prop, tmdbIDs[start:end], out); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (c *WikidataSPARQL) lookupBatch(ctx context.Context, prop string, ids []string, out map[string]WikidataIDs) error {
	var values strings.Builder
	for _, id := range ids {
		if !isDigits(id) {
			continue // only numeric TMDB IDs go into the query
		}
		values.WriteString(` "` + id + `"`)
	}
	if values.Len() == 0 {
		return nil
	}
	query := `SELECT ?item ?tmdb ?imdb ?rt ?mc ?lb WHERE {
  VALUES ?tmdb {` + values.String() + ` }
  ?item wdt:` + prop + ` ?tmdb .
  OPTIONAL { ?item wdt:P345 ?imdb }
  OPTIONAL { ?item wdt:P1258 ?rt }
  OPTIONAL { ?item wdt:P1712 ?mc }
  OPTIONAL { ?item wdt:P6127 ?lb }
}`
	form := url.Values{"query": {query}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/sparql-results+json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("wikidata sparql: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("wikidata sparql status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var res struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return fmt.Errorf("decode wikidata sparql: %w", err)
	}
	for _, b := range res.Results.Bindings {
		tmdb := b["tmdb"].Value
		if tmdb == "" {
			continue
		}
		ids := out[tmdb] // several rows per item when a property has several values; keep the first
		if ids.WikidataID == "" {
			ids.WikidataID = strings.TrimPrefix(b["item"].Value, "http://www.wikidata.org/entity/")
		}
		setFirst(&ids.IMDbID, b["imdb"].Value)
		setFirst(&ids.RottenTomato, b["rt"].Value)
		setFirst(&ids.Metacritic, b["mc"].Value)
		setFirst(&ids.Letterboxd, b["lb"].Value)
		out[tmdb] = ids
	}
	return nil
}

func setFirst(dst *string, v string) {
	if *dst == "" {
		*dst = strings.TrimSpace(v)
	}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
