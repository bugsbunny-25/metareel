-- The premium-format taxonomy admins maintain, the venues and screens a
-- watch can point at, and the context columns on a watch.

-- name: ListWatchFormats :many
SELECT id, slug, label, category, note, sort_order, active, created_at, updated_at, logo_version, logo_display
FROM watch_formats
WHERE (CAST(sqlc.arg(active_only) AS INTEGER) = 0 OR active = 1)
ORDER BY sort_order, label COLLATE NOCASE;

-- name: GetWatchFormat :one
SELECT id, slug, label, category, note, sort_order, active, created_at, updated_at, logo_version, logo_display
FROM watch_formats WHERE id = sqlc.arg(id);

-- name: GetWatchFormatBySlug :one
SELECT id, slug, label, category, note, sort_order, active, created_at, updated_at, logo_version, logo_display
FROM watch_formats WHERE slug = sqlc.arg(slug);

-- name: CreateWatchFormat :one
INSERT INTO watch_formats (slug, label, category, note, sort_order, active)
VALUES (sqlc.arg(slug), sqlc.arg(label), sqlc.arg(category), sqlc.narg(note),
        sqlc.arg(sort_order), sqlc.arg(active))
RETURNING id, slug, label, category, note, sort_order, active, created_at, updated_at, logo_version, logo_display;

-- UpdateWatchFormat changes a format in place. A format is deactivated
-- rather than deleted, because watches reference it for good.
-- name: UpdateWatchFormat :execrows
UPDATE watch_formats SET
    label = sqlc.arg(label),
    category = sqlc.arg(category),
    note = sqlc.narg(note),
    sort_order = sqlc.arg(sort_order),
    active = sqlc.arg(active),
    logo_display = sqlc.arg(logo_display),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: ClearWatchEntryFormats :exec
DELETE FROM watch_entry_formats WHERE entry_id = sqlc.arg(entry_id);

-- name: AddWatchEntryFormat :exec
INSERT INTO watch_entry_formats (entry_id, format_id)
VALUES (sqlc.arg(entry_id), sqlc.arg(format_id))
ON CONFLICT(entry_id, format_id) DO NOTHING;

-- ListFormatsForWatchEntries hydrates a page of watches with their formats.
-- The slice is the last parameter, as sqlc requires.
-- name: ListFormatsForWatchEntries :many
SELECT ef.entry_id, f.id AS format_id, f.slug, f.label, f.category, f.logo_version, f.logo_display
FROM watch_entry_formats ef
JOIN watch_formats f ON f.id = ef.format_id
JOIN watch_entries w ON w.id = ef.entry_id
WHERE w.user_id = sqlc.arg(user_id)
  AND ef.entry_id IN (sqlc.slice(entry_ids))
ORDER BY ef.entry_id, f.sort_order, f.label COLLATE NOCASE;

-- SetWatchEntryContext writes the where-and-how columns of a watch. They are
-- set apart from the insert so the insert keeps a readable parameter list.
-- name: SetWatchEntryContext :execrows
UPDATE watch_entries SET
    venue_id = sqlc.narg(venue_id),
    screen_id = sqlc.narg(screen_id),
    venue_name_at_watch = sqlc.narg(venue_name_at_watch),
    venue_city_at_watch = sqlc.narg(venue_city_at_watch),
    seat = sqlc.narg(seat),
    screening_type = sqlc.narg(screening_type),
    event_name = sqlc.narg(event_name),
    with_qa = sqlc.arg(with_qa),
    price_minor = sqlc.narg(price_minor),
    fees_minor = sqlc.narg(fees_minor),
    concessions_minor = sqlc.narg(concessions_minor),
    currency = sqlc.narg(currency),
    tickets_count = sqlc.narg(tickets_count),
    booking_source = sqlc.narg(booking_source),
    provider_id = sqlc.narg(provider_id),
    provider_name = sqlc.narg(provider_name),
    monetization = sqlc.narg(monetization),
    media = sqlc.narg(media),
    edition = sqlc.narg(edition),
    device = sqlc.narg(device),
    dimension = sqlc.narg(dimension),
    resolution = sqlc.narg(resolution),
    hdr = sqlc.narg(hdr),
    audio_format = sqlc.narg(audio_format),
    frame_rate = sqlc.narg(frame_rate),
    audio_language = sqlc.narg(audio_language),
    subtitle_language = sqlc.narg(subtitle_language),
    content_aspect_ratio_text = sqlc.narg(content_aspect_ratio_text),
    content_aspect_ratio = sqlc.narg(content_aspect_ratio),
    content_aspect_variable = sqlc.arg(content_aspect_variable),
    screen_aspect_ratio = sqlc.narg(screen_aspect_ratio),
    screen_width = sqlc.narg(screen_width),
    screen_height = sqlc.narg(screen_height),
    screen_dim_unit = sqlc.narg(screen_dim_unit),
    screen_width_m = sqlc.narg(screen_width_m),
    screen_height_m = sqlc.narg(screen_height_m),
    subscription_id = sqlc.narg(subscription_id),
    face_value_minor = sqlc.narg(face_value_minor),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: CreateVenue :one
INSERT INTO venues (name, chain, address, city, region, country, latitude, longitude,
                    timezone, opened_on, closed_on, status, note, created_by)
VALUES (sqlc.arg(name), sqlc.narg(chain), sqlc.narg(address), sqlc.narg(city), sqlc.narg(region),
        sqlc.narg(country), sqlc.narg(latitude), sqlc.narg(longitude), sqlc.narg(timezone),
        sqlc.narg(opened_on), sqlc.narg(closed_on), sqlc.arg(status), sqlc.narg(note),
        sqlc.narg(created_by))
RETURNING id;

-- name: UpdateVenue :execrows
UPDATE venues SET
    name = sqlc.arg(name),
    chain = sqlc.narg(chain),
    address = sqlc.narg(address),
    city = sqlc.narg(city),
    region = sqlc.narg(region),
    country = sqlc.narg(country),
    latitude = sqlc.narg(latitude),
    longitude = sqlc.narg(longitude),
    timezone = sqlc.narg(timezone),
    opened_on = sqlc.narg(opened_on),
    closed_on = sqlc.narg(closed_on),
    status = sqlc.arg(status),
    replaced_by_venue_id = sqlc.narg(replaced_by_venue_id),
    note = sqlc.narg(note),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: GetVenue :one
SELECT id, name, chain, address, city, region, country, latitude, longitude, timezone,
       opened_on, closed_on, status, replaced_by_venue_id, note, created_by, created_at, updated_at
FROM venues WHERE id = sqlc.arg(id);

-- ListVenues searches the shared venue list: each word of the query (up to
-- four) must appear in the name, chain, city or address, so "pvr mumbai"
-- finds a PVR in Mumbai. Names starting with the first word come first,
-- then open cinemas before closed ones.
-- name: ListVenues :many
SELECT v.id, v.name, v.chain, v.address, v.city, v.region, v.country, v.latitude, v.longitude,
       v.timezone, v.opened_on, v.closed_on, v.status, v.replaced_by_venue_id, v.note,
       v.created_by, v.created_at, v.updated_at,
       CAST((SELECT COUNT(*) FROM venue_screens s WHERE s.venue_id = v.id) AS INTEGER) AS screen_count
FROM venues v,
     (SELECT CAST(sqlc.arg(w1) AS TEXT) AS w1, CAST(sqlc.arg(w2) AS TEXT) AS w2,
             CAST(sqlc.arg(w3) AS TEXT) AS w3, CAST(sqlc.arg(w4) AS TEXT) AS w4) q
WHERE (q.w1 = '' OR (v.name || ' ' || COALESCE(v.chain, '') || ' ' || COALESCE(v.city, '') || ' ' || COALESCE(v.address, '')) LIKE '%' || q.w1 || '%')
  AND (q.w2 = '' OR (v.name || ' ' || COALESCE(v.chain, '') || ' ' || COALESCE(v.city, '') || ' ' || COALESCE(v.address, '')) LIKE '%' || q.w2 || '%')
  AND (q.w3 = '' OR (v.name || ' ' || COALESCE(v.chain, '') || ' ' || COALESCE(v.city, '') || ' ' || COALESCE(v.address, '')) LIKE '%' || q.w3 || '%')
  AND (q.w4 = '' OR (v.name || ' ' || COALESCE(v.chain, '') || ' ' || COALESCE(v.city, '') || ' ' || COALESCE(v.address, '')) LIKE '%' || q.w4 || '%')
  AND (CAST(sqlc.arg(country) AS TEXT) = '' OR v.country = sqlc.arg(country))
  AND (CAST(sqlc.arg(include_closed) AS INTEGER) = 1 OR v.status <> 'closed')
ORDER BY CASE WHEN q.w1 <> '' AND v.name LIKE q.w1 || '%' THEN 0 ELSE 1 END,
         CASE WHEN v.status = 'open' THEN 0 ELSE 1 END,
         v.name COLLATE NOCASE, v.id
LIMIT sqlc.arg(limit) OFFSET sqlc.arg(offset);

-- CountVenueWatches reports how many watches point at a venue, so one that
-- is in use is never deleted out from under them.
-- name: CountVenueWatches :one
SELECT COUNT(*) FROM watch_entries WHERE venue_id = sqlc.arg(venue_id);

-- name: DeleteVenue :execrows
DELETE FROM venues WHERE id = sqlc.arg(id);

-- name: AddVenueAlias :one
INSERT INTO venue_aliases (venue_id, name, from_date, to_date)
VALUES (sqlc.arg(venue_id), sqlc.arg(name), sqlc.narg(from_date), sqlc.narg(to_date))
RETURNING id, venue_id, name, from_date, to_date;

-- name: ListVenueAliases :many
SELECT id, venue_id, name, from_date, to_date
FROM venue_aliases WHERE venue_id = sqlc.arg(venue_id)
ORDER BY COALESCE(from_date, ''), id;

-- name: DeleteVenueAlias :execrows
DELETE FROM venue_aliases WHERE id = sqlc.arg(id) AND venue_id = sqlc.arg(venue_id);

-- name: UpsertVenueRef :exec
INSERT INTO venue_refs (venue_id, source, ref)
VALUES (sqlc.arg(venue_id), sqlc.arg(source), sqlc.arg(ref))
ON CONFLICT(venue_id, source) DO UPDATE SET ref = excluded.ref;

-- name: ListVenueRefs :many
SELECT venue_id, source, ref FROM venue_refs
WHERE venue_id = sqlc.arg(venue_id) ORDER BY source;

-- name: FindVenueByRef :one
SELECT venue_id FROM venue_refs
WHERE source = sqlc.arg(source) AND ref = sqlc.arg(ref);

-- name: CreateVenueScreen :one
INSERT INTO venue_screens (venue_id, name, number, seats, width, height, dim_unit,
                           width_m, height_m, aspect_ratio, effective_from, effective_to, note)
VALUES (sqlc.arg(venue_id), sqlc.arg(name), sqlc.narg(number), sqlc.narg(seats),
        sqlc.narg(width), sqlc.narg(height), sqlc.narg(dim_unit),
        sqlc.narg(width_m), sqlc.narg(height_m), sqlc.narg(aspect_ratio),
        sqlc.narg(effective_from), sqlc.narg(effective_to), sqlc.narg(note))
RETURNING id;

-- name: UpdateVenueScreen :execrows
UPDATE venue_screens SET
    name = sqlc.arg(name),
    number = sqlc.narg(number),
    seats = sqlc.narg(seats),
    width = sqlc.narg(width),
    height = sqlc.narg(height),
    dim_unit = sqlc.narg(dim_unit),
    width_m = sqlc.narg(width_m),
    height_m = sqlc.narg(height_m),
    aspect_ratio = sqlc.narg(aspect_ratio),
    effective_from = sqlc.narg(effective_from),
    effective_to = sqlc.narg(effective_to),
    note = sqlc.narg(note),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND venue_id = sqlc.arg(venue_id);

-- name: ListVenueScreens :many
SELECT id, venue_id, name, number, seats, width, height, dim_unit, width_m, height_m,
       aspect_ratio, effective_from, effective_to, note, created_at, updated_at
FROM venue_screens WHERE venue_id = sqlc.arg(venue_id)
ORDER BY COALESCE(number, 9999), name COLLATE NOCASE;

-- name: GetVenueScreen :one
SELECT id, venue_id, name, number, seats, width, height, dim_unit, width_m, height_m,
       aspect_ratio, effective_from, effective_to, note, created_at, updated_at
FROM venue_screens WHERE id = sqlc.arg(id);

-- name: DeleteVenueScreen :execrows
DELETE FROM venue_screens WHERE id = sqlc.arg(id) AND venue_id = sqlc.arg(venue_id);

-- name: SaveWatchFormatLogo :exec
INSERT INTO watch_format_logos (format_id, content_type, data, updated_at)
VALUES (sqlc.arg(format_id), sqlc.arg(content_type), sqlc.arg(data), CURRENT_TIMESTAMP)
ON CONFLICT (format_id) DO UPDATE SET
    content_type = excluded.content_type, data = excluded.data, updated_at = CURRENT_TIMESTAMP;

-- name: SetWatchFormatLogoVersion :execrows
UPDATE watch_formats SET logo_version = sqlc.narg(logo_version), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: DeleteWatchFormatLogo :exec
DELETE FROM watch_format_logos WHERE format_id = sqlc.arg(format_id);

-- name: GetWatchFormatLogo :one
SELECT l.content_type, l.data, CAST(COALESCE(f.logo_version, '') AS TEXT) AS logo_version
FROM watch_formats f
JOIN watch_format_logos l ON l.format_id = f.id
WHERE f.slug = sqlc.arg(slug);
