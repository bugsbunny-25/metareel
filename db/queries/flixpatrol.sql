-- Titles (movies + tv shows) + daily rankings (Top 10).

-- name: EnsureTitle :one
-- Scrapes only record that a slug exists; matching (kind, IDs) is the
-- enrichment job's, so an existing title keeps its kind and IDs.
INSERT INTO titles (slug, name, kind)
VALUES (?, ?, ?)
ON CONFLICT(slug) DO UPDATE SET
    name       = excluded.name,
    updated_at = CASE WHEN titles.name = excluded.name THEN titles.updated_at ELSE CURRENT_TIMESTAMP END
RETURNING *;

-- name: GetTitleBySlug :one
SELECT * FROM titles WHERE slug = ?;

-- name: GetTitleByID :one
SELECT * FROM titles WHERE id = ?;

-- name: ListTitlesByIDs :many
SELECT * FROM titles WHERE id IN (sqlc.slice(ids)) ORDER BY id;

-- name: UpdateTitleIDs :one
-- Admin correction: marks the title as a manual match so automation leaves it alone.
UPDATE titles
SET
    kind               = ?,
    tmdb_id            = ?,
    imdb_id            = ?,
    rt_url             = ?,
    match_status       = 'manual',
    match_source       = 'manual',
    next_match_at      = NULL,
    updated_at         = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: ResetTitleMatch :one
-- Puts a title back in the matching queue (due now), clearing a manual lock.
UPDATE titles
SET match_status  = CASE WHEN COALESCE(tmdb_id, '') = '' THEN 'pending' ELSE 'matched' END,
    match_source  = CASE WHEN match_status = 'manual' THEN NULL ELSE match_source END,
    next_match_at = NULL,
    next_rt_at    = NULL,
    updated_at    = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: ClearTitleMatch :one
-- Forgets a title's IDs and queues it for a fresh match.
UPDATE titles
SET tmdb_id = NULL, imdb_id = NULL, rt_url = NULL, justwatch_id = NULL, wikidata_id = NULL,
    match_status = 'pending', match_source = NULL, matched_name = NULL, matched_year = NULL,
    match_attempts = 0, next_match_at = NULL, rt_attempts = 0, next_rt_at = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: ListTitlesDueForMatch :many
-- Unmatched titles whose backoff has expired, never-tried first, then the
-- most recently charting.
SELECT t.*
FROM titles t
WHERE t.match_status IN ('pending', 'unmatched')
  AND (t.next_match_at IS NULL OR t.next_match_at <= CURRENT_TIMESTAMP)
ORDER BY CASE WHEN t.match_status = 'pending' THEN 0 ELSE 1 END,
         (SELECT MAX(r.ranked_on) FROM rankings r WHERE r.title_id = t.id) DESC,
         t.id DESC
LIMIT sqlc.arg(limit);

-- name: ListTitlesDueForRT :many
-- Matched titles still without a Rotten Tomatoes slug whose backoff has expired.
SELECT t.*
FROM titles t
WHERE t.match_status = 'matched'
  AND COALESCE(t.tmdb_id, '') <> ''
  AND COALESCE(t.rt_url, '') = ''
  AND (t.next_rt_at IS NULL OR t.next_rt_at <= CURRENT_TIMESTAMP)
ORDER BY (SELECT MAX(r.ranked_on) FROM rankings r WHERE r.title_id = t.id) DESC, t.id DESC
LIMIT sqlc.arg(limit);

-- name: SaveTitleMatch :one
-- Records one matching attempt. retry_after is a SQLite datetime modifier
-- ('+3 days'); an empty tmdb_id leaves the title unmatched. Never touches
-- manual titles.
UPDATE titles
SET kind               = sqlc.arg(kind),
    tmdb_id            = COALESCE(sqlc.narg(tmdb_id), tmdb_id),
    imdb_id            = COALESCE(sqlc.narg(imdb_id), imdb_id),
    justwatch_id       = COALESCE(sqlc.narg(justwatch_id), justwatch_id),
    match_status       = sqlc.arg(match_status),
    match_source       = sqlc.narg(match_source),
    matched_name       = sqlc.narg(matched_name),
    matched_year       = sqlc.narg(matched_year),
    match_attempts     = match_attempts + 1,
    match_attempted_at = CURRENT_TIMESTAMP,
    next_match_at      = CASE WHEN CAST(sqlc.arg(retry_after) AS TEXT) = '' THEN NULL
                              ELSE datetime('now', CAST(sqlc.arg(retry_after) AS TEXT)) END,
    updated_at         = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND match_status <> 'manual'
RETURNING *;

-- name: SaveTitleExternalIDs :exec
-- Fills IDs found later (TMDB external IDs, Wikidata); never overwrites.
UPDATE titles
SET imdb_id     = COALESCE(imdb_id, sqlc.narg(imdb_id)),
    wikidata_id = COALESCE(wikidata_id, sqlc.narg(wikidata_id)),
    rt_url      = COALESCE(rt_url, sqlc.narg(rt_url)),
    updated_at  = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND match_status <> 'manual';

-- name: SaveTitleRTAttempt :exec
-- Records one Rotten Tomatoes lookup; rt_url empty = not found, retry after
-- retry_after (a SQLite datetime modifier).
UPDATE titles
SET rt_url      = COALESCE(rt_url, NULLIF(CAST(sqlc.arg(rt_url) AS TEXT), '')),
    rt_attempts = rt_attempts + 1,
    next_rt_at  = datetime('now', CAST(sqlc.arg(retry_after) AS TEXT)),
    updated_at  = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND match_status <> 'manual';

-- name: SaveFlixPatrolDetails :exec
UPDATE titles
SET fp_name          = sqlc.narg(fp_name),
    fp_kind          = sqlc.narg(fp_kind),
    fp_premiere_date = sqlc.narg(fp_premiere_date),
    fp_country       = sqlc.narg(fp_country),
    fp_fetched_at    = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: CountTitlesByMatchStatus :many
SELECT match_status, COUNT(*) AS titles FROM titles GROUP BY match_status;

-- name: ListTitles :many
-- sort: name_asc | name_desc | updated_desc | updated_asc | last_ranked_desc |
-- last_ranked_asc | rankings_desc | rankings_asc | id_asc; anything else is
-- newest first.
SELECT
    sqlc.embed(t),
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
  AND (CAST(sqlc.arg(match_status) AS TEXT) = '' OR t.match_status = sqlc.arg(match_status))
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
    OR (sqlc.arg(missing) = 'rt' AND COALESCE(rt_url, '') = ''))
  AND (CAST(sqlc.arg(match_status) AS TEXT) = '' OR match_status = sqlc.arg(match_status));

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

-- name: DeleteChartRankings :exec
DELETE FROM rankings
WHERE ranked_on = CAST(sqlc.arg(ranked_on) AS TEXT)
  AND country = sqlc.arg(country)
  AND streaming_provider = sqlc.arg(streaming_provider)
  AND category = sqlc.arg(category);

-- name: InsertRanking :exec
INSERT INTO rankings (title_id, ranked_on, country, streaming_provider, category, rank, season_number)
VALUES (sqlc.arg(title_id), CAST(sqlc.arg(ranked_on) AS TEXT), sqlc.arg(country), sqlc.arg(streaming_provider),
        sqlc.arg(category), sqlc.arg(rank), sqlc.narg(season_number));

-- name: UpsertChartSnapshot :exec
INSERT INTO chart_snapshots (ranked_on, country, streaming_provider, category, entry_count, signature, source, run_id)
VALUES (sqlc.arg(ranked_on), sqlc.arg(country), sqlc.arg(streaming_provider), sqlc.arg(category),
        sqlc.arg(entry_count), sqlc.arg(signature), sqlc.arg(source), sqlc.narg(run_id))
ON CONFLICT(ranked_on, country, streaming_provider, category) DO UPDATE SET
    entry_count = excluded.entry_count,
    changed_at  = CASE WHEN chart_snapshots.signature = excluded.signature THEN chart_snapshots.changed_at ELSE CURRENT_TIMESTAMP END,
    signature   = excluded.signature,
    source      = excluded.source,
    run_id      = excluded.run_id,
    scraped_at  = CURRENT_TIMESTAMP;

-- name: ListChartSnapshotsForDate :many
SELECT * FROM chart_snapshots
WHERE ranked_on = sqlc.arg(ranked_on)
  AND country = sqlc.arg(country)
  AND streaming_provider = sqlc.arg(streaming_provider);

-- name: GetPreviousChartSnapshot :one
SELECT * FROM chart_snapshots
WHERE country = sqlc.arg(country)
  AND streaming_provider = sqlc.arg(streaming_provider)
  AND category = sqlc.arg(category)
  AND ranked_on < sqlc.arg(ranked_on)
ORDER BY ranked_on DESC
LIMIT 1;

-- name: ListChartSnapshotDates :many
-- Dates with both categories stored for a chart in [from, to].
SELECT ranked_on
FROM chart_snapshots
WHERE country = sqlc.arg(country)
  AND streaming_provider = sqlc.arg(streaming_provider)
  AND ranked_on >= CAST(sqlc.arg(from_date) AS TEXT)
  AND ranked_on <= CAST(sqlc.arg(to_date) AS TEXT)
GROUP BY ranked_on
HAVING COUNT(DISTINCT category) >= 2
ORDER BY ranked_on;

-- name: GetLatestChartForTitle :one
-- The most recent chart a title appeared on (matching searches that
-- country and service first).
SELECT CAST(country AS TEXT) AS country, streaming_provider, category
FROM rankings
WHERE title_id = ?
ORDER BY ranked_on DESC
LIMIT 1;

-- name: ListTmdbRefsChartingSince :many
-- Matched titles on any chart since a date (YYYY-MM-DD), most recent first.
SELECT CASE t.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END AS tmdb_kind,
       CAST(t.tmdb_id AS TEXT) AS tmdb_id,
       CAST(MAX(r.ranked_on) AS TEXT) AS last_ranked_on
FROM rankings r
JOIN titles t ON t.id = r.title_id
WHERE r.ranked_on >= CAST(sqlc.arg(since) AS TEXT)
  AND COALESCE(t.tmdb_id, '') <> ''
GROUP BY t.kind, t.tmdb_id
ORDER BY MAX(r.ranked_on) DESC, MIN(r.rank) ASC
LIMIT sqlc.arg(limit);
