-- User tracking: watchlist and statuses (user_titles), the watch log
-- (watch_entries), per-watch tags and the on-demand TMDB title cache.
-- Every statement is scoped by user_id; nothing here may read another
-- user's rows.

-- name: UpsertTrackedTitle :exec
INSERT INTO tracked_titles (
    tmdb_kind, tmdb_id, title, original_title, release_date, year,
    poster_path, runtime, number_of_seasons, number_of_episodes, fetched_at
) VALUES (
    sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id), sqlc.arg(title), sqlc.arg(original_title),
    sqlc.narg(release_date), sqlc.narg(year), sqlc.narg(poster_path), sqlc.narg(runtime),
    sqlc.narg(number_of_seasons), sqlc.narg(number_of_episodes), CURRENT_TIMESTAMP
)
ON CONFLICT(tmdb_kind, tmdb_id) DO UPDATE SET
    title = excluded.title,
    original_title = excluded.original_title,
    release_date = excluded.release_date,
    year = excluded.year,
    poster_path = excluded.poster_path,
    runtime = excluded.runtime,
    number_of_seasons = excluded.number_of_seasons,
    number_of_episodes = excluded.number_of_episodes,
    fetched_at = CURRENT_TIMESTAMP;

-- GetTitleSnapshot returns one title for display, preferring the rich
-- tmdb_titles row of a charted title over the on-demand tracked_titles cache.
-- name: GetTitleSnapshot :one
SELECT
    CAST(COALESCE(NULLIF(t.title, ''), NULLIF(c.title, ''), '') AS TEXT) AS title,
    CAST(COALESCE(NULLIF(t.release_date, ''), NULLIF(c.release_date, ''), '') AS TEXT) AS release_date,
    CAST(COALESCE(t.poster_path, c.poster_path, '') AS TEXT) AS poster_path,
    CAST(COALESCE(t.runtime, c.runtime, 0) AS INTEGER) AS runtime,
    CAST(COALESCE(t.number_of_seasons, c.number_of_seasons, 0) AS INTEGER) AS number_of_seasons,
    CAST(COALESCE(t.number_of_episodes, c.number_of_episodes, 0) AS INTEGER) AS number_of_episodes,
    CAST(COALESCE(c.fetched_at, '') AS TEXT) AS cached_at,
    CAST(CASE WHEN t.tmdb_id IS NOT NULL THEN 1 ELSE 0 END AS INTEGER) AS charted
FROM (SELECT 1) AS one
LEFT JOIN tmdb_titles t
    ON t.tmdb_kind = sqlc.arg(tmdb_kind) AND t.tmdb_id = sqlc.arg(tmdb_id)
LEFT JOIN tracked_titles c
    ON c.tmdb_kind = sqlc.arg(tmdb_kind) AND c.tmdb_id = sqlc.arg(tmdb_id);

-- name: GetUserTitle :one
SELECT user_id, tmdb_kind, tmdb_id, status, rating, favorite, watchlist_position,
       watchlist_note, first_watched_at, last_watched_at, watch_count, episodes_watched,
       created_at, updated_at
FROM user_titles
WHERE user_id = sqlc.arg(user_id)
  AND tmdb_kind = sqlc.arg(tmdb_kind)
  AND tmdb_id = sqlc.arg(tmdb_id);

-- SetUserTitleState creates or updates the parts of a user's title state that
-- the client owns. The watch roll-up columns are left alone.
-- name: SetUserTitleState :exec
INSERT INTO user_titles (user_id, tmdb_kind, tmdb_id, status, favorite, watchlist_position, watchlist_note)
VALUES (sqlc.arg(user_id), sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id), sqlc.arg(status),
        sqlc.arg(favorite), sqlc.narg(watchlist_position), sqlc.narg(watchlist_note))
ON CONFLICT(user_id, tmdb_kind, tmdb_id) DO UPDATE SET
    status = excluded.status,
    favorite = excluded.favorite,
    watchlist_position = excluded.watchlist_position,
    watchlist_note = excluded.watchlist_note,
    updated_at = CURRENT_TIMESTAMP;

-- EnsureUserTitle creates an empty state row so a watch can roll up into it.
-- name: EnsureUserTitle :exec
INSERT INTO user_titles (user_id, tmdb_kind, tmdb_id)
VALUES (sqlc.arg(user_id), sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id))
ON CONFLICT(user_id, tmdb_kind, tmdb_id) DO NOTHING;

-- RollupUserTitle recomputes the watch roll-up of one title from the log:
-- watch count, distinct episodes seen, first and last watch, and the rating
-- of the most recent watch that carries one ("my rating" = latest watch's).
--
-- episodes_watched counts distinct episodes rather than watches, so a
-- rewatched episode still counts once towards progress. The season and
-- episode numbers are folded into one value because SQLite cannot correlate
-- a FROM-clause subquery with the row being updated.
-- name: RollupUserTitle :exec
UPDATE user_titles SET
    episodes_watched = (SELECT COUNT(DISTINCT w.season_number * 100000 + w.episode_number)
                        FROM watch_entries w
                        WHERE w.user_id = user_titles.user_id
                          AND w.tmdb_kind = user_titles.tmdb_kind
                          AND w.tmdb_id = user_titles.tmdb_id
                          AND w.season_number IS NOT NULL
                          AND w.episode_number IS NOT NULL),
    watch_count = (SELECT COUNT(*) FROM watch_entries w
                   WHERE w.user_id = user_titles.user_id
                     AND w.tmdb_kind = user_titles.tmdb_kind
                     AND w.tmdb_id = user_titles.tmdb_id),
    first_watched_at = (SELECT MIN(w.ended_at) FROM watch_entries w
                        WHERE w.user_id = user_titles.user_id
                          AND w.tmdb_kind = user_titles.tmdb_kind
                          AND w.tmdb_id = user_titles.tmdb_id),
    last_watched_at = (SELECT MAX(w.ended_at) FROM watch_entries w
                       WHERE w.user_id = user_titles.user_id
                         AND w.tmdb_kind = user_titles.tmdb_kind
                         AND w.tmdb_id = user_titles.tmdb_id),
    rating = (SELECT w.rating FROM watch_entries w
              WHERE w.user_id = user_titles.user_id
                AND w.tmdb_kind = user_titles.tmdb_kind
                AND w.tmdb_id = user_titles.tmdb_id
                AND w.rating IS NOT NULL
              ORDER BY w.ended_at DESC, w.id DESC LIMIT 1),
    updated_at = CURRENT_TIMESTAMP
WHERE user_titles.user_id = sqlc.arg(user_id)
  AND user_titles.tmdb_kind = sqlc.arg(tmdb_kind)
  AND user_titles.tmdb_id = sqlc.arg(tmdb_id);

-- name: DeleteUserTitle :execrows
DELETE FROM user_titles
WHERE user_id = sqlc.arg(user_id)
  AND tmdb_kind = sqlc.arg(tmdb_kind)
  AND tmdb_id = sqlc.arg(tmdb_id);

-- DeleteEmptyUserTitle removes a state row that holds nothing worth keeping
-- (no status, no rating, not a favourite, no watches), so clearing a
-- watchlist entry does not leave a dangling row behind.
-- name: DeleteEmptyUserTitle :execrows
DELETE FROM user_titles
WHERE user_id = sqlc.arg(user_id)
  AND tmdb_kind = sqlc.arg(tmdb_kind)
  AND tmdb_id = sqlc.arg(tmdb_id)
  AND status = ''
  AND rating IS NULL
  AND favorite = 0
  AND watch_count = 0;

-- name: NextWatchlistPosition :one
SELECT CAST(COALESCE(MAX(watchlist_position), 0) + 1 AS INTEGER) AS next_position
FROM user_titles
WHERE user_id = sqlc.arg(user_id) AND status = 'watchlist';

-- ListUserTitles lists a user's tracked titles with their display metadata.
-- An empty status lists every tracked title. The sort key is passed through a
-- one-row subquery because sqlc does not rewrite params inside ORDER BY.
-- name: ListUserTitles :many
SELECT
    u.tmdb_kind, u.tmdb_id, u.status, u.rating, u.favorite, u.watchlist_position,
    u.watchlist_note, u.first_watched_at, u.last_watched_at, u.watch_count, u.episodes_watched,
    u.created_at, u.updated_at,
    CAST(COALESCE(NULLIF(t.title, ''), NULLIF(c.title, ''), '') AS TEXT) AS title,
    CAST(COALESCE(NULLIF(t.release_date, ''), NULLIF(c.release_date, ''), '') AS TEXT) AS release_date,
    CAST(COALESCE(t.poster_path, c.poster_path, '') AS TEXT) AS poster_path,
    CAST(COALESCE(t.runtime, c.runtime, 0) AS INTEGER) AS runtime
FROM user_titles u, (SELECT CAST(sqlc.arg(sort) AS TEXT) AS sort_key) o
LEFT JOIN tmdb_titles t ON t.tmdb_kind = u.tmdb_kind AND t.tmdb_id = u.tmdb_id
LEFT JOIN tracked_titles c ON c.tmdb_kind = u.tmdb_kind AND c.tmdb_id = u.tmdb_id
WHERE u.user_id = sqlc.arg(user_id)
  AND (CAST(sqlc.arg(status) AS TEXT) = '' OR u.status = sqlc.arg(status))
  AND (CAST(sqlc.arg(tmdb_kind) AS TEXT) = '' OR u.tmdb_kind = sqlc.arg(tmdb_kind))
  AND (CAST(sqlc.arg(favorite_only) AS INTEGER) = 0 OR u.favorite = 1)
ORDER BY
    CASE WHEN o.sort_key = 'position_asc' THEN u.watchlist_position END ASC,
    CASE WHEN o.sort_key = 'rating_desc' THEN u.rating END DESC,
    CASE WHEN o.sort_key = 'watched_desc' THEN u.last_watched_at END DESC,
    CASE WHEN o.sort_key = 'title_asc' THEN COALESCE(NULLIF(t.title, ''), c.title) END COLLATE NOCASE ASC,
    u.updated_at DESC, u.tmdb_id DESC
LIMIT sqlc.arg(limit) OFFSET sqlc.arg(offset);

-- name: CountUserTitles :one
SELECT COUNT(*) FROM user_titles u
WHERE u.user_id = sqlc.arg(user_id)
  AND (CAST(sqlc.arg(status) AS TEXT) = '' OR u.status = sqlc.arg(status))
  AND (CAST(sqlc.arg(tmdb_kind) AS TEXT) = '' OR u.tmdb_kind = sqlc.arg(tmdb_kind))
  AND (CAST(sqlc.arg(favorite_only) AS INTEGER) = 0 OR u.favorite = 1);

-- name: InsertWatchEntry :one
INSERT INTO watch_entries (
    user_id, client_key, subject_kind, tmdb_kind, tmdb_id,
    season_number, episode_number, tmdb_episode_id,
    ended_at, started_at, runtime_minutes_used, runtime_source,
    tz, utc_offset_minutes, watched_on, time_precision,
    rating, liked, is_rewatch, watch_number, notes, venue_kind, source
) VALUES (
    sqlc.arg(user_id), sqlc.narg(client_key), sqlc.arg(subject_kind), sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id),
    sqlc.narg(season_number), sqlc.narg(episode_number), sqlc.narg(tmdb_episode_id),
    sqlc.arg(ended_at), sqlc.narg(started_at), sqlc.narg(runtime_minutes_used), sqlc.narg(runtime_source),
    sqlc.narg(tz), sqlc.narg(utc_offset_minutes), sqlc.arg(watched_on), sqlc.arg(time_precision),
    sqlc.narg(rating), sqlc.arg(liked), sqlc.arg(is_rewatch), sqlc.arg(watch_number),
    sqlc.narg(notes), sqlc.narg(venue_kind), sqlc.arg(source)
)
RETURNING id;

-- name: GetWatchEntry :one
SELECT * FROM watch_entries
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: GetWatchEntryByClientKey :one
SELECT * FROM watch_entries
WHERE user_id = sqlc.arg(user_id) AND client_key = sqlc.arg(client_key);

-- ListWatchEntries is the diary, newest local day first. Paging is by the
-- (watched_on, id) cursor; an empty cursor_on starts at the newest entry.
-- name: ListWatchEntries :many
SELECT
    w.id, w.subject_kind, w.tmdb_kind, w.tmdb_id, w.season_number, w.episode_number,
    w.ended_at, w.started_at, w.runtime_minutes_used, w.runtime_source, w.tz,
    w.utc_offset_minutes, w.watched_on, w.time_precision, w.rating, w.liked,
    w.is_rewatch, w.watch_number, w.notes, w.venue_kind, w.source, w.created_at,
    w.venue_id, w.venue_name_at_watch, w.venue_city_at_watch, w.screening_type,
    w.price_minor, w.currency, w.provider_name, w.media, w.dimension,
    CAST(COALESCE(NULLIF(t.title, ''), NULLIF(c.title, ''), '') AS TEXT) AS title,
    CAST(COALESCE(NULLIF(t.release_date, ''), NULLIF(c.release_date, ''), '') AS TEXT) AS release_date,
    CAST(COALESCE(t.poster_path, c.poster_path, '') AS TEXT) AS poster_path
FROM watch_entries w
LEFT JOIN tmdb_titles t ON t.tmdb_kind = w.tmdb_kind AND t.tmdb_id = w.tmdb_id
LEFT JOIN tracked_titles c ON c.tmdb_kind = w.tmdb_kind AND c.tmdb_id = w.tmdb_id
WHERE w.user_id = sqlc.arg(user_id)
  AND (CAST(sqlc.arg(tmdb_kind) AS TEXT) = '' OR w.tmdb_kind = sqlc.arg(tmdb_kind))
  AND (CAST(sqlc.arg(tmdb_id) AS TEXT) = '' OR w.tmdb_id = sqlc.arg(tmdb_id))
  AND (CAST(sqlc.arg(subject_kind) AS TEXT) = '' OR w.subject_kind = sqlc.arg(subject_kind))
  AND (CAST(sqlc.arg(from_day) AS TEXT) = '' OR w.watched_on >= CAST(sqlc.arg(from_day) AS TEXT))
  AND (CAST(sqlc.arg(to_day) AS TEXT) = '' OR w.watched_on <= CAST(sqlc.arg(to_day) AS TEXT))
  AND (CAST(sqlc.arg(rated_only) AS INTEGER) = 0 OR w.rating IS NOT NULL)
  AND (CAST(sqlc.arg(cursor_on) AS TEXT) = ''
       OR w.watched_on < CAST(sqlc.arg(cursor_on) AS TEXT)
       OR (w.watched_on = CAST(sqlc.arg(cursor_on) AS TEXT) AND w.id < sqlc.arg(cursor_id)))
ORDER BY w.watched_on DESC, w.id DESC
LIMIT sqlc.arg(limit);

-- name: UpdateWatchEntry :execrows
UPDATE watch_entries SET
    ended_at = sqlc.arg(ended_at),
    started_at = sqlc.narg(started_at),
    runtime_minutes_used = sqlc.narg(runtime_minutes_used),
    runtime_source = sqlc.narg(runtime_source),
    tz = sqlc.narg(tz),
    utc_offset_minutes = sqlc.narg(utc_offset_minutes),
    watched_on = sqlc.arg(watched_on),
    time_precision = sqlc.arg(time_precision),
    rating = sqlc.narg(rating),
    liked = sqlc.arg(liked),
    notes = sqlc.narg(notes),
    venue_kind = sqlc.narg(venue_kind),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: DeleteWatchEntry :execrows
DELETE FROM watch_entries WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- RenumberWatches stamps each of one title's watches with its chronological
-- ordinal: per episode for a show, per title for a movie, ordered by ended_at
-- and then id. A watch logged late for an earlier date therefore counts as
-- the first viewing, and an edit or delete that reorders watches renumbers
-- the rest. Run it in the same transaction as any write that adds, removes or
-- moves a watch, then SyncRewatchFlags. ended_at is UTC text in
-- "YYYY-MM-DD HH:MM:SS", so text order is time order.
-- name: RenumberWatches :exec
UPDATE watch_entries
SET watch_number = (
    SELECT COUNT(*) FROM watch_entries o
    WHERE o.user_id = watch_entries.user_id
      AND o.tmdb_kind = watch_entries.tmdb_kind
      AND o.tmdb_id = watch_entries.tmdb_id
      AND COALESCE(o.season_number, -1) = COALESCE(watch_entries.season_number, -1)
      AND COALESCE(o.episode_number, -1) = COALESCE(watch_entries.episode_number, -1)
      AND (o.ended_at < watch_entries.ended_at OR (o.ended_at = watch_entries.ended_at AND o.id <= watch_entries.id))
)
WHERE watch_entries.user_id = sqlc.arg(user_id)
  AND watch_entries.tmdb_kind = sqlc.arg(tmdb_kind)
  AND watch_entries.tmdb_id = sqlc.arg(tmdb_id);

-- SyncRewatchFlags sets is_rewatch from watch_number for one title's watches.
-- name: SyncRewatchFlags :exec
UPDATE watch_entries
SET is_rewatch = CASE WHEN watch_number > 1 THEN 1 ELSE 0 END
WHERE user_id = sqlc.arg(user_id)
  AND tmdb_kind = sqlc.arg(tmdb_kind)
  AND tmdb_id = sqlc.arg(tmdb_id);

-- name: UpsertUserTag :one
INSERT INTO user_tags (user_id, name)
VALUES (sqlc.arg(user_id), sqlc.arg(name))
ON CONFLICT(user_id, name) DO UPDATE SET name = excluded.name
RETURNING id, user_id, name, created_at;

-- name: ListUserTags :many
SELECT g.id, g.name, g.created_at,
       CAST(COUNT(et.entry_id) AS INTEGER) AS watch_count
FROM user_tags g
LEFT JOIN watch_entry_tags et ON et.tag_id = g.id
WHERE g.user_id = sqlc.arg(user_id)
GROUP BY g.id, g.name, g.created_at
ORDER BY g.name COLLATE NOCASE ASC;

-- name: DeleteUserTag :execrows
DELETE FROM user_tags WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: AddWatchEntryTag :exec
INSERT INTO watch_entry_tags (entry_id, tag_id)
VALUES (sqlc.arg(entry_id), sqlc.arg(tag_id))
ON CONFLICT(entry_id, tag_id) DO NOTHING;

-- name: ClearWatchEntryTags :exec
DELETE FROM watch_entry_tags WHERE entry_id = sqlc.arg(entry_id);

-- ListTagsForWatchEntries hydrates a page of diary entries with their tags.
-- The slice is the last parameter, as sqlc requires.
-- name: ListTagsForWatchEntries :many
SELECT et.entry_id, g.id AS tag_id, g.name
FROM watch_entry_tags et
JOIN user_tags g ON g.id = et.tag_id
JOIN watch_entries w ON w.id = et.entry_id
WHERE w.user_id = sqlc.arg(user_id)
  AND et.entry_id IN (sqlc.slice(entry_ids))
ORDER BY et.entry_id, g.name COLLATE NOCASE;
