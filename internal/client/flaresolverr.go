package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/bugsbunny-25/metareel/internal/config"
)

// FlareSolverrClient fetches pages through a FlareSolverr instance
// (https://github.com/FlareSolverr/FlareSolverr), which loads them in a real
// browser so that Cloudflare challenge pages are cleared before the HTML is
// returned.
type FlareSolverrClient struct {
	http       *resty.Client
	endpoint   string
	maxTimeout time.Duration
	session    string
	sessionTTL time.Duration
}

// NewFlareSolverr returns nil when cfg.URL is empty, i.e. FlareSolverr is not
// configured.
func NewFlareSolverr(cfg config.FlareSolverr) *FlareSolverrClient {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	if endpoint == "" {
		return nil
	}
	if !strings.HasSuffix(endpoint, "/v1") {
		endpoint += "/v1"
	}
	return &FlareSolverrClient{
		// FlareSolverr enforces maxTimeout itself; leave headroom for browser
		// start-up before giving up on the HTTP call.
		http:       resty.New().SetTimeout(cfg.MaxTimeout + 30*time.Second),
		endpoint:   endpoint,
		maxTimeout: cfg.MaxTimeout,
		session:    strings.TrimSpace(cfg.Session),
		sessionTTL: cfg.SessionTTL,
	}
}

type flareSolverrRequest struct {
	Cmd               string `json:"cmd"`
	URL               string `json:"url"`
	MaxTimeout        int64  `json:"maxTimeout"`
	Session           string `json:"session,omitempty"`
	SessionTTLMinutes int64  `json:"session_ttl_minutes,omitempty"`
}

type flareSolverrResponse struct {
	Status         string `json:"status"`
	Message        string `json:"message"`
	Version        string `json:"version"`
	StartTimestamp int64  `json:"startTimestamp"` // unix ms
	EndTimestamp   int64  `json:"endTimestamp"`   // unix ms
	Solution       struct {
		URL      string            `json:"url"`
		Status   int               `json:"status"`
		Headers  map[string]string `json:"headers"`
		Response string            `json:"response"`
	} `json:"solution"`
}

// FlareSolverrPage is the page FlareSolverr ended up on after solving any
// challenge.
type FlareSolverrPage struct {
	URL         string
	Status      int
	ContentType string // often empty: FlareSolverr v3 does not report response headers
	Body        []byte
	Message     string        // FlareSolverr's note, e.g. "Challenge solved!" or "Challenge not detected!"
	SolveTime   time.Duration // time FlareSolverr spent in the browser
	Session     string
}

// Get loads pageURL in FlareSolverr's browser and returns the rendered HTML.
func (c *FlareSolverrClient) Get(ctx context.Context, pageURL string) (*FlareSolverrPage, error) {
	req := flareSolverrRequest{
		Cmd:        "request.get",
		URL:        pageURL,
		MaxTimeout: c.maxTimeout.Milliseconds(),
		Session:    c.session,
	}
	if c.session != "" && c.sessionTTL > 0 {
		req.SessionTTLMinutes = int64(c.sessionTTL / time.Minute)
	}

	var out flareSolverrResponse
	resp, err := c.http.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&out).
		SetError(&out).
		Post(c.endpoint)
	if err != nil {
		return nil, fmt.Errorf("flaresolverr request.get: %w", err)
	}
	if out.Status != "ok" {
		return nil, fmt.Errorf("flaresolverr request.get http_status=%d status=%q message=%q session=%q", resp.StatusCode(), out.Status, out.Message, c.session)
	}

	page := &FlareSolverrPage{
		URL:     out.Solution.URL,
		Status:  out.Solution.Status,
		Body:    []byte(out.Solution.Response),
		Message: out.Message,
		Session: c.session,
	}
	if out.EndTimestamp > out.StartTimestamp {
		page.SolveTime = time.Duration(out.EndTimestamp-out.StartTimestamp) * time.Millisecond
	}
	if page.Status == 0 {
		// Older FlareSolverr builds omit the status of a solved page.
		page.Status = 200
	}
	for k, v := range out.Solution.Headers {
		if strings.EqualFold(k, "content-type") {
			page.ContentType = v
		}
	}
	return page, nil
}

// Endpoint is the /v1 URL requests are sent to.
func (c *FlareSolverrClient) Endpoint() string { return c.endpoint }

// Health calls FlareSolverr's index endpoint and returns its version.
func (c *FlareSolverrClient) Health(ctx context.Context) (string, error) {
	var out struct {
		Msg     string `json:"msg"`
		Version string `json:"version"`
	}
	resp, err := c.http.R().
		SetContext(ctx).
		SetResult(&out).
		Get(strings.TrimSuffix(c.endpoint, "/v1") + "/")
	if err != nil {
		return "", fmt.Errorf("flaresolverr health: %w", err)
	}
	if resp.IsError() {
		return "", fmt.Errorf("flaresolverr health status %d", resp.StatusCode())
	}
	return out.Version, nil
}
