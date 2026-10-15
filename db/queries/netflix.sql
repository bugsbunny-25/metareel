-- Netflix official Top 10: title mapping (admin) and season numbers.

-- name: ListNetflixTitles :many
-- sort: last_week_desc (default) | last_week_asc | name_asc | name_desc |
-- weeks_desc | weeks_asc | best_rank_asc | id_desc. missing: tmdb | season
-- (a season without a number). flag: cross_kind (TMDB kind differs from
-- Netflix's list) | linked (has linked FlixPatrol titles).
SELECT
    sqlc.embed(n),
    CAST(COALESCE(NULLIF(max(
        COALESCE((SELECT MAX(g.week) FROM netflix_top10_global g WHERE g.netflix_title_id = n.id), ''),
        COALESCE((SELECT MAX(c.week) FROM netflix_top10_countries c WHERE c.netflix_title_id = n.id), '')), ''), '') AS TEXT) AS last_week,
    (SELECT COUNT(DISTINCT g.week) FROM netflix_top10_global g WHERE g.netflix_title_id = n.id) AS weeks_global,
    CAST(COALESCE((SELECT MIN(g.weekly_rank) FROM netflix_top10_global g WHERE g.netflix_title_id = n.id), 0) AS INTEGER) AS best_global_rank,
    (SELECT COUNT(*) FROM netflix_seasons s WHERE s.netflix_title_id = n.id) AS seasons_total,
    (SELECT COUNT(*) FROM netflix_seasons s WHERE s.netflix_title_id = n.id AND s.season_number IS NULL
        AND COALESCE(n.tmdb_kind, '') <> 'movie') AS seasons_unmapped,
    (SELECT COUNT(*) FROM title_links l WHERE l.netflix_title_id = n.id) AS flixpatrol_links,
    tm.title AS tmdb_title,
    tm.release_date AS tmdb_release_date,
    CAST(COALESCE(t.slug, '') AS TEXT) AS flixpatrol_slug,
    CAST(COALESCE(t.name, '') AS TEXT) AS flixpatrol_name
FROM netflix_titles n
LEFT JOIN tmdb_titles tm ON tm.tmdb_kind = n.tmdb_kind AND tm.tmdb_id = n.tmdb_id
LEFT JOIN titles t ON t.id = n.title_id,
     (SELECT CAST(sqlc.arg(sort) AS TEXT) AS sort_key) o
WHERE (CAST(sqlc.arg(kind) AS TEXT) = '' OR n.kind = sqlc.arg(kind))
  AND (CAST(sqlc.arg(name) AS TEXT) = '' OR n.show_title LIKE '%' || sqlc.arg(name) || '%' OR COALESCE(n.tmdb_id, '') = sqlc.arg(name))
  AND (CAST(sqlc.arg(match_status) AS TEXT) = '' OR n.match_status = sqlc.arg(match_status))
  AND (CAST(sqlc.arg(missing) AS TEXT) = ''
    OR (sqlc.arg(missing) = 'tmdb' AND COALESCE(n.tmdb_id, '') = '')
    OR (sqlc.arg(missing) = 'season' AND COALESCE(n.tmdb_kind, '') <> 'movie' AND EXISTS (
        SELECT 1 FROM netflix_seasons s WHERE s.netflix_title_id = n.id AND s.season_number IS NULL)))
  AND (CAST(sqlc.arg(flag) AS TEXT) = ''
    OR (sqlc.arg(flag) = 'cross_kind' AND n.tmdb_kind IS NOT NULL
        AND n.tmdb_kind <> CASE n.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END)
    OR (sqlc.arg(flag) = 'linked' AND EXISTS (SELECT 1 FROM title_links l WHERE l.netflix_title_id = n.id)))
ORDER BY
    CASE WHEN o.sort_key = 'name_asc' THEN n.show_title END COLLATE NOCASE ASC,
    CASE WHEN o.sort_key = 'name_desc' THEN n.show_title END COLLATE NOCASE DESC,
    CASE WHEN o.sort_key = 'weeks_desc' THEN weeks_global END DESC,
    CASE WHEN o.sort_key = 'weeks_asc' THEN weeks_global END ASC,
    CASE WHEN o.sort_key = 'best_rank_asc' THEN CASE WHEN best_global_rank = 0 THEN 99 ELSE best_global_rank END END ASC,
    CASE WHEN o.sort_key = 'last_week_asc' THEN last_week END ASC,
    CASE WHEN o.sort_key = 'id_desc' THEN n.id END DESC,
    last_week DESC,
    n.id DESC
LIMIT sqlc.arg(limit) OFFSET sqlc.arg(offset);

-- name: CountNetflixTitles :one
SELECT COUNT(*)
FROM netflix_titles n
WHERE (CAST(sqlc.arg(kind) AS TEXT) = '' OR n.kind = sqlc.arg(kind))
  AND (CAST(sqlc.arg(name) AS TEXT) = '' OR n.show_title LIKE '%' || sqlc.arg(name) || '%' OR COALESCE(n.tmdb_id, '') = sqlc.arg(name))
  AND (CAST(sqlc.arg(match_status) AS TEXT) = '' OR n.match_status = sqlc.arg(match_status))
  AND (CAST(sqlc.arg(missing) AS TEXT) = ''
    OR (sqlc.arg(missing) = 'tmdb' AND COALESCE(n.tmdb_id, '') = '')
    OR (sqlc.arg(missing) = 'season' AND COALESCE(n.tmdb_kind, '') <> 'movie' AND EXISTS (
        SELECT 1 FROM netflix_seasons s WHERE s.netflix_title_id = n.id AND s.season_number IS NULL)))
  AND (CAST(sqlc.arg(flag) AS TEXT) = ''
    OR (sqlc.arg(flag) = 'cross_kind' AND n.tmdb_kind IS NOT NULL
        AND n.tmdb_kind <> CASE n.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END)
    OR (sqlc.arg(flag) = 'linked' AND EXISTS (SELECT 1 FROM title_links l WHERE l.netflix_title_id = n.id)));

-- name: GetNetflixTitle :one
SELECT
    sqlc.embed(n),
    CAST(COALESCE(NULLIF(max(
        COALESCE((SELECT MAX(g.week) FROM netflix_top10_global g WHERE g.netflix_title_id = n.id), ''),
        COALESCE((SELECT MAX(c.week) FROM netflix_top10_countries c WHERE c.netflix_title_id = n.id), '')), ''), '') AS TEXT) AS last_week,
    (SELECT COUNT(DISTINCT g.week) FROM netflix_top10_global g WHERE g.netflix_title_id = n.id) AS weeks_global,
    CAST(COALESCE((SELECT MIN(g.weekly_rank) FROM netflix_top10_global g WHERE g.netflix_title_id = n.id), 0) AS INTEGER) AS best_global_rank,
    (SELECT COUNT(*) FROM netflix_seasons s WHERE s.netflix_title_id = n.id) AS seasons_total,
    (SELECT COUNT(*) FROM netflix_seasons s WHERE s.netflix_title_id = n.id AND s.season_number IS NULL
        AND COALESCE(n.tmdb_kind, '') <> 'movie') AS seasons_unmapped,
    (SELECT COUNT(*) FROM title_links l WHERE l.netflix_title_id = n.id) AS flixpatrol_links,
    tm.title AS tmdb_title,
    tm.release_date AS tmdb_release_date,
    CAST(COALESCE(t.slug, '') AS TEXT) AS flixpatrol_slug,
    CAST(COALESCE(t.name, '') AS TEXT) AS flixpatrol_name
FROM netflix_titles n
LEFT JOIN tmdb_titles tm ON tm.tmdb_kind = n.tmdb_kind AND tm.tmdb_id = n.tmdb_id
LEFT JOIN titles t ON t.id = n.title_id
WHERE n.id = sqlc.arg(id);

-- name: GetNetflixTitleRow :one
SELECT * FROM netflix_titles WHERE id = ?;

-- name: UpdateNetflixTitleIDs :one
-- Admin correction: marks the title as a manual match so automation leaves it alone.
UPDATE netflix_titles
SET tmdb_kind     = sqlc.narg(tmdb_kind),
    tmdb_id       = sqlc.narg(tmdb_id),
    imdb_id       = sqlc.narg(imdb_id),
    match_status  = 'manual',
    match_source  = 'manual',
    next_match_at = NULL,
    updated_at    = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ResetNetflixTitleMatch :one
-- Puts a title back in the matching queue (due now), clearing a manual lock.
UPDATE netflix_titles
SET match_status  = CASE WHEN COALESCE(tmdb_id, '') = '' THEN 'pending' ELSE 'matched' END,
    match_source  = CASE WHEN match_status = 'manual' THEN NULL ELSE match_source END,
    next_match_at = NULL,
    updated_at    = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: ClearNetflixTitleMatch :one
-- Forgets a title's IDs and queues it for a fresh match.
UPDATE netflix_titles
SET tmdb_kind = NULL, tmdb_id = NULL, imdb_id = NULL, title_id = NULL, match_status = 'pending', match_source = NULL,
    match_attempts = 0, next_match_at = NULL, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: ListNetflixTitlesForMatch :many
-- Specific titles to match now (ignoring backoff), with the first week they
-- charted. Manual titles are skipped by SaveNetflixTitleMatch.
SELECT sqlc.embed(n),
       CAST(COALESCE(
           (SELECT MIN(g.week) FROM netflix_top10_global g WHERE g.netflix_title_id = n.id),
           (SELECT MIN(c.week) FROM netflix_top10_countries c WHERE c.netflix_title_id = n.id),
           '') AS TEXT) AS first_week
FROM netflix_titles n
WHERE n.id IN (sqlc.slice(ids));

-- name: EnsureNetflixSeasons :exec
-- One netflix_seasons row per season title a TV title charted with.
INSERT INTO netflix_seasons (netflix_title_id, season_title)
SELECT u.netflix_title_id, u.season_title
FROM (
    SELECT netflix_title_id, season_title FROM netflix_top10_global WHERE season_title IS NOT NULL
    UNION SELECT netflix_title_id, season_title FROM netflix_top10_countries WHERE season_title IS NOT NULL
    UNION SELECT netflix_title_id, season_title FROM netflix_most_popular WHERE season_title IS NOT NULL
) u
JOIN netflix_titles n ON n.id = u.netflix_title_id
WHERE n.kind = 'tv_show'
ON CONFLICT(netflix_title_id, season_title) DO NOTHING;

-- name: ListNetflixSeasonsForMapping :many
-- Seasons not fixed by hand (of one title, or all when netflix_title_id is
-- 0), with what mapping needs: the show, the first week the season charted
-- (YYYY-MM-DD or '') and the TMDB season list (JSON) of its match.
SELECT s.id, s.netflix_title_id, s.season_title, s.season_number, s.season_source,
       n.show_title, n.tmdb_id, n.tmdb_kind,
       CAST(COALESCE(NULLIF(min(
           COALESCE((SELECT MIN(g.week) FROM netflix_top10_global g
                     WHERE g.netflix_title_id = s.netflix_title_id AND g.season_title = s.season_title), '9999-12-31'),
           COALESCE((SELECT MIN(c.week) FROM netflix_top10_countries c
                     WHERE c.netflix_title_id = s.netflix_title_id AND c.season_title = s.season_title), '9999-12-31')),
           '9999-12-31'), '') AS TEXT) AS first_week,
       tm.seasons AS tmdb_seasons
FROM netflix_seasons s
JOIN netflix_titles n ON n.id = s.netflix_title_id
LEFT JOIN tmdb_titles tm ON tm.tmdb_kind = n.tmdb_kind AND n.tmdb_kind = 'tv' AND tm.tmdb_id = n.tmdb_id
WHERE COALESCE(s.season_source, '') <> 'manual'
  AND (CAST(sqlc.arg(netflix_title_id) AS INTEGER) = 0 OR s.netflix_title_id = sqlc.arg(netflix_title_id));

-- name: UpdateNetflixSeasonMapping :exec
UPDATE netflix_seasons
SET season_number = sqlc.narg(season_number),
    season_source = sqlc.narg(season_source),
    updated_at    = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND COALESCE(season_source, '') <> 'manual';

-- name: SetNetflixSeasonManual :one
UPDATE netflix_seasons
SET season_number = sqlc.arg(season_number), season_source = 'manual', updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ResetNetflixSeason :one
-- Hands a season back to automatic mapping.
UPDATE netflix_seasons
SET season_number = NULL, season_source = NULL, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: ListNetflixSeasonsForTitle :many
-- A title's seasons with their global Top 10 stats.
SELECT s.*,
       (SELECT COUNT(DISTINCT g.week) FROM netflix_top10_global g
        WHERE g.netflix_title_id = s.netflix_title_id AND g.season_title = s.season_title) AS weeks_global,
       CAST(COALESCE((SELECT MIN(g.weekly_rank) FROM netflix_top10_global g
        WHERE g.netflix_title_id = s.netflix_title_id AND g.season_title = s.season_title), 0) AS INTEGER) AS best_global_rank,
       CAST(COALESCE((SELECT SUM(g.weekly_views) FROM netflix_top10_global g
        WHERE g.netflix_title_id = s.netflix_title_id AND g.season_title = s.season_title), 0) AS INTEGER) AS total_views,
       CAST(COALESCE((SELECT SUM(g.weekly_hours_viewed) FROM netflix_top10_global g
        WHERE g.netflix_title_id = s.netflix_title_id AND g.season_title = s.season_title), 0) AS INTEGER) AS total_hours,
       CAST(COALESCE(NULLIF(min(
           COALESCE((SELECT MIN(g.week) FROM netflix_top10_global g
                     WHERE g.netflix_title_id = s.netflix_title_id AND g.season_title = s.season_title), '9999-12-31'),
           COALESCE((SELECT MIN(c.week) FROM netflix_top10_countries c
                     WHERE c.netflix_title_id = s.netflix_title_id AND c.season_title = s.season_title), '9999-12-31')),
           '9999-12-31'), '') AS TEXT) AS first_week,
       CAST(COALESCE(NULLIF(max(
           COALESCE((SELECT MAX(g.week) FROM netflix_top10_global g
                     WHERE g.netflix_title_id = s.netflix_title_id AND g.season_title = s.season_title), ''),
           COALESCE((SELECT MAX(c.week) FROM netflix_top10_countries c
                     WHERE c.netflix_title_id = s.netflix_title_id AND c.season_title = s.season_title), '')), ''), '') AS TEXT) AS last_week,
       (SELECT COUNT(DISTINCT c.country) FROM netflix_top10_countries c
        WHERE c.netflix_title_id = s.netflix_title_id AND c.season_title = s.season_title) AS countries
FROM netflix_seasons s
WHERE s.netflix_title_id = sqlc.arg(netflix_title_id)
ORDER BY s.season_number IS NULL, s.season_number, first_week, s.season_title;

-- name: GetNetflixSeason :one
SELECT * FROM netflix_seasons WHERE id = ?;

-- name: CountNetflixSeasonsBySource :many
SELECT CAST(COALESCE(season_source, 'unmapped') AS TEXT) AS source, COUNT(*) AS seasons
FROM netflix_seasons GROUP BY 1;

-- name: ListNetflixGlobalWeeksForTitle :many
-- A Netflix title's global weeks (admin drawer), newest first.
SELECT g.week, g.category, g.weekly_rank, g.season_title, g.weekly_views, g.weekly_hours_viewed, g.cumulative_weeks,
       s.season_number
FROM netflix_top10_global g
LEFT JOIN netflix_seasons s ON s.netflix_title_id = g.netflix_title_id AND s.season_title = g.season_title
WHERE g.netflix_title_id = sqlc.arg(netflix_title_id)
ORDER BY g.week DESC, g.weekly_rank;

-- name: ListNetflixCountryWeeksForTitle :many
SELECT c.country, COUNT(*) AS weeks, CAST(MIN(c.weekly_rank) AS INTEGER) AS best_rank,
       CAST(MIN(c.week) AS TEXT) AS first_week, CAST(MAX(c.week) AS TEXT) AS last_week
FROM netflix_top10_countries c
WHERE c.netflix_title_id = sqlc.arg(netflix_title_id)
GROUP BY c.country
ORDER BY weeks DESC, c.country;

-- Season numbers of FlixPatrol TV rankings.

-- name: ListTVRankingsForSeasons :many
-- TV rankings whose season was not given by the chart itself, from a date.
SELECT r.id, CAST(r.ranked_on AS TEXT) AS ranked_on, CAST(r.country AS TEXT) AS country, r.streaming_provider,
       r.season_number, r.season_source, t.tmdb_id
FROM rankings r
JOIN titles t ON t.id = r.title_id
WHERE r.category = 'tv_shows'
  AND COALESCE(r.season_source, '') <> 'chart'
  AND r.ranked_on >= CAST(sqlc.arg(from_date) AS TEXT);

-- name: UpdateRankingSeason :exec
UPDATE rankings SET season_number = sqlc.narg(season_number), season_source = sqlc.narg(season_source)
WHERE id = sqlc.arg(id);

-- name: ListNetflixGlobalSeasonRanks :many
-- Mapped seasons of TV shows in the global weekly Top 10, from a week.
SELECT n.tmdb_id, g.week, g.weekly_rank, s.season_number
FROM netflix_top10_global g
JOIN netflix_titles n ON n.id = g.netflix_title_id
JOIN netflix_seasons s ON s.netflix_title_id = g.netflix_title_id AND s.season_title = g.season_title
WHERE n.tmdb_kind = 'tv' AND COALESCE(n.tmdb_id, '') <> '' AND s.season_number IS NOT NULL
  AND g.week >= CAST(sqlc.arg(from_week) AS TEXT);

-- name: ListNetflixCountrySeasonRanks :many
SELECT n.tmdb_id, c.country, c.week, c.weekly_rank, s.season_number
FROM netflix_top10_countries c
JOIN netflix_titles n ON n.id = c.netflix_title_id
JOIN netflix_seasons s ON s.netflix_title_id = c.netflix_title_id AND s.season_title = c.season_title
WHERE n.tmdb_kind = 'tv' AND COALESCE(n.tmdb_id, '') <> '' AND s.season_number IS NOT NULL
  AND c.week >= CAST(sqlc.arg(from_week) AS TEXT);

-- name: ListNetflixCountryWeeksImported :many
-- (country, week) pairs present in the per-country Top 10, from a week.
SELECT DISTINCT country, week FROM netflix_top10_countries WHERE week >= CAST(sqlc.arg(from_week) AS TEXT);

-- name: ListTmdbTVSeasons :many
SELECT tmdb_id, seasons FROM tmdb_titles WHERE tmdb_kind = 'tv' AND seasons IS NOT NULL;

-- Netflix IDs from Tudum's Top 10 search.

-- name: ListNetflixTitlesForNetflixID :many
-- Titles without a Netflix ID not looked up within the window (a SQLite
-- modifier, e.g. '-30 days'), the given ones first, then newest charting.
SELECT n.id, n.show_title, n.kind, n.match_status, n.match_source, n.tmdb_kind, n.tmdb_id,
       CAST(COALESCE(n.netflix_id, '') AS TEXT) AS netflix_id
FROM netflix_titles n
WHERE COALESCE(n.netflix_id, '') = ''
  AND (n.netflix_id_checked_at IS NULL OR n.netflix_id_checked_at < datetime('now', CAST(sqlc.arg(window) AS TEXT)))
ORDER BY (SELECT MAX(g.week) FROM netflix_top10_global g WHERE g.netflix_title_id = n.id) IS NULL,
         (SELECT MAX(g.week) FROM netflix_top10_global g WHERE g.netflix_title_id = n.id) DESC,
         n.id DESC
LIMIT sqlc.arg(limit);

-- name: ListNetflixTitlesForNetflixIDByIDs :many
-- These titles, with their Netflix ID if already known.
SELECT n.id, n.show_title, n.kind, n.match_status, n.match_source, n.tmdb_kind, n.tmdb_id,
       CAST(COALESCE(n.netflix_id, '') AS TEXT) AS netflix_id
FROM netflix_titles n
WHERE n.id IN (sqlc.slice(ids));

-- name: ListNetflixTitleWeeks :many
-- Every week a title charted (global or an imported country).
SELECT g.week FROM netflix_top10_global g WHERE g.netflix_title_id = sqlc.arg(id)
UNION
SELECT c.week FROM netflix_top10_countries c WHERE c.netflix_title_id = sqlc.arg(id);

-- name: ListNetflixSeasonTitles :many
SELECT id, season_title FROM netflix_seasons WHERE netflix_title_id = ?;

-- name: SetNetflixTitleNetflixID :exec
UPDATE netflix_titles
SET netflix_id = sqlc.narg(netflix_id), netflix_id_checked_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: SetNetflixSeasonNetflixID :exec
UPDATE netflix_seasons SET netflix_id = sqlc.arg(netflix_id), updated_at = CURRENT_TIMESTAMP WHERE id = sqlc.arg(id);

-- name: SaveNetflixTitleNetflixMatch :execrows
-- Maps a title by its Netflix ID; replaces automatic matches made by name
-- (never manual ones).
UPDATE netflix_titles
SET tmdb_kind = sqlc.arg(tmdb_kind), tmdb_id = sqlc.arg(tmdb_id),
    imdb_id = COALESCE(NULLIF(CAST(sqlc.arg(imdb_id) AS TEXT), ''), netflix_titles.imdb_id),
    match_status = 'matched', match_source = 'netflix_id', next_match_at = NULL,
    match_attempts = netflix_titles.match_attempts + 1, updated_at = CURRENT_TIMESTAMP
WHERE netflix_titles.id = sqlc.arg(id) AND netflix_titles.match_status <> 'manual';
