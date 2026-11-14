package service

import (
	"context"
	"sort"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Detailed statistics for the app's profile: everything about the shows or
// the films a user watched (or both), grouped in Go from one row per watch.

const (
	detailTopTitles   = 5
	detailTopPeople   = 5
	detailTopGenres   = 8
	detailBinges      = 5
	detailTrendWeeks  = 12
	detailTrendMonths = 12
	detailUpcomingMon = 6
	// The pace a catch-up date is projected from.
	detailPaceDays = 56
)

// PeriodStat is a count and its watch time over a period.
type PeriodStat struct {
	Label   string `json:"label"` // YYYY-MM-DD (week start), YYYY-MM or YYYY
	Watches int64  `json:"watches"`
	Minutes int64  `json:"minutes"`
}

// TitleTime is one title with the time spent on it.
type TitleTime struct {
	Kind       string `json:"tmdb_kind"`
	TmdbID     string `json:"tmdb_id"`
	Title      string `json:"title"`
	PosterPath string `json:"poster_path,omitempty"`
	Watches    int64  `json:"watches"`
	Minutes    int64  `json:"minutes"`
}

// Binge is the most episodes of one show watched on one day.
type Binge struct {
	TmdbID     string `json:"tmdb_id"`
	Title      string `json:"title"`
	PosterPath string `json:"poster_path,omitempty"`
	Day        string `json:"day"`
	Episodes   int64  `json:"episodes"`
	Minutes    int64  `json:"minutes"`
}

// LabelCount is a label with a count (genres, show stages).
type LabelCount struct {
	Key   string `json:"key,omitempty"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// DetailedStats is the whole profile view for one kind ("" = both).
type DetailedStats struct {
	Kind      string     `json:"kind"`
	Minutes   int64      `json:"minutes"`
	Watches   int64      `json:"watches"`
	Rewatches int64      `json:"rewatches"`
	Titles    int64      `json:"titles"`
	Episodes  int64      `json:"episodes"`
	Movies    int64      `json:"movies"`
	Shows     int64      `json:"shows"` // distinct shows with a watch
	Films     int64      `json:"films"` // distinct films watched
	ThisYear  PeriodStat `json:"this_year"`
	// Episodes and films this year, for the profile's "This year" row.
	ThisYearEpisodes int64      `json:"this_year_episodes"`
	ThisYearMovies   int64      `json:"this_year_movies"`
	ThisWeek         PeriodStat `json:"this_week"`
	ThisMonth        PeriodStat `json:"this_month"`
	Streak           struct {
		Current int `json:"current"`
		Longest int `json:"longest"`
	} `json:"streak"`
	TopTitles []TitleTime `json:"top_titles"`
	Trend     struct {
		Weeks  []PeriodStat `json:"weeks"`
		Months []PeriodStat `json:"months"`
		Years  []PeriodStat `json:"years"`
	} `json:"trend"`
	ByWeekday []PeriodStat `json:"by_weekday"` // Sunday first; label is the day's name
	Ratings   struct {
		Rated        int64           `json:"rated"`
		Average      *float64        `json:"average,omitempty"`
		Distribution map[int64]int64 `json:"distribution"`
	} `json:"ratings"`
	Genres    []LabelCount            `json:"genres"`
	Actors    []repository.PersonStat `json:"actors"`
	Directors []repository.PersonStat `json:"directors"`
	Binges    []Binge                 `json:"binges"`
	// Shows only (kind tv).
	ShowsByStage    []LabelCount `json:"shows_by_stage,omitempty"`
	MinutesLeft     *int64       `json:"minutes_left,omitempty"`
	UpcomingByMonth []PeriodStat `json:"upcoming_by_month,omitempty"`
	CatchUpDate     string       `json:"catch_up_date,omitempty"`
	PaceMinutesWeek int64        `json:"pace_minutes_per_week"`
}

var stageLabels = []struct{ key, label string }{
	{StageWatching, "Watching"},
	{StageStalled, "Not watched lately"},
	{StageNotStarted, "Not started"},
	{StageOnHold, "On hold"},
	{StageCaughtUp, "Up to date"},
	{StageDropped, "Stopped"},
}

// DetailedStats gathers the profile's statistics for "tv", "movie" or "" (both).
func (s *TrackingService) DetailedStats(ctx context.Context, userID int64, kind string) (*DetailedStats, error) {
	if kind != "" && kind != "tv" && kind != "movie" {
		return nil, &ValidationError{Message: "kind must be tv, movie or empty"}
	}
	rows, err := s.read.StatsWatchRows(ctx, userID, kind)
	if err != nil {
		return nil, err
	}
	today, err := time.Parse(time.DateOnly, s.today())
	if err != nil {
		return nil, err
	}
	out := &DetailedStats{Kind: kind}
	out.Ratings.Distribution = map[int64]int64{}
	aggregate(out, rows, today)

	rng := repository.StatsRange{}
	if out.Actors, err = s.read.StatsTopPeople(ctx, userID, rng, kind, "", detailTopPeople); err != nil {
		return nil, err
	}
	if out.Directors, err = s.read.StatsTopPeople(ctx, userID, rng, kind, jobDirector, detailTopPeople); err != nil {
		return nil, err
	}
	if kind == "tv" {
		if err := s.showStats(ctx, userID, out, today); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// aggregate fills everything that comes from the watch rows themselves.
func aggregate(out *DetailedStats, rows []repository.WatchRow, today time.Time) {
	year := today.Format("2006")
	month := today.Format("2006-01")
	// Weeks start on Monday.
	weekStart := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	paceFrom := today.AddDate(0, 0, -detailPaceDays).Format(time.DateOnly)

	titles := map[string]*TitleTime{}
	shows, films := map[string]bool{}, map[string]bool{}
	genres := map[string]map[string]bool{}
	type bingeKey struct{ id, day string }
	binges := map[bingeKey]*Binge{}
	days := map[string]bool{}
	years := map[string]*PeriodStat{}
	weekdays := make([]PeriodStat, 7)
	for i, n := range []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"} {
		weekdays[i].Label = n
	}
	weeks := make([]PeriodStat, detailTrendWeeks)
	for i := range weeks {
		weeks[i].Label = weekStart.AddDate(0, 0, -7*(detailTrendWeeks-1-i)).Format(time.DateOnly)
	}
	months := make([]PeriodStat, detailTrendMonths)
	firstOfMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	for i := range months {
		months[i].Label = firstOfMonth.AddDate(0, -(detailTrendMonths - 1 - i), 0).Format("2006-01")
	}
	var ratingSum, paceMinutes int64

	for _, r := range rows {
		out.Watches++
		out.Minutes += r.Minutes
		if r.Rewatch {
			out.Rewatches++
		}
		key := r.Kind + ":" + r.TmdbID
		if r.SubjectKind == "episode" {
			out.Episodes++
			shows[key] = true
		} else {
			out.Movies++
			films[key] = true
		}
		t := titles[key]
		if t == nil {
			t = &TitleTime{Kind: r.Kind, TmdbID: r.TmdbID, Title: r.Title, PosterPath: r.PosterPath}
			titles[key] = t
		}
		t.Watches++
		t.Minutes += r.Minutes
		for _, g := range r.Genres {
			if genres[g] == nil {
				genres[g] = map[string]bool{}
			}
			genres[g][key] = true
		}
		if r.Rating > 0 {
			out.Ratings.Rated++
			ratingSum += r.Rating
			out.Ratings.Distribution[r.Rating]++
		}
		days[r.WatchedOn] = true
		day, err := time.Parse(time.DateOnly, r.WatchedOn)
		if err != nil {
			continue
		}
		if len(r.WatchedOn) >= 7 {
			if r.WatchedOn[:4] == year {
				addTo(&out.ThisYear, r.Minutes)
				if r.SubjectKind == "episode" {
					out.ThisYearEpisodes++
				} else {
					out.ThisYearMovies++
				}
			}
			if r.WatchedOn[:7] == month {
				addTo(&out.ThisMonth, r.Minutes)
			}
			y := years[r.WatchedOn[:4]]
			if y == nil {
				y = &PeriodStat{Label: r.WatchedOn[:4]}
				years[r.WatchedOn[:4]] = y
			}
			addTo(y, r.Minutes)
			for i := range months {
				if months[i].Label == r.WatchedOn[:7] {
					addTo(&months[i], r.Minutes)
				}
			}
		}
		if !day.Before(weekStart) {
			addTo(&out.ThisWeek, r.Minutes)
		}
		// Which of the trend's weeks (Monday to Sunday) the day falls in.
		dayWeek := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
		if back := int(weekStart.Sub(dayWeek).Hours()/24) / 7; back >= 0 && back < detailTrendWeeks {
			addTo(&weeks[detailTrendWeeks-1-back], r.Minutes)
		}
		addTo(&weekdays[int(day.Weekday())], r.Minutes)
		if r.WatchedOn > paceFrom {
			paceMinutes += r.Minutes
		}
		if r.SubjectKind == "episode" {
			b := binges[bingeKey{r.TmdbID, r.WatchedOn}]
			if b == nil {
				b = &Binge{TmdbID: r.TmdbID, Title: r.Title, PosterPath: r.PosterPath, Day: r.WatchedOn}
				binges[bingeKey{r.TmdbID, r.WatchedOn}] = b
			}
			b.Episodes++
			b.Minutes += r.Minutes
		}
	}

	out.Shows, out.Films, out.Titles = int64(len(shows)), int64(len(films)), int64(len(titles))
	if out.Ratings.Rated > 0 {
		avg := float64(ratingSum) / float64(out.Ratings.Rated)
		out.Ratings.Average = &avg
	}
	out.PaceMinutesWeek = paceMinutes * 7 / detailPaceDays

	top := make([]TitleTime, 0, len(titles))
	for _, t := range titles {
		top = append(top, *t)
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].Minutes != top[j].Minutes {
			return top[i].Minutes > top[j].Minutes
		}
		return top[i].Title < top[j].Title
	})
	out.TopTitles = top[:min(detailTopTitles, len(top))]

	out.Genres = make([]LabelCount, 0, len(genres))
	for g, set := range genres {
		out.Genres = append(out.Genres, LabelCount{Label: g, Count: int64(len(set))})
	}
	sort.Slice(out.Genres, func(i, j int) bool {
		if out.Genres[i].Count != out.Genres[j].Count {
			return out.Genres[i].Count > out.Genres[j].Count
		}
		return out.Genres[i].Label < out.Genres[j].Label
	})
	out.Genres = out.Genres[:min(detailTopGenres, len(out.Genres))]

	out.Binges = make([]Binge, 0, detailBinges)
	all := make([]Binge, 0, len(binges))
	for _, b := range binges {
		if b.Episodes >= 2 {
			all = append(all, *b)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Episodes != all[j].Episodes {
			return all[i].Episodes > all[j].Episodes
		}
		return all[i].Day > all[j].Day
	})
	out.Binges = append(out.Binges, all[:min(detailBinges, len(all))]...)

	out.Trend.Weeks, out.Trend.Months = weeks, months
	out.Trend.Years = make([]PeriodStat, 0, len(years))
	for _, y := range years {
		out.Trend.Years = append(out.Trend.Years, *y)
	}
	sort.Slice(out.Trend.Years, func(i, j int) bool { return out.Trend.Years[i].Label < out.Trend.Years[j].Label })
	out.ByWeekday = weekdays

	dayList := make([]repository.DayStat, 0, len(days))
	for d := range days {
		dayList = append(dayList, repository.DayStat{Day: d})
	}
	sort.Slice(dayList, func(i, j int) bool { return dayList[i].Day < dayList[j].Day })
	out.Streak.Longest, _ = longestStreak(dayList)
	out.Streak.Current = currentStreak(dayList, today.Format(time.DateOnly))
}

func addTo(p *PeriodStat, minutes int64) {
	p.Watches++
	p.Minutes += minutes
}

// showStats adds the shows-only figures: shows by stage, the time left on
// them, the episodes still to air by month, and when you'd catch up at your
// recent pace.
func (s *TrackingService) showStats(ctx context.Context, userID int64, out *DetailedStats, today time.Time) error {
	staleDays := defaultStaleDays
	if p, _, err := s.preferences(ctx, userID); err == nil && p.StaleDays != nil {
		staleDays = *p.StaleDays
	}
	rows, err := s.read.ShowQueue(ctx, userID, s.today())
	if err != nil {
		return err
	}
	staleBefore := s.now().UTC().AddDate(0, 0, -staleDays)
	counts := map[string]int64{}
	var left int64
	for _, r := range rows {
		st := stageOf(r, staleBefore)
		counts[st]++
		if st == StageWatching || st == StageStalled || st == StageNotStarted {
			left += r.MinutesLeft
		}
	}
	for _, sl := range stageLabels {
		out.ShowsByStage = append(out.ShowsByStage, LabelCount{Key: sl.key, Label: sl.label, Count: counts[sl.key]})
	}
	out.MinutesLeft = &left

	first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	until := first.AddDate(0, detailUpcomingMon, 0).AddDate(0, 0, -1)
	eps, err := s.read.UpcomingEpisodes(ctx, userID, today.Format(time.DateOnly), until.Format(time.DateOnly), maxUpcomingEpisodes)
	if err != nil {
		return err
	}
	out.UpcomingByMonth = make([]PeriodStat, detailUpcomingMon)
	for i := range out.UpcomingByMonth {
		out.UpcomingByMonth[i].Label = first.AddDate(0, i, 0).Format("2006-01")
	}
	for _, e := range eps {
		if e.AirDate == nil || len(*e.AirDate) < 7 {
			continue
		}
		for i := range out.UpcomingByMonth {
			if out.UpcomingByMonth[i].Label == (*e.AirDate)[:7] {
				var m int64
				if e.Runtime != nil {
					m = *e.Runtime
				}
				addTo(&out.UpcomingByMonth[i], m)
			}
		}
	}

	if out.PaceMinutesWeek > 0 && left > 0 {
		weeks := float64(left) / float64(out.PaceMinutesWeek)
		out.CatchUpDate = today.AddDate(0, 0, int(weeks*7+0.5)).Format(time.DateOnly)
	}
	return nil
}
