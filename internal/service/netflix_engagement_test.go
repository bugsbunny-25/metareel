package service

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestEngagementParsing(t *testing.T) {
	for in, want := range map[string][2]string{
		"Views from January to June 2026* (see footnote)": {"2026-01-01", "2026-06-30"},
		"Hours Viewed from July to December 2025":         {"2025-07-01", "2025-12-31"},
		"Views from January to December 2027":             {"2027-01-01", "2027-12-31"},
		"Views from July 2026 to June 2027":               {"2026-07-01", "2027-06-30"},
	} {
		s, e, ok := engagementPeriod(in)
		if !ok || s != want[0] || e != want[1] {
			t.Errorf("engagementPeriod(%q) = %s, %s, %v", in, s, e, ok)
		}
	}
	if _, _, ok := engagementPeriod("Netflix's What We Watched Report"); ok {
		t.Error("title line taken as a period")
	}
	if v := parseCount("454300000"); v == nil || *v != 454300000 {
		t.Errorf("count = %v", v)
	}
	if v := parseCount("4.543E8"); v == nil || *v != 454300000 {
		t.Errorf("scientific count = %v", v)
	}
	if parseCount("") != nil || parseCount("n/a") != nil {
		t.Error("bad counts parsed")
	}
	if v := parseRuntime("4:22"); v == nil || *v != 262 {
		t.Errorf("runtime = %v", v)
	}
	if v := parseRuntime("0.0756944444"); v == nil || *v != 109 { // 1:49 as a fraction of a day
		t.Errorf("fractional runtime = %v", v)
	}
	if d := parseReportDate("2026-01-08"); d != "2026-01-08" {
		t.Errorf("date = %q", d)
	}
	if d := parseReportDate("46030"); d != "2026-01-08" {
		t.Errorf("serial date = %q", d)
	}
	if parseReportDate("") != "" || parseReportDate("soon") != "" {
		t.Error("bad dates parsed")
	}
}

func TestEngagementLinker(t *testing.T) {
	l := newEngagementLinker([]repository.NetflixName{
		{NetflixTitleID: 1, Kind: "tv_show", Name: "Bridgerton"},
		{NetflixTitleID: 1, Kind: "tv_show", Name: "Bridgerton: Season 4", SeasonNumber: 4, IsSeason: true},
		{NetflixTitleID: 2, Kind: "movie", Name: "The Rip"},
		{NetflixTitleID: 3, Kind: "tv_show", Name: "Squid Game: Season 2", SeasonNumber: 2, IsSeason: true},
		{NetflixTitleID: 4, Kind: "movie", Name: "Twins"}, {NetflixTitleID: 5, Kind: "movie", Name: "Twins"},
	})
	check := func(title, kind string, wantID, wantSeason int64) {
		t.Helper()
		e := repository.EngagementRow{Title: title, Kind: kind}
		l.link(&e)
		var id, season int64
		if e.NetflixTitleID != nil {
			id = *e.NetflixTitleID
		}
		if e.SeasonNumber != nil {
			season = *e.SeasonNumber
		}
		if id != wantID || season != wantSeason {
			t.Errorf("%q (%s) -> title %d season %d, want %d / %d", title, kind, id, season, wantID, wantSeason)
		}
	}
	check("Bridgerton: Season 4", "tv_show", 1, 4)
	check("Bridgerton", "tv_show", 1, 0)
	check("The Rip", "movie", 2, 0)
	check("The Rip", "tv_show", 0, 0) // other kind
	check("Squid Game: Season 2 // 오징어 게임: 시즌 2", "tv_show", 3, 2)
	check("Twins", "movie", 0, 0) // ambiguous
	check("Unknown", "movie", 0, 0)
}

// engagementXLSX builds a report like Netflix's: a title and period above a
// header row, one sheet per kind.
func engagementXLSX(t *testing.T, period string, shows, movies [][]string) []byte {
	t.Helper()
	var strs []string
	idx := map[string]int{}
	str := func(s string) int {
		if i, ok := idx[s]; ok {
			return i
		}
		idx[s] = len(strs)
		strs = append(strs, s)
		return idx[s]
	}
	sheet := func(rows [][]string) string {
		var b strings.Builder
		b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
		all := append([][]string{{"Netflix's What We Watched Report"}, {period}, {"Title", "Available Globally?", "Release Date", "Hours Viewed", "Runtime", "Views"}}, rows...)
		for i, r := range all {
			fmt.Fprintf(&b, `<row r="%d">`, i+2)
			for j, v := range r {
				ref := fmt.Sprintf("%c%d", 'B'+j, i+2)
				if v == "" {
					continue
				}
				if i >= 3 && (j == 3 || j == 5) { // numbers
					fmt.Fprintf(&b, `<c r="%s"><v>%s</v></c>`, ref, v)
				} else {
					fmt.Fprintf(&b, `<c r="%s" t="s"><v>%d</v></c>`, ref, str(v))
				}
			}
			b.WriteString(`</row>`)
		}
		b.WriteString(`</sheetData></worksheet>`)
		return b.String()
	}
	s1, s2 := sheet(shows), sheet(movies)
	var sst strings.Builder
	sst.WriteString(`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	for _, s := range strs {
		fmt.Fprintf(&sst, `<si><t>%s</t></si>`, s)
	}
	sst.WriteString(`</sst>`)
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Shows" sheetId="1" r:id="rId1"/><sheet name="Movies" sheetId="2" r:id="rId2"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Target="worksheets/sheet2.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       sst.String(),
		"xl/worksheets/sheet1.xml":   s1,
		"xl/worksheets/sheet2.xml":   s2,
	} {
		f, _ := z.Create(name)
		_, _ = f.Write([]byte(body))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestNetflixEngagementImport(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO netflix_titles (id, show_title, kind, match_status, tmdb_kind, tmdb_id) VALUES
		(1, 'Bridgerton', 'tv_show', 'matched', 'tv', '91239'), (2, 'The Rip', 'movie', 'matched', 'movie', '1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO netflix_seasons (netflix_title_id, season_title, season_number) VALUES (1, 'Bridgerton: Season 4', 4)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tmdb_titles (tmdb_kind, tmdb_id, title, release_date) VALUES
		('tv', '91239', 'Bridgerton', '2020-12-25'), ('movie', '1', 'The Rip', '2026-01-16'),
		('movie', '50', 'Toaster', '2026-04-15'), ('tv', '60', 'Long Runner', '2022-05-01'),
		('movie', '70', 'Twins', '2026-02-01'), ('movie', '71', 'Twins', '2025-11-01'),
		('movie', '80', 'Old Licensed Film', '1999-01-01')`); err != nil {
		t.Fatal(err)
	}
	report := engagementXLSX(t, "Views from January to June 2026* (see footnote)",
		[][]string{
			{"Bridgerton: Season 4", "Yes", "2026-01-29", "889800000", "8:53", "100200000"},
			{"Some Licensed Show: Season 1", "No", "", "1200000", "5:00", "200000"},
			{"Long Runner: Season 3", "Yes", "2026-03-01", "9000000", "6:00", "1500000"}, // later season of a 2022 show
			{"Broken row", "Yes", "", "", "", ""},
		},
		[][]string{
			{"The Rip", "Yes", "2026-01-16", "256400000", "1:53", "136100000"},
			{"Toaster // टोस्टर", "Yes", "2026-04-15", "5000000", "2:00", "2500000"}, // original, only on FlixPatrol
			{"Twins", "Yes", "2026-02-01", "1000000", "1:40", "600000"},              // two TMDB films of that name and year
			{"Old Licensed Film", "No", "", "900000", "1:30", "600000"},              // not an original
		})
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write(report)
	}))
	defer srv.Close()

	ops := repository.NewOpsRepository(db)
	engagement := repository.NewEngagementRepository(db)
	imp := NewNetflixEngagementImporter(slog.New(slog.NewTextHandler(io.Discard, nil)), client.NewDownloader(time.Minute, ""),
		repository.NewImportsRepository(db), engagement, ops)
	// Only the given report (an empty list, not the defaults, in tests).
	if err := ops.SetSetting(ctx, engagementURLsSetting, "[]"); err != nil {
		t.Fatal(err)
	}
	if _, err := imp.Run(ctx, false, "http://insecure.example/x.xlsx", nil); err == nil {
		t.Fatal("http url accepted")
	}
	// httptest serves http; give the importer the URL through the setting.
	if err := ops.SetSetting(ctx, engagementURLsSetting, `["`+srv.URL+`/report.xlsx"]`); err != nil {
		t.Fatal(err)
	}
	rep, err := imp.Run(ctx, false, "", nil)
	if err != nil {
		t.Fatalf("run: %v (%+v)", err, rep)
	}
	if len(rep.Files) != 1 || rep.Files[0].Rows != 7 || rep.Files[0].Skipped != 1 || len(rep.Periods) != 1 ||
		rep.Periods[0].PeriodStart != "2026-01-01" || rep.Periods[0].PeriodEnd != "2026-06-30" || rep.Periods[0].Titles != 7 || rep.Periods[0].Linked != 2 {
		t.Fatalf("report = %+v", rep)
	}
	// Bridgerton and The Rip through their Netflix titles; Toaster and Long
	// Runner by name and year; Twins is ambiguous.
	if rep.OriginalsLinked != 2 || rep.OriginalsByName != 2 {
		t.Fatalf("originals = %d linked, %d by name", rep.OriginalsLinked, rep.OriginalsByName)
	}
	premiere := func(kind, id string) string {
		var p *string
		_ = db.QueryRow(`SELECT netflix_premiere FROM tmdb_titles WHERE tmdb_kind = ? AND tmdb_id = ?`, kind, id).Scan(&p)
		if p == nil {
			return ""
		}
		return *p
	}
	for ref, want := range map[[2]string]string{
		{"tv", "91239"}: "2026-01-29", {"movie", "1"}: "2026-01-16", {"movie", "50"}: "2026-04-15", {"tv", "60"}: "2026-03-01",
		{"movie", "70"}: "", {"movie", "71"}: "", {"movie", "80"}: "",
	} {
		if got := premiere(ref[0], ref[1]); got != want {
			t.Errorf("premiere of %v = %q, want %q", ref, got, want)
		}
	}
	m, _ := repository.NewMetadataRepository(db).Get(ctx, TmdbRef{Kind: "movie", ID: "50"})
	if o := ClassifyOrigin("netflix", true, m); o.Origin != OriginOriginal || !strings.Contains(o.Reason, "2026-04-15") {
		t.Fatalf("toaster origin = %+v", o)
	}
	rows, err := engagement.ForTmdb(ctx, TmdbRef{Kind: "tv", ID: "91239"})
	if err != nil || len(rows) != 1 {
		t.Fatalf("bridgerton rows = %+v, %v", rows, err)
	}
	b := rows[0]
	if b.SeasonNumber == nil || *b.SeasonNumber != 4 || *b.HoursViewed != 889800000 || *b.Views != 100200000 || *b.RuntimeMinutes != 533 ||
		b.ReleaseDate != "2026-01-29" || b.AvailableGlobally == nil || !*b.AvailableGlobally {
		t.Fatalf("bridgerton = %+v", b)
	}
	// Unchanged on the next run.
	if rep, _ = imp.Run(ctx, false, "", nil); rep.Files[0].Status != "unchanged" {
		t.Fatalf("second run = %+v", rep.Files)
	}
	// The title's Netflix numbers carry it.
	if _, err := db.Exec(`INSERT INTO netflix_top10_global (week, category, weekly_rank, netflix_title_id, show_title) VALUES ('2026-02-01', 'TV (English)', 1, 1, 'Bridgerton')`); err != nil {
		t.Fatal(err)
	}
	ov := NewTitleOverviewService(repository.NewAnalyticsRepository(db), repository.NewFlixPatrolRepository(db), repository.NewMetadataRepository(db), nil).WithEngagement(engagement)
	nf, err := ov.Netflix(ctx, TmdbRef{Kind: "tv", ID: "91239"})
	if err != nil || len(nf.Engagement) != 1 {
		t.Fatalf("netflix numbers = %+v, %v", nf, err)
	}
}

// TestParseRealEngagementReport parses a real report when one is given
// (METAREEL_ENGAGEMENT_XLSX=/path/to/report.xlsx).
func TestParseRealEngagementReport(t *testing.T) {
	path := os.Getenv("METAREEL_ENGAGEMENT_XLSX")
	if path == "" {
		t.Skip("METAREEL_ENGAGEMENT_XLSX not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := parseEngagementReport(data)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	withDate := 0
	for _, r := range p.Rows {
		kinds[r.Kind]++
		if r.ReleaseDate != "" {
			withDate++
		}
	}
	t.Logf("period %s..%s: %d rows (%v), %d with a release date, %d skipped; first: %+v", p.Start, p.End, len(p.Rows), kinds, withDate, p.Skipped, p.Rows[0])
}
