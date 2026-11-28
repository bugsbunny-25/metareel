package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/client"
)

// liveEpisodeSource adds TMDB's episode details to the fake title source.
type liveEpisodeSource struct {
	*fakeTitleSource
	detail *client.TMDBEpisodeDetail
	err    error
}

func (e *liveEpisodeSource) TVEpisode(_ context.Context, _ string, season, episode int) (*client.TMDBEpisodeDetail, error) {
	if e.err != nil {
		return nil, e.err
	}
	if e.detail == nil || e.detail.SeasonNumber != season || e.detail.EpisodeNumber != episode {
		return nil, nil
	}
	d := *e.detail
	return &d, nil
}

func TestEpisodeDetail(t *testing.T) {
	s, src, alice, bob := newTrackingEnv(t)
	ctx := context.Background()
	live := &liveEpisodeSource{fakeTitleSource: src, detail: &client.TMDBEpisodeDetail{
		SeasonNumber: 1, EpisodeNumber: 2, Name: "Cat in the Bag", Overview: "Walt and Jesse…", StillPath: "/s.jpg",
		Runtime: 48, Directors: []string{"Adam Bernstein"}, Writers: []string{"Vince Gilligan"}, GuestStars: []client.TMDBGuestStar{},
	}}
	s.titles.tmdb = live

	// Two watches of S1E2 and one of S1E1 for alice; one of S1E2 for bob.
	for _, w := range []struct {
		user    int64
		episode int64
		at      string
	}{{alice, 2, "2026-11-01T20:00:00Z"}, {alice, 2, "2026-11-10T20:00:00Z"}, {alice, 1, "2026-10-01T20:00:00Z"}, {bob, 2, "2026-11-02T20:00:00Z"}} {
		if _, _, err := s.LogWatch(ctx, w.user, WatchInput{Kind: "tv", TmdbID: "1396", SeasonNumber: new(int64(1)), EpisodeNumber: new(w.episode), EndedAt: w.at}); err != nil {
			t.Fatalf("log: %v", err)
		}
	}

	got, err := s.Episode(ctx, alice, "1396", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Live || got.Episode.Overview != "Walt and Jesse…" || got.Show.Title != "Breaking Bad" {
		t.Fatalf("episode = %+v", got)
	}
	if len(got.Watches) != 2 || got.Watches[0].WatchedOn != "2026-11-10" {
		t.Fatalf("watches = %+v, want alice's two of S1E2, newest first", got.Watches)
	}

	// TMDB down: the cached basics, not an error.
	live.err = errors.New("boom")
	got, err = s.Episode(ctx, alice, "1396", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Live || got.Episode.Name != "Cat in the Bag" || got.Episode.Runtime != 48 || got.Episode.Overview != "" {
		t.Fatalf("fallback = %+v", got.Episode)
	}

	// An episode nobody has: 404.
	live.err = nil
	if _, err := s.Episode(ctx, alice, "1396", 1, 99); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unknown episode err = %v, want sql.ErrNoRows", err)
	}
	// Bad input: 400.
	var ve *ValidationError
	if _, err := s.Episode(ctx, alice, "1396", 1, 0); !errors.As(err, &ve) {
		t.Fatalf("episode 0 err = %v, want a ValidationError", err)
	}
	if _, err := s.Episode(ctx, alice, "abc", 1, 1); !errors.As(err, &ve) {
		t.Fatalf("bad id err = %v, want a ValidationError", err)
	}
}
