package service

import (
	"context"
	"errors"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestNearbyVenues(t *testing.T) {
	s, _, alice, bob := newTrackingEnv(t)
	ctx := context.Background()
	const lat, lon = 19.0, 72.8 // the phone

	add := func(name string, in VenueInput) int64 {
		t.Helper()
		in.Name = name
		in.Country, in.Timezone = new("in"), new("Asia/Kolkata")
		out, err := s.CreateVenue(ctx, alice, in)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return out["venue"].(repository.Venue).ID
	}
	// One degree of latitude is about 111 km.
	neighbour := add("Mall Neighbour", VenueInput{Latitude: tp(lat + 0.0009), Longitude: tp(lon)}) // ~100 m
	yours := add("Your Usual", VenueInput{Latitude: tp(lat - 0.0030), Longitude: tp(lon)})         // ~334 m
	add("Old Single Screen", VenueInput{Latitude: tp(lat + 0.0018), Longitude: tp(lon), Status: new("closed")})
	far := add("Across Town", VenueInput{Latitude: tp(lat + 0.18), Longitude: tp(lon)}) // ~20 km
	add("No Coordinates", VenueInput{})

	if _, _, err := s.LogWatch(ctx, alice, WatchInput{
		Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-12T18:20:00Z",
		VenueID: &yours,
	}); err != nil {
		t.Fatalf("log watch: %v", err)
	}

	names := func(out map[string]any) []string {
		var n []string
		for _, v := range out["venues"].([]NearbyVenue) {
			n = append(n, v.Name)
		}
		return n
	}

	// Default 10 km: closed, far and unlocated cinemas are left out, and the
	// one alice goes to beats its slightly nearer neighbour.
	out, err := s.NearbyVenues(ctx, alice, lat, lon, 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	got := out["venues"].([]NearbyVenue)
	if want := []string{"Your Usual", "Mall Neighbour"}; !equalStrings(names(out), want) {
		t.Fatalf("alice nearby = %v, want %v", names(out), want)
	}
	if got[0].ID != yours || got[0].YourWatches != 1 || got[0].YourLastWatch == nil {
		t.Errorf("your venue = %+v, want 1 watch and a last watch", got[0])
	}
	if d := got[1].DistanceM; d < 90 || d > 110 {
		t.Errorf("neighbour distance = %d m, want about 100", d)
	}
	if got[1].ID != neighbour || got[1].YourWatches != 0 || got[1].YourLastWatch != nil {
		t.Errorf("neighbour = %+v", got[1])
	}

	// Bob has never been there: plain distance order.
	out, err = s.NearbyVenues(ctx, bob, lat, lon, 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Mall Neighbour", "Your Usual"}; !equalStrings(names(out), want) {
		t.Errorf("bob nearby = %v, want %v", names(out), want)
	}

	// A wider radius, closed cinemas and a limit.
	out, err = s.NearbyVenues(ctx, bob, lat, lon, 25, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Mall Neighbour", "Old Single Screen", "Your Usual", "Across Town"}; !equalStrings(names(out), want) {
		t.Errorf("25 km incl. closed = %v, want %v", names(out), want)
	}
	if v := out["venues"].([]NearbyVenue); v[len(v)-1].ID != far {
		t.Errorf("last = %d, want the far cinema", v[len(v)-1].ID)
	}
	out, err = s.NearbyVenues(ctx, bob, lat, lon, 25, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(out["venues"].([]NearbyVenue)); n != 1 || out["limit"] != int64(1) {
		t.Errorf("limit 1: %d venues, limit %v", n, out["limit"])
	}
}

// Near the antimeridian the box spans every longitude, so a cinema just
// across it is still found.
func TestNearbyVenuesAcrossTheAntimeridian(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	if _, err := s.CreateVenue(ctx, alice, VenueInput{
		Name: "Date Line Cinema", Country: new("fj"), Latitude: new(-16.8), Longitude: new(-179.98),
	}); err != nil {
		t.Fatal(err)
	}
	out, err := s.NearbyVenues(ctx, alice, -16.8, 179.95, 20, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	v := out["venues"].([]NearbyVenue)
	if len(v) != 1 || v[0].DistanceM < 7000 || v[0].DistanceM > 8000 {
		t.Fatalf("venues = %+v, want the cinema about 7.4 km away", v)
	}
	if b := nearbyBox(-16.8, 179.95, 20); b.MinLon != -180 || b.MaxLon != 180 {
		t.Errorf("box = %+v, want every longitude", b)
	}
	if b := nearbyBox(19, 72.8, 10); b.MinLon <= 72.7 || b.MaxLon >= 72.9 || b.MinLat <= 18.9 || b.MaxLat >= 19.1 {
		t.Errorf("box = %+v, want about 0.1 degrees around the point", b)
	}
}

func TestNearbyVenuesValidation(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name             string
		lat, lon, radius float64
	}{
		{"lat too big", 91, 0, 0},
		{"lon too small", 0, -181, 0},
		{"radius too small", 0, 0, 0.05},
		{"radius too big", 0, 0, 101},
	} {
		var vErr *ValidationError
		if _, err := s.NearbyVenues(ctx, alice, tc.lat, tc.lon, tc.radius, 0, false); !errors.As(err, &vErr) {
			t.Errorf("%s: err = %v, want ValidationError", tc.name, err)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
