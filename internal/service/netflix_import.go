package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// Netflix's official Top 10 downloads (https://www.netflix.com/tudum/top10).
const (
	netflixGlobalURL      = "https://www.netflix.com/tudum/top10/data/all-weeks-global.tsv"
	netflixCountriesURL   = "https://www.netflix.com/tudum/top10/data/all-weeks-countries.tsv"
	netflixMostPopularURL = "https://www.netflix.com/tudum/top10/data/most-popular.tsv"
)

// NetflixImporter imports Netflix's official weekly Top 10 (the
// netflix.top10.import job) and matches its titles to TMDB.
type NetflixImporter struct {
	log       *slog.Logger
	dl        *client.Downloader
	repo      *repository.ImportsRepository
	flixRepo  *repository.FlixPatrolRepository
	schedules *repository.TaskScheduleRepository
	matcher   *TitleMatcher
	metadata  *TitleMetadataService
	// Countries limits the per-country import: nil = the countries of the
	// FlixPatrol schedules; ["all"] = every country.
	Countries []string
}

func NewNetflixImporter(log *slog.Logger, dl *client.Downloader, repo *repository.ImportsRepository, flixRepo *repository.FlixPatrolRepository, schedules *repository.TaskScheduleRepository, matcher *TitleMatcher, metadata *TitleMetadataService, countries []string) *NetflixImporter {
	return &NetflixImporter{log: log, dl: dl, repo: repo, flixRepo: flixRepo, schedules: schedules, matcher: matcher, metadata: metadata, Countries: countries}
}

// ImportReport summarises a file import job.
type ImportReport struct {
	Files     []ImportFileResult `json:"files"`
	Matched   int                `json:"matched,omitempty"`
	Unmatched int                `json:"unmatched,omitempty"`
}

// ImportFileResult is one file of an import.
type ImportFileResult struct {
	Source  string `json:"source"`
	Status  string `json:"status"` // imported | unchanged | failed
	Rows    int    `json:"rows,omitempty"`
	Skipped int    `json:"skipped_rows,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (r *ImportReport) failed() int {
	n := 0
	for _, f := range r.Files {
		if f.Status == "failed" {
			n++
		}
	}
	return n
}

// Run imports the three Netflix files (each skipped when unchanged unless
// force) and matches new Netflix titles.
func (n *NetflixImporter) Run(ctx context.Context, force bool, runLog func(level, msg string)) (*ImportReport, error) {
	logf := func(level, format string, args ...any) {
		if runLog != nil {
			runLog(level, fmt.Sprintf(format, args...))
		}
	}
	rep := &ImportReport{}

	countries, err := n.countryFilter(ctx)
	if err != nil {
		return rep, err
	}
	files := []struct {
		source, url string
		parse       func(io.Reader) (int, int, error)
	}{
		{"netflix.global", netflixGlobalURL, func(r io.Reader) (int, int, error) { return n.importGlobal(ctx, r) }},
		{"netflix.countries", netflixCountriesURL, func(r io.Reader) (int, int, error) { return n.importCountries(ctx, r, countries) }},
		{"netflix.most_popular", netflixMostPopularURL, func(r io.Reader) (int, int, error) { return n.importMostPopular(ctx, r) }},
	}
	for _, f := range files {
		res := importFile(ctx, n.dl, n.repo, f.source, f.url, force, f.parse)
		rep.Files = append(rep.Files, res)
		switch res.Status {
		case "imported":
			logf("info", "%s: imported %d rows (%d skipped)", f.source, res.Rows, res.Skipped)
		case "unchanged":
			logf("info", "%s: unchanged since the last import", f.source)
		default:
			logf("error", "%s: %s", f.source, res.Error)
		}
	}
	if len(countries) > 0 {
		keys := make([]string, 0, len(countries))
		for c := range countries {
			keys = append(keys, c)
		}
		sort.Strings(keys)
		logf("info", "Per-country rows kept for: %s", strings.Join(keys, ", "))
	}

	if err := n.matchTitles(ctx, rep, logf); err != nil {
		return rep, err
	}
	if rep.failed() > 0 {
		return rep, fmt.Errorf("%d of %d files failed", rep.failed(), len(rep.Files))
	}
	return rep, nil
}

// importFile downloads url unless its version is unchanged and hands the body
// to parse, recording the new version on success.
func importFile(ctx context.Context, dl *client.Downloader, repo *repository.ImportsRepository, source, url string, force bool, parse func(io.Reader) (int, int, error)) ImportFileResult {
	res := ImportFileResult{Source: source}
	prev, err := repo.GetImport(ctx, source)
	if err != nil {
		res.Status, res.Error = "failed", err.Error()
		return res
	}
	if force {
		prev.Version = client.FileVersion{}
	}
	d, err := dl.Open(ctx, url, prev.Version)
	if err != nil {
		res.Status, res.Error = "failed", err.Error()
		return res
	}
	if d == nil {
		res.Status = "unchanged"
		return res
	}
	defer d.Body.Close()
	rows, skipped, err := parse(d.Body)
	if err != nil {
		res.Status, res.Error = "failed", err.Error()
		return res
	}
	if err := repo.SaveImport(ctx, source, d.Version, int64(rows)); err != nil {
		res.Status, res.Error = "failed", err.Error()
		return res
	}
	res.Status, res.Rows, res.Skipped = "imported", rows, skipped
	return res
}

// countryFilter returns the countries to keep per-country rows for (nil =
// all).
func (n *NetflixImporter) countryFilter(ctx context.Context) (map[string]bool, error) {
	if len(n.Countries) == 1 && strings.EqualFold(n.Countries[0], "all") {
		return nil, nil
	}
	out := map[string]bool{}
	for _, c := range n.Countries {
		if c = strings.ToUpper(strings.TrimSpace(c)); c != "" {
			out[c] = true
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	targets, err := n.schedules.ListAllFlixPatrolTargets(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range targets {
		if code, err := flixpatrolCountryCode(t.CountrySlug); err == nil {
			out[code] = true
		}
	}
	if len(out) == 0 {
		out["US"] = true
	}
	return out, nil
}

// tsvReader yields rows of a TSV with a header line, by column name.
type tsvReader struct {
	sc     *bufio.Scanner
	cols   map[string]int
	fields []string
}

func newTSVReader(r io.Reader, required ...string) (*tsvReader, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("empty file")
	}
	t := &tsvReader{sc: sc, cols: map[string]int{}}
	for i, c := range strings.Split(strings.TrimRight(sc.Text(), "\r"), "\t") {
		t.cols[strings.TrimSpace(strings.TrimPrefix(c, "\uFEFF"))] = i
	}
	for _, c := range required {
		if _, ok := t.cols[c]; !ok {
			return nil, fmt.Errorf("missing column %q (layout changed?)", c)
		}
	}
	return t, nil
}

func (t *tsvReader) next() bool {
	for t.sc.Scan() {
		line := strings.TrimRight(t.sc.Text(), "\r")
		if line == "" {
			continue
		}
		t.fields = strings.Split(line, "\t")
		return true
	}
	return false
}

func (t *tsvReader) err() error { return t.sc.Err() }

func (t *tsvReader) str(col string) string {
	i, ok := t.cols[col]
	if !ok || i >= len(t.fields) {
		return ""
	}
	v := strings.TrimSpace(t.fields[i])
	if v == "N/A" {
		return ""
	}
	return v
}

func (t *tsvReader) int64p(col string) *int64 {
	v, err := strconv.ParseInt(t.str(col), 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

func (t *tsvReader) floatp(col string) *float64 {
	v, err := strconv.ParseFloat(t.str(col), 64)
	if err != nil {
		return nil
	}
	return &v
}

func validWeek(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

const netflixBatch = 2000

func (n *NetflixImporter) importGlobal(ctx context.Context, r io.Reader) (int, int, error) {
	t, err := newTSVReader(r, "week", "category", "weekly_rank", "show_title")
	if err != nil {
		return 0, 0, err
	}
	var batch []repository.NetflixGlobalRow
	rows, skipped := 0, 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := n.repo.SaveNetflixGlobal(ctx, batch)
		rows += len(batch)
		batch = batch[:0]
		return err
	}
	for t.next() {
		rank := t.int64p("weekly_rank")
		week, show := t.str("week"), t.str("show_title")
		if rank == nil || show == "" || !validWeek(week) {
			skipped++
			continue
		}
		batch = append(batch, repository.NetflixGlobalRow{
			Week: week, Category: t.str("category"), Rank: *rank, ShowTitle: show, SeasonTitle: t.str("season_title"),
			HoursViewed: t.int64p("weekly_hours_viewed"), RuntimeHours: t.floatp("runtime"), Views: t.int64p("weekly_views"),
			CumulativeWeeks: t.int64p("cumulative_weeks_in_top_10"),
		})
		if len(batch) >= netflixBatch {
			if err := flush(); err != nil {
				return rows, skipped, err
			}
		}
	}
	if err := t.err(); err != nil {
		return rows, skipped, err
	}
	return rows, skipped, flush()
}

func (n *NetflixImporter) importCountries(ctx context.Context, r io.Reader, keep map[string]bool) (int, int, error) {
	t, err := newTSVReader(r, "country_iso2", "week", "category", "weekly_rank", "show_title")
	if err != nil {
		return 0, 0, err
	}
	var batch []repository.NetflixCountryRow
	rows, skipped := 0, 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := n.repo.SaveNetflixCountries(ctx, batch)
		rows += len(batch)
		batch = batch[:0]
		return err
	}
	for t.next() {
		country := strings.ToUpper(t.str("country_iso2"))
		if keep != nil && !keep[country] {
			continue
		}
		rank := t.int64p("weekly_rank")
		week, show := t.str("week"), t.str("show_title")
		if rank == nil || show == "" || len(country) != 2 || !validWeek(week) {
			skipped++
			continue
		}
		batch = append(batch, repository.NetflixCountryRow{
			Country: country, Week: week, Category: t.str("category"), Rank: *rank, ShowTitle: show,
			SeasonTitle: t.str("season_title"), CumulativeWeeks: t.int64p("cumulative_weeks_in_top_10"),
		})
		if len(batch) >= netflixBatch {
			if err := flush(); err != nil {
				return rows, skipped, err
			}
		}
	}
	if err := t.err(); err != nil {
		return rows, skipped, err
	}
	return rows, skipped, flush()
}

func (n *NetflixImporter) importMostPopular(ctx context.Context, r io.Reader) (int, int, error) {
	t, err := newTSVReader(r, "category", "rank", "show_title")
	if err != nil {
		return 0, 0, err
	}
	var rows []repository.NetflixMostPopularRow
	skipped := 0
	for t.next() {
		rank := t.int64p("rank")
		if rank == nil || t.str("show_title") == "" {
			skipped++
			continue
		}
		rows = append(rows, repository.NetflixMostPopularRow{
			Category: t.str("category"), Rank: *rank, ShowTitle: t.str("show_title"), SeasonTitle: t.str("season_title"),
			HoursViewed: t.int64p("hours_viewed_first_91_days"), RuntimeHours: t.floatp("runtime"), Views: t.int64p("views_first_91_days"),
		})
	}
	if err := t.err(); err != nil {
		return 0, skipped, err
	}
	if len(rows) == 0 {
		return 0, skipped, fmt.Errorf("no rows parsed")
	}
	return len(rows), skipped, n.repo.ReplaceNetflixMostPopular(ctx, rows)
}

const netflixMatchBatch = 400

// matchTitles matches Netflix titles to TMDB: first through a FlixPatrol
// title of the same name that charted on Netflix, else a JustWatch search of
// Netflix's US catalogue (Netflix gives no year, so only exact names on
// Netflix itself are accepted).
func (n *NetflixImporter) matchTitles(ctx context.Context, rep *ImportReport, logf func(level, format string, args ...any)) error {
	due, err := n.repo.NetflixTitlesDueForMatch(ctx, netflixMatchBatch)
	if err != nil {
		return err
	}
	for i, nt := range due {
		if err := ctx.Err(); err != nil {
			return err
		}
		m := repository.NetflixMatch{RetryAfter: retryAfter(nt.MatchAttempts + 1)}
		fp, err := n.repo.FlixPatrolTitlesNamed(ctx, nt.ShowTitle, nt.Kind)
		if err != nil {
			return err
		}
		if id, title, ok := singleTmdb(fp); ok {
			m.TmdbID, m.ImdbID, m.TitleID, m.Source = id, title.ImdbID.String, title.ID, "flixpatrol_title"
		} else if n.matcher != nil && n.matcher.justWatch != nil {
			if i > 0 {
				if err := sleepCtx(ctx, time.Second); err != nil {
					return err
				}
			}
			if c, ok := n.matchNetflixJustWatch(ctx, nt.ShowTitle, nt.Kind, weekYear(nt.FirstWeek)); ok {
				m.TmdbID, m.ImdbID, m.Source = c.TmdbID, c.ImdbID, c.Source
			}
		}
		if err := n.repo.SaveNetflixTitleMatch(ctx, nt.ID, m); err != nil {
			return err
		}
		if m.TmdbID == "" {
			rep.Unmatched++
			continue
		}
		rep.Matched++
		if n.metadata != nil {
			kind := "movie"
			if nt.Kind == "tv_show" {
				kind = "tv"
			}
			if _, err := n.metadata.RefreshDetails(ctx, repository.TmdbRef{Kind: kind, ID: m.TmdbID}); err != nil {
				n.log.Warn("tmdb details for netflix title failed", slog.String("show_title", nt.ShowTitle), slog.Any("err", err))
			}
		}
	}
	if len(due) > 0 {
		logf("info", "Netflix titles: %d matched, %d not matched (retried with backoff)", rep.Matched, rep.Unmatched)
	}
	return nil
}

type repositoryTitle = sqlc.Title

func flixpatrolCountryCode(slug string) (string, error) {
	code, err := flixpatrol.GetCountryCode(slug)
	return string(code), err
}

// singleTmdb returns the TMDB ID shared by all titles, if there is exactly one.
func singleTmdb(titles []repositoryTitle) (string, repositoryTitle, bool) {
	var id string
	var first repositoryTitle
	for i, t := range titles {
		if i == 0 {
			id, first = t.TmdbID.String, t
			continue
		}
		if t.TmdbID.String != id {
			return "", first, false
		}
	}
	return id, first, id != ""
}

// matchNetflixJustWatch searches JustWatch's US Netflix catalogue for an
// exact name; failing that (licensed titles that are not on Netflix in the
// US), the whole US catalogue. Netflix gives no year, so there a single
// exact-name hit is accepted, or among several the only one released in the
// year the title first charted or the year before (a new release).
func (n *NetflixImporter) matchNetflixJustWatch(ctx context.Context, name, kind string, chartYear int) (titleCandidate, bool) {
	objectType := client.ObjectTypeMovie
	if kind == "tv_show" {
		objectType = client.ObjectTypeTVShow
	}
	want := normalizeTitle(name)
	exact := func(packages []client.Package, source string) []titleCandidate {
		search, err := n.matcher.justWatch.GetTitlesByTopSearchPopular(ctx, name, 10, "US", "en", objectType, packages)
		if err != nil {
			n.log.Debug("justwatch netflix search failed", slog.String("name", name), slog.Any("err", err))
			return nil
		}
		var out []titleCandidate
		for _, edge := range search.PoupularTitles.Edges {
			c := justWatchCandidate(edge.Node, source)
			if c.TmdbID != "" && normalizeTitle(c.Title) == want {
				out = append(out, c)
			}
		}
		return out
	}
	if hits := exact(justWatchPackages["netflix"], "justwatch_netflix"); len(hits) > 0 {
		return hits[0], true
	}
	if err := sleepCtx(ctx, time.Second); err != nil {
		return titleCandidate{}, false
	}
	hits := exact(nil, "justwatch_unique_name")
	if len(hits) == 1 {
		return hits[0], true
	}
	return pickRecentRelease(hits, chartYear)
}

// pickRecentRelease returns the only candidate released in chartYear or the
// year before, if exactly one is.
func pickRecentRelease(hits []titleCandidate, chartYear int) (titleCandidate, bool) {
	if chartYear == 0 {
		return titleCandidate{}, false
	}
	var recent []titleCandidate
	for _, h := range hits {
		if h.Year == chartYear || h.Year == chartYear-1 {
			recent = append(recent, h)
		}
	}
	if len(recent) != 1 {
		return titleCandidate{}, false
	}
	recent[0].Source = "justwatch_recent_release"
	return recent[0], true
}

func weekYear(week string) int {
	if len(week) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(week[:4])
	return y
}
