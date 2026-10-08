-- On-demand ratings cache, keyed by TMDB namespace ('movie' | 'tv') + ID.

-- name: GetTitleRatings :one
SELECT id, tmdb_kind, tmdb_id, title, year, imdb_id, justwatch_id, rt_slug, refreshed_at, created_at, updated_at
FROM title_ratings
WHERE tmdb_kind = ? AND tmdb_id = ?;

-- name: UpsertTitleRatings :one
INSERT INTO title_ratings (tmdb_kind, tmdb_id, title, year, imdb_id, justwatch_id, rt_slug, refreshed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(tmdb_kind, tmdb_id) DO UPDATE SET
    title        = excluded.title,
    year         = excluded.year,
    imdb_id      = excluded.imdb_id,
    justwatch_id = excluded.justwatch_id,
    rt_slug      = excluded.rt_slug,
    refreshed_at = excluded.refreshed_at,
    updated_at   = CURRENT_TIMESTAMP
RETURNING id, tmdb_kind, tmdb_id, title, year, imdb_id, justwatch_id, rt_slug, refreshed_at, created_at, updated_at;

-- name: ListTitleRatingSources :many
SELECT provider, source, value, votes, url, fetched_at
FROM title_rating_sources
WHERE tmdb_kind = ? AND tmdb_id = ?
ORDER BY provider, source;

-- name: DeleteTitleRatingSourcesByProvider :exec
DELETE FROM title_rating_sources
WHERE tmdb_kind = ? AND tmdb_id = ? AND provider = ?;

-- name: InsertTitleRatingSource :exec
INSERT INTO title_rating_sources (tmdb_kind, tmdb_id, provider, source, value, votes, url, fetched_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- Back-fill IDs found while rating onto scraped titles that lack them.
-- name: FillTitleExternalIDs :exec
UPDATE titles
SET
    imdb_id    = COALESCE(imdb_id, sqlc.narg(imdb_id)),
    rt_url     = COALESCE(rt_url, sqlc.narg(rt_url)),
    updated_at = CURRENT_TIMESTAMP
WHERE tmdb_id = sqlc.arg(tmdb_id)
  AND kind = sqlc.arg(kind)
  AND ((imdb_id IS NULL AND sqlc.narg(imdb_id) IS NOT NULL)
    OR (rt_url IS NULL AND sqlc.narg(rt_url) IS NOT NULL));
