package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Wikimedia reads article pageviews from the Wikimedia REST API
// (https://wikimedia.org/api/rest_v1/#/Pageviews%20data). No key; requests
// must identify the client, and daily data is complete about a day later.
type Wikimedia struct {
	http      *http.Client
	endpoint  string
	userAgent string
}

func NewWikimedia(timeout time.Duration) *Wikimedia {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &Wikimedia{http: &http.Client{Timeout: timeout}, endpoint: "https://wikimedia.org/api/rest_v1", userAgent: wikidataUserAgent}
}

// RateLimitedError: Wikimedia answered 429 (anonymous clients with a
// User-Agent may make 200 requests a minute); wait RetryAfter.
type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("wikimedia rate limit, retry after %s", e.RetryAfter)
}

// DailyViews is an article's user views on a day (YYYY-MM-DD, UTC).
type DailyViews struct {
	Day   string
	Views int64
}

// Pageviews returns an article's daily user (not bot) views on a project
// (e.g. en.wikipedia) between two days, inclusive. Nil when the project has
// no such article (or no views in the range).
func (w *Wikimedia) Pageviews(ctx context.Context, project, article string, from, to time.Time) ([]DailyViews, error) {
	title := strings.ReplaceAll(strings.TrimSpace(article), " ", "_")
	u := fmt.Sprintf("%s/metrics/pageviews/per-article/%s/all-access/user/%s/daily/%s/%s",
		w.endpoint, url.PathEscape(project), url.PathEscape(title), from.UTC().Format("20060102"), to.UTC().Format("20060102"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", w.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := w.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wikimedia pageviews: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		wait := 5 * time.Second
		if s, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && s > 0 {
			wait = time.Duration(s) * time.Second
		}
		return nil, &RateLimitedError{RetryAfter: min(wait, time.Minute)}
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("wikimedia pageviews status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		Items []struct {
			Timestamp string `json:"timestamp"` // YYYYMMDDHH
			Views     int64  `json:"views"`
		} `json:"items"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode wikimedia pageviews: %w", err)
	}
	views := make([]DailyViews, 0, len(out.Items))
	for _, it := range out.Items {
		if len(it.Timestamp) < 8 {
			continue
		}
		views = append(views, DailyViews{Day: it.Timestamp[:4] + "-" + it.Timestamp[4:6] + "-" + it.Timestamp[6:8], Views: it.Views})
	}
	return views, nil
}
