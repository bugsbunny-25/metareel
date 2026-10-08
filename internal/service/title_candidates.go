package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// TitleCandidatesService suggests TMDB / IMDb / Rotten Tomatoes mappings for
// a scraped title by searching JustWatch, TMDB and RT, for fixing mappings by
// hand in the admin UI.
type TitleCandidatesService struct {
	log    *slog.Logger
	titles *repository.FlixPatrolRepository
	jw     *client.JustWatchClient
	tmdb   *client.TMDBClient
	rt     *client.RottenTomatoesClient
}

func NewTitleCandidatesService(log *slog.Logger, titles *repository.FlixPatrolRepository, jw *client.JustWatchClient, tmdb *client.TMDBClient, rt *client.RottenTomatoesClient) *TitleCandidatesService {
	return &TitleCandidatesService{log: log, titles: titles, jw: jw, tmdb: tmdb, rt: rt}
}

type TitleCandidatesQuery struct {
	Name string // default: the title's name
	Year int    // default: the year suffix of the slug, if any
	Kind string // movie | tv_show; default: the title's kind
}

type TitleCandidatesResponse struct {
	Query          CandidateQuery     `json:"query"`
	JustWatch      []MappingCandidate `json:"justwatch"`
	TMDB           []MappingCandidate `json:"tmdb"`
	RottenTomatoes []RTCandidate      `json:"rotten_tomatoes"`
	Errors         map[string]string  `json:"errors,omitempty"` // provider -> error, if a search failed
}

type CandidateQuery struct {
	Name string `json:"name"`
	Year int    `json:"year,omitempty"`
	Kind string `json:"kind"`
}

type MappingCandidate struct {
	Title  string  `json:"title"`
	Year   *int    `json:"year"`
	Kind   string  `json:"kind"` // movie | tv_show
	TmdbID *string `json:"tmdb_id"`
	ImdbID *string `json:"imdb_id"`
	URL    *string `json:"url"`
}

type RTCandidate struct {
	Slug          string `json:"slug"` // value for rt_url, e.g. "m/bugonia"
	Title         string `json:"title"`
	Year          int    `json:"year"`
	URL           string `json:"url"`
	CriticsScore  *int64 `json:"critics_score"`
	AudienceScore *int64 `json:"audience_score"`
}

func (s *TitleCandidatesService) Search(ctx context.Context, titleID int64, q TitleCandidatesQuery) (*TitleCandidatesResponse, error) {
	t, err := s.titles.GetTitleByID(ctx, titleID)
	if err != nil {
		return nil, err
	}
	query := CandidateQuery{Name: strings.TrimSpace(q.Name), Year: q.Year, Kind: q.Kind}
	if query.Name == "" {
		query.Name = t.Name
		if query.Year == 0 {
			query.Year = flixpatrol.YearFromSlug(t.Slug)
		}
	}
	if query.Kind == "" {
		query.Kind = t.Kind
	}
	if query.Kind != "movie" && query.Kind != "tv_show" {
		return nil, &ValidationError{Message: "invalid kind, expected movie or tv_show"}
	}

	out := &TitleCandidatesResponse{Query: query, JustWatch: []MappingCandidate{}, TMDB: []MappingCandidate{}, RottenTomatoes: []RTCandidate{}}
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	fail := func(provider string, err error) {
		mu.Lock()
		defer mu.Unlock()
		if out.Errors == nil {
			out.Errors = map[string]string{}
		}
		out.Errors[provider] = err.Error()
		s.log.Warn("mapping candidate search failed", slog.String("provider", provider), slog.String("name", query.Name), slog.Any("err", err))
	}

	wg.Add(3)
	go func() {
		defer wg.Done()
		c, err := s.searchJustWatch(ctx, query)
		if err != nil {
			fail("justwatch", err)
			return
		}
		mu.Lock()
		out.JustWatch = c
		mu.Unlock()
	}()
	go func() {
		defer wg.Done()
		c, err := s.searchTMDB(ctx, query)
		if err != nil {
			fail("tmdb", err)
			return
		}
		mu.Lock()
		out.TMDB = c
		mu.Unlock()
	}()
	go func() {
		defer wg.Done()
		c, err := s.searchRT(ctx, query)
		if err != nil {
			fail("rottentomatoes", err)
			return
		}
		mu.Lock()
		out.RottenTomatoes = c
		mu.Unlock()
	}()
	wg.Wait()
	return out, nil
}

func (s *TitleCandidatesService) searchJustWatch(ctx context.Context, q CandidateQuery) ([]MappingCandidate, error) {
	if s.jw == nil {
		return []MappingCandidate{}, nil
	}
	objectType := client.ObjectTypeMovie
	if q.Kind == "tv_show" {
		objectType = client.ObjectTypeTVShow
	}
	res, err := s.jw.GetTitlesByTopSearchPopular(ctx, q.Name, 10, justWatchCountry, "en", objectType, nil)
	if err != nil {
		return nil, err
	}
	out := []MappingCandidate{}
	for _, e := range res.PoupularTitles.Edges {
		c := e.Node.Content
		mc := MappingCandidate{Title: c.Title, Year: c.OriginalReleaseYear, Kind: q.Kind, TmdbID: nonEmpty(c.ExternalIds.TmdbId), ImdbID: nonEmpty(c.ExternalIds.ImdbId)}
		if c.FullPath != nil {
			u := justWatchWebURL + *c.FullPath
			mc.URL = &u
		}
		out = append(out, mc)
	}
	return out, nil
}

func (s *TitleCandidatesService) searchTMDB(ctx context.Context, q CandidateQuery) ([]MappingCandidate, error) {
	if s.tmdb == nil || !s.tmdb.Enabled() {
		return []MappingCandidate{}, nil
	}
	var (
		results []client.TMDBSearchResult
		err     error
	)
	tmdbKind := "movie"
	if q.Kind == "tv_show" {
		tmdbKind = "tv"
		results, err = s.tmdb.SearchTV(ctx, q.Name)
	} else {
		results, err = s.tmdb.SearchMovie(ctx, q.Name)
	}
	if err != nil {
		return nil, err
	}
	out := []MappingCandidate{}
	for i, r := range results {
		if i == 10 {
			break
		}
		id := strconv.Itoa(r.ID)
		u := fmt.Sprintf("https://www.themoviedb.org/%s/%s", tmdbKind, id)
		mc := MappingCandidate{Title: r.Title, Kind: q.Kind, TmdbID: &id, URL: &u}
		if r.Year > 0 {
			y := r.Year
			mc.Year = &y
		}
		out = append(out, mc)
	}
	return out, nil
}

func (s *TitleCandidatesService) searchRT(ctx context.Context, q CandidateQuery) ([]RTCandidate, error) {
	if s.rt == nil {
		return []RTCandidate{}, nil
	}
	kind := client.RTKindMovie
	if q.Kind == "tv_show" {
		kind = client.RTKindTV
	}
	hits, err := s.rt.Candidates(ctx, kind, q.Name, q.Year)
	if err != nil {
		return nil, err
	}
	out := []RTCandidate{}
	for i, h := range hits {
		if i == 10 {
			break
		}
		out = append(out, RTCandidate{Slug: h.Slug, Title: h.Title, Year: h.Year, URL: h.URL(), CriticsScore: h.CriticsScore, AudienceScore: h.AudienceScore})
	}
	return out, nil
}
