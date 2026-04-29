package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

var ErrTop10NotFound = errors.New("top10 data not found")

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

type Top10ReadService struct {
	repo *repository.FlixPatrolRepository
}

func NewTop10ReadService(repo *repository.FlixPatrolRepository) *Top10ReadService {
	return &Top10ReadService{repo: repo}
}

type Top10Query struct {
	Date        time.Time
	CountryCode string
	Provider    string
}

type Top10Response struct {
	Date     string        `json:"date"`
	Country  string        `json:"country"`
	Category string        `json:"category"`
	Provider string        `json:"provider,omitempty"`
	Items    []Top10Result `json:"items"`
}

type Top10Result struct {
	Rank              int64   `json:"rank"`
	Provider          string  `json:"provider"`
	Slug              string  `json:"slug"`
	Name              string  `json:"name"`
	Kind              string  `json:"kind"`
	TMDBID            *string `json:"tmdb_id,omitempty"`
	IMDbID            *string `json:"imdb_id,omitempty"`
	RottenTomatoesURL *string `json:"rotten_tomatoes_url,omitempty"`
}

func (s *Top10ReadService) GetMoviesByProvider(ctx context.Context, q Top10Query) (*Top10Response, error) {
	return s.getByProvider(ctx, q, "movies")
}

func (s *Top10ReadService) GetMoviesAllProviders(ctx context.Context, q Top10Query) (*Top10Response, error) {
	return s.getAllProviders(ctx, q, "movies")
}

func (s *Top10ReadService) GetTVShowsByProvider(ctx context.Context, q Top10Query) (*Top10Response, error) {
	return s.getByProvider(ctx, q, "tv_shows")
}

func (s *Top10ReadService) GetTVShowsAllProviders(ctx context.Context, q Top10Query) (*Top10Response, error) {
	return s.getAllProviders(ctx, q, "tv_shows")
}

func (s *Top10ReadService) getByProvider(ctx context.Context, q Top10Query, category string) (*Top10Response, error) {
	countryCode, err := validateQuery(q, true)
	if err != nil {
		return nil, err
	}

	items, err := s.repo.ListTop10ByProvider(ctx, q.Date.UTC(), countryCode, strings.TrimSpace(q.Provider), category)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrTop10NotFound
	}

	return buildResponse(q.Date, countryCode, category, q.Provider, items), nil
}

func (s *Top10ReadService) getAllProviders(ctx context.Context, q Top10Query, category string) (*Top10Response, error) {
	countryCode, err := validateQuery(q, false)
	if err != nil {
		return nil, err
	}

	items, err := s.repo.ListTop10AllProviders(ctx, q.Date.UTC(), countryCode, category)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrTop10NotFound
	}

	return buildResponse(q.Date, countryCode, category, "", items), nil
}

func validateQuery(q Top10Query, providerRequired bool) (string, error) {
	if q.Date.IsZero() {
		return "", &ValidationError{Message: "date is required (YYYY-MM-DD)"}
	}
	country := strings.TrimSpace(q.CountryCode)
	if country == "" {
		return "", &ValidationError{Message: "country is required (ISO code, e.g. US)"}
	}
	countryCode, err := flixpatrol.GetCountryCodeFromISO(country)
	if err != nil {
		return "", &ValidationError{Message: fmt.Sprintf("invalid country code: %s", country)}
	}
	if providerRequired && strings.TrimSpace(q.Provider) == "" {
		return "", &ValidationError{Message: "provider is required"}
	}
	return string(countryCode), nil
}

func buildResponse(date time.Time, countryCode string, category string, provider string, items []repository.Top10Item) *Top10Response {
	out := &Top10Response{
		Date:     date.UTC().Format("2006-01-02"),
		Country:  countryCode,
		Category: category,
		Provider: strings.TrimSpace(provider),
		Items:    make([]Top10Result, 0, len(items)),
	}
	for _, item := range items {
		out.Items = append(out.Items, Top10Result{
			Rank:              item.Rank,
			Provider:          item.StreamingProvider,
			Slug:              item.Slug,
			Name:              item.Name,
			Kind:              item.Kind,
			TMDBID:            item.TMDBID,
			IMDbID:            item.IMDbID,
			RottenTomatoesURL: item.RottenTomatoesURL,
		})
	}
	return out
}

