package flixpatrol

import (
	"context"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// SerialFetcher lets one FlixPatrol page fetch run at a time across the whole
// process (scrapes, backfills and title-page lookups run as separate jobs)
// and spaces fetch starts at least MinInterval apart, so concurrent jobs
// never hit FlixPatrol or FlareSolverr in parallel.
type SerialFetcher struct {
	Next        Fetcher
	MinInterval time.Duration

	mu   sync.Mutex
	last time.Time
}

func (f *SerialFetcher) Fetch(ctx context.Context, pageURL string, userAgent string, respectRobots bool) (*goquery.Document, error) {
	// A channel-based lock so waiting for a slow fetch respects ctx.
	if err := f.lock(ctx); err != nil {
		return nil, err
	}
	defer f.mu.Unlock()

	if wait := f.MinInterval - time.Since(f.last); !f.last.IsZero() && wait > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	defer func() { f.last = time.Now() }()
	return f.Next.Fetch(ctx, pageURL, userAgent, respectRobots)
}

func (f *SerialFetcher) lock(ctx context.Context) error {
	for {
		if f.mu.TryLock() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Health checks whether the underlying fetcher can reach FlixPatrol's
// fetch backend, if it supports a health check.
func (f *SerialFetcher) Health(ctx context.Context) error { return CheckHealth(ctx, f.Next) }

// HealthChecker is implemented by fetchers that can tell whether their
// backend (e.g. FlareSolverr) is up before a run starts.
type HealthChecker interface {
	Health(ctx context.Context) error
}

// CheckHealth runs f's health check, or returns nil if it has none.
func CheckHealth(ctx context.Context, f Fetcher) error {
	if h, ok := f.(HealthChecker); ok {
		return h.Health(ctx)
	}
	return nil
}
