package flixpatrol

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
)

type Provider string

const (
	ProviderNetflix       Provider = "netflix"
	ProviderHBOMax        Provider = "hbo-max"
	ProviderDisneyPlus    Provider = "disney"
	ProviderAmazonPrime   Provider = "amazon-prime"
	ProviderParamountPlus Provider = "paramount-plus"
	ProviderPeacock       Provider = "peacock"
	ProviderAppleTV       Provider = "apple-tv"
)

type Category string

const (
	CategoryMovies  Category = "movies"
	CategoryTVShows Category = "tv_shows"
)

type TitleKind string

const (
	TitleKindMovie  TitleKind = "movie"
	TitleKindTVShow TitleKind = "tv_show"
)

type Entry struct {
	Rank      int
	Name      string
	Slug      string // FlixPatrol title slug, e.g. "roommates-2026"
	TitleKind TitleKind
}

type Top10 struct {
	Provider    Provider
	CountrySlug string // e.g. "united-states"
	Date        time.Time
	Movies      []Entry
	TVShows     []Entry
}

// effectiveTop10DateUTC picks the calendar day (UTC) for FlixPatrol Top 10 URLs:
// before 12:00 UTC use the previous day; at or after 12:00 UTC use the current day.
func effectiveTop10DateUTC(t time.Time) time.Time {
	utc := t.UTC()
	y, m, d := utc.Date()
	todayStart := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	noon := todayStart.Add(12 * time.Hour)
	if utc.Before(noon) {
		return todayStart.AddDate(0, 0, -1)
	}
	return todayStart
}

func Top10URL(provider Provider, countrySlug string, date time.Time) string {
	d := effectiveTop10DateUTC(date)
	return fmt.Sprintf("https://flixpatrol.com/top10/%s/%s/%s/",
		provider,
		countrySlug,
		d.Format("2006-01-02"),
	)
}

func ScrapeTop10(ctx context.Context, base *colly.Collector, top10URL string, userAgent string, respectRobots bool) (*Top10, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	u, err := url.Parse(top10URL)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if !isFlixPatrolHost(u.Host) {
		return nil, fmt.Errorf("refusing to scrape non-flixpatrol host: %s", u.Host)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 {
		return nil, fmt.Errorf("unexpected top10 path: %s", u.Path)
	}
	date, err := time.Parse("2006-01-02", parts[len(parts)-1])
	if err != nil {
		return nil, fmt.Errorf("parse date: %w", err)
	}

	c := base.Clone()
	// Colly default is to respect robots.txt; we make it explicit.
	c.IgnoreRobotsTxt = !respectRobots

	var (
		out     *Top10
		scrapeE error
	)

	c.OnRequest(func(r *colly.Request) {
		if userAgent != "" {
			r.Headers.Set("User-Agent", userAgent)
		}
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	})

	c.OnResponse(func(r *colly.Response) {
		if scrapeE != nil {
			return
		}
		if r.Request != nil && strings.EqualFold(r.Request.Method, "HEAD") {
			// Base collector has CheckHead enabled; HEAD is expected to have no body.
			return
		}

		contentType := strings.ToLower(strings.TrimSpace(r.Headers.Get("Content-Type")))
		if r.StatusCode != 200 {
			scrapeE = fmt.Errorf("unexpected status %d content_type=%q body_preview=%q", r.StatusCode, contentType, previewBody(r.Body))
			return
		}
		if len(bytes.TrimSpace(r.Body)) == 0 {
			scrapeE = errors.New("empty html response body")
			return
		}
		if contentType != "" &&
			!strings.Contains(contentType, "text/html") &&
			!strings.Contains(contentType, "application/xhtml+xml") {
			scrapeE = fmt.Errorf("unexpected content type %q body_preview=%q", contentType, previewBody(r.Body))
			return
		}

		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(r.Body))
		if err != nil {
			scrapeE = fmt.Errorf("parse html: %w body_preview=%q", err, previewBody(r.Body))
			return
		}

		movies, err := parseTop10Table(doc, "TOP 10 Movies", TitleKindMovie)
		if err != nil {
			scrapeE = err
			return
		}
		tv, err := parseTop10Table(doc, "TOP 10 TV Shows", TitleKindTVShow)
		if err != nil {
			scrapeE = err
			return
		}

		out = &Top10{
			Provider:    Provider(parts[1]),
			CountrySlug: parts[2],
			Date:        date,
			Movies:      movies,
			TVShows:     tv,
		}
	})

	c.OnError(func(r *colly.Response, err error) {
		if scrapeE == nil {
			if r != nil {
				contentType := strings.ToLower(strings.TrimSpace(r.Headers.Get("Content-Type")))
				scrapeE = fmt.Errorf("request failed: %w status=%d content_type=%q body_preview=%q", err, r.StatusCode, contentType, previewBody(r.Body))
				return
			}
			scrapeE = err
		}
	})

	if err := c.Visit(top10URL); err != nil {
		return nil, fmt.Errorf("colly visit %s: %w", top10URL, err)
	}
	c.Wait()

	if scrapeE != nil {
		return nil, fmt.Errorf("scrape %s: %w", top10URL, scrapeE)
	}
	if out == nil {
		return nil, errors.New("no scrape result produced")
	}
	return out, nil
}

func isFlixPatrolHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(host)
	return host == "flixpatrol.com" || host == "www.flixpatrol.com"
}

func parseTop10Table(doc *goquery.Document, heading string, kind TitleKind) ([]Entry, error) {
	h := findHeading(doc, heading)
	if h.Length() == 0 {
		return nil, fmt.Errorf("missing section heading: %s", heading)
	}

	// Find the first table after the heading in document order.
	// FlixPatrol may place the heading inside nested wrappers where the table
	// is a sibling of one of its ancestors.
	table := findFirstTableAfter(h)
	if table == nil || table.Length() == 0 {
		return nil, fmt.Errorf("missing table after heading: %s", heading)
	}

	var out []Entry
	table.Find("tbody tr, tr").Each(func(_ int, tr *goquery.Selection) {
		cols := tr.Find("td")
		if cols.Length() < 3 {
			return
		}

		rankStr := strings.TrimSpace(cols.Eq(0).Text())
		rankStr = strings.TrimSuffix(rankStr, ".")
		rank, err := strconv.Atoi(rankStr)
		if err != nil || rank < 1 || rank > 10 {
			return
		}

		a := cols.Eq(2).Find("a").First()
		name := strings.TrimSpace(a.Text())
		href, _ := a.Attr("href")
		slug := extractTitleSlug(href)
		if name == "" || slug == "" {
			return
		}

		out = append(out, Entry{
			Rank:      rank,
			Name:      name,
			Slug:      slug,
			TitleKind: kind,
		})
	})

	if len(out) == 0 {
		return nil, errors.New("parsed 0 rows")
	}
	return out, nil
}

func findFirstTableAfter(start *goquery.Selection) *goquery.Selection {
	for cur := start; cur.Length() > 0; cur = cur.Parent() {
		for s := cur.Next(); s.Length() > 0; s = s.Next() {
			if goquery.NodeName(s) == "table" {
				return s
			}
			if nested := s.Find("table").First(); nested.Length() > 0 {
				return nested
			}
		}
	}
	return nil
}

func findHeading(doc *goquery.Document, text string) *goquery.Selection {
	return doc.Find("h1,h2,h3,h4").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return strings.EqualFold(strings.TrimSpace(s.Text()), text)
	}).First()
}

func extractTitleSlug(href string) string {
	if href == "" {
		return ""
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	p := strings.Trim(u.Path, "/")
	parts := strings.Split(p, "/")
	if len(parts) >= 2 && parts[0] == "title" {
		return parts[1]
	}
	return ""
}

func previewBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	s := strings.Join(strings.Fields(string(body)), " ")
	const maxLen = 240
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
