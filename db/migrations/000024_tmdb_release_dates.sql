-- +goose Up
-- TMDB release dates of movies per country and release type (earliest date
-- of each type), from /movie/{id}?append_to_response=release_dates on every
-- titles.metadata refresh. Types: 1 premiere, 2 theatrical (limited),
-- 3 theatrical, 4 digital (PVOD / EST), 5 physical, 6 TV. Feeds
-- /titles/tmdb/movie/{id}/releases and /analytics/release-windows.
-- tmdb_titles.digital_release_dates (type 4 only) stays for the API field
-- of the same name.
CREATE TABLE tmdb_release_dates (
    tmdb_kind    TEXT NOT NULL,              -- always 'movie' (TMDB has no release dates for TV)
    tmdb_id      TEXT NOT NULL,
    country      TEXT NOT NULL,              -- ISO 3166-1 alpha-2
    release_type INTEGER NOT NULL,           -- 1-6 (validated in Go)
    release_date TEXT NOT NULL,              -- YYYY-MM-DD
    note         TEXT,                       -- TMDB's note (e.g. a festival)
    PRIMARY KEY (tmdb_kind, tmdb_id, country, release_type),
    FOREIGN KEY (tmdb_kind, tmdb_id) REFERENCES tmdb_titles(tmdb_kind, tmdb_id) ON DELETE CASCADE
) WITHOUT ROWID;
CREATE INDEX idx_tmdb_release_dates_country_type ON tmdb_release_dates(country, release_type, release_date);

-- Digital dates already stored; the other types arrive with each title's
-- next metadata refresh.
INSERT OR IGNORE INTO tmdb_release_dates (tmdb_kind, tmdb_id, country, release_type, release_date)
SELECT m.tmdb_kind, m.tmdb_id, j.key, 4, substr(j.value, 1, 10)
FROM tmdb_titles m,
     json_each(CASE WHEN json_valid(m.digital_release_dates) AND json_type(m.digital_release_dates) = 'object'
                    THEN m.digital_release_dates ELSE '{}' END) j
WHERE m.tmdb_kind = 'movie' AND m.digital_release_dates IS NOT NULL
  AND j.type = 'text' AND j.key GLOB '[A-Z][A-Z]'
  AND j.value GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]*';

-- +goose Down
DROP INDEX IF EXISTS idx_tmdb_release_dates_country_type;
DROP TABLE IF EXISTS tmdb_release_dates;
