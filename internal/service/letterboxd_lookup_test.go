package service

import (
	"context"
	"errors"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

type fakeLetterboxd struct {
	ref client.LetterboxdRef
	err error
}

func (f fakeLetterboxd) Resolve(context.Context, string) (client.LetterboxdRef, error) {
	return f.ref, f.err
}

func TestLookupLetterboxd(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	s := NewTitleOverviewService(repository.NewAnalyticsRepository(db), repository.NewFlixPatrolRepository(db), repository.NewMetadataRepository(db), nil)

	var vErr *ValidationError
	if _, err := s.LookupLetterboxd(ctx, "https://example.com/film/x/"); !errors.As(err, &vErr) {
		t.Errorf("non-Letterboxd link: err = %v, want ValidationError", err)
	}
	if _, err := s.LookupLetterboxd(ctx, "https://letterboxd.com/film/x/"); !errors.Is(err, ErrUpstream) {
		t.Errorf("no resolver: err = %v, want ErrUpstream", err)
	}

	// An uncharted film still comes back: Letterboxd vouches for the TMDB id.
	s.WithLetterboxd(fakeLetterboxd{ref: client.LetterboxdRef{Kind: "movie", TmdbID: "693134"}})
	out, err := s.LookupLetterboxd(ctx, "https://boxd.it/abc")
	if err != nil || len(out.Refs) != 1 || out.Refs[0] != (LookupRef{Kind: "movie", TmdbID: "693134"}) {
		t.Fatalf("resolved: %+v, %v", out, err)
	}

	for _, tc := range []struct {
		err  error
		want func(error) bool
	}{
		{client.ErrLetterboxdNotFilm, func(e error) bool { return errors.As(e, &vErr) }},
		{client.ErrLetterboxdNoTMDB, func(e error) bool { return errors.Is(e, ErrTitleNotFound) }},
		{errors.New("letterboxd film page: HTTP 403"), func(e error) bool { return errors.Is(e, ErrUpstream) }},
	} {
		s.WithLetterboxd(fakeLetterboxd{err: tc.err})
		if _, err := s.LookupLetterboxd(ctx, "https://letterboxd.com/film/x/"); !tc.want(err) {
			t.Errorf("resolver error %v: got %v", tc.err, err)
		}
	}
}
