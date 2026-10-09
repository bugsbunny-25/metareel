-- Operations: settings lookups, chart freshness.

-- name: GetAppSetting :one
SELECT value FROM app_settings WHERE key = ?;

-- name: SetAppSetting :exec
INSERT INTO app_settings (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP;

-- name: ListLatestChartDates :many
-- Latest stored date and scrape time of each country and provider chart.
SELECT country, streaming_provider,
       CAST(MAX(ranked_on) AS TEXT) AS latest_date,
       CAST(MAX(scraped_at) AS TEXT) AS last_scraped_at
FROM chart_snapshots
GROUP BY country, streaming_provider
ORDER BY country, streaming_provider;

-- name: WeeklyMappingCoverage :many
-- Share of chart entries whose title has a TMDB ID, per ISO week, from a date on.
SELECT strftime('%Y-%W', r.ranked_on) AS week,
       MIN(r.ranked_on) AS first_date,
       COUNT(*) AS entries,
       CAST(SUM(CASE WHEN COALESCE(t.tmdb_id, '') <> '' THEN 1 ELSE 0 END) AS INTEGER) AS mapped,
       CAST(SUM(CASE WHEN COALESCE(t.rt_url, '') <> '' THEN 1 ELSE 0 END) AS INTEGER) AS with_rt
FROM rankings r
JOIN titles t ON t.id = r.title_id
WHERE r.ranked_on >= CAST(sqlc.arg(since) AS TEXT)
GROUP BY week
ORDER BY week;

-- name: UnmatchedTitlesByCountry :many
-- Unmatched titles per country they charted in, from a date on.
SELECT CAST(r.country AS TEXT) AS country, COUNT(DISTINCT t.id) AS titles
FROM rankings r
JOIN titles t ON t.id = r.title_id
WHERE t.match_status IN ('pending', 'unmatched')
  AND r.ranked_on >= CAST(sqlc.arg(since) AS TEXT)
GROUP BY r.country
ORDER BY titles DESC;
