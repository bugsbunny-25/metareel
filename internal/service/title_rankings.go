package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

var ErrTitleNotFound = errors.New("title not found")

// MaxTitleRankingsBatch caps how many titles one batch request may ask for.
const MaxTitleRankingsBatch = 50

// TmdbRef identifies a title by TMDB ID. TMDB movie and TV IDs are separate
// namespaces, so the kind is part of the key.
type TmdbRef struct {
	Kind string // "movie" | "tv" (TMDB naming)
	ID   string
}

func (r TmdbRef) String() string { return r.Kind + ":" + r.ID }

// ParseTmdbRef validates a TMDB kind ("movie" or "tv") and numeric ID.
func ParseTmdbRef(kind, id string) (TmdbRef, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	id = strings.TrimSpace(id)
	if kind != "movie" && kind != "tv" {
		return TmdbRef{}, &ValidationError{Message: fmt.Sprintf("invalid kind %q, expected movie or tv", kind)}
	}
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		return TmdbRef{}, &ValidationError{Message: fmt.Sprintf("invalid tmdb id %q, expected a positive integer", id)}
	}
	return TmdbRef{Kind: kind, ID: id}, nil
}

// ParseTmdbRefList parses "movie:425,tv:154385".
func ParseTmdbRefList(raw string) ([]TmdbRef, error) {
	var refs []TmdbRef
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kind, id, ok := strings.Cut(part, ":")
		if !ok {
			return nil, &ValidationError{Message: fmt.Sprintf("invalid id %q, expected kind:tmdb_id (e.g. movie:425)", part)}
		}
		ref, err := ParseTmdbRef(kind, id)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// titleKindForTmdb maps TMDB's kind naming to titles.kind.
func titleKindForTmdb(kind string) string {
	if kind == "tv" {
		return "tv_show"
	}
	return "movie"
}

type RankingHistoryQuery struct {
	CountryCode string     // optional ISO code
	From        *time.Time // optional, inclusive
	To          *time.Time // optional, inclusive
}

type TitleRankingsResponse struct {
	Kind     string            `json:"kind"` // movie | tv
	TmdbID   string            `json:"tmdb_id"`
	Titles   []RankedTitle     `json:"titles"`   // FlixPatrol titles mapped to this TMDB ID
	Charts   []ChartSummary    `json:"charts"`   // one per country/provider/category the title charted on
	Rankings []RankingSnapshot `json:"rankings"` // every chart appearance, oldest first
}

type RankedTitle struct {
	Slug              string  `json:"slug"`
	Name              string  `json:"name"`
	IMDbID            *string `json:"imdb_id,omitempty"`
	RottenTomatoesURL *string `json:"rotten_tomatoes_url,omitempty"`
}

type ChartSummary struct {
	Country     string `json:"country"`
	Provider    string `json:"provider"`
	Category    string `json:"category"`
	DaysInTop10 int64  `json:"days_in_top10"` // within the requested range
	BestRank    int64  `json:"best_rank"`
	FirstDate   string `json:"first_date"`
	LastDate    string `json:"last_date"`
	LastRank    int64  `json:"last_rank"` // rank on last_date
}

type RankingSnapshot struct {
	Date     string `json:"date"`
	Country  string `json:"country"`
	Provider string `json:"provider"`
	Category string `json:"category"`
	Rank     int64  `json:"rank"`
	Slug     string `json:"slug"`
}

type TitleRankingsBatchResponse struct {
	Items    []TitleRankingsResponse `json:"items"`
	NotFound []string                `json:"not_found"` // "kind:tmdb_id" with no mapped title
}

// GetTitleRankings returns the ranking history of one TMDB title, or
// ErrTitleNotFound if no scraped title is mapped to it.
func (s *Top10ReadService) GetTitleRankings(ctx context.Context, ref TmdbRef, q RankingHistoryQuery) (*TitleRankingsResponse, error) {
	out, err := s.GetTitleRankingsBatch(ctx, []TmdbRef{ref}, q)
	if err != nil {
		return nil, err
	}
	if len(out.Items) == 0 {
		return nil, ErrTitleNotFound
	}
	return &out.Items[0], nil
}

// GetTitleRankingsBatch returns ranking histories for several TMDB titles in
// request order; refs with no mapped title are listed in NotFound.
func (s *Top10ReadService) GetTitleRankingsBatch(ctx context.Context, refs []TmdbRef, q RankingHistoryQuery) (*TitleRankingsBatchResponse, error) {
	refs = dedupeRefs(refs)
	if len(refs) == 0 {
		return nil, &ValidationError{Message: "at least one id is required"}
	}
	if len(refs) > MaxTitleRankingsBatch {
		return nil, &ValidationError{Message: fmt.Sprintf("at most %d ids per request", MaxTitleRankingsBatch)}
	}
	filter, err := validateRankingHistoryQuery(q)
	if err != nil {
		return nil, err
	}

	tmdbIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		tmdbIDs = append(tmdbIDs, ref.ID)
	}
	titles, err := s.repo.ListTitlesByTmdbIDs(ctx, tmdbIDs)
	if err != nil {
		return nil, err
	}

	byRef := make(map[TmdbRef]*TitleRankingsResponse, len(refs))
	refByTitleID := make(map[int64]TmdbRef)
	slugByTitleID := make(map[int64]string)
	var titleIDs []int64
	for _, ref := range refs {
		for _, t := range titles {
			if t.TmdbID.String != ref.ID || t.Kind != titleKindForTmdb(ref.Kind) {
				continue
			}
			resp := byRef[ref]
			if resp == nil {
				resp = &TitleRankingsResponse{Kind: ref.Kind, TmdbID: ref.ID, Titles: []RankedTitle{}, Charts: []ChartSummary{}, Rankings: []RankingSnapshot{}}
				byRef[ref] = resp
			}
			rt := RankedTitle{Slug: t.Slug, Name: t.Name}
			if t.ImdbID.Valid {
				rt.IMDbID = &t.ImdbID.String
			}
			if t.RtUrl.Valid {
				rt.RottenTomatoesURL = &t.RtUrl.String
			}
			resp.Titles = append(resp.Titles, rt)
			refByTitleID[t.ID] = ref
			slugByTitleID[t.ID] = t.Slug
			titleIDs = append(titleIDs, t.ID)
		}
	}

	rankings, err := s.repo.ListRankingsByTitleIDs(ctx, titleIDs, filter)
	if err != nil {
		return nil, err
	}
	for _, r := range rankings {
		resp := byRef[refByTitleID[r.TitleID]]
		resp.Rankings = append(resp.Rankings, RankingSnapshot{
			Date:     r.RankedOn.UTC().Format("2006-01-02"),
			Country:  r.Country,
			Provider: r.StreamingProvider,
			Category: r.Category,
			Rank:     r.Rank,
			Slug:     slugByTitleID[r.TitleID],
		})
	}

	out := &TitleRankingsBatchResponse{Items: []TitleRankingsResponse{}, NotFound: []string{}}
	for _, ref := range refs {
		resp := byRef[ref]
		if resp == nil {
			out.NotFound = append(out.NotFound, ref.String())
			continue
		}
		resp.Charts = summarizeCharts(resp.Rankings)
		out.Items = append(out.Items, *resp)
	}
	return out, nil
}

func validateRankingHistoryQuery(q RankingHistoryQuery) (repository.RankingHistoryFilter, error) {
	var f repository.RankingHistoryFilter
	if country := strings.TrimSpace(q.CountryCode); country != "" {
		code, err := flixpatrol.GetCountryCodeFromISO(country)
		if err != nil {
			return f, &ValidationError{Message: fmt.Sprintf("invalid country code: %s", country)}
		}
		f.Country = string(code)
	}
	if q.From != nil && q.To != nil && q.From.After(*q.To) {
		return f, &ValidationError{Message: "from must be on or before to"}
	}
	f.From, f.To = q.From, q.To
	return f, nil
}

func dedupeRefs(refs []TmdbRef) []TmdbRef {
	seen := make(map[TmdbRef]bool, len(refs))
	out := make([]TmdbRef, 0, len(refs))
	for _, r := range refs {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}

// summarizeCharts groups date-ordered snapshots per chart.
func summarizeCharts(rankings []RankingSnapshot) []ChartSummary {
	type key struct{ country, provider, category string }
	byChart := make(map[key]*ChartSummary)
	days := make(map[key]map[string]bool)
	var order []key
	for _, r := range rankings {
		k := key{r.Country, r.Provider, r.Category}
		c := byChart[k]
		if c == nil {
			c = &ChartSummary{Country: r.Country, Provider: r.Provider, Category: r.Category, BestRank: r.Rank, FirstDate: r.Date}
			byChart[k] = c
			days[k] = make(map[string]bool)
			order = append(order, k)
		}
		if r.Rank < c.BestRank {
			c.BestRank = r.Rank
		}
		if r.Date >= c.LastDate {
			if r.Date > c.LastDate || r.Rank < c.LastRank {
				c.LastRank = r.Rank
			}
			c.LastDate = r.Date
		}
		days[k][r.Date] = true
	}

	out := make([]ChartSummary, 0, len(order))
	for _, k := range order {
		c := byChart[k]
		c.DaysInTop10 = int64(len(days[k]))
		out = append(out, *c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Country != b.Country {
			return a.Country < b.Country
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		return a.Category < b.Category
	})
	return out
}

// GetTitleRankingsByID returns the ranking history of one scraped title,
// whether or not it is mapped to TMDB. sql.ErrNoRows if it does not exist.
func (s *Top10ReadService) GetTitleRankingsByID(ctx context.Context, titleID int64, q RankingHistoryQuery) (*TitleRankingsResponse, error) {
	filter, err := validateRankingHistoryQuery(q)
	if err != nil {
		return nil, err
	}
	t, err := s.repo.GetTitleByID(ctx, titleID)
	if err != nil {
		return nil, err
	}
	kind := "movie"
	if t.Kind == "tv_show" {
		kind = "tv"
	}
	rt := RankedTitle{Slug: t.Slug, Name: t.Name}
	if t.ImdbID.Valid {
		rt.IMDbID = &t.ImdbID.String
	}
	if t.RtUrl.Valid {
		rt.RottenTomatoesURL = &t.RtUrl.String
	}
	out := &TitleRankingsResponse{Kind: kind, TmdbID: t.TmdbID.String, Titles: []RankedTitle{rt}, Rankings: []RankingSnapshot{}}
	rankings, err := s.repo.ListRankingsByTitleIDs(ctx, []int64{t.ID}, filter)
	if err != nil {
		return nil, err
	}
	for _, r := range rankings {
		out.Rankings = append(out.Rankings, RankingSnapshot{
			Date: r.RankedOn.UTC().Format("2006-01-02"), Country: r.Country, Provider: r.StreamingProvider,
			Category: r.Category, Rank: r.Rank, Slug: t.Slug,
		})
	}
	out.Charts = summarizeCharts(out.Rankings)
	return out, nil
}
