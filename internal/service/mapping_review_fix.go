package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// Suggestions for the mapping review queue: other TMDB titles a risky
// mapping could be, ranked by the same signals the queue scores (name,
// release year against the first chart date, kind, an offer on the service
// it charted on), so the admin UI can fix one with a click. Applying a
// suggestion (or confirming the current mapping) is a manual fix through the
// existing title / Netflix title PATCH endpoints.

var ErrReviewSubjectNotFound = errors.New("not an automatic mapping in the review queue")

const (
	reviewSuggestionLimit = 5
	reviewOfferChecks     = 5 // top candidates checked for an offer on the charted service
	// Each call makes up to 8 searches and 5 offer lookups upstream, so
	// calls run one at a time, searches are spaced, and results are kept
	// for a few minutes.
	reviewSearchGap = 500 * time.Millisecond
	reviewCacheTTL  = 5 * time.Minute
	reviewCacheMax  = 200
)

// reviewSearcher finds TMDB candidates by name and kind (movie | tv_show).
type reviewSearcher interface {
	ReviewSearch(ctx context.Context, q CandidateQuery) ([]MappingCandidate, map[string]string)
}

// reviewOffers tells in which countries a TMDB title has an offer from the
// given TMDB provider IDs.
type reviewOffers interface {
	WatchProviderCountries(ctx context.Context, kind, tmdbID string, providerIDs []int) ([]string, error)
}

// ReviewFixService suggests fixes for mappings in the review queue.
type ReviewFixService struct {
	log    *slog.Logger
	repo   *repository.MappingRepository
	search reviewSearcher
	offers reviewOffers
	gap    time.Duration // between searches
	now    func() time.Time

	busy  chan struct{} // one call at a time
	mu    sync.Mutex
	cache map[string]reviewCached
}

type reviewCached struct {
	at  time.Time
	out *ReviewSuggestions
}

func NewReviewFixService(log *slog.Logger, repo *repository.MappingRepository, search reviewSearcher, offers reviewOffers) *ReviewFixService {
	return &ReviewFixService{log: log, repo: repo, search: search, offers: offers, gap: reviewSearchGap, now: time.Now,
		busy: make(chan struct{}, 1), cache: map[string]reviewCached{}}
}

// ReviewSearch searches JustWatch and TMDB (not Rotten Tomatoes) for
// candidates with a TMDB ID; failures are returned per provider.
func (s *TitleCandidatesService) ReviewSearch(ctx context.Context, q CandidateQuery) ([]MappingCandidate, map[string]string) {
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results = map[string][]MappingCandidate{}
		errs    map[string]string
	)
	run := func(name string, f func(context.Context, CandidateQuery) ([]MappingCandidate, error)) {
		defer wg.Done()
		c, err := f(ctx, q)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			if errs == nil {
				errs = map[string]string{}
			}
			errs[name] = "search failed"
			s.log.Warn("review suggestion search failed", slog.String("provider", name), slog.String("name", q.Name), slog.Any("err", err))
			return
		}
		results[name] = c
	}
	wg.Add(2)
	go run("tmdb", s.searchTMDB)
	go run("justwatch", s.searchJustWatch)
	wg.Wait()
	// TMDB first: its hits carry release dates.
	return append(results["tmdb"], results["justwatch"]...), errs
}

// ReviewSuggestion is another TMDB title a mapping could be.
type ReviewSuggestion struct {
	TmdbKind string `json:"tmdb_kind"` // movie | tv
	TmdbID   string `json:"tmdb_id"`
	Title    string `json:"title"`
	Year     *int   `json:"year"`
	// OnProvider: TMDB shows an offer from the service it charted on
	// (any country); null when not checked.
	OnProvider *bool    `json:"on_provider"`
	Score      int      `json:"score"`
	Reasons    []string `json:"reasons"`
	URL        *string  `json:"url,omitempty"`
}

type ReviewSuggestions struct {
	Subject      string             `json:"subject"`
	ID           int64              `json:"id"`
	Name         string             `json:"name"`
	Kind         string             `json:"kind"` // movie | tv_show
	TmdbKind     string             `json:"tmdb_kind"`
	TmdbID       string             `json:"tmdb_id"`
	FirstCharted string             `json:"first_charted,omitempty"`
	Providers    []string           `json:"providers"`
	Suggestions  []ReviewSuggestion `json:"suggestions"`
	Errors       map[string]string  `json:"errors,omitempty"`
}

// Suggestions ranks other TMDB titles for one automatic mapping.
func (s *ReviewFixService) Suggestions(ctx context.Context, subject string, id int64) (*ReviewSuggestions, error) {
	if subject != "flixpatrol" && subject != "netflix" {
		return nil, &ValidationError{Message: fmt.Sprintf("invalid subject %q, expected flixpatrol or netflix", subject)}
	}
	if id <= 0 {
		return nil, &ValidationError{Message: "invalid id"}
	}
	select {
	case s.busy <- struct{}{}:
		defer func() { <-s.busy }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	cacheKey := subject + ":" + strconv.FormatInt(id, 10)
	s.mu.Lock()
	if hit, ok := s.cache[cacheKey]; ok && s.now().Sub(hit.at) < reviewCacheTTL {
		s.mu.Unlock()
		return hit.out, nil
	}
	s.mu.Unlock()
	candidates, err := s.repo.ReviewCandidates(ctx)
	if err != nil {
		return nil, err
	}
	var c *repository.ReviewCandidate
	for i := range candidates {
		if candidates[i].Subject == subject && candidates[i].ID == id {
			c = &candidates[i]
			break
		}
	}
	if c == nil {
		return nil, ErrReviewSubjectNotFound
	}
	out := &ReviewSuggestions{Subject: subject, ID: id, Name: c.Name, Kind: c.Kind, TmdbKind: c.TmdbKind, TmdbID: c.TmdbID,
		FirstCharted: c.FirstCharted, Providers: c.Providers, Suggestions: []ReviewSuggestion{}}
	if out.Providers == nil {
		out.Providers = []string{}
	}
	name := strings.TrimSpace(decodeEntities(c.Name))
	names := []string{name}
	if base, season := flixpatrol.SeasonFromName(name); season > 0 && base != "" {
		names = append(names, base)
	}
	kinds := []string{c.Kind, "movie"}
	if c.Kind == "movie" {
		kinds[1] = "tv_show"
	}
	type key struct{ kind, id string }
	current := key{c.TmdbKind, c.TmdbID}
	hits := map[key]MappingCandidate{}
	var order []key
	searches := 0
	for _, kind := range kinds {
		for _, n := range names {
			if searches > 0 && s.gap > 0 {
				if err := sleepCtx(ctx, s.gap); err != nil {
					return nil, err
				}
			}
			searches++
			found, errs := s.search.ReviewSearch(ctx, CandidateQuery{Name: n, Kind: kind})
			for p, e := range errs {
				if out.Errors == nil {
					out.Errors = map[string]string{}
				}
				out.Errors[p] = e
			}
			for _, h := range found {
				if h.TmdbID == nil || !tmdbNumericRe.MatchString(*h.TmdbID) {
					continue // no TMDB ID, or not a plain number (it goes into URLs and mappings)
				}
				k := key{tmdbKindOf(flixpatrol.TitleKind(h.Kind)), *h.TmdbID}
				if k == current {
					continue
				}
				prev, ok := hits[k]
				switch {
				case !ok:
					order = append(order, k)
					hits[k] = h
				case prev.ReleaseDate == nil && h.ReleaseDate != nil: // a hit with a date tells more
					hits[k] = h
				}
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	found := make([]ReviewSuggestion, 0, len(order))
	for _, k := range order {
		found = append(found, scoreSuggestion(*c, names, k.kind, k.id, hits[k]))
	}
	sortSuggestions(found)
	// The strongest signal is an offer on the service it charted on; check
	// the best few (one TMDB request each).
	providerIDs := chartedProviderIDs(c.Providers)
	if s.offers != nil && len(providerIDs) > 0 {
		for i := range found[:min(len(found), reviewOfferChecks)] {
			countries, err := s.offers.WatchProviderCountries(ctx, found[i].TmdbKind, found[i].TmdbID, providerIDs)
			if err != nil {
				s.log.Warn("review suggestion offer check failed", slog.String("tmdb_id", found[i].TmdbID), slog.Any("err", err))
				continue
			}
			on := len(countries) > 0
			found[i].OnProvider = &on
			if on {
				found[i].Score += 35
				found[i].Reasons = append(found[i].Reasons, fmt.Sprintf("on %s (%d countries)", providerList(c.Providers), len(countries)))
			}
		}
		sortSuggestions(found)
	}
	out.Suggestions = found[:min(len(found), reviewSuggestionLimit)]
	if out.Errors == nil { // a failed search is worth retrying, so not kept
		s.mu.Lock()
		if len(s.cache) >= reviewCacheMax {
			s.cache = map[string]reviewCached{}
		}
		s.cache[cacheKey] = reviewCached{at: s.now(), out: out}
		s.mu.Unlock()
	}
	return out, nil
}

func scoreSuggestion(c repository.ReviewCandidate, names []string, tmdbKind, tmdbID string, h MappingCandidate) ReviewSuggestion {
	sg := ReviewSuggestion{TmdbKind: tmdbKind, TmdbID: tmdbID, Title: h.Title, Year: h.Year, URL: h.URL, Reasons: []string{}}
	title := normalizeTitle(h.Title)
	exact, partial := false, false
	for _, n := range names {
		want := normalizeTitle(n)
		if want == "" {
			continue
		}
		if want == title {
			exact = true
		} else if strings.Contains(title, want) || strings.Contains(want, title) {
			partial = true
		}
	}
	switch {
	case exact:
		sg.Score += 40
		sg.Reasons = append(sg.Reasons, "same name")
	case partial:
		sg.Score += 15
		sg.Reasons = append(sg.Reasons, "similar name")
	}
	if tmdbKindOf(flixpatrol.TitleKind(c.Kind)) == tmdbKind {
		sg.Score += 10
		sg.Reasons = append(sg.Reasons, "same kind as the chart")
	}
	// When it came out against when it first charted: a release date when
	// TMDB gave one (a title can't chart before it is out), else the year.
	chartYear := dateYear(c.FirstCharted)
	if h.ReleaseDate != nil && c.FirstCharted != "" {
		if days := windowDays(*h.ReleaseDate, c.FirstCharted); days != nil {
			switch d := *days; {
			case d < 0:
				sg.Score -= 50
				sg.Reasons = append(sg.Reasons, fmt.Sprintf("released %s, after it first charted (%s)", *h.ReleaseDate, c.FirstCharted))
			case d <= 540:
				sg.Score += 15
				sg.Reasons = append(sg.Reasons, fmt.Sprintf("released %s, shortly before it charted", *h.ReleaseDate))
			case d <= 3*365:
				sg.Score += 5
			}
		}
	} else if h.Year != nil && *h.Year > 0 && chartYear > 0 {
		switch y := *h.Year; {
		case y > chartYear:
			sg.Score -= 50
			sg.Reasons = append(sg.Reasons, fmt.Sprintf("released %d, after it first charted", y))
		case chartYear-y == 1:
			sg.Score += 15
			sg.Reasons = append(sg.Reasons, fmt.Sprintf("released %d, the year before it charted", y))
		case chartYear-y <= 3:
			sg.Score += 5 // includes the chart year itself: it may have come out after
		}
	}
	if fp := dateYear(c.FPPremiereDate); fp > 1900 && h.Year != nil && absInt(fp-*h.Year) <= maxYearDrift {
		sg.Score += 20
		sg.Reasons = append(sg.Reasons, fmt.Sprintf("FlixPatrol's premiere year (%d)", fp))
	}
	return sg
}

func sortSuggestions(s []ReviewSuggestion) {
	sort.SliceStable(s, func(i, j int) bool { return s[i].Score > s[j].Score })
}

// chartedProviderIDs are the TMDB provider IDs of the services a title
// charted on.
func chartedProviderIDs(providers []string) []int {
	var out []int
	for _, p := range providers {
		for _, id := range providerOffers[p] {
			out = append(out, int(id))
		}
	}
	return out
}

func providerList(providers []string) string {
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, providerName(p))
	}
	return strings.Join(names, " / ")
}
