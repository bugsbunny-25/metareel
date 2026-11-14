package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// Credits: the cast and key crew of the titles metareel knows, from TMDB
// (tmdb.credits.refresh), for a title's credits, a person's titles with
// their chart record, and people ranked by the chart points of their
// titles.

var ErrPersonNotFound = errors.New("person not found")

const (
	creditsMaxCast       = 25
	creditsMaxPerJob     = 10 // TV crew: most episodes first
	creditsRefreshAfter  = 60 * 24 * time.Hour
	creditsMaxTitles     = 1000 // per run
	creditsGap           = 250 * time.Millisecond
	creditsMaxFailStreak = 20
)

// creditJobs are the crew jobs kept, by people-analytics role.
var creditJobs = map[string]string{
	"Director":   "director",
	"Screenplay": "writer", "Writer": "writer", "Story": "writer", "Novel": "writer", "Teleplay": "writer", "Characters": "writer",
	"Creator":                 "creator",
	"Original Music Composer": "composer",
	"Director of Photography": "cinematographer",
}

// PeopleRoles are the roles of /analytics/people.
var PeopleRoles = []string{"actor", "director", "writer", "creator", "composer", "cinematographer"}

type creditSource interface {
	Enabled() bool
	Credits(ctx context.Context, kind, tmdbID string) (*client.TMDBCredits, error)
}

// CreditsRefresher fetches TMDB credits of titles never fetched or not
// fetched for 60 days.
type CreditsRefresher struct {
	log    *slog.Logger
	source creditSource
	repo   *repository.CreditsRepository
	now    func() time.Time
	gap    time.Duration
}

func NewCreditsRefresher(log *slog.Logger, source creditSource, repo *repository.CreditsRepository) *CreditsRefresher {
	return &CreditsRefresher{log: log, source: source, repo: repo, now: time.Now, gap: creditsGap}
}

type CreditsReport struct {
	Titles   int    `json:"titles"`
	Due      int    `json:"due"`
	Fetched  int    `json:"fetched"`
	Missing  int    `json:"missing"` // unknown to TMDB
	Failed   int    `json:"failed"`
	Deferred int    `json:"deferred"` // due, left for the next run
	Cast     int    `json:"cast"`
	Crew     int    `json:"crew"`
	Skipped  string `json:"skipped,omitempty"`
}

func (r *CreditsRefresher) Run(ctx context.Context, force bool, runLog func(level, msg string)) (*CreditsReport, error) {
	logf := func(level, format string, args ...any) {
		if runLog != nil {
			runLog(level, fmt.Sprintf(format, args...))
		}
	}
	rep := &CreditsReport{}
	if r.source == nil || !r.source.Enabled() {
		rep.Skipped = "TMDB_API_KEY is not set"
		logf("warn", "TMDB credits: skipped, TMDB_API_KEY is not set")
		return rep, nil
	}
	targets, err := r.repo.CreditTargets(ctx)
	if err != nil {
		return nil, err
	}
	rep.Titles = len(targets)
	now := r.now().UTC()
	var due []repository.CreditTarget
	for _, t := range targets {
		if force || creditsDue(t.FetchedAt, now) {
			due = append(due, t)
		}
	}
	rep.Due = len(due)
	if len(due) > creditsMaxTitles {
		rep.Deferred = len(due) - creditsMaxTitles
		due = due[:creditsMaxTitles] // never fetched, then oldest (query order)
	}
	logf("info", "TMDB credits: %d titles, %d due", rep.Titles, rep.Due)
	streak := 0
	for i, t := range due {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		if i > 0 && r.gap > 0 {
			if err := sleepCtx(ctx, r.gap); err != nil {
				return rep, err
			}
		}
		credits, err := r.source.Credits(ctx, t.Ref.Kind, t.Ref.ID)
		if err != nil {
			rep.Failed++
			streak++
			if rep.Failed <= 5 {
				logf("warn", "%s: %v", t.Ref, err)
			}
			r.log.Warn("tmdb credits fetch failed", slog.String("ref", t.Ref.String()), slog.Any("err", err))
			if streak >= creditsMaxFailStreak {
				return rep, fmt.Errorf("tmdb credits: %d requests in a row failed, last: %w", streak, err)
			}
			continue
		}
		streak = 0
		var cast, crew []client.TMDBCredit
		if credits == nil {
			rep.Missing++ // marked fetched anyway: it waits for its next interval
		} else {
			cast, crew = keepCredits(credits)
		}
		if err := r.repo.SaveCredits(ctx, t.Ref, cast, crew); err != nil {
			return rep, err
		}
		rep.Fetched++
		rep.Cast += len(cast)
		rep.Crew += len(crew)
	}
	logf("info", "TMDB credits: %d titles fetched (%d cast, %d crew credits), %d unknown to TMDB, %d failed, %d left for the next run",
		rep.Fetched, rep.Cast, rep.Crew, rep.Missing, rep.Failed, rep.Deferred)
	if rep.Failed > 0 && rep.Fetched == 0 {
		return rep, fmt.Errorf("every TMDB credits request failed (%d)", rep.Failed)
	}
	return rep, nil
}

func creditsDue(fetchedAt string, now time.Time) bool {
	if fetchedAt == "" {
		return true
	}
	t, err := time.Parse(sqliteTimeLayout, fetchedAt)
	return err != nil || now.Sub(t) >= creditsRefreshAfter
}

// keepCredits keeps the top-billed cast and the kept crew jobs (on TV the
// people with the most episodes per job), one credit per person and job.
func keepCredits(c *client.TMDBCredits) (cast, crew []client.TMDBCredit) {
	sorted := append([]client.TMDBCredit(nil), c.Cast...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Order < sorted[j].Order })
	seen := map[int]bool{}
	for _, x := range sorted {
		if x.PersonID <= 0 || x.Name == "" || seen[x.PersonID] {
			continue
		}
		seen[x.PersonID] = true
		cast = append(cast, x)
		if len(cast) == creditsMaxCast {
			break
		}
	}
	byJob := map[string][]client.TMDBCredit{}
	var jobs []string
	type pj struct {
		person int
		job    string
	}
	seenJob := map[pj]bool{}
	for _, x := range c.Crew {
		if _, ok := creditJobs[x.Job]; !ok || x.PersonID <= 0 || x.Name == "" || seenJob[pj{x.PersonID, x.Job}] {
			continue
		}
		seenJob[pj{x.PersonID, x.Job}] = true
		if byJob[x.Job] == nil {
			jobs = append(jobs, x.Job)
		}
		byJob[x.Job] = append(byJob[x.Job], x)
	}
	for _, j := range jobs {
		list := byJob[j]
		sort.SliceStable(list, func(a, b int) bool { return list[a].Episodes > list[b].Episodes })
		crew = append(crew, list[:min(len(list), creditsMaxPerJob)]...)
	}
	return cast, crew
}

// --- reads ------------------------------------------------------------------------

// CreditsService serves title credits and people.
type CreditsService struct {
	repo     *repository.CreditsRepository
	metadata *repository.MetadataRepository

	// TMDB profiles and filmographies (WithTMDB), cached by person ID.
	tmdb    personSource
	mu      sync.Mutex
	people  map[int64]personCacheEntry
	fetches singleflight.Group
	now     func() time.Time
}

func NewCreditsService(repo *repository.CreditsRepository, metadata *repository.MetadataRepository) *CreditsService {
	return &CreditsService{repo: repo, metadata: metadata}
}

type CastCredit struct {
	PersonID    int64   `json:"person_id"`
	Name        string  `json:"name"`
	ProfilePath *string `json:"profile_path"`
	Character   string  `json:"character"`
	Order       int64   `json:"order"`
	Episodes    *int64  `json:"episodes"` // TV
}

type CrewCredit struct {
	PersonID    int64   `json:"person_id"`
	Name        string  `json:"name"`
	ProfilePath *string `json:"profile_path"`
	Job         string  `json:"job"`
	Department  string  `json:"department"`
	Episodes    *int64  `json:"episodes"`
}

type TitleCreditsResponse struct {
	Kind   string       `json:"kind"`
	TmdbID string       `json:"tmdb_id"`
	Title  *string      `json:"title"`
	Cast   []CastCredit `json:"cast"`
	Crew   []CrewCredit `json:"crew"`
}

// TitleCredits returns a title's stored cast and key crew (empty until the
// credits job has fetched them).
func (s *CreditsService) TitleCredits(ctx context.Context, ref TmdbRef) (*TitleCreditsResponse, error) {
	meta, err := s.metadata.Get(ctx, ref)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, ErrTitleNotFound
	}
	credits, err := s.repo.TitleCredits(ctx, ref)
	if err != nil {
		return nil, err
	}
	out := &TitleCreditsResponse{Kind: ref.Kind, TmdbID: ref.ID, Title: meta.Title, Cast: []CastCredit{}, Crew: []CrewCredit{}}
	for _, c := range credits {
		if c.Type == repository.CreditCast {
			out.Cast = append(out.Cast, CastCredit{PersonID: c.PersonID, Name: c.Name, ProfilePath: strPtrOrNil(c.ProfilePath),
				Character: c.Character, Order: c.Order, Episodes: c.Episodes})
		} else {
			out.Crew = append(out.Crew, CrewCredit{PersonID: c.PersonID, Name: c.Name, ProfilePath: strPtrOrNil(c.ProfilePath),
				Job: c.Job, Department: c.Department, Episodes: c.Episodes})
		}
	}
	return out, nil
}

// PersonTitle is one of a person's titles with their roles and its chart
// record (all stored FlixPatrol charts).
type PersonTitle struct {
	Kind       string   `json:"kind"`
	TmdbID     string   `json:"tmdb_id"`
	Title      string   `json:"title"`
	Year       *int     `json:"year"`
	PosterPath *string  `json:"poster_path"`
	Roles      []string `json:"roles"` // "as <character>" for cast, else the job
	// Chart record: Top 10 points, days, best rank, countries, dates.
	Points       int64   `json:"points"`
	Days         int64   `json:"days"`
	BestRank     *int64  `json:"best_rank"`
	Countries    int64   `json:"countries"`
	FirstCharted *string `json:"first_charted"`
	LastCharted  *string `json:"last_charted"`
}

type PersonResponse struct {
	PersonID    int64         `json:"person_id"`
	Name        string        `json:"name"`
	ProfilePath *string       `json:"profile_path"`
	KnownFor    *string       `json:"known_for"`
	Points      int64         `json:"points"`   // over every title
	Charting    int           `json:"charting"` // titles that charted
	Titles      []PersonTitle `json:"titles"`   // most charted first, then newest
	// TMDB profile and every movie and TV credit, live from TMDB (cached for
	// 12 hours). DetailsStatus says why they are missing: not_configured,
	// unavailable (TMDB failed) or not_found.
	DetailsStatus string             `json:"details_status"`
	Details       *PersonDetails     `json:"details"`
	Filmography   []FilmographyTitle `json:"filmography"`
}

// Person returns a person, their titles metareel knows with their chart
// record, and their TMDB profile and filmography. A person metareel has no
// credits for is still found when TMDB knows them.
func (s *CreditsService) Person(ctx context.Context, id int64) (*PersonResponse, error) {
	if id <= 0 {
		return nil, &ValidationError{Message: "invalid person id"}
	}
	p, err := s.repo.GetPerson(ctx, id)
	if err != nil {
		return nil, err
	}
	tp, status, tmdbErr := s.tmdbPerson(ctx, id)
	if p == nil {
		switch {
		case tp != nil:
			out := &PersonResponse{PersonID: id, Titles: []PersonTitle{}, DetailsStatus: status}
			addTMDB(out, tp)
			return out, nil
		case tmdbErr != nil:
			return nil, personUnavailable(tmdbErr)
		}
		return nil, ErrPersonNotFound
	}
	out, err := s.chartedPerson(ctx, p)
	if err != nil {
		return nil, err
	}
	out.DetailsStatus, out.Filmography = status, []FilmographyTitle{}
	if tp != nil {
		addTMDB(out, tp)
	}
	return out, nil
}

// chartedPerson returns a person with their titles metareel knows.
func (s *CreditsService) chartedPerson(ctx context.Context, p *repository.Person) (*PersonResponse, error) {
	id := p.ID
	credits, err := s.repo.PersonCredits(ctx, id)
	if err != nil {
		return nil, err
	}
	stats, err := s.repo.PersonChartStats(ctx, id)
	if err != nil {
		return nil, err
	}
	out := &PersonResponse{PersonID: p.ID, Name: p.Name, ProfilePath: strPtrOrNil(p.ProfilePath), KnownFor: strPtrOrNil(p.KnownFor), Titles: []PersonTitle{}}
	byRef := map[string]*PersonTitle{}
	var order []string
	for _, c := range credits {
		key := c.Ref.String()
		t := byRef[key]
		if t == nil {
			t = &PersonTitle{Kind: c.Ref.Kind, TmdbID: c.Ref.ID, Title: c.Title, Year: yearPtr(c.ReleaseDate), PosterPath: strPtrOrNil(c.PosterPath), Roles: []string{}}
			if st, ok := stats[key]; ok {
				best := st.BestRank
				t.Points, t.Days, t.BestRank, t.Countries = st.Points, st.Days, &best, st.Countries
				t.FirstCharted, t.LastCharted = strPtrOrNil(st.FirstCharted), strPtrOrNil(st.LastCharted)
			}
			byRef[key] = t
			order = append(order, key)
		}
		role := c.Job
		if c.Type == repository.CreditCast {
			role = "Actor"
			if c.Character != "" {
				role = "as " + c.Character
			}
		}
		t.Roles = append(t.Roles, role)
	}
	for _, key := range order {
		t := byRef[key]
		out.Points += t.Points
		if t.Days > 0 {
			out.Charting++
		}
		out.Titles = append(out.Titles, *t)
	}
	sort.SliceStable(out.Titles, func(i, j int) bool { return out.Titles[i].Points > out.Titles[j].Points })
	return out, nil
}

// --- people analytics -------------------------------------------------------------

// WithCredits enables people analytics.
func (s *AnalyticsService) WithCredits(repo *repository.CreditsRepository) *AnalyticsService {
	s.credits = repo
	return s
}

type PersonTopTitle struct {
	Kind   string `json:"kind"`
	TmdbID string `json:"tmdb_id"`
	Title  string `json:"title"`
	Points int64  `json:"points"`
}

type PersonPoints struct {
	PersonID    int64            `json:"person_id"`
	Name        string           `json:"name"`
	ProfilePath *string          `json:"profile_path"`
	Points      int64            `json:"points"`    // Top 10 points of their titles in the range
	Titles      int              `json:"titles"`    // their titles that charted in the range
	BestRank    int64            `json:"best_rank"` // best rank of any of them
	TopTitles   []PersonTopTitle `json:"top_titles"`
}

type PeopleResponse struct {
	AnalyticsEcho
	Role   string         `json:"role"`
	Total  int            `json:"total"`
	People []PersonPoints `json:"people"`
}

const peopleActorTopBilling = 10 // actors count when billed in the top 10

// People ranks people in a role by the Top 10 points their titles earned in
// the range (each title's points go in full to each person credited).
func (s *AnalyticsService) People(ctx context.Context, q AnalyticsQuery) (*PeopleResponse, error) {
	f, err := s.filter(q, 90)
	if err != nil {
		return nil, err
	}
	role := q.Role
	if role == "" {
		role = "actor"
	}
	if !containsStr(PeopleRoles, role) {
		return nil, &ValidationError{Message: "invalid role, expected one of " + strings.Join(PeopleRoles, ", ")}
	}
	limit, err := intRange("limit", q.Limit, 50, 1, 200)
	if err != nil {
		return nil, err
	}
	if s.credits == nil {
		return nil, errors.New("people analytics not configured")
	}
	rows, err := s.repo.ChartEntries(ctx, repository.ChartEntryFilter{From: f.from, To: f.to, Country: f.country, Provider: f.provider, Category: f.category})
	if err != nil {
		return nil, err
	}
	type titleAgg struct {
		points, best int64
		name         string
	}
	titles := map[string]*titleAgg{}
	for _, r := range rows {
		if r.TmdbID == nil || *r.TmdbID == "" || (f.kind != "" && r.Kind != f.kind) {
			continue
		}
		key := tmdbKindOf(flixpatrol.TitleKind(r.Kind)) + ":" + *r.TmdbID
		t := titles[key]
		if t == nil {
			t = &titleAgg{best: r.Rank, name: r.Name}
			titles[key] = t
		}
		t.points += Points(r.Rank)
		t.best = min(t.best, r.Rank)
	}
	creditType := repository.CreditCrew
	if role == "actor" {
		creditType = repository.CreditCast
	}
	credits, err := s.credits.CreditsOfType(ctx, creditType)
	if err != nil {
		return nil, err
	}
	type personAgg struct {
		PersonPoints
		seen map[string]bool
	}
	people := map[int64]*personAgg{}
	for _, c := range credits {
		if role == "actor" {
			if c.Order >= peopleActorTopBilling {
				continue
			}
		} else if creditJobs[c.Job] != role {
			continue
		}
		key := c.Ref.String()
		t := titles[key]
		if t == nil {
			continue
		}
		p := people[c.PersonID]
		if p == nil {
			p = &personAgg{PersonPoints: PersonPoints{PersonID: c.PersonID, Name: c.Name, ProfilePath: strPtrOrNil(c.ProfilePath), BestRank: t.best,
				TopTitles: []PersonTopTitle{}}, seen: map[string]bool{}}
			people[c.PersonID] = p
		}
		if p.seen[key] {
			continue // e.g. writer credited as Screenplay and Story
		}
		p.seen[key] = true
		p.Points += t.points
		p.Titles++
		p.BestRank = min(p.BestRank, t.best)
		p.TopTitles = append(p.TopTitles, PersonTopTitle{Kind: c.Ref.Kind, TmdbID: c.Ref.ID, Title: t.name, Points: t.points})
	}
	out := &PeopleResponse{AnalyticsEcho: f.echo(), Role: role, People: []PersonPoints{}}
	for _, p := range people {
		sort.Slice(p.TopTitles, func(i, j int) bool { return p.TopTitles[i].Points > p.TopTitles[j].Points })
		p.TopTitles = p.TopTitles[:min(len(p.TopTitles), 3)]
		out.People = append(out.People, p.PersonPoints)
	}
	sort.Slice(out.People, func(i, j int) bool {
		a, b := out.People[i], out.People[j]
		if a.Points != b.Points {
			return a.Points > b.Points
		}
		return a.PersonID < b.PersonID
	})
	out.Total = len(out.People)
	out.People = out.People[:min(len(out.People), limit)]
	return out, nil
}
