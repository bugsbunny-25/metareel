-- +goose Up
-- Store chart dates as plain ISO dates (YYYY-MM-DD) so SQLite's date
-- functions work on them. They were written as Go's time.Time text form
-- ("2026-04-28 00:00:00 +0000 UTC"), which sorts the same way but is opaque
-- to date() / julianday().
UPDATE rankings SET ranked_on = substr(ranked_on, 1, 10) WHERE length(ranked_on) > 10;

-- One row per stored chart (date × country × provider × category): tells
-- "scraped, unchanged" apart from "never scraped", gives the API a
-- scraped_at, and lets a scrape skip a chart that is already stored.
-- signature is the chart's content ("1:slug|2:slug|..."), used to spot a
-- chart FlixPatrol has not updated yet (same content as the previous date).
CREATE TABLE chart_snapshots (
    id                 INTEGER PRIMARY KEY,
    ranked_on          TEXT NOT NULL,              -- YYYY-MM-DD
    country            TEXT NOT NULL,              -- ISO 3166-1 alpha-2
    streaming_provider TEXT NOT NULL,
    category           TEXT NOT NULL CHECK(category IN ('movies', 'tv_shows', 'overall')),
    entry_count        INTEGER NOT NULL,
    signature          TEXT NOT NULL,
    source             TEXT NOT NULL DEFAULT 'flixpatrol',
    run_id             INTEGER REFERENCES task_runs(id) ON DELETE SET NULL,
    first_scraped_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    scraped_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    changed_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(ranked_on, country, streaming_provider, category)
);
CREATE INDEX idx_chart_snapshots_scraped_at ON chart_snapshots(scraped_at);
CREATE INDEX idx_chart_snapshots_chart ON chart_snapshots(country, streaming_provider, category, ranked_on);

INSERT INTO chart_snapshots (ranked_on, country, streaming_provider, category, entry_count, signature, first_scraped_at, scraped_at, changed_at)
SELECT ranked_on, country, streaming_provider, category, COUNT(*),
       group_concat(rank || ':' || slug, '|'),
       MIN(scraped_at), MAX(scraped_at), MAX(scraped_at)
FROM (
    SELECT r.ranked_on, r.country, r.streaming_provider, r.category, r.rank, t.slug, r.scraped_at
    FROM rankings r
    JOIN titles t ON t.id = r.title_id
    ORDER BY r.ranked_on, r.country, r.streaming_provider, r.category, r.rank
)
GROUP BY ranked_on, country, streaming_provider, category;

-- +goose Down
DROP INDEX IF EXISTS idx_chart_snapshots_chart;
DROP INDEX IF EXISTS idx_chart_snapshots_scraped_at;
DROP TABLE IF EXISTS chart_snapshots;
-- Dates stay in ISO form: the old code reads both.
