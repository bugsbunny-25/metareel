package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
)

// A person's TMDB profile and full filmography, fetched live per request and
// kept in memory for a while: /people/{id} adds them to the titles metareel
// charted.

// ErrPersonUnavailable: TMDB failed for a person metareel has no record of.
var ErrPersonUnavailable = errors.New("tmdb person request failed")

// ErrCreditsUnavailable: TMDB failed for credits of a title metareel has no
// record of.
var ErrCreditsUnavailable = errors.New("tmdb credits request failed")

const (
	personCacheTTL     = 12 * time.Hour
	personMissTTL      = time.Hour // TMDB has no such person
	personCacheMax     = 1000
	personFetchTimeout = 15 * time.Second
)

type personSource interface {
	Enabled() bool
	Person(ctx context.Context, id int) (*client.TMDBPersonDetails, error)
}

// Details status of a PersonResponse.
const (
	PersonDetailsOK            = "ok"
	PersonDetailsNotConfigured = "not_configured" // no TMDB key
	PersonDetailsUnavailable   = "unavailable"    // TMDB failed; try later
	PersonDetailsNotFound      = "not_found"      // TMDB has no such person
)

// PersonDetails is a person's TMDB profile.
type PersonDetails struct {
	Biography    *string  `json:"biography"`
	Birthday     *string  `json:"birthday"` // YYYY-MM-DD
	Deathday     *string  `json:"deathday"`
	PlaceOfBirth *string  `json:"place_of_birth"`
	KnownFor     *string  `json:"known_for"` // department, e.g. Acting, Directing
	Homepage     *string  `json:"homepage"`
	IMDbID       *string  `json:"imdb_id"`
	AlsoKnownAs  []string `json:"also_known_as"`
}

// FilmographyTitle is one title of a person's TMDB filmography with every
// role they had on it.
type FilmographyTitle struct {
	Kind        string   `json:"kind"` // movie | tv
	TmdbID      string   `json:"tmdb_id"`
	Title       string   `json:"title"`
	Date        *string  `json:"date"` // release or first air date; null if not announced
	Year        *int     `json:"year"`
	PosterPath  *string  `json:"poster_path"`
	Roles       []string `json:"roles"`       // "as <character>" for cast, else the job
	Departments []string `json:"departments"` // Acting, Directing, Writing, ...
	Episodes    *int     `json:"episodes"`    // TV: most episodes of any of the roles
	Popularity  float64  `json:"popularity"`
	VoteAverage float64  `json:"vote_average"`
	VoteCount   int      `json:"vote_count"`
}

// tmdbCache keeps TMDB lookups in memory: a hit for personCacheTTL, a miss
// (TMDB has no such thing) for personMissTTL, at most max entries.
type tmdbCache[K comparable, V any] struct {
	mu  sync.Mutex
	m   map[K]tmdbCacheEntry[V]
	max int
}

type tmdbCacheEntry[V any] struct {
	at   time.Time
	v    V
	miss bool
}

func newTMDBCache[K comparable, V any](max int) *tmdbCache[K, V] {
	return &tmdbCache[K, V]{m: map[K]tmdbCacheEntry[V]{}, max: max}
}

// get returns a fresh entry: its value, whether it is a miss, and ok.
func (c *tmdbCache[K, V]) get(k K, now time.Time) (V, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[k]
	if !ok || (!e.miss && now.Sub(e.at) >= personCacheTTL) || (e.miss && now.Sub(e.at) >= personMissTTL) {
		var zero V
		return zero, false, false
	}
	return e.v, e.miss, true
}

// put caches a lookup; when full it drops expired entries, then the oldest.
func (c *tmdbCache[K, V]) put(k K, v V, miss bool, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= c.max {
		var oldest K
		var oldestAt time.Time
		for key, e := range c.m {
			if now.Sub(e.at) >= personCacheTTL {
				delete(c.m, key)
				continue
			}
			if oldestAt.IsZero() || e.at.Before(oldestAt) {
				oldest, oldestAt = key, e.at
			}
		}
		if len(c.m) >= c.max {
			delete(c.m, oldest)
		}
	}
	c.m[k] = tmdbCacheEntry[V]{at: now, v: v, miss: miss}
}

func (c *tmdbCache[K, V]) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.m)
}

// tmdbSource is the TMDB client: people and title credits.
type tmdbSource interface {
	personSource
	Credits(ctx context.Context, kind, tmdbID string) (*client.TMDBCredits, error)
	SearchPeople(ctx context.Context, query string) ([]client.TMDBPersonResult, error)
}

// PersonSearchResult is one person found by name on TMDB.
type PersonSearchResult struct {
	PersonID    int64    `json:"person_id"`
	Name        string   `json:"name"`
	ProfilePath *string  `json:"profile_path"`
	KnownFor    *string  `json:"known_for"`    // department
	KnownTitles []string `json:"known_titles"` // up to 3
}

// SearchPeople finds people on TMDB by name, most popular first.
func (s *CreditsService) SearchPeople(ctx context.Context, query string, limit int) ([]PersonSearchResult, error) {
	query = strings.TrimSpace(query)
	if n := len([]rune(query)); n < 2 || n > 100 {
		return nil, &ValidationError{Message: "q must be 2-100 characters"}
	}
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	if s.tmdb == nil || !s.tmdb.Enabled() {
		return nil, fmt.Errorf("%w: TMDB_API_KEY is not set", ErrPersonUnavailable)
	}
	hits, err := s.tmdb.SearchPeople(ctx, query)
	if err != nil {
		return nil, personUnavailable(err)
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	// Sized by TMDB's page (at most 20), never by the caller's limit.
	out := make([]PersonSearchResult, 0, len(hits))
	for _, h := range hits {
		out = append(out, PersonSearchResult{PersonID: int64(h.ID), Name: h.Name, ProfilePath: strPtrOrNil(h.ProfilePath),
			KnownFor: strPtrOrNil(h.KnownFor), KnownTitles: h.KnownTitles})
	}
	return out, nil
}

// WithTMDB adds TMDB profiles and filmographies to Person, and live credits
// to TitleCredits for titles without stored ones.
func (s *CreditsService) WithTMDB(src tmdbSource) *CreditsService {
	s.tmdb = src
	s.people = newTMDBCache[int64, *client.TMDBPersonDetails](personCacheMax)
	s.liveCredits = newTMDBCache[string, *client.TMDBCredits](personCacheMax)
	return s
}

// tmdbPerson returns a person from the cache or TMDB, and a details status.
func (s *CreditsService) tmdbPerson(ctx context.Context, id int64) (*client.TMDBPersonDetails, string, error) {
	if s.tmdb == nil || !s.tmdb.Enabled() {
		return nil, PersonDetailsNotConfigured, nil
	}
	if p, _, ok := s.people.get(id, s.clock()); ok {
		return p, detailsStatus(p), nil
	}
	v, err, _ := s.fetches.Do("person:"+strconv.FormatInt(id, 10), func() (any, error) {
		// Detached from the caller so one cancelled request doesn't fail the others waiting on it.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), personFetchTimeout)
		defer cancel()
		p, err := s.tmdb.Person(fctx, int(id))
		if err != nil {
			return nil, err
		}
		s.people.put(id, p, p == nil, s.clock())
		return p, nil
	})
	if err != nil {
		return nil, PersonDetailsUnavailable, err
	}
	p, _ := v.(*client.TMDBPersonDetails)
	return p, detailsStatus(p), nil
}

// tmdbCredits returns a title's credits from the cache or TMDB; nil, nil
// when TMDB is not configured or has no such title.
func (s *CreditsService) tmdbCredits(ctx context.Context, ref TmdbRef) (*client.TMDBCredits, error) {
	if s.tmdb == nil || !s.tmdb.Enabled() {
		return nil, nil
	}
	key := ref.Kind + ":" + ref.ID
	if c, _, ok := s.liveCredits.get(key, s.clock()); ok {
		return c, nil
	}
	v, err, _ := s.fetches.Do("credits:"+key, func() (any, error) {
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), personFetchTimeout)
		defer cancel()
		c, err := s.tmdb.Credits(fctx, ref.Kind, ref.ID)
		if err != nil {
			return nil, err
		}
		s.liveCredits.put(key, c, c == nil, s.clock())
		return c, nil
	})
	if err != nil {
		return nil, err
	}
	c, _ := v.(*client.TMDBCredits)
	return c, nil
}

func detailsStatus(p *client.TMDBPersonDetails) string {
	if p == nil {
		return PersonDetailsNotFound
	}
	return PersonDetailsOK
}

func (s *CreditsService) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// addTMDB fills a person's profile and filmography from TMDB.
func addTMDB(out *PersonResponse, p *client.TMDBPersonDetails) {
	if out.Name == "" {
		out.Name = p.Name
	}
	if p.ProfilePath != "" {
		out.ProfilePath = &p.ProfilePath
	}
	if out.KnownFor == nil {
		out.KnownFor = strPtrOrNil(p.KnownFor)
	}
	aka := p.AlsoKnownAs
	if aka == nil {
		aka = []string{}
	}
	out.Details = &PersonDetails{
		Biography: strPtrOrNil(p.Biography), Birthday: strPtrOrNil(p.Birthday), Deathday: strPtrOrNil(p.Deathday),
		PlaceOfBirth: strPtrOrNil(p.PlaceOfBirth), KnownFor: strPtrOrNil(p.KnownFor), Homepage: strPtrOrNil(p.Homepage),
		IMDbID: strPtrOrNil(p.IMDbID), AlsoKnownAs: aka,
	}
	out.Filmography = filmography(p.Credits)
}

// filmography merges a person's credits into one entry per title, newest
// first; titles without a date (not announced yet) lead.
func filmography(credits []client.TMDBPersonCredit) []FilmographyTitle {
	byKey := map[string]*FilmographyTitle{}
	var order []string
	for _, c := range credits {
		if c.ID <= 0 || c.Title == "" {
			continue
		}
		key := c.Kind + ":" + strconv.Itoa(c.ID)
		t := byKey[key]
		if t == nil {
			t = &FilmographyTitle{Kind: c.Kind, TmdbID: strconv.Itoa(c.ID), Title: c.Title, Date: strPtrOrNil(c.Date), Year: yearPtr(c.Date),
				PosterPath: strPtrOrNil(c.PosterPath), Roles: []string{}, Departments: []string{},
				Popularity: c.Popularity, VoteAverage: c.VoteAverage, VoteCount: c.VoteCount}
			byKey[key] = t
			order = append(order, key)
		}
		role := c.Job
		if c.Department == "Acting" {
			role = "Actor"
			if c.Character != "" {
				role = "as " + c.Character
			}
		}
		if role != "" && !containsFold(t.Roles, role) {
			t.Roles = append(t.Roles, role)
		}
		if c.Department != "" && !containsFold(t.Departments, c.Department) {
			t.Departments = append(t.Departments, c.Department)
		}
		if c.Kind == "tv" && c.EpisodeCount > 0 && (t.Episodes == nil || c.EpisodeCount > *t.Episodes) {
			n := c.EpisodeCount
			t.Episodes = &n
		}
	}
	out := make([]FilmographyTitle, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Date, out[j].Date
		if (a == nil) != (b == nil) {
			return a == nil
		}
		if a == nil {
			return out[i].Popularity > out[j].Popularity
		}
		return *a > *b
	})
	return out
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

func personUnavailable(err error) error {
	return fmt.Errorf("%w: %w", ErrPersonUnavailable, err)
}
