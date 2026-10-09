package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// TitleEnricher matches FlixPatrol titles to TMDB / IMDb / Rotten Tomatoes
// (the titles.enrich job). Scrapes only record titles; this job matches new
// ones right after a scrape and retries failures with backoff, so a title no
// one can match no longer costs lookups on every scrape.
type TitleEnricher struct {
	log      *slog.Logger
	repo     *repository.FlixPatrolRepository
	matcher  *TitleMatcher
	metadata *TitleMetadataService // optional: TMDB details + Wikidata IDs
}

func NewTitleEnricher(log *slog.Logger, repo *repository.FlixPatrolRepository, matcher *TitleMatcher, metadata *TitleMetadataService) *TitleEnricher {
	return &TitleEnricher{log: log, repo: repo, matcher: matcher, metadata: metadata}
}

// EnrichOptions controls an enrichment run.
type EnrichOptions struct {
	// TitleIDs limits the run to these titles and ignores their backoff
	// (manual titles are still left alone). Empty = the due backlog.
	TitleIDs      []int64
	Limit         int64         // max titles per phase for backlog runs (default 100)
	RequestDelay  time.Duration // before each FlixPatrol title-page fetch
	UserAgent     string
	RespectRobots bool
	RunLog        func(level, message string)
}

func (o EnrichOptions) logf(level, format string, args ...any) {
	if o.RunLog != nil {
		o.RunLog(level, fmt.Sprintf(format, args...))
	}
}

// EnrichReport summarises a run.
type EnrichReport struct {
	Attempted int `json:"attempted"`
	Matched   int `json:"matched"`
	Unmatched int `json:"unmatched"`
	Failed    int `json:"failed"`
	RTChecked int `json:"rt_checked"`
	RTFound   int `json:"rt_found"`
}

// retryAfter is the matching / RT lookup backoff after n failed attempts.
func retryAfter(attempts int64) string {
	switch {
	case attempts <= 1:
		return "+1 days"
	case attempts == 2:
		return "+3 days"
	case attempts == 3:
		return "+7 days"
	default:
		return "+30 days"
	}
}

const defaultEnrichLimit = 100

// Run matches unmatched titles, then looks up missing Rotten Tomatoes slugs
// (Wikidata first, in one batch, then RT's own search).
func (e *TitleEnricher) Run(ctx context.Context, opts EnrichOptions) (*EnrichReport, error) {
	rep := &EnrichReport{}
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultEnrichLimit
	}

	var toMatch, toRT []sqlc.Title
	if len(opts.TitleIDs) > 0 {
		titles, err := e.repo.ListTitlesByIDs(ctx, opts.TitleIDs)
		if err != nil {
			return rep, err
		}
		for _, t := range titles {
			switch {
			case t.MatchStatus == repository.MatchStatusManual:
			case !t.TmdbID.Valid || t.TmdbID.String == "":
				toMatch = append(toMatch, t)
			case !t.RtUrl.Valid || t.RtUrl.String == "":
				toRT = append(toRT, t)
			}
		}
	} else {
		var err error
		if toMatch, err = e.repo.ListTitlesDueForMatch(ctx, limit); err != nil {
			return rep, err
		}
	}
	opts.logf("info", "Matching %d title(s)", len(toMatch))

	fetched := 0
	for _, t := range toMatch {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		rep.Attempted++
		updated, err := e.matchOne(ctx, t, opts, &fetched)
		if err != nil {
			if ctx.Err() != nil {
				return rep, ctx.Err()
			}
			rep.Failed++
			opts.logf("error", "%s: %v", t.Slug, err)
			continue
		}
		if updated.MatchStatus == repository.MatchStatusMatched {
			rep.Matched++
			if !updated.RtUrl.Valid || updated.RtUrl.String == "" {
				toRT = append(toRT, updated)
			}
		} else {
			rep.Unmatched++
		}
	}

	if len(opts.TitleIDs) == 0 {
		due, err := e.repo.ListTitlesDueForRT(ctx, limit)
		if err != nil {
			return rep, err
		}
		toRT = mergeTitles(toRT, due)
	}
	if err := e.findRT(ctx, toRT, opts, rep); err != nil {
		return rep, err
	}

	opts.logf("info", "Done: %d matched, %d unmatched, %d failed; Rotten Tomatoes found for %d of %d",
		rep.Matched, rep.Unmatched, rep.Failed, rep.RTFound, rep.RTChecked)
	if rep.Failed > 0 && rep.Matched+rep.Unmatched == 0 {
		return rep, fmt.Errorf("every matching attempt failed (%d)", rep.Failed)
	}
	return rep, nil
}

// matchOne reads (once) the FlixPatrol title page, matches the title and
// records the attempt.
func (e *TitleEnricher) matchOne(ctx context.Context, t sqlc.Title, opts EnrichOptions, fetched *int) (sqlc.Title, error) {
	details := storedDetails(t)
	if details == nil && e.matcher.fetcher != nil {
		delay := opts.RequestDelay
		if *fetched == 0 {
			delay = 0
		}
		*fetched++
		d, err := e.matcher.FetchDetails(ctx, t.Slug, FetchOptions{RequestDelay: delay, UserAgent: opts.UserAgent, RespectRobots: opts.RespectRobots})
		if err != nil {
			if ctx.Err() != nil {
				return t, ctx.Err()
			}
			// Match on the chart name and slug year; try the page next time.
			opts.logf("warn", "%s: FlixPatrol title page unavailable (%s); matching on the chart name", t.Slug, truncateErr(err, 200))
		} else {
			details = d
			if err := e.repo.SaveFlixPatrolDetails(ctx, t.ID, repository.FlixPatrolDetails{
				Name: d.Name, Kind: string(d.TitleKind), PremiereDate: d.Premiere, Country: d.Country,
			}); err != nil {
				return t, err
			}
		}
	}

	country, provider, category, err := e.repo.LatestChart(ctx, t.ID)
	if err != nil {
		return t, err
	}
	chartKind := flixpatrol.TitleKind(t.Kind)
	switch category {
	case "movies":
		chartKind = flixpatrol.TitleKindMovie
	case "tv_shows":
		chartKind = flixpatrol.TitleKindTVShow
	}
	in := MatchInput{
		Slug: t.Slug, ChartName: t.Name, ChartKind: chartKind, Details: details,
		Provider: flixpatrol.Provider(provider), Country: country,
	}
	m, err := e.matcher.Match(ctx, in)
	if err != nil {
		return t, err
	}

	res := repository.TitleMatchResult{Kind: string(m.Kind), RetryAfter: retryAfter(t.MatchAttempts + 1)}
	if m.TmdbID != "" {
		res.TmdbID, res.ImdbID, res.JustWatchID = m.TmdbID, m.ImdbID, m.JustWatchID
		res.Source, res.MatchedName, res.MatchedYear = m.Source, m.Title, m.Year
	}
	updated, err := e.repo.SaveTitleMatch(ctx, t.ID, res)
	if repository.IsNoRows(err) {
		return t, nil // became manual meanwhile
	}
	if err != nil {
		return t, err
	}
	if m.TmdbID == "" {
		opts.logf("warn", "%s: no confident match for %s; retry %s", t.Slug, in.QueryDescription(), res.RetryAfter)
		return updated, nil
	}
	opts.logf("info", "%s: matched %s %q (%d), TMDB %s via %s", t.Slug, m.Kind, m.Title, m.Year, m.TmdbID, m.Source)

	// Details, watch providers and IMDb / Wikidata IDs straight away.
	if e.metadata != nil {
		ref := repository.TmdbRef{Kind: tmdbKindOf(m.Kind), ID: m.TmdbID}
		d, err := e.metadata.RefreshDetails(ctx, ref)
		if err != nil {
			e.log.Warn("tmdb details after match failed", slog.String("slug", t.Slug), slog.Any("err", err))
		} else if d != nil {
			if err := e.repo.SaveTitleExternalIDs(ctx, t.ID, d.IMDbID, d.WikidataID, ""); err != nil {
				return updated, err
			}
		}
	}
	return updated, nil
}

// findRT looks up missing Rotten Tomatoes slugs: one Wikidata batch for all
// titles, then RT's search for the rest, recording each attempt's backoff.
func (e *TitleEnricher) findRT(ctx context.Context, titles []sqlc.Title, opts EnrichOptions, rep *EnrichReport) error {
	if len(titles) == 0 {
		return nil
	}
	wd := map[string]string{} // ref → RT slug from Wikidata
	if e.metadata != nil {
		refs := make([]repository.TmdbRef, 0, len(titles))
		for _, t := range titles {
			refs = append(refs, repository.TmdbRef{Kind: tmdbKindOf(flixpatrol.TitleKind(t.Kind)), ID: t.TmdbID.String})
		}
		found, err := e.metadata.LookupWikidata(ctx, refs)
		if err != nil {
			opts.logf("warn", "Wikidata lookup failed: %v; falling back to Rotten Tomatoes search", err)
		}
		for ref, ids := range found {
			if ids.RottenTomato != "" {
				wd[ref] = ids.RottenTomato
			}
		}
	}

	for _, t := range titles {
		if err := ctx.Err(); err != nil {
			return err
		}
		rep.RTChecked++
		kind := flixpatrol.TitleKind(t.Kind)
		ref := repository.TmdbRef{Kind: tmdbKindOf(kind), ID: t.TmdbID.String}
		slug, from := wd[ref.String()], "wikidata"
		if slug == "" {
			name, year := t.Name, flixpatrol.YearFromSlug(t.Slug)
			if t.MatchedName.Valid && t.MatchedYear.Valid {
				name, year = t.MatchedName.String, int(t.MatchedYear.Int64)
			}
			var err error
			slug, err = e.matcher.FindRTSlug(ctx, RTQuery{Slug: t.Slug, Name: name, Year: year, Kind: kind, TmdbID: t.TmdbID.String, From: "match"})
			if err != nil {
				continue // transient: retry on the next run, no backoff recorded
			}
			from = "search"
		}
		retry := retryAfter(t.RtAttempts + 1)
		if err := e.repo.SaveTitleRTAttempt(ctx, t.ID, slug, retry); err != nil {
			return err
		}
		if slug != "" {
			rep.RTFound++
			opts.logf("info", "%s: Rotten Tomatoes %s (via %s)", t.Slug, slug, from)
		}
	}
	return nil
}

// storedDetails returns the FlixPatrol title-page details saved earlier, or
// nil if the page was never read.
func storedDetails(t sqlc.Title) *flixpatrol.TitleDetails {
	if !t.FpFetchedAt.Valid {
		return nil
	}
	d := &flixpatrol.TitleDetails{Slug: t.Slug, Name: t.FpName.String, TitleKind: flixpatrol.TitleKind(t.FpKind.String), Premiere: t.FpPremiereDate.String, Country: t.FpCountry.String}
	if len(d.Premiere) >= 4 {
		fmt.Sscanf(d.Premiere[:4], "%d", &d.Year)
	}
	if d.Year == 0 {
		d.Year = flixpatrol.YearFromSlug(t.Slug)
	}
	return d
}

func mergeTitles(a, b []sqlc.Title) []sqlc.Title {
	seen := make(map[int64]bool, len(a))
	for _, t := range a {
		seen[t.ID] = true
	}
	for _, t := range b {
		if !seen[t.ID] {
			a = append(a, t)
			seen[t.ID] = true
		}
	}
	return a
}
