-- +goose Up
-- Netflix's "What We Watched" engagement reports (netflix.engagement.import):
-- hours viewed and views of every title watched for 50k+ hours in a period
-- (six months; yearly from 2027), far beyond the weekly Top 10. A row is
-- linked to the Netflix Top 10 title (and season) of the same name when there
-- is one, which maps it to TMDB. release_date is only given for Netflix
-- originals.
CREATE TABLE netflix_engagement (
    period_start       TEXT NOT NULL,   -- YYYY-MM-DD, first day of the report period
    period_end         TEXT NOT NULL,   -- YYYY-MM-DD, last day
    kind               TEXT NOT NULL,   -- movie | tv_show (the report's sheet)
    title              TEXT NOT NULL,   -- as in the report (non-English: "English // Original")
    available_globally BOOLEAN,
    release_date       TEXT,            -- YYYY-MM-DD, Netflix originals only
    hours_viewed       INTEGER,
    runtime_minutes    INTEGER,
    views              INTEGER,
    netflix_title_id   INTEGER REFERENCES netflix_titles(id) ON DELETE SET NULL,
    season_number      INTEGER,
    PRIMARY KEY (period_start, kind, title)
);
CREATE INDEX idx_netflix_engagement_title ON netflix_engagement(netflix_title_id);

INSERT INTO task_schedules (task_type, name, enabled, max_retries, request_delay_seconds)
VALUES ('netflix.engagement.import', 'Netflix engagement reports', 1, 2, 0)
ON CONFLICT(name) DO NOTHING;
INSERT INTO task_schedule_run_times (schedule_id, run_time_utc)
SELECT id, '07:00' FROM task_schedules WHERE name = 'Netflix engagement reports'
ON CONFLICT(schedule_id, run_time_utc) DO NOTHING;

-- +goose Down
DELETE FROM task_schedules WHERE task_type = 'netflix.engagement.import';
DROP INDEX IF EXISTS idx_netflix_engagement_title;
DROP TABLE IF EXISTS netflix_engagement;
DELETE FROM app_settings WHERE key = 'netflix.engagement.urls';
