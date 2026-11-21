package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
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
	tmdb     tmdbLister
	metadata *repository.MetadataRepository // optional: season counts of known shows
	now      func() time.Time
	mu       sync.Mutex
	cache    map[string]TitleListResponse
}

func NewTitleListsService(tmdb tmdbLister) *TitleListsService {
	return &TitleListsService{tmdb: tmdb, now: time.Now, cache: map[string]TitleListResponse{}}
}

// WithMetadata adds stored facts (season count, logo, certification) of
// titles with stored TMDB details.
func (s *TitleListsService) WithMetadata(metadata *repository.MetadataRepository) *TitleListsService {
	s.metadata = metadata
	return s
}

// withStoredFacts returns a copy of list with each known title's season
// count (TV), logo and certification in country ("" = none); the cached list
// itself is never changed.
func (s *TitleListsService) withStoredFacts(ctx context.Context, list *TitleListResponse, country string) (*TitleListResponse, error) {
	if s.metadata == nil || list == nil {
		return list, nil
	}
	var refs []repository.TmdbRef
	for _, it := range list.Items {
		refs = append(refs, repository.TmdbRef{Kind: it.Kind, ID: it.ID})
	}
	if len(refs) == 0 {
		return list, nil
	}
	meta, err := s.metadata.GetMany(ctx, refs)
	if err != nil {
		return nil, err
	}
	out := &TitleListResponse{Items: append([]client.TMDBListItem(nil), list.Items...), FetchedAt: list.FetchedAt}
	for i := range out.Items {
		it := &out.Items[i]
		m := meta[repository.TmdbRef{Kind: it.Kind, ID: it.ID}.String()]
		if m == nil {
			continue
		}
		if it.Kind == "tv" && m.NumberOfSeasons != nil && *m.NumberOfSeasons > 0 {
			it.SeasonCount = m.NumberOfSeasons
		}
		it.LogoPath = nonEmpty(m.LogoPath)
		if c := m.Certifications[country]; country != "" && c != "" {
			it.Certification = &c
		}
	}
	return out, nil
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
	list, err := s.cached(ctx, "trending:"+kind, func() ([]client.TMDBListItem, error) {
		return s.tmdb.Trending(ctx, kind, "day")
	})
	if err != nil {
		return nil, err
	}
	return s.withStoredFacts(ctx, list, "")
}

// InCinemas: the films TMDB lists as now playing in a country.
func (s *TitleListsService) InCinemas(ctx context.Context, country string) (*TitleListResponse, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if !countryCodeRe.MatchString(country) {
		return nil, &ValidationError{Message: "country must be a two-letter code"}
	}
	list, err := s.cached(ctx, "cinemas:"+country, func() ([]client.TMDBListItem, error) {
		return s.tmdb.NowPlaying(ctx, country)
	})
	if err != nil {
		return nil, err
	}
	return s.withStoredFacts(ctx, list, country)
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
