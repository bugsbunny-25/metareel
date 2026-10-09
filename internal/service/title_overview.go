package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// TitleOverviewService answers per-title questions across sources: metadata,
// chart performance, ratings, where to watch and Netflix's official numbers.
type TitleOverviewService struct {
	analytics *repository.AnalyticsRepository
	flix      *repository.FlixPatrolRepository
	metadata  *repository.MetadataRepository
	ratings   *RatingsService
	now       func() time.Time
}

func NewTitleOverviewService(analytics *repository.AnalyticsRepository, flix *repository.FlixPatrolRepository, metadata *repository.MetadataRepository, ratings *RatingsService) *TitleOverviewService {
	return &TitleOverviewService{analytics: analytics, flix: flix, metadata: metadata, ratings: ratings, now: time.Now}
}

// --- stats ----------------------------------------------------------------------

// TitleStats is a title's chart performance across every chart.
type TitleStats struct {
	TotalPoints   int64          `json:"total_points"`
	DaysOnCharts  int64          `json:"days_on_charts"` // distinct dates on any chart
	Appearances   int64          `json:"appearances"`    // chart × date entries
	PeakRank      *int64         `json:"peak_rank"`
	DaysAtNumber1 int64          `json:"days_at_number_1"` // distinct dates at #1 on some chart
	Debut         *ChartMoment   `json:"debut"`
	LastSeen      *ChartMoment   `json:"last_seen"`
	LongestStreak int64          `json:"longest_streak_days"` // consecutive dates on one chart
	Countries     []string       `json:"countries"`
	Providers     []string       `json:"providers"`
	Spread        []CountryDebut `json:"spread"` // first date per country, in order
	Charts        []ChartSummary `json:"charts"`
}

// ChartMoment is a title's rank on a chart on a date.
type ChartMoment struct {
	Date     string `json:"date"`
	Country  string `json:"country"`
	Provider string `json:"provider"`
	Category string `json:"category"`
	Rank     int64  `json:"rank"`
}

// CountryDebut is when a title first charted in a country.
type CountryDebut struct {
	Country   string `json:"country"`
	FirstDate string `json:"first_date"`
	DaysAfter int    `json:"days_after_debut"`
}

// Stats computes chart performance from date-ordered snapshots.
func Stats(rankings []RankingSnapshot) TitleStats {
	st := TitleStats{Countries: []string{}, Providers: []string{}, Spread: []CountryDebut{}, Charts: summarizeCharts(rankings)}
	if len(rankings) == 0 {
		return st
	}
	dates, number1 := map[string]bool{}, map[string]bool{}
	countries, providers := map[string]bool{}, map[string]bool{}
	firstByCountry := map[string]string{}
	type chartKey struct{ country, provider, category string }
	chartDates := map[chartKey][]string{}
	for _, r := range rankings {
		st.TotalPoints += Points(r.Rank)
		st.Appearances++
		dates[r.Date] = true
		if r.Rank == 1 {
			number1[r.Date] = true
		}
		if st.PeakRank == nil || r.Rank < *st.PeakRank {
			rank := r.Rank
			st.PeakRank = &rank
		}
		m := ChartMoment{Date: r.Date, Country: r.Country, Provider: r.Provider, Category: r.Category, Rank: r.Rank}
		if st.Debut == nil || r.Date < st.Debut.Date || (r.Date == st.Debut.Date && r.Rank < st.Debut.Rank) {
			d := m
			st.Debut = &d
		}
		if st.LastSeen == nil || r.Date > st.LastSeen.Date || (r.Date == st.LastSeen.Date && r.Rank < st.LastSeen.Rank) {
			l := m
			st.LastSeen = &l
		}
		countries[r.Country] = true
		providers[r.Provider] = true
		if f, ok := firstByCountry[r.Country]; !ok || r.Date < f {
			firstByCountry[r.Country] = r.Date
		}
		k := chartKey{r.Country, r.Provider, r.Category}
		chartDates[k] = append(chartDates[k], r.Date)
	}
	st.DaysOnCharts, st.DaysAtNumber1 = int64(len(dates)), int64(len(number1))
	st.Countries, st.Providers = sortedKeys(countries), sortedKeys(providers)
	debut, _ := time.Parse(repository.DateLayout, st.Debut.Date)
	for c, d := range firstByCountry {
		t, _ := time.Parse(repository.DateLayout, d)
		st.Spread = append(st.Spread, CountryDebut{Country: c, FirstDate: d, DaysAfter: int(t.Sub(debut).Hours() / 24)})
	}
	sort.Slice(st.Spread, func(i, j int) bool {
		if st.Spread[i].FirstDate != st.Spread[j].FirstDate {
			return st.Spread[i].FirstDate < st.Spread[j].FirstDate
		}
		return st.Spread[i].Country < st.Spread[j].Country
	})
	for _, ds := range chartDates {
		sort.Strings(ds)
		run := int64(1)
		for i := 1; i < len(ds); i++ {
			a, _ := time.Parse(repository.DateLayout, ds[i-1])
			b, _ := time.Parse(repository.DateLayout, ds[i])
			switch b.Sub(a) {
			case 24 * time.Hour:
				run++
			case 0:
			default:
				run = 1
			}
			if run > st.LongestStreak {
				st.LongestStreak = run
			}
		}
		if st.LongestStreak == 0 {
			st.LongestStreak = 1
		}
	}
	return st
}

// rankingsForRef loads every chart appearance of a TMDB title.
func (s *TitleOverviewService) rankingsForRef(ctx context.Context, ref TmdbRef, q RankingHistoryQuery) (*TitleRankingsResponse, error) {
	top := NewTop10ReadService(s.flix)
	return top.GetTitleRankings(ctx, ref, q)
}

// TitleStatsResponse is GET /titles/tmdb/{kind}/{id}/stats.
type TitleStatsResponse struct {
	Kind   string        `json:"kind"`
	TmdbID string        `json:"tmdb_id"`
	Titles []RankedTitle `json:"titles"`
	TitleStats
}

func (s *TitleOverviewService) Stats(ctx context.Context, ref TmdbRef, q RankingHistoryQuery) (*TitleStatsResponse, error) {
	r, err := s.rankingsForRef(ctx, ref, q)
	if err != nil {
		return nil, err
	}
	return &TitleStatsResponse{Kind: ref.Kind, TmdbID: ref.ID, Titles: r.Titles, TitleStats: Stats(r.Rankings)}, nil
}

// --- availability -------------------------------------------------------------

// AvailabilityResponse is where a title can be watched.
type AvailabilityResponse struct {
	Kind        string                     `json:"kind"`
	TmdbID      string                     `json:"tmdb_id"`
	Country     *string                    `json:"country"`
	FetchedAt   *time.Time                 `json:"fetched_at"`
	Attribution string                     `json:"attribution"`
	Countries   map[string]*CountryOffers  `json:"countries"`
	Providers   []repository.WatchProvider `json:"-"`
}

// CountryOffers groups a country's providers by monetization.
type CountryOffers struct {
	Link     *string                    `json:"link"`
	Flatrate []repository.WatchProvider `json:"flatrate"`
	Free     []repository.WatchProvider `json:"free"`
	Ads      []repository.WatchProvider `json:"ads"`
	Rent     []repository.WatchProvider `json:"rent"`
	Buy      []repository.WatchProvider `json:"buy"`
}

// ErrNoMetadata means TMDB metadata for the title has not been fetched yet.
var ErrNoMetadata = errors.New("no TMDB metadata for this title yet")

func (s *TitleOverviewService) Availability(ctx context.Context, ref TmdbRef, country string) (*AvailabilityResponse, error) {
	country, err := validCountry(country, false)
	if err != nil {
		return nil, err
	}
	meta, err := s.metadata.Get(ctx, ref)
	if err != nil {
		return nil, err
	}
	if meta == nil || meta.DetailsFetchedAt == nil {
		return nil, ErrNoMetadata
	}
	providers, err := s.metadata.WatchProviders(ctx, ref, country)
	if err != nil {
		return nil, err
	}
	out := &AvailabilityResponse{Kind: ref.Kind, TmdbID: ref.ID, Country: optStr(country), FetchedAt: meta.DetailsFetchedAt,
		Attribution: "Watch provider data by JustWatch, via TMDB", Countries: map[string]*CountryOffers{}}
	for _, p := range providers {
		c := out.Countries[p.Country]
		if c == nil {
			c = &CountryOffers{Link: p.Link, Flatrate: []repository.WatchProvider{}, Free: []repository.WatchProvider{},
				Ads: []repository.WatchProvider{}, Rent: []repository.WatchProvider{}, Buy: []repository.WatchProvider{}}
			out.Countries[p.Country] = c
		}
		p.Link = nil // per country, above
		switch p.Monetization {
		case "flatrate":
			c.Flatrate = append(c.Flatrate, p)
		case "free":
			c.Free = append(c.Free, p)
		case "ads":
			c.Ads = append(c.Ads, p)
		case "rent":
			c.Rent = append(c.Rent, p)
		case "buy":
			c.Buy = append(c.Buy, p)
		}
	}
	return out, nil
}

// --- netflix --------------------------------------------------------------------

// TitleNetflixResponse is a title's Netflix official Top 10 history.
type TitleNetflixResponse struct {
	Kind         string                      `json:"kind"`
	TmdbID       string                      `json:"tmdb_id"`
	WeeksGlobal  int                         `json:"weeks_in_global_top10"`
	BestRank     *int64                      `json:"best_global_rank"`
	TotalViews   int64                       `json:"total_views"`
	TotalHours   int64                       `json:"total_hours_viewed"`
	Countries    []string                    `json:"countries"`
	Global       []repository.NetflixWeekRow `json:"global"`
	CountryWeeks []repository.NetflixWeekRow `json:"country_weeks"`
}

func (s *TitleOverviewService) Netflix(ctx context.Context, ref TmdbRef) (*TitleNetflixResponse, error) {
	global, countries, err := s.analytics.NetflixForTitle(ctx, titleKindForTmdb(ref.Kind), ref.ID)
	if err != nil {
		return nil, err
	}
	if len(global) == 0 && len(countries) == 0 {
		return nil, ErrTitleNotFound
	}
	out := &TitleNetflixResponse{Kind: ref.Kind, TmdbID: ref.ID, Global: global, CountryWeeks: countries, Countries: []string{}}
	weeks, cs := map[string]bool{}, map[string]bool{}
	for _, g := range global {
		weeks[g.Week] = true
		if out.BestRank == nil || g.Rank < *out.BestRank {
			r := g.Rank
			out.BestRank = &r
		}
		if g.Views != nil {
			out.TotalViews += *g.Views
		}
		if g.HoursViewed != nil {
			out.TotalHours += *g.HoursViewed
		}
	}
	for _, c := range countries {
		cs[c.Country] = true
	}
	out.WeeksGlobal, out.Countries = len(weeks), sortedKeys(cs)
	return out, nil
}

// --- lookup ---------------------------------------------------------------------

// LookupResult maps an external ID to TMDB titles and FlixPatrol titles.
type LookupResult struct {
	Refs   []LookupRef   `json:"items"`
	Titles []RankedTitle `json:"flixpatrol_titles"`
}

type LookupRef struct {
	Kind   string `json:"kind"`
	TmdbID string `json:"tmdb_id"`
}

var (
	imdbIDRe = regexp.MustCompile(`^tt\d{5,10}$`)
	slugRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,200}$`)
)

// Lookup finds titles by IMDb ID, TMDB ID (kind:id) or FlixPatrol slug.
func (s *TitleOverviewService) Lookup(ctx context.Context, imdbID, tmdb, slug string) (*LookupResult, error) {
	set := 0
	for _, v := range []string{imdbID, tmdb, slug} {
		if strings.TrimSpace(v) != "" {
			set++
		}
	}
	if set != 1 {
		return nil, &ValidationError{Message: "give exactly one of imdb, tmdb (kind:id) or slug"}
	}
	out := &LookupResult{Refs: []LookupRef{}, Titles: []RankedTitle{}}
	refs := map[string]bool{}
	addRef := func(kind, id string) {
		if id != "" && !refs[kind+":"+id] {
			refs[kind+":"+id] = true
			out.Refs = append(out.Refs, LookupRef{Kind: kind, TmdbID: id})
		}
	}
	addTitle := func(slug, name string, imdb, rt *string) {
		out.Titles = append(out.Titles, RankedTitle{Slug: slug, Name: name, IMDbID: imdb, RottenTomatoesURL: rt})
	}
	switch {
	case imdbID != "":
		imdbID = strings.TrimSpace(imdbID)
		if !imdbIDRe.MatchString(imdbID) {
			return nil, &ValidationError{Message: "invalid imdb id, expected tt1234567"}
		}
		titles, err := s.analytics.TitlesByIMDb(ctx, imdbID)
		if err != nil {
			return nil, err
		}
		for _, t := range titles {
			addTitle(t.Slug, t.Name, nullStr(t.ImdbID), nullStr(t.RtUrl))
			if t.TmdbID.Valid {
				addRef(tmdbKindOf(flixpatrol.TitleKind(t.Kind)), t.TmdbID.String)
			}
		}
		more, err := s.analytics.TmdbRefsByIMDb(ctx, imdbID)
		if err != nil {
			return nil, err
		}
		for _, r := range more {
			addRef(r.Kind, r.ID)
		}
	case tmdb != "":
		kind, id, ok := strings.Cut(strings.TrimSpace(tmdb), ":")
		if !ok {
			return nil, &ValidationError{Message: "tmdb must be kind:id, e.g. movie:425"}
		}
		ref, err := ParseTmdbRef(kind, id)
		if err != nil {
			return nil, err
		}
		titles, err := s.analytics.TitlesByTmdb(ctx, titleKindForTmdb(ref.Kind), ref.ID)
		if err != nil {
			return nil, err
		}
		for _, t := range titles {
			addTitle(t.Slug, t.Name, nullStr(t.ImdbID), nullStr(t.RtUrl))
		}
		meta, err := s.metadata.Get(ctx, ref)
		if err != nil {
			return nil, err
		}
		if len(titles) > 0 || meta != nil {
			addRef(ref.Kind, ref.ID)
		}
	default:
		slug = strings.ToLower(strings.TrimSpace(slug))
		if !slugRe.MatchString(slug) {
			return nil, &ValidationError{Message: "invalid slug"}
		}
		t, err := s.flix.GetTitleBySlug(ctx, slug)
		if err != nil {
			return nil, err
		}
		if t != nil {
			addTitle(t.Slug, t.Name, nullStr(t.ImdbID), nullStr(t.RtUrl))
			if t.TmdbID.Valid {
				addRef(tmdbKindOf(flixpatrol.TitleKind(t.Kind)), t.TmdbID.String)
			}
		}
	}
	if len(out.Refs) == 0 && len(out.Titles) == 0 {
		return nil, ErrTitleNotFound
	}
	return out, nil
}

// --- overview -------------------------------------------------------------------

// OverviewQuery selects the sections of a title overview.
type OverviewQuery struct {
	Country string          // filters availability and current positions
	Include map[string]bool // metadata, stats, current, ratings, availability, netflix ("" = all but netflix weeks)
}

// OverviewIncludes are the sections GET /titles/tmdb/{kind}/{id} can return.
var OverviewIncludes = []string{"metadata", "stats", "current", "ratings", "availability", "netflix"}

// ParseIncludes parses include=a,b ("" = every section).
func ParseIncludes(raw string) (map[string]bool, error) {
	out := map[string]bool{}
	if strings.TrimSpace(raw) == "" {
		for _, k := range OverviewIncludes {
			out[k] = true
		}
		return out, nil
	}
	valid := map[string]bool{}
	for _, k := range OverviewIncludes {
		valid[k] = true
	}
	for _, p := range strings.Split(raw, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if !valid[p] {
			return nil, &ValidationError{Message: fmt.Sprintf("invalid include %q, expected any of %s", p, strings.Join(OverviewIncludes, ","))}
		}
		out[p] = true
	}
	return out, nil
}

// TitleOverview combines everything known about a TMDB title.
type TitleOverview struct {
	Kind         string                   `json:"kind"`
	TmdbID       string                   `json:"tmdb_id"`
	Titles       []RankedTitle            `json:"flixpatrol_titles"`
	Metadata     *repository.TmdbMetadata `json:"metadata,omitempty"`
	Stats        *TitleStats              `json:"stats,omitempty"`
	Current      []ChartMoment            `json:"current_positions,omitempty"`
	Ratings      *RatingsResponse         `json:"ratings,omitempty"`
	Availability *AvailabilityResponse    `json:"availability,omitempty"`
	Netflix      *NetflixSummary          `json:"netflix,omitempty"`
	Errors       map[string]string        `json:"errors,omitempty"` // sections that could not be loaded
}

// NetflixSummary is the overview's Netflix section (weeks via /netflix).
type NetflixSummary struct {
	WeeksGlobal int      `json:"weeks_in_global_top10"`
	BestRank    *int64   `json:"best_global_rank"`
	TotalViews  int64    `json:"total_views"`
	TotalHours  int64    `json:"total_hours_viewed"`
	Countries   []string `json:"countries"`
}

// Overview returns a title's sections; a missing section (e.g. no ratings
// provider reachable) is reported in Errors rather than failing the request.
// ErrTitleNotFound when nothing at all is known about the title.
func (s *TitleOverviewService) Overview(ctx context.Context, ref TmdbRef, q OverviewQuery) (*TitleOverview, error) {
	country, err := validCountry(q.Country, false)
	if err != nil {
		return nil, err
	}
	out := &TitleOverview{Kind: ref.Kind, TmdbID: ref.ID, Titles: []RankedTitle{}, Errors: map[string]string{}}
	known := false

	meta, err := s.metadata.Get(ctx, ref)
	if err != nil {
		return nil, err
	}
	if meta != nil {
		known = true
		if q.Include["metadata"] {
			out.Metadata = meta
		}
	}

	rankings, err := s.rankingsForRef(ctx, ref, RankingHistoryQuery{})
	switch {
	case errors.Is(err, ErrTitleNotFound):
	case err != nil:
		return nil, err
	default:
		known = true
		out.Titles = rankings.Titles
		if q.Include["stats"] {
			st := Stats(rankings.Rankings)
			out.Stats = &st
		}
		if q.Include["current"] {
			out.Current = s.currentPositions(ctx, rankings.Rankings, country)
		}
	}

	if q.Include["netflix"] {
		nf, err := s.Netflix(ctx, ref)
		switch {
		case errors.Is(err, ErrTitleNotFound):
		case err != nil:
			out.Errors["netflix"] = "unavailable"
		default:
			known = true
			out.Netflix = &NetflixSummary{WeeksGlobal: nf.WeeksGlobal, BestRank: nf.BestRank, TotalViews: nf.TotalViews, TotalHours: nf.TotalHours, Countries: nf.Countries}
		}
	}
	if !known {
		return nil, ErrTitleNotFound
	}
	if q.Include["availability"] {
		av, err := s.Availability(ctx, ref, country)
		switch {
		case errors.Is(err, ErrNoMetadata):
		case err != nil:
			out.Errors["availability"] = "unavailable"
		default:
			out.Availability = av
		}
	}
	if q.Include["ratings"] && s.ratings != nil {
		r, err := s.ratings.GetRatings(ctx, ref, false)
		if err != nil {
			out.Errors["ratings"] = "unavailable"
		} else {
			out.Ratings = r
		}
	}
	if len(out.Errors) == 0 {
		out.Errors = nil
	}
	return out, nil
}

// currentPositions returns the title's entries on each chart's latest stored
// date (i.e. where it charts right now).
func (s *TitleOverviewService) currentPositions(ctx context.Context, rankings []RankingSnapshot, country string) []ChartMoment {
	out := []ChartMoment{}
	if len(rankings) == 0 {
		return out
	}
	charts := NewChartsService(s.analytics)
	catalog, err := charts.Catalog(ctx, country, "")
	if err != nil {
		return out
	}
	latest := map[string]string{}
	for _, c := range catalog.Items {
		latest[c.Country+"|"+c.Provider+"|"+c.Category] = c.LatestDate
	}
	for _, r := range rankings {
		if country != "" && r.Country != country {
			continue
		}
		if latest[r.Country+"|"+r.Provider+"|"+r.Category] == r.Date {
			out = append(out, ChartMoment{Date: r.Date, Country: r.Country, Provider: r.Provider, Category: r.Category, Rank: r.Rank})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out
}
