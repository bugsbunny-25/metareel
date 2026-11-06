-- +goose Up
-- English Wikipedia article of each TMDB title (Wikidata sitelink, refreshed
-- by titles.metadata) and its daily user pageviews (wikipedia.pageviews.import,
-- from the Wikimedia REST API), a demand signal outside streaming.
ALTER TABLE tmdb_titles ADD COLUMN enwiki_title TEXT;
ALTER TABLE tmdb_titles ADD COLUMN enwiki_fetched_at DATETIME;

CREATE TABLE wikipedia_pageviews (
    tmdb_kind TEXT NOT NULL,
    tmdb_id   TEXT NOT NULL,
    project   TEXT NOT NULL,    -- e.g. en.wikipedia
    day       TEXT NOT NULL,    -- YYYY-MM-DD (UTC)
    views     INTEGER NOT NULL, -- user (not bot / spider) views, all access methods
    PRIMARY KEY (tmdb_kind, tmdb_id, project, day)
) WITHOUT ROWID;

INSERT INTO task_schedules (task_type, name, enabled, max_retries, request_delay_seconds)
VALUES ('wikipedia.pageviews.import', 'Wikipedia pageviews', 1, 2, 0)
ON CONFLICT(name) DO NOTHING;
INSERT INTO task_schedule_run_times (schedule_id, run_time_utc)
SELECT id, '10:00' FROM task_schedules WHERE name = 'Wikipedia pageviews'
ON CONFLICT(schedule_id, run_time_utc) DO NOTHING;

-- +goose Down
DELETE FROM task_schedules WHERE task_type = 'wikipedia.pageviews.import';
DROP TABLE IF EXISTS wikipedia_pageviews;
ALTER TABLE tmdb_titles DROP COLUMN enwiki_fetched_at;
ALTER TABLE tmdb_titles DROP COLUMN enwiki_title;
