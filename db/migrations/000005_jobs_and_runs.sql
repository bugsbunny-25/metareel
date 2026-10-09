-- +goose NO TRANSACTION
-- +goose Up
-- Generalise schedules and runs beyond FlixPatrol scrapes:
--   * task_schedules.task_type is validated in Go (no CHECK), so new job types
--     need no table rebuild; FlixPatrol schedules gain backfill_days and
--     recheck_hours.
--   * task_runs.schedule_id is nullable (ad-hoc jobs such as backfills and
--     remaps have no schedule), status is validated in Go (adds 'partial'),
--     and runs carry target counts and a JSON summary.
-- SQLite can't drop a CHECK or a NOT NULL in place, so both tables are
-- rebuilt. Foreign keys are switched off while rebuilding so dropping the old
-- tables does not cascade-delete their children; this needs NO TRANSACTION
-- because the pragma is a no-op inside a transaction.
PRAGMA foreign_keys = OFF;

CREATE TABLE task_schedules_new (
    id                    INTEGER PRIMARY KEY,
    task_type             TEXT NOT NULL,
    name                  TEXT NOT NULL UNIQUE,
    enabled               BOOLEAN NOT NULL DEFAULT 1,
    max_retries           INTEGER NOT NULL DEFAULT 3 CHECK(max_retries >= 0),
    request_delay_seconds INTEGER NOT NULL DEFAULT 10 CHECK(request_delay_seconds >= 0),
    respect_robots        BOOLEAN NOT NULL DEFAULT 1,
    user_agent            TEXT NOT NULL DEFAULT 'metareel-flixpatrol-bot/0.1',
    backfill_days         INTEGER NOT NULL DEFAULT 0 CHECK(backfill_days BETWEEN 0 AND 60),
    recheck_hours         INTEGER NOT NULL DEFAULT 0 CHECK(recheck_hours BETWEEN 0 AND 23),
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO task_schedules_new (id, task_type, name, enabled, max_retries, request_delay_seconds, respect_robots, user_agent, created_at, updated_at)
SELECT id, task_type, name, enabled, max_retries, request_delay_seconds, respect_robots, user_agent, created_at, updated_at
FROM task_schedules;
DROP TABLE task_schedules;
ALTER TABLE task_schedules_new RENAME TO task_schedules;
CREATE INDEX idx_task_schedules_enabled ON task_schedules(enabled);

CREATE TABLE task_runs_new (
    id                INTEGER PRIMARY KEY,
    schedule_id       INTEGER REFERENCES task_schedules(id) ON DELETE CASCADE,
    task_type         TEXT NOT NULL,
    asynq_task_id     TEXT,
    status            TEXT NOT NULL,      -- started | succeeded | partial | failed (validated in Go)
    retry_count       INTEGER NOT NULL DEFAULT 0,
    max_retry         INTEGER NOT NULL DEFAULT 0,
    started_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at       DATETIME,
    error_message     TEXT,
    targets_total     INTEGER NOT NULL DEFAULT 0,
    targets_succeeded INTEGER NOT NULL DEFAULT 0,
    targets_failed    INTEGER NOT NULL DEFAULT 0,
    targets_skipped   INTEGER NOT NULL DEFAULT 0,
    summary           TEXT                -- JSON, job specific
);
INSERT INTO task_runs_new (id, schedule_id, task_type, asynq_task_id, status, retry_count, max_retry, started_at, finished_at, error_message)
SELECT id, schedule_id, task_type, asynq_task_id, status, retry_count, max_retry, started_at, finished_at, error_message
FROM task_runs;
DROP TABLE task_runs;
ALTER TABLE task_runs_new RENAME TO task_runs;
CREATE INDEX idx_task_runs_schedule_started_at ON task_runs(schedule_id, started_at DESC);
CREATE INDEX idx_task_runs_status ON task_runs(status);
CREATE INDEX idx_task_runs_task_type_started_at ON task_runs(task_type, started_at DESC);

PRAGMA foreign_keys = ON;

-- Existing scrapes fill gaps from the last 3 days and re-check for up to 6
-- hours when FlixPatrol has not published the day's chart yet.
UPDATE task_schedules SET backfill_days = 3, recheck_hours = 6 WHERE task_type = 'flixpatrol.top10.scrape';

-- Default schedules for the background jobs (editable in the admin UI).
INSERT INTO task_schedules (task_type, name, enabled, max_retries, request_delay_seconds)
VALUES
    ('titles.enrich',        'Title matching backlog',      1, 1, 10),
    ('titles.metadata',      'TMDB metadata + providers',   1, 1, 0),
    ('ratings.prewarm',      'Ratings pre-warm',            1, 1, 0),
    ('netflix.top10.import', 'Netflix official Top 10',     1, 2, 0),
    ('imdb.ratings.import',  'IMDb ratings dataset',        1, 2, 0),
    ('maintenance',          'Maintenance',                 1, 0, 0)
ON CONFLICT(name) DO NOTHING;

INSERT INTO task_schedule_run_times (schedule_id, run_time_utc)
SELECT s.id, v.run_time
FROM task_schedules s
JOIN (
    SELECT 'titles.enrich' AS task_type, '03:00' AS run_time
    UNION ALL SELECT 'titles.metadata', '04:00'
    UNION ALL SELECT 'ratings.prewarm', '15:30'
    UNION ALL SELECT 'netflix.top10.import', '21:00'
    UNION ALL SELECT 'imdb.ratings.import', '06:00'
    UNION ALL SELECT 'maintenance', '02:00'
) v ON v.task_type = s.task_type
ON CONFLICT(schedule_id, run_time_utc) DO NOTHING;

-- +goose Down
PRAGMA foreign_keys = OFF;

DELETE FROM task_schedules WHERE task_type <> 'flixpatrol.top10.scrape';

CREATE TABLE task_runs_old (
    id             INTEGER PRIMARY KEY,
    schedule_id    INTEGER NOT NULL REFERENCES task_schedules(id) ON DELETE CASCADE,
    task_type      TEXT NOT NULL,
    asynq_task_id  TEXT,
    status         TEXT NOT NULL CHECK(status IN ('started', 'succeeded', 'failed')),
    retry_count    INTEGER NOT NULL DEFAULT 0,
    max_retry      INTEGER NOT NULL DEFAULT 0,
    started_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at    DATETIME,
    error_message  TEXT
);
INSERT INTO task_runs_old (id, schedule_id, task_type, asynq_task_id, status, retry_count, max_retry, started_at, finished_at, error_message)
SELECT id, schedule_id, task_type, asynq_task_id,
       CASE WHEN status = 'partial' THEN 'failed' ELSE status END,
       retry_count, max_retry, started_at, finished_at, error_message
FROM task_runs
WHERE schedule_id IN (SELECT id FROM task_schedules);
DELETE FROM task_run_logs WHERE run_id NOT IN (SELECT id FROM task_runs_old);
DROP TABLE task_runs;
ALTER TABLE task_runs_old RENAME TO task_runs;
CREATE INDEX idx_task_runs_schedule_started_at ON task_runs(schedule_id, started_at DESC);
CREATE INDEX idx_task_runs_status ON task_runs(status);

CREATE TABLE task_schedules_old (
    id                    INTEGER PRIMARY KEY,
    task_type             TEXT NOT NULL CHECK(task_type IN ('flixpatrol.top10.scrape')),
    name                  TEXT NOT NULL UNIQUE,
    enabled               BOOLEAN NOT NULL DEFAULT 1,
    max_retries           INTEGER NOT NULL DEFAULT 3 CHECK(max_retries >= 0),
    request_delay_seconds INTEGER NOT NULL DEFAULT 10 CHECK(request_delay_seconds >= 0),
    respect_robots        BOOLEAN NOT NULL DEFAULT 1,
    user_agent            TEXT NOT NULL DEFAULT 'metareel-flixpatrol-bot/0.1',
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO task_schedules_old (id, task_type, name, enabled, max_retries, request_delay_seconds, respect_robots, user_agent, created_at, updated_at)
SELECT id, task_type, name, enabled, max_retries, request_delay_seconds, respect_robots, user_agent, created_at, updated_at
FROM task_schedules;
DROP TABLE task_schedules;
ALTER TABLE task_schedules_old RENAME TO task_schedules;
CREATE INDEX idx_task_schedules_enabled ON task_schedules(enabled);
DELETE FROM task_schedule_targets WHERE schedule_id NOT IN (SELECT id FROM task_schedules);
DELETE FROM task_schedule_run_times WHERE schedule_id NOT IN (SELECT id FROM task_schedules);

PRAGMA foreign_keys = ON;
