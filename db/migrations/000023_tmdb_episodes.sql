-- +goose Up
-- Episodes of the TV shows metareel knows, from TMDB's season details
-- (tmdb.episodes.refresh): air dates feed the episode calendar and the
-- release-pattern analytic (binge / batch / weekly). Recent, upcoming and
-- latest seasons only (see EpisodesRefresher). air_date is TMDB's (usually
-- the original network or service release, not per country); NULL = TBA.
CREATE TABLE tmdb_episodes (
    tmdb_id        TEXT NOT NULL,      -- TMDB TV show ID
    season_number  INTEGER NOT NULL,
    episode_number INTEGER NOT NULL,
    name           TEXT,
    air_date       TEXT,               -- YYYY-MM-DD
    episode_type   TEXT,               -- standard | finale | mid_season (TMDB)
    runtime        INTEGER,            -- minutes
    PRIMARY KEY (tmdb_id, season_number, episode_number)
) WITHOUT ROWID;
CREATE INDEX idx_tmdb_episodes_air_date ON tmdb_episodes(air_date);

-- When the show's episodes were last fetched (NULL = never).
ALTER TABLE tmdb_titles ADD COLUMN episodes_fetched_at DATETIME;

-- Daily, after the TMDB metadata run (04:00) has refreshed season lists.
INSERT INTO task_schedules (task_type, name, enabled, max_retries, request_delay_seconds)
VALUES ('tmdb.episodes.refresh', 'TMDB episodes', 1, 2, 0)
ON CONFLICT(name) DO NOTHING;
INSERT INTO task_schedule_run_times (schedule_id, run_time_utc)
SELECT id, '05:00' FROM task_schedules WHERE name = 'TMDB episodes'
ON CONFLICT(schedule_id, run_time_utc) DO NOTHING;

-- +goose Down
DELETE FROM task_schedules WHERE task_type = 'tmdb.episodes.refresh';
ALTER TABLE tmdb_titles DROP COLUMN episodes_fetched_at;
DROP INDEX IF EXISTS idx_tmdb_episodes_air_date;
DROP TABLE IF EXISTS tmdb_episodes;
