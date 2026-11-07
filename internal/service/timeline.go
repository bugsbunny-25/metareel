package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Timeline event types (GET /titles/tmdb/{kind}/{id}/timeline?types=).
const (
	TimelineChartDebut          = "chart_debut"
	TimelineChartPeak           = "chart_peak"
	TimelineChartNumberOne      = "chart_number_one"
	TimelineChartReentry        = "chart_reentry"
	TimelineAvailabilityAdded   = "availability_added"
	TimelineAvailabilityRemoved = "availability_removed"
	TimelineRatingChanged       = "rating_changed"
	TimelinePageviewSpike       = "pageview_spike"
	TimelineNetflixTop10Week    = "netflix_top10_week"
	TimelineNetflixEngagement   = "netflix_engagement"
	TimelineSeasonPremiere      = "season_premiere"
	TimelineRelease             = "release"
	TimelineNetflixPremiere     = "netflix_premiere"
)

// TimelineEventTypes lists every event type, in display order for equal dates.
var TimelineEventTypes = []string{
	TimelineRelease, TimelineNetflixPremiere, TimelineSeasonPremiere,
	TimelineAvailabilityAdded, TimelineAvailabilityRemoved,
	TimelineChartDebut, TimelineChartReentry, TimelineChartPeak, TimelineChartNumberOne,
	TimelineNetflixTop10Week, TimelineNetflixEngagement,
	TimelinePageviewSpike, TimelineRatingChanged,
}

const (
	defaultTimelineLimit = 200
	maxTimelineLimit     = 1000
	// A chart return counts as a re-entry after at least this many days away
	// (and only when the chart was stored without the title in between).
	reentryMinGapDays = 7
	// Pageview spikes: a day with at least spikeMinViews views and
	// spikeRatio times the median of the previous 28 days (at least
	// spikeMinBaselineDays of them stored).
	spikeWindowDays      = 28
	spikeMinBaselineDays = 14
	spikeRatio           = 3.0
	spikeMinViews        = 1000
	maxTimelineOffers    = 5000
)

// TimelineService builds a title's timeline from charts, availability,
// rating history, Wikipedia pageviews, Netflix's numbers and TMDB dates.
type TimelineService struct {
	top10      *Top10ReadService
	metadata   *repository.MetadataRepository
	history    *repository.HistoryRepository
	analytics  *repository.AnalyticsRepository
	engagement *repository.EngagementRepository // optional
}

func NewTimelineService(top10 *Top10ReadService, metadata *repository.MetadataRepository, history *repository.HistoryRepository,
	analytics *repository.AnalyticsRepository, engagement *repository.EngagementRepository) *TimelineService {
	return &TimelineService{top10: top10, metadata: metadata, history: history, analytics: analytics, engagement: engagement}
}

// TimelineQuery filters a timeline. Country limits the events that have a
// country (charts, availability, Netflix weeks); the rest are worldwide and
// always kept.
type TimelineQuery struct {
	Types    string // comma-separated event types; empty = all
	Country  string
	From, To *time.Time
	Limit    int
}

// TimelineResponse is a title's events, newest first.
type TimelineResponse struct {
	Kind    string  `json:"kind"`
	TmdbID  string  `json:"tmdb_id"`
	Title   *string `json:"title"`
	Country *string `json:"country"`
	From    *string `json:"from"`
	To      *string `json:"to"`
	// Total: matching events before the limit; Counts: matching events per type.
	Total  int             `json:"total"`
	Counts map[string]int  `json:"counts"`
	Events []TimelineEvent `json:"events"`
}

// TimelineEvent is one dated event. Exactly one detail object is set,
// depending on the type's group.
type TimelineEvent struct {
	Type    string  `json:"type"`
	Date    string  `json:"date"`               // YYYY-MM-DD (UTC); a span's first day
	EndDate *string `json:"end_date,omitempty"` // a span's last day
	Summary string  `json:"summary"`

	Chart        *TimelineChart        `json:"chart,omitempty"`
	Availability *TimelineAvailability `json:"availability,omitempty"`
	Rating       *TimelineRating       `json:"rating,omitempty"`
	Pageviews    *TimelinePageviews    `json:"pageviews,omitempty"`
	Netflix      *TimelineNetflixWeek  `json:"netflix,omitempty"`
	Engagement   *TimelineEngagement   `json:"engagement,omitempty"`
	Season       *TimelineSeason       `json:"season,omitempty"`
}

// TimelineChart: chart_debut, chart_peak, chart_number_one, chart_reentry.
type TimelineChart struct {
	Country      string `json:"country"`
	Provider     string `json:"provider"`
	Category     string `json:"category"` // movies | tv_shows
	Rank         int64  `json:"rank"`
	SeasonNumber *int64 `json:"season_number,omitempty"`
	// Days: chart_number_one = days at #1 in the run; chart_peak = days
	// after the debut; chart_reentry = days away from the chart.
	Days *int64 `json:"days,omitempty"`
}

// TimelineAvailability: availability_added / availability_removed, one per
// provider, offer type and day, with every country it happened in.
type TimelineAvailability struct {
	ProviderID   int64    `json:"provider_id"`
	ProviderName string   `json:"provider_name"`
	Monetization string   `json:"monetization"` // flatrate | free | ads | rent | buy
	Countries    []string `json:"countries"`
}

// TimelineRating: rating_changed (the displayed value changed).
type TimelineRating struct {
	Provider      string  `json:"provider"` // imdb_dataset | rottentomatoes | mdblist | justwatch
	Source        string  `json:"source"`   // imdb, tomatoes, popcorn, metacritic, ...
	Value         float64 `json:"value"`
	PreviousValue float64 `json:"previous_value"`
	Votes         *int64  `json:"votes,omitempty"`
}

// TimelinePageviews: pageview_spike on the English Wikipedia article.
type TimelinePageviews struct {
	PeakDay       string  `json:"peak_day"`
	PeakViews     int64   `json:"peak_views"`
	BaselineViews int64   `json:"baseline_views"` // median daily views of the 28 days before the spike
	Ratio         float64 `json:"ratio"`          // peak_views / baseline_views
	Days          int64   `json:"days"`           // spike days in a row
}

// TimelineNetflixWeek: netflix_top10_week (Monday to Sunday, week = Sunday).
type TimelineNetflixWeek struct {
	Week            string  `json:"week"`
	Category        string  `json:"category"`
	GlobalRank      *int64  `json:"global_rank"` // null: only in country lists that week
	SeasonTitle     *string `json:"season_title,omitempty"`
	SeasonNumber    *int64  `json:"season_number,omitempty"`
	HoursViewed     *int64  `json:"hours_viewed,omitempty"`
	Views           *int64  `json:"views,omitempty"`
	CumulativeWeeks *int64  `json:"cumulative_weeks,omitempty"`
	Countries       int     `json:"countries"`              // country Top 10s it was in that week
	CountryRank     *int64  `json:"country_rank,omitempty"` // with ?country=
}

// TimelineEngagement: netflix_engagement, a row of a "What We Watched" report.
type TimelineEngagement struct {
	PeriodStart       string `json:"period_start"`
	PeriodEnd         string `json:"period_end"`
	Title             string `json:"title"`
	HoursViewed       *int64 `json:"hours_viewed"`
	Views             *int64 `json:"views"`
	RuntimeMinutes    *int64 `json:"runtime_minutes"`
	SeasonNumber      *int64 `json:"season_number,omitempty"`
	AvailableGlobally *bool  `json:"available_globally"`
}

// TimelineSeason: season_premiere (may be in the future).
type TimelineSeason struct {
	SeasonNumber int64  `json:"season_number"`
	Name         string `json:"name"`
	EpisodeCount int64  `json:"episode_count"`
}

// ParseTimelineTypes validates a comma-separated type list (nil = all).
func ParseTimelineTypes(raw string) (map[string]bool, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	valid := map[string]bool{}
	for _, t := range TimelineEventTypes {
		valid[t] = true
	}
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		t := strings.ToLower(strings.TrimSpace(part))
		if t == "" {
			continue
		}
		if !valid[t] {
			return nil, &ValidationError{Message: fmt.Sprintf("invalid type %q, expected one of %s", part, strings.Join(TimelineEventTypes, ", "))}
		}
		out[t] = true
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// Timeline returns a TMDB title's events, newest first; ErrTitleNotFound
// when metareel has neither metadata nor chart entries for it.
func (s *TimelineService) Timeline(ctx context.Context, ref TmdbRef, q TimelineQuery) (*TimelineResponse, error) {
	types, err := ParseTimelineTypes(q.Types)
	if err != nil {
		return nil, err
	}
	country, err := validCountry(q.Country, false)
	if err != nil {
		return nil, err
	}
	if q.From != nil && q.To != nil && q.From.After(*q.To) {
		return nil, &ValidationError{Message: "from must not be after to"}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = defaultTimelineLimit
	}
	if limit > maxTimelineLimit {
		return nil, &ValidationError{Message: fmt.Sprintf("limit must be at most %d", maxTimelineLimit)}
	}
	want := func(group ...string) bool {
		if types == nil {
			return true
		}
		for _, t := range group {
			if types[t] {
				return true
			}
		}
		return false
	}

	meta, err := s.metadata.Get(ctx, ref)
	if err != nil {
		return nil, err
	}
	rankings, err := s.top10.GetTitleRankings(ctx, ref, RankingHistoryQuery{CountryCode: country})
	if err != nil && !errors.Is(err, ErrTitleNotFound) {
		return nil, err
	}
	if meta == nil && rankings == nil {
		return nil, ErrTitleNotFound
	}

	out := &TimelineResponse{Kind: ref.Kind, TmdbID: ref.ID, Country: optStr(country), Counts: map[string]int{}, Events: []TimelineEvent{}}
	if meta != nil {
		out.Title = meta.Title
	}
	var from, to string
	if q.From != nil {
		from = repository.FormatDate(q.From.UTC())
		out.From = &from
	}
	if q.To != nil {
		to = repository.FormatDate(q.To.UTC())
		out.To = &to
	}
	var events []TimelineEvent
	add := func(evs []TimelineEvent) { events = append(events, evs...) }

	if rankings != nil && want(TimelineChartDebut, TimelineChartPeak, TimelineChartNumberOne, TimelineChartReentry) {
		// Re-entries cost a query per gap: only look for them when asked
		// for, and only in the requested range.
		var reentries *dateRange
		if want(TimelineChartReentry) {
			reentries = &dateRange{from: from, to: to}
		}
		evs, err := s.chartEvents(ctx, rankings.Rankings, reentries)
		if err != nil {
			return nil, err
		}
		add(evs)
	}
	if meta != nil && want(TimelineRelease, TimelineNetflixPremiere, TimelineSeasonPremiere) {
		add(metadataEvents(ref, meta))
	}
	if want(TimelineAvailabilityAdded, TimelineAvailabilityRemoved) {
		offers, err := s.history.TitleOfferEvents(ctx, ref, country, maxTimelineOffers)
		if err != nil {
			return nil, err
		}
		add(availabilityEvents(offers))
	}
	if want(TimelineRatingChanged) {
		evs, err := s.ratingEvents(ctx, ref, meta)
		if err != nil {
			return nil, err
		}
		add(evs)
	}
	if meta != nil && meta.EnwikiTitle != nil && *meta.EnwikiTitle != "" && want(TimelinePageviewSpike) {
		// Spikes need the 28 days before the range as a baseline.
		pvFrom, pvTo := "0001-01-01", "9999-12-31"
		if q.From != nil {
			pvFrom = repository.FormatDate(q.From.UTC().AddDate(0, 0, -spikeWindowDays))
		}
		if to != "" {
			pvTo = to
		}
		points, err := s.history.Pageviews(ctx, ref, pageviewsProject, pvFrom, pvTo)
		if err != nil {
			return nil, err
		}
		add(pageviewSpikes(points))
	}
	if want(TimelineNetflixTop10Week) {
		global, countries, err := s.analytics.NetflixForTitle(ctx, ref.Kind, ref.ID)
		if err != nil {
			return nil, err
		}
		add(netflixWeekEvents(global, countries, country))
	}
	if s.engagement != nil && want(TimelineNetflixEngagement) {
		rows, err := s.engagement.ForTmdb(ctx, ref)
		if err != nil {
			return nil, err
		}
		add(engagementEvents(rows))
	}

	kept := events[:0]
	for _, e := range events {
		if types != nil && !types[e.Type] {
			continue
		}
		end := e.Date
		if e.EndDate != nil {
			end = *e.EndDate
		}
		if (from != "" && end < from) || (to != "" && e.Date > to) {
			continue
		}
		kept = append(kept, e)
		out.Counts[e.Type]++
	}
	sortTimeline(kept)
	out.Total = len(kept)
	if len(kept) > limit {
		kept = kept[:limit]
	}
	out.Events = append(out.Events, kept...)
	return out, nil
}

// sortTimeline orders events newest first; equal dates by type order, then
// by country / provider for a stable output.
func sortTimeline(events []TimelineEvent) {
	order := map[string]int{}
	for i, t := range TimelineEventTypes {
		order[t] = i
	}
	key := func(e TimelineEvent) string {
		switch {
		case e.Chart != nil:
			return e.Chart.Country + "|" + e.Chart.Provider + "|" + e.Chart.Category
		case e.Availability != nil:
			return e.Availability.ProviderName + "|" + e.Availability.Monetization
		case e.Rating != nil:
			return e.Rating.Source
		case e.Netflix != nil:
			return e.Netflix.Category + "|" + derefStr(e.Netflix.SeasonTitle)
		case e.Engagement != nil:
			return e.Engagement.Title
		}
		return ""
	}
	sort.SliceStable(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if a.Date != b.Date {
			return a.Date > b.Date
		}
		if a.Type != b.Type {
			return order[a.Type] < order[b.Type]
		}
		return key(a) < key(b)
	})
}

// --- charts ---------------------------------------------------------------------

var categoryLabels = map[string]string{"movies": "movies", "tv_shows": "TV shows"}

func chartLabel(country, provider, category string) string {
	label := categoryLabels[category]
	if label == "" {
		label = category
	}
	return fmt.Sprintf("%s %s (%s)", provider, label, country)
}

func daysBetween(a, b string) int64 {
	ta, err1 := time.Parse(repository.DateLayout, a)
	tb, err2 := time.Parse(repository.DateLayout, b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int64(math.Round(tb.Sub(ta).Hours() / 24))
}

// dateRange is an inclusive YYYY-MM-DD range; empty bounds are open.
type dateRange struct{ from, to string }

func (r dateRange) contains(d string) bool {
	return (r.from == "" || d >= r.from) && (r.to == "" || d <= r.to)
}

// chartEvents finds per chart (country × provider × category): the debut,
// the first day at the best rank (when after the debut), every run at #1
// and every return after at least a week away. Re-entries are looked for
// only when reentries is set, and only those dated within it.
func (s *TimelineService) chartEvents(ctx context.Context, rankings []RankingSnapshot, reentries *dateRange) ([]TimelineEvent, error) {
	type chartKey struct{ country, provider, category string }
	byChart := map[chartKey][]RankingSnapshot{}
	var keys []chartKey
	for _, r := range rankings {
		k := chartKey{r.Country, r.Provider, r.Category}
		list := byChart[k]
		if list == nil {
			keys = append(keys, k)
		}
		// Several FlixPatrol titles can map to one TMDB title: keep the
		// best rank per date.
		if n := len(list); n > 0 && list[n-1].Date == r.Date {
			if r.Rank < list[n-1].Rank {
				list[n-1] = r
			}
			continue
		}
		byChart[k] = append(list, r)
	}
	var out []TimelineEvent
	for _, k := range keys {
		list := byChart[k]
		sort.SliceStable(list, func(i, j int) bool { return list[i].Date < list[j].Date })
		label := chartLabel(k.country, k.provider, k.category)
		detail := func(r RankingSnapshot, days *int64) *TimelineChart {
			return &TimelineChart{Country: k.country, Provider: k.provider, Category: k.category, Rank: r.Rank, SeasonNumber: r.SeasonNumber, Days: days}
		}
		first := list[0]
		out = append(out, TimelineEvent{Type: TimelineChartDebut, Date: first.Date, Chart: detail(first, nil),
			Summary: fmt.Sprintf("Debuted at #%d on %s", first.Rank, label)})

		best := first
		for _, r := range list[1:] {
			if r.Rank < best.Rank {
				best = r
			}
		}
		if best.Date != first.Date {
			d := daysBetween(first.Date, best.Date)
			out = append(out, TimelineEvent{Type: TimelineChartPeak, Date: best.Date, Chart: detail(best, &d),
				Summary: fmt.Sprintf("Peaked at #%d on %s, %d days after its debut", best.Rank, label, d)})
		}

		for i := 0; i < len(list); i++ {
			if list[i].Rank != 1 {
				continue
			}
			j := i
			for j+1 < len(list) && list[j+1].Rank == 1 && daysBetween(list[j].Date, list[j+1].Date) <= 2 {
				j++
			}
			days := int64(j - i + 1)
			ev := TimelineEvent{Type: TimelineChartNumberOne, Date: list[i].Date, Chart: detail(list[i], &days),
				Summary: fmt.Sprintf("#1 on %s for %d %s", label, days, plural(days, "day", "days"))}
			if j > i {
				end := list[j].Date
				ev.EndDate = &end
			}
			out = append(out, ev)
			i = j
		}

		for i := 1; reentries != nil && i < len(list); i++ {
			gap := daysBetween(list[i-1].Date, list[i].Date)
			if gap < reentryMinGapDays || !reentries.contains(list[i].Date) {
				continue
			}
			n, err := s.analytics.ChartSnapshotsBetween(ctx, k.country, k.provider, k.category, list[i-1].Date, list[i].Date)
			if err != nil {
				return nil, err
			}
			if n == 0 {
				continue // the chart was not stored in between: no evidence it left
			}
			away := gap - 1
			out = append(out, TimelineEvent{Type: TimelineChartReentry, Date: list[i].Date, Chart: detail(list[i], &away),
				Summary: fmt.Sprintf("Re-entered %s at #%d after %d days away", label, list[i].Rank, away)})
		}
	}
	return out, nil
}

func plural(n int64, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// --- TMDB dates -----------------------------------------------------------------

func metadataEvents(ref TmdbRef, meta *repository.TmdbMetadata) []TimelineEvent {
	var out []TimelineEvent
	if d := derefStr(meta.ReleaseDate); validDate(d) {
		summary := "Released"
		if ref.Kind == "tv" {
			summary = "First aired"
		}
		out = append(out, TimelineEvent{Type: TimelineRelease, Date: d, Summary: summary})
	}
	if d := derefStr(meta.NetflixPremiere); validDate(d) {
		out = append(out, TimelineEvent{Type: TimelineNetflixPremiere, Date: d, Summary: "Premiered on Netflix as a Netflix original"})
	}
	for _, se := range meta.Seasons {
		if se.Number < 1 || !validDate(se.AirDate) {
			continue // specials (season 0) or no date yet
		}
		name := se.Name
		if name == "" {
			name = fmt.Sprintf("Season %d", se.Number)
		}
		summary := fmt.Sprintf("%s premiered", name)
		if se.EpisodeCount > 0 {
			summary += fmt.Sprintf(" (%d %s)", se.EpisodeCount, plural(int64(se.EpisodeCount), "episode", "episodes"))
		}
		out = append(out, TimelineEvent{Type: TimelineSeasonPremiere, Date: se.AirDate, Summary: summary,
			Season: &TimelineSeason{SeasonNumber: int64(se.Number), Name: name, EpisodeCount: int64(se.EpisodeCount)}})
	}
	return out
}

func validDate(s string) bool {
	_, err := time.Parse(repository.DateLayout, s)
	return err == nil
}

// --- availability ---------------------------------------------------------------

var monetizationLabels = map[string]string{"flatrate": "subscription", "free": "free", "ads": "with ads", "rent": "rent", "buy": "buy"}

// availabilityEvents groups offer changes by day, event, provider and offer
// type, listing the countries.
func availabilityEvents(offers []repository.OfferEvent) []TimelineEvent {
	type key struct {
		date, event, monetization string
		provider                  int64
	}
	byKey := map[key]*TimelineEvent{}
	var keys []key
	for _, o := range offers {
		date := o.ObservedAt
		if len(date) >= 10 {
			date = date[:10]
		}
		k := key{date, o.Event, o.Monetization, o.ProviderID}
		ev := byKey[k]
		if ev == nil {
			typ := TimelineAvailabilityAdded
			if o.Event == repository.OfferRemoved {
				typ = TimelineAvailabilityRemoved
			}
			ev = &TimelineEvent{Type: typ, Date: date, Availability: &TimelineAvailability{ProviderID: o.ProviderID,
				ProviderName: o.ProviderName, Monetization: o.Monetization, Countries: []string{}}}
			byKey[k] = ev
			keys = append(keys, k)
		}
		if !containsStr(ev.Availability.Countries, o.Country) {
			ev.Availability.Countries = append(ev.Availability.Countries, o.Country)
		}
	}
	out := make([]TimelineEvent, 0, len(keys))
	for _, k := range keys {
		ev := byKey[k]
		a := ev.Availability
		sort.Strings(a.Countries)
		verb := "Added to"
		if ev.Type == TimelineAvailabilityRemoved {
			verb = "Removed from"
		}
		where := a.Countries[0]
		if len(a.Countries) > 1 {
			where = fmt.Sprintf("%d countries", len(a.Countries))
		}
		label := monetizationLabels[a.Monetization]
		if label == "" {
			label = a.Monetization
		}
		ev.Summary = fmt.Sprintf("%s %s (%s) in %s", verb, a.ProviderName, label, where)
		out = append(out, *ev)
	}
	return out
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// --- ratings --------------------------------------------------------------------

// Provider preference when several providers report the same source.
var ratingProviderRank = map[string]int{"imdb_dataset": 0, ProviderRottenTomatoes: 1, ProviderMDBList: 2, ProviderJustWatch: 3}

// Sources that are not ratings.
var nonRatingSources = map[string]bool{"tmdb_popularity": true}

var ratingSourceLabels = map[string]string{
	"imdb": "IMDb rating", "tomatoes": "Rotten Tomatoes critics score", "popcorn": "Rotten Tomatoes audience score",
	"tomatoes_certified": "Certified Fresh", "metacritic": "Metacritic score", "metacriticuser": "Metacritic user score",
	"letterboxd": "Letterboxd rating", "tmdb": "TMDB rating", "trakt": "Trakt rating", "justwatch": "JustWatch rating",
	"mdblist": "MDBList score", "mdblist_average": "MDBList average",
}

// ratingEvents emits a rating_changed whenever a source's displayed value
// (rounded to its scale) changed. Per source, one provider's series is used:
// the IMDb dataset for IMDb, otherwise Rotten Tomatoes > MDBList > JustWatch.
func (s *TimelineService) ratingEvents(ctx context.Context, ref TmdbRef, meta *repository.TmdbMetadata) ([]TimelineEvent, error) {
	const to = "9999-12-31"
	series, err := s.history.RatingHistory(ctx, ref, to)
	if err != nil {
		return nil, err
	}
	if meta != nil && meta.IMDbID != nil && *meta.IMDbID != "" {
		points, err := s.history.IMDbHistory(ctx, *meta.IMDbID, to)
		if err != nil {
			return nil, err
		}
		if len(points) > 0 {
			series = append(series, repository.RatingSeries{Provider: "imdb_dataset", Source: "imdb", Points: points})
		}
	}
	chosen := map[string]repository.RatingSeries{}
	for _, sr := range series {
		if nonRatingSources[sr.Source] || len(sr.Points) == 0 {
			continue
		}
		cur, ok := chosen[sr.Source]
		if !ok || providerRank(sr.Provider) < providerRank(cur.Provider) {
			chosen[sr.Source] = sr
		}
	}
	sources := make([]string, 0, len(chosen))
	for src := range chosen {
		sources = append(sources, src)
	}
	sort.Strings(sources)
	var out []TimelineEvent
	for _, src := range sources {
		out = append(out, ratingChanges(chosen[src])...)
	}
	return out, nil
}

func providerRank(p string) int {
	if r, ok := ratingProviderRank[p]; ok {
		return r
	}
	return len(ratingProviderRank)
}

// ratingChanges returns the points whose displayed value differs from the
// previous displayed value (the first point is the baseline).
func ratingChanges(sr repository.RatingSeries) []TimelineEvent {
	scale := 0.0
	for _, p := range sr.Points {
		scale = math.Max(scale, math.Abs(p.Value))
	}
	decimals := 0
	switch {
	case scale <= 1:
		decimals = 2
	case scale <= 10:
		decimals = 1
	}
	round := func(v float64) float64 {
		f := math.Pow(10, float64(decimals))
		return math.Round(v*f) / f
	}
	label := ratingSourceLabels[sr.Source]
	if label == "" {
		label = sr.Source
	}
	var out []TimelineEvent
	prev := round(sr.Points[0].Value)
	for _, p := range sr.Points[1:] {
		v := round(p.Value)
		if v == prev {
			continue
		}
		summary := fmt.Sprintf("%s %s → %s", label, strconv.FormatFloat(prev, 'f', decimals, 64), strconv.FormatFloat(v, 'f', decimals, 64))
		if sr.Source == "tomatoes_certified" {
			summary = "Became Certified Fresh"
			if v == 0 {
				summary = "Lost Certified Fresh status"
			}
		}
		out = append(out, TimelineEvent{Type: TimelineRatingChanged, Date: p.Date, Summary: summary,
			Rating: &TimelineRating{Provider: sr.Provider, Source: sr.Source, Value: p.Value, PreviousValue: prev, Votes: p.Votes}})
		prev = v
	}
	return out
}

// --- Wikipedia pageviews --------------------------------------------------------

// pageviewSpikes finds runs of days with unusually many views: at least
// spikeMinViews and spikeRatio × the median of the previous 28 days. A run's
// baseline is the median before its first day.
func pageviewSpikes(points []repository.PageviewPoint) []TimelineEvent {
	var out []TimelineEvent
	var cur *TimelineEvent
	var lastDay string
	for i, p := range points {
		dayT, err := time.Parse(repository.DateLayout, p.Date)
		if err != nil {
			continue
		}
		windowStart := repository.FormatDate(dayT.AddDate(0, 0, -spikeWindowDays))
		var window []int64
		for j := i - 1; j >= 0 && points[j].Date >= windowStart; j-- {
			window = append(window, points[j].Views)
		}
		if cur != nil && daysBetween(lastDay, p.Date) == 1 && p.Views >= spikeMinViews &&
			float64(p.Views) >= spikeRatio*float64(cur.Pageviews.BaselineViews) {
			pv := cur.Pageviews
			pv.Days++
			if p.Views > pv.PeakViews {
				pv.PeakViews, pv.PeakDay = p.Views, p.Date
			}
			end := p.Date
			cur.EndDate = &end
			lastDay = p.Date
			continue
		}
		if cur != nil {
			out = append(out, finishSpike(*cur))
			cur = nil
		}
		if len(window) < spikeMinBaselineDays || p.Views < spikeMinViews {
			continue
		}
		base := max(median(window), 1)
		if float64(p.Views) < spikeRatio*float64(base) {
			continue
		}
		cur = &TimelineEvent{Type: TimelinePageviewSpike, Date: p.Date,
			Pageviews: &TimelinePageviews{PeakDay: p.Date, PeakViews: p.Views, BaselineViews: base, Days: 1}}
		lastDay = p.Date
	}
	if cur != nil {
		out = append(out, finishSpike(*cur))
	}
	return out
}

func finishSpike(e TimelineEvent) TimelineEvent {
	pv := e.Pageviews
	pv.Ratio = math.Round(float64(pv.PeakViews)/float64(pv.BaselineViews)*10) / 10
	e.Summary = fmt.Sprintf("Wikipedia pageviews spiked to %s on %s (%.1f× the usual %s a day)",
		groupDigits(pv.PeakViews), pv.PeakDay, pv.Ratio, groupDigits(pv.BaselineViews))
	return e
}

func median(v []int64) int64 {
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// groupDigits formats 1234567 as "1,234,567".
func groupDigits(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// --- Netflix --------------------------------------------------------------------

// netflixWeekEvents emits one event per week and season title the title
// was in Netflix's global Top 10 or in a country's Top 10, with the number of
// countries. With a country, only weeks it charted there are kept.
func netflixWeekEvents(global, countries []repository.NetflixWeekRow, country string) []TimelineEvent {
	type key struct{ week, season string }
	byKey := map[key]*TimelineNetflixWeek{}
	var keys []key
	get := func(r repository.NetflixWeekRow) *TimelineNetflixWeek {
		k := key{r.Week, derefStr(r.SeasonTitle)}
		w := byKey[k]
		if w == nil {
			w = &TimelineNetflixWeek{Week: r.Week, Category: r.Category, SeasonTitle: r.SeasonTitle, SeasonNumber: r.SeasonNumber}
			byKey[k] = w
			keys = append(keys, k)
		}
		return w
	}
	for _, r := range global {
		w := get(r)
		rank := r.Rank
		if w.GlobalRank == nil || rank < *w.GlobalRank {
			w.GlobalRank, w.Category = &rank, r.Category
		}
		w.HoursViewed, w.Views, w.CumulativeWeeks = r.HoursViewed, r.Views, r.CumulativeWeeks
	}
	for _, r := range countries {
		w := get(r)
		w.Countries++
		if country != "" && strings.EqualFold(r.Country, country) {
			rank := r.Rank
			w.CountryRank = &rank
		}
		if w.GlobalRank == nil && w.CumulativeWeeks == nil {
			w.CumulativeWeeks = r.CumulativeWeeks
		}
	}
	var out []TimelineEvent
	for _, k := range keys {
		w := byKey[k]
		if country != "" && w.CountryRank == nil {
			continue
		}
		end, err := time.Parse(repository.DateLayout, w.Week)
		if err != nil {
			continue
		}
		start := repository.FormatDate(end.AddDate(0, 0, -6))
		week := w.Week
		var b strings.Builder
		if w.GlobalRank != nil {
			fmt.Fprintf(&b, "#%d in Netflix's global Top 10, %s", *w.GlobalRank, w.Category)
		} else {
			b.WriteString("In Netflix's country Top 10s")
		}
		if w.SeasonTitle != nil && *w.SeasonTitle != "" {
			fmt.Fprintf(&b, " with %s", *w.SeasonTitle)
		}
		if w.Views != nil {
			fmt.Fprintf(&b, ", %s views", groupDigits(*w.Views))
		}
		if w.Countries > 0 {
			fmt.Fprintf(&b, ", in %d %s", w.Countries, plural(int64(w.Countries), "country's Top 10", "countries' Top 10s"))
		}
		if w.CountryRank != nil {
			fmt.Fprintf(&b, " (#%d in %s)", *w.CountryRank, strings.ToUpper(country))
		}
		out = append(out, TimelineEvent{Type: TimelineNetflixTop10Week, Date: start, EndDate: &week, Summary: b.String(), Netflix: w})
	}
	return out
}

func engagementEvents(rows []repository.EngagementRow) []TimelineEvent {
	out := make([]TimelineEvent, 0, len(rows))
	for _, r := range rows {
		if !validDate(r.PeriodStart) || !validDate(r.PeriodEnd) {
			continue
		}
		end := r.PeriodEnd
		var parts []string
		if r.HoursViewed != nil {
			parts = append(parts, groupDigits(*r.HoursViewed)+" hours viewed")
		}
		if r.Views != nil {
			parts = append(parts, groupDigits(*r.Views)+" views")
		}
		summary := fmt.Sprintf("Netflix engagement report %s to %s: %s", r.PeriodStart, r.PeriodEnd, r.Title)
		if len(parts) > 0 {
			summary += ", " + strings.Join(parts, ", ")
		}
		out = append(out, TimelineEvent{Type: TimelineNetflixEngagement, Date: r.PeriodStart, EndDate: &end, Summary: summary,
			Engagement: &TimelineEngagement{PeriodStart: r.PeriodStart, PeriodEnd: r.PeriodEnd, Title: r.Title, HoursViewed: r.HoursViewed,
				Views: r.Views, RuntimeMinutes: r.RuntimeMinutes, SeasonNumber: r.SeasonNumber, AvailableGlobally: r.AvailableGlobally}})
	}
	return out
}
