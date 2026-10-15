package client

import (
	"bytes"
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

// NetflixTop10Search looks titles up in Netflix's Top 10 search on Tudum
// (netflix.com/tudum/top10), which gives each charted title's Netflix ID
// (a season's ID for TV) and the weeks it charted. It sends the persisted
// GraphQL query the page itself uses, anonymously; Netflix can change the
// query's ID or version at any time, so both are configurable.
type NetflixTop10Search struct {
	http         *http.Client
	endpoint     string
	titleBase    string // https://www.netflix.com/title/
	queryID      string
	queryVersion int
	userAgent    string
}

func NewNetflixTop10Search(timeout time.Duration, queryID string, queryVersion int) *NetflixTop10Search {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &NetflixTop10Search{
		// Season IDs answer with a redirect to their show: read it, don't follow.
		http: &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
		endpoint:     "https://www.netflix.com/graphql",
		titleBase:    "https://www.netflix.com/title/",
		queryID:      queryID,
		queryVersion: queryVersion,
		userAgent:    "metareel/0.1 (+https://github.com/bugsbunny-25/metareel)",
	}
}

// NetflixSearchItem is one title of a Top 10 search.
type NetflixSearchItem struct {
	NetflixID   string // the title's ID; a TV season's own ID for TV
	Title       string // e.g. "Wednesday: Season 2" (as in the Top 10 files)
	Category    string // FILMS | TV
	GlobalWeeks []string
	// CountryWeeks are the weeks it charted in any country.
	CountryWeeks []string
}

// Weeks returns every week (YYYY-MM-DD, Sunday) the item charted, global
// or in any country.
func (i NetflixSearchItem) Weeks() []string {
	return append(append([]string{}, i.GlobalWeeks...), i.CountryWeeks...)
}

// Search returns the Top 10 titles matching term.
func (c *NetflixTop10Search) Search(ctx context.Context, term string) ([]NetflixSearchItem, error) {
	body, err := json.Marshal(map[string]any{
		"operationName": "Top10SearchQuery",
		"variables":     map[string]any{"request": map[string]any{"searchTerm": term, "language": "EN"}},
		"extensions":    map[string]any{"persistedQuery": map[string]any{"id": c.queryID, "version": c.queryVersion}},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("netflix top 10 search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, fmt.Errorf("netflix top 10 search status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	type weeks []struct {
		EndDate string `json:"endDate"`
	}
	var out struct {
		Data *struct {
			Search *struct {
				Items []struct {
					Title       string                 `json:"title"`
					TitleID     json.RawMessage        `json:"titleId"`
					Category    string                 `json:"category"`
					GlobalData  *struct{ Weeks weeks } `json:"globalData"`
					CountryData []struct {
						Weeks weeks `json:"weeks"`
					} `json:"countryData"`
				} `json:"items"`
			} `json:"tudumTop10Search"`
		} `json:"data"`
		Errors []struct {
			Message    string `json:"message"`
			Extensions struct {
				ErrorDetail string `json:"errorDetail"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode netflix top 10 search: %w", err)
	}
	if len(out.Errors) > 0 {
		// e.g. PERSISTED_QUERY_NOT_FOUND after Netflix changes the page.
		return nil, fmt.Errorf("netflix top 10 search: %s %s", out.Errors[0].Message, out.Errors[0].Extensions.ErrorDetail)
	}
	if out.Data == nil || out.Data.Search == nil {
		return nil, errors.New("netflix top 10 search: empty response")
	}
	items := make([]NetflixSearchItem, 0, len(out.Data.Search.Items))
	for _, it := range out.Data.Search.Items {
		id := strings.Trim(string(it.TitleID), `"`)
		if !isDigits(id) {
			continue
		}
		item := NetflixSearchItem{NetflixID: id, Title: strings.TrimSpace(it.Title), Category: it.Category}
		if it.GlobalData != nil {
			for _, w := range it.GlobalData.Weeks {
				item.GlobalWeeks = append(item.GlobalWeeks, w.EndDate)
			}
		}
		for _, cd := range it.CountryData {
			for _, w := range cd.Weeks {
				item.CountryWeeks = append(item.CountryWeeks, w.EndDate)
			}
		}
		items = append(items, item)
	}
	return items, nil
}

var netflixTitlePathRe = regexp.MustCompile(`/title/(\d+)`)

// ShowID returns the Netflix ID of the show a season ID belongs to (its
// public title page redirects there), or the ID itself when it is not a
// season.
func (c *NetflixTop10Search) ShowID(ctx context.Context, netflixID string) (string, error) {
	if !isDigits(netflixID) {
		return "", fmt.Errorf("invalid netflix id %q", netflixID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.titleBase+netflixID, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("netflix title page: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		loc, err := url.Parse(resp.Header.Get("Location"))
		if err != nil {
			return "", fmt.Errorf("netflix title redirect: %w", err)
		}
		if m := netflixTitlePathRe.FindStringSubmatch(loc.Path); m != nil {
			return m[1], nil
		}
		return netflixID, nil // e.g. a locale redirect: not a season
	case resp.StatusCode == http.StatusOK:
		return netflixID, nil
	default:
		return "", fmt.Errorf("netflix title page status %d", resp.StatusCode)
	}
}
