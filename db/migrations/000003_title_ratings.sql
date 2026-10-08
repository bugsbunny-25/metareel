-- +goose Up
-- On-demand ratings cache per TMDB title (see RATINGS_TTL). Keyed by TMDB
-- namespace + ID rather than titles.id so any TMDB title can be rated, and
-- because several FlixPatrol titles can share one TMDB ID.
--
-- title_ratings holds the IDs used to look the title up again;
-- title_rating_sources holds every rating each provider returned.
CREATE TABLE title_ratings (
    id            INTEGER PRIMARY KEY,
    tmdb_kind     TEXT NOT NULL CHECK(tmdb_kind IN ('movie', 'tv')),
    tmdb_id       TEXT NOT NULL,
    title         TEXT,
    year          INTEGER,
    imdb_id       TEXT,                -- e.g. "tt1234567"
    justwatch_id  TEXT,                -- e.g. "tm1504418"
    rt_slug       TEXT,                -- e.g. "m/bugonia", "tv/beef"
    refreshed_at  DATETIME NOT NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tmdb_kind, tmdb_id)
);

CREATE TABLE title_rating_sources (
    id          INTEGER PRIMARY KEY,
    tmdb_kind   TEXT NOT NULL,
    tmdb_id     TEXT NOT NULL,
    provider    TEXT NOT NULL CHECK(provider IN ('justwatch', 'mdblist', 'rottentomatoes')),
    source      TEXT NOT NULL,        -- e.g. imdb, tomatoes, popcorn, metacritic, letterboxd
    value       REAL NOT NULL,        -- on the provider's own scale for that source
    votes       INTEGER,
    url         TEXT,
    fetched_at  DATETIME NOT NULL,
    UNIQUE(tmdb_kind, tmdb_id, provider, source),
    FOREIGN KEY(tmdb_kind, tmdb_id) REFERENCES title_ratings(tmdb_kind, tmdb_id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE IF EXISTS title_rating_sources;
DROP TABLE IF EXISTS title_ratings;
