package service

import (
	"context"
	"errors"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// progressOf reads the progress block out of a show payload.
func progressOf(t *testing.T, out map[string]any) map[string]any {
	t.Helper()
	p, ok := out["progress"].(map[string]any)
	if !ok {
		t.Fatalf("no progress in %v", out)
	}
	return p
}

// Logging one episode stores it as an episode watch and uses the episode's
// own runtime, not the show's typical one.
func TestLogEpisodeWatch(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)

	e, _, err := s.LogWatch(context.Background(), alice, WatchInput{
		Kind: "tv", TmdbID: "1396", SeasonNumber: tp(int64(1)), EpisodeNumber: tp(int64(1)),
		EndedAt: "2026-11-10T02:00:00Z", Rating: tp(int64(9)), Tags: []string{"pilot"},
	})
	if err != nil {
		t.Fatalf("log episode: %v", err)
	}
	if e.SubjectKind != "episode" {
		t.Errorf("subject_kind = %q, want episode", e.SubjectKind)
	}
	if e.SeasonNumber == nil || *e.SeasonNumber != 1 || e.EpisodeNumber == nil || *e.EpisodeNumber != 1 {
		t.Errorf("season/episode = %v/%v, want 1/1", e.SeasonNumber, e.EpisodeNumber)
	}
	if e.TmdbEpisodeID == nil || *e.TmdbEpisodeID != "901" {
		t.Errorf("tmdb_episode_id = %v, want 901", e.TmdbEpisodeID)
	}
	// The pilot runs 58 minutes; the show's typical episode is 47.
	if e.RuntimeMinutesUsed == nil || *e.RuntimeMinutesUsed != 58 {
		t.Errorf("runtime_minutes_used = %v, want 58 (the episode's own)", derefInt(e.RuntimeMinutesUsed))
	}
	if e.RuntimeSource == nil || *e.RuntimeSource != "tmdb_episode" {
		t.Errorf("runtime_source = %v, want tmdb_episode", e.RuntimeSource)
	}
	// 02:00Z minus 58 minutes is 01:02Z, still the 10th.
	if e.WatchedOn != "2026-11-10" {
		t.Errorf("watched_on = %q, want 2026-11-10", e.WatchedOn)
	}
}

func TestLogEpisodeRejectsBadCoordinates(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	for _, tc := range []struct {
		name string
		in   WatchInput
	}{
		{"a show without an episode", WatchInput{Kind: "tv", TmdbID: "1396", EndedAt: "2026-11-10T02:00:00Z"}},
		{"a season without an episode", WatchInput{Kind: "tv", TmdbID: "1396", SeasonNumber: tp(int64(1)), EndedAt: "2026-11-10T02:00:00Z"}},
		{"a movie with an episode", WatchInput{Kind: "movie", TmdbID: "27205", SeasonNumber: tp(int64(1)), EpisodeNumber: tp(int64(1)), EndedAt: "2026-11-10T02:00:00Z"}},
		{"an episode TMDB does not have", WatchInput{Kind: "tv", TmdbID: "1396", SeasonNumber: tp(int64(9)), EpisodeNumber: tp(int64(9)), EndedAt: "2026-11-10T02:00:00Z"}},
		{"episode zero", WatchInput{Kind: "tv", TmdbID: "1396", SeasonNumber: tp(int64(1)), EpisodeNumber: tp(int64(0)), EndedAt: "2026-11-10T02:00:00Z"}},
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

// A season mark covers only episodes that have aired and that the user has
// not already seen, so it never piles rewatches onto what they have watched.
func TestMarkSeasonSkipsWatchedAndUnaired(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()

	// Already seen S2E1.
	if _, _, err := s.LogWatch(ctx, alice, WatchInput{Kind: "tv", TmdbID: "1396",
		SeasonNumber: tp(int64(2)), EpisodeNumber: tp(int64(1)), EndedAt: "2026-11-01T20:00:00Z"}); err != nil {
		t.Fatalf("log S2E1: %v", err)
	}

	out, err := s.MarkEpisodes(ctx, alice, "1396", MarkEpisodesInput{
		Scope: ScopeSeason, SeasonNumber: tp(int64(2)), EndedAt: "2026-11-12T20:00:00Z"})
	if err != nil {
		t.Fatalf("mark season 2: %v", err)
	}
	// S2 has three episodes: E1 is seen, E2 has aired, E3 has no air date.
	if out["marked"].(int) != 1 {
		t.Errorf("marked = %v, want 1 (E2 only)", out["marked"])
	}
	progress := progressOf(t, out)
	if progress["watched_episodes"].(int64) != 2 {
		t.Errorf("watched_episodes = %v, want 2", progress["watched_episodes"])
	}

	// S2E1 still has exactly one watch: the mark did not rewatch it.
	eps, err := s.read.ShowEpisodes(ctx, alice, "1396", false)
	if err != nil {
		t.Fatalf("show episodes: %v", err)
	}
	for _, e := range eps {
		if e.SeasonNumber == 2 && e.EpisodeNumber == 1 && e.WatchCount != 1 {
			t.Errorf("S2E1 watch_count = %d, want 1", e.WatchCount)
		}
		if e.SeasonNumber == 2 && e.EpisodeNumber == 3 && e.WatchCount != 0 {
			t.Errorf("S2E3 has not aired but was marked")
		}
	}
}

// "Everything up to here" crosses earlier seasons and leaves specials out
// unless they are asked for.
func TestMarkUpToCrossesSeasons(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()

	out, err := s.MarkEpisodes(ctx, alice, "1396", MarkEpisodesInput{
		Scope: ScopeUpTo, SeasonNumber: tp(int64(2)), EpisodeNumber: tp(int64(1)),
		EndedAt: "2026-11-12T20:00:00Z"})
	if err != nil {
		t.Fatalf("mark up to S2E1: %v", err)
	}
	// S1E1-3 and S2E1; the special is left out.
	if out["marked"].(int) != 4 {
		t.Errorf("marked = %v, want 4", out["marked"])
	}
	progress := progressOf(t, out)
	if progress["total_episodes"].(int64) != 6 {
		t.Errorf("total_episodes = %v, want 6 (specials excluded)", progress["total_episodes"])
	}
	if progress["aired_episodes"].(int64) != 5 {
		t.Errorf("aired_episodes = %v, want 5", progress["aired_episodes"])
	}
	if progress["percent"].(int64) != 80 {
		t.Errorf("percent = %v, want 80 (4 of 5 aired)", progress["percent"])
	}

	// With specials counted in, the special is there to be marked.
	out, err = s.MarkEpisodes(ctx, alice, "1396", MarkEpisodesInput{
		Scope: ScopeAll, IncludeSpecials: true, EndedAt: "2026-11-12T20:00:00Z"})
	if err != nil {
		t.Fatalf("mark all with specials: %v", err)
	}
	if out["marked"].(int) != 2 {
		t.Errorf("marked = %v, want 2 (the special and S2E2)", out["marked"])
	}
}

// Marking and unmarking move a show between watching and completed, so
// up-next works without the user setting a status by hand.
func TestShowStatusFollowsProgress(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()

	status := func() string {
		t.Helper()
		st, err := s.read.UserTitle(ctx, alice, "tv", "1396")
		if err != nil {
			t.Fatalf("user title: %v", err)
		}
		if st == nil {
			return "<none>"
		}
		return st.Status
	}

	if _, err := s.MarkEpisodes(ctx, alice, "1396", MarkEpisodesInput{
		Scope: ScopeSeason, SeasonNumber: tp(int64(1)), EndedAt: "2026-11-12T20:00:00Z"}); err != nil {
		t.Fatalf("mark season 1: %v", err)
	}
	if got := status(); got != StatusWatching {
		t.Errorf("status after part of the show = %q, want watching", got)
	}

	if _, err := s.MarkEpisodes(ctx, alice, "1396", MarkEpisodesInput{
		Scope: ScopeAll, EndedAt: "2026-11-12T20:00:00Z"}); err != nil {
		t.Fatalf("mark all: %v", err)
	}
	if got := status(); got != StatusCompleted {
		t.Errorf("status after every aired episode = %q, want completed", got)
	}

	if _, err := s.UnmarkEpisodes(ctx, alice, "1396", ScopeEpisode, tp(int64(1)), tp(int64(1)), false); err != nil {
		t.Fatalf("unmark S1E1: %v", err)
	}
	if got := status(); got != StatusWatching {
		t.Errorf("status after unmarking one = %q, want watching", got)
	}

	// A status the user chose is not overwritten.
	if _, err := s.SetTitleState(ctx, alice, "tv", "1396", TitleStateInput{Status: tp(StatusDropped)}); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := s.MarkEpisodes(ctx, alice, "1396", MarkEpisodesInput{
		Scope: ScopeAll, EndedAt: "2026-11-12T20:00:00Z"}); err != nil {
		t.Fatalf("mark all after dropping: %v", err)
	}
	if got := status(); got != StatusDropped {
		t.Errorf("status = %q, want dropped to be left alone", got)
	}
}

// Unmarking deletes the watches, and taking everything back leaves no state
// row behind.
func TestUnmarkEpisodes(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	ctx := context.Background()

	if _, err := s.MarkEpisodes(ctx, alice, "1396", MarkEpisodesInput{
		Scope: ScopeAll, IncludeSpecials: true, EndedAt: "2026-11-12T20:00:00Z"}); err != nil {
		t.Fatalf("mark all: %v", err)
	}
	out, err := s.UnmarkEpisodes(ctx, alice, "1396", ScopeSeason, tp(int64(1)), nil, false)
	if err != nil {
		t.Fatalf("unmark season 1: %v", err)
	}
	if out["removed"].(int64) != 3 {
		t.Errorf("removed = %v, want 3", out["removed"])
	}

	if _, err := s.UnmarkEpisodes(ctx, alice, "1396", ScopeAll, nil, nil, true); err != nil {
		t.Fatalf("unmark everything: %v", err)
	}
	st, err := s.read.UserTitle(ctx, alice, "tv", "1396")
	if err != nil {
		t.Fatalf("user title: %v", err)
	}
	if st != nil {
		t.Errorf("state = %+v, want the empty row pruned", st)
	}
	diary, err := s.ListWatches(ctx, alice, DiaryInput{})
	if err != nil {
		t.Fatalf("diary: %v", err)
	}
	if got := diary["watches"].([]repository.WatchEntry); len(got) != 0 {
		t.Errorf("diary still has %d entries", len(got))
	}
}

// Up-next is the first unwatched aired episode of each show in progress, and
// a rewatch does not push it forward.
func TestUpNext(t *testing.T) {
	s, _, alice, bob := newTrackingEnv(t)
	ctx := context.Background()

	next := func(userID int64) []repository.UpNextEpisode {
		t.Helper()
		out, err := s.UpNext(ctx, userID, 0)
		if err != nil {
			t.Fatalf("up next: %v", err)
		}
		return out["episodes"].([]repository.UpNextEpisode)
	}

	// Nothing watched yet: the show is not in progress.
	if got := next(alice); len(got) != 0 {
		t.Fatalf("up next with nothing watched = %d entries, want 0", len(got))
	}

	if _, err := s.MarkEpisodes(ctx, alice, "1396", MarkEpisodesInput{
		Scope: ScopeUpTo, SeasonNumber: tp(int64(1)), EpisodeNumber: tp(int64(2)),
		EndedAt: "2026-11-12T20:00:00Z"}); err != nil {
		t.Fatalf("mark up to S1E2: %v", err)
	}
	got := next(alice)
	if len(got) != 1 || got[0].SeasonNumber != 1 || got[0].EpisodeNumber != 3 {
		t.Fatalf("up next = %+v, want S1E3", got)
	}
	if got[0].ShowTitle != "Breaking Bad" {
		t.Errorf("show_title = %q", got[0].ShowTitle)
	}

	// Rewatching an episode already seen does not move the queue on.
	if _, _, err := s.LogWatch(ctx, alice, WatchInput{Kind: "tv", TmdbID: "1396",
		SeasonNumber: tp(int64(1)), EpisodeNumber: tp(int64(1)), EndedAt: "2026-11-13T20:00:00Z"}); err != nil {
		t.Fatalf("rewatch S1E1: %v", err)
	}
	if got := next(alice); len(got) != 1 || got[0].EpisodeNumber != 3 {
		t.Errorf("up next after a rewatch = %+v, want S1E3 still", got)
	}

	// Finishing every aired episode empties the queue.
	if _, err := s.MarkEpisodes(ctx, alice, "1396", MarkEpisodesInput{
		Scope: ScopeAll, EndedAt: "2026-11-12T20:00:00Z"}); err != nil {
		t.Fatalf("mark all: %v", err)
	}
	if got := next(alice); len(got) != 0 {
		t.Errorf("up next when caught up = %+v, want empty", got)
	}

	// Another user's progress is their own.
	if got := next(bob); len(got) != 0 {
		t.Errorf("bob's up next = %+v, want empty", got)
	}
}

// A show's episodes are fetched once and reused, and the episode roll-up
// counts distinct episodes rather than watches.
func TestEpisodesAreCachedAndRolledUp(t *testing.T) {
	s, src, alice, _ := newTrackingEnv(t)
	ctx := context.Background()

	if _, err := s.GetShow(ctx, alice, "1396", false); err != nil {
		t.Fatalf("get show: %v", err)
	}
	after := src.episodeCalls
	if after == 0 {
		t.Fatal("the show page made no TMDB episode call")
	}
	if _, err := s.GetShow(ctx, alice, "1396", false); err != nil {
		t.Fatalf("get show again: %v", err)
	}
	if src.episodeCalls != after {
		t.Errorf("TMDB episode calls = %d, want %d: the cache was not reused", src.episodeCalls, after)
	}

	// Watch one episode twice: two watches, one episode of progress.
	for _, when := range []string{"2026-11-10T02:00:00Z", "2026-11-11T02:00:00Z"} {
		if _, _, err := s.LogWatch(ctx, alice, WatchInput{Kind: "tv", TmdbID: "1396",
			SeasonNumber: tp(int64(1)), EpisodeNumber: tp(int64(1)), EndedAt: when}); err != nil {
			t.Fatalf("log %s: %v", when, err)
		}
	}
	st, err := s.read.UserTitle(ctx, alice, "tv", "1396")
	if err != nil || st == nil {
		t.Fatalf("user title: %v %v", st, err)
	}
	if st.WatchCount != 2 {
		t.Errorf("watch_count = %d, want 2", st.WatchCount)
	}
	out, err := s.GetShow(ctx, alice, "1396", false)
	if err != nil {
		t.Fatalf("get show: %v", err)
	}
	progress := progressOf(t, out)
	if progress["watched_episodes"].(int64) != 1 {
		t.Errorf("watched_episodes = %v, want 1: a rewatch is not progress", progress["watched_episodes"])
	}
}

func TestMarkEpisodesRejectsBadScopes(t *testing.T) {
	s, _, alice, _ := newTrackingEnv(t)
	for _, tc := range []struct {
		name string
		in   MarkEpisodesInput
	}{
		{"unknown scope", MarkEpisodesInput{Scope: "everything"}},
		{"season scope without a season", MarkEpisodesInput{Scope: ScopeSeason}},
		{"up_to without an episode", MarkEpisodesInput{Scope: ScopeUpTo, SeasonNumber: tp(int64(1))}},
		{"negative season", MarkEpisodesInput{Scope: ScopeSeason, SeasonNumber: tp(int64(-1))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.MarkEpisodes(context.Background(), alice, "1396", tc.in)
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v, want a ValidationError", err)
			}
		})
	}
}
