-- File imports: Netflix official Top 10 and IMDb ratings.

-- name: GetDataImport :one
SELECT * FROM data_imports WHERE source = ?;

-- name: ListDataImports :many
SELECT * FROM data_imports ORDER BY source;

-- name: SaveDataImport :exec
INSERT INTO data_imports (source, etag, last_modified, rows, imported_at)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(source) DO UPDATE SET
    etag = excluded.etag, last_modified = excluded.last_modified, rows = excluded.rows, imported_at = CURRENT_TIMESTAMP;

-- name: EnsureNetflixTitle :one
INSERT INTO netflix_titles (show_title, kind) VALUES (?, ?)
ON CONFLICT(show_title, kind) DO UPDATE SET show_title = excluded.show_title
RETURNING *;

-- name: UpsertNetflixGlobal :exec
INSERT INTO netflix_top10_global (week, category, weekly_rank, netflix_title_id, show_title, season_title,
    weekly_hours_viewed, runtime_hours, weekly_views, cumulative_weeks)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(week, category, weekly_rank) DO UPDATE SET
    netflix_title_id = excluded.netflix_title_id, show_title = excluded.show_title, season_title = excluded.season_title,
    weekly_hours_viewed = excluded.weekly_hours_viewed, runtime_hours = excluded.runtime_hours,
    weekly_views = excluded.weekly_views, cumulative_weeks = excluded.cumulative_weeks;

-- name: UpsertNetflixCountry :exec
INSERT INTO netflix_top10_countries (country, week, category, weekly_rank, netflix_title_id, show_title, season_title, cumulative_weeks)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(country, week, category, weekly_rank) DO UPDATE SET
    netflix_title_id = excluded.netflix_title_id, show_title = excluded.show_title,
    season_title = excluded.season_title, cumulative_weeks = excluded.cumulative_weeks;

-- name: DeleteNetflixMostPopular :exec
DELETE FROM netflix_most_popular;

-- name: InsertNetflixMostPopular :exec
INSERT INTO netflix_most_popular (category, rank, netflix_title_id, show_title, season_title, hours_viewed_91d, runtime_hours, views_91d)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListNetflixTitlesDueForMatch :many
-- first_week is the first week the title was in any imported list
-- (YYYY-MM-DD, or '' if only in the most-popular list).
SELECT sqlc.embed(n),
       CAST(COALESCE(
           (SELECT MIN(g.week) FROM netflix_top10_global g WHERE g.netflix_title_id = n.id),
           (SELECT MIN(c.week) FROM netflix_top10_countries c WHERE c.netflix_title_id = n.id),
           '') AS TEXT) AS first_week
FROM netflix_titles n
WHERE n.match_status IN ('pending', 'unmatched')
  AND (n.next_match_at IS NULL OR n.next_match_at <= CURRENT_TIMESTAMP)
ORDER BY CASE WHEN n.match_status = 'pending' THEN 0 ELSE 1 END,
         (SELECT MAX(g.week) FROM netflix_top10_global g WHERE g.netflix_title_id = n.id) DESC
LIMIT sqlc.arg(limit);

-- name: SaveNetflixTitleMatch :exec
UPDATE netflix_titles
SET tmdb_id        = COALESCE(sqlc.narg(tmdb_id), tmdb_id),
    imdb_id        = COALESCE(sqlc.narg(imdb_id), imdb_id),
    title_id       = COALESCE(sqlc.narg(title_id), title_id),
    match_status   = sqlc.arg(match_status),
    match_source   = sqlc.narg(match_source),
    match_attempts = match_attempts + 1,
    next_match_at  = CASE WHEN CAST(sqlc.arg(retry_after) AS TEXT) = '' THEN NULL
                          ELSE datetime('now', CAST(sqlc.arg(retry_after) AS TEXT)) END,
    updated_at     = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND match_status <> 'manual';

-- name: FindFlixPatrolTitlesByName :many
-- FlixPatrol titles of a kind with this exact name (case-insensitive) that
-- charted on Netflix, to match Netflix titles without another lookup.
SELECT DISTINCT t.*
FROM titles t
JOIN rankings r ON r.title_id = t.id AND r.streaming_provider = 'netflix'
WHERE t.kind = sqlc.arg(kind)
  AND COALESCE(t.tmdb_id, '') <> ''
  AND (lower(t.name) = lower(sqlc.arg(name)) OR lower(COALESCE(t.fp_name, '')) = lower(sqlc.arg(name)));

-- name: CountNetflixTitlesByMatchStatus :many
SELECT match_status, COUNT(*) AS titles FROM netflix_titles GROUP BY match_status;

-- name: UpsertIMDbRating :exec
INSERT INTO imdb_ratings (imdb_id, average_rating, num_votes, imported_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(imdb_id) DO UPDATE SET
    average_rating = excluded.average_rating, num_votes = excluded.num_votes, imported_at = CURRENT_TIMESTAMP;

-- name: ListKnownIMDbIDs :many
-- Every IMDb ID metareel knows, for filtering the IMDb ratings dataset.
SELECT imdb_id FROM titles WHERE COALESCE(imdb_id, '') <> ''
UNION SELECT imdb_id FROM tmdb_titles WHERE COALESCE(imdb_id, '') <> ''
UNION SELECT imdb_id FROM title_ratings WHERE COALESCE(imdb_id, '') <> ''
UNION SELECT imdb_id FROM netflix_titles WHERE COALESCE(imdb_id, '') <> '';

-- name: GetIMDbRating :one
SELECT * FROM imdb_ratings WHERE imdb_id = ?;

-- name: ListIMDbRatings :many
SELECT * FROM imdb_ratings WHERE imdb_id IN (sqlc.slice(ids));

-- name: CountIMDbRatings :one
SELECT COUNT(*) FROM imdb_ratings;
