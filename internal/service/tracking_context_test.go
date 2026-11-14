package service

import (
	"context"
	"errors"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestParseAspectRatio(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want float64
		bad  bool
	}{
		{in: "1.90:1", want: 1.9},
		{in: "2.39:1", want: 2.39},
		{in: "1.43:1", want: 1.43},
		{in: "16:9", want: 1.78},
		{in: "4:3", want: 1.33},
		{in: "1.9", want: 1.9},
		{in: " 2.35 : 1 ", want: 2.35},
		{in: "", bad: true},
		{in: "wide", bad: true},
		{in: "16:0", bad: true},
		{in: "-2:1", bad: true},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseAspectRatio(tc.in)
			if tc.bad {
				if err == nil {
					t.Fatalf("parsed %q as %v, want an error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse %q: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("parse %q = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestToMetres(t *testing.T) {
	ft := 70.0
	got := toMetres(&ft, "ft")
	if got == nil || *got != 21.336 {
		t.Errorf("70 ft = %v, want 21.336 m", got)
	}
	m := 21.336
	if got := toMetres(&m, "m"); got == nil || *got != 21.336 {
		t.Errorf("metres should pass through, got %v", got)
	}
	if toMetres(nil, "ft") != nil {
		t.Error("nil should stay nil")
	}
}

// makeVenue adds a cinema with a screen and returns both ids.
func makeVenue(t *testing.T, s *TrackingService, userID int64) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	out, err := s.CreateVenue(ctx, userID, VenueInput{
		Name: "PVR Phoenix", Chain: tp("PVR INOX"), City: tp("Mumbai"), Country: tp("in"),
		Latitude: tp(19.0995), Longitude: tp(72.9156), Timezone: tp("Asia/Kolkata"),
		Refs: map[string]string{"wikidata_id": "Q123456"},
	})
	if err != nil {
		t.Fatalf("create venue: %v", err)
	}
	venue := out["venue"].(repository.Venue)
	screen, err := s.CreateScreen(ctx, venue.ID, ScreenInput{
		Name: "Audi 1 IMAX", Number: tp(int64(1)), Seats: tp(int64(400)),
		Width: tp(70.0), Height: tp(52.0), DimUnit: tp("ft"), AspectRatio: tp("1.43:1"),
	})
	if err != nil {
		t.Fatalf("create screen: %v", err)
	}
	return venue.ID, screen.ID
}

func TestCreateVenueNormalizesAndStoresRefs(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	venueID, screenID := makeVenue(t, s, alice)

	out, err := s.GetVenue(ctx, venueID)
	if err != nil {
		t.Fatalf("get venue: %v", err)
	}
	venue := out["venue"].(repository.Venue)
	if venue.Country == nil || *venue.Country != "IN" {
		t.Errorf("country = %v, want IN uppercased", venue.Country)
	}
	if venue.Status != "open" {
		t.Errorf("status = %q, want open", venue.Status)
	}
	refs := out["refs"].([]repository.VenueRef)
	if len(refs) != 1 || refs[0].Source != "wikidata_id" {
		t.Errorf("refs = %+v", refs)
	}

	screens := out["screens"].([]repository.VenueScreen)
	if len(screens) != 1 {
		t.Fatalf("screens = %d, want 1", len(screens))
	}
	sc := screens[0]
	// 70 ft is 21.336 m; the ratio is stored as a number so screens compare.
	if sc.WidthM == nil || *sc.WidthM != 21.336 {
		t.Errorf("width_m = %v, want 21.336", sc.WidthM)
	}
	if sc.AspectRatio == nil || *sc.AspectRatio != 1.43 {
		t.Errorf("aspect_ratio = %v, want 1.43", sc.AspectRatio)
	}
	if sc.ID != screenID {
		t.Errorf("screen id = %d, want %d", sc.ID, screenID)
	}
}

// A cinema already stored under the same outside id is not added twice.
func TestCreateVenueRefusesADuplicateRef(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	makeVenue(t, s, alice)
	_, err := s.CreateVenue(context.Background(), alice, VenueInput{
		Name: "PVR Phoenix (again)", Refs: map[string]string{"wikidata_id": "Q123456"}})
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want a ConflictError", err)
	}
}

func TestVenueValidation(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		in   VenueInput
	}{
		{"no name", VenueInput{}},
		{"bad country", VenueInput{Name: "X", Country: tp("India")}},
		{"latitude without longitude", VenueInput{Name: "X", Latitude: tp(19.0)}},
		{"latitude out of range", VenueInput{Name: "X", Latitude: tp(119.0), Longitude: tp(72.0)}},
		{"bad timezone", VenueInput{Name: "X", Timezone: tp("Mars/Olympus")}},
		{"bad status", VenueInput{Name: "X", Status: tp("demolished")}},
		{"bad date", VenueInput{Name: "X", OpenedOn: tp("1997")}},
		{"unknown ref source", VenueInput{Name: "X", Refs: map[string]string{"imdb": "tt1"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.CreateVenue(ctx, alice, tc.in)
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want a ValidationError", err)
			}
		})
	}
}

// A watch at a cinema snapshots the venue and takes its time zone, so a
// later rename cannot rewrite the diary and the local day is right.
func TestWatchAtAVenueSnapshotsItAndTakesItsZone(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	venueID, screenID := makeVenue(t, s, alice)

	e, _, err := s.LogWatch(ctx, alice, WatchInput{
		Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-12T18:20:00Z",
		Formats: []string{"imax", "imax-70mm", "dolby-atmos"},
		WatchContextInput: WatchContextInput{
			VenueID: &venueID, ScreenID: &screenID, Seat: tp("H14"),
			ScreeningType: tp("regular"), PriceMinor: tp(int64(45000)), Currency: tp("inr"),
			TicketsCount: tp(int64(2)), ContentAspectRatio: tp("1.43:1"), Dimension: tp("2d"),
		},
	})
	if err != nil {
		t.Fatalf("log watch: %v", err)
	}
	if e.VenueNameAtWatch == nil || *e.VenueNameAtWatch != "PVR Phoenix" {
		t.Errorf("venue_name_at_watch = %v", e.VenueNameAtWatch)
	}
	if e.VenueCityAtWatch == nil || *e.VenueCityAtWatch != "Mumbai" {
		t.Errorf("venue_city_at_watch = %v", e.VenueCityAtWatch)
	}
	// No tz was sent, so the cinema's own applies: +330 in Kolkata.
	if e.Tz == nil || *e.Tz != "Asia/Kolkata" {
		t.Errorf("tz = %v, want the venue's", e.Tz)
	}
	if e.UTCOffsetMinutes == nil || *e.UTCOffsetMinutes != 330 {
		t.Errorf("utc_offset_minutes = %v, want 330", derefInt(e.UTCOffsetMinutes))
	}
	if e.Currency == nil || *e.Currency != "INR" {
		t.Errorf("currency = %v, want INR uppercased", e.Currency)
	}
	if e.ContentAspectRatio == nil || *e.ContentAspectRatio != 1.43 {
		t.Errorf("content_aspect_ratio = %v, want 1.43", e.ContentAspectRatio)
	}
	// The screen's geometry fills in what the watch did not carry.
	if e.ScreenAspectRatio == nil || *e.ScreenAspectRatio != 1.43 {
		t.Errorf("screen_aspect_ratio = %v, want the screen's 1.43", e.ScreenAspectRatio)
	}
	if e.ScreenWidthM == nil || *e.ScreenWidthM != 21.336 {
		t.Errorf("screen_width_m = %v, want the screen's 21.336", e.ScreenWidthM)
	}
	if len(e.Formats) != 3 {
		t.Fatalf("formats = %+v, want three", e.Formats)
	}

	// Renaming the cinema does not touch the watch.
	if _, err := s.UpdateVenue(ctx, venueID, VenueInput{Name: "Phoenix Palladium", City: tp("Mumbai")}); err != nil {
		t.Fatalf("rename venue: %v", err)
	}
	again, err := s.GetWatch(ctx, alice, e.ID)
	if err != nil {
		t.Fatalf("get watch: %v", err)
	}
	if again.VenueNameAtWatch == nil || *again.VenueNameAtWatch != "PVR Phoenix" {
		t.Errorf("after the rename the watch says %v, want the name on the night", again.VenueNameAtWatch)
	}
}

func TestWatchContextValidation(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	_, screenID := makeVenue(t, s, alice)
	other, err := s.CreateVenue(ctx, alice, VenueInput{Name: "Other cinema"})
	if err != nil {
		t.Fatalf("create other venue: %v", err)
	}
	otherID := other["venue"].(repository.Venue).ID

	base := WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-12T18:20:00Z"}
	for _, tc := range []struct {
		name string
		ctxt WatchContextInput
		fmts []string
	}{
		{name: "unknown screening type", ctxt: WatchContextInput{ScreeningType: tp("sneak")}},
		{name: "unknown media", ctxt: WatchContextInput{Media: tp("laserdisc")}},
		{name: "unknown hdr", ctxt: WatchContextInput{HDR: tp("hdr11")}},
		{name: "price without currency", ctxt: WatchContextInput{PriceMinor: tp(int64(450))}},
		{name: "bad currency", ctxt: WatchContextInput{PriceMinor: tp(int64(450)), Currency: tp("rupees")}},
		{name: "negative price", ctxt: WatchContextInput{PriceMinor: tp(int64(-1)), Currency: tp("INR")}},
		{name: "zero tickets", ctxt: WatchContextInput{TicketsCount: tp(int64(0))}},
		{name: "bad aspect ratio", ctxt: WatchContextInput{ContentAspectRatio: tp("very wide")}},
		{name: "screen size without a unit", ctxt: WatchContextInput{ScreenWidth: tp(21.0)}},
		{name: "bad unit", ctxt: WatchContextInput{ScreenWidth: tp(21.0), ScreenDimUnit: tp("cubits")}},
		{name: "no such venue", ctxt: WatchContextInput{VenueID: tp(int64(99999))}},
		{name: "screen without a venue", ctxt: WatchContextInput{ScreenID: &screenID}},
		{name: "screen of another venue", ctxt: WatchContextInput{VenueID: &otherID, ScreenID: &screenID}},
		{name: "unknown format", fmts: []string{"imax-42d"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			in.WatchContextInput = tc.ctxt
			in.Formats = tc.fmts
			_, _, err := s.LogWatch(ctx, alice, in)
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want a ValidationError", err)
			}
		})
	}
}

// A retired format cannot be put on a new watch, but stays on the ones that
// already carry it.
func TestRetiredFormatsStayOnOldWatches(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()

	e, _, err := s.LogWatch(ctx, alice, WatchInput{
		Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-12T18:20:00Z", Formats: []string{"4dx"}})
	if err != nil {
		t.Fatalf("log watch: %v", err)
	}

	f, err := s.write.FormatBySlug(ctx, "4dx")
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if _, err := s.UpdateFormat(ctx, f.ID, FormatInput{Active: tp(false)}); err != nil {
		t.Fatalf("retire: %v", err)
	}

	_, _, err = s.LogWatch(ctx, alice, WatchInput{
		Kind: "movie", TmdbID: "155", EndedAt: "2026-11-12T18:20:00Z", Formats: []string{"4dx"}})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("logging with a retired format: %v, want a ValidationError", err)
	}

	again, err := s.GetWatch(ctx, alice, e.ID)
	if err != nil {
		t.Fatalf("get watch: %v", err)
	}
	if len(again.Formats) != 1 || again.Formats[0].Slug != "4dx" {
		t.Errorf("the old watch lost its retired format: %+v", again.Formats)
	}

	// The user-facing list leaves it out; the admin list keeps it.
	out, err := s.ListFormats(ctx, true)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	for _, f := range out["formats"].([]repository.WatchFormat) {
		if f.Slug == "4dx" {
			t.Error("a retired format is still offered")
		}
	}
	out, err = s.ListFormats(ctx, false)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	found := false
	for _, f := range out["formats"].([]repository.WatchFormat) {
		if f.Slug == "4dx" {
			found = true
		}
	}
	if !found {
		t.Error("the admin list should still show a retired format")
	}
}

func TestCreateFormatValidation(t *testing.T) {
	s, _, _, _ := newTrackingEnv(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		in   FormatInput
	}{
		{"bad slug", FormatInput{Slug: "IMAX 70mm", Label: "x", Category: "brand"}},
		{"no label", FormatInput{Slug: "x-y", Category: "brand"}},
		{"bad category", FormatInput{Slug: "x-y", Label: "x", Category: "smell"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.CreateFormat(ctx, tc.in)
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want a ValidationError", err)
			}
		})
	}
	if _, err := s.CreateFormat(ctx, FormatInput{Slug: "imax", Label: "IMAX", Category: "brand"}); err == nil {
		t.Error("a duplicate slug was accepted")
	}
}

// A venue watches point at is closed rather than deleted, so the diary keeps
// its meaning.
func TestDeleteVenueRefusedWhileInUse(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	venueID, _ := makeVenue(t, s, alice)

	if _, _, err := s.LogWatch(ctx, alice, WatchInput{
		Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-12T18:20:00Z",
		WatchContextInput: WatchContextInput{VenueID: &venueID}}); err != nil {
		t.Fatalf("log watch: %v", err)
	}

	err := s.DeleteVenue(ctx, venueID)
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want a ConflictError", err)
	}

	// An unused one goes.
	out, err := s.CreateVenue(ctx, alice, VenueInput{Name: "Unused cinema"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.DeleteVenue(ctx, out["venue"].(repository.Venue).ID); err != nil {
		t.Errorf("delete an unused venue: %v", err)
	}
}

// Former names are kept so an old watch can still be explained.
func TestVenueAliases(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	venueID, _ := makeVenue(t, s, alice)

	alias, err := s.AddVenueAlias(ctx, venueID, "Phoenix Mills Cinema", tp("2008-01-01"), tp("2019-06-30"))
	if err != nil {
		t.Fatalf("add alias: %v", err)
	}
	out, err := s.GetVenue(ctx, venueID)
	if err != nil {
		t.Fatalf("get venue: %v", err)
	}
	if got := out["aliases"].([]repository.VenueAlias); len(got) != 1 || got[0].Name != "Phoenix Mills Cinema" {
		t.Fatalf("aliases = %+v", got)
	}
	if err := s.DeleteVenueAlias(ctx, venueID, alias.ID); err != nil {
		t.Fatalf("delete alias: %v", err)
	}
	if _, err := s.AddVenueAlias(ctx, venueID, "", nil, nil); err == nil {
		t.Error("an empty alias was accepted")
	}
}
