package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/xlsx"
)

// Netflix publishes a "What We Watched" engagement report every six months
// (yearly from 2027) as an .xlsx linked from about.netflix.com: hours viewed,
// runtime and views of every title watched for 50k+ hours, with the premiere
// date of Netflix originals. There is no feed, so the reports to import are
// a list in app_settings (netflix.engagement.urls), seeded with the known
// ones; a run with a url adds it.
const engagementURLsSetting = "netflix.engagement.urls"

// defaultEngagementReports are the reports known when this was written.
var defaultEngagementReports = []string{
	"https://assets.ctfassets.net/4cd45et68cgf/2vdDPGLKA0XX2cjF2APJFn/7f1c367b39ed73a6a588751d3c5d0252/What_We_Watched_A_Netflix_Engagement_Report_2025Jul-Dec__6_.xlsx",
	"https://assets.ctfassets.net/4cd45et68cgf/40WGcHJa9vRua31kU6Gbz5/43b15c7fbd6924392fc80e885839e39f/Netflix-s_What_We_Watched_Report_2026Jan-Jun__1_.xlsx",
}

const maxEngagementFile = 50 << 20

// NetflixEngagementImporter imports engagement reports (the
// netflix.engagement.import job).
type NetflixEngagementImporter struct {
	log     *slog.Logger
	dl      *client.Downloader
	imports *repository.ImportsRepository
	repo    *repository.EngagementRepository
	ops     *repository.OpsRepository
}

func NewNetflixEngagementImporter(log *slog.Logger, dl *client.Downloader, imports *repository.ImportsRepository, repo *repository.EngagementRepository, ops *repository.OpsRepository) *NetflixEngagementImporter {
	return &NetflixEngagementImporter{log: log, dl: dl, imports: imports, repo: repo, ops: ops}
}

// EngagementReport summarises an engagement import.
type EngagementReport struct {
	Files   []ImportFileResult            `json:"files"`
	Periods []repository.EngagementPeriod `json:"periods"`
	// Originals: TMDB titles marked as Netflix originals (with a premiere
	// date), through linked Netflix Top 10 titles or by name and year.
	OriginalsLinked int64 `json:"originals_linked"`
	OriginalsByName int   `json:"originals_by_name"`
}

// ValidEngagementURL checks a report URL given by hand.
func ValidEngagementURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || len(raw) > 2000 {
		return "", &ValidationError{Message: "url must be an https URL of a Netflix engagement report (.xlsx)"}
	}
	return raw, nil
}

// reportURLs returns the configured report list (the defaults when unset),
// adding extra when given.
func (n *NetflixEngagementImporter) reportURLs(ctx context.Context, extra string) ([]string, error) {
	raw, err := n.ops.Setting(ctx, engagementURLsSetting)
	if err != nil {
		return nil, err
	}
	urls := defaultEngagementReports
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &urls); err != nil {
			return nil, fmt.Errorf("bad %s setting: %w", engagementURLsSetting, err)
		}
	}
	if extra != "" {
		for _, u := range urls {
			if u == extra {
				return urls, nil
			}
		}
		urls = append(append([]string{}, urls...), extra)
		b, _ := json.Marshal(urls)
		if err := n.ops.SetSetting(ctx, engagementURLsSetting, string(b)); err != nil {
			return nil, err
		}
	}
	return urls, nil
}

func engagementSource(u string) string {
	h := sha256.Sum256([]byte(u))
	return "netflix.engagement:" + hex.EncodeToString(h[:4])
}

// Run imports every listed report that changed since its last import (all
// with force); extraURL (optional) is added to the list first.
func (n *NetflixEngagementImporter) Run(ctx context.Context, force bool, extraURL string, runLog func(level, msg string)) (*EngagementReport, error) {
	logf := func(level, format string, args ...any) {
		if runLog != nil {
			runLog(level, fmt.Sprintf(format, args...))
		}
	}
	if extraURL != "" {
		var err error
		if extraURL, err = ValidEngagementURL(extraURL); err != nil {
			return nil, err
		}
	}
	urls, err := n.reportURLs(ctx, extraURL)
	if err != nil {
		return nil, err
	}
	names, err := n.repo.NetflixNames(ctx)
	if err != nil {
		return nil, err
	}
	link := newEngagementLinker(names)
	rep := &EngagementReport{}
	var failed []string
	for _, u := range urls {
		var period string
		res := importFile(ctx, n.dl, n.imports, engagementSource(u), u, force, func(r io.Reader) (int, int, error) {
			data, err := io.ReadAll(io.LimitReader(r, maxEngagementFile+1))
			if err != nil {
				return 0, 0, fmt.Errorf("read report: %w", err)
			}
			if len(data) > maxEngagementFile {
				return 0, 0, fmt.Errorf("report larger than %d MB", maxEngagementFile>>20)
			}
			parsed, err := parseEngagementReport(data)
			if err != nil {
				return 0, 0, err
			}
			for i := range parsed.Rows {
				link.link(&parsed.Rows[i])
			}
			period = parsed.Start + " to " + parsed.End
			if err := n.repo.SavePeriod(ctx, parsed.Start, parsed.End, parsed.Rows); err != nil {
				return 0, 0, err
			}
			return len(parsed.Rows), parsed.Skipped, nil
		})
		rep.Files = append(rep.Files, res)
		switch res.Status {
		case "imported":
			logf("info", "Engagement report %s: %d titles (%d rows skipped)", period, res.Rows, res.Skipped)
		case "unchanged":
			logf("info", "Engagement report %s: unchanged", res.Source)
		default:
			logf("error", "Engagement report %s: %s", res.Source, res.Error)
			failed = append(failed, res.Source)
		}
	}
	if rep.Periods, err = n.repo.Periods(ctx); err != nil {
		return rep, err
	}
	if err := n.markOriginals(ctx, rep); err != nil {
		return rep, err
	}
	logf("info", "Netflix originals: %d TMDB titles through Netflix Top 10 titles, %d more by name and year", rep.OriginalsLinked, rep.OriginalsByName)
	for _, p := range rep.Periods {
		logf("info", "Period %s to %s: %d titles, %d linked to a Netflix Top 10 title", p.PeriodStart, p.PeriodEnd, p.Titles, p.Linked)
	}
	if len(failed) > 0 {
		return rep, fmt.Errorf("engagement import failed: %v", failed)
	}
	return rep, nil
}

// markOriginals records on TMDB titles the earliest Netflix premiere the
// reports give (Netflix originals only): from rows linked to Netflix Top 10
// titles, then from unlinked rows whose name (without a season suffix)
// matches exactly one stored TMDB title of the kind with a fitting year: a
// film within a year of its premiere, a show starting no later than the
// premiered season (within a year for a first season).
func (n *NetflixEngagementImporter) markOriginals(ctx context.Context, rep *EngagementReport) error {
	linked, err := n.repo.ResetNetflixPremieres(ctx)
	if err != nil {
		return err
	}
	rep.OriginalsLinked = linked
	originals, err := n.repo.UnlinkedOriginals(ctx)
	if err != nil || len(originals) == 0 {
		return err
	}
	titles, err := n.repo.TmdbTitlesWithoutPremiere(ctx)
	if err != nil {
		return err
	}
	byName := map[string][]repository.TmdbName{}
	for _, t := range titles {
		for _, name := range []string{t.Title, t.OriginalTitle} {
			if k := normalizeTitle(name); k != "" {
				key := t.Ref.Kind + ":" + k
				if l := byName[key]; len(l) == 0 || l[len(l)-1].Ref != t.Ref {
					byName[key] = append(l, t)
				}
			}
		}
	}
	marked := map[repository.TmdbRef]bool{}
	for _, o := range originals {
		kind := "movie"
		if o.Kind == "tv_show" {
			kind = "tv"
		}
		name, _, _ := strings.Cut(o.Title, " // ")
		laterSeason := false
		if kind == "tv" {
			if m := seasonNumberRE.FindStringSubmatch(name); m != nil && m[1] != "1" {
				laterSeason = true
			}
			name = chartNameSuffix.ReplaceAllString(name, "")
		}
		premYear := dateYear(o.ReleaseDate)
		var match *repository.TmdbName
		unique := true
		for i, c := range byName[kind+":"+normalizeTitle(name)] {
			y := dateYear(c.ReleaseDate)
			ok := y > 0 && premYear > 0 && (absInt(y-premYear) <= 1 || (laterSeason && y <= premYear))
			if !ok {
				continue
			}
			if match != nil && match.Ref != c.Ref {
				unique = false
				break
			}
			match = &byName[kind+":"+normalizeTitle(name)][i]
		}
		if match == nil || !unique {
			continue
		}
		if err := n.repo.SetNetflixPremiere(ctx, match.Ref, o.ReleaseDate, "name"); err != nil {
			return err
		}
		if !marked[match.Ref] {
			marked[match.Ref] = true
			rep.OriginalsByName++
		}
	}
	return nil
}

var seasonNumberRE = regexp.MustCompile(`(?i)\bseason\s+(\d+)`)

// parsedEngagement is one report.
type parsedEngagement struct {
	Start, End string
	Rows       []repository.EngagementRow
	Skipped    int
}

var engagementPeriodRE = regexp.MustCompile(`(?i)from\s+([a-z]+)\s+(?:(\d{4})\s+)?(?:to|through|-)\s+([a-z]+)\s+(\d{4})`)

// engagementPeriod reads "Views from January to June 2026" into its first
// and last day.
func engagementPeriod(text string) (string, string, bool) {
	m := engagementPeriodRE.FindStringSubmatch(text)
	if m == nil {
		return "", "", false
	}
	month := func(s string) (time.Month, bool) {
		t, err := time.Parse("January", strings.ToUpper(s[:1])+strings.ToLower(s[1:]))
		return t.Month(), err == nil
	}
	m1, ok1 := month(m[1])
	m2, ok2 := month(m[3])
	y2, err := strconv.Atoi(m[4])
	if !ok1 || !ok2 || err != nil {
		return "", "", false
	}
	y1 := y2
	if m[2] != "" {
		y1, _ = strconv.Atoi(m[2])
	} else if m1 > m2 {
		y1 = y2 - 1
	}
	start := time.Date(y1, m1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(y2, m2+1, 0, 0, 0, 0, 0, time.UTC)
	return start.Format(repository.DateLayout), end.Format(repository.DateLayout), true
}

// parseEngagementReport reads the report's sheets (one per kind: "Shows",
// "Movies"), finding the period in the text above the header row and the
// columns by their names.
func parseEngagementReport(data []byte) (*parsedEngagement, error) {
	sheets, err := xlsx.Read(data)
	if err != nil {
		return nil, err
	}
	out := &parsedEngagement{}
	for _, sh := range sheets {
		name := strings.ToLower(sh.Name)
		kind := ""
		switch {
		case strings.Contains(name, "movie") || strings.Contains(name, "film"):
			kind = "movie"
		case strings.Contains(name, "show") || strings.Contains(name, "tv") || strings.Contains(name, "series"):
			kind = "tv_show"
		default:
			continue // a sheet without a kind (the 2023 reports' single sheet)
		}
		header, cols := -1, map[string]int{}
		for i, row := range sh.Rows {
			if out.Start == "" {
				for _, c := range row {
					if s, e, ok := engagementPeriod(c); ok {
						out.Start, out.End = s, e
					}
				}
			}
			for j, c := range row {
				cols[strings.ToLower(strings.TrimSpace(c))] = j
			}
			if _, ok := cols["title"]; ok {
				if _, ok := cols["hours viewed"]; ok {
					header = i
					break
				}
			}
			cols = map[string]int{}
		}
		if header < 0 {
			return nil, fmt.Errorf("sheet %q: no header row with Title and Hours Viewed", sh.Name)
		}
		cell := func(row []string, col string) string {
			if j, ok := cols[col]; ok && j < len(row) {
				return strings.TrimSpace(row[j])
			}
			return ""
		}
		for _, row := range sh.Rows[header+1:] {
			title := cell(row, "title")
			if title == "" {
				continue
			}
			hours := parseCount(cell(row, "hours viewed"))
			if hours == nil {
				out.Skipped++
				continue
			}
			e := repository.EngagementRow{Kind: kind, Title: title, HoursViewed: hours,
				Views: parseCount(cell(row, "views")), RuntimeMinutes: parseRuntime(cell(row, "runtime")),
				ReleaseDate: parseReportDate(cell(row, "release date"))}
			switch strings.ToLower(cell(row, "available globally?")) {
			case "yes":
				v := true
				e.AvailableGlobally = &v
			case "no":
				v := false
				e.AvailableGlobally = &v
			}
			out.Rows = append(out.Rows, e)
		}
	}
	if out.Start == "" {
		return nil, fmt.Errorf("no report period found (expected a line like \"Views from January to June 2026\")")
	}
	if len(out.Rows) == 0 {
		return nil, fmt.Errorf("no titles found")
	}
	return out, nil
}

// parseCount reads a whole number written plainly or in scientific notation.
func parseCount(s string) *int64 {
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 || math.IsInf(f, 0) {
		return nil
	}
	v := int64(math.Round(f))
	return &v
}

// parseRuntime reads "4:22" (hours:minutes) or a fraction of a day (Excel
// time) into minutes.
func parseRuntime(s string) *int64 {
	if s == "" {
		return nil
	}
	if h, m, ok := strings.Cut(s, ":"); ok {
		hh, err1 := strconv.Atoi(h)
		mm, err2 := strconv.Atoi(strings.SplitN(m, ":", 2)[0])
		if err1 != nil || err2 != nil {
			return nil
		}
		v := int64(hh*60 + mm)
		return &v
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return nil
	}
	v := int64(math.Round(f * 24 * 60))
	return &v
}

// parseReportDate reads YYYY-MM-DD or an Excel serial day number.
func parseReportDate(s string) string {
	if s == "" {
		return ""
	}
	if t, err := time.Parse(repository.DateLayout, s[:min(len(s), 10)]); err == nil {
		return t.Format(repository.DateLayout)
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 20000 && f < 80000 {
		return time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(f)).Format(repository.DateLayout)
	}
	return ""
}

// engagementLinker links report rows to Netflix Top 10 titles by name: TV
// rows to a season title (with its number) or a show title, films to a film
// title; ambiguous names are left unlinked.
type engagementLinker struct {
	byName map[string][]repository.NetflixName
}

func newEngagementLinker(names []repository.NetflixName) *engagementLinker {
	l := &engagementLinker{byName: map[string][]repository.NetflixName{}}
	for _, n := range names {
		key := n.Kind + ":" + normalizeTitle(n.Name)
		l.byName[key] = append(l.byName[key], n)
	}
	return l
}

func (l *engagementLinker) link(e *repository.EngagementRow) {
	// Non-English titles are "English // Original".
	name, _, _ := strings.Cut(e.Title, " // ")
	hits := l.byName[e.Kind+":"+normalizeTitle(name)]
	var pick *repository.NetflixName
	for i := range hits {
		h := &hits[i]
		switch {
		case pick == nil:
			pick = h
		case h.NetflixTitleID != pick.NetflixTitleID:
			// Prefer a season title over a same-named show title, else
			// the name is ambiguous.
			if h.IsSeason == pick.IsSeason {
				return
			}
			if h.IsSeason {
				pick = h
			}
		case h.IsSeason && !pick.IsSeason:
			pick = h
		}
	}
	if pick == nil {
		return
	}
	id := pick.NetflixTitleID
	e.NetflixTitleID = &id
	if pick.IsSeason && pick.SeasonNumber > 0 {
		s := pick.SeasonNumber
		e.SeasonNumber = &s
	}
}
