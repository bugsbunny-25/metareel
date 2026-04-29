-- Titles (movies + tv shows) + daily rankings (Top 10).

-- name: UpsertTitle :one
INSERT INTO titles (slug, name, kind, tmdb_id, imdb_id, rt_url)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(slug) DO UPDATE SET
    name       = excluded.name,
    kind       = excluded.kind,
    tmdb_id    = COALESCE(excluded.tmdb_id, titles.tmdb_id),
    imdb_id    = COALESCE(excluded.imdb_id, titles.imdb_id),
    rt_url     = COALESCE(excluded.rt_url, titles.rt_url),
    updated_at = CURRENT_TIMESTAMP
RETURNING id, slug, name, kind, tmdb_id, imdb_id, rt_url, created_at, updated_at;

-- name: GetTitleBySlug :one
SELECT id, slug, name, kind, tmdb_id, imdb_id, rt_url, created_at, updated_at
FROM titles
WHERE slug = ?;

-- name: GetTitleByID :one
SELECT id, slug, name, kind, tmdb_id, imdb_id, rt_url, created_at, updated_at
FROM titles
WHERE id = ?;

-- name: UpdateTitleIDs :one
UPDATE titles
SET
    tmdb_id    = ?,
    imdb_id    = ?,
    rt_url     = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING id, slug, name, kind, tmdb_id, imdb_id, rt_url, created_at, updated_at;

-- name: ListTitles :many
SELECT id, slug, name, kind, tmdb_id, imdb_id, rt_url, created_at, updated_at
FROM titles
WHERE (? = '' OR kind = ?)
  AND (? = '' OR name LIKE '%' || ? || '%')
ORDER BY id ASC
LIMIT ? OFFSET ?;

-- name: CountTitles :one
SELECT COUNT(*)
FROM titles
WHERE (? = '' OR kind = ?)
  AND (? = '' OR name LIKE '%' || ? || '%');

-- name: UpsertRanking :one
INSERT INTO rankings (title_id, ranked_on, country, streaming_provider, category, rank, season_number)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(ranked_on, country, streaming_provider, category, rank) DO UPDATE SET
    title_id      = excluded.title_id,
    season_number = excluded.season_number,
    scraped_at    = CURRENT_TIMESTAMP
RETURNING id, title_id, ranked_on, country, streaming_provider, category, rank, season_number, scraped_at;

