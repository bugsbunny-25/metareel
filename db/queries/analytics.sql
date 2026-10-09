-- Chart entries over a date range, for history, movers, leaderboards and
-- analytics (computed in Go). Empty filters match all.

-- name: ListChartEntries :many
SELECT CAST(r.ranked_on AS TEXT) AS ranked_on,
       CAST(r.country AS TEXT) AS country,
       r.streaming_provider,
       r.category,
       r.rank,
       r.season_number,
       t.id AS title_id,
       t.slug,
       t.name,
       t.kind,
       t.tmdb_id,
       t.imdb_id
FROM rankings r
JOIN titles t ON t.id = r.title_id
WHERE r.ranked_on >= CAST(sqlc.arg(from_date) AS TEXT)
  AND r.ranked_on <= CAST(sqlc.arg(to_date) AS TEXT)
  AND (CAST(sqlc.arg(country) AS TEXT) = '' OR r.country = sqlc.arg(country))
  AND (CAST(sqlc.arg(provider) AS TEXT) = '' OR r.streaming_provider = sqlc.arg(provider))
  AND (CAST(sqlc.arg(category) AS TEXT) = '' OR r.category = sqlc.arg(category))
ORDER BY r.ranked_on, r.country, r.streaming_provider, r.category, r.rank;

-- name: ListChartCatalog :many
-- Every stored chart: date range, number of dates, last scrape.
SELECT country, streaming_provider, category,
       CAST(MIN(ranked_on) AS TEXT) AS first_date,
       CAST(MAX(ranked_on) AS TEXT) AS latest_date,
       COUNT(*) AS dates,
       CAST(MAX(scraped_at) AS TEXT) AS last_scraped_at
FROM chart_snapshots
GROUP BY country, streaming_provider, category
ORDER BY country, streaming_provider, category;

-- name: ListChartDates :many
SELECT ranked_on, category, entry_count, scraped_at
FROM chart_snapshots
WHERE country = sqlc.arg(country)
  AND streaming_provider = sqlc.arg(provider)
  AND (CAST(sqlc.arg(category) AS TEXT) = '' OR category = sqlc.arg(category))
ORDER BY ranked_on DESC, category
LIMIT sqlc.arg(limit);

-- name: ListChartChangesSince :many
-- Charts stored or re-scraped after a timestamp ('YYYY-MM-DD HH:MM:SS' UTC).
SELECT * FROM chart_snapshots
WHERE scraped_at > CAST(sqlc.arg(since) AS TEXT)
ORDER BY scraped_at, id
LIMIT sqlc.arg(limit);

-- name: ListTitlesByTmdbRef :many
SELECT * FROM titles
WHERE kind = sqlc.arg(kind) AND tmdb_id = sqlc.arg(tmdb_id)
ORDER BY id;

-- name: FindTitlesByIMDb :many
SELECT * FROM titles WHERE imdb_id = ? ORDER BY id;

-- name: FindTmdbRefsByIMDb :many
SELECT tmdb_kind, tmdb_id FROM tmdb_titles WHERE imdb_id = ?;

-- name: ListNetflixGlobalWeek :many
SELECT g.*, n.tmdb_id, n.imdb_id, n.kind
FROM netflix_top10_global g
JOIN netflix_titles n ON n.id = g.netflix_title_id
WHERE g.week = sqlc.arg(week)
  AND (CAST(sqlc.arg(category) AS TEXT) = '' OR g.category = sqlc.arg(category))
ORDER BY g.category, g.weekly_rank;

-- name: ListNetflixCountryWeek :many
SELECT c.*, n.tmdb_id, n.imdb_id, n.kind
FROM netflix_top10_countries c
JOIN netflix_titles n ON n.id = c.netflix_title_id
WHERE c.country = sqlc.arg(country) AND c.week = sqlc.arg(week)
  AND (CAST(sqlc.arg(category) AS TEXT) = '' OR c.category = sqlc.arg(category))
ORDER BY c.category, c.weekly_rank;

-- name: LatestNetflixGlobalWeek :one
SELECT CAST(COALESCE(MAX(week), '') AS TEXT) FROM netflix_top10_global;

-- name: LatestNetflixCountryWeek :one
SELECT CAST(COALESCE(MAX(week), '') AS TEXT) FROM netflix_top10_countries WHERE country = ?;

-- name: ListNetflixMostPopular :many
SELECT p.*, n.tmdb_id, n.imdb_id, n.kind
FROM netflix_most_popular p
JOIN netflix_titles n ON n.id = p.netflix_title_id
WHERE (CAST(sqlc.arg(category) AS TEXT) = '' OR p.category = sqlc.arg(category))
ORDER BY p.category, p.rank;

-- name: ListNetflixGlobalByTmdb :many
SELECT g.*
FROM netflix_top10_global g
JOIN netflix_titles n ON n.id = g.netflix_title_id
WHERE n.kind = sqlc.arg(kind) AND n.tmdb_id = sqlc.arg(tmdb_id)
ORDER BY g.week, g.category;

-- name: ListNetflixCountriesByTmdb :many
SELECT c.*
FROM netflix_top10_countries c
JOIN netflix_titles n ON n.id = c.netflix_title_id
WHERE n.kind = sqlc.arg(kind) AND n.tmdb_id = sqlc.arg(tmdb_id)
ORDER BY c.week, c.country;

-- name: ListNetflixGlobalRange :many
-- Global weekly rows with TMDB IDs in a week range, for calibration.
SELECT g.week, g.category, g.weekly_rank, g.show_title, g.weekly_hours_viewed, g.weekly_views, n.tmdb_id, n.kind
FROM netflix_top10_global g
JOIN netflix_titles n ON n.id = g.netflix_title_id
WHERE g.week >= CAST(sqlc.arg(from_week) AS TEXT) AND g.week <= CAST(sqlc.arg(to_week) AS TEXT)
ORDER BY g.week, g.category, g.weekly_rank;
