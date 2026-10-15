-- TMDB metadata, watch providers and Wikidata IDs per TMDB title.

-- name: UpsertTmdbTitleDetails :exec
-- TMDB's own IDs are stored with UpsertTmdbTitleIDs (source tmdb).
INSERT INTO tmdb_titles (
    tmdb_kind, tmdb_id, title, original_title, original_language, overview, status,
    release_date, last_air_date, runtime, number_of_seasons, number_of_episodes,
    genres, origin_countries, production_companies, networks, poster_path, backdrop_path,
    popularity, vote_average, vote_count, digital_release_dates, seasons,
    details_fetched_at
) VALUES (
    sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id), sqlc.narg(title), sqlc.narg(original_title), sqlc.narg(original_language),
    sqlc.narg(overview), sqlc.narg(status), sqlc.narg(release_date), sqlc.narg(last_air_date), sqlc.narg(runtime),
    sqlc.narg(number_of_seasons), sqlc.narg(number_of_episodes), sqlc.narg(genres), sqlc.narg(origin_countries),
    sqlc.narg(production_companies), sqlc.narg(networks), sqlc.narg(poster_path), sqlc.narg(backdrop_path),
    sqlc.narg(popularity), sqlc.narg(vote_average), sqlc.narg(vote_count), sqlc.narg(digital_release_dates),
    sqlc.narg(seasons), CURRENT_TIMESTAMP
)
ON CONFLICT(tmdb_kind, tmdb_id) DO UPDATE SET
    title = excluded.title, original_title = excluded.original_title, original_language = excluded.original_language,
    overview = excluded.overview, status = excluded.status, release_date = excluded.release_date,
    last_air_date = excluded.last_air_date, runtime = excluded.runtime, number_of_seasons = excluded.number_of_seasons,
    number_of_episodes = excluded.number_of_episodes, genres = excluded.genres, origin_countries = excluded.origin_countries,
    production_companies = excluded.production_companies, networks = excluded.networks,
    poster_path = excluded.poster_path, backdrop_path = excluded.backdrop_path, popularity = excluded.popularity,
    vote_average = excluded.vote_average, vote_count = excluded.vote_count,
    digital_release_dates = excluded.digital_release_dates, seasons = excluded.seasons,
    details_fetched_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP;

-- name: DeleteTmdbWatchProviders :exec
DELETE FROM tmdb_watch_providers WHERE tmdb_kind = ? AND tmdb_id = ?;

-- name: InsertTmdbWatchProvider :exec
INSERT INTO tmdb_watch_providers (tmdb_kind, tmdb_id, country, monetization, provider_id, provider_name, logo_path, display_priority, link)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(tmdb_kind, tmdb_id, country, monetization, provider_id) DO NOTHING;

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
-- TV shows stored before seasons were kept are due once.
SELECT k.tmdb_kind, k.tmdb_id
FROM (
    SELECT u.tmdb_kind, u.tmdb_id, MAX(u.last_seen) AS last_seen
    FROM (
        SELECT CASE t.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END AS tmdb_kind, t.tmdb_id AS tmdb_id,
               (SELECT MAX(r.ranked_on) FROM rankings r WHERE r.title_id = t.id) AS last_seen
        FROM titles t
        WHERE COALESCE(t.tmdb_id, '') <> ''
        UNION ALL
        SELECT n.tmdb_kind, n.tmdb_id,
               (SELECT MAX(g.week) FROM netflix_top10_global g WHERE g.netflix_title_id = n.id)
        FROM netflix_titles n
        WHERE COALESCE(n.tmdb_id, '') <> '' AND n.tmdb_kind IS NOT NULL
    ) u
    GROUP BY u.tmdb_kind, u.tmdb_id
) k
LEFT JOIN tmdb_titles m ON m.tmdb_kind = k.tmdb_kind AND m.tmdb_id = k.tmdb_id
WHERE m.details_fetched_at IS NULL
   OR (k.tmdb_kind = 'tv' AND m.seasons IS NULL)
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
    SELECT tmdb_kind, tmdb_id
    FROM netflix_titles WHERE COALESCE(tmdb_id, '') <> '' AND tmdb_kind IS NOT NULL
) k
LEFT JOIN tmdb_titles m ON m.tmdb_kind = k.tmdb_kind AND m.tmdb_id = k.tmdb_id
WHERE m.wikidata_fetched_at IS NULL OR m.wikidata_fetched_at < datetime('now', CAST(sqlc.arg(window) AS TEXT))
LIMIT sqlc.arg(limit);

-- name: CountTmdbTitles :one
SELECT COUNT(*) AS titles,
       CAST(COALESCE(SUM(CASE WHEN details_fetched_at IS NOT NULL THEN 1 ELSE 0 END), 0) AS INTEGER) AS with_details,
       CAST(COALESCE(SUM(CASE WHEN wikidata_id IS NOT NULL THEN 1 ELSE 0 END), 0) AS INTEGER) AS with_wikidata
FROM tmdb_titles;

-- name: FindTmdbRefsByNetflixID :many
SELECT tmdb_kind, tmdb_id FROM tmdb_titles WHERE netflix_id = ?;

-- name: UpsertTmdbTitleIDs :exec
-- Stores the IDs given (NULL = not given) from one source (manual | tmdb |
-- wikidata | match). An ID is only replaced by one from the same or a
-- higher-ranked source: manual > tmdb > wikidata > match.
INSERT INTO tmdb_titles (tmdb_kind, tmdb_id,
    imdb_id, imdb_source, wikidata_id, wikidata_source, rt_id, rt_source, metacritic_id, metacritic_source,
    letterboxd_id, letterboxd_source, netflix_id, netflix_source, justwatch_id, justwatch_source)
VALUES (sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id),
    sqlc.narg(imdb_id), CASE WHEN sqlc.narg(imdb_id) IS NULL THEN NULL ELSE CAST(sqlc.arg(source) AS TEXT) END,
    sqlc.narg(wikidata_id), CASE WHEN sqlc.narg(wikidata_id) IS NULL THEN NULL ELSE CAST(sqlc.arg(source) AS TEXT) END,
    sqlc.narg(rt_id), CASE WHEN sqlc.narg(rt_id) IS NULL THEN NULL ELSE CAST(sqlc.arg(source) AS TEXT) END,
    sqlc.narg(metacritic_id), CASE WHEN sqlc.narg(metacritic_id) IS NULL THEN NULL ELSE CAST(sqlc.arg(source) AS TEXT) END,
    sqlc.narg(letterboxd_id), CASE WHEN sqlc.narg(letterboxd_id) IS NULL THEN NULL ELSE CAST(sqlc.arg(source) AS TEXT) END,
    sqlc.narg(netflix_id), CASE WHEN sqlc.narg(netflix_id) IS NULL THEN NULL ELSE CAST(sqlc.arg(source) AS TEXT) END,
    sqlc.narg(justwatch_id), CASE WHEN sqlc.narg(justwatch_id) IS NULL THEN NULL ELSE CAST(sqlc.arg(source) AS TEXT) END)
ON CONFLICT(tmdb_kind, tmdb_id) DO UPDATE SET
    imdb_id = CASE WHEN excluded.imdb_id IS NOT NULL AND (CASE excluded.imdb_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END) >= (CASE tmdb_titles.imdb_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END)
        THEN excluded.imdb_id ELSE tmdb_titles.imdb_id END,
    imdb_source = CASE WHEN excluded.imdb_id IS NOT NULL AND (CASE excluded.imdb_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END) >= (CASE tmdb_titles.imdb_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END)
        THEN excluded.imdb_source ELSE tmdb_titles.imdb_source END,
    wikidata_id = CASE WHEN excluded.wikidata_id IS NOT NULL AND (CASE excluded.wikidata_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END) >= (CASE tmdb_titles.wikidata_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END)
        THEN excluded.wikidata_id ELSE tmdb_titles.wikidata_id END,
    wikidata_source = CASE WHEN excluded.wikidata_id IS NOT NULL AND (CASE excluded.wikidata_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END) >= (CASE tmdb_titles.wikidata_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END)
        THEN excluded.wikidata_source ELSE tmdb_titles.wikidata_source END,
    rt_id = CASE WHEN excluded.rt_id IS NOT NULL AND (CASE excluded.rt_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END) >= (CASE tmdb_titles.rt_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END)
        THEN excluded.rt_id ELSE tmdb_titles.rt_id END,
    rt_source = CASE WHEN excluded.rt_id IS NOT NULL AND (CASE excluded.rt_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END) >= (CASE tmdb_titles.rt_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END)
        THEN excluded.rt_source ELSE tmdb_titles.rt_source END,
    metacritic_id = CASE WHEN excluded.metacritic_id IS NOT NULL AND (CASE excluded.metacritic_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END) >= (CASE tmdb_titles.metacritic_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END)
        THEN excluded.metacritic_id ELSE tmdb_titles.metacritic_id END,
    metacritic_source = CASE WHEN excluded.metacritic_id IS NOT NULL AND (CASE excluded.metacritic_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END) >= (CASE tmdb_titles.metacritic_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END)
        THEN excluded.metacritic_source ELSE tmdb_titles.metacritic_source END,
    letterboxd_id = CASE WHEN excluded.letterboxd_id IS NOT NULL AND (CASE excluded.letterboxd_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END) >= (CASE tmdb_titles.letterboxd_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END)
        THEN excluded.letterboxd_id ELSE tmdb_titles.letterboxd_id END,
    letterboxd_source = CASE WHEN excluded.letterboxd_id IS NOT NULL AND (CASE excluded.letterboxd_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END) >= (CASE tmdb_titles.letterboxd_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END)
        THEN excluded.letterboxd_source ELSE tmdb_titles.letterboxd_source END,
    netflix_id = CASE WHEN excluded.netflix_id IS NOT NULL AND (CASE excluded.netflix_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END) >= (CASE tmdb_titles.netflix_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END)
        THEN excluded.netflix_id ELSE tmdb_titles.netflix_id END,
    netflix_source = CASE WHEN excluded.netflix_id IS NOT NULL AND (CASE excluded.netflix_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END) >= (CASE tmdb_titles.netflix_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END)
        THEN excluded.netflix_source ELSE tmdb_titles.netflix_source END,
    justwatch_id = CASE WHEN excluded.justwatch_id IS NOT NULL AND (CASE excluded.justwatch_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END) >= (CASE tmdb_titles.justwatch_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END)
        THEN excluded.justwatch_id ELSE tmdb_titles.justwatch_id END,
    justwatch_source = CASE WHEN excluded.justwatch_id IS NOT NULL AND (CASE excluded.justwatch_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END) >= (CASE tmdb_titles.justwatch_source WHEN 'manual' THEN 4 WHEN 'tmdb' THEN 3 WHEN 'wikidata' THEN 2 WHEN 'match' THEN 1 ELSE 0 END)
        THEN excluded.justwatch_source ELSE tmdb_titles.justwatch_source END,
    updated_at = CURRENT_TIMESTAMP;

-- name: ClearTmdbTitleIDs :exec
-- IDs cleared by hand (1 = clear): they are filled again from TMDB,
-- Wikidata or matching.
UPDATE tmdb_titles
SET imdb_id         = CASE WHEN CAST(sqlc.arg(imdb) AS INTEGER) = 1 THEN NULL ELSE imdb_id END,
    imdb_source     = CASE WHEN CAST(sqlc.arg(imdb) AS INTEGER) = 1 THEN NULL ELSE imdb_source END,
    wikidata_id     = CASE WHEN CAST(sqlc.arg(wikidata) AS INTEGER) = 1 THEN NULL ELSE wikidata_id END,
    wikidata_source = CASE WHEN CAST(sqlc.arg(wikidata) AS INTEGER) = 1 THEN NULL ELSE wikidata_source END,
    wikidata_fetched_at = CASE WHEN CAST(sqlc.arg(wikidata) AS INTEGER) = 1 THEN NULL ELSE wikidata_fetched_at END,
    rt_id           = CASE WHEN CAST(sqlc.arg(rt) AS INTEGER) = 1 THEN NULL ELSE rt_id END,
    rt_source       = CASE WHEN CAST(sqlc.arg(rt) AS INTEGER) = 1 THEN NULL ELSE rt_source END,
    rt_attempts     = CASE WHEN CAST(sqlc.arg(rt) AS INTEGER) = 1 THEN 0 ELSE rt_attempts END,
    next_rt_at      = CASE WHEN CAST(sqlc.arg(rt) AS INTEGER) = 1 THEN NULL ELSE next_rt_at END,
    updated_at      = CURRENT_TIMESTAMP
WHERE tmdb_kind = sqlc.arg(tmdb_kind) AND tmdb_id = sqlc.arg(tmdb_id);

-- name: MarkTmdbTitleWikidataFetched :exec
-- Records a Wikidata lookup (found or not), creating the row if needed.
INSERT INTO tmdb_titles (tmdb_kind, tmdb_id, wikidata_fetched_at)
VALUES (sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id), CURRENT_TIMESTAMP)
ON CONFLICT(tmdb_kind, tmdb_id) DO UPDATE SET wikidata_fetched_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP;

-- name: ListTmdbRefsDueForRT :many
-- TMDB titles FlixPatrol titles are mapped to, without a Rotten Tomatoes
-- slug, whose RT search backoff has expired; most recently charting first,
-- with the names to search by.
SELECT m.tmdb_kind, m.tmdb_id, CAST(COALESCE(m.title, '') AS TEXT) AS title,
       CAST(COALESCE(m.release_date, '') AS TEXT) AS release_date, m.rt_attempts,
       t.slug, t.name, t.matched_name, t.matched_year
FROM tmdb_titles m
JOIN titles t ON t.id = (
    SELECT t2.id FROM titles t2
    WHERE t2.tmdb_id = m.tmdb_id AND CASE t2.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END = m.tmdb_kind
    ORDER BY t2.id LIMIT 1)
WHERE COALESCE(m.rt_id, '') = ''
  AND (m.next_rt_at IS NULL OR m.next_rt_at <= CURRENT_TIMESTAMP)
  AND (CAST(sqlc.arg(only_ref) AS TEXT) = '' OR m.tmdb_kind || ':' || m.tmdb_id = sqlc.arg(only_ref))
ORDER BY (SELECT MAX(r.ranked_on) FROM rankings r WHERE r.title_id IN (
    SELECT t3.id FROM titles t3 WHERE t3.tmdb_id = m.tmdb_id AND CASE t3.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END = m.tmdb_kind)) DESC,
    m.tmdb_id DESC
LIMIT sqlc.arg(limit);

-- name: RecordTmdbRTAttempt :exec
-- One Rotten Tomatoes search for a TMDB title; retry after retry_after (a
-- SQLite datetime modifier). A slug found is stored with UpsertTmdbTitleIDs.
UPDATE tmdb_titles
SET rt_attempts = rt_attempts + 1,
    next_rt_at  = datetime('now', CAST(sqlc.arg(retry_after) AS TEXT)),
    updated_at  = CURRENT_TIMESTAMP
WHERE tmdb_kind = sqlc.arg(tmdb_kind) AND tmdb_id = sqlc.arg(tmdb_id);

-- name: EnsureTmdbTitle :exec
-- Every TMDB title anything is mapped to has a row (filled by the refresh).
INSERT INTO tmdb_titles (tmdb_kind, tmdb_id) VALUES (sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id))
ON CONFLICT(tmdb_kind, tmdb_id) DO NOTHING;
