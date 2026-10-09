-- +goose Up
-- TMDB metadata per TMDB title (several FlixPatrol slugs can share one), plus
-- IDs from Wikidata. Filled by the titles.metadata job and after matching.
CREATE TABLE tmdb_titles (
    tmdb_kind             TEXT NOT NULL CHECK(tmdb_kind IN ('movie', 'tv')),
    tmdb_id               TEXT NOT NULL,
    title                 TEXT,
    original_title        TEXT,
    original_language     TEXT,
    overview              TEXT,
    status                TEXT,            -- e.g. Released, Returning Series, Ended
    release_date          TEXT,            -- YYYY-MM-DD: movie release / TV first air date
    last_air_date         TEXT,            -- TV only
    runtime               INTEGER,         -- minutes (TV: typical episode)
    number_of_seasons     INTEGER,
    number_of_episodes    INTEGER,
    genres                TEXT,            -- JSON array of names
    origin_countries      TEXT,            -- JSON array of ISO 3166-1 codes
    production_companies  TEXT,            -- JSON array of names
    networks              TEXT,            -- JSON array of names (TV)
    poster_path           TEXT,
    backdrop_path         TEXT,
    popularity            REAL,
    vote_average          REAL,
    vote_count            INTEGER,
    digital_release_dates TEXT,            -- JSON {"US": "YYYY-MM-DD"} (movies, release type 4)
    imdb_id               TEXT,
    wikidata_id           TEXT,
    rt_id                 TEXT,            -- from Wikidata P1258, e.g. m/bugonia
    metacritic_id         TEXT,            -- P1712, e.g. movie/bugonia
    letterboxd_id         TEXT,            -- P6127 (movies)
    details_fetched_at    DATETIME,
    wikidata_fetched_at   DATETIME,
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tmdb_kind, tmdb_id)
);
CREATE INDEX idx_tmdb_titles_imdb_id ON tmdb_titles(imdb_id);

-- Where a title can be watched, per country, from TMDB (data by JustWatch).
CREATE TABLE tmdb_watch_providers (
    tmdb_kind        TEXT NOT NULL,
    tmdb_id          TEXT NOT NULL,
    country          TEXT NOT NULL,        -- ISO 3166-1 alpha-2
    monetization     TEXT NOT NULL CHECK(monetization IN ('flatrate', 'free', 'ads', 'rent', 'buy')),
    provider_id      INTEGER NOT NULL,
    provider_name    TEXT NOT NULL,
    logo_path        TEXT,
    display_priority INTEGER,
    link             TEXT,                 -- TMDB watch page for the country
    fetched_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tmdb_kind, tmdb_id, country, monetization, provider_id),
    FOREIGN KEY (tmdb_kind, tmdb_id) REFERENCES tmdb_titles(tmdb_kind, tmdb_id) ON DELETE CASCADE
);
CREATE INDEX idx_tmdb_watch_providers_country ON tmdb_watch_providers(country, provider_id);

-- +goose Down
DROP INDEX IF EXISTS idx_tmdb_watch_providers_country;
DROP TABLE IF EXISTS tmdb_watch_providers;
DROP INDEX IF EXISTS idx_tmdb_titles_imdb_id;
DROP TABLE IF EXISTS tmdb_titles;
