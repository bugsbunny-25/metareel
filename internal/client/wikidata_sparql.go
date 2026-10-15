package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
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
	NetflixID    string // P1874, e.g. 81231974 (netflix.com/title/81231974)
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
	query := `SELECT ?item ?tmdb ?imdb ?rt ?mc ?lb ?nf WHERE {
  VALUES ?tmdb {` + values.String() + ` }
  ?item wdt:` + prop + ` ?tmdb .
  OPTIONAL { ?item wdt:P345 ?imdb }
  OPTIONAL { ?item wdt:P1258 ?rt }
  OPTIONAL { ?item wdt:P1712 ?mc }
  OPTIONAL { ?item wdt:P6127 ?lb }
  OPTIONAL { ?item wdt:P1874 ?nf }
}`
	bindings, err := c.query(ctx, query)
	if err != nil {
		return err
	}
	for _, b := range bindings {
		tmdb := b["tmdb"]
		if tmdb == "" {
			continue
		}
		ids := out[tmdb] // several rows per item when a property has several values; keep the first
		if ids.WikidataID == "" {
			ids.WikidataID = strings.TrimPrefix(b["item"], wikidataEntityPrefix)
		}
		setFirst(&ids.IMDbID, b["imdb"])
		setFirst(&ids.RottenTomato, b["rt"])
		setFirst(&ids.Metacritic, b["mc"])
		setFirst(&ids.Letterboxd, b["lb"])
		if isDigits(b["nf"]) {
			setFirst(&ids.NetflixID, b["nf"])
		}
		out[tmdb] = ids
	}
	return nil
}

const wikidataEntityPrefix = "http://www.wikidata.org/entity/"

// WikidataNetflixHit is a Wikidata item with a Netflix ID whose English
// label or alias is a searched name.
type WikidataNetflixHit struct {
	Name       string // the searched name it matched
	WikidataID string
	NetflixID  string
	TmdbKind   string // movie | tv
	TmdbID     string
	IMDbID     string
	Year       int // earliest publication year (P577), 0 if unknown
}

// WikidataNameBatchSize is the most names FindNetflixTitles sends per query.
const WikidataNameBatchSize = 50

// FindNetflixTitles looks names up among Wikidata items that have a Netflix
// ID (P1874) and a TMDB ID, by English label or alias. It returns the hits
// per searched name; an item with both a TMDB movie and TV ID yields two.
func (c *WikidataSPARQL) FindNetflixTitles(ctx context.Context, names []string) (map[string][]WikidataNetflixHit, error) {
	out := map[string][]WikidataNetflixHit{}
	for start := 0; start < len(names); start += WikidataNameBatchSize {
		end := min(start+WikidataNameBatchSize, len(names))
		if err := c.findNetflixBatch(ctx, names[start:end], out); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (c *WikidataSPARQL) findNetflixBatch(ctx context.Context, names []string, out map[string][]WikidataNetflixHit) error {
	var values strings.Builder
	for _, n := range names {
		if lit, ok := sparqlString(n); ok {
			values.WriteString(" " + lit + "@en")
		}
	}
	if values.Len() == 0 {
		return nil
	}
	query := `SELECT ?name ?item ?nf ?tmdbm ?tmdbtv ?imdb (MIN(YEAR(?date)) AS ?year) WHERE {
  VALUES ?name {` + values.String() + ` }
  { ?item rdfs:label ?name } UNION { ?item skos:altLabel ?name }
  ?item wdt:P1874 ?nf .
  OPTIONAL { ?item wdt:P4947 ?tmdbm }
  OPTIONAL { ?item wdt:P4983 ?tmdbtv }
  OPTIONAL { ?item wdt:P345 ?imdb }
  OPTIONAL { ?item wdt:P577 ?date }
} GROUP BY ?name ?item ?nf ?tmdbm ?tmdbtv ?imdb`
	bindings, err := c.query(ctx, query)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, b := range bindings {
		year, _ := strconv.Atoi(b["year"])
		for kind, id := range map[string]string{"movie": b["tmdbm"], "tv": b["tmdbtv"]} {
			if !isDigits(id) || !isDigits(b["nf"]) {
				continue
			}
			key := b["name"] + "|" + kind + "|" + id
			if seen[key] {
				continue // several Netflix / IMDb IDs for one item
			}
			seen[key] = true
			out[b["name"]] = append(out[b["name"]], WikidataNetflixHit{
				Name: b["name"], WikidataID: strings.TrimPrefix(b["item"], wikidataEntityPrefix), NetflixID: b["nf"],
				TmdbKind: kind, TmdbID: id, IMDbID: b["imdb"], Year: year,
			})
		}
	}
	return nil
}

// sparqlString quotes s as a SPARQL string literal; names with characters
// that cannot be safely quoted are skipped.
func sparqlString(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 300 || strings.ContainsAny(s, "\"\\\n\r") {
		return "", false
	}
	return `"` + s + `"`, true
}

// query runs a SPARQL query and returns its bindings as name → value.
func (c *WikidataSPARQL) query(ctx context.Context, query string) ([]map[string]string, error) {
	form := url.Values{"query": {query}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/sparql-results+json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wikidata sparql: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, fmt.Errorf("wikidata sparql status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var res struct {
		Results struct {
			Bindings []map[string]struct {
				Value string `json:"value"`
			} `json:"bindings"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode wikidata sparql: %w", err)
	}
	out := make([]map[string]string, 0, len(res.Results.Bindings))
	for _, b := range res.Results.Bindings {
		row := make(map[string]string, len(b))
		for k, v := range b {
			row[k] = v.Value
		}
		out = append(out, row)
	}
	return out, nil
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

// WikidataQIDRe matches a Wikidata item ID such as Q141461750.
var WikidataQIDRe = regexp.MustCompile(`^Q[1-9]\d{0,11}$`)

// WikidataItem is what Wikidata holds for one item, by QID.
type WikidataItem struct {
	WikidataID   string
	TmdbMovieID  string // P4947
	TmdbTVID     string // P4983
	IMDbID       string
	RottenTomato string
	NetflixID    string
}

// LookupByQID returns an item's TMDB / IMDb / Rotten Tomatoes / Netflix IDs;
// nil when the item does not exist or has none of them.
func (c *WikidataSPARQL) LookupByQID(ctx context.Context, qid string) (*WikidataItem, error) {
	if !WikidataQIDRe.MatchString(qid) {
		return nil, fmt.Errorf("invalid wikidata id %q", qid)
	}
	bindings, err := c.query(ctx, `SELECT ?tmdbm ?tmdbtv ?imdb ?rt ?nf WHERE {
  VALUES ?item { wd:`+qid+` }
  OPTIONAL { ?item wdt:P4947 ?tmdbm }
  OPTIONAL { ?item wdt:P4983 ?tmdbtv }
  OPTIONAL { ?item wdt:P345 ?imdb }
  OPTIONAL { ?item wdt:P1258 ?rt }
  OPTIONAL { ?item wdt:P1874 ?nf }
}`)
	if err != nil {
		return nil, err
	}
	it := &WikidataItem{WikidataID: qid}
	for _, b := range bindings {
		if isDigits(b["tmdbm"]) {
			setFirst(&it.TmdbMovieID, b["tmdbm"])
		}
		if isDigits(b["tmdbtv"]) {
			setFirst(&it.TmdbTVID, b["tmdbtv"])
		}
		setFirst(&it.IMDbID, b["imdb"])
		setFirst(&it.RottenTomato, b["rt"])
		if isDigits(b["nf"]) {
			setFirst(&it.NetflixID, b["nf"])
		}
	}
	if it.TmdbMovieID == "" && it.TmdbTVID == "" && it.IMDbID == "" && it.NetflixID == "" {
		return nil, nil
	}
	return it, nil
}

// LookupByNetflixIDs returns, per Netflix ID (P1874), the Wikidata items
// with it and their TMDB IDs (one hit per TMDB kind).
func (c *WikidataSPARQL) LookupByNetflixIDs(ctx context.Context, netflixIDs []string) (map[string][]WikidataNetflixHit, error) {
	out := map[string][]WikidataNetflixHit{}
	for start := 0; start < len(netflixIDs); start += WikidataBatchSize {
		end := min(start+WikidataBatchSize, len(netflixIDs))
		var values strings.Builder
		for _, id := range netflixIDs[start:end] {
			if isDigits(id) {
				values.WriteString(` "` + id + `"`)
			}
		}
		if values.Len() == 0 {
			continue
		}
		bindings, err := c.query(ctx, `SELECT ?nf ?item ?tmdbm ?tmdbtv ?imdb WHERE {
  VALUES ?nf {`+values.String()+` }
  ?item wdt:P1874 ?nf .
  OPTIONAL { ?item wdt:P4947 ?tmdbm }
  OPTIONAL { ?item wdt:P4983 ?tmdbtv }
  OPTIONAL { ?item wdt:P345 ?imdb }
}`)
		if err != nil {
			return out, err
		}
		seen := map[string]bool{}
		for _, b := range bindings {
			for kind, id := range map[string]string{"movie": b["tmdbm"], "tv": b["tmdbtv"]} {
				key := b["nf"] + "|" + kind + "|" + id
				if !isDigits(id) || seen[key] {
					continue
				}
				seen[key] = true
				out[b["nf"]] = append(out[b["nf"]], WikidataNetflixHit{
					Name: b["nf"], WikidataID: strings.TrimPrefix(b["item"], wikidataEntityPrefix), NetflixID: b["nf"],
					TmdbKind: kind, TmdbID: id, IMDbID: b["imdb"],
				})
			}
		}
	}
	return out, nil
}
