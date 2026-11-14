package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

func TestMergeVenueRefs(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := repository.NewTrackingRepository(db)
	dup, _ := repo.CreateVenue(ctx, repository.Venue{Name: "Dup", Status: "open"}, 0)
	kept, _ := repo.CreateVenue(ctx, repository.Venue{Name: "Kept", Status: "open"}, 0)
	other, _ := repo.CreateVenue(ctx, repository.Venue{Name: "Old", Status: "closed"}, 0)
	if _, err := repo.UpdateVenue(ctx, repository.Venue{ID: other, Name: "Old", Status: "closed", ReplacedByVenueID: &dup}); err != nil {
		t.Fatal(err)
	}
	_ = repo.SaveVenueRef(ctx, dup, repository.VenueRef{Source: "osm_id", Ref: "node/1"})
	_ = repo.SaveVenueRef(ctx, dup, repository.VenueRef{Source: "wikidata_id", Ref: "Q1"})
	_ = repo.SaveVenueRef(ctx, kept, repository.VenueRef{Source: "wikidata_id", Ref: "Q2"})

	res, err := repo.MergeVenues(ctx, dup, kept)
	if err != nil {
		t.Fatal(err)
	}
	refs, _ := repo.VenueRefs(ctx, kept)
	got := map[string]string{}
	for _, r := range refs {
		got[r.Source] = r.Ref
	}
	// The OSM ref moves; the kept venue's own Wikidata ID wins.
	if res.Refs != 1 || got["osm_id"] != "node/1" || got["wikidata_id"] != "Q2" || len(got) != 2 {
		t.Fatalf("refs = %+v, result %+v", got, res)
	}
	if v, _ := repo.Venue(ctx, other); v.ReplacedByVenueID == nil || *v.ReplacedByVenueID != kept {
		t.Fatalf("replaced_by not repointed: %+v", v)
	}
	if _, err := repo.MergeVenues(ctx, dup, kept); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("merging a gone venue: %v", err)
	}
}
