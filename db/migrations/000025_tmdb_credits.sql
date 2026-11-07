-- +goose Up
-- Cast and key crew of the titles metareel knows, from TMDB
-- (tmdb.credits.refresh): /movie/{id}/credits and TV aggregate credits plus
-- created_by. Only the top 25 cast and directing, writing, creator, music and
-- photography crew are kept. Feeds /titles/tmdb/{kind}/{id}/credits,
-- /people/{id} and /analytics/people.
CREATE TABLE tmdb_people (
    person_id    INTEGER PRIMARY KEY,    -- TMDB person ID
    name         TEXT NOT NULL,
    profile_path TEXT,
    known_for    TEXT,                   -- TMDB known_for_department (Acting, Directing, ...)
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE tmdb_credits (
    tmdb_kind     TEXT NOT NULL,
    tmdb_id       TEXT NOT NULL,
    person_id     INTEGER NOT NULL REFERENCES tmdb_people(person_id),
    credit_type   TEXT NOT NULL,         -- cast | crew (validated in Go)
    job           TEXT NOT NULL,         -- '' for cast; Director, Screenplay, Creator, ...
    department    TEXT NOT NULL,         -- Acting for cast
    character     TEXT,                  -- cast
    credit_order  INTEGER NOT NULL,      -- billing order (cast) / order kept (crew)
    episodes      INTEGER,               -- TV: episodes credited
    PRIMARY KEY (tmdb_kind, tmdb_id, credit_type, job, person_id),
    FOREIGN KEY (tmdb_kind, tmdb_id) REFERENCES tmdb_titles(tmdb_kind, tmdb_id) ON DELETE CASCADE
) WITHOUT ROWID;
CREATE INDEX idx_tmdb_credits_person ON tmdb_credits(person_id);

-- When the title's credits were last fetched (NULL = never).
ALTER TABLE tmdb_titles ADD COLUMN credits_fetched_at DATETIME;

INSERT INTO task_schedules (task_type, name, enabled, max_retries, request_delay_seconds)
VALUES ('tmdb.credits.refresh', 'TMDB credits', 1, 2, 0)
ON CONFLICT(name) DO NOTHING;
INSERT INTO task_schedule_run_times (schedule_id, run_time_utc)
SELECT id, '05:30' FROM task_schedules WHERE name = 'TMDB credits'
ON CONFLICT(schedule_id, run_time_utc) DO NOTHING;

-- +goose Down
DELETE FROM task_schedules WHERE task_type = 'tmdb.credits.refresh';
ALTER TABLE tmdb_titles DROP COLUMN credits_fetched_at;
DROP INDEX IF EXISTS idx_tmdb_credits_person;
DROP TABLE IF EXISTS tmdb_credits;
DROP TABLE IF EXISTS tmdb_people;
