-- Netflix "What We Watched" engagement reports.

-- name: DeleteNetflixEngagementPeriod :exec
DELETE FROM netflix_engagement WHERE period_start = CAST(sqlc.arg(period_start) AS TEXT);

-- name: InsertNetflixEngagement :exec
INSERT INTO netflix_engagement (period_start, period_end, kind, title, available_globally, release_date,
    hours_viewed, runtime_minutes, views, netflix_title_id, season_number)
VALUES (CAST(sqlc.arg(period_start) AS TEXT), CAST(sqlc.arg(period_end) AS TEXT), sqlc.arg(kind), sqlc.arg(title),
    sqlc.narg(available_globally), sqlc.narg(release_date), sqlc.narg(hours_viewed), sqlc.narg(runtime_minutes),
    sqlc.narg(views), sqlc.narg(netflix_title_id), sqlc.narg(season_number))
ON CONFLICT(period_start, kind, title) DO UPDATE SET
    period_end = excluded.period_end, available_globally = excluded.available_globally, release_date = excluded.release_date,
    hours_viewed = excluded.hours_viewed, runtime_minutes = excluded.runtime_minutes, views = excluded.views,
    netflix_title_id = excluded.netflix_title_id, season_number = excluded.season_number;

-- name: ListNetflixTitleNames :many
-- Netflix Top 10 titles and season titles by name, for linking report rows.
SELECT n.id, n.kind, n.show_title AS name, CAST(0 AS INTEGER) AS season_number, CAST(0 AS INTEGER) AS is_season
FROM netflix_titles n
UNION ALL
SELECT s.netflix_title_id, n.kind, s.season_title, CAST(COALESCE(s.season_number, 0) AS INTEGER), CAST(1 AS INTEGER)
FROM netflix_seasons s JOIN netflix_titles n ON n.id = s.netflix_title_id;

-- name: ListNetflixEngagementForTmdb :many
-- Report rows of the Netflix titles mapped to a TMDB title, newest period first.
SELECT CAST(e.period_start AS TEXT) AS period_start, CAST(e.period_end AS TEXT) AS period_end, e.kind, e.title,
       e.available_globally, CAST(COALESCE(e.release_date, '') AS TEXT) AS release_date,
       e.hours_viewed, e.runtime_minutes, e.views, e.season_number
FROM netflix_engagement e
JOIN netflix_titles n ON n.id = e.netflix_title_id
WHERE n.tmdb_kind = sqlc.arg(tmdb_kind) AND n.tmdb_id = sqlc.arg(tmdb_id)
ORDER BY e.period_start DESC, e.season_number, e.title;

-- name: CountNetflixEngagement :many
SELECT CAST(period_start AS TEXT) AS period_start, CAST(period_end AS TEXT) AS period_end,
       COUNT(*) AS titles, CAST(SUM(CASE WHEN netflix_title_id IS NOT NULL THEN 1 ELSE 0 END) AS INTEGER) AS linked
FROM netflix_engagement GROUP BY period_start, period_end ORDER BY period_start DESC;

-- name: ClearNetflixPremieres :exec
UPDATE tmdb_titles SET netflix_premiere = NULL, netflix_premiere_source = NULL
WHERE netflix_premiere IS NOT NULL;

-- name: SetLinkedNetflixPremieres :execrows
-- From report rows linked to Netflix Top 10 titles mapped to the TMDB title.
UPDATE tmdb_titles SET netflix_premiere_source = 'linked', netflix_premiere = (
    SELECT MIN(e.release_date) FROM netflix_engagement e
    JOIN netflix_titles n ON n.id = e.netflix_title_id
    WHERE n.tmdb_kind = tmdb_titles.tmdb_kind AND n.tmdb_id = tmdb_titles.tmdb_id AND COALESCE(e.release_date, '') <> '')
WHERE EXISTS (
    SELECT 1 FROM netflix_engagement e
    JOIN netflix_titles n ON n.id = e.netflix_title_id
    WHERE n.tmdb_kind = tmdb_titles.tmdb_kind AND n.tmdb_id = tmdb_titles.tmdb_id AND COALESCE(e.release_date, '') <> '');

-- name: ListUnlinkedNetflixOriginals :many
-- Report rows of Netflix originals not linked to a Netflix Top 10 title.
SELECT kind, title, CAST(release_date AS TEXT) AS release_date FROM netflix_engagement
WHERE netflix_title_id IS NULL AND COALESCE(release_date, '') <> '';

-- name: ListTmdbTitleNames :many
-- Stored TMDB titles without a Netflix premiere yet, with names and year.
SELECT tmdb_kind, tmdb_id, CAST(COALESCE(title, '') AS TEXT) AS title,
       CAST(COALESCE(original_title, '') AS TEXT) AS original_title,
       CAST(COALESCE(release_date, '') AS TEXT) AS release_date
FROM tmdb_titles WHERE netflix_premiere IS NULL AND COALESCE(title, '') <> '';

-- name: SetNetflixPremiere :exec
UPDATE tmdb_titles SET netflix_premiere = CAST(sqlc.arg(premiere) AS TEXT), netflix_premiere_source = sqlc.arg(source)
WHERE tmdb_kind = sqlc.arg(tmdb_kind) AND tmdb_id = sqlc.arg(tmdb_id)
  AND (netflix_premiere IS NULL OR netflix_premiere > CAST(sqlc.arg(premiere) AS TEXT));
