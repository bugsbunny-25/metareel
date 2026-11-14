package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// WikidataAPI reads Wikidata through its action API (www.wikidata.org/w/api.php):
// search with haswbstatement filters, then wbgetentities 50 at a time. Large
// SPARQL queries over every cinema of a country time out; this doesn't.
type WikidataAPI struct {
	http      *http.Client
	endpoint  string
	userAgent string
	gap       time.Duration // between requests
}

func NewWikidataAPI(timeout time.Duration) *WikidataAPI {
	if timeout < 30*time.Second {
		timeout = 30 * time.Second
	}
	return &WikidataAPI{http: &http.Client{Timeout: timeout}, endpoint: "https://www.wikidata.org/w/api.php", userAgent: wikidataUserAgent, gap: 150 * time.Millisecond}
}

// WikidataDate is a date as precise as Wikidata knows it: YYYY-MM-DD,
// YYYY-MM or YYYY.
type WikidataDate string

// Full reports a day-precision date.
func (d WikidataDate) Full() bool { return len(d) == 10 }

// Year is the date's year, "" if none.
func (d WikidataDate) Year() string {
	if len(d) >= 4 {
		return string(d[:4])
	}
	return ""
}

// WikidataCinema is a closed or demolished cinema on Wikidata.
type WikidataCinema struct {
	QID        string
	Name       string
	Address    string // P6375 street address
	Lat, Lon   float64
	HasCoord   bool
	Opened     WikidataDate // P1619 official opening, else P571 inception
	Closed     WikidataDate // P3999 official closure, else P576 dissolved
	Demolished bool         // P5817 state of use: demolished or destroyed
}

const (
	wdMovieTheater   = "Q41253"
	wdPermClosed     = "Q104664889" // state of use: permanently closed
	wdDemolished     = "Q56556915"  // state of use: demolished or destroyed
	wdSearchPage     = 500
	wdSearchMaxDepth = 10000 // CirrusSearch serves at most this many results per query
	wdEntitiesBatch  = 50
)

var errWikidataBusy = errors.New("wikidata busy")

// CountryQID finds a country's item by its ISO 3166-1 alpha-2 code (P297).
func (c *WikidataAPI) CountryQID(ctx context.Context, iso string) (string, error) {
	if !countryCode.MatchString(iso) {
		return "", fmt.Errorf("invalid country code %q", iso)
	}
	if q, ok := knownCountryQIDs[iso]; ok {
		return q, nil
	}
	ids, _, err := c.search(ctx, "haswbstatement:P297="+iso, "", 0, 5)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("wikidata has no country with code %s", iso)
	}
	return ids[0], nil
}

var knownCountryQIDs = map[string]string{"US": "Q30", "GB": "Q145", "IN": "Q668", "CA": "Q16", "AU": "Q408", "IE": "Q27", "NZ": "Q664", "DE": "Q183", "FR": "Q142"}

// ClosedCinemas lists the cinemas of a country Wikidata records as
// permanently closed, demolished or dissolved. limitNote is set when there
// were more than one search can page through (20,000).
func (c *WikidataAPI) ClosedCinemas(ctx context.Context, countryQID string) (cinemas []WikidataCinema, limitNote string, err error) {
	q := fmt.Sprintf("haswbstatement:P31=%s haswbstatement:P17=%s haswbstatement:P5817=%s|P5817=%s|P576",
		wdMovieTheater, countryQID, wdPermClosed, wdDemolished)
	seen := map[string]bool{}
	var ids []string
	total := 0
	// Search pages stop at 10,000; reading oldest-first and newest-first covers up to 20,000.
	for _, sort := range []string{"create_timestamp_asc", "create_timestamp_desc"} {
		for offset := 0; offset < wdSearchMaxDepth; offset += wdSearchPage {
			page, hits, err := c.search(ctx, q, sort, offset, wdSearchPage)
			if err != nil {
				return nil, "", err
			}
			total = hits
			for _, id := range page {
				if !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
			if len(page) < wdSearchPage || len(ids) >= total {
				break
			}
		}
		if len(ids) >= total {
			break
		}
	}
	if total > len(ids) {
		limitNote = fmt.Sprintf("Wikidata has %d; search reached %d", total, len(ids))
	}
	for i := 0; i < len(ids); i += wdEntitiesBatch {
		batch, err := c.entities(ctx, ids[i:min(i+wdEntitiesBatch, len(ids))])
		if err != nil {
			return nil, "", err
		}
		cinemas = append(cinemas, batch...)
	}
	return cinemas, limitNote, nil
}

func (c *WikidataAPI) search(ctx context.Context, query, sort string, offset, limit int) ([]string, int, error) {
	params := url.Values{"action": {"query"}, "list": {"search"}, "srsearch": {query}, "srlimit": {strconv.Itoa(limit)},
		"sroffset": {strconv.Itoa(offset)}, "srprop": {""}, "srinfo": {"totalhits"}, "format": {"json"}, "maxlag": {"5"}}
	if sort != "" {
		params.Set("srsort", sort)
	}
	var out struct {
		Query struct {
			SearchInfo struct {
				TotalHits int `json:"totalhits"`
			} `json:"searchinfo"`
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := c.get(ctx, params, &out); err != nil {
		return nil, 0, err
	}
	ids := make([]string, 0, len(out.Query.Search))
	for _, s := range out.Query.Search {
		ids = append(ids, s.Title)
	}
	return ids, out.Query.SearchInfo.TotalHits, nil
}

type wdSnak struct {
	Snaktype  string `json:"snaktype"`
	Datavalue struct {
		Value json.RawMessage `json:"value"`
	} `json:"datavalue"`
}

type wdClaim struct {
	Mainsnak wdSnak `json:"mainsnak"`
	Rank     string `json:"rank"`
}

func (c *WikidataAPI) entities(ctx context.Context, ids []string) ([]WikidataCinema, error) {
	params := url.Values{"action": {"wbgetentities"}, "ids": {strings.Join(ids, "|")}, "props": {"labels|claims"},
		"languages": {"en"}, "languagefallback": {"1"}, "format": {"json"}, "maxlag": {"5"}}
	var out struct {
		Entities map[string]struct {
			ID     string `json:"id"`
			Labels map[string]struct {
				Value string `json:"value"`
			} `json:"labels"`
			Claims  map[string][]wdClaim `json:"claims"`
			Missing *string              `json:"missing"`
		} `json:"entities"`
	}
	if err := c.get(ctx, params, &out); err != nil {
		return nil, err
	}
	res := make([]WikidataCinema, 0, len(out.Entities))
	for _, id := range ids { // keep search order
		e, ok := out.Entities[id]
		if !ok || e.Missing != nil {
			continue
		}
		cin := WikidataCinema{QID: e.ID}
		if l, ok := e.Labels["en"]; ok {
			cin.Name = strings.TrimSpace(l.Value)
		}
		var coord struct{ Latitude, Longitude float64 }
		if v := best(e.Claims["P625"]); v != nil && json.Unmarshal(v, &coord) == nil {
			cin.Lat, cin.Lon, cin.HasCoord = coord.Latitude, coord.Longitude, true
		}
		var addr struct{ Text string }
		if v := best(e.Claims["P6375"]); v != nil && json.Unmarshal(v, &addr) == nil {
			cin.Address = strings.TrimSpace(addr.Text)
		}
		cin.Opened = firstDate(e.Claims["P1619"], e.Claims["P571"])
		cin.Closed = firstDate(e.Claims["P3999"], e.Claims["P576"])
		for _, cl := range e.Claims["P5817"] {
			var item struct{ ID string }
			if json.Unmarshal(cl.Mainsnak.Datavalue.Value, &item) == nil && item.ID == wdDemolished {
				cin.Demolished = true
			}
		}
		res = append(res, cin)
	}
	return res, nil
}

// best returns the value of the preferred claim, else the first normal one.
func best(claims []wdClaim) json.RawMessage {
	var first json.RawMessage
	for _, c := range claims {
		if c.Mainsnak.Snaktype != "value" || c.Rank == "deprecated" {
			continue
		}
		if c.Rank == "preferred" {
			return c.Mainsnak.Datavalue.Value
		}
		if first == nil {
			first = c.Mainsnak.Datavalue.Value
		}
	}
	return first
}

// firstDate parses the first property that has a usable date: "+1909-11-08T00:00:00Z"
// at precision 11 (day), 10 (month) or 9 (year).
func firstDate(props ...[]wdClaim) WikidataDate {
	for _, claims := range props {
		v := best(claims)
		if v == nil {
			continue
		}
		var t struct {
			Time      string `json:"time"`
			Precision int    `json:"precision"`
		}
		if json.Unmarshal(v, &t) != nil || len(t.Time) < 11 || t.Time[0] != '+' {
			continue
		}
		d := t.Time[1:11] // YYYY-MM-DD
		switch {
		case t.Precision >= 11 && d[5:7] != "00" && d[8:10] != "00":
			return WikidataDate(d)
		case t.Precision == 10 && d[5:7] != "00":
			return WikidataDate(d[:7])
		case t.Precision == 9:
			return WikidataDate(d[:4])
		}
	}
	return ""
}

// get calls the API, waiting out maxlag and rate limits (twice).
func (c *WikidataAPI) get(ctx context.Context, params url.Values, out any) error {
	for attempt := 0; ; attempt++ {
		err := c.getOnce(ctx, params, out)
		if !errors.Is(err, errWikidataBusy) || attempt == 2 {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(5*(attempt+1)) * time.Second):
		}
	}
}

func (c *WikidataAPI) getOnce(ctx context.Context, params url.Values, out any) error {
	if c.gap > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.gap):
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("wikidata api: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return fmt.Errorf("%w: status %d", errWikidataBusy, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wikidata api status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("wikidata api: read: %w", err)
	}
	var apiErr struct {
		Error *struct {
			Code string `json:"code"`
			Info string `json:"info"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &apiErr) == nil && apiErr.Error != nil {
		if apiErr.Error.Code == "maxlag" {
			return fmt.Errorf("%w: maxlag", errWikidataBusy)
		}
		return fmt.Errorf("wikidata api: %s: %s", apiErr.Error.Code, apiErr.Error.Info)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("wikidata api: decode: %w", err)
	}
	return nil
}
