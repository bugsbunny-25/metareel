package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// ImportVenueRow is an existing venue as the cinema import sees it.
type ImportVenueRow struct {
	ID          int64
	Name        string
	City        *string
	Latitude    *float64
	Longitude   *float64
	OSMRef      string // venue_refs osm_id, "" if none
	WikidataRef string
}

// VenuesForImport lists a country's venues with their OSM and Wikidata refs.
func (r *TrackingRepository) VenuesForImport(ctx context.Context, country string) ([]ImportVenueRow, error) {
	rows, err := r.q.ListVenuesForImport(ctx, sql.NullString{String: country, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list venues for import: %w", err)
	}
	out := make([]ImportVenueRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, ImportVenueRow{ID: row.ID, Name: row.Name, City: nullStringPtr(row.City),
			Latitude: nullFloatPtr(row.Latitude), Longitude: nullFloatPtr(row.Longitude), OSMRef: row.OsmRef, WikidataRef: row.WikidataRef})
	}
	return out, nil
}

// VenueImportCreate adds a venue; VenueImportFill fills an existing one's
// empty fields (never overwriting a value) and adds refs it lacks.
type VenueImportCreate struct {
	Venue Venue
	Refs  []VenueRef
}

type VenueImportFill struct {
	ID    int64
	Venue Venue
	Refs  []VenueRef
}

// VenueImportResult counts what ApplyVenueImport changed.
type VenueImportResult struct {
	Created     int
	Filled      int // existing venues that gained at least one field
	RefsAdded   int
	RefsSkipped int // the ref already belongs to another venue
}

// ApplyVenueImport writes one country's import in a single transaction. A
// ref is only added where it is free: one already held by a venue (or a
// venue already holding that kind of ref) is left alone.
func (r *TrackingRepository) ApplyVenueImport(ctx context.Context, creates []VenueImportCreate, fills []VenueImportFill) (VenueImportResult, error) {
	var res VenueImportResult
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback() //nolint:errcheck
	q := r.q.WithTx(tx)
	addRefs := func(id int64, refs []VenueRef) error {
		for _, ref := range refs {
			n, err := q.AddVenueRefIfFree(ctx, sqlc.AddVenueRefIfFreeParams{VenueID: id, Source: ref.Source, Ref: ref.Ref})
			if err != nil {
				return fmt.Errorf("add venue %d ref %s: %w", id, ref.Source, err)
			}
			if n > 0 {
				res.RefsAdded++
			} else {
				res.RefsSkipped++
			}
		}
		return nil
	}
	for _, c := range creates {
		v := c.Venue
		id, err := q.CreateVenue(ctx, sqlc.CreateVenueParams{
			Name: v.Name, Chain: toNullString(v.Chain), Address: toNullString(v.Address),
			City: toNullString(v.City), Region: toNullString(v.Region), Country: toNullString(v.Country),
			Latitude: floatPtrNull(v.Latitude), Longitude: floatPtrNull(v.Longitude),
			Timezone: toNullString(v.Timezone), OpenedOn: toNullString(v.OpenedOn), Status: "open",
		})
		if err != nil {
			return res, fmt.Errorf("create venue %q: %w", v.Name, err)
		}
		res.Created++
		if err := addRefs(id, c.Refs); err != nil {
			return res, err
		}
	}
	for _, f := range fills {
		v := f.Venue
		n, err := q.FillVenueBlanks(ctx, sqlc.FillVenueBlanksParams{ID: f.ID,
			Chain: toNullString(v.Chain), Address: toNullString(v.Address), City: toNullString(v.City), Region: toNullString(v.Region),
			Latitude: floatPtrNull(v.Latitude), Longitude: floatPtrNull(v.Longitude), Timezone: toNullString(v.Timezone), OpenedOn: toNullString(v.OpenedOn)})
		if err != nil {
			return res, fmt.Errorf("fill venue %d: %w", f.ID, err)
		}
		if n > 0 {
			res.Filled++
		}
		if err := addRefs(f.ID, f.Refs); err != nil {
			return res, err
		}
	}
	return res, tx.Commit()
}
