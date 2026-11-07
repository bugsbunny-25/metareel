-- TMDB release dates of movies (migration 000024) and the observations of
-- when a movie reached streaming, for release windows.

-- name: DeleteReleaseDates :exec
DELETE FROM tmdb_release_dates WHERE tmdb_kind = sqlc.arg(tmdb_kind) AND tmdb_id = sqlc.arg(tmdb_id);

-- name: InsertReleaseDate :exec
INSERT INTO tmdb_release_dates (tmdb_kind, tmdb_id, country, release_type, release_date, note)
VALUES (sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id), sqlc.arg(country), sqlc.arg(release_type),
        CAST(sqlc.arg(release_date) AS TEXT), sqlc.narg(note))
ON CONFLICT(tmdb_kind, tmdb_id, country, release_type) DO UPDATE SET
    release_date = excluded.release_date, note = excluded.note;

-- name: ListMovieReleaseDates :many
SELECT country, release_type, release_date, CAST(COALESCE(note, '') AS TEXT) AS note
FROM tmdb_release_dates
WHERE tmdb_kind = 'movie' AND tmdb_id = sqlc.arg(tmdb_id)
ORDER BY country, release_type;

-- name: ListTheatricalReleaseDates :many
-- Every release date in a country of the movies with a theatrical release
-- (type 2 or 3) there between two dates, with the movie's title.
SELECT r.tmdb_id, r.release_type, r.release_date, CAST(COALESCE(r.note, '') AS TEXT) AS note,
       CAST(COALESCE(m.title, '') AS TEXT) AS title, CAST(COALESCE(m.poster_path, '') AS TEXT) AS poster_path
FROM tmdb_release_dates r
JOIN tmdb_titles m ON m.tmdb_kind = r.tmdb_kind AND m.tmdb_id = r.tmdb_id
WHERE r.tmdb_kind = 'movie' AND r.country = sqlc.arg(country)
  AND r.tmdb_id IN (
      SELECT x.tmdb_id FROM tmdb_release_dates x
      WHERE x.tmdb_kind = 'movie' AND x.country = sqlc.arg(country) AND x.release_type IN (2, 3)
        AND x.release_date >= CAST(sqlc.arg(from_date) AS TEXT) AND x.release_date <= CAST(sqlc.arg(to_date) AS TEXT)
  )
ORDER BY r.tmdb_id, r.release_type;

-- name: ListFirstStreamingOffers :many
-- The first time each subscription / free / ads offer of the movies was
-- seen arriving (watch_provider_events; offers present at a title's first
-- refresh have no event), per country (country '' = every country).
SELECT tmdb_id, country, provider_id, provider_name, monetization,
       CAST(substr(MIN(observed_at), 1, 10) AS TEXT) AS first_seen
FROM watch_provider_events
WHERE tmdb_kind = 'movie' AND event = 'added' AND monetization IN ('flatrate', 'free', 'ads')
  AND (CAST(sqlc.arg(country) AS TEXT) = '' OR country = sqlc.arg(country))
  AND tmdb_id IN (sqlc.slice(ids))
GROUP BY tmdb_id, country, provider_id, monetization
ORDER BY tmdb_id, country, first_seen;

-- name: ListFirstMovieCharts :many
-- Each movie's first FlixPatrol chart date per country and provider
-- (country '' = every country), with how many of the 14 days before it the
-- chart was stored (to tell a real chart debut from a chart that wasn't
-- stored yet, or had a gap).
SELECT f.tmdb_id, f.country, f.streaming_provider, f.first_chart,
       (SELECT COUNT(*) FROM chart_snapshots s
        WHERE s.country = f.country AND s.streaming_provider = f.streaming_provider AND s.category = 'movies'
          AND s.ranked_on >= date(f.first_chart, '-14 days') AND s.ranked_on < f.first_chart) AS stored_before
FROM (
    SELECT CAST(t.tmdb_id AS TEXT) AS tmdb_id, CAST(r.country AS TEXT) AS country, r.streaming_provider,
           CAST(MIN(r.ranked_on) AS TEXT) AS first_chart
    FROM rankings r
    JOIN titles t ON t.id = r.title_id
    WHERE t.kind = 'movie' AND t.tmdb_id IS NOT NULL
      AND (CAST(sqlc.arg(country) AS TEXT) = '' OR r.country = sqlc.arg(country))
      AND t.tmdb_id IN (sqlc.slice(ids))
    GROUP BY t.tmdb_id, r.country, r.streaming_provider
) f
ORDER BY f.tmdb_id, f.country, f.first_chart;

-- name: ListMovieStreamingOffers :many
-- Current subscription / free / ads offers of the movies (country '' =
-- every country).
SELECT tmdb_id, country, provider_id, provider_name, monetization
FROM tmdb_watch_providers
WHERE tmdb_kind = 'movie' AND monetization IN ('flatrate', 'free', 'ads')
  AND (CAST(sqlc.arg(country) AS TEXT) = '' OR country = sqlc.arg(country))
  AND tmdb_id IN (sqlc.slice(ids))
ORDER BY tmdb_id, country, display_priority, provider_id;
