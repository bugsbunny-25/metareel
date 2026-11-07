package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Release patterns of a TV season, from its episodes' air dates.
const (
	PatternBinge     = "binge"     // every episode on one day
	PatternBatch     = "batch"     // 2 to 4 drops of 2+ episodes on average (e.g. "Volume 1" / "Volume 2")
	PatternWeekly    = "weekly"    // drops about a week apart (5 to 9 days, median)
	PatternDaily     = "daily"     // drops at most 2 days apart (median)
	PatternIrregular = "irregular" // anything else
)

// ReleasePatterns lists the patterns in display order.
var ReleasePatterns = []string{PatternBinge, PatternBatch, PatternWeekly, PatternDaily, PatternIrregular}

const (
	defaultReleaseWindow = 60
	minReleaseWindow     = 30
	maxReleaseWindow     = 180
	// lateDay: a chart run "stayed" when the season was still charting on or
	// after this many days from its premiere.
	lateDay = 28
)

// WithEpisodes adds TMDB episodes for the release-pattern analytic.
func (s *AnalyticsService) WithEpisodes(repo *repository.EpisodesRepository) *AnalyticsService {
	s.episodes = repo
	return s
}

// classifyRelease returns a season's release pattern and its number of
// release dates (drops); ok is false with fewer than 2 dated episodes.
func classifyRelease(airDates []string) (pattern string, drops int, ok bool) {
	perDate := map[string]int{}
	for _, d := range airDates {
		if d != "" {
			perDate[d]++
		}
	}
	if len(airDates) < 2 || len(perDate) == 0 {
		return "", 0, false
	}
	dates := make([]string, 0, len(perDate))
	for d := range perDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	drops = len(dates)
	switch {
	case drops == 1:
		return PatternBinge, drops, true
	case drops <= 4 && float64(len(airDates))/float64(drops) >= 2:
		return PatternBatch, drops, true
	}
	gaps := make([]int64, 0, drops-1)
	for i := 1; i < drops; i++ {
		gaps = append(gaps, daysBetween(dates[i-1], dates[i]))
	}
	switch g := median(gaps); {
	case g >= 5 && g <= 9:
		return PatternWeekly, drops, true
	case g <= 2:
		return PatternDaily, drops, true
	}
	return PatternIrregular, drops, true
}

// ReleasePatternsResponse compares how long TV seasons stay in the Top 10
// by release pattern.
type ReleasePatternsResponse struct {
	AnalyticsEcho
	WindowDays int `json:"window_days"`
	// Patterns: one row per pattern with seasons, in display order.
	Patterns []ReleasePatternStats `json:"patterns"`
	// Seasons: every season counted, newest premiere first.
	Seasons []ReleaseSeason `json:"seasons"`
	// Excluded: seasons premiering in the range that were not counted.
	Excluded ReleaseExcluded `json:"excluded"`
}

// ReleasePatternStats aggregates the chart runs (season x chart) of one
// release pattern within the window after each premiere.
type ReleasePatternStats struct {
	Pattern           string  `json:"pattern"`
	Seasons           int     `json:"seasons"`
	ChartRuns         int     `json:"chart_runs"`
	MedianDaysInTop10 float64 `json:"median_days_in_top10"`
	MeanDaysInTop10   float64 `json:"mean_days_in_top10"`
	MedianLastDay     float64 `json:"median_last_day"` // days from premiere to the last day in the Top 10
	ShareStayed       float64 `json:"share_still_charting_day_28"`
	MedianBestRank    float64 `json:"median_best_rank"`
	MedianPoints      float64 `json:"median_points"`
}

// ReleaseSeason is one counted season.
type ReleaseSeason struct {
	Kind         string `json:"kind"` // tv
	TmdbID       string `json:"tmdb_id"`
	Title        string `json:"title"`
	SeasonNumber int64  `json:"season_number"`
	Pattern      string `json:"pattern"`
	Premiere     string `json:"premiere"`
	LastEpisode  string `json:"last_episode"`
	Episodes     int    `json:"episodes"`
	Drops        int    `json:"drops"`  // release dates
	Charts       int    `json:"charts"` // charts it entered within the window
	// Per chart run, within the window: median days in the Top 10 and of the
	// last day charting (days after the premiere); best rank on any chart.
	MedianDaysInTop10 float64 `json:"median_days_in_top10"`
	MedianLastDay     float64 `json:"median_last_day"`
	BestRank          int64   `json:"best_rank"`
}

// ReleaseExcluded counts seasons premiering in the range that were left out.
type ReleaseExcluded struct {
	Unaired    int `json:"unaired"`     // episodes without an air date, or later than today
	Single     int `json:"single"`      // fewer than 2 episodes
	InProgress int `json:"in_progress"` // charted, but its window is not over on any chart yet
	Uncharted  int `json:"uncharted"`   // on no stored chart (complete for the window) within the window
}

// ReleasePatterns classifies TV seasons premiering between from and to
// (default: the last 365 days) by release pattern and measures their chart
// runs over the window (days) after the premiere. A chart counts only when
// it was stored from the premiere to the end of the window; the window stops
// before the next season's premiere. Rankings count for the season when
// tagged with its number, or untagged within the window.
func (s *AnalyticsService) ReleasePatterns(ctx context.Context, q AnalyticsQuery) (*ReleasePatternsResponse, error) {
	if s.episodes == nil {
		return nil, fmt.Errorf("release patterns: no episodes repository")
	}
	window := defaultReleaseWindow
	if raw := strings.TrimSpace(q.Window); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < minReleaseWindow || n > maxReleaseWindow {
			return nil, &ValidationError{Message: fmt.Sprintf("invalid window %q, expected %d to %d days", q.Window, minReleaseWindow, maxReleaseWindow)}
		}
		window = n
	}
	q.Category, q.Kind = "", ""
	f, err := s.filter(q, 365)
	if err != nil {
		return nil, err
	}
	f.category, f.kind = "tv_shows", "tv_show"
	fromS, toS := repository.FormatDate(f.from), repository.FormatDate(f.to)
	today := repository.FormatDate(s.now().UTC())

	eps, err := s.episodes.SeasonEpisodes(ctx, fromS, toS)
	if err != nil {
		return nil, err
	}
	premieres, err := s.episodes.SeasonPremieres(ctx, fromS)
	if err != nil {
		return nil, err
	}
	catalog, err := s.repo.ChartCatalog(ctx)
	if err != nil {
		return nil, err
	}
	type chartSpan struct{ first, last string }
	charts := map[string]chartSpan{}
	for _, c := range catalog {
		if c.Category != "tv_shows" || (f.country != "" && c.Country != f.country) || (f.provider != "" && c.Provider != f.provider) {
			continue
		}
		charts[c.Country+"|"+c.Provider+"|"+c.Category] = chartSpan{c.FirstDate, c.LatestDate}
	}
	rows, err := s.repo.ChartEntries(ctx, repository.ChartEntryFilter{
		From: f.from, To: f.to.AddDate(0, 0, window), Country: f.country, Provider: f.provider, Category: "tv_shows",
	})
	if err != nil {
		return nil, err
	}
	// Rankings per show: chart -> date -> best rank, with the season tag.
	type ranked struct {
		date   string
		rank   int64
		season *int64
	}
	byShow := map[string]map[string][]ranked{}
	for _, r := range rows {
		if r.Kind != "tv_show" || r.TmdbID == nil || *r.TmdbID == "" {
			continue
		}
		chart := r.Country + "|" + r.Provider + "|" + r.Category
		if byShow[*r.TmdbID] == nil {
			byShow[*r.TmdbID] = map[string][]ranked{}
		}
		byShow[*r.TmdbID][chart] = append(byShow[*r.TmdbID][chart], ranked{r.Date, r.Rank, r.SeasonNumber})
	}

	out := &ReleasePatternsResponse{AnalyticsEcho: f.echo(), WindowDays: window, Patterns: []ReleasePatternStats{}, Seasons: []ReleaseSeason{}}
	out.Category, out.Kind = nil, nil
	type run struct {
		days, lastDay, best, points int64
	}
	runsByPattern := map[string][]run{}
	seasonsByPattern := map[string]int{}

	for i := 0; i < len(eps); {
		j := i
		for j < len(eps) && eps[j].TmdbID == eps[i].TmdbID && eps[j].SeasonNumber == eps[i].SeasonNumber {
			j++
		}
		season := eps[i:j]
		i = j
		var dates []string
		unaired := false
		for _, e := range season {
			if e.AirDate == "" || e.AirDate > today {
				unaired = true
			}
			dates = append(dates, e.AirDate)
		}
		if unaired {
			out.Excluded.Unaired++
			continue
		}
		pattern, drops, ok := classifyRelease(dates)
		if !ok {
			out.Excluded.Single++
			continue
		}
		sort.Strings(dates)
		premiere, lastEp := dates[0], dates[len(dates)-1]
		show, number := season[0].TmdbID, season[0].SeasonNumber
		end := repository.FormatDate(parseDay(premiere).AddDate(0, 0, window-1))
		if next, ok := premieres[show][number+1]; ok && next > premiere && next <= end {
			end = repository.FormatDate(parseDay(next).AddDate(0, 0, -1))
		}
		var runs []run
		var best int64
		pending := false
		for chart, list := range byShow[show] {
			span, ok := charts[chart]
			if !ok || span.first > premiere {
				continue // chart not stored from the premiere on
			}
			if span.last < end {
				pending = true // window not over yet on this chart
				continue
			}
			dayRank := map[string]int64{}
			for _, r := range list {
				if r.date < premiere || r.date > end || (r.season != nil && *r.season != number) {
					continue
				}
				if cur, ok := dayRank[r.date]; !ok || r.rank < cur {
					dayRank[r.date] = r.rank
				}
			}
			if len(dayRank) == 0 {
				continue
			}
			rn := run{days: int64(len(dayRank)), best: 11}
			for d, rank := range dayRank {
				rn.lastDay = max(rn.lastDay, daysBetween(premiere, d))
				rn.best = min(rn.best, rank)
				rn.points += Points(rank)
			}
			runs = append(runs, rn)
			if best == 0 || rn.best < best {
				best = rn.best
			}
		}
		if len(runs) == 0 {
			if pending {
				out.Excluded.InProgress++
			} else {
				out.Excluded.Uncharted++
			}
			continue
		}
		days, last := make([]int64, 0, len(runs)), make([]int64, 0, len(runs))
		for _, rn := range runs {
			days, last = append(days, rn.days), append(last, rn.lastDay)
		}
		out.Seasons = append(out.Seasons, ReleaseSeason{Kind: "tv", TmdbID: show, Title: season[0].Title, SeasonNumber: number,
			Pattern: pattern, Premiere: premiere, LastEpisode: lastEp, Episodes: len(season), Drops: drops, Charts: len(runs),
			MedianDaysInTop10: medianF(days), MedianLastDay: medianF(last), BestRank: best})
		runsByPattern[pattern] = append(runsByPattern[pattern], runs...)
		seasonsByPattern[pattern]++
	}

	for _, p := range ReleasePatterns {
		runs := runsByPattern[p]
		if len(runs) == 0 {
			continue
		}
		var days, last, best, points []int64
		var sum, stayed int64
		for _, rn := range runs {
			days, last, best, points = append(days, rn.days), append(last, rn.lastDay), append(best, rn.best), append(points, rn.points)
			sum += rn.days
			if rn.lastDay >= lateDay {
				stayed++
			}
		}
		out.Patterns = append(out.Patterns, ReleasePatternStats{Pattern: p, Seasons: seasonsByPattern[p], ChartRuns: len(runs),
			MedianDaysInTop10: medianF(days), MeanDaysInTop10: round(float64(sum)/float64(len(runs)), 2),
			MedianLastDay: medianF(last), ShareStayed: round(float64(stayed)/float64(len(runs)), 3),
			MedianBestRank: medianF(best), MedianPoints: medianF(points)})
	}
	sort.SliceStable(out.Seasons, func(i, j int) bool {
		a, b := out.Seasons[i], out.Seasons[j]
		if a.Premiere != b.Premiere {
			return a.Premiere > b.Premiere
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.SeasonNumber < b.SeasonNumber
	})
	return out, nil
}

// medianF is the median of v as a float (the mean of the middle two when
// even); 0 for none.
func medianF(v []int64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	n := len(s)
	if n%2 == 1 {
		return float64(s[n/2])
	}
	return float64(s[n/2-1]+s[n/2]) / 2
}
