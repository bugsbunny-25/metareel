package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
)

// Title lists for the home screens: what is trending on TMDB today and what
// is in cinemas in a country. Fetched live from TMDB (nothing is stored) and
// kept in memory for listTTL, so a busy home screen costs TMDB one call per
// list and country every half hour.

// ErrListsUnavailable means TMDB couldn't be asked (no key) or failed.
var ErrListsUnavailable = errors.New("tmdb lists unavailable")

const listTTL = 30 * time.Minute

type tmdbLister interface {
	Enabled() bool
	Trending(ctx context.Context, kind, window string) ([]client.TMDBListItem, error)
	NowPlaying(ctx context.Context, region string) ([]client.TMDBListItem, error)
}

// TitleListResponse is a list of titles with the time it was fetched.
type TitleListResponse struct {
	Items     []client.TMDBListItem `json:"items"`
	FetchedAt time.Time             `json:"fetched_at"`
}

type TitleListsService struct {
	tmdb  tmdbLister
	now   func() time.Time
	mu    sync.Mutex
	cache map[string]TitleListResponse
}

func NewTitleListsService(tmdb tmdbLister) *TitleListsService {
	return &TitleListsService{tmdb: tmdb, now: time.Now, cache: map[string]TitleListResponse{}}
}

var countryCodeRe = regexp.MustCompile(`^[A-Z]{2}$`)

// Trending: kind all | movie | tv ("" = all), today's TMDB trending.
func (s *TitleListsService) Trending(ctx context.Context, kind string) (*TitleListResponse, error) {
	if kind == "" {
		kind = "all"
	}
	if kind != "all" && kind != "movie" && kind != "tv" {
		return nil, &ValidationError{Message: "kind must be all, movie or tv"}
	}
	return s.cached(ctx, "trending:"+kind, func() ([]client.TMDBListItem, error) {
		return s.tmdb.Trending(ctx, kind, "day")
	})
}

// InCinemas: the films TMDB lists as now playing in a country.
func (s *TitleListsService) InCinemas(ctx context.Context, country string) (*TitleListResponse, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if !countryCodeRe.MatchString(country) {
		return nil, &ValidationError{Message: "country must be a two-letter code"}
	}
	return s.cached(ctx, "cinemas:"+country, func() ([]client.TMDBListItem, error) {
		return s.tmdb.NowPlaying(ctx, country)
	})
}

func (s *TitleListsService) cached(ctx context.Context, key string, fetch func() ([]client.TMDBListItem, error)) (*TitleListResponse, error) {
	if s.tmdb == nil || !s.tmdb.Enabled() {
		return nil, ErrListsUnavailable
	}
	s.mu.Lock()
	hit, ok := s.cache[key]
	s.mu.Unlock()
	if ok && s.now().Sub(hit.FetchedAt) < listTTL {
		return &hit, nil
	}
	items, err := fetch()
	if err != nil {
		if ok { // serve the stale list rather than nothing
			return &hit, nil
		}
		return nil, errors.Join(ErrListsUnavailable, err)
	}
	if items == nil {
		items = []client.TMDBListItem{}
	}
	out := TitleListResponse{Items: items, FetchedAt: s.now().UTC()}
	s.mu.Lock()
	s.cache[key] = out
	s.mu.Unlock()
	return &out, nil
}
