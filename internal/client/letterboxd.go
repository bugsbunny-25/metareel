package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Letterboxd links shared to the mobile app (docs/mobile-app-plan.md, B11):
// a boxd.it short link or a letterboxd.com film or review URL resolves to the
// film's TMDB id, which Letterboxd puts on the film page as
// data-tmdb-id / data-tmdb-type. A plain GET with no cookies; if Letterboxd
// answers with anything else (a challenge, an error), the lookup fails and
// the app falls back to a title search. No bot-protection evasion.

var (
	// ErrLetterboxdNotFilm: the link is a Letterboxd page but not a film
	// (a list, a tag, a profile).
	ErrLetterboxdNotFilm = errors.New("not a Letterboxd film link")
	// ErrLetterboxdNoTMDB: the film page has no TMDB id.
	ErrLetterboxdNoTMDB = errors.New("letterboxd film has no TMDB id")

	letterboxdFilmPathRe = regexp.MustCompile(`^/(?:[A-Za-z0-9_]+/)?film/([a-z0-9-]{1,120})(?:/|$)`)
	letterboxdTMDBIDRe   = regexp.MustCompile(`data-tmdb-id="(\d{1,12})"`)
	letterboxdTMDBTypeRe = regexp.MustCompile(`data-tmdb-type="(movie|tv)"`)
)

const (
	letterboxdMaxPage    = 3 << 20
	letterboxdMaxHops    = 3
	letterboxdHitTTL     = 24 * time.Hour
	letterboxdMissTTL    = time.Hour
	letterboxdCacheLimit = 2000
)

// LetterboxdRef is the TMDB title a Letterboxd film is.
type LetterboxdRef struct {
	Kind   string // movie | tv
	TmdbID string
}

type letterboxdCached struct {
	ref     LetterboxdRef
	err     error
	expires time.Time
}

// LetterboxdClient resolves Letterboxd links.
type LetterboxdClient struct {
	http      *http.Client
	userAgent string
	base      string // https://letterboxd.com (tests point it elsewhere)
	now       func() time.Time

	mu    sync.Mutex
	cache map[string]letterboxdCached // film slug -> answer
}

func NewLetterboxd(timeout time.Duration, userAgent string) *LetterboxdClient {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return &LetterboxdClient{
		http: &http.Client{
			Timeout: timeout,
			// Redirects are followed by hand, so each hop's host is checked.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		userAgent: userAgent,
		base:      "https://letterboxd.com",
		now:       time.Now,
		cache:     map[string]letterboxdCached{},
	}
}

// IsLetterboxdURL reports whether s is a boxd.it or letterboxd.com link.
func IsLetterboxdURL(s string) bool {
	u, err := url.Parse(strings.TrimSpace(s))
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && letterboxdHost(u.Hostname())
}

func letterboxdHost(h string) bool {
	h = strings.ToLower(h)
	return h == "boxd.it" || h == "letterboxd.com" || h == "www.letterboxd.com"
}

// Resolve turns a Letterboxd link into a TMDB reference.
func (c *LetterboxdClient) Resolve(ctx context.Context, link string) (LetterboxdRef, error) {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || !letterboxdHost(u.Hostname()) {
		return LetterboxdRef{}, ErrLetterboxdNotFilm
	}
	for hop := 0; strings.EqualFold(u.Hostname(), "boxd.it"); hop++ {
		if hop >= letterboxdMaxHops {
			return LetterboxdRef{}, fmt.Errorf("letterboxd: too many redirects")
		}
		next, err := c.redirect(ctx, u)
		if err != nil {
			return LetterboxdRef{}, err
		}
		if !letterboxdHost(next.Hostname()) {
			return LetterboxdRef{}, ErrLetterboxdNotFilm
		}
		u = next
	}
	m := letterboxdFilmPathRe.FindStringSubmatch(u.EscapedPath())
	if m == nil {
		return LetterboxdRef{}, ErrLetterboxdNotFilm
	}
	return c.film(ctx, m[1])
}

func (c *LetterboxdClient) redirect(ctx context.Context, u *url.URL) (*url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("letterboxd: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		return nil, fmt.Errorf("letterboxd short link: HTTP %d", resp.StatusCode)
	}
	loc, err := resp.Location()
	if err != nil {
		return nil, fmt.Errorf("letterboxd short link: %w", err)
	}
	return loc, nil
}

func (c *LetterboxdClient) film(ctx context.Context, slug string) (LetterboxdRef, error) {
	now := c.now()
	c.mu.Lock()
	if hit, ok := c.cache[slug]; ok && now.Before(hit.expires) {
		c.mu.Unlock()
		return hit.ref, hit.err
	}
	c.mu.Unlock()

	ref, err := c.fetchFilm(ctx, slug)
	// Cache answers, not transient failures (timeouts, HTTP errors).
	if err == nil || errors.Is(err, ErrLetterboxdNoTMDB) || errors.Is(err, ErrLetterboxdNotFilm) {
		ttl := letterboxdHitTTL
		if err != nil {
			ttl = letterboxdMissTTL
		}
		c.mu.Lock()
		if len(c.cache) > letterboxdCacheLimit {
			c.cache = map[string]letterboxdCached{}
		}
		c.cache[slug] = letterboxdCached{ref: ref, err: err, expires: now.Add(ttl)}
		c.mu.Unlock()
	}
	return ref, err
}

func (c *LetterboxdClient) fetchFilm(ctx context.Context, slug string) (LetterboxdRef, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/film/"+slug+"/", nil)
	if err != nil {
		return LetterboxdRef{}, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return LetterboxdRef{}, fmt.Errorf("letterboxd: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return LetterboxdRef{}, ErrLetterboxdNotFilm
	}
	if resp.StatusCode != http.StatusOK {
		return LetterboxdRef{}, fmt.Errorf("letterboxd film page: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, letterboxdMaxPage))
	if err != nil {
		return LetterboxdRef{}, fmt.Errorf("letterboxd film page: %w", err)
	}
	id := letterboxdTMDBIDRe.FindSubmatch(body)
	if id == nil {
		return LetterboxdRef{}, ErrLetterboxdNoTMDB
	}
	kind := "movie"
	if t := letterboxdTMDBTypeRe.FindSubmatch(body); t != nil {
		kind = string(t[1])
	}
	return LetterboxdRef{Kind: kind, TmdbID: string(id[1])}, nil
}
