-- +goose Up
-- watch_number used to be stamped as MAX + 1 when a watch was logged, so a
-- watch logged late for an earlier date counted as a rewatch, and edits and
-- deletes never renumbered. Writes now renumber a title in time order (see
-- RenumberWatches); this repairs the rows stored before that. Same rule: per
-- episode for a show, per title for a movie, ordered by ended_at then id.
UPDATE watch_entries
SET watch_number = (
    SELECT COUNT(*) FROM watch_entries o
    WHERE o.user_id = watch_entries.user_id
      AND o.tmdb_kind = watch_entries.tmdb_kind
      AND o.tmdb_id = watch_entries.tmdb_id
      AND COALESCE(o.season_number, -1) = COALESCE(watch_entries.season_number, -1)
      AND COALESCE(o.episode_number, -1) = COALESCE(watch_entries.episode_number, -1)
      AND (o.ended_at < watch_entries.ended_at OR (o.ended_at = watch_entries.ended_at AND o.id <= watch_entries.id))
);
UPDATE watch_entries SET is_rewatch = CASE WHEN watch_number > 1 THEN 1 ELSE 0 END;

-- +goose Down
-- Nothing to undo: the old numbers came from the order watches were logged,
-- which isn't stored, and the time-ordered numbers are valid under the old
-- code as well.
SELECT 1;
