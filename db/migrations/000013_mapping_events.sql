-- +goose Up
-- Every change to what a FlixPatrol or Netflix title is mapped to, written by
-- the single mapping writer in the same transaction as the change. Kept
-- without foreign keys so the history outlives deleted titles.
CREATE TABLE mapping_events (
    id             INTEGER PRIMARY KEY,
    subject_type   TEXT NOT NULL,          -- flixpatrol | netflix
    subject_id     INTEGER NOT NULL,       -- titles.id | netflix_titles.id
    action         TEXT NOT NULL,          -- match | manual | adopt | remap | clear | unshare | requeue
    from_tmdb_kind TEXT,                   -- movie | tv; NULL while unmapped
    from_tmdb_id   TEXT,
    to_tmdb_kind   TEXT,
    to_tmdb_id     TEXT,
    source         TEXT,                   -- match source after the change (e.g. justwatch, netflix_id, manual)
    actor          TEXT NOT NULL,          -- job type, mapping.sync, or admin
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_mapping_events_subject ON mapping_events(subject_type, subject_id, id);
CREATE INDEX idx_mapping_events_to ON mapping_events(to_tmdb_kind, to_tmdb_id);
CREATE INDEX idx_mapping_events_from ON mapping_events(from_tmdb_kind, from_tmdb_id);

-- IMDb / Wikidata IDs of a TMDB title set by hand: TMDB details, Wikidata and
-- matching never replace them (clearing one by hand clears the flag).
ALTER TABLE tmdb_titles ADD COLUMN imdb_manual INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tmdb_titles ADD COLUMN wikidata_manual INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE tmdb_titles DROP COLUMN wikidata_manual;
ALTER TABLE tmdb_titles DROP COLUMN imdb_manual;
DROP INDEX IF EXISTS idx_mapping_events_from;
DROP INDEX IF EXISTS idx_mapping_events_to;
DROP INDEX IF EXISTS idx_mapping_events_subject;
DROP TABLE IF EXISTS mapping_events;
