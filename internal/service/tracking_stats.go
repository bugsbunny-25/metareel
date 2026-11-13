package service

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

const (
	maxTopPeople   = 25
	maxInsightRows = 100
	// Crew jobs worth ranking separately from the cast.
	jobDirector = "Director"
	jobWriter   = "Screenplay"
)

// Summary is the headline view: what a user watched over a range, where, in
// what formats and at what cost.
func (s *TrackingService) Summary(ctx context.Context, userID int64, from, to string) (map[string]any, error) {
	rng, err := s.statsRange(from, to)
	if err != nil {
		return nil, err
	}
	totals, err := s.read.StatsTotals(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	venues, err := s.read.StatsByVenueKind(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	formats, err := s.read.StatsByFormat(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	spend, err := s.read.StatsSpendByCurrency(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"range":    rangePayload(rng),
		"totals":   totalsPayload(totals),
		"by_venue": venues,
		"formats":  formats,
		"spend":    spend,
	}, nil
}

// Calendar is the day-by-day heatmap and the streaks that come out of it.
func (s *TrackingService) Calendar(ctx context.Context, userID int64, from, to string) (map[string]any, error) {
	rng, err := s.statsRange(from, to)
	if err != nil {
		return nil, err
	}
	days, err := s.read.StatsPerDay(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	longest, longestEnd := longestStreak(days)
	return map[string]any{
		"range": rangePayload(rng),
		"days":  days,
		"streaks": map[string]any{
			"longest":       longest,
			"longest_ended": longestEnd,
			"current":       currentStreak(days, s.today()),
			"days_watched":  len(days),
		},
	}, nil
}

// Spend totals money per currency, per cinema and per format. Amounts are
// never converted between currencies: there is no rate to do it honestly.
func (s *TrackingService) Spend(ctx context.Context, userID int64, from, to string) (map[string]any, error) {
	rng, err := s.statsRange(from, to)
	if err != nil {
		return nil, err
	}
	byCurrency, err := s.read.StatsSpendByCurrency(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	byVenue, err := s.read.StatsSpendByVenue(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	formats, err := s.read.StatsByFormat(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"range": rangePayload(rng), "by_currency": byCurrency,
		"by_venue": byVenue, "by_format": formats,
	}, nil
}

// Ratings is how a user rates, and how that compares with the crowd.
func (s *TrackingService) Ratings(ctx context.Context, userID int64, from, to string) (map[string]any, error) {
	rng, err := s.statsRange(from, to)
	if err != nil {
		return nil, err
	}
	dist, err := s.read.StatsRatingDistribution(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	rows, err := s.read.StatsRatingComparison(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Ratings are 1-10 half-stars, which is already a 0-10 scale, so the
	// crowd's scores compare directly without rescaling.
	buckets := make([]map[string]any, 0, 10)
	var rated, weighted int64
	for r := int64(1); r <= 10; r++ {
		n := dist[r]
		buckets = append(buckets, map[string]any{
			"rating": r, "stars": float64(r) / 2, "watches": n})
		rated += n
		weighted += n * r
	}
	out := map[string]any{"range": rangePayload(rng), "distribution": buckets, "rated": rated}
	if rated > 0 {
		out["average"] = round2(float64(weighted) / float64(rated))
	}

	out["vs_crowd"] = crowdComparison(rows)
	out["titles"] = rows
	return out, nil
}

// YearInReview is the look back over one year.
func (s *TrackingService) YearInReview(ctx context.Context, userID int64, year int) (map[string]any, error) {
	if year < 1900 || year > 2200 {
		return nil, &ValidationError{Message: "year must be a four-digit year"}
	}
	from := strconv.Itoa(year) + "-01-01"
	to := strconv.Itoa(year) + "-12-31"
	rng := repository.StatsRange{From: from, To: to}

	totals, err := s.read.StatsTotals(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	days, err := s.read.StatsPerDay(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	months, err := s.read.StatsByMonth(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	weekdays, err := s.read.StatsByWeekday(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	hours, err := s.read.StatsByHour(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	venues, err := s.read.StatsByVenueKind(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	formats, err := s.read.StatsByFormat(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	spend, err := s.read.StatsSpendByCurrency(ctx, userID, rng)
	if err != nil {
		return nil, err
	}
	titles, err := s.read.StatsWatchedTitles(ctx, userID, rng)
	if err != nil {
		return nil, err
	}

	people := map[string]any{}
	for key, job := range map[string]string{"cast": "", "directors": jobDirector, "writers": jobWriter} {
		rows, err := s.read.StatsTopPeople(ctx, userID, rng, job, 10)
		if err != nil {
			return nil, err
		}
		people[key] = rows
	}

	longest, longestEnd := longestStreak(days)
	genres, countries, decades, languages := tasteBreakdown(titles)
	top := titles
	if len(top) > 10 {
		top = top[:10]
	}

	return map[string]any{
		"year":         year,
		"range":        rangePayload(rng),
		"totals":       totalsPayload(totals),
		"by_month":     months,
		"by_weekday":   weekdayPayload(weekdays),
		"by_hour":      hours,
		"by_venue":     venues,
		"formats":      formats,
		"spend":        spend,
		"genres":       genres,
		"countries":    countries,
		"decades":      decades,
		"languages":    languages,
		"people":       people,
		"most_watched": top,
		"streaks": map[string]any{
			"longest": longest, "longest_ended": longestEnd, "days_watched": len(days),
		},
	}, nil
}

// WatchlistInsights is what a user's watchlist is doing in metareel's own
// data: what has charted, what they can watch now, and what has gone.
//
// "Leaving soon" is not among them: metareel records availability changes it
// has observed, not departures a service has scheduled, so the honest
// version of that question is what has already left.
func (s *TrackingService) WatchlistInsights(ctx context.Context, userID int64, country string, days int64) (map[string]any, error) {
	if country == "" {
		return nil, &ValidationError{Message: "country is required, e.g. US or IN"}
	}
	if !countryRe.MatchString(country) {
		return nil, &ValidationError{Message: "country must be a 2-letter ISO 3166-1 code"}
	}
	country = strings.ToUpper(country)
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	since := s.now().UTC().AddDate(0, 0, -int(days))

	charting, err := s.read.WatchlistInCharts(ctx, userID, since.Format("2006-01-02"), country, maxInsightRows)
	if err != nil {
		return nil, err
	}
	streaming, err := s.read.WatchlistStreamingNow(ctx, userID, country, "flatrate", maxInsightRows)
	if err != nil {
		return nil, err
	}
	left, err := s.read.WatchlistRecentlyLeft(ctx, userID, country,
		repository.FormatTimestamp(since), maxInsightRows)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"country": country, "days": days,
		"charting": charting, "streaming_now": streaming, "recently_left": left,
	}, nil
}

// TopPeople ranks the people behind what a user watched.
func (s *TrackingService) TopPeople(ctx context.Context, userID int64, from, to, job string, limit int64) (map[string]any, error) {
	rng, err := s.statsRange(from, to)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > maxTopPeople {
		limit = maxTopPeople
	}
	rows, err := s.read.StatsTopPeople(ctx, userID, rng, job, limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{"range": rangePayload(rng), "job": job, "people": rows}, nil
}

// statsRange validates an optional local-day range.
func (s *TrackingService) statsRange(from, to string) (repository.StatsRange, error) {
	for name, v := range map[string]string{"from": from, "to": to} {
		if v == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", v); err != nil {
			return repository.StatsRange{}, &ValidationError{Message: name + " must be YYYY-MM-DD"}
		}
	}
	if from != "" && to != "" && from > to {
		return repository.StatsRange{}, &ValidationError{Message: "from must not be after to"}
	}
	return repository.StatsRange{From: from, To: to}, nil
}

func rangePayload(rng repository.StatsRange) map[string]any {
	out := map[string]any{}
	if rng.From != "" {
		out["from"] = rng.From
	}
	if rng.To != "" {
		out["to"] = rng.To
	}
	return out
}

func totalsPayload(t repository.StatsTotals) map[string]any {
	return map[string]any{
		"watches": t.Watches, "movie_watches": t.MovieWatches, "episode_watches": t.EpisodeWatches,
		"rewatches": t.Rewatches, "distinct_titles": t.DistinctTitles,
		"minutes": t.Minutes, "hours": round2(float64(t.Minutes) / 60),
		"days": t.Days, "rated": t.Rated, "liked": t.Liked,
	}
}

var weekdayNames = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// weekdayPayload names the days SQLite returns as "0".."6".
func weekdayPayload(rows []repository.LabelledStat) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		name := r.Label
		if n, err := strconv.Atoi(r.Label); err == nil && n >= 0 && n < len(weekdayNames) {
			name = weekdayNames[n]
		}
		out = append(out, map[string]any{"weekday": r.Label, "name": name, "watches": r.Watches})
	}
	return out
}

// longestStreak is the longest run of consecutive days with a watch, and the
// day it ended.
func longestStreak(days []repository.DayStat) (int, string) {
	best, run, bestEnd := 0, 0, ""
	var prev time.Time
	for _, d := range days {
		day, err := time.Parse("2006-01-02", d.Day)
		if err != nil {
			continue
		}
		if run > 0 && day.Equal(prev.AddDate(0, 0, 1)) {
			run++
		} else {
			run = 1
		}
		if run > best {
			best, bestEnd = run, d.Day
		}
		prev = day
	}
	return best, bestEnd
}

// currentStreak is the run of consecutive days ending today or yesterday.
// Yesterday still counts, so a streak does not look broken all morning.
func currentStreak(days []repository.DayStat, today string) int {
	seen := make(map[string]bool, len(days))
	for _, d := range days {
		seen[d.Day] = true
	}
	start, err := time.Parse("2006-01-02", today)
	if err != nil {
		return 0
	}
	if !seen[today] {
		start = start.AddDate(0, 0, -1)
		if !seen[start.Format("2006-01-02")] {
			return 0
		}
	}
	run := 0
	for seen[start.Format("2006-01-02")] {
		run++
		start = start.AddDate(0, 0, -1)
	}
	return run
}

// tasteBreakdown counts genres, countries, decades and languages over the
// distinct titles watched. Titles metareel has no metadata for simply do not
// contribute, rather than being counted as unknown.
func tasteBreakdown(titles []repository.WatchedTitle) (genres, countries, decades, languages []map[string]any) {
	byGenre, byCountry, byDecade, byLanguage := map[string]int64{}, map[string]int64{}, map[string]int64{}, map[string]int64{}
	for _, t := range titles {
		for _, g := range t.Genres {
			byGenre[g]++
		}
		for _, c := range t.Countries {
			byCountry[c]++
		}
		if t.Language != "" {
			byLanguage[t.Language]++
		}
		if len(t.ReleaseDate) >= 4 {
			if year, err := strconv.Atoi(t.ReleaseDate[:4]); err == nil {
				byDecade[strconv.Itoa(year/10*10)+"s"]++
			}
		}
	}
	return rankCounts(byGenre, "genre"), rankCounts(byCountry, "country"),
		rankCounts(byDecade, "decade"), rankCounts(byLanguage, "language")
}

// rankCounts turns a tally into a ranked list, most first.
func rankCounts(counts map[string]int64, key string) []map[string]any {
	out := make([]map[string]any, 0, len(counts))
	for k, n := range counts {
		out = append(out, map[string]any{key: k, "titles": n})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i]["titles"].(int64), out[j]["titles"].(int64)
		if a != b {
			return a > b
		}
		return out[i][key].(string) < out[j][key].(string)
	})
	return out
}

// crowdComparison summarises how a user's ratings sit against each outside
// source: the average gap, and how often they are above it.
func crowdComparison(rows []repository.RatingComparison) map[string]any {
	type acc struct {
		n, above int64
		sum      float64
	}
	sources := map[string]*acc{"tmdb": {}, "imdb": {}, "rotten_tomatoes": {}}
	for _, r := range rows {
		user := float64(r.UserRating)
		for name, crowd := range map[string]*float64{
			"tmdb": r.TmdbRating, "imdb": r.ImdbRating, "rotten_tomatoes": r.RtRating,
		} {
			if crowd == nil || *crowd <= 0 {
				continue
			}
			a := sources[name]
			a.n++
			a.sum += user - *crowd
			if user > *crowd {
				a.above++
			}
		}
	}
	out := map[string]any{}
	for name, a := range sources {
		entry := map[string]any{"compared": a.n, "above_the_crowd": a.above}
		if a.n > 0 {
			entry["average_gap"] = round2(a.sum / float64(a.n))
		}
		out[name] = entry
	}
	return out
}

// CurrentYear is the year a year-in-review defaults to.
func (s *TrackingService) CurrentYear() int { return s.now().UTC().Year() }
