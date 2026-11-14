package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Overpass reads OpenStreetMap data through an Overpass API instance
// (https://wiki.openstreetmap.org/wiki/Overpass_API). OSM data is ODbL:
// © OpenStreetMap contributors.
type Overpass struct {
	http      *http.Client
	endpoint  string
	userAgent string
}

// DefaultOverpassURL is the main public instance; its usage policy asks for
// well under 10,000 queries a day, one at a time.
const DefaultOverpassURL = "https://overpass-api.de/api/interpreter"

// overpassQueryTimeout is the server-side limit sent with each query; a
// country's cinemas take seconds, the US up to a minute on a busy server.
const overpassQueryTimeout = 300

// overpassMaxBody caps one response (the US is a few MB).
const overpassMaxBody = 64 << 20

// ErrOverpassBusy: the instance is rate limiting or overloaded (429, 504);
// try again later.
var ErrOverpassBusy = errors.New("overpass instance busy")

var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)

func NewOverpass(endpoint string) *Overpass {
	if endpoint == "" {
		endpoint = DefaultOverpassURL
	}
	return &Overpass{
		http:      &http.Client{Timeout: (overpassQueryTimeout + 60) * time.Second},
		endpoint:  endpoint,
		userAgent: wikidataUserAgent,
	}
}

// OSMPlace is one OpenStreetMap element: a node, or a way or relation with
// its centre.
type OSMPlace struct {
	Type string // node | way | relation
	ID   int64
	Lat  float64
	Lon  float64
	Tags map[string]string
}

// Ref is the element's stable OSM reference, e.g. "way/123456".
func (p OSMPlace) Ref() string { return fmt.Sprintf("%s/%d", p.Type, p.ID) }

type overpassResp struct {
	Remark   string `json:"remark"`
	Elements []struct {
		Type   string                      `json:"type"`
		ID     int64                       `json:"id"`
		Lat    *float64                    `json:"lat"`
		Lon    *float64                    `json:"lon"`
		Center *struct{ Lat, Lon float64 } `json:"center"`
		Tags   map[string]string           `json:"tags"`
	} `json:"elements"`
}

// Cinemas returns every amenity=cinema in a country (ISO 3166-1 alpha-2).
func (c *Overpass) Cinemas(ctx context.Context, country string) ([]OSMPlace, error) {
	return c.query(ctx, country, `nwr["amenity"="cinema"]`)
}

// Towns returns the city and town nodes of a country, to name the town a
// cinema without an address is in.
func (c *Overpass) Towns(ctx context.Context, country string) ([]OSMPlace, error) {
	return c.query(ctx, country, `node["place"~"^(city|town)$"]`)
}

func (c *Overpass) query(ctx context.Context, country, selector string) ([]OSMPlace, error) {
	if !countryCode.MatchString(country) {
		return nil, fmt.Errorf("invalid country code %q", country)
	}
	q := fmt.Sprintf(`[out:json][timeout:%d];area["ISO3166-1"="%s"][admin_level=2]->.c;%s(area.c);out center tags;`,
		overpassQueryTimeout, country, selector)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(url.Values{"data": {q}}.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("overpass: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusGatewayTimeout {
		return nil, fmt.Errorf("%w: status %d", ErrOverpassBusy, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("overpass status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, overpassMaxBody+1))
	if err != nil {
		return nil, fmt.Errorf("overpass: read: %w", err)
	}
	if len(body) > overpassMaxBody {
		return nil, fmt.Errorf("overpass: response over %d MB", overpassMaxBody>>20)
	}
	var r overpassResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("overpass: decode: %w", err)
	}
	// A query that ran out of time or memory still answers 200, with a
	// partial (or empty) result and a remark.
	if strings.Contains(r.Remark, "error") {
		return nil, fmt.Errorf("%w: %s", ErrOverpassBusy, strings.TrimSpace(r.Remark))
	}
	out := make([]OSMPlace, 0, len(r.Elements))
	for _, e := range r.Elements {
		p := OSMPlace{Type: e.Type, ID: e.ID, Tags: e.Tags}
		switch {
		case e.Lat != nil && e.Lon != nil:
			p.Lat, p.Lon = *e.Lat, *e.Lon
		case e.Center != nil:
			p.Lat, p.Lon = e.Center.Lat, e.Center.Lon
		default:
			continue
		}
		if p.Tags == nil {
			p.Tags = map[string]string{}
		}
		out = append(out, p)
	}
	return out, nil
}
