-- +goose Up
-- Season numbers for Netflix's official Top 10 and FlixPatrol TV rankings,
-- plus a separate Netflix title matching job.

-- One row per season title a Netflix show charted with (e.g. "Wednesday:
-- Season 2", "Adolescence: Limited Series"), mapped to a TMDB season number.
-- season_source: parsed | tmdb_name | number | tmdb_air_date | tmdb_single |
-- manual (validated in Go). Automation never changes manual rows.
CREATE TABLE netflix_seasons (
    id               INTEGER PRIMARY KEY,
    netflix_title_id INTEGER NOT NULL REFERENCES netflix_titles(id) ON DELETE CASCADE,
    season_title     TEXT NOT NULL,
    season_number    INTEGER,              -- NULL = not mapped yet
    season_source    TEXT,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(netflix_title_id, season_title)
);

-- TMDB season list of TV shows: JSON [{"season_number":1,"name":"Season 1",
-- "air_date":"2024-01-01","episode_count":8}]. NULL = not fetched yet.
ALTER TABLE tmdb_titles ADD COLUMN seasons TEXT;

-- Where rankings.season_number came from: chart (the chart name said so),
-- netflix (Netflix's official Top 10 that week) or tmdb_air_date (the latest
-- season aired by that date). NULL = unknown.
ALTER TABLE rankings ADD COLUMN season_source TEXT;
UPDATE rankings SET season_source = 'chart' WHERE season_number IS NOT NULL;

INSERT INTO task_schedules (task_type, name, enabled, max_retries, request_delay_seconds)
VALUES ('netflix.titles.match', 'Netflix title matching', 1, 1, 0)
ON CONFLICT(name) DO NOTHING;

INSERT INTO task_schedule_run_times (schedule_id, run_time_utc)
SELECT id, '09:00' FROM task_schedules WHERE name = 'Netflix title matching'
ON CONFLICT(schedule_id, run_time_utc) DO NOTHING;

-- +goose Down
DELETE FROM task_schedules WHERE task_type = 'netflix.titles.match';
ALTER TABLE rankings DROP COLUMN season_source;
ALTER TABLE tmdb_titles DROP COLUMN seasons;
DROP TABLE IF EXISTS netflix_seasons;
