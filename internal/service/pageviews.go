package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// pageviewSource reads daily article pageviews (client.Wikimedia).
type pageviewSource interface {
	Pageviews(ctx context.Context, project, article string, from, to time.Time) ([]client.DailyViews, error)
}

const (
	pageviewsProject   = "en.wikipedia"
	pageviewsFirstDays = 90 // history fetched for a title's first import
	// Wikimedia allows anonymous clients with a User-Agent 200 requests a
	// minute (https://www.mediawiki.org/wiki/Wikimedia_APIs/Rate_limits).
	pageviewsGap           = 350 * time.Millisecond
	pageviewsRateRetries   = 3
	pageviewsMaxFailStreak = 20 // consecutive failures that abort the run
)

// PageviewsImporter stores the daily English Wikipedia pageviews of every
// TMDB title with an article (the wikipedia.pageviews.import job): each run
// fetches the days since the last stored one, up to yesterday (UTC).
type PageviewsImporter struct {
	log     *slog.Logger
	source  pageviewSource
	history *repository.HistoryRepository
	now     func() time.Time
	gap     time.Duration
}

func NewPageviewsImporter(log *slog.Logger, source pageviewSource, history *repository.HistoryRepository) *PageviewsImporter {
	return &PageviewsImporter{log: log, source: source, history: history, now: time.Now, gap: pageviewsGap}
}

// PageviewsReport summarises a run.
type PageviewsReport struct {
	Titles   int `json:"titles"`     // with an English Wikipedia article
	UpToDate int `json:"up_to_date"` // already had yesterday
	Fetched  int `json:"fetched"`
	Missing  int `json:"missing"` // article not found (renamed or deleted; the metadata job looks it up again monthly)
	Failed   int `json:"failed"`
	Days     int `json:"days"` // day rows stored
}

// fetch reads one article's views, waiting out rate limits (Retry-After)
// up to pageviewsRateRetries times.
func (p *PageviewsImporter) fetch(ctx context.Context, article string, from, to time.Time) ([]client.DailyViews, error) {
	for attempt := 0; ; attempt++ {
		views, err := p.source.Pageviews(ctx, pageviewsProject, article, from, to)
		var rl *client.RateLimitedError
		if !errors.As(err, &rl) || attempt >= pageviewsRateRetries {
			return views, err
		}
		if err := sleepCtx(ctx, rl.RetryAfter); err != nil {
			return nil, err
		}
	}
}

// Run imports pageviews; force refetches the last 7 days of every title too.
func (p *PageviewsImporter) Run(ctx context.Context, force bool, runLog func(level, msg string)) (*PageviewsReport, error) {
	logf := func(level, format string, args ...any) {
		if runLog != nil {
			runLog(level, fmt.Sprintf(format, args...))
		}
	}
	targets, err := p.history.PageviewTargets(ctx)
	if err != nil {
		return nil, err
	}
	rep := &PageviewsReport{Titles: len(targets)}
	today := p.now().UTC().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)
	first := today.AddDate(0, 0, -pageviewsFirstDays)
	logf("info", "Wikipedia pageviews: %d titles with an English article", len(targets))
	streak, calls := 0, 0
	for _, t := range targets {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		from := first
		if t.LastDay != "" {
			last, err := time.Parse(repository.DateLayout, t.LastDay)
			if err == nil {
				from = last.AddDate(0, 0, 1)
				if force {
					from = last.AddDate(0, 0, -6)
				}
			}
		}
		if from.Before(first) {
			from = first
		}
		if from.After(yesterday) {
			rep.UpToDate++
			continue
		}
		if calls > 0 && p.gap > 0 {
			if err := sleepCtx(ctx, p.gap); err != nil {
				return rep, err
			}
		}
		calls++
		views, err := p.fetch(ctx, t.Article, from, yesterday)
		if err != nil {
			rep.Failed++
			streak++
			if rep.Failed <= 5 {
				logf("warn", "%s (%s): %v", t.Article, t.Ref, err)
			}
			if streak >= pageviewsMaxFailStreak {
				return rep, fmt.Errorf("wikipedia pageviews: %d requests in a row failed, last: %w", streak, err)
			}
			continue
		}
		streak = 0
		if views == nil {
			rep.Missing++
			continue
		}
		points := make([]repository.PageviewPoint, 0, len(views))
		for _, v := range views {
			points = append(points, repository.PageviewPoint{Date: v.Day, Views: v.Views})
		}
		if err := p.history.SavePageviews(ctx, t.Ref, pageviewsProject, points); err != nil {
			return rep, err
		}
		rep.Fetched++
		rep.Days += len(points)
	}
	logf("info", "Wikipedia pageviews: %d titles fetched (%d days), %d up to date, %d articles not found, %d failed",
		rep.Fetched, rep.Days, rep.UpToDate, rep.Missing, rep.Failed)
	if rep.Failed > 0 && rep.Fetched == 0 && rep.UpToDate == 0 {
		return rep, fmt.Errorf("every pageviews request failed (%d)", rep.Failed)
	}
	return rep, nil
}
