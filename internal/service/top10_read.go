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
	Date        time.Time // zero = latest scraped chart
	CountryCode string
	Provider    string
}

type Top10Response struct {
	Date     string        `json:"date"` // requested date, or the newest item date when date was omitted
	Country  string        `json:"country"`
	Category string        `json:"category"`
	Provider string        `json:"provider,omitempty"`
	Items    []Top10Result `json:"items"`
}

type Top10Result struct {
	Date              string  `json:"date"`
	Rank              int64   `json:"rank"`
	PreviousRank      *int64  `json:"previous_rank"` // rank on the chart's previous scraped date; null = new entry
	DaysInTop10       int64   `json:"days_in_top10"` // dates on this chart up to and including date
	Provider          string  `json:"provider"`
	TitleID           int64   `json:"title_id"`
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

	items, err := s.repo.ListTop10ByProvider(ctx, datePtr(q.Date), countryCode, strings.TrimSpace(q.Provider), category)
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

	items, err := s.repo.ListTop10AllProviders(ctx, datePtr(q.Date), countryCode, category)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrTop10NotFound
	}

	return buildResponse(q.Date, countryCode, category, "", items), nil
}

func validateQuery(q Top10Query, providerRequired bool) (string, error) {
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
	if date.IsZero() {
		for _, item := range items {
			if item.RankedOn.After(date) {
				date = item.RankedOn
			}
		}
	}
	out := &Top10Response{
		Date:     date.UTC().Format("2006-01-02"),
		Country:  countryCode,
		Category: category,
		Provider: strings.TrimSpace(provider),
		Items:    make([]Top10Result, 0, len(items)),
	}
	for _, item := range items {
		out.Items = append(out.Items, Top10Result{
			Date:              item.RankedOn.UTC().Format("2006-01-02"),
			Rank:              item.Rank,
			PreviousRank:      item.PreviousRank,
			DaysInTop10:       item.DaysInTop10,
			Provider:          item.StreamingProvider,
			TitleID:           item.TitleID,
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

// datePtr maps the zero date ("latest") to nil.
func datePtr(d time.Time) *time.Time {
	if d.IsZero() {
		return nil
	}
	return &d
}
