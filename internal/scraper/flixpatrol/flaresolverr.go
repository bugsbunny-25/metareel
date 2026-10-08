package flixpatrol

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/bugsbunny-25/metareel/internal/client"
)

// FlareSolverrFetcher fetches pages through FlareSolverr. FlareSolverr uses
// its own browser User-Agent and does not consult robots.txt, so userAgent and
// respectRobots are ignored.
type FlareSolverrFetcher struct {
	Client *client.FlareSolverrClient
	Log    *slog.Logger // optional
}

func (f FlareSolverrFetcher) Fetch(ctx context.Context, pageURL string, _ string, _ bool) (*goquery.Document, error) {
	page, err := f.Client.Get(ctx, pageURL)
	if err != nil {
		return nil, err
	}
	if f.Log != nil {
		f.Log.Debug("flaresolverr response",
			slog.String("url", pageURL),
			slog.String("final_url", page.URL),
			slog.Int("status", page.Status),
			slog.String("content_type", page.ContentType),
			slog.Int("bytes", len(page.Body)),
			slog.String("message", page.Message),
			slog.Int64("solve_ms", page.SolveTime.Milliseconds()),
			slog.String("session", page.Session))
	}

	doc, err := parseHTMLResponse(page.Status, page.ContentType, page.Body)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(strings.TrimSpace(doc.Find("title").First().Text()), "Just a moment...") {
		return nil, errors.New("flaresolverr returned an unsolved cloudflare challenge page (message=" + page.Message + ")")
	}
	return doc, nil
}
