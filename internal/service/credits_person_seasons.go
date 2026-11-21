package service

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
)

// A person's TV credits per season (GET /people/{id}?seasons=true): each
// show's season list and per-season aggregate credits, one TMDB request per
// 20 seasons, cached per show and shared by everyone on it.

// showSeasonsSource is TMDB's per-season credits of a show.
type showSeasonsSource interface {
	ShowSeasonCredits(ctx context.Context, tvID int, seasons []int) (*client.TMDBShowSeasons, error)
}

const (
	maxSeasonShows       = 40 // shows looked up per person request (most episodes first)
	maxShowSeasons       = 60 // seasons per show (3 requests)
	seasonFetchParallel  = 6
	seasonsRequestBudget = 25 * time.Second
	showSeasonsCacheMax  = 300
	seasonsPerRequest    = 20
)

// Status of a person's season credits.
const (
	SeasonsComplete    = "complete"
	SeasonsPartial     = "partial"     // some shows were skipped (cap) or failed
	SeasonsUnavailable = "unavailable" // TMDB failed for every show
)

// FilmographySeason is one season of a show a person is credited in.
type FilmographySeason struct {
	TmdbID       string  `json:"tmdb_id"` // the show
	Title        string  `json:"title"`
	SeasonNumber int     `json:"season_number"` // 0 = specials
	SeasonName   *string `json:"season_name"`
	AirDate      *string `json:"air_date"` // the season's first air date; null if not announced
	Year         *int    `json:"year"`
	// EpisodeCount: episodes of the season the person is credited in;
	// SeasonEpisodes: episodes the season has.
	EpisodeCount   int      `json:"episode_count"`
	SeasonEpisodes *int     `json:"season_episode_count"`
	PosterPath     *string  `json:"poster_path"` // the season's poster, else the show's
	Roles          []string `json:"roles"`       // "as <character>" for cast, else the job
	Departments    []string `json:"departments"`
}

type seasonShow struct {
	id     int
	title  string
	weight int // most episodes in any credit
	pop    float64
}

// personSeasons returns a person's TV credits per season and a status.
func (s *CreditsService) personSeasons(ctx context.Context, personID int64, credits []client.TMDBPersonCredit) ([]FilmographySeason, string) {
	src, ok := s.tmdb.(showSeasonsSource)
	if !ok || s.tmdb == nil || !s.tmdb.Enabled() {
		return []FilmographySeason{}, SeasonsUnavailable
	}
	byID := map[int]*seasonShow{}
	for _, c := range credits {
		if c.Kind != "tv" || c.ID <= 0 {
			continue
		}
		sh := byID[c.ID]
		if sh == nil {
			sh = &seasonShow{id: c.ID, title: c.Title, pop: c.Popularity}
			byID[c.ID] = sh
		}
		sh.weight = max(sh.weight, c.EpisodeCount)
	}
	shows := make([]*seasonShow, 0, len(byID))
	for _, sh := range byID {
		shows = append(shows, sh)
	}
	sort.Slice(shows, func(i, j int) bool {
		if shows[i].weight != shows[j].weight {
			return shows[i].weight > shows[j].weight
		}
		if shows[i].pop != shows[j].pop {
			return shows[i].pop > shows[j].pop
		}
		return shows[i].id < shows[j].id
	})
	status := SeasonsComplete
	if len(shows) > maxSeasonShows {
		shows, status = shows[:maxSeasonShows], SeasonsPartial
	}

	ctx, cancel := context.WithTimeout(ctx, seasonsRequestBudget)
	defer cancel()
	results := make([]*client.TMDBShowSeasons, len(shows))
	failed := 0
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		sem = make(chan struct{}, seasonFetchParallel)
	)
	for i, sh := range shows {
		wg.Add(1)
		go func(i int, id int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r, err := s.showSeasons(ctx, src, id)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
				return
			}
			results[i] = r
		}(i, sh.id)
	}
	wg.Wait()
	if failed > 0 {
		status = SeasonsPartial
		if failed == len(shows) {
			status = SeasonsUnavailable
		}
	}

	out := []FilmographySeason{}
	for i, show := range results {
		if show == nil {
			continue
		}
		out = append(out, seasonsOf(show, shows[i].title, int(personID))...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].AirDate, out[j].AirDate
		if (a == nil) != (b == nil) {
			return a == nil
		}
		if a != nil && *a != *b {
			return *a > *b
		}
		if out[i].TmdbID != out[j].TmdbID {
			return out[i].TmdbID < out[j].TmdbID
		}
		return out[i].SeasonNumber > out[j].SeasonNumber
	})
	return out, status
}

// seasonsOf lists the seasons of show the person is credited in.
func seasonsOf(show *client.TMDBShowSeasons, title string, personID int) []FilmographySeason {
	if title == "" {
		title = show.Name
	}
	meta := map[int]client.TMDBShowSeason{}
	for _, se := range show.Seasons {
		meta[se.Number] = se
	}
	bySeason := map[int]*FilmographySeason{}
	var order []int
	for _, c := range show.People[personID] {
		fs := bySeason[c.Season]
		if fs == nil {
			m := meta[c.Season]
			fs = &FilmographySeason{TmdbID: strconv.Itoa(show.ID), Title: title, SeasonNumber: c.Season,
				SeasonName: strPtrOrNil(m.Name), AirDate: strPtrOrNil(m.AirDate), Year: yearPtr(m.AirDate),
				PosterPath: strPtrOrNil(m.PosterPath), Roles: []string{}, Departments: []string{}}
			if fs.PosterPath == nil {
				fs.PosterPath = strPtrOrNil(show.PosterPath)
			}
			if m.EpisodeCount > 0 {
				n := m.EpisodeCount
				fs.SeasonEpisodes = &n
			}
			bySeason[c.Season] = fs
			order = append(order, c.Season)
		}
		role := c.Job
		if c.Department == "Acting" {
			role = "Actor"
			if c.Character != "" {
				role = "as " + c.Character
			}
		}
		if role != "" && !containsFold(fs.Roles, role) {
			fs.Roles = append(fs.Roles, role)
		}
		if c.Department != "" && !containsFold(fs.Departments, c.Department) {
			fs.Departments = append(fs.Departments, c.Department)
		}
		fs.EpisodeCount = max(fs.EpisodeCount, c.EpisodeCount)
	}
	out := make([]FilmographySeason, 0, len(order))
	for _, n := range order {
		out = append(out, *bySeason[n])
	}
	return out
}

// showSeasons returns a show's seasons with every season's credits, from
// the cache or TMDB (seasons 0-19 first, then 20 at a time as the show has
// them, up to maxShowSeasons). nil, nil when TMDB has no such show.
func (s *CreditsService) showSeasons(ctx context.Context, src showSeasonsSource, id int) (*client.TMDBShowSeasons, error) {
	key := int64(id)
	if v, _, ok := s.shows.get(key, s.clock()); ok {
		return v, nil
	}
	v, err, _ := s.fetches.Do("show-seasons:"+strconv.Itoa(id), func() (any, error) {
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), personFetchTimeout)
		defer cancel()
		var show *client.TMDBShowSeasons
		for from := 0; from < maxShowSeasons; from += seasonsPerRequest {
			if show != nil {
				last := 0
				for _, se := range show.Seasons {
					last = max(last, se.Number)
				}
				if last < from {
					break
				}
			}
			nums := make([]int, 0, seasonsPerRequest)
			for n := from; n < from+seasonsPerRequest; n++ {
				nums = append(nums, n)
			}
			part, err := src.ShowSeasonCredits(fctx, id, nums)
			if err != nil {
				return nil, err
			}
			if part == nil {
				break
			}
			if show == nil {
				show = part
				continue
			}
			for pid, cs := range part.People {
				show.People[pid] = append(show.People[pid], cs...)
			}
			show.Fetched = append(show.Fetched, part.Fetched...)
		}
		s.shows.put(key, show, show == nil, s.clock())
		return show, nil
	})
	if err != nil {
		return nil, err
	}
	show, _ := v.(*client.TMDBShowSeasons)
	return show, nil
}
