package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
)

// A person's TMDB profile and full filmography, fetched live per request and
// kept in memory for a while: /people/{id} adds them to the titles metareel
// charted.

// ErrPersonUnavailable: TMDB failed for a person metareel has no record of.
var ErrPersonUnavailable = errors.New("tmdb person request failed")

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

type personCacheEntry struct {
	at time.Time
	p  *client.TMDBPersonDetails // nil: TMDB has no such person
}

// WithTMDB adds TMDB profiles and filmographies to Person.
func (s *CreditsService) WithTMDB(src personSource) *CreditsService {
	s.tmdb = src
	s.people = map[int64]personCacheEntry{}
	return s
}

// tmdbPerson returns a person from the cache or TMDB, and a details status.
func (s *CreditsService) tmdbPerson(ctx context.Context, id int64) (*client.TMDBPersonDetails, string, error) {
	if s.tmdb == nil || !s.tmdb.Enabled() {
		return nil, PersonDetailsNotConfigured, nil
	}
	now := s.clock()
	s.mu.Lock()
	e, ok := s.people[id]
	s.mu.Unlock()
	if ok && (e.p != nil && now.Sub(e.at) < personCacheTTL || e.p == nil && now.Sub(e.at) < personMissTTL) {
		return e.p, detailsStatus(e.p), nil
	}
	v, err, _ := s.fetches.Do(strconv.FormatInt(id, 10), func() (any, error) {
		// Detached from the caller so one cancelled request doesn't fail the others waiting on it.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), personFetchTimeout)
		defer cancel()
		p, err := s.tmdb.Person(fctx, int(id))
		if err != nil {
			return nil, err
		}
		s.remember(id, p, s.clock())
		return p, nil
	})
	if err != nil {
		return nil, PersonDetailsUnavailable, err
	}
	p, _ := v.(*client.TMDBPersonDetails)
	return p, detailsStatus(p), nil
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

// remember caches a lookup; when full it drops expired entries, then the oldest.
func (s *CreditsService) remember(id int64, p *client.TMDBPersonDetails, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.people) >= personCacheMax {
		var oldest int64
		var oldestAt time.Time
		for k, e := range s.people {
			if now.Sub(e.at) >= personCacheTTL {
				delete(s.people, k)
				continue
			}
			if oldestAt.IsZero() || e.at.Before(oldestAt) {
				oldest, oldestAt = k, e.at
			}
		}
		if len(s.people) >= personCacheMax {
			delete(s.people, oldest)
		}
	}
	s.people[id] = personCacheEntry{at: now, p: p}
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
