-- +goose Up
-- Movie and TV tracking for signed-in users: what they watchlist, what they
-- have watched (one row per watch, so rewatches are separate rows with their
-- own rating and tags) and the tags on each watch.
--
-- These tables are deliberately NOT foreign-keyed to the chart domain
-- (titles / tmdb_titles): a user can log anything on TMDB, including titles
-- that never appeared on a chart, and user data must never be rewritten by
-- the mapping or scraping jobs. The only foreign keys are to users(id).
--
-- All instants are TEXT 'YYYY-MM-DD HH:MM:SS' in UTC, the same format as
-- CURRENT_TIMESTAMP, so they sort and compare against datetime('now').

-- Minimal TMDB snapshot of the titles users track, so a diary page needs no
-- TMDB call per row. Display only and refreshed on demand; charted titles
-- keep their rich metadata in tmdb_titles, which reads prefer.
CREATE TABLE tracked_titles (
    tmdb_kind          TEXT NOT NULL,            -- movie | tv
    tmdb_id            TEXT NOT NULL,
    title              TEXT NOT NULL DEFAULT '',
    original_title     TEXT NOT NULL DEFAULT '',
    release_date       TEXT,                     -- YYYY-MM-DD, release / first air date
    year               INTEGER,
    poster_path        TEXT,
    runtime            INTEGER,                  -- minutes; TV: typical episode
    number_of_seasons  INTEGER,
    number_of_episodes INTEGER,
    fetched_at         TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tmdb_kind, tmdb_id)
) WITHOUT ROWID;

-- A user's state for one title: watchlist / status / favourite, plus the
-- roll-up of their watches so list pages never aggregate watch_entries.
-- rating mirrors the most recent watch's rating ("my rating" = latest watch).
-- status and tmdb_kind are validated in Go, not by a CHECK, so new values
-- need no table rebuild.
CREATE TABLE user_titles (
    user_id            INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tmdb_kind          TEXT NOT NULL,            -- movie | tv
    tmdb_id            TEXT NOT NULL,
    status             TEXT NOT NULL DEFAULT '', -- '' | watchlist | watching | completed | dropped | on_hold | not_interested
    rating             INTEGER,                  -- 1..10 half-stars (5 stars, Letterboxd style)
    favorite           INTEGER NOT NULL DEFAULT 0,
    watchlist_position INTEGER,
    watchlist_note     TEXT,
    first_watched_at   TEXT,
    last_watched_at    TEXT,
    watch_count        INTEGER NOT NULL DEFAULT 0,
    created_at         TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, tmdb_kind, tmdb_id),
    CHECK (rating IS NULL OR (rating >= 1 AND rating <= 10))
);
CREATE INDEX idx_user_titles_status ON user_titles(user_id, status);
CREATE INDEX idx_user_titles_watchlist ON user_titles(user_id, watchlist_position)
    WHERE status = 'watchlist';

-- One row per watch. A rewatch is another row, so there is no uniqueness on
-- (user, title); client_key makes a retried POST idempotent instead.
--
-- Time: the client logs ended_at in UTC and may send an explicit started_at.
-- The effective start is started_at, else ended_at minus the runtime; it is
-- recomputed on read and may drift if TMDB revises a runtime. watched_on is
-- the local day of that start, computed once at write time and frozen, so a
-- later runtime change can never shuffle a diary day.
-- The season/episode columns are unused until episode tracking lands; they
-- are here so that adding it needs no table rebuild.
CREATE TABLE watch_entries (
    id                   INTEGER PRIMARY KEY,
    user_id              INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_key           TEXT,                      -- idempotency key from the client
    subject_kind         TEXT NOT NULL,             -- movie | episode
    tmdb_kind            TEXT NOT NULL,             -- movie | tv
    tmdb_id              TEXT NOT NULL,
    season_number        INTEGER,                   -- episodes only
    episode_number       INTEGER,                   -- episodes only
    tmdb_episode_id      TEXT,                      -- stable across TMDB renumbering

    ended_at             TEXT NOT NULL,             -- UTC instant the watch ended
    started_at           TEXT,                      -- UTC, only when the client sends one
    runtime_minutes_used INTEGER,                   -- runtime behind a derived start
    runtime_source       TEXT,                      -- tmdb_title | tmdb_episode | user | unknown
    tz                   TEXT,                      -- IANA zone, e.g. America/New_York
    utc_offset_minutes   INTEGER,                   -- derived from tz at ended_at
    watched_on           TEXT NOT NULL,             -- YYYY-MM-DD local day of the start
    time_precision       TEXT NOT NULL DEFAULT 'minute', -- minute | day | month | year

    rating               INTEGER,                   -- 1..10 half-stars
    liked                INTEGER NOT NULL DEFAULT 0,
    is_rewatch           INTEGER NOT NULL DEFAULT 0,
    watch_number         INTEGER NOT NULL DEFAULT 1,
    notes                TEXT,
    venue_kind           TEXT,                      -- theater | streaming | physical | broadcast | other
    source               TEXT NOT NULL DEFAULT 'manual',
    created_at           TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (rating IS NULL OR (rating >= 1 AND rating <= 10))
);
CREATE UNIQUE INDEX idx_watch_entries_client_key ON watch_entries(user_id, client_key)
    WHERE client_key IS NOT NULL;
CREATE INDEX idx_watch_entries_user_day ON watch_entries(user_id, watched_on DESC, id DESC);
CREATE INDEX idx_watch_entries_user_title ON watch_entries(user_id, tmdb_kind, tmdb_id, ended_at DESC);
CREATE INDEX idx_watch_entries_user_episode ON watch_entries(user_id, tmdb_id, season_number, episode_number);

-- Tags belong to a user and are attached per watch, not per title.
CREATE TABLE user_tags (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL COLLATE NOCASE,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, name)
);
CREATE TABLE watch_entry_tags (
    entry_id INTEGER NOT NULL REFERENCES watch_entries(id) ON DELETE CASCADE,
    tag_id   INTEGER NOT NULL REFERENCES user_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (entry_id, tag_id)
);
CREATE INDEX idx_watch_entry_tags_tag ON watch_entry_tags(tag_id);

-- +goose Down
DROP INDEX IF EXISTS idx_watch_entry_tags_tag;
DROP TABLE IF EXISTS watch_entry_tags;
DROP TABLE IF EXISTS user_tags;
DROP INDEX IF EXISTS idx_watch_entries_user_episode;
DROP INDEX IF EXISTS idx_watch_entries_user_title;
DROP INDEX IF EXISTS idx_watch_entries_user_day;
DROP INDEX IF EXISTS idx_watch_entries_client_key;
DROP TABLE IF EXISTS watch_entries;
DROP INDEX IF EXISTS idx_user_titles_watchlist;
DROP INDEX IF EXISTS idx_user_titles_status;
DROP TABLE IF EXISTS user_titles;
DROP TABLE IF EXISTS tracked_titles;
