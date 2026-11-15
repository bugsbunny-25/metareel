-- Physical media: shared disc releases (with box-set parts, titles and
-- images), each user's copies, and the barcode lookup cache.

-- name: InsertDiscRelease :one
INSERT INTO disc_releases (parent_id, position, barcode, title, edition, media_format, packaging,
    has_slipcover, region, country, studio, label, release_date, discs, special_features, notes,
    source, created_by)
VALUES (sqlc.narg(parent_id), sqlc.arg(position), sqlc.narg(barcode), sqlc.arg(title), sqlc.arg(edition),
    sqlc.arg(media_format), sqlc.arg(packaging), sqlc.arg(has_slipcover), sqlc.arg(region),
    sqlc.narg(country), sqlc.arg(studio), sqlc.narg(label), sqlc.narg(release_date), sqlc.arg(discs),
    sqlc.arg(special_features), sqlc.narg(notes), sqlc.arg(source), sqlc.narg(created_by))
RETURNING id;

-- name: UpdateDiscRelease :execrows
UPDATE disc_releases
SET position = sqlc.arg(position), barcode = sqlc.narg(barcode), title = sqlc.arg(title),
    edition = sqlc.arg(edition), media_format = sqlc.arg(media_format), packaging = sqlc.arg(packaging),
    has_slipcover = sqlc.arg(has_slipcover), region = sqlc.arg(region), country = sqlc.narg(country),
    studio = sqlc.arg(studio), label = sqlc.narg(label), release_date = sqlc.narg(release_date),
    discs = sqlc.arg(discs), special_features = sqlc.arg(special_features), notes = sqlc.narg(notes),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: GetDiscRelease :one
SELECT * FROM disc_releases WHERE id = sqlc.arg(id);

-- name: GetDiscReleaseByBarcode :one
SELECT * FROM disc_releases WHERE barcode = sqlc.arg(barcode);

-- name: ListDiscReleaseParts :many
SELECT * FROM disc_releases WHERE parent_id = sqlc.arg(parent_id) ORDER BY position, id;

-- ListDiscReleasesByParents is the parts of several box sets at once.
-- name: ListDiscReleasesByParents :many
SELECT * FROM disc_releases WHERE parent_id IN (sqlc.slice(parent_ids)) ORDER BY parent_id, position, id;

-- name: DeleteDiscRelease :exec
DELETE FROM disc_releases WHERE id = sqlc.arg(id);

-- name: CountDiscReleaseCopies :one
SELECT COUNT(*) FROM disc_copies WHERE release_id = sqlc.arg(release_id);

-- name: DeleteDiscReleaseTitles :exec
DELETE FROM disc_release_titles WHERE release_id = sqlc.arg(release_id);

-- name: InsertDiscReleaseTitle :exec
INSERT INTO disc_release_titles (release_id, position, tmdb_kind, tmdb_id, season_number)
VALUES (sqlc.arg(release_id), sqlc.arg(position), sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id), sqlc.narg(season_number));

-- ListDiscReleaseTitles is what is on several releases, with the display
-- snapshot of each title (the charted row first, else the tracking cache).
-- name: ListDiscReleaseTitles :many
SELECT rt.release_id, rt.position, rt.tmdb_kind, rt.tmdb_id, rt.season_number,
    CAST(COALESCE(NULLIF(t.title, ''), NULLIF(c.title, ''), '') AS TEXT) AS title,
    CAST(COALESCE(NULLIF(t.release_date, ''), NULLIF(c.release_date, ''), '') AS TEXT) AS release_date,
    CAST(COALESCE(t.poster_path, c.poster_path, '') AS TEXT) AS poster_path
FROM disc_release_titles rt
LEFT JOIN tmdb_titles t ON t.tmdb_kind = rt.tmdb_kind AND t.tmdb_id = rt.tmdb_id
LEFT JOIN tracked_titles c ON c.tmdb_kind = rt.tmdb_kind AND c.tmdb_id = rt.tmdb_id
WHERE rt.release_id IN (sqlc.slice(release_ids))
ORDER BY rt.release_id, rt.position;

-- name: InsertDiscReleaseImage :one
INSERT INTO disc_release_images (release_id, kind, position, sha256, content_type, width, height, bytes, source, source_url, uploaded_by)
VALUES (sqlc.arg(release_id), sqlc.arg(kind), sqlc.arg(position), sqlc.arg(sha256), sqlc.arg(content_type), sqlc.arg(width),
    sqlc.arg(height), sqlc.arg(bytes), sqlc.arg(source), sqlc.narg(source_url), sqlc.narg(uploaded_by))
RETURNING *;

-- name: ListDiscReleaseImages :many
SELECT * FROM disc_release_images
WHERE release_id IN (sqlc.slice(release_ids))
ORDER BY release_id, position, id;

-- name: GetDiscReleaseImage :one
SELECT * FROM disc_release_images WHERE id = sqlc.arg(id) AND release_id = sqlc.arg(release_id);

-- name: UpdateDiscReleaseImage :execrows
UPDATE disc_release_images SET kind = sqlc.arg(kind), position = sqlc.arg(position)
WHERE id = sqlc.arg(id) AND release_id = sqlc.arg(release_id);

-- name: DeleteDiscReleaseImage :execrows
DELETE FROM disc_release_images WHERE id = sqlc.arg(id) AND release_id = sqlc.arg(release_id);

-- name: CountDiscImagesBySha :one
SELECT COUNT(*) FROM disc_release_images WHERE sha256 = sqlc.arg(sha256);

-- ListDiscImageShasForTree is every image file of a release and its parts,
-- so deleting a part can remove files nothing else uses.
-- name: ListDiscImageShasForTree :many
SELECT DISTINCT i.sha256 FROM disc_release_images i
JOIN disc_releases r ON r.id = i.release_id
WHERE r.id = sqlc.arg(release_id) OR r.parent_id = sqlc.arg(release_id);

-- name: MaxDiscImagePosition :one
SELECT CAST(COALESCE(MAX(position), -1) AS INTEGER) FROM disc_release_images
WHERE release_id = sqlc.arg(release_id) AND kind = sqlc.arg(kind);

-- name: InsertDiscCopy :one
INSERT INTO disc_copies (user_id, release_id, purchased_on, price_minor, currency, retailer, condition, notes)
VALUES (sqlc.arg(user_id), sqlc.arg(release_id), sqlc.narg(purchased_on), sqlc.narg(price_minor),
    sqlc.narg(currency), sqlc.arg(retailer), sqlc.arg(condition), sqlc.narg(notes))
RETURNING id;

-- name: UpdateDiscCopy :execrows
UPDATE disc_copies
SET purchased_on = sqlc.narg(purchased_on), price_minor = sqlc.narg(price_minor), currency = sqlc.narg(currency),
    retailer = sqlc.arg(retailer), condition = sqlc.arg(condition), notes = sqlc.narg(notes),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: DeleteDiscCopy :execrows
DELETE FROM disc_copies WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: GetDiscCopy :one
SELECT * FROM disc_copies WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: ListDiscCopiesOfRelease :many
SELECT * FROM disc_copies WHERE user_id = sqlc.arg(user_id) AND release_id = sqlc.arg(release_id) ORDER BY id;

-- ListDiscCopies is a user's shelf. tmdb_kind/tmdb_id keep the copies whose
-- release (or one of its parts) holds that title; q matches the printed
-- title, edition, studio or retailer.
-- name: ListDiscCopies :many
SELECT c.id, c.user_id, c.release_id, c.purchased_on, c.price_minor, c.currency, c.retailer,
    c.condition, c.notes, c.created_at, c.updated_at,
    r.title, r.edition, r.media_format, r.packaging, r.barcode, r.label, r.studio, r.has_slipcover, r.discs
FROM disc_copies c
JOIN disc_releases r ON r.id = c.release_id,
    (SELECT CAST(sqlc.arg(sort) AS TEXT) AS sort_key) o
WHERE c.user_id = sqlc.arg(user_id)
  AND (CAST(sqlc.arg(media_format) AS TEXT) = '' OR r.media_format = sqlc.arg(media_format)
       OR EXISTS (SELECT 1 FROM disc_releases p WHERE p.parent_id = r.id AND p.media_format = sqlc.arg(media_format)))
  AND (CAST(sqlc.arg(q) AS TEXT) = ''
       OR r.title LIKE '%' || sqlc.arg(q) || '%' OR r.edition LIKE '%' || sqlc.arg(q) || '%'
       OR r.studio LIKE '%' || sqlc.arg(q) || '%' OR c.retailer LIKE '%' || sqlc.arg(q) || '%')
  AND (CAST(sqlc.arg(tmdb_id) AS TEXT) = '' OR EXISTS (
       SELECT 1 FROM disc_release_titles rt JOIN disc_releases x ON x.id = rt.release_id
       WHERE (x.id = r.id OR x.parent_id = r.id)
         AND rt.tmdb_kind = sqlc.arg(tmdb_kind) AND rt.tmdb_id = sqlc.arg(tmdb_id)))
ORDER BY
    CASE WHEN o.sort_key = 'title' THEN r.title END COLLATE NOCASE ASC,
    CASE WHEN o.sort_key = 'purchased' THEN COALESCE(c.purchased_on, '') END DESC,
    CASE WHEN o.sort_key = 'price' THEN COALESCE(c.price_minor, -1) END DESC,
    CASE WHEN o.sort_key = 'format' THEN r.media_format END ASC,
    CASE WHEN o.sort_key = 'format' THEN r.title END COLLATE NOCASE ASC,
    c.id DESC
LIMIT 5000;

-- name: GetBarcodeLookup :one
SELECT * FROM disc_barcode_lookups WHERE barcode = sqlc.arg(barcode) AND provider = sqlc.arg(provider);

-- name: SaveBarcodeLookup :exec
INSERT INTO disc_barcode_lookups (barcode, provider, found, payload, fetched_at)
VALUES (sqlc.arg(barcode), sqlc.arg(provider), sqlc.arg(found), sqlc.arg(payload), CURRENT_TIMESTAMP)
ON CONFLICT (barcode, provider) DO UPDATE
SET found = excluded.found, payload = excluded.payload, fetched_at = excluded.fetched_at;
