-- +goose Up
-- Matching state per FlixPatrol title, so scrapes only store charts and a
-- separate enrichment job matches titles to TMDB / IMDb / Rotten Tomatoes
-- with backoff instead of retrying every title on every scrape.
--
-- match_status: pending (never tried) | matched (has a TMDB ID) |
-- unmatched (tried, no confident match) | manual (set in the admin UI;
-- automation never changes it).
-- (validated in Go: a CHECK would stop the Down migration dropping it)
ALTER TABLE titles ADD COLUMN match_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE titles ADD COLUMN match_source TEXT;        -- justwatch_path | justwatch_search | tmdb_search | manual | legacy
ALTER TABLE titles ADD COLUMN matched_name TEXT;        -- the title the match was made against
ALTER TABLE titles ADD COLUMN matched_year INTEGER;
ALTER TABLE titles ADD COLUMN match_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE titles ADD COLUMN match_attempted_at DATETIME;
ALTER TABLE titles ADD COLUMN next_match_at DATETIME;   -- NULL = due now
ALTER TABLE titles ADD COLUMN rt_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE titles ADD COLUMN next_rt_at DATETIME;      -- NULL = due now
ALTER TABLE titles ADD COLUMN justwatch_id TEXT;        -- e.g. tm1504418
ALTER TABLE titles ADD COLUMN wikidata_id TEXT;         -- e.g. Q123
-- From the FlixPatrol title page (fetched once).
ALTER TABLE titles ADD COLUMN fp_name TEXT;
ALTER TABLE titles ADD COLUMN fp_kind TEXT;
ALTER TABLE titles ADD COLUMN fp_premiere_date TEXT;    -- YYYY-MM-DD
ALTER TABLE titles ADD COLUMN fp_country TEXT;
ALTER TABLE titles ADD COLUMN fp_fetched_at DATETIME;

UPDATE titles SET match_status = 'matched', match_source = 'legacy'
WHERE COALESCE(tmdb_id, '') <> '';
UPDATE titles SET match_status = 'unmatched', match_attempts = 1
WHERE COALESCE(tmdb_id, '') = '';

CREATE INDEX idx_titles_match_due ON titles(match_status, next_match_at);
CREATE INDEX idx_titles_imdb_id ON titles(imdb_id);

-- +goose Down
DROP INDEX IF EXISTS idx_titles_imdb_id;
DROP INDEX IF EXISTS idx_titles_match_due;
ALTER TABLE titles DROP COLUMN fp_fetched_at;
ALTER TABLE titles DROP COLUMN fp_country;
ALTER TABLE titles DROP COLUMN fp_premiere_date;
ALTER TABLE titles DROP COLUMN fp_kind;
ALTER TABLE titles DROP COLUMN fp_name;
ALTER TABLE titles DROP COLUMN wikidata_id;
ALTER TABLE titles DROP COLUMN justwatch_id;
ALTER TABLE titles DROP COLUMN next_rt_at;
ALTER TABLE titles DROP COLUMN rt_attempts;
ALTER TABLE titles DROP COLUMN next_match_at;
ALTER TABLE titles DROP COLUMN match_attempted_at;
ALTER TABLE titles DROP COLUMN match_attempts;
ALTER TABLE titles DROP COLUMN matched_year;
ALTER TABLE titles DROP COLUMN matched_name;
ALTER TABLE titles DROP COLUMN match_source;
ALTER TABLE titles DROP COLUMN match_status;
