-- +goose Up
-- History of values metareel otherwise keeps only the latest of. Dates are
-- ISO YYYY-MM-DD text (UTC). Rating series are sparse: a row is written only
-- when the value changed since the title's previous row, so a series is a
-- step function (the last point before a date holds until the next one).

-- IMDb rating and votes from the imdb.ratings.import job, for the IMDb IDs
-- metareel knows.
CREATE TABLE imdb_rating_history (
    imdb_id        TEXT NOT NULL,
    captured_on    TEXT NOT NULL,
    average_rating REAL NOT NULL,
    num_votes      INTEGER NOT NULL,
    PRIMARY KEY (imdb_id, captured_on)
) WITHOUT ROWID;

-- Rating values per provider and source (title_rating_sources) from ratings
-- refreshes.
CREATE TABLE title_rating_history (
    tmdb_kind   TEXT NOT NULL,
    tmdb_id     TEXT NOT NULL,
    provider    TEXT NOT NULL,
    source      TEXT NOT NULL,
    captured_on TEXT NOT NULL,
    value       REAL NOT NULL,
    votes       INTEGER,
    PRIMARY KEY (tmdb_kind, tmdb_id, provider, source, captured_on)
) WITHOUT ROWID;

-- TMDB popularity per day from TMDB's daily ID exports
-- (tmdb.popularity.import), for the TMDB titles metareel knows. Dense: one
-- row per title per export.
CREATE TABLE tmdb_popularity_history (
    tmdb_kind   TEXT NOT NULL,
    tmdb_id     TEXT NOT NULL,
    captured_on TEXT NOT NULL,
    popularity  REAL NOT NULL,
    PRIMARY KEY (tmdb_kind, tmdb_id, captured_on)
) WITHOUT ROWID;

-- Watch provider offers that appeared or disappeared between two TMDB
-- refreshes of a title (none on a title's first refresh: that is the
-- baseline). The change happened between previous_check_at and observed_at.
CREATE TABLE watch_provider_events (
    id                INTEGER PRIMARY KEY,
    tmdb_kind         TEXT NOT NULL,
    tmdb_id           TEXT NOT NULL,
    country           TEXT NOT NULL,
    monetization      TEXT NOT NULL,
    provider_id       INTEGER NOT NULL,
    provider_name     TEXT NOT NULL,
    event             TEXT NOT NULL,   -- added | removed (validated in Go)
    previous_check_at DATETIME,
    observed_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_watch_provider_events_title ON watch_provider_events(tmdb_kind, tmdb_id, observed_at);
CREATE INDEX idx_watch_provider_events_observed ON watch_provider_events(observed_at, country);

-- Daily popularity import, after TMDB publishes the exports (about 08:00 UTC).
INSERT INTO task_schedules (task_type, name, enabled, max_retries, request_delay_seconds)
VALUES ('tmdb.popularity.import', 'TMDB popularity export', 1, 2, 0)
ON CONFLICT(name) DO NOTHING;
INSERT INTO task_schedule_run_times (schedule_id, run_time_utc)
SELECT id, '09:30' FROM task_schedules WHERE name = 'TMDB popularity export'
ON CONFLICT(schedule_id, run_time_utc) DO NOTHING;

-- +goose Down
DELETE FROM task_schedules WHERE task_type = 'tmdb.popularity.import';
DROP INDEX IF EXISTS idx_watch_provider_events_observed;
DROP INDEX IF EXISTS idx_watch_provider_events_title;
DROP TABLE IF EXISTS watch_provider_events;
DROP TABLE IF EXISTS tmdb_popularity_history;
DROP TABLE IF EXISTS title_rating_history;
DROP TABLE IF EXISTS imdb_rating_history;
