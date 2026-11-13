package service

import (
	"context"
	"os"
	"strings"
	"testing"
)

// numbers reads back each watch's (watch_number, is_rewatch) by id.
func numbers(t *testing.T, s *TrackingService, userID int64, ids ...int64) [][2]any {
	t.Helper()
	out := make([][2]any, len(ids))
	for i, id := range ids {
		w, err := s.GetWatch(context.Background(), userID, id)
		if err != nil {
			t.Fatalf("get watch %d: %v", id, err)
		}
		out[i] = [2]any{w.WatchNumber, w.IsRewatch}
	}
	return out
}

func wantNumbers(t *testing.T, got [][2]any, want ...int64) {
	t.Helper()
	for i, n := range want {
		if got[i][0] != n || got[i][1] != (n > 1) {
			t.Fatalf("watch %d: number %v rewatch %v, want %d %v (all: %v)", i, got[i][0], got[i][1], n, n > 1, got)
		}
	}
}

// A watch logged late for an earlier date is the first viewing, not a rewatch.
func TestWatchNumbersFollowTimeNotLogOrder(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	nov := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-12T02:40:00Z"})
	aug := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-08-02T01:10:00Z"})
	if aug.WatchNumber != 1 || aug.IsRewatch {
		t.Fatalf("backdated watch returned as number %d rewatch %v, want 1 false", aug.WatchNumber, aug.IsRewatch)
	}
	wantNumbers(t, numbers(t, s, alice, aug.ID, nov.ID), 1, 2)
}

// Editing a watch's time renumbers the title around it.
func TestWatchNumbersFollowAnEditedTime(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	jan := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-01-10T20:00:00Z"})
	feb := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-02-10T20:00:00Z"})
	mar := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-03-10T20:00:00Z"})
	wantNumbers(t, numbers(t, s, alice, jan.ID, feb.ID, mar.ID), 1, 2, 3)

	// March's watch was really last December.
	if _, err := s.UpdateWatch(ctx, alice, mar.ID, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2025-12-20T20:00:00Z"}); err != nil {
		t.Fatalf("update watch: %v", err)
	}
	wantNumbers(t, numbers(t, s, alice, mar.ID, jan.ID, feb.ID), 1, 2, 3)
}

// Deleting a watch closes the gap; the remaining ones keep their order.
func TestWatchNumbersCloseTheGapAfterADelete(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()
	a := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-01-10T20:00:00Z"})
	b := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-02-10T20:00:00Z"})
	c := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-03-10T20:00:00Z"})
	if err := s.DeleteWatch(ctx, alice, b.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	wantNumbers(t, numbers(t, s, alice, a.ID, c.ID), 1, 2)

	// Deleting the first one makes the next the first viewing again.
	if err := s.DeleteWatch(ctx, alice, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	wantNumbers(t, numbers(t, s, alice, c.ID), 1)
}

// Two watches at the same instant are ordered by id, so the numbering is stable.
func TestWatchNumbersBreakTiesByID(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	x := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-05-01T20:00:00Z"})
	y := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-05-01T20:00:00Z"})
	wantNumbers(t, numbers(t, s, alice, x.ID, y.ID), 1, 2)
}

// A show is numbered per episode, and bulk marks join the same time order.
func TestWatchNumbersArePerEpisode(t *testing.T) {
	s, _, alice, _, db := newTrackingEnvDB(t)
	ctx := context.Background()
	ep := func(season, episode int64, at string) int64 {
		return mustLog(t, s, alice, WatchInput{Kind: "tv", TmdbID: "1396", SeasonNumber: tp(season), EpisodeNumber: tp(episode), EndedAt: at}).ID
	}
	e1a := ep(1, 1, "2026-03-01T20:00:00Z")
	e2a := ep(1, 2, "2026-03-02T20:00:00Z")
	e1b := ep(1, 1, "2026-06-01T20:00:00Z")
	e1early := ep(1, 1, "2026-02-01T20:00:00Z") // logged last, watched first
	wantNumbers(t, numbers(t, s, alice, e1early, e1a, e1b, e2a), 1, 2, 3, 1)

	// A bulk mark skips episodes already seen, so marking season 1 adds only E3.
	if _, err := s.MarkEpisodes(ctx, alice, "1396", MarkEpisodesInput{Scope: ScopeSeason, SeasonNumber: tp(int64(1)), EndedAt: "2026-07-01T12:00:00Z"}); err != nil {
		t.Fatalf("mark season: %v", err)
	}
	var marked int64
	if err := db.QueryRow(`SELECT id FROM watch_entries WHERE tmdb_id = '1396' AND season_number = 1 AND episode_number = 3`).Scan(&marked); err != nil {
		t.Fatalf("find the marked E3: %v", err)
	}
	wantNumbers(t, numbers(t, s, alice, marked), 1)
	wantNumbers(t, numbers(t, s, alice, e1early, e1a, e1b, e2a), 1, 2, 3, 1) // other episodes untouched

	// E3 was really seen earlier as well: the backdated watch comes first, the mark second.
	e3early := ep(1, 3, "2026-01-15T20:00:00Z")
	wantNumbers(t, numbers(t, s, alice, e3early, marked), 1, 2)
}

// The 000030 migration repairs rows numbered by the old log-order rule.
func TestRenumberMigrationRepairsLogOrderNumbers(t *testing.T) {
	s, _, alice, _, db := newTrackingEnvDB(t)
	nov := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-11-12T02:40:00Z"})
	aug := mustLog(t, s, alice, WatchInput{Kind: "movie", TmdbID: "27205", EndedAt: "2026-08-02T01:10:00Z"})

	// Put back what the old code stored: numbers in log order.
	if _, err := db.Exec(`UPDATE watch_entries SET watch_number = 1, is_rewatch = 0 WHERE id = ?`, nov.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE watch_entries SET watch_number = 2, is_rewatch = 1 WHERE id = ?`, aug.ID); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile("../../db/migrations/000030_renumber_watches_by_time.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.SplitN(strings.SplitN(string(raw), "-- +goose Down", 2)[0], "-- +goose Up", 2)[1]
	if _, err := db.Exec(up); err != nil {
		t.Fatalf("run migration up: %v", err)
	}
	wantNumbers(t, numbers(t, s, alice, aug.ID, nov.ID), 1, 2)
}
