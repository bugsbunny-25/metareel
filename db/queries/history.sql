-- History of ratings, TMDB popularity and watch providers (migration 000017).
-- Rating rows are written only when the value changed since the previous row.

-- name: RecordIMDbRating :exec
INSERT INTO imdb_rating_history (imdb_id, captured_on, average_rating, num_votes)
SELECT sqlc.arg(imdb_id), CAST(sqlc.arg(captured_on) AS TEXT), sqlc.arg(average_rating), sqlc.arg(num_votes)
WHERE NOT EXISTS (
    SELECT 1 FROM (
        SELECT h.average_rating, h.num_votes FROM imdb_rating_history h
        WHERE h.imdb_id = sqlc.arg(imdb_id) AND h.captured_on < CAST(sqlc.arg(captured_on) AS TEXT)
        ORDER BY h.captured_on DESC LIMIT 1
    ) p
    WHERE p.average_rating = sqlc.arg(average_rating) AND p.num_votes = sqlc.arg(num_votes)
)
ON CONFLICT(imdb_id, captured_on) DO UPDATE SET
    average_rating = excluded.average_rating, num_votes = excluded.num_votes;

-- name: RecordTitleRating :exec
INSERT INTO title_rating_history (tmdb_kind, tmdb_id, provider, source, captured_on, value, votes)
SELECT sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id), sqlc.arg(provider), sqlc.arg(source),
       CAST(sqlc.arg(captured_on) AS TEXT), sqlc.arg(value), sqlc.narg(votes)
WHERE NOT EXISTS (
    SELECT 1 FROM (
        SELECT h.value, h.votes FROM title_rating_history h
        WHERE h.tmdb_kind = sqlc.arg(tmdb_kind) AND h.tmdb_id = sqlc.arg(tmdb_id)
          AND h.provider = sqlc.arg(provider) AND h.source = sqlc.arg(source)
          AND h.captured_on < CAST(sqlc.arg(captured_on) AS TEXT)
        ORDER BY h.captured_on DESC LIMIT 1
    ) p
    WHERE p.value = sqlc.arg(value) AND p.votes IS sqlc.narg(votes)
)
ON CONFLICT(tmdb_kind, tmdb_id, provider, source, captured_on) DO UPDATE SET
    value = excluded.value, votes = excluded.votes;

-- name: RecordTmdbPopularity :exec
INSERT INTO tmdb_popularity_history (tmdb_kind, tmdb_id, captured_on, popularity)
VALUES (sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id), CAST(sqlc.arg(captured_on) AS TEXT), sqlc.arg(popularity))
ON CONFLICT(tmdb_kind, tmdb_id, captured_on) DO UPDATE SET popularity = excluded.popularity;

-- name: ListKnownTmdbRefs :many
-- Every TMDB title metareel stores, for filtering TMDB's daily exports.
SELECT tmdb_kind, tmdb_id FROM tmdb_titles;

-- name: InsertWatchProviderEvent :exec
INSERT INTO watch_provider_events (tmdb_kind, tmdb_id, country, monetization, provider_id, provider_name, event, previous_check_at)
VALUES (sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id), sqlc.arg(country), sqlc.arg(monetization), sqlc.arg(provider_id),
        sqlc.arg(provider_name), sqlc.arg(event), CAST(sqlc.narg(previous_check_at) AS TEXT));

-- name: ListIMDbRatingHistory :many
-- Points up to the end date; the caller keeps the last one before the start
-- date as the series' starting value.
SELECT CAST(captured_on AS TEXT) AS captured_on, average_rating, num_votes
FROM imdb_rating_history
WHERE imdb_id = sqlc.arg(imdb_id) AND captured_on <= CAST(sqlc.arg(to_date) AS TEXT)
ORDER BY captured_on;

-- name: ListTitleRatingHistory :many
SELECT provider, source, CAST(captured_on AS TEXT) AS captured_on, value, votes
FROM title_rating_history
WHERE tmdb_kind = sqlc.arg(tmdb_kind) AND tmdb_id = sqlc.arg(tmdb_id)
  AND captured_on <= CAST(sqlc.arg(to_date) AS TEXT)
ORDER BY provider, source, captured_on;

-- name: ListTmdbPopularityHistory :many
SELECT CAST(captured_on AS TEXT) AS captured_on, popularity
FROM tmdb_popularity_history
WHERE tmdb_kind = sqlc.arg(tmdb_kind) AND tmdb_id = sqlc.arg(tmdb_id)
  AND captured_on >= CAST(sqlc.arg(from_date) AS TEXT) AND captured_on <= CAST(sqlc.arg(to_date) AS TEXT)
ORDER BY captured_on;

-- name: ListTitleWatchProviderEvents :many
SELECT id, tmdb_kind, tmdb_id, country, monetization, provider_id, provider_name, event,
       CAST(COALESCE(previous_check_at, '') AS TEXT) AS previous_check_at, CAST(observed_at AS TEXT) AS observed_at
FROM watch_provider_events
WHERE tmdb_kind = sqlc.arg(tmdb_kind) AND tmdb_id = sqlc.arg(tmdb_id)
  AND (CAST(sqlc.arg(country) AS TEXT) = '' OR country = sqlc.arg(country))
ORDER BY observed_at DESC, id DESC
LIMIT sqlc.arg(limit);

-- name: ListWatchProviderEvents :many
-- Changes across titles since a time (observed_at >= since, a
-- 'YYYY-MM-DD HH:MM:SS' UTC string), newest first, with the TMDB title.
SELECT e.id, e.tmdb_kind, e.tmdb_id, e.country, e.monetization, e.provider_id, e.provider_name, e.event,
       CAST(COALESCE(e.previous_check_at, '') AS TEXT) AS previous_check_at, CAST(e.observed_at AS TEXT) AS observed_at,
       CAST(COALESCE(m.title, '') AS TEXT) AS title, CAST(COALESCE(m.release_date, '') AS TEXT) AS release_date,
       CAST(COALESCE(m.poster_path, '') AS TEXT) AS poster_path
FROM watch_provider_events e
LEFT JOIN tmdb_titles m ON m.tmdb_kind = e.tmdb_kind AND m.tmdb_id = e.tmdb_id
WHERE e.observed_at >= CAST(sqlc.arg(since) AS TEXT)
  AND (CAST(sqlc.arg(country) AS TEXT) = '' OR e.country = sqlc.arg(country))
  AND (CAST(sqlc.arg(event) AS TEXT) = '' OR e.event = sqlc.arg(event))
  AND (CAST(sqlc.arg(monetization) AS TEXT) = '' OR e.monetization = sqlc.arg(monetization))
  AND (CAST(sqlc.arg(provider_id) AS INTEGER) = 0 OR e.provider_id = sqlc.arg(provider_id))
ORDER BY e.observed_at DESC, e.id DESC
LIMIT sqlc.arg(limit);

-- name: ListPageviewTargets :many
-- TMDB titles with an English Wikipedia article and the last day stored.
SELECT m.tmdb_kind, m.tmdb_id, CAST(m.enwiki_title AS TEXT) AS article,
       CAST(COALESCE((SELECT MAX(p.day) FROM wikipedia_pageviews p
                      WHERE p.tmdb_kind = m.tmdb_kind AND p.tmdb_id = m.tmdb_id AND p.project = 'en.wikipedia'), '') AS TEXT) AS last_day
FROM tmdb_titles m
WHERE COALESCE(m.enwiki_title, '') <> ''
ORDER BY last_day, m.tmdb_kind, m.tmdb_id;

-- name: SavePageview :exec
INSERT INTO wikipedia_pageviews (tmdb_kind, tmdb_id, project, day, views)
VALUES (sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id), sqlc.arg(project), CAST(sqlc.arg(day) AS TEXT), sqlc.arg(views))
ON CONFLICT(tmdb_kind, tmdb_id, project, day) DO UPDATE SET views = excluded.views;

-- name: ListPageviews :many
SELECT CAST(day AS TEXT) AS day, views FROM wikipedia_pageviews
WHERE tmdb_kind = sqlc.arg(tmdb_kind) AND tmdb_id = sqlc.arg(tmdb_id) AND project = sqlc.arg(project)
  AND day >= CAST(sqlc.arg(from_date) AS TEXT) AND day <= CAST(sqlc.arg(to_date) AS TEXT)
ORDER BY day;
