-- Cinema import (venues.import): every venue of a country with the refs
-- the import matches on, and a fill-in update that never overwrites a value
-- someone already set.

-- name: ListVenuesForImport :many
SELECT v.id, v.name, v.city, v.latitude, v.longitude, v.status,
       CAST(COALESCE((SELECT r.ref FROM venue_refs r WHERE r.venue_id = v.id AND r.source = 'osm_id'), '') AS TEXT) AS osm_ref,
       CAST(COALESCE((SELECT r.ref FROM venue_refs r WHERE r.venue_id = v.id AND r.source = 'wikidata_id'), '') AS TEXT) AS wikidata_ref
FROM venues v
WHERE v.country = sqlc.arg(country)
ORDER BY v.id;

-- name: FillVenueBlanks :execrows
UPDATE venues SET
    chain     = COALESCE(chain, sqlc.narg(chain)),
    address   = COALESCE(address, sqlc.narg(address)),
    city      = COALESCE(city, sqlc.narg(city)),
    region    = COALESCE(region, sqlc.narg(region)),
    latitude  = COALESCE(latitude, sqlc.narg(latitude)),
    longitude = COALESCE(longitude, sqlc.narg(longitude)),
    timezone  = COALESCE(timezone, sqlc.narg(timezone)),
    opened_on = COALESCE(opened_on, sqlc.narg(opened_on)),
    closed_on = CASE WHEN status = 'closed' THEN COALESCE(closed_on, sqlc.narg(closed_on)) ELSE closed_on END,
    note      = COALESCE(note, sqlc.narg(note)),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
  AND ((chain IS NULL AND sqlc.narg(chain) IS NOT NULL)
    OR (address IS NULL AND sqlc.narg(address) IS NOT NULL)
    OR (city IS NULL AND sqlc.narg(city) IS NOT NULL)
    OR (region IS NULL AND sqlc.narg(region) IS NOT NULL)
    OR (latitude IS NULL AND sqlc.narg(latitude) IS NOT NULL)
    OR (longitude IS NULL AND sqlc.narg(longitude) IS NOT NULL)
    OR (timezone IS NULL AND sqlc.narg(timezone) IS NOT NULL)
    OR (opened_on IS NULL AND sqlc.narg(opened_on) IS NOT NULL)
    OR (status = 'closed' AND closed_on IS NULL AND sqlc.narg(closed_on) IS NOT NULL)
    OR (note IS NULL AND sqlc.narg(note) IS NOT NULL));

-- name: AddVenueRefIfFree :execrows
INSERT INTO venue_refs (venue_id, source, ref)
VALUES (sqlc.arg(venue_id), sqlc.arg(source), sqlc.arg(ref))
ON CONFLICT DO NOTHING;
