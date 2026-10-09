-- TMDB metadata, watch providers and Wikidata IDs per TMDB title.

-- name: UpsertTmdbTitleDetails :exec
INSERT INTO tmdb_titles (
    tmdb_kind, tmdb_id, title, original_title, original_language, overview, status,
    release_date, last_air_date, runtime, number_of_seasons, number_of_episodes,
    genres, origin_countries, production_companies, networks, poster_path, backdrop_path,
    popularity, vote_average, vote_count, digital_release_dates, imdb_id, wikidata_id,
    details_fetched_at
) VALUES (
    sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id), sqlc.narg(title), sqlc.narg(original_title), sqlc.narg(original_language),
    sqlc.narg(overview), sqlc.narg(status), sqlc.narg(release_date), sqlc.narg(last_air_date), sqlc.narg(runtime),
    sqlc.narg(number_of_seasons), sqlc.narg(number_of_episodes), sqlc.narg(genres), sqlc.narg(origin_countries),
    sqlc.narg(production_companies), sqlc.narg(networks), sqlc.narg(poster_path), sqlc.narg(backdrop_path),
    sqlc.narg(popularity), sqlc.narg(vote_average), sqlc.narg(vote_count), sqlc.narg(digital_release_dates),
    sqlc.narg(imdb_id), sqlc.narg(wikidata_id), CURRENT_TIMESTAMP
)
ON CONFLICT(tmdb_kind, tmdb_id) DO UPDATE SET
    title = excluded.title, original_title = excluded.original_title, original_language = excluded.original_language,
    overview = excluded.overview, status = excluded.status, release_date = excluded.release_date,
    last_air_date = excluded.last_air_date, runtime = excluded.runtime, number_of_seasons = excluded.number_of_seasons,
    number_of_episodes = excluded.number_of_episodes, genres = excluded.genres, origin_countries = excluded.origin_countries,
    production_companies = excluded.production_companies, networks = excluded.networks,
    poster_path = excluded.poster_path, backdrop_path = excluded.backdrop_path, popularity = excluded.popularity,
    vote_average = excluded.vote_average, vote_count = excluded.vote_count,
    digital_release_dates = excluded.digital_release_dates,
    imdb_id = COALESCE(excluded.imdb_id, tmdb_titles.imdb_id),
    wikidata_id = COALESCE(excluded.wikidata_id, tmdb_titles.wikidata_id),
    details_fetched_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP;

-- name: DeleteTmdbWatchProviders :exec
DELETE FROM tmdb_watch_providers WHERE tmdb_kind = ? AND tmdb_id = ?;

-- name: InsertTmdbWatchProvider :exec
INSERT INTO tmdb_watch_providers (tmdb_kind, tmdb_id, country, monetization, provider_id, provider_name, logo_path, display_priority, link)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(tmdb_kind, tmdb_id, country, monetization, provider_id) DO NOTHING;

-- name: UpsertTmdbTitleWikidata :exec
INSERT INTO tmdb_titles (tmdb_kind, tmdb_id, imdb_id, wikidata_id, rt_id, metacritic_id, letterboxd_id, wikidata_fetched_at)
VALUES (sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id), sqlc.narg(imdb_id), sqlc.narg(wikidata_id), sqlc.narg(rt_id),
        sqlc.narg(metacritic_id), sqlc.narg(letterboxd_id), CURRENT_TIMESTAMP)
ON CONFLICT(tmdb_kind, tmdb_id) DO UPDATE SET
    imdb_id = COALESCE(tmdb_titles.imdb_id, excluded.imdb_id),
    wikidata_id = COALESCE(excluded.wikidata_id, tmdb_titles.wikidata_id),
    rt_id = COALESCE(excluded.rt_id, tmdb_titles.rt_id),
    metacritic_id = COALESCE(excluded.metacritic_id, tmdb_titles.metacritic_id),
    letterboxd_id = COALESCE(excluded.letterboxd_id, tmdb_titles.letterboxd_id),
    wikidata_fetched_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP;

-- name: GetTmdbTitle :one
SELECT * FROM tmdb_titles WHERE tmdb_kind = ? AND tmdb_id = ?;

-- name: ListTmdbTitlesByIDs :many
-- Callers filter by kind (TMDB movie and TV IDs overlap).
SELECT * FROM tmdb_titles WHERE tmdb_id IN (sqlc.slice(ids));

-- name: ListTmdbWatchProviders :many
SELECT * FROM tmdb_watch_providers
WHERE tmdb_kind = sqlc.arg(tmdb_kind) AND tmdb_id = sqlc.arg(tmdb_id)
  AND (CAST(sqlc.arg(country) AS TEXT) = '' OR country = sqlc.arg(country))
ORDER BY country, CASE monetization WHEN 'flatrate' THEN 0 WHEN 'free' THEN 1 WHEN 'ads' THEN 2 WHEN 'rent' THEN 3 ELSE 4 END,
         display_priority, provider_name;

-- name: ListTmdbRefsDueForDetails :many
-- Matched TMDB titles (from FlixPatrol or Netflix titles) whose details are
-- missing or older than the refresh window. Recently charting titles use the
-- short window (watch providers change), the rest the long one.
-- Windows are SQLite datetime modifiers, e.g. '-3 days' and '-30 days'.
SELECT k.tmdb_kind, k.tmdb_id
FROM (
    SELECT u.tmdb_kind, u.tmdb_id, MAX(u.last_seen) AS last_seen
    FROM (
        SELECT CASE t.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END AS tmdb_kind, t.tmdb_id AS tmdb_id,
               (SELECT MAX(r.ranked_on) FROM rankings r WHERE r.title_id = t.id) AS last_seen
        FROM titles t
        WHERE COALESCE(t.tmdb_id, '') <> ''
        UNION ALL
        SELECT CASE n.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END, n.tmdb_id,
               (SELECT MAX(g.week) FROM netflix_top10_global g WHERE g.netflix_title_id = n.id)
        FROM netflix_titles n
        WHERE COALESCE(n.tmdb_id, '') <> ''
    ) u
    GROUP BY u.tmdb_kind, u.tmdb_id
) k
LEFT JOIN tmdb_titles m ON m.tmdb_kind = k.tmdb_kind AND m.tmdb_id = k.tmdb_id
WHERE m.details_fetched_at IS NULL
   OR (k.last_seen >= date('now', '-30 days') AND m.details_fetched_at < datetime('now', CAST(sqlc.arg(recent_window) AS TEXT)))
   OR m.details_fetched_at < datetime('now', CAST(sqlc.arg(stale_window) AS TEXT))
ORDER BY m.details_fetched_at IS NOT NULL, k.last_seen DESC
LIMIT sqlc.arg(limit);

-- name: ListTmdbRefsDueForWikidata :many
-- Matched TMDB titles never looked up on Wikidata, or not in the window.
SELECT k.tmdb_kind, k.tmdb_id
FROM (
    SELECT CASE kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END AS tmdb_kind, tmdb_id
    FROM titles WHERE COALESCE(tmdb_id, '') <> ''
    UNION
    SELECT CASE kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END, tmdb_id
    FROM netflix_titles WHERE COALESCE(tmdb_id, '') <> ''
) k
LEFT JOIN tmdb_titles m ON m.tmdb_kind = k.tmdb_kind AND m.tmdb_id = k.tmdb_id
WHERE m.wikidata_fetched_at IS NULL OR m.wikidata_fetched_at < datetime('now', CAST(sqlc.arg(window) AS TEXT))
LIMIT sqlc.arg(limit);

-- name: CountTmdbTitles :one
SELECT COUNT(*) AS titles,
       CAST(COALESCE(SUM(CASE WHEN details_fetched_at IS NOT NULL THEN 1 ELSE 0 END), 0) AS INTEGER) AS with_details,
       CAST(COALESCE(SUM(CASE WHEN wikidata_id IS NOT NULL THEN 1 ELSE 0 END), 0) AS INTEGER) AS with_wikidata
FROM tmdb_titles;
