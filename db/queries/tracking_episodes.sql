-- Episode tracking: the cached episodes of tracked shows, a user's progress
-- through them, and the up-next queue. Every statement that touches a user's
-- watches is scoped by user_id.

-- name: UpsertTrackedEpisode :exec
INSERT INTO tracked_episodes (
    tmdb_id, season_number, episode_number, tmdb_episode_id, name, air_date, episode_type, runtime
) VALUES (
    sqlc.arg(tmdb_id), sqlc.arg(season_number), sqlc.arg(episode_number), sqlc.narg(tmdb_episode_id),
    sqlc.arg(name), sqlc.narg(air_date), sqlc.narg(episode_type), sqlc.narg(runtime)
)
ON CONFLICT(tmdb_id, season_number, episode_number) DO UPDATE SET
    tmdb_episode_id = excluded.tmdb_episode_id,
    name = excluded.name,
    air_date = excluded.air_date,
    episode_type = excluded.episode_type,
    runtime = excluded.runtime;

-- name: DeleteTrackedSeason :exec
DELETE FROM tracked_episodes
WHERE tmdb_id = sqlc.arg(tmdb_id) AND season_number = sqlc.arg(season_number);

-- name: SetTrackedEpisodesFetched :exec
UPDATE tracked_titles SET episodes_fetched_at = CURRENT_TIMESTAMP
WHERE tmdb_kind = 'tv' AND tmdb_id = sqlc.arg(tmdb_id);

-- name: GetTrackedEpisodesFetchedAt :one
SELECT CAST(COALESCE(episodes_fetched_at, '') AS TEXT) AS fetched_at
FROM tracked_titles
WHERE tmdb_kind = 'tv' AND tmdb_id = sqlc.arg(tmdb_id);

-- name: GetTrackedEpisode :one
SELECT tmdb_id, season_number, episode_number, tmdb_episode_id, name, air_date, episode_type, runtime
FROM tracked_episodes
WHERE tmdb_id = sqlc.arg(tmdb_id)
  AND season_number = sqlc.arg(season_number)
  AND episode_number = sqlc.arg(episode_number);

-- ListShowEpisodes is a show's episodes with how often the user has watched
-- each, for the show page.
-- name: ListShowEpisodes :many
SELECT e.season_number, e.episode_number, e.tmdb_episode_id, e.name, e.air_date, e.episode_type, e.runtime,
    CAST(COALESCE((SELECT COUNT(*) FROM watch_entries w
                   WHERE w.user_id = sqlc.arg(user_id) AND w.tmdb_kind = 'tv'
                     AND w.tmdb_id = e.tmdb_id AND w.season_number = e.season_number
                     AND w.episode_number = e.episode_number), 0) AS INTEGER) AS watch_count,
    CAST(COALESCE((SELECT MAX(w.ended_at) FROM watch_entries w
                   WHERE w.user_id = sqlc.arg(user_id) AND w.tmdb_kind = 'tv'
                     AND w.tmdb_id = e.tmdb_id AND w.season_number = e.season_number
                     AND w.episode_number = e.episode_number), '') AS TEXT) AS last_watched_at
FROM tracked_episodes e
WHERE e.tmdb_id = sqlc.arg(tmdb_id)
  AND (CAST(sqlc.arg(include_specials) AS INTEGER) = 1 OR e.season_number > 0)
ORDER BY e.season_number, e.episode_number;

-- ListMarkableEpisodes picks the episodes a bulk mark or unmark applies to.
-- scope is one of: episode (just this one), season (all of this season),
-- up_to (everything up to and including this one, across earlier seasons),
-- all (the whole show).
-- name: ListMarkableEpisodes :many
SELECT e.season_number, e.episode_number, e.tmdb_episode_id, e.runtime, e.name
FROM tracked_episodes e
WHERE e.tmdb_id = sqlc.arg(tmdb_id)
  AND (CAST(sqlc.arg(include_specials) AS INTEGER) = 1 OR e.season_number > 0)
  AND (CAST(sqlc.arg(aired_only) AS INTEGER) = 0
       OR (e.air_date IS NOT NULL AND e.air_date <> ''
           AND e.air_date <= CAST(sqlc.arg(today) AS TEXT)))
  AND (CAST(sqlc.arg(scope) AS TEXT) = 'all'
       OR (CAST(sqlc.arg(scope) AS TEXT) = 'season'
           AND e.season_number = sqlc.arg(season_number))
       OR (CAST(sqlc.arg(scope) AS TEXT) = 'episode'
           AND e.season_number = sqlc.arg(season_number)
           AND e.episode_number = sqlc.arg(episode_number))
       OR (CAST(sqlc.arg(scope) AS TEXT) = 'up_to'
           AND (e.season_number < sqlc.arg(season_number)
                OR (e.season_number = sqlc.arg(season_number)
                    AND e.episode_number <= sqlc.arg(episode_number)))))
  AND (CAST(sqlc.arg(only_unwatched) AS INTEGER) = 0
       OR NOT EXISTS (SELECT 1 FROM watch_entries w
                      WHERE w.user_id = sqlc.arg(user_id) AND w.tmdb_kind = 'tv'
                        AND w.tmdb_id = e.tmdb_id AND w.season_number = e.season_number
                        AND w.episode_number = e.episode_number))
ORDER BY e.season_number, e.episode_number;

-- DeleteEpisodeWatches removes every watch of the chosen episodes. scope is
-- as for ListMarkableEpisodes.
-- name: DeleteEpisodeWatches :execrows
DELETE FROM watch_entries
WHERE user_id = sqlc.arg(user_id)
  AND tmdb_kind = 'tv'
  AND tmdb_id = sqlc.arg(tmdb_id)
  AND season_number IS NOT NULL
  AND episode_number IS NOT NULL
  AND (CAST(sqlc.arg(include_specials) AS INTEGER) = 1 OR season_number > 0)
  AND (CAST(sqlc.arg(scope) AS TEXT) = 'all'
       OR (CAST(sqlc.arg(scope) AS TEXT) = 'season'
           AND season_number = sqlc.arg(season_number))
       OR (CAST(sqlc.arg(scope) AS TEXT) = 'episode'
           AND season_number = sqlc.arg(season_number)
           AND episode_number = sqlc.arg(episode_number))
       OR (CAST(sqlc.arg(scope) AS TEXT) = 'up_to'
           AND (season_number < sqlc.arg(season_number)
                OR (season_number = sqlc.arg(season_number)
                    AND episode_number <= sqlc.arg(episode_number)))));

-- ShowProgress counts a show's episodes, how many have aired and how many the
-- user has seen at least once. The denominator moves as episodes air, so it
-- is computed rather than stored.
-- name: ShowProgress :one
SELECT
    CAST(COUNT(*) AS INTEGER) AS total_episodes,
    CAST(COALESCE(SUM(CASE WHEN e.air_date IS NOT NULL AND e.air_date <> ''
                            AND e.air_date <= CAST(sqlc.arg(today) AS TEXT)
                           THEN 1 ELSE 0 END), 0) AS INTEGER) AS aired_episodes,
    CAST(COALESCE(SUM(CASE WHEN EXISTS (SELECT 1 FROM watch_entries w
                                        WHERE w.user_id = sqlc.arg(user_id) AND w.tmdb_kind = 'tv'
                                          AND w.tmdb_id = e.tmdb_id AND w.season_number = e.season_number
                                          AND w.episode_number = e.episode_number)
                           THEN 1 ELSE 0 END), 0) AS INTEGER) AS watched_episodes,
    CAST(COALESCE(MIN(CASE WHEN e.air_date > CAST(sqlc.arg(today) AS TEXT) THEN e.air_date END), '') AS TEXT) AS next_air_date
FROM tracked_episodes e
WHERE e.tmdb_id = sqlc.arg(tmdb_id)
  AND (CAST(sqlc.arg(include_specials) AS INTEGER) = 1 OR e.season_number > 0);

-- UpNext is the next aired, unwatched episode of each show the user is part
-- way through, most recently aired first. Specials are left out.
-- name: UpNext :many
WITH candidates AS (
    SELECT u.tmdb_id,
           e.season_number, e.episode_number, e.name, e.air_date, e.runtime, e.episode_type,
           ROW_NUMBER() OVER (PARTITION BY u.tmdb_id ORDER BY e.season_number, e.episode_number) AS rn
    FROM user_titles u
    JOIN tracked_episodes e ON e.tmdb_id = u.tmdb_id
    WHERE u.user_id = sqlc.arg(user_id)
      AND u.tmdb_kind = 'tv'
      AND u.status NOT IN ('completed', 'dropped', 'not_interested')
      AND (u.episodes_watched > 0 OR u.status = 'watching')
      AND e.season_number > 0
      AND e.air_date IS NOT NULL AND e.air_date <> ''
      AND e.air_date <= CAST(sqlc.arg(today) AS TEXT)
      AND NOT EXISTS (SELECT 1 FROM watch_entries w
                      WHERE w.user_id = sqlc.arg(user_id) AND w.tmdb_kind = 'tv'
                        AND w.tmdb_id = e.tmdb_id AND w.season_number = e.season_number
                        AND w.episode_number = e.episode_number)
)
SELECT c.tmdb_id, c.season_number, c.episode_number, c.name, c.air_date, c.runtime, c.episode_type,
    CAST(COALESCE(NULLIF(t.title, ''), NULLIF(tr.title, ''), '') AS TEXT) AS show_title,
    CAST(COALESCE(t.poster_path, tr.poster_path, '') AS TEXT) AS poster_path
FROM candidates c
LEFT JOIN tmdb_titles t ON t.tmdb_kind = 'tv' AND t.tmdb_id = c.tmdb_id
LEFT JOIN tracked_titles tr ON tr.tmdb_kind = 'tv' AND tr.tmdb_id = c.tmdb_id
WHERE c.rn = 1
ORDER BY c.air_date DESC, c.tmdb_id
LIMIT sqlc.arg(limit);

-- name: ListShowRefreshCandidates :many
-- The user's shows with when their episodes were last cached; never-fetched
-- first, then oldest. Shows they are not interested in are left out.
SELECT u.tmdb_id,
    CAST(COALESCE(tr.episodes_fetched_at, '') AS TEXT) AS fetched_at
FROM user_titles u
LEFT JOIN tracked_titles tr ON tr.tmdb_kind = 'tv' AND tr.tmdb_id = u.tmdb_id
WHERE u.user_id = sqlc.arg(user_id) AND u.tmdb_kind = 'tv' AND u.status <> 'not_interested'
ORDER BY tr.episodes_fetched_at IS NOT NULL, tr.episodes_fetched_at, u.tmdb_id;

-- name: ListShowQueue :many
-- Every show in the user's library with its progress (seasons 1+ only):
-- episodes listed, aired and watched, minutes left of aired unwatched ones,
-- the first aired unwatched episode and the next air date.
WITH shows AS (
    SELECT u.tmdb_id, u.status, u.favorite, u.watchlist_position, u.last_watched_at,
           u.created_at, u.updated_at
    FROM user_titles u
    WHERE u.user_id = sqlc.arg(user_id) AND u.tmdb_kind = 'tv' AND u.status <> 'not_interested'
),
eps AS (
    SELECT e.tmdb_id, e.season_number, e.episode_number, e.name, e.air_date, e.runtime, e.episode_type,
           CASE WHEN EXISTS (SELECT 1 FROM watch_entries w
                             WHERE w.user_id = sqlc.arg(user_id) AND w.tmdb_kind = 'tv'
                               AND w.tmdb_id = e.tmdb_id AND w.season_number = e.season_number
                               AND w.episode_number = e.episode_number) THEN 1 ELSE 0 END AS seen,
           CASE WHEN e.air_date IS NOT NULL AND e.air_date <> ''
                 AND e.air_date <= CAST(sqlc.arg(today) AS TEXT) THEN 1 ELSE 0 END AS aired
    FROM tracked_episodes e
    JOIN shows s ON s.tmdb_id = e.tmdb_id
    WHERE e.season_number > 0
),
agg AS (
    SELECT tmdb_id,
           COUNT(*) AS total,
           SUM(aired) AS aired,
           SUM(aired * seen) AS watched,
           SUM(CASE WHEN aired = 1 AND seen = 0 THEN COALESCE(runtime, 0) ELSE 0 END) AS minutes_left,
           MIN(CASE WHEN aired = 0 AND air_date IS NOT NULL AND air_date <> '' THEN air_date END) AS next_air_date
    FROM eps
    GROUP BY tmdb_id
),
nxt AS (
    SELECT tmdb_id, season_number, episode_number, name, air_date, runtime, episode_type,
           ROW_NUMBER() OVER (PARTITION BY tmdb_id ORDER BY season_number, episode_number) AS rn
    FROM eps
    WHERE aired = 1 AND seen = 0
)
SELECT s.tmdb_id, s.status, s.favorite,
    CAST(COALESCE(s.watchlist_position, -1) AS INTEGER) AS watchlist_position,
    CAST(COALESCE(s.last_watched_at, '') AS TEXT) AS last_watched_at,
    CAST(s.created_at AS TEXT) AS created_at,
    CAST(s.updated_at AS TEXT) AS updated_at,
    CAST(COALESCE(NULLIF(t.title, ''), NULLIF(tr.title, ''), '') AS TEXT) AS title,
    CAST(COALESCE(t.poster_path, tr.poster_path, '') AS TEXT) AS poster_path,
    CAST(COALESCE(a.total, 0) AS INTEGER) AS total_episodes,
    CAST(COALESCE(a.aired, 0) AS INTEGER) AS aired_episodes,
    CAST(COALESCE(a.watched, 0) AS INTEGER) AS watched_episodes,
    CAST(COALESCE(a.minutes_left, 0) AS INTEGER) AS minutes_left,
    CAST(COALESCE(a.next_air_date, '') AS TEXT) AS next_air_date,
    CAST(COALESCE(n.season_number, -1) AS INTEGER) AS next_season,
    CAST(COALESCE(n.episode_number, 0) AS INTEGER) AS next_episode,
    CAST(COALESCE(n.name, '') AS TEXT) AS next_name,
    CAST(COALESCE(n.air_date, '') AS TEXT) AS next_episode_air_date,
    CAST(COALESCE(n.runtime, 0) AS INTEGER) AS next_runtime,
    CAST(COALESCE(n.episode_type, '') AS TEXT) AS next_episode_type
FROM shows s
LEFT JOIN agg a ON a.tmdb_id = s.tmdb_id
LEFT JOIN nxt n ON n.tmdb_id = s.tmdb_id AND n.rn = 1
LEFT JOIN tmdb_titles t ON t.tmdb_kind = 'tv' AND t.tmdb_id = s.tmdb_id
LEFT JOIN tracked_titles tr ON tr.tmdb_kind = 'tv' AND tr.tmdb_id = s.tmdb_id
ORDER BY s.tmdb_id;

-- name: ListUpcomingEpisodes :many
-- Episodes airing after today and up to a day of shows in the user's
-- library, except the ones on hold, dropped or not wanted.
SELECT e.tmdb_id, e.season_number, e.episode_number, e.name, e.runtime, e.episode_type,
    CAST(e.air_date AS TEXT) AS air_date,
    u.status,
    CAST(COALESCE(NULLIF(t.title, ''), NULLIF(tr.title, ''), '') AS TEXT) AS show_title,
    CAST(COALESCE(t.poster_path, tr.poster_path, '') AS TEXT) AS poster_path
FROM user_titles u
JOIN tracked_episodes e ON e.tmdb_id = u.tmdb_id
LEFT JOIN tmdb_titles t ON t.tmdb_kind = 'tv' AND t.tmdb_id = u.tmdb_id
LEFT JOIN tracked_titles tr ON tr.tmdb_kind = 'tv' AND tr.tmdb_id = u.tmdb_id
WHERE u.user_id = sqlc.arg(user_id) AND u.tmdb_kind = 'tv'
  AND u.status NOT IN ('on_hold', 'dropped', 'not_interested')
  AND e.season_number > 0
  AND e.air_date > CAST(sqlc.arg(after) AS TEXT)
  AND e.air_date <= CAST(sqlc.arg(until) AS TEXT)
ORDER BY e.air_date, show_title, e.tmdb_id, e.season_number, e.episode_number
LIMIT sqlc.arg(limit);
