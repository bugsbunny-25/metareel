-- TMDB episodes of TV shows (migration 000023).

-- name: ListEpisodeTargets :many
-- TV shows with a TMDB season list, the last episode fetch and whether a
-- stored episode airs in a window (an "airing" show is refreshed daily).
SELECT m.tmdb_id, CAST(COALESCE(m.status, '') AS TEXT) AS status, m.seasons,
       CAST(COALESCE(m.episodes_fetched_at, '') AS TEXT) AS episodes_fetched_at,
       CAST(EXISTS (
           SELECT 1 FROM tmdb_episodes e
           WHERE e.tmdb_id = m.tmdb_id
             AND e.air_date >= CAST(sqlc.arg(airing_from) AS TEXT) AND e.air_date <= CAST(sqlc.arg(airing_to) AS TEXT)
       ) AS INTEGER) AS airing
FROM tmdb_titles m
WHERE m.tmdb_kind = 'tv' AND m.seasons IS NOT NULL
ORDER BY m.episodes_fetched_at IS NOT NULL, m.episodes_fetched_at, m.tmdb_id;

-- name: DeleteSeasonEpisodes :exec
DELETE FROM tmdb_episodes WHERE tmdb_id = sqlc.arg(tmdb_id) AND season_number = sqlc.arg(season_number);

-- name: InsertEpisode :exec
INSERT INTO tmdb_episodes (tmdb_id, season_number, episode_number, name, air_date, episode_type, runtime)
VALUES (sqlc.arg(tmdb_id), sqlc.arg(season_number), sqlc.arg(episode_number), sqlc.narg(name),
        CAST(sqlc.narg(air_date) AS TEXT), sqlc.narg(episode_type), sqlc.narg(runtime))
ON CONFLICT(tmdb_id, season_number, episode_number) DO UPDATE SET
    name = excluded.name, air_date = excluded.air_date, episode_type = excluded.episode_type, runtime = excluded.runtime;

-- name: SetEpisodesFetched :exec
UPDATE tmdb_titles SET episodes_fetched_at = CURRENT_TIMESTAMP
WHERE tmdb_kind = 'tv' AND tmdb_id = sqlc.arg(tmdb_id);

-- name: ListCalendarEpisodes :many
-- Episodes airing between two dates of shows with a subscription, free or
-- ad-supported offer in a country (optionally from one provider).
SELECT e.tmdb_id, e.season_number, e.episode_number,
       CAST(COALESCE(e.name, '') AS TEXT) AS name, CAST(e.air_date AS TEXT) AS air_date,
       CAST(COALESCE(e.episode_type, '') AS TEXT) AS episode_type, e.runtime,
       CAST(COALESCE(m.title, '') AS TEXT) AS title, CAST(COALESCE(m.poster_path, '') AS TEXT) AS poster_path
FROM tmdb_episodes e
JOIN tmdb_titles m ON m.tmdb_kind = 'tv' AND m.tmdb_id = e.tmdb_id
WHERE e.air_date >= CAST(sqlc.arg(from_date) AS TEXT) AND e.air_date <= CAST(sqlc.arg(to_date) AS TEXT)
  AND EXISTS (
      SELECT 1 FROM tmdb_watch_providers w
      WHERE w.tmdb_kind = 'tv' AND w.tmdb_id = e.tmdb_id AND w.country = sqlc.arg(country)
        AND w.monetization IN ('flatrate', 'free', 'ads')
        AND (CAST(sqlc.arg(provider_id) AS INTEGER) = 0 OR w.provider_id = sqlc.arg(provider_id))
  )
ORDER BY e.air_date, m.title, e.tmdb_id, e.season_number, e.episode_number;

-- name: ListCalendarProviders :many
-- Subscription, free and ad-supported offers in a country of the shows
-- with an episode airing between two dates.
SELECT w.tmdb_id, w.provider_id, w.provider_name, w.monetization, CAST(COALESCE(w.url, '') AS TEXT) AS url
FROM tmdb_watch_providers w
WHERE w.tmdb_kind = 'tv' AND w.country = sqlc.arg(country)
  AND w.monetization IN ('flatrate', 'free', 'ads')
  AND w.tmdb_id IN (
      SELECT e.tmdb_id FROM tmdb_episodes e
      WHERE e.air_date >= CAST(sqlc.arg(from_date) AS TEXT) AND e.air_date <= CAST(sqlc.arg(to_date) AS TEXT)
  )
ORDER BY w.tmdb_id, w.display_priority, w.provider_id, w.monetization;

-- name: ListSeasonEpisodes :many
-- Every episode of the regular seasons whose first episode aired between
-- two dates, with the show's title.
SELECT e.tmdb_id, e.season_number, e.episode_number, CAST(COALESCE(e.air_date, '') AS TEXT) AS air_date,
       CAST(COALESCE(m.title, '') AS TEXT) AS title
FROM tmdb_episodes e
JOIN (
    SELECT x.tmdb_id, x.season_number, MIN(x.air_date) AS premiere
    FROM tmdb_episodes x
    WHERE x.season_number > 0 AND x.air_date IS NOT NULL
    GROUP BY x.tmdb_id, x.season_number
) s ON s.tmdb_id = e.tmdb_id AND s.season_number = e.season_number
JOIN tmdb_titles m ON m.tmdb_kind = 'tv' AND m.tmdb_id = e.tmdb_id
WHERE s.premiere >= CAST(sqlc.arg(from_date) AS TEXT) AND s.premiere <= CAST(sqlc.arg(to_date) AS TEXT)
ORDER BY e.tmdb_id, e.season_number, e.episode_number;

-- name: ListShowSeasonPremieres :many
-- First air date of each regular season of the shows with a season
-- premiering after a date (to cut a season's window at the next season).
SELECT x.tmdb_id, x.season_number, CAST(MIN(x.air_date) AS TEXT) AS premiere
FROM tmdb_episodes x
WHERE x.season_number > 0 AND x.air_date IS NOT NULL
  AND x.tmdb_id IN (SELECT y.tmdb_id FROM tmdb_episodes y WHERE y.air_date >= CAST(sqlc.arg(from_date) AS TEXT))
GROUP BY x.tmdb_id, x.season_number
ORDER BY x.tmdb_id, x.season_number;
