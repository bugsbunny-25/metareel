package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
	"github.com/bugsbunny-25/metareel/internal/tasks"
)

// TitleEnricher matches FlixPatrol titles to TMDB / IMDb / Rotten Tomatoes
// (the titles.enrich job). Scrapes only record titles; this job matches new
// ones right after a scrape and retries failures with backoff, so a title no
// one can match no longer costs lookups on every scrape.
type TitleEnricher struct {
	log      *slog.Logger
	repo     *repository.FlixPatrolRepository
	writer   *MappingWriter
	matcher  *TitleMatcher
	metadata *TitleMetadataService // optional: TMDB details + Wikidata IDs
	mapping  *TitleMappingService  // optional: shares mappings with linked Netflix titles
}

func NewTitleEnricher(log *slog.Logger, repo *repository.FlixPatrolRepository, writer *MappingWriter, matcher *TitleMatcher, metadata *TitleMetadataService) *TitleEnricher {
	return &TitleEnricher{log: log, repo: repo, writer: writer, matcher: matcher, metadata: metadata}
}

// WithMapping links titles to Netflix's official Top 10 titles before
// matching (a title linked to a matched Netflix title takes its mapping
// without a lookup) and shares new matches with them afterwards.
func (e *TitleEnricher) WithMapping(mapping *TitleMappingService) *TitleEnricher {
	e.mapping = mapping
	return e
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

	Mapping *MappingSyncReport `json:"mapping,omitempty"`
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

	if e.mapping != nil && len(opts.TitleIDs) == 0 {
		mr, err := e.mapping.Sync(ctx, true, opts.RunLog)
		rep.Mapping = mr
		if err != nil {
			return rep, err
		}
	}

	var toMatch []sqlc.Title
	var rtRefs []repository.TmdbRef // RT lookups for these TMDB titles (besides the due backlog)
	if len(opts.TitleIDs) > 0 {
		titles, err := e.repo.ListTitlesByIDs(ctx, opts.TitleIDs)
		if err != nil {
			return rep, err
		}
		for _, t := range titles {
			switch ref := repository.TitleRef(t); {
			case ref.ID != "":
				rtRefs = append(rtRefs, ref)
			case t.MatchStatus != repository.MatchStatusManual:
				toMatch = append(toMatch, t)
			}
		}
	} else {
		var err error
		if toMatch, err = e.repo.ListTitlesDueForMatch(ctx, limit); err != nil {
			return rep, err
		}
	}
	opts.logf("info", "Matching %d title(s)", len(toMatch))

	// Matches are written through one session: their TMDB details, IDs and
	// season numbers follow in one mapping refresh.
	session := e.writer.Session(tasks.TypeTitlesEnrich)
	defer session.Flush(ctx)
	fetched := 0
	for _, t := range toMatch {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		rep.Attempted++
		updated, err := e.matchOne(ctx, session, t, opts, &fetched)
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
			rtRefs = append(rtRefs, repository.TitleRef(updated))
		} else {
			rep.Unmatched++
		}
	}

	if err := e.findRT(ctx, rtRefs, len(opts.TitleIDs) == 0, limit, opts, rep); err != nil {
		return rep, err
	}

	if e.mapping != nil && len(opts.TitleIDs) > 0 {
		mr, err := e.mapping.SyncIn(ctx, session, true, opts.RunLog)
		if rep.Mapping == nil {
			rep.Mapping = mr
		} else if mr != nil {
			rep.Mapping.NetflixUpdated += mr.NetflixUpdated
			rep.Mapping.FlixPatrolUpdated += mr.FlixPatrolUpdated
			rep.Mapping.Conflicts = mr.Conflicts
		}
		if err != nil {
			return rep, err
		}
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
func (e *TitleEnricher) matchOne(ctx context.Context, session *MappingSession, t sqlc.Title, opts EnrichOptions, fetched *int) (sqlc.Title, error) {
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
	updated, err := session.MatchTitle(ctx, t.ID, res)
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
	// TMDB details, watch providers and IMDb / Wikidata IDs follow in the
	// mapping refresh.
	opts.logf("info", "%s: matched %s %q (%d), TMDB %s via %s", t.Slug, m.Kind, m.Title, m.Year, m.TmdbID, m.Source)
	return updated, nil
}

// findRT looks up missing Rotten Tomatoes slugs of TMDB titles (refs, plus
// the due backlog when backlog): one Wikidata batch for all, then RT's
// search for the rest, recording each attempt's backoff on the TMDB title.
func (e *TitleEnricher) findRT(ctx context.Context, refs []repository.TmdbRef, backlog bool, limit int64, opts EnrichOptions, rep *EnrichReport) error {
	if e.metadata == nil {
		return nil
	}
	var due []repository.RTDue
	seen := map[repository.TmdbRef]bool{}
	add := func(list []repository.RTDue) {
		for _, d := range list {
			if !seen[d.Ref] {
				seen[d.Ref] = true
				due = append(due, d)
			}
		}
	}
	for _, ref := range refs {
		if seen[ref] {
			continue
		}
		ref := ref
		list, err := e.metadata.RefsDueForRT(ctx, &ref, 1)
		if err != nil {
			return err
		}
		add(list)
	}
	if backlog {
		list, err := e.metadata.RefsDueForRT(ctx, nil, limit)
		if err != nil {
			return err
		}
		add(list)
	}
	if len(due) == 0 {
		return nil
	}
	wd := map[string]string{} // ref → RT slug from Wikidata
	lookup := make([]repository.TmdbRef, 0, len(due))
	for _, d := range due {
		lookup = append(lookup, d.Ref)
	}
	found, err := e.metadata.LookupWikidata(ctx, lookup)
	if err != nil {
		opts.logf("warn", "Wikidata lookup failed: %v; falling back to Rotten Tomatoes search", err)
	}
	for ref, ids := range found {
		if ids.RottenTomato != "" {
			wd[ref] = ids.RottenTomato
		}
	}

	for _, d := range due {
		if err := ctx.Err(); err != nil {
			return err
		}
		rep.RTChecked++
		kind := flixpatrol.TitleKind(titleKindOf(d.Ref.Kind))
		slug, from := wd[d.Ref.String()], "wikidata"
		if slug != "" {
			rep.RTFound++ // stored with the Wikidata IDs
			opts.logf("info", "%s: Rotten Tomatoes %s (via %s)", d.Ref, slug, from)
			continue
		}
		name, year := d.Name, flixpatrol.YearFromSlug(d.Slug)
		switch {
		case d.MatchedName != "" && d.MatchedYear > 0:
			name, year = d.MatchedName, d.MatchedYear
		case d.Title != "":
			name, year = d.Title, d.Year
		}
		slug, err := e.matcher.FindRTSlug(ctx, RTQuery{Slug: d.Slug, Name: name, Year: year, Kind: kind, TmdbID: d.Ref.ID, From: "match"})
		if err != nil {
			continue // transient: retry on the next run, no backoff recorded
		}
		if err := e.metadata.RecordRTAttempt(ctx, d.Ref, slug, retryAfter(d.Attempts+1)); err != nil {
			return err
		}
		if slug != "" {
			rep.RTFound++
			opts.logf("info", "%s (%s): Rotten Tomatoes %s (via search)", d.Ref, d.Slug, slug)
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
