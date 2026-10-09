package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// FlixPatrolJob scrapes FlixPatrol Top 10 charts and stores them. It only
// records which titles charted; matching them to TMDB / IMDb / RT is
// TitleEnricher's job, so a scrape is a handful of page fetches.
type FlixPatrolJob struct {
	log     *slog.Logger
	fetcher flixpatrol.Fetcher
	repo    *repository.FlixPatrolRepository
	now     func() time.Time
}

func NewFlixPatrolJob(log *slog.Logger, fetcher flixpatrol.Fetcher, repo *repository.FlixPatrolRepository) *FlixPatrolJob {
	return &FlixPatrolJob{log: log, fetcher: fetcher, repo: repo, now: time.Now}
}

type FlixPatrolScrapeTarget struct {
	CountrySlug string
	Provider    flixpatrol.Provider
}

type FlixPatrolRunOptions struct {
	RequestDelay  time.Duration
	UserAgent     string
	RespectRobots bool
	RunID         int64 // task run the charts are recorded under (0 = none)

	// Dates to scrape; empty = the current chart date (plus gaps, see
	// BackfillDays).
	Dates []time.Time
	// Force re-scrapes charts that are already stored.
	Force bool
	// BackfillDays also scrapes charts missing from the N days before the
	// current chart date (ignored when Dates is set).
	BackfillDays int
	// AcceptUnchanged stores the current date's chart even if it is identical
	// to the previous stored date (FlixPatrol may not have updated it yet).
	// When false such a chart is reported as not_fresh and not stored.
	AcceptUnchanged bool

	// RunLog, if set, receives a human-readable progress log for the run
	// (level is debug, info, warn or error), shown with the run in the admin UI.
	RunLog func(level, message string)
}

func (o FlixPatrolRunOptions) runLog(level, format string, args ...any) {
	if o.RunLog != nil {
		o.RunLog(level, fmt.Sprintf(format, args...))
	}
}

// Target result statuses.
const (
	TargetSucceeded = "succeeded"
	TargetFailed    = "failed"
	TargetSkipped   = "skipped"   // already stored
	TargetNotFresh  = "not_fresh" // FlixPatrol still shows the previous day's chart
)

// TargetResult is the outcome of one chart page (target × date).
type TargetResult struct {
	Country     string `json:"country"`
	CountrySlug string `json:"country_slug"`
	Provider    string `json:"provider"`
	Date        string `json:"date"`
	Status      string `json:"status"`
	Movies      int    `json:"movies,omitempty"`
	TVShows     int    `json:"tv_shows,omitempty"`
	NewTitles   int    `json:"new_titles,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ScrapeReport summarises a run.
type ScrapeReport struct {
	Results []TargetResult `json:"targets"`
	// NewTitleIDs are titles first seen in this run (to match next).
	NewTitleIDs []int64 `json:"-"`
}

// Count returns how many results have status.
func (r *ScrapeReport) Count(status string) int {
	n := 0
	for _, res := range r.Results {
		if res.Status == status {
			n++
		}
	}
	return n
}

// NotFresh returns the targets whose current chart was not published yet.
func (r *ScrapeReport) NotFresh() []FlixPatrolScrapeTarget {
	var out []FlixPatrolScrapeTarget
	for _, res := range r.Results {
		if res.Status == TargetNotFresh {
			out = append(out, FlixPatrolScrapeTarget{CountrySlug: res.CountrySlug, Provider: flixpatrol.Provider(res.Provider)})
		}
	}
	return out
}

// maxConsecutiveFetchFailures stops a run early when FlixPatrol (or
// FlareSolverr) is clearly down, instead of timing out on every target.
const maxConsecutiveFetchFailures = 3

// Run scrapes every target for each date to scrape. A failed target does not
// stop the others; the returned error is only for cancellation.
func (j *FlixPatrolJob) Run(ctx context.Context, targets []FlixPatrolScrapeTarget, opts FlixPatrolRunOptions) (*ScrapeReport, error) {
	report := &ScrapeReport{}
	current := flixpatrol.EffectiveTop10Date(j.now())

	if err := flixpatrol.CheckHealth(ctx, j.fetcher); err != nil {
		msg := fmt.Sprintf("FlixPatrol fetcher is not reachable: %v", err)
		opts.runLog("error", "%s; no targets scraped", msg)
		dates := opts.Dates
		if len(dates) == 0 {
			dates = []time.Time{current}
		}
		for _, t := range targets {
			code, _ := flixpatrol.GetCountryCode(t.CountrySlug)
			for _, d := range dates {
				report.Results = append(report.Results, TargetResult{
					Country: string(code), CountrySlug: t.CountrySlug, Provider: string(t.Provider),
					Date: repository.FormatDate(d), Status: TargetFailed, Error: msg,
				})
			}
		}
		return report, nil
	}

	fetched, failStreak := 0, 0
	for i, target := range targets {
		code, err := flixpatrol.GetCountryCode(target.CountrySlug)
		if err != nil {
			report.Results = append(report.Results, TargetResult{
				CountrySlug: target.CountrySlug, Provider: string(target.Provider), Status: TargetFailed, Error: err.Error(),
			})
			continue
		}
		country := string(code)

		dates, err := j.datesFor(ctx, country, target, current, opts)
		if err != nil {
			return report, err
		}
		for _, date := range dates {
			res := TargetResult{
				Country: country, CountrySlug: target.CountrySlug, Provider: string(target.Provider),
				Date: repository.FormatDate(date),
			}
			label := fmt.Sprintf("[%d/%d] %s / %s %s", i+1, len(targets), target.Provider, country, res.Date)

			if failStreak >= maxConsecutiveFetchFailures {
				res.Status, res.Error = TargetFailed, "skipped: FlixPatrol fetches kept failing in this run"
				report.Results = append(report.Results, res)
				continue
			}

			if !opts.Force {
				stored, err := j.repo.ChartSnapshotsForDate(ctx, date, country, string(target.Provider))
				if err != nil {
					return report, err
				}
				if len(stored) >= 2 {
					res.Status = TargetSkipped
					opts.runLog("info", "%s: already stored, skipped", label)
					report.Results = append(report.Results, res)
					continue
				}
			}

			if fetched > 0 {
				if err := sleepCtx(ctx, opts.RequestDelay); err != nil {
					return report, err
				}
			}
			fetched++
			isCurrent := date.Equal(current)
			j.scrapeOne(ctx, target, country, date, isCurrent, opts, label, &res, report)
			if res.Status == TargetFailed {
				failStreak++
			} else {
				failStreak = 0
			}
			if err := ctx.Err(); err != nil {
				return report, err
			}
			report.Results = append(report.Results, res)
		}
	}
	return report, nil
}

// datesFor lists the dates to scrape for a target, oldest first: explicit
// dates, or the current chart date plus any missing dates in the backfill
// window. Oldest first matters: the freshness check compares today's chart
// with the previous stored date, which should be yesterday's.
func (j *FlixPatrolJob) datesFor(ctx context.Context, country string, target FlixPatrolScrapeTarget, current time.Time, opts FlixPatrolRunOptions) ([]time.Time, error) {
	if len(opts.Dates) > 0 {
		dates := append([]time.Time(nil), opts.Dates...)
		sort.Slice(dates, func(a, b int) bool { return dates[a].Before(dates[b]) })
		return dates, nil
	}
	var dates []time.Time
	if opts.BackfillDays > 0 {
		from := current.AddDate(0, 0, -opts.BackfillDays)
		to := current.AddDate(0, 0, -1)
		stored, err := j.repo.StoredChartDates(ctx, country, string(target.Provider), from, to)
		if err != nil {
			return nil, err
		}
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			if !stored[repository.FormatDate(d)] {
				dates = append(dates, d)
			}
		}
	}
	return append(dates, current), nil
}

func (j *FlixPatrolJob) scrapeOne(ctx context.Context, target FlixPatrolScrapeTarget, country string, date time.Time, isCurrent bool, opts FlixPatrolRunOptions, label string, res *TargetResult, report *ScrapeReport) {
	u := flixpatrol.Top10URLForDate(target.Provider, target.CountrySlug, date)
	start := time.Now()
	j.log.Info("flixpatrol scraping", slog.String("provider", string(target.Provider)), slog.String("country", target.CountrySlug), slog.String("url", u))
	top10, err := flixpatrol.ScrapeTop10(ctx, j.fetcher, u, opts.UserAgent, opts.RespectRobots)
	if err != nil {
		res.Status, res.Error = TargetFailed, truncateErr(err, 300)
		opts.runLog("error", "%s: scrape failed after %s: %v", label, time.Since(start).Round(time.Millisecond), truncateErr(err, 300))
		return
	}
	res.Movies, res.TVShows = len(top10.Movies), len(top10.TVShows)

	charts := []struct {
		category string
		entries  []flixpatrol.Entry
	}{{"movies", top10.Movies}, {"tv_shows", top10.TVShows}}

	if isCurrent && !opts.AcceptUnchanged {
		unchanged := true
		for _, c := range charts {
			prev, err := j.repo.PreviousChartSnapshot(ctx, date, country, string(target.Provider), c.category)
			if err != nil {
				res.Status, res.Error = TargetFailed, err.Error()
				return
			}
			if prev == nil || prev.Signature != repository.ChartSignature(chartEntries(c.entries, nil)) {
				unchanged = false
				break
			}
		}
		if unchanged {
			res.Status = TargetNotFresh
			opts.runLog("warn", "%s: same chart as the previous stored date; FlixPatrol has probably not published today's yet, not saved", label)
			return
		}
	}

	for _, c := range charts {
		ids := make(map[string]int64, len(c.entries))
		for _, e := range c.entries {
			title, err := j.repo.EnsureTitle(ctx, e.Slug, e.Name, string(e.TitleKind))
			if err != nil {
				res.Status, res.Error = TargetFailed, fmt.Sprintf("save title %s: %v", e.Slug, err)
				opts.runLog("error", "%s: %s", label, res.Error)
				return
			}
			ids[e.Slug] = title.ID
			if title.MatchStatus == repository.MatchStatusPending && title.MatchAttempts == 0 {
				report.NewTitleIDs = appendUnique(report.NewTitleIDs, title.ID)
				res.NewTitles++
			}
		}
		if err := j.repo.SaveChart(ctx, repository.ChartWrite{
			RankedOn: date, Country: country, StreamingProvider: string(target.Provider), Category: c.category,
			Source: "flixpatrol", RunID: opts.RunID, Entries: chartEntries(c.entries, ids),
		}); err != nil {
			res.Status, res.Error = TargetFailed, fmt.Sprintf("save %s chart: %v", c.category, err)
			opts.runLog("error", "%s: %s", label, res.Error)
			return
		}
	}
	res.Status = TargetSucceeded
	opts.runLog("info", "%s: saved %d movies and %d TV shows (%d new titles) in %s",
		label, res.Movies, res.TVShows, res.NewTitles, time.Since(start).Round(time.Millisecond))
}

// chartEntries converts parsed entries for storage; ids maps slug → title ID
// (nil when only the signature is needed). TV entries named "...: Season N"
// carry the season number.
func chartEntries(entries []flixpatrol.Entry, ids map[string]int64) []repository.ChartEntry {
	out := make([]repository.ChartEntry, 0, len(entries))
	for _, e := range entries {
		ce := repository.ChartEntry{TitleID: ids[e.Slug], Slug: e.Slug, Rank: int64(e.Rank)}
		if e.TitleKind == flixpatrol.TitleKindTVShow {
			if _, season := flixpatrol.SeasonFromName(e.Name); season > 0 {
				n := int64(season)
				ce.SeasonNumber = &n
			}
		}
		out = append(out, ce)
	}
	return out
}

func appendUnique(ids []int64, id int64) []int64 {
	for _, x := range ids {
		if x == id {
			return ids
		}
	}
	return append(ids, id)
}
