package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// VenueMergeResult counts what moved when one cinema was merged into another.
type VenueMergeResult struct {
	Watches int64 `json:"watches"`
	Screens int64 `json:"screens"`
	Aliases int64 `json:"aliases"` // former names moved, plus the merged venue's own name
	Refs    int64 `json:"refs"`    // outside IDs moved (one the kept venue already has of that kind is dropped)
	Filled  bool  `json:"filled"`  // the kept venue gained empty fields
}

// MergeVenues moves everything that points at venue from onto venue into,
// records from's name as a former name of into, fills into's empty fields
// from it and deletes from, in one transaction. Watches keep the name and
// city they were logged with. sql.ErrNoRows when either venue is missing.
func (r *TrackingRepository) MergeVenues(ctx context.Context, from, into int64) (VenueMergeResult, error) {
	var res VenueMergeResult
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback() //nolint:errcheck
	q := r.q.WithTx(tx)

	src, err := q.GetVenue(ctx, from)
	if err != nil {
		return res, err
	}
	dst, err := q.GetVenue(ctx, into)
	if err != nil {
		return res, err
	}
	nf := sql.NullInt64{Int64: from, Valid: true}
	ni := sql.NullInt64{Int64: into, Valid: true}
	if res.Watches, err = q.MoveVenueWatches(ctx, sqlc.MoveVenueWatchesParams{FromID: nf, IntoID: ni}); err != nil {
		return res, fmt.Errorf("move watches: %w", err)
	}
	if res.Screens, err = q.MoveVenueScreens(ctx, sqlc.MoveVenueScreensParams{FromID: from, IntoID: into}); err != nil {
		return res, fmt.Errorf("move screens: %w", err)
	}

	// Former names: the merged venue's own name joins them unless the kept
	// venue already goes by it.
	known := map[string]bool{strings.ToLower(strings.TrimSpace(dst.Name)): true}
	aliases, err := q.ListVenueAliases(ctx, into)
	if err != nil {
		return res, err
	}
	for _, a := range aliases {
		known[strings.ToLower(strings.TrimSpace(a.Name))] = true
	}
	if res.Aliases, err = q.MoveVenueAliases(ctx, sqlc.MoveVenueAliasesParams{FromID: from, IntoID: into}); err != nil {
		return res, fmt.Errorf("move aliases: %w", err)
	}
	if name := strings.TrimSpace(src.Name); !known[strings.ToLower(name)] {
		if _, err := q.AddVenueAlias(ctx, sqlc.AddVenueAliasParams{VenueID: into, Name: name, ToDate: src.ClosedOn}); err != nil {
			return res, fmt.Errorf("add alias: %w", err)
		}
		res.Aliases++
	}

	refs, err := q.ListVenueRefs(ctx, from)
	if err != nil {
		return res, err
	}
	for _, ref := range refs {
		// Free the ref first: (source, ref) is unique across venues.
		if err := q.DeleteVenueRef(ctx, sqlc.DeleteVenueRefParams{VenueID: from, Source: ref.Source}); err != nil {
			return res, fmt.Errorf("move ref: %w", err)
		}
		n, err := q.AddVenueRefIfFree(ctx, sqlc.AddVenueRefIfFreeParams{VenueID: into, Source: ref.Source, Ref: ref.Ref})
		if err != nil {
			return res, fmt.Errorf("move ref: %w", err)
		}
		res.Refs += n
	}

	if _, err := q.RepointVenueReplacedBy(ctx, sqlc.RepointVenueReplacedByParams{FromID: nf, IntoID: ni}); err != nil {
		return res, fmt.Errorf("repoint replaced_by: %w", err)
	}
	n, err := q.FillVenueBlanksFrom(ctx, sqlc.FillVenueBlanksFromParams{FromID: from, IntoID: into})
	if err != nil {
		return res, fmt.Errorf("fill venue: %w", err)
	}
	after, err := q.GetVenue(ctx, into)
	if err != nil {
		return res, err
	}
	res.Filled = n > 0 && venueRowChanged(dst, after)
	if _, err := q.DeleteVenue(ctx, from); err != nil {
		return res, fmt.Errorf("delete merged venue: %w", err)
	}
	return res, tx.Commit()
}

func venueRowChanged(a, b sqlc.Venue) bool {
	return a.Chain != b.Chain || a.Address != b.Address || a.City != b.City || a.Region != b.Region || a.Country != b.Country ||
		a.Latitude != b.Latitude || a.Longitude != b.Longitude || a.Timezone != b.Timezone || a.OpenedOn != b.OpenedOn ||
		a.ClosedOn != b.ClosedOn || a.Note != b.Note
}
