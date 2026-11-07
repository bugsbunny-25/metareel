package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// SearchService finds metareel's titles (mapped TMDB titles) by any name
// they go by: TMDB's title and original title, FlixPatrol's and Netflix's
// names. A few thousand titles fit in memory, so the index is built from the
// database and rebuilt when its stamp (counts and latest updates of names,
// mappings and charts) changes; the stamp is checked at most every
// searchStampTTL.
type SearchService struct {
	repo *repository.SearchRepository
	now  func() time.Time

	mu        sync.Mutex
	index     *searchIndex
	stamp     string
	checkedAt time.Time
}

const (
	searchStampTTL     = 30 * time.Second
	defaultSearchLimit = 20
	maxSearchLimit     = 50
	maxSearchQuery     = 200 // bytes
	maxSearchWords     = 8   // query words scored (each costs a pass over every name)
	searchBuildTimeout = 20 * time.Second
)

func NewSearchService(repo *repository.SearchRepository) *SearchService {
	return &SearchService{repo: repo, now: time.Now}
}

// SearchQuery is a title search.
type SearchQuery struct {
	Q     string
	Kind  string // movie | tv | ""
	Limit int
}

// SearchResult is a title matching a search.
type SearchResult struct {
	Kind          string   `json:"kind"` // movie | tv (TMDB namespace)
	TmdbID        string   `json:"tmdb_id"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"original_title,omitempty"`
	Year          int      `json:"year,omitempty"`
	PosterPath    string   `json:"poster_path,omitempty"`
	Matched       string   `json:"matched"`      // the name that matched
	Sources       []string `json:"sources"`      // where the title charted: flixpatrol, netflix
	LastCharted   string   `json:"last_charted"` // YYYY-MM-DD (FlixPatrol date or Netflix week)
	Score         int      `json:"score"`        // match quality, 0-100
}

// SearchResponse lists results, best first.
type SearchResponse struct {
	Query string         `json:"query"`
	Items []SearchResult `json:"items"`
}

type searchName struct {
	norm   string   // folded name
	tokens []string // its words
	raw    string
}

type searchDoc struct {
	result SearchResult
	names  []searchName
}

type searchIndex struct {
	docs []*searchDoc
}

// foldName lowercases, strips accents, spells "&" as "and" and turns every
// other non-letter, non-digit into a space.
func foldName(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	folded, _, err := transform.String(t, s)
	if err != nil {
		folded = s
	}
	folded = strings.ReplaceAll(strings.ToLower(folded), "&", " and ")
	var b strings.Builder
	space := true
	for _, r := range folded {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// Search returns the titles best matching q (at least 2 characters).
func (s *SearchService) Search(ctx context.Context, q SearchQuery) (*SearchResponse, error) {
	if len(q.Q) > maxSearchQuery {
		return nil, &ValidationError{Message: fmt.Sprintf("q must be at most %d characters", maxSearchQuery)}
	}
	query := foldName(q.Q)
	if len([]rune(query)) < 2 {
		return nil, &ValidationError{Message: "q must have at least 2 letters or digits"}
	}
	switch q.Kind {
	case "", "movie", "tv":
	default:
		return nil, &ValidationError{Message: fmt.Sprintf("invalid kind %q, expected movie or tv", q.Kind)}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	limit = min(limit, maxSearchLimit)
	idx, err := s.currentIndex(ctx)
	if err != nil {
		return nil, err
	}
	qTokens := strings.Fields(query)
	if len(qTokens) > maxSearchWords {
		qTokens = qTokens[:maxSearchWords]
		query = strings.Join(qTokens, " ")
	}
	var hits []SearchResult
	for i, d := range idx.docs {
		if i%500 == 0 && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if q.Kind != "" && d.result.Kind != q.Kind {
			continue
		}
		best, matched := 0, ""
		for _, n := range d.names {
			if sc := matchScore(query, qTokens, n); sc > best {
				best, matched = sc, n.raw
			}
		}
		if best == 0 {
			continue
		}
		r := d.result
		r.Score, r.Matched = best, matched
		hits = append(hits, r)
	}
	sort.Slice(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.LastCharted != b.LastCharted {
			return a.LastCharted > b.LastCharted
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.Kind+a.TmdbID < b.Kind+b.TmdbID
	})
	out := &SearchResponse{Query: q.Q, Items: []SearchResult{}}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out.Items = append(out.Items, hits...)
	return out, nil
}

// matchScore rates how well a name matches the folded query: the whole name
// 100, the name starting with it 85, every query word starting a word of
// the name in order 70 / in any order 60, the query inside the name 40.
func matchScore(query string, qTokens []string, n searchName) int {
	switch {
	case n.norm == query:
		return 100
	case strings.HasPrefix(n.norm, query):
		return 85
	}
	all, inOrder, pos := true, true, 0
	for _, qt := range qTokens {
		if i := wordWithPrefix(n.tokens, qt, pos); i >= 0 {
			pos = i + 1
			continue
		}
		inOrder = false
		if wordWithPrefix(n.tokens, qt, 0) < 0 {
			all = false
			break
		}
	}
	switch {
	case all && inOrder:
		return 70
	case all:
		return 60
	case len(query) >= 3 && strings.Contains(n.norm, query):
		return 40
	}
	return 0
}

// wordWithPrefix returns the index of the first word from `from` on that
// starts with prefix, or -1.
func wordWithPrefix(words []string, prefix string, from int) int {
	for i := from; i < len(words); i++ {
		if strings.HasPrefix(words[i], prefix) {
			return i
		}
	}
	return -1
}

// currentIndex returns the index, rebuilding it when the data changed.
func (s *SearchService) currentIndex(ctx context.Context) (*searchIndex, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if s.index != nil && now.Sub(s.checkedAt) < searchStampTTL {
		return s.index, nil
	}
	// The rebuild is not cancelled with the request that triggered it:
	// other requests wait for it. A failed rebuild keeps the old index.
	bctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), searchBuildTimeout)
	defer cancel()
	stamp, err := s.repo.IndexStamp(bctx)
	if err != nil {
		if s.index != nil {
			return s.index, nil
		}
		return nil, err
	}
	s.checkedAt = now
	if s.index != nil && stamp == s.stamp {
		return s.index, nil
	}
	idx, err := s.build(bctx)
	if err != nil {
		if s.index != nil {
			return s.index, nil
		}
		return nil, err
	}
	s.index, s.stamp = idx, stamp
	return idx, nil
}

func (s *SearchService) build(ctx context.Context) (*searchIndex, error) {
	titles, err := s.repo.SearchTitles(ctx)
	if err != nil {
		return nil, err
	}
	names, err := s.repo.SearchNames(ctx)
	if err != nil {
		return nil, err
	}
	byRef := make(map[string]*searchDoc, len(titles))
	idx := &searchIndex{docs: make([]*searchDoc, 0, len(titles))}
	for _, t := range titles {
		r := SearchResult{Kind: t.Ref.Kind, TmdbID: t.Ref.ID, Title: t.Title, PosterPath: t.PosterPath, Sources: []string{}}
		if t.OriginalTitle != t.Title {
			r.OriginalTitle = t.OriginalTitle
		}
		r.Year = dateYear(t.ReleaseDate)
		if t.LastFlixPatrol != "" {
			r.Sources = append(r.Sources, "flixpatrol")
		}
		if t.LastNetflix != "" {
			r.Sources = append(r.Sources, "netflix")
		}
		r.LastCharted = max(t.LastFlixPatrol, t.LastNetflix)
		d := &searchDoc{result: r}
		byRef[t.Ref.String()] = d
		idx.docs = append(idx.docs, d)
	}
	seen := map[string]bool{}
	for _, n := range names {
		d := byRef[n.Ref.String()]
		folded := foldName(n.Name)
		if d == nil || folded == "" || seen[n.Ref.String()+"\x00"+folded] {
			continue
		}
		seen[n.Ref.String()+"\x00"+folded] = true
		d.names = append(d.names, searchName{norm: folded, tokens: strings.Fields(folded), raw: n.Name})
		if d.result.Title == "" && n.Source != "tmdb" {
			d.result.Title = n.Name // TMDB details not fetched yet
		}
	}
	return idx, nil
}
