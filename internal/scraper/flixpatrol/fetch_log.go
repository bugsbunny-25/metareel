package flixpatrol

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// LoggingFetcher logs every page fetch with the fetcher used, how long it took
// and the page <title>, which makes a challenge page or a layout change easy
// to spot. Successes log at debug, failures at warn.
type LoggingFetcher struct {
	Next Fetcher
	Via  string // e.g. "colly" or "flaresolverr"
	Log  *slog.Logger
}

func (f LoggingFetcher) Fetch(ctx context.Context, pageURL string, userAgent string, respectRobots bool) (*goquery.Document, error) {
	start := time.Now()
	doc, err := f.Next.Fetch(ctx, pageURL, userAgent, respectRobots)
	attrs := []any{
		slog.String("url", pageURL),
		slog.String("via", f.Via),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if err != nil {
		f.Log.Warn("flixpatrol fetch failed", append(attrs, slog.Any("err", err))...)
		return nil, err
	}
	f.Log.Debug("flixpatrol fetch ok", append(attrs, slog.String("page_title", strings.TrimSpace(doc.Find("title").First().Text())))...)
	return doc, nil
}
