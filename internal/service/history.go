package service

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// HistoryService serves the history of a title's ratings, TMDB popularity
// and watch providers, and watch provider changes across titles.
type HistoryService struct {
	history  *repository.HistoryRepository
	metadata *repository.MetadataRepository
	now      func() time.Time
}

func NewHistoryService(history *repository.HistoryRepository, metadata *repository.MetadataRepository) *HistoryService {
	return &HistoryService{history: history, metadata: metadata, now: time.Now}
}

const (
	defaultTitleHistoryDays = 90
	maxTitleHistoryDays     = 366
)

// TitleHistoryQuery is a date range (nil = default: the last 90 days to today).
type TitleHistoryQuery struct {
	From, To *time.Time
}

// TitleHistoryResponse is a title's history over a range. Rating series
// (imdb, ratings) hold a point whenever a value changed; the first point may
// be before `from` (the value in effect at `from`). Popularity has a point
// per day of TMDB's export.
type TitleHistoryResponse struct {
	Kind       string                       `json:"kind"`
	TmdbID     string                       `json:"tmdb_id"`
	From       string                       `json:"from"`
	To         string                       `json:"to"`
	IMDb       *IMDbHistory                 `json:"imdb"`
	Ratings    []repository.RatingSeries    `json:"ratings"`
	Popularity []repository.PopularityPoint `json:"popularity"`
	// Wikipedia: daily user views of the English Wikipedia article (nil
	// when the title has none).
	Wikipedia *WikipediaHistory `json:"wikipedia"`
}

// WikipediaHistory is an article's daily pageviews.
type WikipediaHistory struct {
	Article string                     `json:"article"`
	URL     string                     `json:"url"`
	Points  []repository.PageviewPoint `json:"points"`
}

// IMDbHistory is the IMDb dataset's rating (value) and votes over time.
type IMDbHistory struct {
	IMDbID string                   `json:"imdb_id"`
	Points []repository.RatingPoint `json:"points"`
}

func (s *HistoryService) historyRange(q TitleHistoryQuery) (string, string, error) {
	to := s.now().UTC().Truncate(24 * time.Hour)
	if q.To != nil {
		to = q.To.UTC()
	}
	from := to.AddDate(0, 0, -defaultTitleHistoryDays)
	if q.From != nil {
		from = q.From.UTC()
	}
	if from.After(to) {
		return "", "", &ValidationError{Message: "from must not be after to"}
	}
	if to.Sub(from) > maxTitleHistoryDays*24*time.Hour {
		return "", "", &ValidationError{Message: fmt.Sprintf("range must be at most %d days", maxTitleHistoryDays)}
	}
	return repository.FormatDate(from), repository.FormatDate(to), nil
}

// TitleHistory returns a stored TMDB title's history (404 if unknown).
func (s *HistoryService) TitleHistory(ctx context.Context, ref TmdbRef, q TitleHistoryQuery) (*TitleHistoryResponse, error) {
	from, to, err := s.historyRange(q)
	if err != nil {
		return nil, err
	}
	meta, err := s.metadata.Get(ctx, ref)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, ErrTitleNotFound
	}
	out := &TitleHistoryResponse{Kind: ref.Kind, TmdbID: ref.ID, From: from, To: to,
		Ratings: []repository.RatingSeries{}}
	if meta.IMDbID != nil && *meta.IMDbID != "" {
		points, err := s.history.IMDbHistory(ctx, *meta.IMDbID, to)
		if err != nil {
			return nil, err
		}
		out.IMDb = &IMDbHistory{IMDbID: *meta.IMDbID, Points: fromStart(points, from)}
	}
	series, err := s.history.RatingHistory(ctx, ref, to)
	if err != nil {
		return nil, err
	}
	for _, sr := range series {
		if sr.Points = fromStart(sr.Points, from); len(sr.Points) > 0 {
			out.Ratings = append(out.Ratings, sr)
		}
	}
	if out.Popularity, err = s.history.PopularityHistory(ctx, ref, from, to); err != nil {
		return nil, err
	}
	if meta.EnwikiTitle != nil && *meta.EnwikiTitle != "" {
		points, err := s.history.Pageviews(ctx, ref, pageviewsProject, from, to)
		if err != nil {
			return nil, err
		}
		article := *meta.EnwikiTitle
		out.Wikipedia = &WikipediaHistory{Article: article, Points: points,
			URL: "https://en.wikipedia.org/wiki/" + url.PathEscape(strings.ReplaceAll(article, " ", "_"))}
	}
	return out, nil
}

// fromStart keeps the points on or after from, plus the last one before it
// (the value in effect at from). points are sorted by date.
func fromStart(points []repository.RatingPoint, from string) []repository.RatingPoint {
	i := 0
	for i < len(points) && points[i].Date < from {
		i++
	}
	if i > 0 {
		i--
	}
	out := points[i:]
	if out == nil {
		out = []repository.RatingPoint{}
	}
	return out
}

// --- availability changes ---------------------------------------------------------

// Offer monetization types (tmdb_watch_providers.monetization).
var monetizations = map[string]bool{"flatrate": true, "free": true, "ads": true, "rent": true, "buy": true}

// TitleAvailabilityHistoryResponse lists a title's watch provider changes.
type TitleAvailabilityHistoryResponse struct {
	Kind    string                  `json:"kind"`
	TmdbID  string                  `json:"tmdb_id"`
	Country *string                 `json:"country"`
	Events  []repository.OfferEvent `json:"events"`
}

const maxTitleOfferEvents = 500

// TitleAvailabilityHistory returns a stored TMDB title's offer changes
// (newest first, up to 500), optionally in one country.
func (s *HistoryService) TitleAvailabilityHistory(ctx context.Context, ref TmdbRef, country string) (*TitleAvailabilityHistoryResponse, error) {
	country, err := validCountry(country, false)
	if err != nil {
		return nil, err
	}
	meta, err := s.metadata.Get(ctx, ref)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, ErrTitleNotFound
	}
	events, err := s.history.TitleOfferEvents(ctx, ref, country, maxTitleOfferEvents)
	if err != nil {
		return nil, err
	}
	return &TitleAvailabilityHistoryResponse{Kind: ref.Kind, TmdbID: ref.ID, Country: optStr(country), Events: events}, nil
}

// AvailabilityChangesQuery filters offer changes across titles.
type AvailabilityChangesQuery struct {
	Since        *time.Time // default: 7 days ago
	Country      string
	Event        string // added | removed
	Monetization string // flatrate | free | ads | rent | buy
	ProviderID   string // TMDB provider ID
	Limit        int
}

// AvailabilityChangesResponse lists offer changes, newest first.
type AvailabilityChangesResponse struct {
	Since  string                  `json:"since"`
	Events []repository.OfferEvent `json:"events"`
}

// AvailabilityChanges returns watch provider changes across titles.
func (s *HistoryService) AvailabilityChanges(ctx context.Context, q AvailabilityChangesQuery) (*AvailabilityChangesResponse, error) {
	country, err := validCountry(q.Country, false)
	if err != nil {
		return nil, err
	}
	f := repository.OfferEventFilter{Country: country, Limit: int64(q.Limit)}
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 200
	}
	switch e := strings.ToLower(strings.TrimSpace(q.Event)); e {
	case "", repository.OfferAdded, repository.OfferRemoved:
		f.Event = e
	default:
		return nil, &ValidationError{Message: fmt.Sprintf("invalid event %q, expected added or removed", q.Event)}
	}
	if m := strings.ToLower(strings.TrimSpace(q.Monetization)); m != "" {
		if !monetizations[m] {
			return nil, &ValidationError{Message: fmt.Sprintf("invalid monetization %q, expected flatrate, free, ads, rent or buy", q.Monetization)}
		}
		f.Monetization = m
	}
	if raw := strings.TrimSpace(q.ProviderID); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return nil, &ValidationError{Message: fmt.Sprintf("invalid provider_id %q", q.ProviderID)}
		}
		f.ProviderID = id
	}
	since := s.now().UTC().AddDate(0, 0, -7)
	if q.Since != nil {
		since = q.Since.UTC()
	}
	f.Since = since.Format("2006-01-02 15:04:05")
	events, err := s.history.OfferEvents(ctx, f)
	if err != nil {
		return nil, err
	}
	return &AvailabilityChangesResponse{Since: since.Format(time.RFC3339), Events: events}, nil
}
