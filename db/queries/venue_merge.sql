-- Merging one cinema into another (the console's "Merge into..."): every row
-- that points at the merged venue moves to the one kept.

-- name: MoveVenueWatches :execrows
UPDATE watch_entries SET venue_id = sqlc.arg(into_id) WHERE venue_id = sqlc.arg(from_id);

-- name: MoveVenueScreens :execrows
UPDATE venue_screens SET venue_id = sqlc.arg(into_id), updated_at = CURRENT_TIMESTAMP
WHERE venue_id = sqlc.arg(from_id);

-- name: MoveVenueAliases :execrows
UPDATE venue_aliases SET venue_id = sqlc.arg(into_id) WHERE venue_id = sqlc.arg(from_id);

-- name: RepointVenueReplacedBy :execrows
UPDATE venues SET replaced_by_venue_id = sqlc.arg(into_id), updated_at = CURRENT_TIMESTAMP
WHERE replaced_by_venue_id = sqlc.arg(from_id) AND id <> sqlc.arg(into_id);

-- name: DeleteVenueRef :exec
DELETE FROM venue_refs WHERE venue_id = sqlc.arg(venue_id) AND source = sqlc.arg(source);

-- name: FillVenueBlanksFrom :execrows
UPDATE venues SET
    chain = COALESCE(chain, (SELECT s.chain FROM venues s WHERE s.id = sqlc.arg(from_id))),
    address = COALESCE(address, (SELECT s.address FROM venues s WHERE s.id = sqlc.arg(from_id))),
    city = COALESCE(city, (SELECT s.city FROM venues s WHERE s.id = sqlc.arg(from_id))),
    region = COALESCE(region, (SELECT s.region FROM venues s WHERE s.id = sqlc.arg(from_id))),
    country = COALESCE(country, (SELECT s.country FROM venues s WHERE s.id = sqlc.arg(from_id))),
    latitude = COALESCE(latitude, (SELECT s.latitude FROM venues s WHERE s.id = sqlc.arg(from_id))),
    longitude = COALESCE(longitude, (SELECT s.longitude FROM venues s WHERE s.id = sqlc.arg(from_id))),
    timezone = COALESCE(timezone, (SELECT s.timezone FROM venues s WHERE s.id = sqlc.arg(from_id))),
    opened_on = COALESCE(opened_on, (SELECT s.opened_on FROM venues s WHERE s.id = sqlc.arg(from_id))),
    closed_on = COALESCE(closed_on, (SELECT s.closed_on FROM venues s WHERE s.id = sqlc.arg(from_id))),
    note = COALESCE(note, (SELECT s.note FROM venues s WHERE s.id = sqlc.arg(from_id))),
    updated_at = CURRENT_TIMESTAMP
WHERE venues.id = sqlc.arg(into_id);
