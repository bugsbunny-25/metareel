package service

import (
	"context"
	"errors"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// fakeDetails stores details for the IDs it knows, like TitleMetadataService.RefreshDetails.
type fakeDetails struct {
	repo  *repository.MetadataRepository
	known map[string]string // id -> title
	calls int
}

func (f *fakeDetails) RefreshDetails(ctx context.Context, ref repository.TmdbRef) (*client.TMDBFullDetails, error) {
	f.calls++
	title, ok := f.known[ref.ID]
	if !ok {
		return nil, nil
	}
	d := &client.TMDBFullDetails{Kind: ref.Kind, ID: ref.ID, Title: title, Overview: "A trending film.", ReleaseDate: "2026-11-01"}
	return d, f.repo.SaveDetails(ctx, d, true)
}

func TestOverviewFetchesDetailsForUnknownTitles(t *testing.T) {
	db := newTestDB(t)
	meta := repository.NewMetadataRepository(db)
	fake := &fakeDetails{repo: meta, known: map[string]string{"777": "Trending Film"}}
	svc := NewTitleOverviewService(repository.NewAnalyticsRepository(db), repository.NewFlixPatrolRepository(db), meta, nil).WithDetailsFetcher(fake)
	ctx := context.Background()
	q := OverviewQuery{Include: map[string]bool{"metadata": true}}

	out, err := svc.Overview(ctx, TmdbRef{Kind: "movie", ID: "777"}, q)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if out.Metadata == nil || out.Metadata.Title == nil || *out.Metadata.Title != "Trending Film" {
		t.Fatalf("metadata = %+v, want the fetched title", out.Metadata)
	}
	// Stored now: a second view reads it without fetching again.
	if _, err := svc.Overview(ctx, TmdbRef{Kind: "movie", ID: "777"}, q); err != nil || fake.calls != 1 {
		t.Fatalf("second overview: err %v, calls %d, want 1 call", err, fake.calls)
	}

	// A row with IDs but no details (a ratings lookup stores those first) still fetches.
	if err := meta.SaveIDs(ctx, TmdbRef{Kind: "movie", ID: "888"}, repository.TmdbIDs{IMDbID: "tt0000888"}, repository.IDSourceTMDB); err != nil {
		t.Fatalf("save ids: %v", err)
	}
	fake.known["888"] = "Rated First"
	out, err = svc.Overview(ctx, TmdbRef{Kind: "movie", ID: "888"}, q)
	if err != nil || out.Metadata == nil || out.Metadata.Title == nil || *out.Metadata.Title != "Rated First" {
		t.Fatalf("ids-only row: err %v, metadata %+v", err, out)
	}

	// Not on TMDB: 404, and asked once per retry window, not on every view.
	for range 3 {
		if _, err := svc.Overview(ctx, TmdbRef{Kind: "movie", ID: "999"}, q); !errors.Is(err, ErrTitleNotFound) {
			t.Fatalf("unknown title: err %v, want ErrTitleNotFound", err)
		}
	}
	if fake.calls != 3 {
		t.Fatalf("calls = %d, want 3 (one per title)", fake.calls)
	}
}
