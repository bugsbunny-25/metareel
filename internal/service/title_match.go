package service

import (
	"strings"
	"unicode"

	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// titleQuery is what we know about a FlixPatrol title when mapping it to
// JustWatch / TMDB.
type titleQuery struct {
	Names []string             // full title from the FlixPatrol title page, then the Top 10 table name
	Year  int                  // premiere year, 0 if unknown
	Kind  flixpatrol.TitleKind // kind from the FlixPatrol title page, empty if unknown
}

// kinds returns the kinds to search, most likely first: the title page's
// kind, then the chart's kind if it differs.
func (q titleQuery) kinds(chartKind flixpatrol.TitleKind) []flixpatrol.TitleKind {
	if q.Kind == "" || q.Kind == chartKind {
		return []flixpatrol.TitleKind{chartKind}
	}
	return []flixpatrol.TitleKind{q.Kind, chartKind}
}

// titleCandidate is a normalized JustWatch or TMDB search hit.
type titleCandidate struct {
	Title         string
	OriginalTitle string
	Year          int
	TmdbID        string
	ImdbID        string
	Source        string // where the candidate came from, for logs
}

// maxYearDrift tolerates festival vs. streaming premiere dates landing in
// adjacent years across FlixPatrol, JustWatch and TMDB.
const maxYearDrift = 1

// pickCandidate returns the best candidate whose title matches q. When q.Year
// is known it prefers an exact year match, then one within maxYearDrift, and
// only falls back to a candidate with no year at all; candidates with a
// conflicting year are never returned. Ties keep the source's ranking order.
func pickCandidate(q titleQuery, candidates []titleCandidate) (titleCandidate, bool) {
	var near, undated *titleCandidate
	for i := range candidates {
		c := &candidates[i]
		if !q.matchesName(c) {
			continue
		}
		if q.Year == 0 {
			return *c, true
		}
		switch d := absInt(c.Year - q.Year); {
		case c.Year == 0:
			if undated == nil {
				undated = c
			}
		case d == 0:
			return *c, true
		case d <= maxYearDrift:
			if near == nil {
				near = c
			}
		}
	}
	if near != nil {
		return *near, true
	}
	if undated != nil {
		return *undated, true
	}
	return titleCandidate{}, false
}

func (q titleQuery) matchesName(c *titleCandidate) bool {
	for _, n := range q.Names {
		want := normalizeTitle(n)
		if want == "" {
			continue
		}
		if want == normalizeTitle(c.Title) || want == normalizeTitle(c.OriginalTitle) {
			return true
		}
	}
	return false
}

// normalizeTitle lowercases and drops punctuation / whitespace so that e.g.
// "Spider-Man: No Way Home" and "Spider Man No Way Home" compare equal.
func normalizeTitle(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "&", "and")
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
