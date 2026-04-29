-- +goose Up
-- Initial schema. Keep migrations small, additive, and reversible.

CREATE TABLE titles (
    id          INTEGER PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,  -- from flixpatrol URL e.g. "roommates-2026"
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK(kind IN ('movie', 'tv_show')),
    tmdb_id     TEXT,
    imdb_id     TEXT,                  -- e.g. "tt1234567"
    rt_url      TEXT,                  -- rotten tomatoes URL if available
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE rankings (
    id                           INTEGER PRIMARY KEY,
    title_id                     INTEGER NOT NULL REFERENCES titles(id),
    ranked_on                    DATE NOT NULL,          -- the date from the URL
    country                      CHAR(2) NOT NULL,       -- e.g. "US"
    streaming_provider           TEXT NOT NULL,          -- e.g. "netflix", "hbo-max"
    category                     TEXT NOT NULL CHECK(category IN ('movies', 'tv_shows', 'overall')),
    rank                         INTEGER NOT NULL CHECK(rank BETWEEN 1 AND 10),
    season_number                INTEGER,                -- NULL for movies, populated for tv_shows where known
    scraped_at                   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(ranked_on, country, streaming_provider, category, rank)
);

CREATE INDEX idx_rankings_date_country_streaming_provider ON rankings(ranked_on, country, streaming_provider);
CREATE INDEX idx_rankings_title_id ON rankings(title_id);
CREATE INDEX idx_titles_tmdb_id ON titles(tmdb_id);
CREATE INDEX idx_titles_slug ON titles(slug);

CREATE TABLE task_schedules (
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

CREATE TABLE task_schedule_targets (
    id            INTEGER PRIMARY KEY,
    schedule_id   INTEGER NOT NULL REFERENCES task_schedules(id) ON DELETE CASCADE,
    country_slug  TEXT NOT NULL,
    provider_slug TEXT NOT NULL,
    UNIQUE(schedule_id, country_slug, provider_slug)
);

CREATE TABLE task_schedule_run_times (
    id           INTEGER PRIMARY KEY,
    schedule_id  INTEGER NOT NULL REFERENCES task_schedules(id) ON DELETE CASCADE,
    run_time_utc TEXT NOT NULL, -- HH:MM in UTC
    UNIQUE(schedule_id, run_time_utc)
);

CREATE INDEX idx_task_schedules_enabled ON task_schedules(enabled);
CREATE INDEX idx_task_schedule_targets_schedule_id ON task_schedule_targets(schedule_id);
CREATE INDEX idx_task_schedule_run_times_schedule_id ON task_schedule_run_times(schedule_id);

CREATE TABLE task_runs (
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

CREATE TABLE task_run_logs (
    id          INTEGER PRIMARY KEY,
    run_id      INTEGER NOT NULL REFERENCES task_runs(id) ON DELETE CASCADE,
    level       TEXT NOT NULL CHECK(level IN ('debug', 'info', 'warn', 'error')),
    message     TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_task_runs_schedule_started_at ON task_runs(schedule_id, started_at DESC);
CREATE INDEX idx_task_runs_status ON task_runs(status);
CREATE INDEX idx_task_run_logs_run_id_created_at ON task_run_logs(run_id, created_at ASC);

-- +goose Down
DROP INDEX IF EXISTS idx_rankings_date_country_streaming_provider;
DROP INDEX IF EXISTS idx_rankings_title_id;
DROP INDEX IF EXISTS idx_titles_tmdb_id;
DROP INDEX IF EXISTS idx_titles_slug;
DROP INDEX IF EXISTS idx_task_schedules_enabled;
DROP INDEX IF EXISTS idx_task_schedule_targets_schedule_id;
DROP INDEX IF EXISTS idx_task_schedule_run_times_schedule_id;
DROP INDEX IF EXISTS idx_task_runs_schedule_started_at;
DROP INDEX IF EXISTS idx_task_runs_status;
DROP INDEX IF EXISTS idx_task_run_logs_run_id_created_at;

DROP TABLE IF EXISTS task_run_logs;
DROP TABLE IF EXISTS task_runs;
DROP TABLE IF EXISTS task_schedule_run_times;
DROP TABLE IF EXISTS task_schedule_targets;
DROP TABLE IF EXISTS task_schedules;
DROP TABLE IF EXISTS rankings;
DROP TABLE IF EXISTS titles;
