package service

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// fakeTitleSource stands in for TMDB.
type fakeTitleSource struct {
	enabled bool
	details map[string]client.TMDBTitleDetails
	// episodes is show id -> season -> episodes.
	episodes     map[string]map[int][]client.TMDBEpisode
	calls        int
	episodeCalls int
	err          error
}

func (f *fakeTitleSource) TVSeasonEpisodes(_ context.Context, tvID string, seasons []int) (map[int][]client.TMDBEpisode, error) {
	f.episodeCalls++
	if f.err != nil {
		return nil, f.err
	}
	show, ok := f.episodes[tvID]
	if !ok {
		return nil, nil
	}
	out := make(map[int][]client.TMDBEpisode, len(seasons))
	for _, n := range seasons {
		if eps, ok := show[n]; ok {
			out[n] = eps
		}
	}
	return out, nil
}

func (f *fakeTitleSource) Enabled() bool { return f.enabled }

func (f *fakeTitleSource) SearchMovie(context.Context, string) ([]client.TMDBSearchResult, error) {
	return []client.TMDBSearchResult{{ID: 27205, Title: "Inception", Year: 2010}}, nil
}

func (f *fakeTitleSource) SearchTV(context.Context, string) ([]client.TMDBSearchResult, error) {
	return []client.TMDBSearchResult{{ID: 1396, Title: "Breaking Bad", Year: 2008}}, nil
}

func (f *fakeTitleSource) TitleDetails(_ context.Context, kind, id string) (client.TMDBTitleDetails, error) {
	f.calls++
	if f.err != nil {
		return client.TMDBTitleDetails{}, f.err
	}
	d, ok := f.details[kind+"/"+id]
	if !ok {
		return client.TMDBTitleDetails{}, nil // Found false: TMDB 404
	}
	return d, nil
}

func newTrackingEnv(t *testing.T) (*TrackingService, *fakeTitleSource, int64, int64) {
	t.Helper()
	s, src, alice, bob, _ := newTrackingEnvDB(t)
	return s, src, alice, bob
}

// newTrackingEnvDB also hands back the database, for tests that need to seed
// the chart-side tables tracking reads but never writes.
func newTrackingEnvDB(t *testing.T) (*TrackingService, *fakeTitleSource, int64, int64, *sql.DB) {
	t.Helper()
	db := newTestDB(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	userRepo := repository.NewUserRepository(db)
	sessions := NewSessionService(log, userRepo, SessionConfig{TTL: time.Hour, IdleTimeout: time.Hour})
	users := NewUserService(log, userRepo, sessions)
	alice, err := users.Create(context.Background(), "alice", "long enough pw", RoleReader)
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := users.Create(context.Background(), "bob", "long enough pw", RoleReader)
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	src := &fakeTitleSource{enabled: true, details: map[string]client.TMDBTitleDetails{
		"movie/27205": {Title: "Inception", ReleaseDate: "2010-07-15", Year: 2010, Runtime: 148, Found: true},
		"movie/155":   {Title: "The Dark Knight", ReleaseDate: "2008-07-16", Year: 2008, Runtime: 152, Found: true},
		"movie/999":   {Title: "No Runtime", Found: true},
		"tv/1396": {Title: "Breaking Bad", ReleaseDate: "2008-01-20", Year: 2008, Runtime: 47, Found: true,
			NumberOfSeasons: 2, Seasons: []client.TMDBSeason{{Number: 0}, {Number: 1}, {Number: 2}}},
	}}
	src.episodes = map[string]map[int][]client.TMDBEpisode{"1396": {
		0: {{ID: 900, SeasonNumber: 0, EpisodeNumber: 1, Name: "Special", AirDate: "2008-01-01", Runtime: 20}},
		1: {
			{ID: 901, SeasonNumber: 1, EpisodeNumber: 1, Name: "Pilot", AirDate: "2008-01-20", Runtime: 58},
			{ID: 902, SeasonNumber: 1, EpisodeNumber: 2, Name: "Cat in the Bag", AirDate: "2008-01-27", Runtime: 48},
			{ID: 903, SeasonNumber: 1, EpisodeNumber: 3, Name: "Bag Polka", AirDate: "2008-02-10", Runtime: 48},
		},
		2: {
			{ID: 904, SeasonNumber: 2, EpisodeNumber: 1, Name: "Seven Thirty-Seven", AirDate: "2009-03-08", Runtime: 47},
			{ID: 905, SeasonNumber: 2, EpisodeNumber: 2, Name: "Grilled", AirDate: "2009-03-15", Runtime: 47, EpisodeType: "finale"},
			{ID: 906, SeasonNumber: 2, EpisodeNumber: 3, Name: "Not Out Yet", Runtime: 47},
		},
	}}
	repo := repository.NewTrackingRepository(db)
	titles := NewTrackedTitleService(log, src, repo, repo)
	return NewTrackingService(log, repo, repo, titles), src, alice.ID, bob.ID, db
}

func mustLog(t *testing.T, s *TrackingService, userID int64, in WatchInput) repository.WatchEntry {
	t.Helper()
	e, _, err := s.LogWatch(context.Background(), userID, in)
	if err != nil {
		t.Fatalf("log watch: %v", err)
	}
	return e
}

// tp is a generic pointer helper (ptr in this package is int64-only).
func tp[T any](v T) *T { return &v }

// The day a watch belongs to is the local day of its start, which is
// ended_at minus the runtime unless the client sent a start.
func TestLogWatchDerivesTheLocalDay(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)

	for _, tc := range []struct {
		name      string
		in        WatchInput
		wantDay   string
		wantStart *string
		wantRtSrc string
	}{
		{
			// 01:30Z ended, 148 minutes long: started 2026-11-09 23:02Z,
			// which is still the 9th in UTC.
			name:      "runtime pulls the start back over midnight",
			in:        WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-10T01:30:00Z"},
			wantDay:   "2026-11-09",
			wantRtSrc: "tmdb_title",
		},
		{
			// Same instant in New York: 20:30 local on the 9th, started
			// 18:02 local, still the 9th.
			name:      "local zone decides the day",
			in:        WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-10T01:30:00Z", TZ: tp("America/New_York")},
			wantDay:   "2026-11-09",
			wantRtSrc: "tmdb_title",
		},
		{
			// 02:00Z on 1 January is still 31 December in New York, so the
			// watch belongs to the previous year.
			name:      "year boundary follows the local zone",
			in:        WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2027-01-01T02:00:00Z", TZ: tp("America/New_York")},
			wantDay:   "2026-12-31",
			wantRtSrc: "tmdb_title",
		},
		{
			// A half-hour zone: 18:20Z is 23:50 in Kolkata, and 148 minutes
			// earlier is 21:22 the same day.
			name:      "half-hour offset zone",
			in:        WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-06-15T18:20:00Z", TZ: tp("Asia/Kolkata")},
			wantDay:   "2026-06-15",
			wantRtSrc: "tmdb_title",
		},
		{
			name:      "an explicit start wins over the runtime",
			in:        WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-10T01:30:00Z", StartedAt: tp("2026-11-10T00:10:00Z")},
			wantDay:   "2026-11-10",
			wantStart: tp("2026-11-10 00:10:00"),
		},
		{
			name:      "no runtime known: the end is the start",
			in:        WatchInput{Kind: "movie", TmdbID: "999", EndedAt: "2026-11-10T01:30:00Z"},
			wantDay:   "2026-11-10",
			wantRtSrc: "unknown",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := mustLog(t, s, alice, tc.in)
			if e.WatchedOn != tc.wantDay {
				t.Errorf("watched_on = %q, want %q", e.WatchedOn, tc.wantDay)
			}
			if tc.wantStart == nil && e.StartedAt != nil {
				t.Errorf("started_at = %q, want none stored", *e.StartedAt)
			}
			if tc.wantStart != nil && (e.StartedAt == nil || *e.StartedAt != *tc.wantStart) {
				t.Errorf("started_at = %v, want %q", e.StartedAt, *tc.wantStart)
			}
			if tc.wantRtSrc != "" && (e.RuntimeSource == nil || *e.RuntimeSource != tc.wantRtSrc) {
				t.Errorf("runtime_source = %v, want %q", e.RuntimeSource, tc.wantRtSrc)
			}
		})
	}
}

// The offset stored is the one in force at that instant, so a zone that
// changes between two watches is recorded correctly for each.
func TestLogWatchStoresTheOffsetInForce(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)

	for _, tc := range []struct {
		name       string
		endedAt    string
		tz         string
		wantOffset int64
	}{
		{"new york in summer time", "2026-07-04T18:00:00Z", "America/New_York", -240},
		{"new york in winter", "2026-12-04T18:00:00Z", "America/New_York", -300},
		{"kolkata has no dst", "2026-07-04T18:00:00Z", "Asia/Kolkata", 330},
		{"kathmandu is three quarters of an hour", "2026-07-04T18:00:00Z", "Asia/Kathmandu", 345},
		{"utc", "2026-07-04T18:00:00Z", "UTC", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: tc.endedAt, TZ: tp(tc.tz)})
			if e.UTCOffsetMinutes == nil || *e.UTCOffsetMinutes != tc.wantOffset {
				t.Fatalf("utc_offset_minutes = %v, want %d", derefInt(e.UTCOffsetMinutes), tc.wantOffset)
			}
			if e.Tz == nil || *e.Tz != tc.tz {
				t.Errorf("tz = %v, want %q", e.Tz, tc.tz)
			}
		})
	}
}

// Both sides of each DST transition are stored with the offset in force at
// that instant.
//
// Clients send UTC instants, so the two cases that make local wall-clock
// times ambiguous cannot arise here: the hour that occurs twice on the
// fall-back is two distinct instants, and the hour that never happens on the
// spring-forward is no instant at all. That is the point of taking UTC from
// the client rather than a local time plus a zone.
func TestLogWatchHandlesDSTEdges(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)

	for _, tc := range []struct {
		name       string
		endedAt    string
		wantOffset int64
		wantDay    string
	}{
		// 01:30 happens twice in New York on 1 November 2026. The two are
		// separate instants an hour apart, each with its own offset.
		{"first 01:30 of the repeated hour", "2026-11-01T05:30:00Z", -240, "2026-11-01"},
		{"second 01:30 of the repeated hour", "2026-11-01T06:30:00Z", -300, "2026-11-01"},
		// Summer time starts at 02:00 local on 8 March 2026, when the clocks
		// jump to 03:00; 01:59 and 03:00 are a minute apart.
		{"a minute before the clocks go forward", "2026-03-08T06:59:00Z", -300, "2026-03-08"},
		{"the instant the clocks go forward", "2026-03-08T07:00:00Z", -240, "2026-03-08"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := mustLog(t, s, alice, WatchInput{
				Kind: "movie", TmdbID: "999", EndedAt: tc.endedAt, TZ: tp("America/New_York")})
			if e.UTCOffsetMinutes == nil || *e.UTCOffsetMinutes != tc.wantOffset {
				t.Errorf("utc_offset_minutes = %v, want %d", derefInt(e.UTCOffsetMinutes), tc.wantOffset)
			}
			if e.WatchedOn != tc.wantDay {
				t.Errorf("watched_on = %q, want %q", e.WatchedOn, tc.wantDay)
			}
		})
	}
}

// derefInt renders a nullable int for a failure message.
func derefInt(p *int64) any {
	if p == nil {
		return "nil"
	}
	return *p
}

// An imprecise time is stored at the earliest instant it could mean, and no
// runtime is subtracted (a "2019" watch must not land in 2018).
func TestLogWatchNormalizesImpreciseTimes(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)

	for _, tc := range []struct {
		name      string
		precision string
		endedAt   string
		tz        *string
		wantDay   string
		wantEnded string
	}{
		{"year", PrecisionYear, "2019-08-17T21:45:00Z", nil, "2019-01-01", "2019-01-01 00:00:00"},
		{"month", PrecisionMonth, "2019-08-17T21:45:00Z", nil, "2019-08-01", "2019-08-01 00:00:00"},
		{"day", PrecisionDay, "2019-08-17T21:45:00Z", nil, "2019-08-17", "2019-08-17 00:00:00"},
		// 01:00Z on 18 August is still the 17th in New York, so the day
		// precision keeps the user's own day and stores its local midnight.
		{"day in a western zone", PrecisionDay, "2019-08-18T01:00:00Z", tp("America/New_York"), "2019-08-17", "2019-08-17 04:00:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := mustLog(t, s, alice, WatchInput{
				Kind: "movie", TmdbID: "27205", EndedAt: tc.endedAt, TZ: tc.tz, Precision: tc.precision})
			if e.WatchedOn != tc.wantDay {
				t.Errorf("watched_on = %q, want %q", e.WatchedOn, tc.wantDay)
			}
			if e.EndedAt != tc.wantEnded {
				t.Errorf("ended_at = %q, want %q", e.EndedAt, tc.wantEnded)
			}
			if e.RuntimeMinutesUsed != nil {
				t.Errorf("runtime_minutes_used = %v, want none for an imprecise time", *e.RuntimeMinutesUsed)
			}
			if e.TimePrecision != tc.precision {
				t.Errorf("time_precision = %q, want %q", e.TimePrecision, tc.precision)
			}
		})
	}
}

func TestLogWatchRejectsBadInput(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)

	for _, tc := range []struct {
		name string
		in   WatchInput
	}{
		{"no kind", WatchInput{TmdbID: "27205", EndedAt: "2026-11-10T01:30:00Z"}},
		{"not a tmdb id", WatchInput{Kind: "movie", TmdbID: "tt1375666", EndedAt: "2026-11-10T01:30:00Z"}},
		{"no ended_at", WatchInput{Kind: "movie", TmdbID: "27205"}},
		{"ended_at is not rfc3339", WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "10/11/2026"}},
		{"unknown zone", WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-10T01:30:00Z", TZ: tp("Mars/Olympus")}},
		{"rating too high", WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-10T01:30:00Z", Rating: tp(int64(11))}},
		{"rating zero", WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-10T01:30:00Z", Rating: tp(int64(0))}},
		{"unknown precision", WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-10T01:30:00Z", Precision: "decade"}},
		{"start after end", WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-10T01:30:00Z", StartedAt: tp("2026-11-10T02:00:00Z")}},
		{"unknown venue kind", WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-10T01:30:00Z", VenueKind: tp("submarine")}},
		{"a show without an episode number", WatchInput{Kind: "tv", TmdbID: "1396", EndedAt: "2026-11-10T01:30:00Z"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := s.LogWatch(context.Background(), alice, tc.in)
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want a ValidationError", err)
			}
		})
	}
}

func TestLogWatchRejectsAnUnknownTitle(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	_, _, err := s.LogWatch(context.Background(), alice, WatchInput{
		Kind: "movie", TmdbID: "424242", EndedAt: "2026-11-10T01:30:00Z"})
	if !errors.Is(err, ErrTrackedTitleUnknown) {
		t.Fatalf("err = %v, want ErrTrackedTitleUnknown", err)
	}
}

// Rewatches are separate entries, numbered, and the title's rating follows
// the most recent watch that carries one.
func TestRewatchesAndRollup(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()

	first := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205",
		EndedAt: "2026-11-10T01:30:00Z", Rating: tp(int64(9)), Tags: []string{"imax", "Imax", " nolan "}})
	if first.WatchNumber != 1 || first.IsRewatch {
		t.Fatalf("first watch: number %d rewatch %v", first.WatchNumber, first.IsRewatch)
	}
	if len(first.Tags) != 2 {
		t.Fatalf("tags = %v, want imax and nolan de-duplicated", first.Tags)
	}

	second := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205",
		EndedAt: "2026-11-12T23:00:00Z", Rating: tp(int64(8))})
	if second.WatchNumber != 2 || !second.IsRewatch {
		t.Fatalf("second watch: number %d rewatch %v", second.WatchNumber, second.IsRewatch)
	}

	st, err := s.read.UserTitle(ctx, alice, "movie", "27205")
	if err != nil || st == nil {
		t.Fatalf("user title: %v %v", st, err)
	}
	if st.WatchCount != 2 {
		t.Errorf("watch_count = %d, want 2", st.WatchCount)
	}
	// "My rating" is the latest watch's rating, not the highest or the first.
	if st.Rating == nil || *st.Rating != 8 {
		t.Errorf("rating = %v, want 8 (the latest watch)", st.Rating)
	}

	// Deleting the latest watch hands the rating back to the earlier one.
	if err := s.DeleteWatch(ctx, alice, second.ID); err != nil {
		t.Fatalf("delete watch: %v", err)
	}
	st, err = s.read.UserTitle(ctx, alice, "movie", "27205")
	if err != nil || st == nil {
		t.Fatalf("user title after delete: %v %v", st, err)
	}
	if st.WatchCount != 1 || st.Rating == nil || *st.Rating != 9 {
		t.Errorf("after delete: count %d rating %v, want 1 and 9", st.WatchCount, st.Rating)
	}
}

func TestLogWatchIsIdempotentPerClientKey(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	in := WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-10T01:30:00Z", ClientKey: tp("abc-123")}

	first, created, err := s.LogWatch(ctx, alice, in)
	if err != nil || !created {
		t.Fatalf("first log: created %v err %v", created, err)
	}
	second, created, err := s.LogWatch(ctx, alice, in)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if created {
		t.Error("replay created a second watch")
	}
	if second.ID != first.ID {
		t.Errorf("replay returned id %d, want %d", second.ID, first.ID)
	}

}

// One user must never see or change another's rows, and a miss looks like a
// miss rather than a refusal, so ids do not leak.
func TestWatchesAreScopedToTheirUser(t *testing.T) {
	s, _, alice, bob := newTrackingEnv(t)
	ctx := context.Background()

	mine := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-10T01:30:00Z"})

	if _, err := s.GetWatch(ctx, bob, mine.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("bob reading alice's watch: %v, want sql.ErrNoRows", err)
	}
	if _, err := s.UpdateWatch(ctx, bob, mine.ID, WatchInput{EndedAt: "2026-11-11T01:30:00Z"}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("bob updating alice's watch: %v, want sql.ErrNoRows", err)
	}
	if err := s.DeleteWatch(ctx, bob, mine.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("bob deleting alice's watch: %v, want sql.ErrNoRows", err)
	}

	// Alice's watch is untouched, and bob's diary is empty.
	if _, err := s.GetWatch(ctx, alice, mine.ID); err != nil {
		t.Errorf("alice reading her own watch: %v", err)
	}
	out, err := s.ListWatches(ctx, bob, DiaryInput{})
	if err != nil {
		t.Fatalf("bob's diary: %v", err)
	}
	if got := out["watches"].([]repository.WatchEntry); len(got) != 0 {
		t.Errorf("bob's diary has %d entries, want 0", len(got))
	}

	// The same client key may be used by each of them.
	for _, u := range []int64{alice, bob} {
		if _, _, err := s.LogWatch(ctx, u, WatchInput{Kind: "movie", TmdbID: "155",
			EndedAt: "2026-11-10T01:30:00Z", ClientKey: tp("shared-key")}); err != nil {
			t.Fatalf("log for user %d: %v", u, err)
		}
	}
}

func TestWatchlistPositionsAndRemoval(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()

	for _, id := range []string{"27205", "155"} {
		if _, err := s.SetTitleState(ctx, alice, "movie", id, TitleStateInput{Status: tp(StatusWatchlist)}); err != nil {
			t.Fatalf("watchlist %s: %v", id, err)
		}
	}
	out, err := s.ListTitles(ctx, alice, ListTitlesInput{Status: StatusWatchlist})
	if err != nil {
		t.Fatalf("list watchlist: %v", err)
	}
	rows := out["titles"].([]repository.UserTitle)
	if len(rows) != 2 {
		t.Fatalf("watchlist has %d entries, want 2", len(rows))
	}
	if rows[0].ID != "27205" || rows[1].ID != "155" {
		t.Errorf("watchlist order = %s, %s, want insertion order", rows[0].ID, rows[1].ID)
	}
	if rows[0].WatchlistPosition == nil || *rows[0].WatchlistPosition != 1 {
		t.Errorf("first position = %v, want 1", rows[0].WatchlistPosition)
	}

	// Leaving the watchlist clears the position but keeps the row's other
	// state; with nothing else on it, the row goes away entirely.
	if _, err := s.SetTitleState(ctx, alice, "movie", "155", TitleStateInput{Status: tp(StatusNone)}); err != nil {
		t.Fatalf("remove from watchlist: %v", err)
	}
	st, err := s.read.UserTitle(ctx, alice, "movie", "155")
	if err != nil {
		t.Fatalf("user title: %v", err)
	}
	if st != nil {
		t.Errorf("state = %+v, want the empty row pruned", st)
	}
}

// A rating belongs to a watch, so setting one on a title is refused rather
// than silently stored somewhere it would not be read from.
func TestSetTitleStateRefusesARating(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	_, err := s.SetTitleState(context.Background(), alice, "movie", "27205",
		TitleStateInput{Rating: tp(int64(8))})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want a ValidationError", err)
	}
}

func TestDiaryPagesByCursor(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	for _, day := range []string{"2026-11-09", "2026-11-10", "2026-11-11"} {
		mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "999",
			EndedAt: day + "T20:00:00Z", Precision: PrecisionDay})
	}

	out, err := s.ListWatches(ctx, alice, DiaryInput{Limit: 2})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	page1 := out["watches"].([]repository.WatchEntry)
	if len(page1) != 2 || page1[0].WatchedOn != "2026-11-11" {
		t.Fatalf("page 1 = %d entries starting %q", len(page1), page1[0].WatchedOn)
	}
	cursor, ok := out["next_cursor"].(string)
	if !ok || cursor == "" {
		t.Fatalf("next_cursor = %v, want one", out["next_cursor"])
	}

	out, err = s.ListWatches(ctx, alice, DiaryInput{Limit: 2, Cursor: cursor})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	page2 := out["watches"].([]repository.WatchEntry)
	if len(page2) != 1 || page2[0].WatchedOn != "2026-11-09" {
		t.Fatalf("page 2 = %d entries, first %q", len(page2), page2[0].WatchedOn)
	}
	if _, ok := out["next_cursor"]; ok {
		t.Error("the last page still offered a cursor")
	}

	if _, err := s.ListWatches(ctx, alice, DiaryInput{Cursor: "nonsense"}); err == nil {
		t.Error("a bad cursor was accepted")
	}
}

// The title cache is filled once and reused, so a diary page costs no TMDB
// calls per row.
func TestResolveCachesTitles(t *testing.T) {
	s, src, alice, _ := newTrackingEnv(t)
	ctx := context.Background()

	mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-10T01:30:00Z"})
	after := src.calls
	if after == 0 {
		t.Fatal("the first log made no TMDB call")
	}
	mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-12T23:00:00Z"})
	if src.calls != after {
		t.Errorf("TMDB calls = %d, want %d: the cached snapshot was not reused", src.calls, after)
	}

	out, err := s.ListWatches(ctx, alice, DiaryInput{})
	if err != nil {
		t.Fatalf("diary: %v", err)
	}
	rows := out["watches"].([]repository.WatchEntry)
	if len(rows) != 2 || rows[0].Title != "Inception" {
		t.Fatalf("diary = %d rows, first title %q", len(rows), rows[0].Title)
	}
}

// Without a TMDB key tracking keeps working rather than failing, with empty
// metadata for titles that never appeared on a chart.
func TestTrackingWorksWithoutATMDBKey(t *testing.T) {
	s, src, alice, _ := newTrackingEnv(t)
	src.enabled = false

	e := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-10T01:30:00Z"})
	if e.WatchedOn != "2026-11-10" {
		t.Errorf("watched_on = %q: with no runtime known the end is the start", e.WatchedOn)
	}
	if _, err := s.SearchTitles(context.Background(), "inception", "", 0); !errors.Is(err, ErrUpstream) {
		t.Errorf("search without a key: %v, want ErrUpstream", err)
	}
}

func TestNormalizeTagsLimits(t *testing.T) {
	if _, err := normalizeTags([]string{string(make([]byte, maxTagLength+1))}); err == nil {
		t.Error("an over-long tag was accepted")
	}
	many := make([]string, maxTagsPerWatch+1)
	for i := range many {
		many[i] = string(rune('a' + i))
	}
	if _, err := normalizeTags(many); err == nil {
		t.Error("too many tags were accepted")
	}
	got, err := normalizeTags([]string{" b ", "A", "a", ""})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(got) != 2 || got[0] != "A" || got[1] != "b" {
		t.Errorf("tags = %v, want A and b", got)
	}
}
