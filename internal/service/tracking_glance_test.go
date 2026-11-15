package service

import (
	"context"
	"testing"
	"time"
)

func TestGlance(t *testing.T) {
	s, _, alice, bob, _ := newTrackingEnvDB(t)
	ctx := context.Background()

	g, err := s.Glance(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.UpNext) != 0 || g.NextAiring != nil || g.Subscription != nil || g.LastWatch != nil || g.GeneratedAt == "" {
		t.Fatalf("empty glance = %+v", g)
	}

	mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)})
	price := int64(2299)
	start := time.Now().UTC().AddDate(0, -2, 0).Format(time.DateOnly)
	if _, err := s.CreateSubscription(ctx, alice, SubscriptionInput{Plan: "A-List", Currency: "usd", Period: "month",
		StartedOn: start, PriceMinor: &price}); err != nil {
		t.Fatal(err)
	}

	g, err = s.Glance(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	if g.LastWatch == nil || g.LastWatch.TmdbID != "27205" || g.LastWatch.Kind != "movie" {
		t.Fatalf("last watch = %+v", g.LastWatch)
	}
	if g.Subscription == nil || g.Subscription.Plan != "A-List" || g.Subscription.Month != time.Now().UTC().Format("2006-01") {
		t.Fatalf("subscription = %+v", g.Subscription)
	}

	// Another user sees none of it.
	if g, err := s.Glance(ctx, bob); err != nil || g.LastWatch != nil || g.Subscription != nil {
		t.Fatalf("bob's glance = %+v, %v", g, err)
	}
}
