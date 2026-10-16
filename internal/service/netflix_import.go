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
	"github.com/bugsbunny-25/metareel/internal/tasks"
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
	writer    *MappingWriter
	flixRepo  *repository.FlixPatrolRepository
	schedules *repository.TaskScheduleRepository
	matcher   *TitleMatcher
	metadata  *TitleMetadataService
	seasons   *SeasonMapper
	mapping   *TitleMappingService // optional: links to FlixPatrol titles and shares mappings
	search    NetflixSearcher      // optional: Netflix IDs from Tudum's Top 10 search
	// Countries limits the per-country import: nil = the countries of the
	// FlixPatrol schedules; ["all"] = every country.
	Countries []string
}

func NewNetflixImporter(log *slog.Logger, dl *client.Downloader, repo *repository.ImportsRepository, writer *MappingWriter, flixRepo *repository.FlixPatrolRepository, schedules *repository.TaskScheduleRepository, matcher *TitleMatcher, metadata *TitleMetadataService, seasons *SeasonMapper, countries []string) *NetflixImporter {
	return &NetflixImporter{log: log, dl: dl, repo: repo, writer: writer, flixRepo: flixRepo, schedules: schedules, matcher: matcher, metadata: metadata, seasons: seasons, Countries: countries}
}

// WithMapping makes matching use and update the mapping shared with linked
// FlixPatrol titles.
func (n *NetflixImporter) WithMapping(mapping *TitleMappingService) *NetflixImporter {
	n.mapping = mapping
	return n
}

// ImportReport summarises a file import job.
type ImportReport struct {
	Files     []ImportFileResult `json:"files"`
	Matched   int                `json:"matched,omitempty"`
	Unmatched int                `json:"unmatched,omitempty"`
	Mapping   *MappingSyncReport `json:"mapping,omitempty"`
	NetflixID *NetflixIDReport   `json:"netflix_ids,omitempty"`
	Seasons   *SeasonMapReport   `json:"seasons,omitempty"`
	// Wikidata label matches dropped because the title's own Netflix ID
	// belongs to another title, and matched again.
	Contradicted int `json:"contradicted_label_matches,omitempty"`
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

	if err := n.matchTitles(ctx, nil, netflixMatchBatch, rep, logf); err != nil {
		return rep, err
	}
	if n.seasons != nil {
		sr, err := n.seasons.Refresh(ctx, time.Time{}, runLog)
		rep.Seasons = sr
		if err != nil {
			return rep, err
		}
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

// Netflix titles matched per run: by the import (after the files) and by
// the netflix.titles.match job, which works through the backlog.
const (
	netflixMatchBatch    = 400
	netflixMatchJobBatch = 1500
)

// MatchReport summarises a netflix.titles.match run.
type MatchReport struct {
	Requested int                `json:"requested,omitempty"` // title_ids given
	Matched   int                `json:"matched"`
	Unmatched int                `json:"unmatched"`
	Mapping   *MappingSyncReport `json:"mapping,omitempty"`
	NetflixID *NetflixIDReport   `json:"netflix_ids,omitempty"`
	Seasons   *SeasonMapReport   `json:"seasons,omitempty"`
	// Wikidata label matches the title's own Netflix ID contradicted, matched again.
	Contradicted int `json:"contradicted_label_matches,omitempty"`
}

// RunMatch matches the given Netflix titles (ignoring their backoff; manual
// ones are skipped), or the due backlog when ids is empty, then remaps
// seasons.
func (n *NetflixImporter) RunMatch(ctx context.Context, ids []int64, runLog func(level, msg string)) (*MatchReport, error) {
	logf := func(level, format string, args ...any) {
		if runLog != nil {
			runLog(level, fmt.Sprintf(format, args...))
		}
	}
	rep := &ImportReport{}
	err := n.matchTitles(ctx, ids, netflixMatchJobBatch, rep, logf)
	if err == nil && len(ids) == 0 {
		// Backlog: Netflix IDs of titles matched earlier (fixing name matches).
		session := n.writer.Session(tasks.TypeNetflixTitlesMatch)
		_, err = n.resolveNetflixIDs(ctx, session, nil, netflixIDBacklog, rep.NetflixID, logf)
		session.Flush(ctx)
	}
	out := &MatchReport{Requested: len(ids), Matched: rep.Matched, Unmatched: rep.Unmatched, Mapping: rep.Mapping, NetflixID: rep.NetflixID, Contradicted: rep.Contradicted}
	if err != nil {
		return out, err
	}
	if n.seasons != nil {
		sr, err := n.seasons.Refresh(ctx, time.Time{}, runLog)
		out.Seasons = sr
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

// matchTitles matches Netflix titles to TMDB (ids, or up to limit due
// titles). Links to FlixPatrol titles are refreshed first, so titles linked
// to a matched FlixPatrol title take its mapping (TMDB kind included);
// the rest are matched through a matched FlixPatrol title of the same name
// in the same Netflix list, else JustWatch's US Netflix catalogue (either
// TMDB kind: Netflix lists specials as TV). New matches are then shared with
// linked FlixPatrol titles.
func (n *NetflixImporter) matchTitles(ctx context.Context, ids []int64, limit int64, rep *ImportReport, logf func(level, format string, args ...any)) error {
	runLog := func(level, msg string) { logf(level, "%s", msg) }
	if n.mapping != nil {
		mr, err := n.mapping.Sync(ctx, true, runLog)
		rep.Mapping = mr
		if err != nil {
			return err
		}
	}
	session := n.writer.Session(tasks.TypeNetflixTitlesMatch)
	defer session.Flush(ctx)
	// Label matches the title's own Netflix ID contradicts are matched again.
	if err := n.requeueContradicted(ctx, session, ids, rep, logf); err != nil {
		return err
	}
	var (
		due []repository.NetflixTitleDue
		err error
	)
	if len(ids) > 0 {
		due, err = n.repo.NetflixTitlesForMatch(ctx, ids)
	} else {
		due, err = n.repo.NetflixTitlesDueForMatch(ctx, limit)
	}
	if err != nil {
		return err
	}
	if len(due) > 0 {
		logf("info", "Matching %d Netflix title(s)", len(due))
	}
	// Netflix IDs first: an exact key, where Tudum's search knows the title.
	if rep.NetflixID == nil {
		rep.NetflixID = &NetflixIDReport{}
	}
	dueIDs := make([]int64, 0, len(due))
	for _, nt := range due {
		dueIDs = append(dueIDs, nt.ID)
	}
	byNetflixID, err := n.resolveNetflixIDs(ctx, session, dueIDs, netflixIDDueLimit, rep.NetflixID, logf)
	if err != nil {
		return err
	}
	rep.Matched += len(byNetflixID)
	// The pass may have found Netflix IDs: label matches must agree with them.
	if len(dueIDs) > 0 {
		if due, err = n.repo.NetflixTitlesForMatch(ctx, dueIDs); err != nil {
			return err
		}
	}
	wiki := n.wikidataNetflixHits(ctx, due, logf)
	jwCalls := 0
	for _, nt := range due {
		if err := ctx.Err(); err != nil {
			return err
		}
		if nt.MatchStatus == repository.MatchStatusManual || byNetflixID[nt.ID] {
			continue
		}
		m := repository.NetflixMatch{RetryAfter: retryAfter(nt.MatchAttempts + 1)}
		fp, err := n.repo.FlixPatrolTitlesNamed(ctx, nt.ShowTitle, nt.Kind)
		if err != nil {
			return err
		}
		var ids repository.TmdbIDs
		if ref, title, ok := singleTmdb(fp); ok {
			m.TmdbKind, m.TmdbID, m.TitleID, m.Source = ref.Kind, ref.ID, title.ID, MatchSourceFlixPatrolTitle
		} else if h, ok := pickWikidataNetflix(wiki[nt.ShowTitle], nt.Kind, weekYear(nt.FirstWeek), nt.NetflixID.String); ok {
			m.TmdbKind, m.TmdbID, m.ImdbID, m.Source = h.TmdbKind, h.TmdbID, h.IMDbID, "wikidata_netflix"
			ids.NetflixID, ids.WikidataID = h.NetflixID, h.WikidataID
		} else if n.matcher != nil && n.matcher.justWatch != nil {
			if jwCalls > 0 {
				if err := sleepCtx(ctx, time.Second); err != nil {
					return err
				}
			}
			jwCalls++
			if c, kind, ok := n.matchNetflixJustWatch(ctx, nt.ShowTitle, nt.Kind, weekYear(nt.FirstWeek)); ok {
				m.TmdbKind, m.TmdbID, m.ImdbID, m.Source = kind, c.TmdbID, c.ImdbID, c.Source
				ids.JustWatchID = c.JustWatchID
			}
		}
		if err := session.MatchNetflixTitle(ctx, nt.ID, m); err != nil {
			return err
		}
		if m.TmdbID == "" {
			rep.Unmatched++
			continue
		}
		rep.Matched++
		if m.TmdbKind != tmdbKindOf(flixpatrol.TitleKind(nt.Kind)) {
			logf("info", "%q (Netflix %s list) matched a TMDB %s: %s via %s", nt.ShowTitle, netflixListName(nt.Kind), m.TmdbKind, m.TmdbID, m.Source)
		}
		// IDs the match came with; TMDB details follow in the mapping refresh.
		if err := n.metadata.SaveMatchedIDs(ctx, repository.TmdbRef{Kind: m.TmdbKind, ID: m.TmdbID}, ids); err != nil {
			return err
		}
	}
	if len(due) > 0 {
		logf("info", "Netflix titles: %d matched, %d not matched (retried with backoff)", rep.Matched, rep.Unmatched)
	}
	return nil
}

// contradictedLimit caps the contradicted label matches redone per run.
const contradictedLimit = 200

// requeueContradicted clears Wikidata label matches (wikidata_netflix) the
// title's own Netflix ID contradicts, so they are matched again (ids: only
// these titles; nil = any).
func (n *NetflixImporter) requeueContradicted(ctx context.Context, session *MappingSession, ids []int64, rep *ImportReport, logf func(level, format string, args ...any)) error {
	if n.mapping == nil {
		return nil
	}
	found, err := n.mapping.repo.ContradictedNetflixLabelMatches(ctx, contradictedLimit)
	if err != nil {
		return err
	}
	only := map[int64]bool{}
	for _, id := range ids {
		only[id] = true
	}
	for _, id := range found {
		if len(ids) > 0 && !only[id] {
			continue
		}
		if err := session.RequeueNetflixTitle(ctx, id, true); err != nil {
			return err
		}
		rep.Contradicted++
	}
	if rep.Contradicted > 0 {
		logf("info", "%d Netflix title(s) matched by Wikidata label to a title with another Netflix ID: matching them again", rep.Contradicted)
	}
	return nil
}

// wikidataNetflixHits looks the due titles' names up on Wikidata (items
// with a Netflix ID) in batches; a failure only skips this source.
func (n *NetflixImporter) wikidataNetflixHits(ctx context.Context, due []repository.NetflixTitleDue, logf func(level, format string, args ...any)) map[string][]client.WikidataNetflixHit {
	if n.metadata == nil {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for _, nt := range due {
		if nt.MatchStatus != repository.MatchStatusManual && !seen[nt.ShowTitle] {
			seen[nt.ShowTitle] = true
			names = append(names, nt.ShowTitle)
		}
	}
	hits, err := n.metadata.FindNetflixOnWikidata(ctx, names)
	if err != nil {
		n.log.Warn("wikidata netflix lookup failed", slog.Any("err", err))
		logf("warn", "Wikidata lookup of Netflix titles failed (%s); matching without it", truncateErr(err, 200))
	}
	return hits
}

// pickWikidataNetflix picks a Wikidata hit for a Netflix title of listKind
// (movie | tv_show) that first charted in chartYear: the only TMDB title of
// the list's kind (or among several, the only recent release); else the
// only one of the other kind if it is a recent release (a special in the TV
// list), so a same-name older title of the other kind is not taken. A title
// with its own Netflix ID (ownNetflixID, from Tudum) only takes a hit with
// that ID: a hit with another one is a different title of the same name
// (e.g. the 2010 film for the 2025 remake).
func pickWikidataNetflix(hits []client.WikidataNetflixHit, listKind string, chartYear int, ownNetflixID string) (client.WikidataNetflixHit, bool) {
	if ownNetflixID != "" {
		var agree []client.WikidataNetflixHit
		for _, h := range hits {
			if h.NetflixID == ownNetflixID {
				agree = append(agree, h)
			}
		}
		hits = agree
	}
	want := tmdbKindOf(flixpatrol.TitleKind(listKind))
	byRef := func(kind string) []client.WikidataNetflixHit {
		seen := map[string]bool{}
		var out []client.WikidataNetflixHit
		for _, h := range hits {
			if h.TmdbKind == kind && !seen[h.TmdbID] {
				seen[h.TmdbID] = true
				out = append(out, h)
			}
		}
		return out
	}
	recent := func(hs []client.WikidataNetflixHit) []client.WikidataNetflixHit {
		var out []client.WikidataNetflixHit
		for _, h := range hs {
			if chartYear > 0 && (h.Year == chartYear || h.Year == chartYear-1) {
				out = append(out, h)
			}
		}
		return out
	}
	same := byRef(want)
	if len(same) == 1 {
		return same[0], true
	}
	if len(same) > 1 {
		if r := recent(same); len(r) == 1 {
			return r[0], true
		}
		return client.WikidataNetflixHit{}, false
	}
	other := "movie"
	if want == "movie" {
		other = "tv"
	}
	if r := recent(byRef(other)); len(r) == 1 && len(byRef(other)) == 1 {
		return r[0], true
	}
	return client.WikidataNetflixHit{}, false
}

func netflixListName(kind string) string {
	if kind == "tv_show" {
		return "TV"
	}
	return "Films"
}

type repositoryTitle = sqlc.Title

func flixpatrolCountryCode(slug string) (string, error) {
	code, err := flixpatrol.GetCountryCode(slug)
	return string(code), err
}

// singleTmdb returns the TMDB title (kind and ID) shared by all titles, if
// there is exactly one.
func singleTmdb(titles []repositoryTitle) (repository.TmdbRef, repositoryTitle, bool) {
	var ref repository.TmdbRef
	var first repositoryTitle
	for i, t := range titles {
		r := repository.TmdbRef{Kind: tmdbKindOf(flixpatrol.TitleKind(t.Kind)), ID: t.TmdbID.String}
		if i == 0 {
			ref, first = r, t
			continue
		}
		if r != ref {
			return repository.TmdbRef{}, first, false
		}
	}
	return ref, first, ref.ID != ""
}

// matchNetflixJustWatch searches JustWatch's US Netflix catalogue for an
// exact name, as the kind of Netflix's list and then the other kind (Netflix
// lists stand-up specials and some documentaries as TV, TMDB as movies);
// failing that (licensed titles that are not on Netflix in the US), the
// whole US catalogue for the list's kind. Netflix gives no year, so there a
// single exact-name hit is accepted, or among several the only one released
// in the year the title first charted or the year before (a new release).
// It returns the candidate and its TMDB kind (movie | tv).
func (n *NetflixImporter) matchNetflixJustWatch(ctx context.Context, name, listKind string, chartYear int) (titleCandidate, string, bool) {
	want := normalizeTitle(name)
	exact := func(kind string, packages []client.Package, source string) []titleCandidate {
		objectType := client.ObjectTypeMovie
		if kind == "tv" {
			objectType = client.ObjectTypeTVShow
		}
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
	listTmdbKind := tmdbKindOf(flixpatrol.TitleKind(listKind))
	otherKind := "tv"
	if listTmdbKind == "tv" {
		otherKind = "movie"
	}
	if hits := exact(listTmdbKind, justWatchPackages["netflix"], "justwatch_netflix"); len(hits) > 0 {
		return hits[0], listTmdbKind, true
	}
	if err := sleepCtx(ctx, time.Second); err != nil {
		return titleCandidate{}, "", false
	}
	if hits := exact(otherKind, justWatchPackages["netflix"], "justwatch_netflix_other_kind"); len(hits) == 1 {
		return hits[0], otherKind, true
	}
	if err := sleepCtx(ctx, time.Second); err != nil {
		return titleCandidate{}, "", false
	}
	hits := exact(listTmdbKind, nil, "justwatch_unique_name")
	if len(hits) == 1 {
		return hits[0], listTmdbKind, true
	}
	c, ok := pickRecentRelease(hits, chartYear)
	return c, listTmdbKind, ok
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
