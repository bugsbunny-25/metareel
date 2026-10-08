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
    kind       = ?,
    tmdb_id    = ?,
    imdb_id    = ?,
    rt_url     = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING id, slug, name, kind, tmdb_id, imdb_id, rt_url, created_at, updated_at;

-- name: ListTitles :many
-- sort: name_asc | name_desc | updated_desc | updated_asc | last_ranked_desc |
-- last_ranked_asc | rankings_desc | rankings_asc | id_asc; anything else is
-- newest first.
SELECT
    t.id, t.slug, t.name, t.kind, t.tmdb_id, t.imdb_id, t.rt_url, t.created_at, t.updated_at,
    CAST(COALESCE((SELECT MAX(r.ranked_on) FROM rankings r WHERE r.title_id = t.id), '') AS TEXT) AS last_ranked_on,
    (SELECT COUNT(*) FROM rankings r WHERE r.title_id = t.id) AS rankings_count
-- sqlc does not rewrite params inside ORDER BY, so the sort key comes in
-- through this one-row subquery.
FROM titles t, (SELECT CAST(sqlc.arg(sort) AS TEXT) AS sort_key) o
WHERE (CAST(sqlc.arg(kind) AS TEXT) = '' OR t.kind = sqlc.arg(kind))
  AND (CAST(sqlc.arg(name) AS TEXT) = '' OR t.name LIKE '%' || sqlc.arg(name) || '%' OR t.slug LIKE '%' || sqlc.arg(name) || '%')
  AND (CAST(sqlc.arg(missing) AS TEXT) = ''
    OR (sqlc.arg(missing) = 'tmdb' AND COALESCE(t.tmdb_id, '') = '')
    OR (sqlc.arg(missing) = 'imdb' AND COALESCE(t.imdb_id, '') = '')
    OR (sqlc.arg(missing) = 'rt' AND COALESCE(t.rt_url, '') = ''))
ORDER BY
    CASE WHEN o.sort_key = 'name_asc' THEN t.name END COLLATE NOCASE ASC,
    CASE WHEN o.sort_key = 'name_desc' THEN t.name END COLLATE NOCASE DESC,
    CASE WHEN o.sort_key = 'updated_desc' THEN t.updated_at END DESC,
    CASE WHEN o.sort_key = 'updated_asc' THEN t.updated_at END ASC,
    CASE WHEN o.sort_key = 'last_ranked_desc' THEN last_ranked_on END DESC,
    CASE WHEN o.sort_key = 'last_ranked_asc' THEN last_ranked_on END ASC,
    CASE WHEN o.sort_key = 'rankings_desc' THEN rankings_count END DESC,
    CASE WHEN o.sort_key = 'rankings_asc' THEN rankings_count END ASC,
    CASE WHEN o.sort_key = 'id_asc' THEN t.id END ASC,
    t.id DESC
LIMIT sqlc.arg(limit) OFFSET sqlc.arg(offset);

-- name: CountTitles :one
SELECT COUNT(*)
FROM titles
WHERE (CAST(sqlc.arg(kind) AS TEXT) = '' OR kind = sqlc.arg(kind))
  AND (CAST(sqlc.arg(name) AS TEXT) = '' OR name LIKE '%' || sqlc.arg(name) || '%' OR slug LIKE '%' || sqlc.arg(name) || '%')
  AND (CAST(sqlc.arg(missing) AS TEXT) = ''
    OR (sqlc.arg(missing) = 'tmdb' AND COALESCE(tmdb_id, '') = '')
    OR (sqlc.arg(missing) = 'imdb' AND COALESCE(imdb_id, '') = '')
    OR (sqlc.arg(missing) = 'rt' AND COALESCE(rt_url, '') = ''));

-- name: TitleMappingStats :one
SELECT
    COUNT(*) AS total,
    CAST(COALESCE(SUM(CASE WHEN COALESCE(tmdb_id, '') = '' THEN 1 ELSE 0 END), 0) AS INTEGER) AS missing_tmdb,
    CAST(COALESCE(SUM(CASE WHEN COALESCE(imdb_id, '') = '' THEN 1 ELSE 0 END), 0) AS INTEGER) AS missing_imdb,
    CAST(COALESCE(SUM(CASE WHEN COALESCE(rt_url, '') = '' THEN 1 ELSE 0 END), 0) AS INTEGER) AS missing_rt
FROM titles;

-- name: RankingStats :one
SELECT COUNT(*) AS rankings, CAST(COALESCE(MAX(ranked_on), '') AS TEXT) AS latest_ranked_on
FROM rankings;

-- name: UpsertRanking :one
INSERT INTO rankings (title_id, ranked_on, country, streaming_provider, category, rank, season_number)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(ranked_on, country, streaming_provider, category, rank) DO UPDATE SET
    title_id      = excluded.title_id,
    season_number = excluded.season_number,
    scraped_at    = CURRENT_TIMESTAMP
RETURNING id, title_id, ranked_on, country, streaming_provider, category, rank, season_number, scraped_at;

