package service

import (
	"context"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// AnalyticsService computes cross-chart analytics from stored rankings,
// metadata, ratings and Netflix's official numbers.
type AnalyticsService struct {
	charts   *ChartsService
	repo     *repository.AnalyticsRepository
	metadata *repository.MetadataRepository
	imports  *repository.ImportsRepository
	ratings  *repository.RatingsRepository
	now      func() time.Time
}

func NewAnalyticsService(repo *repository.AnalyticsRepository, metadata *repository.MetadataRepository, imports *repository.ImportsRepository, ratings *repository.RatingsRepository) *AnalyticsService {
	return &AnalyticsService{charts: NewChartsService(repo), repo: repo, metadata: metadata, imports: imports, ratings: ratings, now: time.Now}
}

// AnalyticsQuery is the common filter of analytics endpoints.
type AnalyticsQuery struct {
	From, To                    *time.Time
	Country, Provider, Category string
	Kind                        string
}

type analyticsFilter struct {
	from, to                          time.Time
	country, provider, category, kind string
}

const maxAnalyticsDays = 366

func (s *AnalyticsService) filter(q AnalyticsQuery, defaultDays int) (analyticsFilter, error) {
	var f analyticsFilter
	var err error
	if f.country, err = validCountry(q.Country, false); err != nil {
		return f, err
	}
	if f.provider, err = validProvider(q.Provider); err != nil {
		return f, err
	}
	if f.category, err = validCategory(q.Category); err != nil {
		return f, err
	}
	if f.kind, err = validKind(q.Kind); err != nil {
		return f, err
	}
	f.from, f.to, err = s.charts.dateRange(q.From, q.To, defaultDays, maxAnalyticsDays)
	return f, err
}

func (f analyticsFilter) echo() AnalyticsEcho {
	return AnalyticsEcho{From: repository.FormatDate(f.from), To: repository.FormatDate(f.to),
		Country: optStr(f.country), Provider: optStr(f.provider), Category: optStr(f.category), Kind: optStr(f.kind)}
}

// AnalyticsEcho repeats the resolved filter in responses.
type AnalyticsEcho struct {
	From     string  `json:"from"`
	To       string  `json:"to"`
	Country  *string `json:"country"`
	Provider *string `json:"provider"`
	Category *string `json:"category"`
	Kind     *string `json:"kind"`
}

func titleKey(e repository.ChartEntryRow) string {
	if e.TmdbID != nil && *e.TmdbID != "" {
		return e.Kind + ":" + *e.TmdbID
	}
	return "id:" + strconv.FormatInt(e.TitleID, 10)
}

func parseDay(s string) time.Time {
	t, _ := time.Parse(repository.DateLayout, s)
	return t
}

// --- decay ------------------------------------------------------------------------

// DecayPoint is the average state of debuting titles N days after debut.
type DecayPoint struct {
	Day       int      `json:"day"`        // 0 = debut day
	Titles    int      `json:"titles"`     // titles whose day N is within the data
	Charting  int      `json:"charting"`   // of those, still on the chart
	Retention float64  `json:"retention"`  // charting / titles
	AvgRank   *float64 `json:"avg_rank"`   // among those charting
	AvgPoints float64  `json:"avg_points"` // points per tracked title (0 when off chart)
}

type DecayResponse struct {
	AnalyticsEcho
	Debuts int          `json:"debuts"` // chart debuts in the range
	Curve  []DecayPoint `json:"curve"`
}

const decayDays = 30

// Decay follows every title that debuted on a chart in the range (not on that
// chart in the 30 days before) for 30 days: retention and average rank.
// Titles on a chart's first stored date are not counted as debuts.
func (s *AnalyticsService) Decay(ctx context.Context, q AnalyticsQuery) (*DecayResponse, error) {
	f, err := s.filter(q, 60)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ChartEntries(ctx, repository.ChartEntryFilter{
		From: f.from.AddDate(0, 0, -decayDays), To: f.to.AddDate(0, 0, decayDays), Country: f.country, Provider: f.provider, Category: f.category,
	})
	if err != nil {
		return nil, err
	}
	type chartTitleKey struct{ chart, title string }
	ranks := map[chartTitleKey]map[string]int64{}
	chartDates := map[string]map[string]bool{}
	for _, r := range rows {
		if f.kind != "" && r.Kind != f.kind {
			continue
		}
		chart := r.Country + "|" + r.Provider + "|" + r.Category
		k := chartTitleKey{chart, titleKey(r)}
		if ranks[k] == nil {
			ranks[k] = map[string]int64{}
		}
		if cur, ok := ranks[k][r.Date]; !ok || r.Rank < cur {
			ranks[k][r.Date] = r.Rank
		}
		if chartDates[chart] == nil {
			chartDates[chart] = map[string]bool{}
		}
		chartDates[chart][r.Date] = true
	}
	lastData := f.to.AddDate(0, 0, decayDays)
	if cur := flixpatrol.EffectiveTop10Date(s.now()); cur.Before(lastData) {
		lastData = cur
	}
	firstStored := map[string]string{}
	for chart, ds := range chartDates {
		for d := range ds {
			if firstStored[chart] == "" || d < firstStored[chart] {
				firstStored[chart] = d
			}
		}
	}
	type agg struct {
		titles, charting int
		rankSum, points  float64
	}
	curve := make([]agg, decayDays)
	debuts := 0
	for k, byDate := range ranks {
		var first string
		for d := range byDate {
			if first == "" || d < first {
				first = d
			}
		}
		debut := parseDay(first)
		if debut.Before(f.from) || debut.After(f.to) {
			continue // debuted outside the range (or charted before it)
		}
		if first == firstStored[k.chart] {
			continue // on the chart's first stored date: can't tell if it was new
		}
		debuts++
		for day := 0; day < decayDays; day++ {
			d := debut.AddDate(0, 0, day)
			if d.After(lastData) || !chartDates[k.chart][repository.FormatDate(d)] {
				continue // chart not stored that day: no information
			}
			curve[day].titles++
			if rank, ok := byDate[repository.FormatDate(d)]; ok {
				curve[day].charting++
				curve[day].rankSum += float64(rank)
				curve[day].points += float64(Points(rank))
			}
		}
	}
	out := &DecayResponse{AnalyticsEcho: f.echo(), Debuts: debuts, Curve: []DecayPoint{}}
	for day, a := range curve {
		if a.titles == 0 {
			continue
		}
		p := DecayPoint{Day: day, Titles: a.titles, Charting: a.charting, Retention: round(float64(a.charting)/float64(a.titles), 3),
			AvgPoints: round(a.points/float64(a.titles), 2)}
		if a.charting > 0 {
			avg := round(a.rankSum/float64(a.charting), 2)
			p.AvgRank = &avg
		}
		out.Curve = append(out.Curve, p)
	}
	return out, nil
}

// --- country similarity -----------------------------------------------------

type CountryPair struct {
	A       string  `json:"a"`
	B       string  `json:"b"`
	Jaccard float64 `json:"jaccard"` // shared titles / titles in either
	Shared  int     `json:"shared"`
}

type SimilarityResponse struct {
	AnalyticsEcho
	Countries map[string]int `json:"countries"` // distinct titles per country
	Pairs     []CountryPair  `json:"pairs"`     // most similar first
}

// CountrySimilarity compares which titles chart in each country (Jaccard
// overlap of the title sets) for the range.
func (s *AnalyticsService) CountrySimilarity(ctx context.Context, q AnalyticsQuery) (*SimilarityResponse, error) {
	q.Country = ""
	f, err := s.filter(q, 30)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ChartEntries(ctx, repository.ChartEntryFilter{From: f.from, To: f.to, Provider: f.provider, Category: f.category})
	if err != nil {
		return nil, err
	}
	sets := map[string]map[string]bool{}
	for _, r := range rows {
		if f.kind != "" && r.Kind != f.kind {
			continue
		}
		if sets[r.Country] == nil {
			sets[r.Country] = map[string]bool{}
		}
		sets[r.Country][titleKey(r)] = true
	}
	countries := sortedKeys(func() map[string]bool {
		m := map[string]bool{}
		for c := range sets {
			m[c] = true
		}
		return m
	}())
	out := &SimilarityResponse{AnalyticsEcho: f.echo(), Countries: map[string]int{}, Pairs: []CountryPair{}}
	for _, c := range countries {
		out.Countries[c] = len(sets[c])
	}
	for i := 0; i < len(countries); i++ {
		for j := i + 1; j < len(countries); j++ {
			a, b := sets[countries[i]], sets[countries[j]]
			shared := 0
			for k := range a {
				if b[k] {
					shared++
				}
			}
			union := len(a) + len(b) - shared
			p := CountryPair{A: countries[i], B: countries[j], Shared: shared}
			if union > 0 {
				p.Jaccard = round(float64(shared)/float64(union), 3)
			}
			out.Pairs = append(out.Pairs, p)
		}
	}
	sort.SliceStable(out.Pairs, func(i, j int) bool { return out.Pairs[i].Jaccard > out.Pairs[j].Jaccard })
	return out, nil
}

// --- release lag --------------------------------------------------------------

type ReleaseLagItem struct {
	ChartTitle
	Country      string `json:"country"`
	Provider     string `json:"provider"`
	ReleaseDate  string `json:"release_date"`
	ReleaseBasis string `json:"release_basis"` // digital_<country> | tmdb_release
	FirstChart   string `json:"first_chart_date"`
	LagDays      int    `json:"lag_days"`
	PeakRank     int64  `json:"peak_rank"`
}

type LagSummary struct {
	Titles int      `json:"titles"`
	Median *float64 `json:"median_days"`
	P25    *float64 `json:"p25_days"`
	P75    *float64 `json:"p75_days"`
}

type ReleaseLagResponse struct {
	AnalyticsEcho
	Summary    LagSummary            `json:"summary"`
	ByProvider map[string]LagSummary `json:"by_provider"`
	Items      []ReleaseLagItem      `json:"items"`
}

// ReleaseLag measures days from a title's release (its digital release in
// the chart's country when TMDB has one, else TMDB's release / first air
// date) to its first chart appearance in the range. Titles with lags over a
// year (catalog titles resurfacing) are left out.
func (s *AnalyticsService) ReleaseLag(ctx context.Context, q AnalyticsQuery) (*ReleaseLagResponse, error) {
	f, err := s.filter(q, 90)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ChartEntries(ctx, repository.ChartEntryFilter{From: f.from, To: f.to, Country: f.country, Provider: f.provider, Category: f.category})
	if err != nil {
		return nil, err
	}
	type firstKey struct{ ref, country, provider string }
	first := map[firstKey]*ReleaseLagItem{}
	var keys []firstKey
	refs := map[string]repository.TmdbRef{}
	for _, r := range rows {
		if r.TmdbID == nil || *r.TmdbID == "" || (f.kind != "" && r.Kind != f.kind) {
			continue
		}
		ref := repository.TmdbRef{Kind: tmdbKindOf(flixpatrol.TitleKind(r.Kind)), ID: *r.TmdbID}
		refs[ref.String()] = ref
		k := firstKey{ref.String(), r.Country, r.Provider}
		it := first[k]
		if it == nil {
			it = &ReleaseLagItem{ChartTitle: chartTitle(r), Country: r.Country, Provider: r.Provider, FirstChart: r.Date, PeakRank: r.Rank}
			first[k] = it
			keys = append(keys, k)
		}
		if r.Rank < it.PeakRank {
			it.PeakRank = r.Rank
		}
	}
	list := make([]repository.TmdbRef, 0, len(refs))
	for _, r := range refs {
		list = append(list, r)
	}
	meta, err := s.metadata.GetMany(ctx, list)
	if err != nil {
		return nil, err
	}
	out := &ReleaseLagResponse{AnalyticsEcho: f.echo(), ByProvider: map[string]LagSummary{}, Items: []ReleaseLagItem{}}
	var all []float64
	byProvider := map[string][]float64{}
	for _, k := range keys {
		m := meta[k.ref]
		if m == nil {
			continue
		}
		it := first[k]
		release, basis := "", ""
		if d, ok := m.DigitalReleaseDates[k.country]; ok {
			release, basis = d, "digital_"+k.country
		} else if m.ReleaseDate != nil && *m.ReleaseDate != "" {
			release, basis = *m.ReleaseDate, "tmdb_release"
		}
		if release == "" {
			continue
		}
		lag := int(parseDay(it.FirstChart).Sub(parseDay(release)).Hours() / 24)
		if lag < -30 || lag > 365 {
			continue
		}
		it.ReleaseDate, it.ReleaseBasis, it.LagDays = release, basis, lag
		out.Items = append(out.Items, *it)
		all = append(all, float64(lag))
		byProvider[it.Provider] = append(byProvider[it.Provider], float64(lag))
	}
	sort.SliceStable(out.Items, func(i, j int) bool { return out.Items[i].FirstChart > out.Items[j].FirstChart })
	out.Summary = lagSummary(all)
	for p, v := range byProvider {
		out.ByProvider[p] = lagSummary(v)
	}
	return out, nil
}

func lagSummary(v []float64) LagSummary {
	s := LagSummary{Titles: len(v)}
	if len(v) == 0 {
		return s
	}
	sort.Float64s(v)
	med, p25, p75 := quantile(v, 0.5), quantile(v, 0.25), quantile(v, 0.75)
	s.Median, s.P25, s.P75 = &med, &p25, &p75
	return s
}

func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return round(sorted[lo]+(sorted[hi]-sorted[lo])*(pos-float64(lo)), 2)
}

// --- ratings vs popularity ------------------------------------------------------

type RatedTitle struct {
	ChartTitle
	Points        int64    `json:"points"`
	BestRank      int64    `json:"best_rank"`
	IMDbRating    *float64 `json:"imdb_rating"`
	IMDbVotes     *int64   `json:"imdb_votes"`
	RTCritics     *float64 `json:"rt_critics"`
	RTAudience    *float64 `json:"rt_audience"`
	RatingsSource string   `json:"imdb_source,omitempty"` // imdb_dataset | justwatch | mdblist
}

type RatingsVsPopularityResponse struct {
	AnalyticsEcho
	Titles                int          `json:"titles"`
	WithIMDb              int          `json:"with_imdb"`
	CorrelationIMDb       *float64     `json:"correlation_points_imdb"` // Pearson r
	CorrelationRT         *float64     `json:"correlation_points_rt_critics"`
	AcclaimedUnderwatched []RatedTitle `json:"acclaimed_underwatched"` // IMDb ≥ 7.5, points below median
	PopularPoorlyRated    []RatedTitle `json:"popular_poorly_rated"`   // top-quartile points, IMDb < 6
	Items                 []RatedTitle `json:"items"`
}

// RatingsVsPopularity joins chart points with stored ratings (IMDb's dataset
// first, then provider ratings; RT from title_rating_sources).
func (s *AnalyticsService) RatingsVsPopularity(ctx context.Context, q AnalyticsQuery) (*RatingsVsPopularityResponse, error) {
	f, err := s.filter(q, 30)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ChartEntries(ctx, repository.ChartEntryFilter{From: f.from, To: f.to, Country: f.country, Provider: f.provider, Category: f.category})
	if err != nil {
		return nil, err
	}
	board := aggregateLeaderboard(rows, f.kind, 300)
	out := &RatingsVsPopularityResponse{AnalyticsEcho: f.echo(), AcclaimedUnderwatched: []RatedTitle{}, PopularPoorlyRated: []RatedTitle{}, Items: []RatedTitle{}}
	var imdbIDs []string
	for _, e := range board {
		if e.TmdbID == nil {
			continue
		}
		if e.IMDbID != nil {
			imdbIDs = append(imdbIDs, *e.IMDbID)
		}
	}
	dataset, err := s.imports.IMDbRatings(ctx, imdbIDs)
	if err != nil {
		return nil, err
	}
	for _, e := range board {
		if e.TmdbID == nil {
			continue
		}
		rt := RatedTitle{ChartTitle: e.ChartTitle, Points: e.Points, BestRank: e.BestRank}
		if e.IMDbID != nil {
			if d, ok := dataset[*e.IMDbID]; ok {
				v, votes := d.Rating, d.Votes
				rt.IMDbRating, rt.IMDbVotes, rt.RatingsSource = &v, &votes, ProviderIMDbDataset
			}
		}
		_, sources, err := s.ratings.GetTitleRatings(ctx, tmdbKindOf(flixpatrol.TitleKind(e.Kind)), *e.TmdbID)
		if err != nil {
			return nil, err
		}
		for _, src := range sources {
			v := src.Value
			switch {
			case src.Source == sourceIMDb && rt.IMDbRating == nil:
				rt.IMDbRating, rt.IMDbVotes, rt.RatingsSource = &v, src.Votes, src.Provider
			case src.Source == sourceTomatoes && (rt.RTCritics == nil || src.Provider == ProviderRottenTomatoes):
				rt.RTCritics = &v
			case src.Source == sourcePopcorn && (rt.RTAudience == nil || src.Provider == ProviderRottenTomatoes):
				rt.RTAudience = &v
			}
		}
		out.Items = append(out.Items, rt)
	}
	out.Titles = len(out.Items)

	var pts, imdb, ptsRT, rtc []float64
	for _, it := range out.Items {
		if it.IMDbRating != nil {
			pts, imdb = append(pts, float64(it.Points)), append(imdb, *it.IMDbRating)
		}
		if it.RTCritics != nil {
			ptsRT, rtc = append(ptsRT, float64(it.Points)), append(rtc, *it.RTCritics)
		}
	}
	out.WithIMDb = len(imdb)
	out.CorrelationIMDb, out.CorrelationRT = pearson(pts, imdb), pearson(ptsRT, rtc)
	if len(out.Items) > 0 {
		sortedPts := make([]float64, 0, len(out.Items))
		for _, it := range out.Items {
			sortedPts = append(sortedPts, float64(it.Points))
		}
		sort.Float64s(sortedPts)
		median, q3 := quantile(sortedPts, 0.5), quantile(sortedPts, 0.75)
		for _, it := range out.Items {
			if it.IMDbRating == nil {
				continue
			}
			if *it.IMDbRating >= 7.5 && float64(it.Points) < median {
				out.AcclaimedUnderwatched = append(out.AcclaimedUnderwatched, it)
			}
			if *it.IMDbRating < 6 && float64(it.Points) >= q3 {
				out.PopularPoorlyRated = append(out.PopularPoorlyRated, it)
			}
		}
	}
	return out, nil
}

// pearson returns Pearson's r, or nil with fewer than 3 points or no variance.
func pearson(x, y []float64) *float64 {
	n := len(x)
	if n < 3 || n != len(y) {
		return nil
	}
	var sx, sy float64
	for i := range x {
		sx += x[i]
		sy += y[i]
	}
	mx, my := sx/float64(n), sy/float64(n)
	var cov, vx, vy float64
	for i := range x {
		dx, dy := x[i]-mx, y[i]-my
		cov += dx * dy
		vx += dx * dx
		vy += dy * dy
	}
	if vx == 0 || vy == 0 {
		return nil
	}
	r := round(cov/math.Sqrt(vx*vy), 3)
	return &r
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

// --- genres ---------------------------------------------------------------------

type GenreShare struct {
	Genre  string  `json:"genre"`
	Points int64   `json:"points"`
	Titles int     `json:"titles"`
	Share  float64 `json:"share"` // of points of titles with genres
}

type GenresResponse struct {
	AnalyticsEcho
	PointsWithGenres int64        `json:"points_with_genres"`
	PointsUnknown    int64        `json:"points_without_genres"` // unmatched or no TMDB metadata yet
	Items            []GenreShare `json:"items"`
}

// Genres splits chart points by TMDB genre (a title's points count towards
// each of its genres; shares are of the total points of titles with genres).
func (s *AnalyticsService) Genres(ctx context.Context, q AnalyticsQuery) (*GenresResponse, error) {
	f, err := s.filter(q, 30)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ChartEntries(ctx, repository.ChartEntryFilter{From: f.from, To: f.to, Country: f.country, Provider: f.provider, Category: f.category})
	if err != nil {
		return nil, err
	}
	board := aggregateLeaderboard(rows, f.kind, math.MaxInt32)
	var refs []repository.TmdbRef
	for _, e := range board {
		if e.TmdbID != nil {
			refs = append(refs, repository.TmdbRef{Kind: tmdbKindOf(flixpatrol.TitleKind(e.Kind)), ID: *e.TmdbID})
		}
	}
	meta, err := s.metadata.GetMany(ctx, refs)
	if err != nil {
		return nil, err
	}
	out := &GenresResponse{AnalyticsEcho: f.echo(), Items: []GenreShare{}}
	byGenre := map[string]*GenreShare{}
	for _, e := range board {
		var genres []string
		if e.TmdbID != nil {
			if m := meta[tmdbKindOf(flixpatrol.TitleKind(e.Kind))+":"+*e.TmdbID]; m != nil {
				genres = m.Genres
			}
		}
		if len(genres) == 0 {
			out.PointsUnknown += e.Points
			continue
		}
		out.PointsWithGenres += e.Points
		for _, g := range genres {
			gs := byGenre[g]
			if gs == nil {
				gs = &GenreShare{Genre: g}
				byGenre[g] = gs
			}
			gs.Points += e.Points
			gs.Titles++
		}
	}
	for _, gs := range byGenre {
		if out.PointsWithGenres > 0 {
			gs.Share = round(float64(gs.Points)/float64(out.PointsWithGenres), 3)
		}
		out.Items = append(out.Items, *gs)
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].Points > out.Items[j].Points })
	return out, nil
}

// --- Netflix calibration -----------------------------------------------------

type CalibrationRow struct {
	Week        string   `json:"week"` // Netflix week (Mon–Sun) ending on this Sunday
	ShowTitle   string   `json:"show_title"`
	Kind        string   `json:"kind"`
	TmdbID      string   `json:"tmdb_id"`
	NetflixRank int64    `json:"netflix_global_rank"`
	Views       *int64   `json:"netflix_views"`
	HoursViewed *int64   `json:"netflix_hours_viewed"`
	FPPoints    int64    `json:"flixpatrol_points"` // Netflix chart points that week
	FPDays      int      `json:"flixpatrol_days"`
	ViewsPerPt  *float64 `json:"views_per_point,omitempty"`
}

type CalibrationResponse struct {
	AnalyticsEcho
	Rows              []CalibrationRow `json:"rows"`
	CorrelationViews  *float64         `json:"correlation_points_views"`
	MedianViewsPerPt  *float64         `json:"median_views_per_point"`
	WeeksWithFPCharts int              `json:"weeks_with_flixpatrol_charts"`
}

// NetflixCalibration lines up Netflix's official weekly views (global) with
// the FlixPatrol Netflix chart points the same titles earned that week (in a
// country, or summed over countries), to see what a chart position is worth.
func (s *AnalyticsService) NetflixCalibration(ctx context.Context, q AnalyticsQuery) (*CalibrationResponse, error) {
	q.Provider, q.Category = "netflix", ""
	f, err := s.filter(q, 84)
	if err != nil {
		return nil, err
	}
	nf, err := s.repo.NetflixGlobalRange(ctx, repository.FormatDate(f.from), repository.FormatDate(f.to))
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ChartEntries(ctx, repository.ChartEntryFilter{From: f.from.AddDate(0, 0, -6), To: f.to, Country: f.country, Provider: "netflix"})
	if err != nil {
		return nil, err
	}
	// points[title key][week] where week = the Sunday ending the entry's week.
	points := map[string]map[string]int64{}
	days := map[string]map[string]map[string]bool{}
	fpWeeks := map[string]bool{}
	for _, r := range rows {
		if r.TmdbID == nil || *r.TmdbID == "" {
			continue
		}
		d := parseDay(r.Date)
		week := repository.FormatDate(d.AddDate(0, 0, (7-int(d.Weekday()))%7))
		fpWeeks[week] = true
		k := r.Kind + ":" + *r.TmdbID
		if points[k] == nil {
			points[k], days[k] = map[string]int64{}, map[string]map[string]bool{}
		}
		points[k][week] += Points(r.Rank)
		if days[k][week] == nil {
			days[k][week] = map[string]bool{}
		}
		days[k][week][r.Date] = true
	}
	out := &CalibrationResponse{AnalyticsEcho: f.echo(), Rows: []CalibrationRow{}, WeeksWithFPCharts: 0}
	var xs, ys, perPt []float64
	for _, n := range nf {
		if n.TmdbID == nil || !fpWeeks[n.Week] {
			continue
		}
		if f.kind != "" && n.Kind != f.kind {
			continue
		}
		k := n.Kind + ":" + *n.TmdbID
		row := CalibrationRow{Week: n.Week, ShowTitle: n.ShowTitle, Kind: n.Kind, TmdbID: *n.TmdbID, NetflixRank: n.Rank,
			Views: n.Views, HoursViewed: n.HoursViewed, FPPoints: points[k][n.Week], FPDays: len(days[k][n.Week])}
		if row.Views != nil && row.FPPoints > 0 {
			v := round(float64(*row.Views)/float64(row.FPPoints), 0)
			row.ViewsPerPt = &v
			perPt = append(perPt, v)
			xs, ys = append(xs, float64(row.FPPoints)), append(ys, float64(*row.Views))
		}
		out.Rows = append(out.Rows, row)
	}
	for w := range fpWeeks {
		if !parseDay(w).Before(f.from) && !parseDay(w).After(f.to) {
			out.WeeksWithFPCharts++
		}
	}
	out.CorrelationViews = pearson(xs, ys)
	if len(perPt) > 0 {
		sort.Float64s(perPt)
		m := quantile(perPt, 0.5)
		out.MedianViewsPerPt = &m
	}
	return out, nil
}

// --- Netflix official reads -----------------------------------------------------

type NetflixWeekResponse struct {
	Week     string                      `json:"week"`
	Country  *string                     `json:"country"` // null = global
	Category *string                     `json:"category"`
	Items    []repository.NetflixWeekRow `json:"items"`
}

// NetflixWeek returns Netflix's official Top 10 for a week (default latest);
// country "" = global. week is a date within the week or its Sunday.
func (s *AnalyticsService) NetflixWeek(ctx context.Context, country, week, category string) (*NetflixWeekResponse, error) {
	var err error
	if country, err = validCountry(country, false); err != nil {
		return nil, err
	}
	if week == "" {
		if week, err = s.repo.LatestNetflixWeek(ctx, country); err != nil {
			return nil, err
		}
		if week == "" {
			return nil, ErrTop10NotFound
		}
	} else {
		d, err := time.Parse(repository.DateLayout, week)
		if err != nil {
			return nil, &ValidationError{Message: "invalid week, expected YYYY-MM-DD"}
		}
		week = repository.FormatDate(d.AddDate(0, 0, (7-int(d.Weekday()))%7))
	}
	var items []repository.NetflixWeekRow
	if country == "" {
		items, err = s.repo.NetflixGlobalWeek(ctx, week, category)
	} else {
		items, err = s.repo.NetflixCountryWeek(ctx, country, week, category)
	}
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrTop10NotFound
	}
	return &NetflixWeekResponse{Week: week, Country: optStr(country), Category: optStr(category), Items: items}, nil
}

type NetflixPopularResponse struct {
	Items []repository.NetflixPopularRow `json:"items"`
}

func (s *AnalyticsService) NetflixMostPopular(ctx context.Context, category string) (*NetflixPopularResponse, error) {
	items, err := s.repo.NetflixMostPopular(ctx, category)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrTop10NotFound
	}
	return &NetflixPopularResponse{Items: items}, nil
}
