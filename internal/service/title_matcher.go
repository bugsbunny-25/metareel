package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// TitleMatcher maps a FlixPatrol title to TMDB / IMDb (via JustWatch and
// TMDB) and Rotten Tomatoes. It holds no state; the enrichment job and the
// fpcheck tool both use it.
type TitleMatcher struct {
	log       *slog.Logger
	fetcher   flixpatrol.Fetcher // optional: reads FlixPatrol title pages
	tmdb      *client.TMDBClient
	justWatch *client.JustWatchClient
	rt        *client.RottenTomatoesClient
}

func NewTitleMatcher(log *slog.Logger, fetcher flixpatrol.Fetcher, tmdbClient *client.TMDBClient, justWatchClient *client.JustWatchClient, rtClient *client.RottenTomatoesClient) *TitleMatcher {
	return &TitleMatcher{log: log, fetcher: fetcher, tmdb: tmdbClient, justWatch: justWatchClient, rt: rtClient}
}

// FetchOptions controls FlixPatrol page fetches.
type FetchOptions struct {
	RequestDelay  time.Duration // wait before the fetch
	UserAgent     string
	RespectRobots bool
}

// FetchDetails reads the FlixPatrol title page (full name, premiere date,
// kind, country) after waiting opts.RequestDelay.
func (m *TitleMatcher) FetchDetails(ctx context.Context, slug string, opts FetchOptions) (*flixpatrol.TitleDetails, error) {
	if m.fetcher == nil {
		return nil, fmt.Errorf("no flixpatrol fetcher configured")
	}
	if err := sleepCtx(ctx, opts.RequestDelay); err != nil {
		return nil, err
	}
	return flixpatrol.ScrapeTitleDetails(ctx, m.fetcher, slug, opts.UserAgent, opts.RespectRobots)
}

// MatchInput is what is known about a FlixPatrol title when matching it.
type MatchInput struct {
	Slug      string
	ChartName string               // name in the Top 10 table
	ChartKind flixpatrol.TitleKind // kind of the chart it appeared on
	Details   *flixpatrol.TitleDetails
	// Provider and Country of a chart the title appeared on: the JustWatch
	// lookups search that country and service first. Optional.
	Provider flixpatrol.Provider
	Country  string // ISO 3166-1 alpha-2
}

// TitleMatch is the JustWatch / TMDB title a FlixPatrol entry was mapped to.
type TitleMatch struct {
	// Kind is what the title is: the matched TMDB namespace, or if nothing
	// matched, the FlixPatrol title page's kind, falling back to the chart's.
	Kind        flixpatrol.TitleKind
	TmdbID      string // empty if not mapped
	ImdbID      string // may be empty; filled from TMDB external IDs later
	JustWatchID string
	Title       string
	Year        int
	Source      string // "justwatch_path", "justwatch_search" or "tmdb_search"
}

func (in MatchInput) query() titleQuery {
	q := titleQuery{Year: flixpatrol.YearFromSlug(in.Slug)}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		for _, n := range q.Names {
			if strings.EqualFold(n, name) {
				return
			}
		}
		q.Names = append(q.Names, name)
	}
	if in.Details != nil {
		add(in.Details.Name)
		if in.Details.Year != 0 {
			q.Year = in.Details.Year
		}
		q.Kind = in.Details.TitleKind
	}
	add(in.ChartName)
	// "Wednesday: Season 2" → also try "Wednesday".
	if base, season := flixpatrol.SeasonFromName(in.ChartName); season > 0 {
		add(base)
	}
	return q
}

// Match finds the TMDB title for in: the JustWatch URL lookup, JustWatch
// search and TMDB search in turn, accepting only a match on both title and
// year. When the title page's kind disagrees with the chart (e.g. a special
// charting as a TV show but listed as a movie), the page's kind is tried
// first and the chart's second.
func (m *TitleMatcher) Match(ctx context.Context, in MatchInput) (TitleMatch, error) {
	q := in.query()
	if in.Country == "" {
		in.Country = "US"
	}

	for _, kind := range q.kinds(in.ChartKind) {
		var c titleCandidate
		var ok bool
		var err error
		if m.justWatch != nil {
			c, ok, err = m.matchJustWatch(ctx, in, kind, q)
			if err != nil {
				return TitleMatch{}, err
			}
		}
		if !ok && m.tmdb != nil && m.tmdb.Enabled() {
			c, ok = m.matchTMDB(ctx, kind, q)
		}
		if ok {
			m.log.Info("title mapped",
				slog.String("slug", in.Slug),
				slog.String("chart_kind", string(in.ChartKind)),
				slog.String("kind", string(kind)),
				slog.Any("query_names", q.Names),
				slog.Int("query_year", q.Year),
				slog.String("source", c.Source),
				slog.String("matched_title", c.Title),
				slog.Int("matched_year", c.Year),
				slog.String("tmdb_id", c.TmdbID),
				slog.String("imdb_id", c.ImdbID))
			return TitleMatch{Kind: kind, TmdbID: c.TmdbID, ImdbID: c.ImdbID, JustWatchID: c.JustWatchID, Title: c.Title, Year: c.Year, Source: c.Source}, nil
		}
	}

	kind := q.kinds(in.ChartKind)[0]
	m.log.Info("title not mapped",
		slog.String("slug", in.Slug),
		slog.String("chart_kind", string(in.ChartKind)),
		slog.String("kind", string(kind)),
		slog.Any("query_names", q.Names),
		slog.Int("query_year", q.Year))
	return TitleMatch{Kind: kind}, nil
}

// QueryDescription is a human-readable summary of what Match searched for.
func (in MatchInput) QueryDescription() string {
	q := in.query()
	name := in.ChartName
	if len(q.Names) > 0 {
		name = q.Names[0]
	}
	return fmt.Sprintf("%q (year %d, %s)", name, q.Year, q.kinds(in.ChartKind)[0])
}

// matchJustWatch tries the JustWatch URL that mirrors the FlixPatrol slug,
// then a JustWatch title search, accepting a hit only if its title and
// release year agree with q.
func (m *TitleMatcher) matchJustWatch(ctx context.Context, in MatchInput, kind flixpatrol.TitleKind, q titleQuery) (titleCandidate, bool, error) {
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

	fullPath := fmt.Sprintf("/%s/%s/%s", in.Country, slugType, in.Slug)
	byPath, err := m.justWatch.GetTitlesByPath(ctx, fullPath, in.Country, "en")
	if err == nil {
		pathCandidate := justWatchCandidate(byPath.UrlV2.Node.MovieOrShowFragment, "justwatch_path")
		if c, ok := pick([]titleCandidate{pathCandidate}); ok {
			return c, true, nil
		}
		m.log.Debug("justwatch path lookup rejected",
			slog.String("path", fullPath),
			slog.String("jw_title", pathCandidate.Title),
			slog.Int("jw_year", pathCandidate.Year),
			slog.String("jw_tmdb_id", pathCandidate.TmdbID),
			slog.Any("query_names", q.Names),
			slog.Int("query_year", q.Year))
	} else {
		m.log.Debug("justwatch mapping failed by path lookup", slog.String("slug", in.Slug), slog.Any("err", err))
	}

	if err := sleepCtx(ctx, time.Second); err != nil {
		return titleCandidate{}, false, err
	}

	var candidates []titleCandidate
	for i, name := range q.Names {
		if i > 0 {
			if err := sleepCtx(ctx, time.Second); err != nil {
				return titleCandidate{}, false, err
			}
		}
		search, err := m.justWatch.GetTitlesByTopSearchPopular(ctx, name, 10, in.Country, "en", objectType, justWatchPackages[in.Provider])
		if err != nil {
			m.log.Debug("justwatch mapping failed by top search popular", slog.String("name", name), slog.Any("err", err))
			continue
		}
		candidates = candidates[:0]
		for _, edge := range search.PoupularTitles.Edges {
			candidates = append(candidates, justWatchCandidate(edge.Node, "justwatch_search"))
		}
		if c, ok := pick(candidates); ok {
			return c, true, sleepCtx(ctx, time.Second)
		}
		m.log.Debug("justwatch search found no match", slog.String("query", name), slog.String("kind", string(kind)), slog.Int("query_year", q.Year), slog.Int("results", len(candidates)))
	}
	return titleCandidate{}, false, sleepCtx(ctx, time.Second)
}

func justWatchCandidate(f client.MovieOrShowFragment, source string) titleCandidate {
	c := titleCandidate{Title: f.Content.Title, Source: source, JustWatchID: f.Id}
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
func (m *TitleMatcher) matchTMDB(ctx context.Context, kind flixpatrol.TitleKind, q titleQuery) (titleCandidate, bool) {
	for _, name := range q.Names {
		var (
			results []client.TMDBSearchResult
			err     error
		)
		switch kind {
		case flixpatrol.TitleKindMovie:
			results, err = m.tmdb.SearchMovie(ctx, name)
		case flixpatrol.TitleKindTVShow:
			results, err = m.tmdb.SearchTV(ctx, name)
		default:
			err = fmt.Errorf("invalid title kind: %s", kind)
		}
		if err != nil {
			m.log.Debug("tmdb mapping failed by search", slog.String("name", name), slog.Any("err", err))
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
		m.log.Debug("tmdb search found no match", slog.String("query", name), slog.String("kind", string(kind)), slog.Int("query_year", q.Year), slog.Int("results", len(candidates)))
	}
	return titleCandidate{}, false
}

// RTQuery is what FindRTSlug searches Rotten Tomatoes with.
type RTQuery struct {
	Slug   string // FlixPatrol slug (for logs and its year suffix)
	Name   string // best known title
	Year   int    // release year; 0 = unknown
	Kind   flixpatrol.TitleKind
	TmdbID string // if set and TMDB is configured, TMDB's title + year win
	From   string // where Name/Year came from, for logs
}

// FindRTSlug searches Rotten Tomatoes' index the way Seerr does (title
// similarity × release-year closeness) and returns the best match's slug,
// e.g. "m/bugonia" or "tv/beef", or "" if none scores high enough. TMDB's
// title and year are preferred when available. Without a year it does not
// search, since same-name titles would be indistinguishable.
func (m *TitleMatcher) FindRTSlug(ctx context.Context, q RTQuery) (string, error) {
	if m.rt == nil {
		return "", nil
	}
	name, year, from := q.Name, q.Year, q.From
	if year == 0 {
		year = flixpatrol.YearFromSlug(q.Slug)
	}
	if q.TmdbID != "" && m.tmdb != nil && m.tmdb.Enabled() {
		n, y, err := m.tmdb.Details(ctx, tmdbKindOf(q.Kind), q.TmdbID)
		if err != nil {
			m.log.Debug("tmdb details failed", slog.String("slug", q.Slug), slog.Any("err", err))
		} else if n != "" && y > 0 {
			name, year, from = n, y, "tmdb"
		}
	}
	if year == 0 || name == "" {
		m.log.Debug("rotten tomatoes search skipped: release year unknown", slog.String("slug", q.Slug), slog.String("name", name))
		return "", nil
	}

	rtKind := client.RTKindMovie
	if q.Kind == flixpatrol.TitleKindTVShow {
		rtKind = client.RTKindTV
	}
	r, err := m.rt.Search(ctx, rtKind, name, year)
	if err != nil {
		m.log.Warn("rotten tomatoes search failed", slog.String("slug", q.Slug), slog.String("name", name), slog.Any("err", err))
		return "", err
	}
	if r == nil {
		m.log.Info("rotten tomatoes slug not found", slog.String("slug", q.Slug), slog.String("name", name), slog.Int("year", year), slog.String("name_from", from))
		return "", nil
	}
	m.log.Info("rotten tomatoes slug found by search",
		slog.String("slug", q.Slug),
		slog.String("name", name),
		slog.Int("year", year),
		slog.String("name_from", from),
		slog.String("rt_slug", r.Slug),
		slog.String("rt_title", r.Title),
		slog.Int("rt_year", r.Year))
	return r.Slug, nil
}

// tmdbKindOf maps a title kind to TMDB's URL namespace.
func tmdbKindOf(kind flixpatrol.TitleKind) string {
	if kind == flixpatrol.TitleKindTVShow {
		return "tv"
	}
	return "movie"
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
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
