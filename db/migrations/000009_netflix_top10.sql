-- +goose Up
-- Netflix's official weekly Top 10 (https://www.netflix.com/tudum/top10),
-- imported from its TSV downloads. Weeks run Monday–Sunday; week is the
-- Sunday the week ends on (YYYY-MM-DD).

-- One row per Netflix show / film name, matched to TMDB like FlixPatrol
-- titles are (match_status as in titles).
CREATE TABLE netflix_titles (
    id             INTEGER PRIMARY KEY,
    show_title     TEXT NOT NULL,
    kind           TEXT NOT NULL CHECK(kind IN ('movie', 'tv_show')),
    tmdb_id        TEXT,
    imdb_id        TEXT,
    title_id       INTEGER REFERENCES titles(id) ON DELETE SET NULL, -- FlixPatrol title it matched through, if any
    match_status   TEXT NOT NULL DEFAULT 'pending',
    match_source   TEXT,
    match_attempts INTEGER NOT NULL DEFAULT 0,
    next_match_at  DATETIME,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(show_title, kind)
);
CREATE INDEX idx_netflix_titles_tmdb ON netflix_titles(kind, tmdb_id);

CREATE TABLE netflix_top10_global (
    week                TEXT NOT NULL,
    category            TEXT NOT NULL,     -- Films (English) | Films (Non-English) | TV (English) | TV (Non-English)
    weekly_rank         INTEGER NOT NULL,
    netflix_title_id    INTEGER NOT NULL REFERENCES netflix_titles(id) ON DELETE CASCADE,
    show_title          TEXT NOT NULL,
    season_title        TEXT,
    weekly_hours_viewed INTEGER,
    runtime_hours       REAL,
    weekly_views        INTEGER,
    cumulative_weeks    INTEGER,
    PRIMARY KEY (week, category, weekly_rank)
);
CREATE INDEX idx_netflix_global_title ON netflix_top10_global(netflix_title_id, week);

CREATE TABLE netflix_top10_countries (
    country          TEXT NOT NULL,        -- ISO 3166-1 alpha-2
    week             TEXT NOT NULL,
    category         TEXT NOT NULL,        -- Films | TV
    weekly_rank      INTEGER NOT NULL,
    netflix_title_id INTEGER NOT NULL REFERENCES netflix_titles(id) ON DELETE CASCADE,
    show_title       TEXT NOT NULL,
    season_title     TEXT,
    cumulative_weeks INTEGER,
    PRIMARY KEY (country, week, category, weekly_rank)
);
CREATE INDEX idx_netflix_countries_title ON netflix_top10_countries(netflix_title_id, week);

-- Netflix's all-time most popular list (views in the first 91 days).
CREATE TABLE netflix_most_popular (
    category          TEXT NOT NULL,
    rank              INTEGER NOT NULL,
    netflix_title_id  INTEGER NOT NULL REFERENCES netflix_titles(id) ON DELETE CASCADE,
    show_title        TEXT NOT NULL,
    season_title      TEXT,
    hours_viewed_91d  INTEGER,
    runtime_hours     REAL,
    views_91d         INTEGER,
    PRIMARY KEY (category, rank)
);

-- Change tracking for file imports (Netflix TSVs, IMDb datasets): an import
-- is skipped when the file's Last-Modified / ETag has not changed.
CREATE TABLE data_imports (
    source        TEXT PRIMARY KEY,        -- e.g. netflix.global, imdb.ratings
    etag          TEXT,
    last_modified TEXT,
    rows          INTEGER NOT NULL DEFAULT 0,
    imported_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS data_imports;
DROP TABLE IF EXISTS netflix_most_popular;
DROP INDEX IF EXISTS idx_netflix_countries_title;
DROP TABLE IF EXISTS netflix_top10_countries;
DROP INDEX IF EXISTS idx_netflix_global_title;
DROP TABLE IF EXISTS netflix_top10_global;
DROP INDEX IF EXISTS idx_netflix_titles_tmdb;
DROP TABLE IF EXISTS netflix_titles;
