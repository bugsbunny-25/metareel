package flixpatrol

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const netflixUSSamplePath = "testdata/netflix_united_states_2026-04-28.html"

var providedMovieSlugs = []string{
	"/apex-2026",
	"/roommates-2026",
	"/untold-the-shooting-at-hawthorne-hill",
	"/minions-the-rise-of-gru",
	"/kpop-demon-hunters",
	"/bugonia",
	"/thrash",
	"/snake-eyes-gi-joe-origins",
	"/jumanji-welcome-to-the-jungle",
	"/180",
}

var providedTVShowSlugs = []string{
	"/raw-1993",
	"/running-point",
	"/unchosen",
	"/funny-af-with-kevin-hart",
	"/hulk-hogan-real-american",
	"/beef",
	"/this-is-a-gardening-show",
	"/trust-me-the-false-prophet",
	"/stranger-things-tales-from-85",
	"/million-dollar-secret",
}

func TestParseTop10Table_FromNetflixUSSample(t *testing.T) {
	doc := mustLoadDocument(t, netflixUSSamplePath)
	wantMovieSlugs := normalizeProvidedSlugs(providedMovieSlugs)
	wantTVShowSlugs := normalizeProvidedSlugs(providedTVShowSlugs)

	tests := []struct {
		name      string
		heading   string
		kind      TitleKind
		wantFirst Entry
		wantLast  Entry
	}{
		{
			name:    "movies",
			heading: "TOP 10 Movies",
			kind:    TitleKindMovie,
			wantFirst: Entry{
				Rank:      1,
				Name:      "Apex",
				Slug:      "apex-2026",
				TitleKind: TitleKindMovie,
			},
			wantLast: Entry{
				Rank:      10,
				Name:      "180",
				Slug:      "180",
				TitleKind: TitleKindMovie,
			},
		},
		{
			name:    "tv_shows",
			heading: "TOP 10 TV Shows",
			kind:    TitleKindTVShow,
			wantFirst: Entry{
				Rank:      1,
				Name:      "Raw",
				Slug:      "raw-1993",
				TitleKind: TitleKindTVShow,
			},
			wantLast: Entry{
				Rank:      10,
				Name:      "Million Dollar Secret",
				Slug:      "million-dollar-secret",
				TitleKind: TitleKindTVShow,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTop10Table(doc, tt.heading, tt.kind)
			if err != nil {
				t.Fatalf("parseTop10Table() error = %v", err)
			}
			if len(got) != 10 {
				t.Fatalf("parseTop10Table() len = %d, want 10", len(got))
			}
			if got[0] != tt.wantFirst {
				t.Fatalf("first entry = %#v, want %#v", got[0], tt.wantFirst)
			}
			if got[len(got)-1] != tt.wantLast {
				t.Fatalf("last entry = %#v, want %#v", got[len(got)-1], tt.wantLast)
			}

			if tt.kind == TitleKindMovie {
				gotSlugs := collectSlugs(got)
				if !reflect.DeepEqual(gotSlugs, wantMovieSlugs) {
					t.Fatalf("movie slugs = %#v, want %#v", gotSlugs, wantMovieSlugs)
				}
			}
			if tt.kind == TitleKindTVShow {
				gotSlugs := collectSlugs(got)
				if !reflect.DeepEqual(gotSlugs, wantTVShowSlugs) {
					t.Fatalf("tv show slugs = %#v, want %#v", gotSlugs, wantTVShowSlugs)
				}
			}
		})
	}
}

func TestParseTop10Table_MissingHeading(t *testing.T) {
	doc := mustLoadDocument(t, netflixUSSamplePath)

	_, err := parseTop10Table(doc, "TOP 10 Documentaries", TitleKindMovie)
	if err == nil {
		t.Fatal("parseTop10Table() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "missing section heading") {
		t.Fatalf("error = %q, want to contain %q", err.Error(), "missing section heading")
	}
}

func TestParseTop10Table_MissingTableAfterHeading(t *testing.T) {
	doc := mustLoadHTML(t, `
		<html><body>
			<h2>TOP 10 Movies</h2>
			<div>No table here</div>
		</body></html>
	`)
	_, err := parseTop10Table(doc, "TOP 10 Movies", TitleKindMovie)
	if err == nil {
		t.Fatal("parseTop10Table() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "missing table after heading") {
		t.Fatalf("error = %q, want to contain %q", err.Error(), "missing table after heading")
	}
}

func TestParseTop10Table_IgnoresInvalidRows_ThenErrorsOnZero(t *testing.T) {
	// Every row is invalid:
	// - rank outside 1..10
	// - missing href/title slug
	// - missing required columns
	doc := mustLoadHTML(t, `
		<html><body>
			<h3>TOP 10 Movies</h3>
			<table>
				<tbody>
					<tr><td>0.</td><td>x</td><td><a href="/title/roommates-2026/">Roommates</a></td></tr>
					<tr><td>11.</td><td>x</td><td><a href="/title/roommates-2026/">Roommates</a></td></tr>
					<tr><td>1.</td><td>x</td><td><a href="/not-a-title/roommates-2026/">Roommates</a></td></tr>
					<tr><td>1.</td><td>x</td><td><a href="">Roommates</a></td></tr>
					<tr><td>1.</td><td>x</td><td><a href="/title/roommates-2026/"></a></td></tr>
					<tr><td>1.</td><td>x</td></tr>
				</tbody>
			</table>
		</body></html>
	`)

	_, err := parseTop10Table(doc, "TOP 10 Movies", TitleKindMovie)
	if err == nil {
		t.Fatal("parseTop10Table() error = nil, want non-nil")
	}
	if err.Error() != "parsed 0 rows" {
		t.Fatalf("error = %q, want %q", err.Error(), "parsed 0 rows")
	}
}

func TestExtractTitleSlug(t *testing.T) {
	tests := []struct {
		name string
		href string
		want string
	}{
		{name: "empty", href: "", want: ""},
		{name: "invalid_url", href: "http://%", want: ""},
		{name: "relative_title_path", href: "/title/roommates-2026/", want: "roommates-2026"},
		{name: "absolute_title_url", href: "https://flixpatrol.com/title/roommates-2026/", want: "roommates-2026"},
		{name: "non_title_path", href: "/top10/netflix/united-states/2026-04-28/", want: ""},
		{name: "too_short", href: "/title/", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractTitleSlug(tt.href); got != tt.want {
				t.Fatalf("extractTitleSlug(%q) = %q, want %q", tt.href, got, tt.want)
			}
		})
	}
}

func TestIsFlixPatrolHost(t *testing.T) {
	tests := []struct {
		hostport string
		want     bool
	}{
		{"flixpatrol.com", true},
		{"www.flixpatrol.com", true},
		{"FLIXPATROL.COM", true},
		{"flixpatrol.com:443", true},
		{"www.flixpatrol.com:443", true},
		{"evilflixpatrol.com", false},
		{"flixpatrol.com.evil.com", false},
		{"example.com:80", false},
	}
	for _, tt := range tests {
		t.Run(tt.hostport, func(t *testing.T) {
			if got := isFlixPatrolHost(tt.hostport); got != tt.want {
				t.Fatalf("isFlixPatrolHost(%q) = %v, want %v", tt.hostport, got, tt.want)
			}
		})
	}
}

func TestEffectiveTop10DateUTC_NoonCutover(t *testing.T) {
	// Before 12:00 UTC => previous day; at/after 12:00 UTC => current day.
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "before_noon_utc",
			in:   time.Date(2026, 4, 23, 11, 59, 59, 0, time.UTC),
			want: "2026-04-22",
		},
		{
			name: "at_noon_utc",
			in:   time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC),
			want: "2026-04-23",
		},
		{
			name: "after_noon_utc",
			in:   time.Date(2026, 4, 23, 20, 0, 0, 0, time.UTC),
			want: "2026-04-23",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveTop10DateUTC(tt.in).Format("2006-01-02")
			if got != tt.want {
				t.Fatalf("effectiveTop10DateUTC(%s) = %s, want %s", tt.in.UTC().Format(time.RFC3339), got, tt.want)
			}
		})
	}
}

func TestTop10URL_UsesEffectiveDateUTC(t *testing.T) {
	// 2026-04-23 11:00 UTC should map to 2026-04-22
	in := time.Date(2026, 4, 23, 11, 0, 0, 0, time.UTC)
	got := Top10URL(ProviderNetflix, "united-states", in)
	if !strings.HasSuffix(got, "/2026-04-22/") {
		t.Fatalf("Top10URL() = %q, want suffix %q", got, "/2026-04-22/")
	}
}

func mustLoadDocument(t *testing.T, path string) *goquery.Document {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("os.Open(%q) error = %v", path, err)
	}
	defer f.Close()

	doc, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		t.Fatalf("goquery.NewDocumentFromReader(%q) error = %v", path, err)
	}
	return doc
}

func mustLoadHTML(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("goquery.NewDocumentFromReader(html) error = %v", err)
	}
	return doc
}

func collectSlugs(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Slug)
	}
	return out
}

func normalizeProvidedSlugs(raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		out = append(out, normalizeProvidedSlug(s))
	}
	return out
}

func normalizeProvidedSlug(slug string) string {
	slug = strings.TrimSpace(slug)
	slug = strings.TrimPrefix(slug, "/")
	slug = strings.ReplaceAll(slug, "/", "-")
	return slug
}
