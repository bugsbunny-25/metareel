-- Imports of a user's history from other trackers (TV Time, Suite): the
-- staged import, its items (one per title) and the watches it wrote.

-- name: CreateUserImport :one
INSERT INTO user_imports (user_id, source, file_name, tz, status, summary)
VALUES (sqlc.arg(user_id), sqlc.arg(source), sqlc.arg(file_name), sqlc.narg(tz), sqlc.arg(status), sqlc.arg(summary))
RETURNING id;

-- name: InsertUserImportItem :exec
INSERT INTO user_import_items (import_id, kind, source_ref, title, year, release_date, data, episodes_total)
VALUES (sqlc.arg(import_id), sqlc.arg(kind), sqlc.arg(source_ref), sqlc.arg(title), sqlc.narg(year),
        sqlc.narg(release_date), sqlc.arg(data), sqlc.arg(episodes_total));

-- name: GetUserImport :one
SELECT * FROM user_imports WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: ListUserImports :many
SELECT * FROM user_imports WHERE user_id = sqlc.arg(user_id) ORDER BY id DESC LIMIT 50;

-- NextActiveUserImport is the oldest import with background work left.
-- name: NextActiveUserImport :one
SELECT * FROM user_imports
WHERE status IN ('matching', 'importing')
ORDER BY id
LIMIT 1;

-- name: SetUserImportStatus :exec
UPDATE user_imports
SET status = sqlc.arg(status), error = sqlc.narg(error), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: MarkUserImportCommitted :exec
UPDATE user_imports
SET status = 'done', committed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: MarkUserImportUndone :exec
UPDATE user_imports
SET status = 'undone', undone_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: DeleteUserImport :execrows
DELETE FROM user_imports WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- CountUserImportItems is an import's progress, per kind and match status.
-- name: CountUserImportItems :many
SELECT kind, match_status,
    COUNT(*) AS items,
    CAST(COALESCE(SUM(episodes_total), 0) AS INTEGER) AS episodes_total,
    CAST(COALESCE(SUM(episodes_mapped), 0) AS INTEGER) AS episodes_mapped,
    CAST(COALESCE(SUM(CASE WHEN committed_at IS NOT NULL THEN 1 ELSE 0 END), 0) AS INTEGER) AS committed
FROM user_import_items
WHERE import_id = sqlc.arg(import_id)
GROUP BY kind, match_status;

-- SumUserImportResults adds up what the import wrote.
-- name: SumUserImportResults :one
SELECT
    CAST(COALESCE(SUM(json_extract(result, '$.added')), 0) AS INTEGER) AS added,
    CAST(COALESCE(SUM(json_extract(result, '$.duplicates')), 0) AS INTEGER) AS duplicates,
    CAST(COALESCE(SUM(json_extract(result, '$.unmapped')), 0) AS INTEGER) AS unmapped
FROM user_import_items
WHERE import_id = sqlc.arg(import_id) AND result IS NOT NULL;

-- name: ListUserImportItemsToMatch :many
SELECT * FROM user_import_items
WHERE import_id = sqlc.arg(import_id) AND match_status = 'pending'
ORDER BY id
LIMIT sqlc.arg(limit);

-- name: ListUserImportItemsToCommit :many
SELECT * FROM user_import_items
WHERE import_id = sqlc.arg(import_id) AND match_status = 'matched' AND committed_at IS NULL
ORDER BY id
LIMIT sqlc.arg(limit);

-- name: ListCommittedUserImportItems :many
SELECT * FROM user_import_items
WHERE import_id = sqlc.arg(import_id) AND committed_at IS NOT NULL
ORDER BY id;

-- name: GetUserImportItem :one
SELECT * FROM user_import_items
WHERE id = sqlc.arg(id) AND import_id = sqlc.arg(import_id);

-- ListUserImportItems is the review list: an import's items with what is
-- known about the TMDB title each was matched to.
-- name: ListUserImportItems :many
SELECT i.*,
    CAST(COALESCE(NULLIF(t.title, ''), NULLIF(c.title, ''), '') AS TEXT) AS tmdb_title,
    CAST(COALESCE(NULLIF(t.release_date, ''), NULLIF(c.release_date, ''), '') AS TEXT) AS tmdb_release_date,
    CAST(COALESCE(t.poster_path, c.poster_path, '') AS TEXT) AS tmdb_poster_path
FROM user_import_items i
LEFT JOIN tmdb_titles t ON t.tmdb_kind = i.kind AND t.tmdb_id = i.tmdb_id
LEFT JOIN tracked_titles c ON c.tmdb_kind = i.kind AND c.tmdb_id = i.tmdb_id
WHERE i.import_id = sqlc.arg(import_id)
  AND (CAST(sqlc.arg(kind) AS TEXT) = '' OR i.kind = sqlc.arg(kind))
  AND (CAST(sqlc.arg(match_status) AS TEXT) = '' OR i.match_status = sqlc.arg(match_status))
ORDER BY CASE i.match_status WHEN 'unmatched' THEN 0 WHEN 'pending' THEN 1 WHEN 'matched' THEN 2 ELSE 3 END,
         i.title COLLATE NOCASE, i.id
LIMIT sqlc.arg(limit) OFFSET sqlc.arg(offset);

-- name: CountUserImportItemsFiltered :one
SELECT COUNT(*) FROM user_import_items i
WHERE i.import_id = sqlc.arg(import_id)
  AND (CAST(sqlc.arg(kind) AS TEXT) = '' OR i.kind = sqlc.arg(kind))
  AND (CAST(sqlc.arg(match_status) AS TEXT) = '' OR i.match_status = sqlc.arg(match_status));

-- name: SaveUserImportItemMatch :exec
UPDATE user_import_items
SET match_status = sqlc.arg(match_status), match_method = sqlc.narg(match_method),
    tmdb_id = sqlc.narg(tmdb_id), candidates = sqlc.narg(candidates),
    data = sqlc.arg(data), episodes_mapped = sqlc.arg(episodes_mapped)
WHERE id = sqlc.arg(id);

-- name: SaveUserImportItemCommit :exec
UPDATE user_import_items
SET result = sqlc.arg(result), prev_state = sqlc.arg(prev_state), committed_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- ListTitleWatchTimes is every watch a user has of one title, for spotting
-- duplicates before an import writes more.
-- name: ListTitleWatchTimes :many
SELECT CAST(COALESCE(season_number, -1) AS INTEGER) AS season_number,
       CAST(COALESCE(episode_number, -1) AS INTEGER) AS episode_number,
       ended_at
FROM watch_entries
WHERE user_id = sqlc.arg(user_id) AND tmdb_kind = sqlc.arg(tmdb_kind) AND tmdb_id = sqlc.arg(tmdb_id);

-- name: DeleteImportedWatches :execrows
DELETE FROM watch_entries
WHERE user_id = sqlc.arg(user_id) AND import_id = sqlc.arg(import_id)
  AND tmdb_kind = sqlc.arg(tmdb_kind) AND tmdb_id = sqlc.arg(tmdb_id);

-- MoviesWatchedAt returns the movies a user has a watch of at exactly one of
-- these instants: an import from an app that copied another app's history
-- carries the same timestamps, which identifies a title its name cannot.
-- name: MoviesWatchedAt :many
SELECT DISTINCT tmdb_id FROM watch_entries
WHERE user_id = sqlc.arg(user_id) AND tmdb_kind = 'movie' AND ended_at IN (sqlc.slice(ended_at));
