-- +goose Up
-- Imports of a user's history from other trackers (TV Time, Suite).
--
-- An import is staged before anything reaches the diary: the upload is
-- parsed into one item per title, each item is matched to TMDB in the
-- background, the user reviews and fixes the matches, and only then are the
-- watches written. Every step keeps its progress in these rows, so a server
-- restart resumes an import where it stopped instead of failing it.
--
-- status: matching -> ready -> importing -> done (-> undone), or failed.
-- Statuses and sources are validated in Go, so new ones need no rebuild.
CREATE TABLE user_imports (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source       TEXT NOT NULL,                    -- tvtime | suite
    file_name    TEXT NOT NULL DEFAULT '',
    tz           TEXT,                             -- IANA zone the user watched in
    status       TEXT NOT NULL,
    error        TEXT,
    summary      TEXT NOT NULL DEFAULT '{}',       -- JSON: what the parse found and skipped
    created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    committed_at TEXT,
    undone_at    TEXT
);
CREATE INDEX idx_user_imports_user ON user_imports(user_id, id DESC);
CREATE INDEX idx_user_imports_active ON user_imports(status, id)
    WHERE status IN ('matching', 'importing');

-- One title of an import: what the export said about it (data, JSON: its
-- watches and flags), how it was matched to TMDB, and after the import what
-- was written (result) and the user's state for the title beforehand
-- (prev_state), which an undo puts back.
--
-- source_ref is the export's own id for the title: tvdb:<id>, tmdb:<id> or
-- tvtime:<uuid> (TV Time movies carry no outside id).
CREATE TABLE user_import_items (
    id              INTEGER PRIMARY KEY,
    import_id       INTEGER NOT NULL REFERENCES user_imports(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL,                 -- movie | tv
    source_ref      TEXT NOT NULL,
    title           TEXT NOT NULL DEFAULT '',
    year            INTEGER,
    release_date    TEXT,                          -- YYYY-MM-DD, when the export has one
    data            TEXT NOT NULL,
    match_status    TEXT NOT NULL DEFAULT 'pending', -- pending | matched | unmatched | skipped
    match_method    TEXT,                          -- tmdb_id | tvdb | search | manual
    tmdb_id         TEXT,
    candidates      TEXT,                          -- JSON: TMDB hits to pick from when unmatched
    episodes_total  INTEGER NOT NULL DEFAULT 0,
    episodes_mapped INTEGER NOT NULL DEFAULT 0,
    result          TEXT,                          -- JSON: added / duplicates / unmapped
    prev_state      TEXT,                          -- JSON user_titles state before; 'null' = none
    committed_at    TEXT,
    UNIQUE (import_id, kind, source_ref)
);
CREATE INDEX idx_user_import_items_status ON user_import_items(import_id, match_status, id);

-- The import a watch came from, so an import can be undone without touching
-- watches logged by hand. Not a foreign key: forgetting an import keeps its
-- watches.
ALTER TABLE watch_entries ADD COLUMN import_id INTEGER;
CREATE INDEX idx_watch_entries_import ON watch_entries(user_id, import_id)
    WHERE import_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_watch_entries_import;
ALTER TABLE watch_entries DROP COLUMN import_id;
DROP TABLE IF EXISTS user_import_items;
DROP TABLE IF EXISTS user_imports;
