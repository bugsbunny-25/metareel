package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
)

type fakeLister struct {
	calls    int
	fail     bool
	lastKind string
	region   string
}

func (f *fakeLister) Enabled() bool { return true }
func (f *fakeLister) Trending(_ context.Context, kind, _ string) ([]client.TMDBListItem, error) {
	f.calls++
	f.lastKind = kind
	if f.fail {
		return nil, errors.New("boom")
	}
	return []client.TMDBListItem{{Kind: "movie", ID: "1", Title: "A"}}, nil
}
func (f *fakeLister) NowPlaying(_ context.Context, region string) ([]client.TMDBListItem, error) {
	f.calls++
	f.region = region
	if f.fail {
		return nil, errors.New("boom")
	}
	return []client.TMDBListItem{{Kind: "movie", ID: "2", Title: "B"}}, nil
}

func TestTitleListsCachesAndServesStale(t *testing.T) {
	f := &fakeLister{}
	s := NewTitleListsService(f)
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	ctx := context.Background()

	if _, err := s.Trending(ctx, ""); err != nil || f.calls != 1 || f.lastKind != "all" {
		t.Fatalf("first trending: err=%v calls=%d kind=%q", err, f.calls, f.lastKind)
	}
	if _, err := s.Trending(ctx, ""); err != nil || f.calls != 1 {
		t.Fatalf("cached trending refetched: calls=%d", f.calls)
	}
	now = now.Add(listTTL + time.Minute)
	f.fail = true
	got, err := s.Trending(ctx, "")
	if err != nil || len(got.Items) != 1 || f.calls != 2 {
		t.Fatalf("stale fallback: err=%v items=%v calls=%d", err, got, f.calls)
	}
	if _, err := s.InCinemas(ctx, "us"); !errors.Is(err, ErrListsUnavailable) {
		t.Fatalf("uncached failure should be unavailable, got %v", err)
	}
	f.fail = false
	if r, err := s.InCinemas(ctx, "us"); err != nil || f.region != "US" || r.Items[0].ID != "2" {
		t.Fatalf("in cinemas: err=%v region=%q", err, f.region)
	}
}

func TestTitleListsValidates(t *testing.T) {
	s := NewTitleListsService(&fakeLister{})
	var v *ValidationError
	if _, err := s.Trending(context.Background(), "person"); !errors.As(err, &v) {
		t.Fatalf("kind person: %v", err)
	}
	if _, err := s.InCinemas(context.Background(), "USA"); !errors.As(err, &v) {
		t.Fatalf("country USA: %v", err)
	}
	if _, err := NewTitleListsService(nil).Trending(context.Background(), ""); !errors.Is(err, ErrListsUnavailable) {
		t.Fatalf("no tmdb: %v", err)
	}
}
