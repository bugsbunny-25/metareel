package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

type FlixPatrolJob struct {
	log       *slog.Logger
	fetcher   flixpatrol.Fetcher
	repo      *repository.FlixPatrolRepository
	tmdb      *client.TMDBClient
	justWatch *client.JustWatchClient
	wikidata  *client.WikidataClient
	rt        *client.RottenTomatoesClient
}

func NewFlixPatrolJob(log *slog.Logger, fetcher flixpatrol.Fetcher, repo *repository.FlixPatrolRepository, tmdbClient *client.TMDBClient, justWatchClient *client.JustWatchClient, wikidataClient *client.WikidataClient, rtClient *client.RottenTomatoesClient) *FlixPatrolJob {
	return &FlixPatrolJob{
		log:       log,
		fetcher:   fetcher,
		repo:      repo,
		tmdb:      tmdbClient,
		justWatch: justWatchClient,
		wikidata:  wikidataClient,
		rt:        rtClient,
	}
}

type FlixPatrolScrapeTarget struct {
	CountrySlug string
	Provider    flixpatrol.Provider
}

type FlixPatrolRunOptions struct {
	RequestDelay  time.Duration
	UserAgent     string
	RespectRobots bool
	// RunLog, if set, receives a human-readable progress log for the run
	// (level is debug, info, warn or error), shown with the run in the admin UI.
	RunLog func(level, message string)
}

func (o FlixPatrolRunOptions) runLog(level, format string, args ...any) {
	if o.RunLog != nil {
		o.RunLog(level, fmt.Sprintf(format, args...))
	}
}

func (j *FlixPatrolJob) RunTargets(ctx context.Context, targets []FlixPatrolScrapeTarget, date time.Time, opts FlixPatrolRunOptions) error {
	for i, target := range targets {
		if i > 0 && opts.RequestDelay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(opts.RequestDelay):
			}
		}

		u := flixpatrol.Top10URL(target.Provider, target.CountrySlug, date)
		j.log.Info("flixpatrol scraping", slog.String("provider", string(target.Provider)), slog.String("country", target.CountrySlug), slog.String("url", u))
		opts.runLog("info", "[%d/%d] Scraping %s / %s: %s", i+1, len(targets), target.Provider, target.CountrySlug, u)
		start := time.Now()
		top10, err := flixpatrol.ScrapeTop10(ctx, j.fetcher, u, opts.UserAgent, opts.RespectRobots)
		if err != nil {
			opts.runLog("error", "[%d/%d] Scrape failed after %s: %v", i+1, len(targets), time.Since(start).Round(time.Millisecond), err)
			return fmt.Errorf("scrape %s: %w", u, err)
		}
		j.log.Debug("flixpatrol top 10 parsed", slog.String("url", u), slog.Int("movies", len(top10.Movies)), slog.Int("tv_shows", len(top10.TVShows)))
		opts.runLog("info", "[%d/%d] Parsed %d movies and %d TV shows for %s in %s", i+1, len(targets), len(top10.Movies), len(top10.TVShows), top10.Date.Format("2006-01-02"), time.Since(start).Round(time.Millisecond))

		if err := j.persistTop10(ctx, top10, opts); err != nil {
			opts.runLog("error", "[%d/%d] Saving failed: %v", i+1, len(targets), err)
			return err
		}
		j.log.Info("flixpatrol scraped", slog.String("provider", string(target.Provider)), slog.String("country", target.CountrySlug), slog.String("date", top10.Date.Format("2006-01-02")))
		opts.runLog("info", "[%d/%d] Saved %s / %s for %s", i+1, len(targets), target.Provider, target.CountrySlug, top10.Date.Format("2006-01-02"))
	}
	return nil
}

func (j *FlixPatrolJob) persistTop10(ctx context.Context, top10 *flixpatrol.Top10, opts FlixPatrolRunOptions) error {
	countryCode, err := flixpatrol.GetCountryCode(top10.CountrySlug)
	if err != nil {
		return err
	}

	if err := j.persistEntries(ctx, top10.Provider, top10.Date, string(countryCode), "movies", top10.Movies, opts); err != nil {
		return err
	}
	if err := j.persistEntries(ctx, top10.Provider, top10.Date, string(countryCode), "tv_shows", top10.TVShows, opts); err != nil {
		return err
	}
	return nil
}

func (j *FlixPatrolJob) persistEntries(ctx context.Context, provider flixpatrol.Provider, rankedOn time.Time, country string, category string, entries []flixpatrol.Entry, opts FlixPatrolRunOptions) error {
	var known, mapped, unmapped int
	defer func() {
		opts.runLog("info", "%s %s: %d ranks saved (%d already mapped, %d newly mapped, %d without TMDB match)",
			country, category, known+mapped+unmapped, known, mapped, unmapped)
	}()
	for _, e := range entries {
		// check if the title already exists
		existing, err := j.repo.GetTitleBySlug(ctx, e.Slug)
		if err != nil {
			return fmt.Errorf("get title %s: %w", e.Slug, err)
		}
		// if the title already exists and has both TMDB ID and IMDb ID and Rotten Tomatoes URL, upsert the ranking
		if existing != nil && existing.TmdbID.Valid && existing.TmdbID.String != "" && existing.ImdbID.Valid && existing.ImdbID.String != "" && existing.RtUrl.Valid && existing.RtUrl.String != "" {
			_, err = j.repo.UpsertRanking(ctx, repository.UpsertRankingInput{
				TitleID:           existing.ID,
				RankedOn:          rankedOn,
				Country:           country,
				StreamingProvider: string(provider),
				Category:          category,
				Rank:              int64(e.Rank),
			})
			if err != nil {
				return fmt.Errorf("upsert ranking %s rank %d: %w", e.Slug, e.Rank, err)
			}
			known++
			continue
		}

		// get the existing title IDs from the database. kind is what the title
		// is (and so which TMDB namespace tmdb_id is in), which can differ
		// from the chart it appears on: e.g. a stand-up special listed in the
		// TV chart is a movie on TMDB.
		var (
			tmdbID string
			imdbID string
			rtURL  string
			kind   = e.TitleKind
		)
		if existing != nil {
			kind = flixpatrol.TitleKind(existing.Kind)
			if existing.TmdbID.Valid && existing.TmdbID.String != "" {
				tmdbID = existing.TmdbID.String
			}
			if existing.ImdbID.Valid && existing.ImdbID.String != "" {
				imdbID = existing.ImdbID.String
			}
			if existing.RtUrl.Valid && existing.RtUrl.String != "" {
				rtURL = existing.RtUrl.String
			}
		}

		var match TitleMatch
		if tmdbID == "" {
			m, err := j.MatchTitle(ctx, e, provider, country, opts)
			if err != nil {
				return err
			}
			match = m
			kind = m.Kind
			tmdbID = m.TmdbID
			if m.ImdbID != "" {
				imdbID = m.ImdbID
			}
		}

		// if tmdbID is found, get the external IDs
		if tmdbID != "" && j.tmdb != nil && j.tmdb.Enabled() {
			tmdbIDInt, err := strconv.Atoi(tmdbID)
			var wikidataID string
			var tempImdbID string
			if err != nil {
				j.log.Debug("tmdb mapping failed by conversion", slog.String("name", e.Name), slog.Any("err", err))
			} else {
				switch kind {
				case flixpatrol.TitleKindMovie:
					tempImdbID, wikidataID, _ = j.tmdb.MovieExternalIDs(ctx, tmdbIDInt)
				case flixpatrol.TitleKindTVShow:
					tempImdbID, wikidataID, _ = j.tmdb.TVExternalIDs(ctx, tmdbIDInt)
				default:
					err = fmt.Errorf("invalid title kind: %s", kind)
				}
			}
			if tempImdbID != "" {
				imdbID = tempImdbID
			}

			// if wikidataID is found, use it to get rtURL
			if wikidataID != "" && j.wikidata != nil && j.wikidata.Enabled() && rtURL == "" {
				wdURL, err := j.wikidata.GetRottenTomatoesID(ctx, wikidataID, client.WikidataTitleKind(kind))
				if err != nil {
					j.log.Debug("wikidata mapping failed by get rotten tomatoes id", slog.String("name", e.Name), slog.Any("err", err))
				} else {
					rtURL = wdURL
				}
			}
		}

		// Wikidata has no RT ID for many recent titles; search RT itself.
		if rtURL == "" && tmdbID != "" {
			rtURL = j.FindRTSlug(ctx, e, kind, tmdbID, match)
			if rtURL != "" {
				opts.runLog("info", "#%d %s: Rotten Tomatoes slug %s found by search", e.Rank, e.Slug, rtURL)
			}
		}
		switch {
		case existing != nil && existing.TmdbID.Valid && existing.TmdbID.String != "":
			known++
		case tmdbID != "":
			mapped++
		default:
			unmapped++
		}

		title, err := j.repo.UpsertTitle(ctx, repository.UpsertTitleInput{
			Slug:   e.Slug,
			Name:   e.Name,
			Kind:   string(kind),
			TmdbID: tmdbID,
			ImdbID: imdbID,
			RtURL:  rtURL,
		})
		if err != nil {
			return fmt.Errorf("upsert title %s: %w", e.Slug, err)
		}

		_, err = j.repo.UpsertRanking(ctx, repository.UpsertRankingInput{
			TitleID:           title.ID,
			RankedOn:          rankedOn,
			Country:           country,
			StreamingProvider: string(provider),
			Category:          category,
			Rank:              int64(e.Rank),
		})
		if err != nil {
			return fmt.Errorf("upsert ranking %s rank %d: %w", e.Slug, e.Rank, err)
		}
	}
	return nil
}

// TitleMatch is the JustWatch / TMDB title a FlixPatrol entry was mapped to.
type TitleMatch struct {
	// Kind is what the title is: the matched TMDB namespace, or if nothing
	// matched, the FlixPatrol title page's kind, falling back to the chart's.
	Kind   flixpatrol.TitleKind
	TmdbID string // empty if not mapped
	ImdbID string // may be empty; filled from TMDB external IDs later
	Title  string
	Year   int
	Source string // "justwatch_path", "justwatch_search" or "tmdb_search"
}

// MatchTitle finds the TMDB title for e: it reads the FlixPatrol title page
// for the full name, premiere year and kind, then tries the JustWatch URL
// lookup, JustWatch search and TMDB search in turn, accepting only a match on
// both title and year. When the title page's kind disagrees with the chart
// (e.g. a special charting as a TV show but listed as a movie), the page's
// kind is tried first and the chart's second.
func (j *FlixPatrolJob) MatchTitle(ctx context.Context, e flixpatrol.Entry, provider flixpatrol.Provider, country string, opts FlixPatrolRunOptions) (TitleMatch, error) {
	q, err := j.lookupTitleQuery(ctx, e, opts)
	if err != nil {
		return TitleMatch{}, err
	}

	for _, kind := range q.kinds(e.TitleKind) {
		var c titleCandidate
		var ok bool
		if j.justWatch != nil {
			c, ok, err = j.matchJustWatch(ctx, e, kind, q, provider, country)
			if err != nil {
				return TitleMatch{}, err
			}
		}
		if !ok && j.tmdb != nil && j.tmdb.Enabled() {
			c, ok = j.matchTMDB(ctx, kind, q)
		}
		if ok {
			j.log.Info("title mapped",
				slog.String("slug", e.Slug),
				slog.String("chart_kind", string(e.TitleKind)),
				slog.String("kind", string(kind)),
				slog.Any("query_names", q.Names),
				slog.Int("query_year", q.Year),
				slog.String("source", c.Source),
				slog.String("matched_title", c.Title),
				slog.Int("matched_year", c.Year),
				slog.String("tmdb_id", c.TmdbID),
				slog.String("imdb_id", c.ImdbID))
			opts.runLog("info", "#%d %s: mapped to %s %q (%d), TMDB %s via %s", e.Rank, e.Slug, kind, c.Title, c.Year, c.TmdbID, c.Source)
			return TitleMatch{Kind: kind, TmdbID: c.TmdbID, ImdbID: c.ImdbID, Title: c.Title, Year: c.Year, Source: c.Source}, nil
		}
	}

	kind := q.kinds(e.TitleKind)[0]
	j.log.Info("title not mapped",
		slog.String("slug", e.Slug),
		slog.String("chart_kind", string(e.TitleKind)),
		slog.String("kind", string(kind)),
		slog.Any("query_names", q.Names),
		slog.Int("query_year", q.Year))
	opts.runLog("warn", "#%d %s: no TMDB match for %q (year %d, %s); fix it in Titles", e.Rank, e.Slug, q.Names[0], q.Year, kind)
	return TitleMatch{Kind: kind}, nil
}

// lookupTitleQuery builds the name + year used to match e against JustWatch
// and TMDB. It reads the FlixPatrol title page for the full title and
// premiere year; if that page can't be fetched it falls back to the Top 10
// name and the year suffix of the slug (e.g. "roommates-2026").
func (j *FlixPatrolJob) lookupTitleQuery(ctx context.Context, e flixpatrol.Entry, opts FlixPatrolRunOptions) (titleQuery, error) {
	q := titleQuery{Names: []string{e.Name}, Year: flixpatrol.YearFromSlug(e.Slug)}
	if j.fetcher == nil {
		return q, nil
	}

	if opts.RequestDelay > 0 {
		select {
		case <-ctx.Done():
			return q, ctx.Err()
		case <-time.After(opts.RequestDelay):
		}
	}

	details, err := flixpatrol.ScrapeTitleDetails(ctx, j.fetcher, e.Slug, opts.UserAgent, opts.RespectRobots)
	if err != nil {
		if ctx.Err() != nil {
			return q, ctx.Err()
		}
		// The fetch error itself is logged by the fetcher.
		j.log.Info("flixpatrol title details unavailable, matching on top 10 name and slug year", slog.String("slug", e.Slug), slog.Int("slug_year", q.Year))
		opts.runLog("warn", "%s: FlixPatrol title page unavailable (%v); matching on the Top 10 name", e.Slug, truncateErr(err, 200))
		return q, nil
	}
	if details.Name != "" && !strings.EqualFold(details.Name, e.Name) {
		q.Names = []string{details.Name, e.Name}
	}
	if details.Year != 0 {
		q.Year = details.Year
	}
	q.Kind = details.TitleKind
	j.log.Debug("flixpatrol title details",
		slog.String("slug", e.Slug),
		slog.String("name", details.Name),
		slog.String("kind", string(details.TitleKind)),
		slog.Int("year", details.Year),
		slog.String("country", details.Country))
	return q, nil
}

// matchJustWatch tries the JustWatch URL that mirrors the FlixPatrol slug,
// then a JustWatch title search, accepting a hit only if its title and
// release year agree with q.
func (j *FlixPatrolJob) matchJustWatch(ctx context.Context, e flixpatrol.Entry, kind flixpatrol.TitleKind, q titleQuery, provider flixpatrol.Provider, country string) (titleCandidate, bool, error) {
	objectType, slugType := client.ObjectTypeMovie, "movie"
	if kind == flixpatrol.TitleKindTVShow {
		objectType, slugType = client.ObjectTypeTVShow, "tv-show"
	}

	// A JustWatch match without a TMDB ID is no better than no match; keep
	// looking so the TMDB search can have a go.
	pick := func(candidates []titleCandidate) (titleCandidate, bool) {
		c, ok := pickCandidate(q, candidates)
		return c, ok && c.TmdbID != ""
	}

	fullPath := fmt.Sprintf("/%s/%s/%s", country, slugType, e.Slug)
	byPath, err := j.justWatch.GetTitlesByPath(ctx, fullPath, country, "en")
	if err == nil {
		pathCandidate := justWatchCandidate(byPath.UrlV2.Node.MovieOrShowFragment, "justwatch_path")
		if c, ok := pick([]titleCandidate{pathCandidate}); ok {
			return c, true, nil
		}
		j.log.Debug("justwatch path lookup rejected",
			slog.String("path", fullPath),
			slog.String("jw_title", pathCandidate.Title),
			slog.Int("jw_year", pathCandidate.Year),
			slog.String("jw_tmdb_id", pathCandidate.TmdbID),
			slog.Any("query_names", q.Names),
			slog.Int("query_year", q.Year))
	} else {
		j.log.Debug("justwatch mapping failed by path lookup", slog.String("name", e.Name), slog.Any("err", err))
	}

	if err := sleepCtx(ctx, time.Second); err != nil {
		return titleCandidate{}, false, err
	}

	var candidates []titleCandidate
	search, err := j.justWatch.GetTitlesByTopSearchPopular(ctx, q.Names[0], 10, country, "en", objectType, justWatchPackages[provider])
	if err == nil {
		for _, edge := range search.PoupularTitles.Edges {
			candidates = append(candidates, justWatchCandidate(edge.Node, "justwatch_search"))
		}
	} else {
		j.log.Debug("justwatch mapping failed by top search popular", slog.String("name", e.Name), slog.Any("err", err))
	}
	c, ok := pick(candidates)
	if !ok {
		j.log.Debug("justwatch search found no match", slog.String("query", q.Names[0]), slog.String("kind", string(kind)), slog.Int("query_year", q.Year), slog.Int("results", len(candidates)))
	}
	return c, ok, sleepCtx(ctx, time.Second)
}

func justWatchCandidate(f client.MovieOrShowFragment, source string) titleCandidate {
	c := titleCandidate{Title: f.Content.Title, Source: source}
	if f.Content.OriginalReleaseYear != nil {
		c.Year = *f.Content.OriginalReleaseYear
	}
	if f.Content.ExternalIds.TmdbId != nil {
		c.TmdbID = *f.Content.ExternalIds.TmdbId
	}
	if f.Content.ExternalIds.ImdbId != nil {
		c.ImdbID = *f.Content.ExternalIds.ImdbId
	}
	return c
}

// matchTMDB searches TMDB by each known name, accepting a hit only if its
// title and release year agree with q.
func (j *FlixPatrolJob) matchTMDB(ctx context.Context, kind flixpatrol.TitleKind, q titleQuery) (titleCandidate, bool) {
	for _, name := range q.Names {
		var (
			results []client.TMDBSearchResult
			err     error
		)
		switch kind {
		case flixpatrol.TitleKindMovie:
			results, err = j.tmdb.SearchMovie(ctx, name)
		case flixpatrol.TitleKindTVShow:
			results, err = j.tmdb.SearchTV(ctx, name)
		default:
			err = fmt.Errorf("invalid title kind: %s", kind)
		}
		if err != nil {
			j.log.Debug("tmdb mapping failed by search", slog.String("name", name), slog.Any("err", err))
			continue
		}

		candidates := make([]titleCandidate, 0, len(results))
		for _, r := range results {
			candidates = append(candidates, titleCandidate{
				Title:         r.Title,
				OriginalTitle: r.OriginalTitle,
				Year:          r.Year,
				TmdbID:        strconv.Itoa(r.ID),
				Source:        "tmdb_search",
			})
		}
		if c, ok := pickCandidate(q, candidates); ok {
			return c, true
		}
		j.log.Debug("tmdb search found no match", slog.String("query", name), slog.String("kind", string(kind)), slog.Int("query_year", q.Year), slog.Int("results", len(candidates)))
	}
	return titleCandidate{}, false
}

// FindRTSlug searches Rotten Tomatoes' index the way Seerr does (title
// similarity × release-year closeness) and returns the best match's slug,
// e.g. "m/bugonia" or "tv/beef", or "" if none scores high enough. It searches
// with TMDB's title and year when available, else the JustWatch / TMDB match
// from MatchTitle, else the FlixPatrol name and slug year. Without a year it
// does not search, since same-name titles would be indistinguishable.
func (j *FlixPatrolJob) FindRTSlug(ctx context.Context, e flixpatrol.Entry, kind flixpatrol.TitleKind, tmdbID string, match TitleMatch) string {
	if j.rt == nil {
		return ""
	}
	name, year, from := e.Name, flixpatrol.YearFromSlug(e.Slug), "flixpatrol"
	if match.Title != "" && match.Year > 0 {
		name, year, from = match.Title, match.Year, match.Source
	}
	if tmdbID != "" && j.tmdb != nil && j.tmdb.Enabled() {
		tmdbKind := "movie"
		if kind == flixpatrol.TitleKindTVShow {
			tmdbKind = "tv"
		}
		n, y, err := j.tmdb.Details(ctx, tmdbKind, tmdbID)
		if err != nil {
			j.log.Debug("tmdb details failed", slog.String("slug", e.Slug), slog.Any("err", err))
		} else if n != "" && y > 0 {
			name, year, from = n, y, "tmdb"
		}
	}
	if year == 0 {
		j.log.Debug("rotten tomatoes search skipped: release year unknown", slog.String("slug", e.Slug), slog.String("name", name))
		return ""
	}

	rtKind := client.RTKindMovie
	if kind == flixpatrol.TitleKindTVShow {
		rtKind = client.RTKindTV
	}
	r, err := j.rt.Search(ctx, rtKind, name, year)
	if err != nil {
		j.log.Warn("rotten tomatoes search failed", slog.String("slug", e.Slug), slog.String("name", name), slog.Any("err", err))
		return ""
	}
	if r == nil {
		j.log.Info("rotten tomatoes slug not found", slog.String("slug", e.Slug), slog.String("name", name), slog.Int("year", year), slog.String("name_from", from))
		return ""
	}
	j.log.Info("rotten tomatoes slug found by search",
		slog.String("slug", e.Slug),
		slog.String("name", name),
		slog.Int("year", year),
		slog.String("name_from", from),
		slog.String("rt_slug", r.Slug),
		slog.String("rt_title", r.Title),
		slog.Int("rt_year", r.Year))
	return r.Slug
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// justWatchPackages lists the JustWatch package short names of each
// FlixPatrol provider, most common first; which one exists depends on the
// country.
var justWatchPackages = map[flixpatrol.Provider][]client.Package{
	flixpatrol.ProviderNetflix:       {client.PackageNetflix},
	flixpatrol.ProviderHBOMax:        {client.PackageHBOMax, client.PackageHBOMaxUNext},
	flixpatrol.ProviderDisneyPlus:    {client.PackageDisneyPlus},
	flixpatrol.ProviderAmazonPrime:   {client.PackageAmazonPrime, client.PackageAmazonPrimeVideo},
	flixpatrol.ProviderParamountPlus: {client.PackageParamountPlus, client.PackageParamountPlusPremium},
	flixpatrol.ProviderPeacock:       {client.PackagePeacock},
	flixpatrol.ProviderAppleTV:       {client.PackageAppleTVPlus},
}

// truncateErr shortens an error for run logs (fetch errors can embed HTML).
func truncateErr(err error, n int) string {
	msg := err.Error()
	if len(msg) <= n {
		return msg
	}
	return msg[:n] + "…"
}
