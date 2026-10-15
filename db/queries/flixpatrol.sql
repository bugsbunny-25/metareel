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
    (SELECT COUNT(*) FROM rankings r WHERE r.title_id = t.id) AS rankings_count,
    m.imdb_id AS ref_imdb_id, m.rt_id AS ref_rt_id, m.wikidata_id AS ref_wikidata_id, m.justwatch_id AS ref_justwatch_id
-- sqlc does not rewrite params inside ORDER BY, so the sort key comes in
-- through this one-row subquery.
FROM titles t
LEFT JOIN tmdb_titles m ON m.tmdb_kind = CASE t.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END AND m.tmdb_id = t.tmdb_id,
     (SELECT CAST(sqlc.arg(sort) AS TEXT) AS sort_key) o
WHERE (CAST(sqlc.arg(kind) AS TEXT) = '' OR t.kind = sqlc.arg(kind))
  AND (CAST(sqlc.arg(name) AS TEXT) = '' OR t.name LIKE '%' || sqlc.arg(name) || '%' OR t.slug LIKE '%' || sqlc.arg(name) || '%')
  AND (CAST(sqlc.arg(missing) AS TEXT) = ''
    OR (sqlc.arg(missing) = 'tmdb' AND COALESCE(t.tmdb_id, '') = '')
    OR (sqlc.arg(missing) = 'imdb' AND COALESCE(m.imdb_id, '') = '')
    OR (sqlc.arg(missing) = 'rt' AND COALESCE(m.rt_id, '') = ''))
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
FROM titles t
LEFT JOIN tmdb_titles m ON m.tmdb_kind = CASE t.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END AND m.tmdb_id = t.tmdb_id
WHERE (CAST(sqlc.arg(kind) AS TEXT) = '' OR t.kind = sqlc.arg(kind))
  AND (CAST(sqlc.arg(name) AS TEXT) = '' OR t.name LIKE '%' || sqlc.arg(name) || '%' OR t.slug LIKE '%' || sqlc.arg(name) || '%')
  AND (CAST(sqlc.arg(missing) AS TEXT) = ''
    OR (sqlc.arg(missing) = 'tmdb' AND COALESCE(t.tmdb_id, '') = '')
    OR (sqlc.arg(missing) = 'imdb' AND COALESCE(m.imdb_id, '') = '')
    OR (sqlc.arg(missing) = 'rt' AND COALESCE(m.rt_id, '') = ''))
  AND (CAST(sqlc.arg(match_status) AS TEXT) = '' OR t.match_status = sqlc.arg(match_status));

-- name: TitleMappingStats :one
SELECT
    COUNT(*) AS total,
    CAST(COALESCE(SUM(CASE WHEN COALESCE(t.tmdb_id, '') = '' THEN 1 ELSE 0 END), 0) AS INTEGER) AS missing_tmdb,
    CAST(COALESCE(SUM(CASE WHEN COALESCE(m.imdb_id, '') = '' THEN 1 ELSE 0 END), 0) AS INTEGER) AS missing_imdb,
    CAST(COALESCE(SUM(CASE WHEN COALESCE(m.rt_id, '') = '' THEN 1 ELSE 0 END), 0) AS INTEGER) AS missing_rt
FROM titles t
LEFT JOIN tmdb_titles m ON m.tmdb_kind = CASE t.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END AND m.tmdb_id = t.tmdb_id;

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
INSERT INTO rankings (title_id, ranked_on, country, streaming_provider, category, rank, season_number, season_source)
VALUES (sqlc.arg(title_id), CAST(sqlc.arg(ranked_on) AS TEXT), sqlc.arg(country), sqlc.arg(streaming_provider),
        sqlc.arg(category), sqlc.arg(rank), sqlc.narg(season_number), sqlc.narg(season_source));

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

-- name: RecordTitleMatchAttempt :one
-- One automatic matching attempt (what it found is mapping evidence): a
-- title without a result is unmatched until retry_after (a SQLite datetime
-- modifier). Never touches a title fixed by hand.
UPDATE titles
SET match_status       = CASE WHEN CAST(sqlc.arg(found) AS INTEGER) = 1 THEN match_status ELSE 'unmatched' END,
    matched_name       = CASE WHEN CAST(sqlc.arg(found) AS INTEGER) = 1 THEN sqlc.narg(matched_name) ELSE matched_name END,
    matched_year       = CASE WHEN CAST(sqlc.arg(found) AS INTEGER) = 1 THEN sqlc.narg(matched_year) ELSE matched_year END,
    match_attempts     = match_attempts + 1,
    match_attempted_at = CURRENT_TIMESTAMP,
    next_match_at      = CASE WHEN CAST(sqlc.arg(retry_after) AS TEXT) = '' THEN NULL
                              ELSE datetime('now', CAST(sqlc.arg(retry_after) AS TEXT)) END,
    updated_at         = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND match_status <> 'manual'
RETURNING *;

-- name: RequeueTitleMatch :exec
-- Due for matching now; clear also forgets the attempts and matched name.
UPDATE titles
SET next_match_at  = NULL,
    match_attempts = CASE WHEN CAST(sqlc.arg(clear) AS INTEGER) = 1 THEN 0 ELSE match_attempts END,
    matched_name   = CASE WHEN CAST(sqlc.arg(clear) AS INTEGER) = 1 THEN NULL ELSE matched_name END,
    matched_year   = CASE WHEN CAST(sqlc.arg(clear) AS INTEGER) = 1 THEN NULL ELSE matched_year END,
    updated_at     = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: ApplyTitleMapping :one
-- Stores a title's computed mapping. kind '' keeps the kind (an unmapped
-- title); status '' keeps the match status; status pending (a title that lost
-- its mapping) makes it due for matching now.
UPDATE titles
SET kind          = CASE WHEN CAST(sqlc.arg(kind) AS TEXT) = '' THEN kind ELSE sqlc.arg(kind) END,
    tmdb_id       = sqlc.narg(tmdb_id),
    match_status  = CASE WHEN CAST(sqlc.arg(match_status) AS TEXT) = '' THEN match_status ELSE sqlc.arg(match_status) END,
    match_source  = sqlc.narg(match_source),
    next_match_at = CASE WHEN CAST(sqlc.arg(match_status) AS TEXT) = 'matched' OR CAST(sqlc.arg(match_status) AS TEXT) = 'manual'
                         OR CAST(sqlc.arg(match_status) AS TEXT) = 'pending' THEN NULL ELSE next_match_at END,
    matched_name  = CASE WHEN CAST(sqlc.arg(own_match) AS INTEGER) = 1 THEN matched_name END,
    matched_year  = CASE WHEN CAST(sqlc.arg(own_match) AS INTEGER) = 1 THEN matched_year END,
    updated_at    = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ListTitleIDsOnRef :many
SELECT id FROM titles WHERE kind = sqlc.arg(kind) AND tmdb_id = sqlc.arg(tmdb_id) ORDER BY id;
