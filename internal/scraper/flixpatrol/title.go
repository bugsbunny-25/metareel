package flixpatrol

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// TitleDetails is the metadata shown in the header of a FlixPatrol title page
// (https://flixpatrol.com/title/<slug>/). It is used to disambiguate titles
// that share a name when mapping to JustWatch / TMDB.
type TitleDetails struct {
	Slug      string
	Name      string    // full title from the page <h1>
	TitleKind TitleKind // empty if the page does not say
	Year      int       // premiere year, 0 if unknown
	Country   string    // production country as displayed, e.g. "United States"
}

var (
	yearRe     = regexp.MustCompile(`\b(1[89]\d{2}|2\d{3})\b`)
	slugYearRe = regexp.MustCompile(`-(1[89]\d{2}|2\d{3})$`)
)

func TitleURL(slug string) string {
	return fmt.Sprintf("https://flixpatrol.com/title/%s/", url.PathEscape(slug))
}

func ScrapeTitleDetails(ctx context.Context, fetcher Fetcher, slug string, userAgent string, respectRobots bool) (*TitleDetails, error) {
	if strings.TrimSpace(slug) == "" {
		return nil, fmt.Errorf("empty title slug")
	}
	doc, err := fetchDocument(ctx, fetcher, TitleURL(slug), userAgent, respectRobots)
	if err != nil {
		return nil, err
	}
	details, err := parseTitleDetails(doc)
	if err != nil {
		return nil, fmt.Errorf("scrape %s: %w", TitleURL(slug), err)
	}
	details.Slug = slug
	if details.Year == 0 {
		details.Year = YearFromSlug(slug)
	}
	return details, nil
}

// parseTitleDetails reads the title header, which looks like:
//
//	<h1>Bugonia</h1>
//	<div class="flex flex-wrap ...">
//	  <div title="163558"><div>Movie</div>|</div>
//	  <div><span class="fflag ..."></span><span>United States</span>|</div>
//	  <div title="Premiere"><span>10/24/</span>2025|</div>
//	  ...
//	</div>
func parseTitleDetails(doc *goquery.Document) (*TitleDetails, error) {
	h1 := doc.Find("h1").First()
	name := strings.TrimSpace(h1.Text())
	if name == "" {
		return nil, fmt.Errorf("missing title heading")
	}
	out := &TitleDetails{Name: name}

	meta := h1.Parent().Next()
	meta.Children().Each(func(i int, item *goquery.Selection) {
		// Drop the "|" separators before reading the text.
		item = item.Clone()
		item.Find(".select-none").Remove()
		text := strings.Join(strings.Fields(item.Text()), " ")

		switch {
		case i == 0 && strings.EqualFold(text, "Movie"):
			out.TitleKind = TitleKindMovie
		case i == 0 && strings.EqualFold(text, "TV Show"):
			out.TitleKind = TitleKindTVShow
		case item.AttrOr("title", "") == "Premiere":
			if m := yearRe.FindString(strings.ReplaceAll(text, " ", "")); m != "" {
				out.Year, _ = strconv.Atoi(m)
			}
		case item.Find(".fflag").Length() > 0 && out.Country == "":
			out.Country = text
		}
	})

	return out, nil
}

// YearFromSlug returns the year suffix FlixPatrol appends to slugs of titles
// that share a name (e.g. "roommates-2026"), or 0 if there is none.
func YearFromSlug(slug string) int {
	m := slugYearRe.FindStringSubmatch(slug)
	if m == nil {
		return 0
	}
	y, _ := strconv.Atoi(m[1])
	return y
}
