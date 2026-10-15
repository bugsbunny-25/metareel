-- +goose Up
-- 1. tmdb_titles is the only home of external IDs. Each ID records where it
--    came from; a write only replaces an ID from the same or a lower-ranked
--    source: manual > tmdb > wikidata > match.
ALTER TABLE tmdb_titles ADD COLUMN imdb_source TEXT;
ALTER TABLE tmdb_titles ADD COLUMN wikidata_source TEXT;
ALTER TABLE tmdb_titles ADD COLUMN rt_source TEXT;
ALTER TABLE tmdb_titles ADD COLUMN metacritic_source TEXT;
ALTER TABLE tmdb_titles ADD COLUMN letterboxd_source TEXT;
ALTER TABLE tmdb_titles ADD COLUMN netflix_source TEXT;
ALTER TABLE tmdb_titles ADD COLUMN justwatch_source TEXT;
-- Rotten Tomatoes search backoff, per TMDB title (was per FlixPatrol title).
ALTER TABLE tmdb_titles ADD COLUMN rt_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tmdb_titles ADD COLUMN next_rt_at DATETIME;

-- Every TMDB title anything is mapped to has a row.
INSERT INTO tmdb_titles (tmdb_kind, tmdb_id)
SELECT DISTINCT CASE kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END, tmdb_id FROM titles WHERE COALESCE(tmdb_id, '') <> ''
ON CONFLICT DO NOTHING;
INSERT INTO tmdb_titles (tmdb_kind, tmdb_id)
SELECT DISTINCT tmdb_kind, tmdb_id FROM netflix_titles WHERE COALESCE(tmdb_id, '') <> '' AND tmdb_kind IN ('movie', 'tv')
ON CONFLICT DO NOTHING;

-- Sources of the IDs already there.
UPDATE tmdb_titles SET
    imdb_source = CASE WHEN imdb_id IS NULL THEN NULL WHEN imdb_manual = 1 THEN 'manual'
                       WHEN details_fetched_at IS NOT NULL THEN 'tmdb' ELSE 'match' END,
    wikidata_source = CASE WHEN wikidata_id IS NULL THEN NULL WHEN wikidata_manual = 1 THEN 'manual'
                           WHEN details_fetched_at IS NOT NULL THEN 'tmdb' ELSE 'wikidata' END,
    rt_source = CASE WHEN rt_id IS NULL THEN NULL WHEN wikidata_fetched_at IS NOT NULL THEN 'wikidata' ELSE 'match' END,
    metacritic_source = CASE WHEN metacritic_id IS NULL THEN NULL ELSE 'wikidata' END,
    letterboxd_source = CASE WHEN letterboxd_id IS NULL THEN NULL ELSE 'wikidata' END,
    netflix_source = CASE WHEN netflix_id IS NULL THEN NULL WHEN wikidata_fetched_at IS NOT NULL THEN 'wikidata' ELSE 'match' END,
    justwatch_source = CASE WHEN justwatch_id IS NULL THEN NULL ELSE 'match' END;

-- Gaps filled from the titles mapped to them (titles fixed by hand first).
UPDATE tmdb_titles SET
    imdb_id = COALESCE(imdb_id, (
        SELECT t.imdb_id FROM titles t
        WHERE t.tmdb_id = tmdb_titles.tmdb_id AND CASE t.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END = tmdb_titles.tmdb_kind
          AND COALESCE(t.imdb_id, '') <> '' ORDER BY t.match_status = 'manual' DESC, t.id LIMIT 1), (
        SELECT n.imdb_id FROM netflix_titles n
        WHERE n.tmdb_kind = tmdb_titles.tmdb_kind AND n.tmdb_id = tmdb_titles.tmdb_id AND COALESCE(n.imdb_id, '') <> ''
        ORDER BY n.match_status = 'manual' DESC, n.id LIMIT 1)),
    rt_id = COALESCE(rt_id, (
        SELECT t.rt_url FROM titles t
        WHERE t.tmdb_id = tmdb_titles.tmdb_id AND CASE t.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END = tmdb_titles.tmdb_kind
          AND COALESCE(t.rt_url, '') <> '' ORDER BY t.match_status = 'manual' DESC, t.id LIMIT 1)),
    justwatch_id = COALESCE(justwatch_id, (
        SELECT t.justwatch_id FROM titles t
        WHERE t.tmdb_id = tmdb_titles.tmdb_id AND CASE t.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END = tmdb_titles.tmdb_kind
          AND COALESCE(t.justwatch_id, '') <> '' ORDER BY t.id LIMIT 1)),
    wikidata_id = COALESCE(wikidata_id, (
        SELECT t.wikidata_id FROM titles t
        WHERE t.tmdb_id = tmdb_titles.tmdb_id AND CASE t.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END = tmdb_titles.tmdb_kind
          AND COALESCE(t.wikidata_id, '') <> '' ORDER BY t.id LIMIT 1));
UPDATE tmdb_titles SET
    imdb_source = COALESCE(imdb_source, CASE WHEN imdb_id IS NOT NULL THEN 'match' END),
    rt_source = COALESCE(rt_source, CASE WHEN rt_id IS NOT NULL THEN 'match' END),
    justwatch_source = COALESCE(justwatch_source, CASE WHEN justwatch_id IS NOT NULL THEN 'match' END),
    wikidata_source = COALESCE(wikidata_source, CASE WHEN wikidata_id IS NOT NULL THEN 'match' END);

ALTER TABLE tmdb_titles DROP COLUMN imdb_manual;
ALTER TABLE tmdb_titles DROP COLUMN wikidata_manual;

-- 2. What each source says a title is (its own evidence). A title's mapping
--    (titles.kind + tmdb_id, netflix_titles.tmdb_kind + tmdb_id) is computed
--    from its evidence and that of the titles linked to it: a title fixed by
--    hand wins, then a Netflix ID, then FlixPatrol's match, then Netflix's.
--    Mappings taken from a linked title have no evidence of their own, so
--    they go away by themselves when the source's evidence does.
CREATE TABLE mapping_evidence (
    subject_type TEXT NOT NULL,            -- flixpatrol | netflix
    subject_id   INTEGER NOT NULL,         -- titles.id | netflix_titles.id
    kind         TEXT NOT NULL,            -- manual | netflix_id | match
    tmdb_kind    TEXT,                     -- movie | tv; NULL for a title fixed by hand as unmapped
    tmdb_id      TEXT,
    source       TEXT,                     -- how it was found, e.g. justwatch_path, wikidata_netflix, manual
    imdb_id      TEXT,                     -- the IMDb ID it came with (checked against the TMDB title's)
    via_title_id INTEGER,                  -- a Netflix match through a same-name FlixPatrol title
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (subject_type, subject_id, kind)
);
CREATE INDEX idx_mapping_evidence_ref ON mapping_evidence(tmdb_kind, tmdb_id);
CREATE INDEX idx_mapping_evidence_via ON mapping_evidence(via_title_id);

INSERT INTO mapping_evidence (subject_type, subject_id, kind, tmdb_kind, tmdb_id, source, imdb_id)
SELECT 'flixpatrol', id, 'manual',
       CASE WHEN COALESCE(tmdb_id, '') = '' THEN NULL WHEN kind = 'tv_show' THEN 'tv' ELSE 'movie' END,
       NULLIF(tmdb_id, ''), 'manual', NULLIF(imdb_id, '')
FROM titles WHERE match_status = 'manual';
INSERT INTO mapping_evidence (subject_type, subject_id, kind, tmdb_kind, tmdb_id, source, imdb_id)
SELECT 'flixpatrol', id, 'match', CASE kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END, tmdb_id, match_source, NULLIF(imdb_id, '')
FROM titles
-- Any status: a failed rematch leaves a title 'unmatched' with its mapping.
-- Mappings taken over a link that still exists are not the title's own.
WHERE match_status <> 'manual' AND COALESCE(tmdb_id, '') <> ''
  AND NOT (COALESCE(match_source, '') = 'netflix_title'
           AND EXISTS (SELECT 1 FROM title_links l WHERE l.title_id = titles.id));

INSERT INTO mapping_evidence (subject_type, subject_id, kind, tmdb_kind, tmdb_id, source, imdb_id)
SELECT 'netflix', id, 'manual', CASE WHEN COALESCE(tmdb_id, '') = '' THEN NULL ELSE tmdb_kind END,
       NULLIF(tmdb_id, ''), 'manual', NULLIF(imdb_id, '')
FROM netflix_titles WHERE match_status = 'manual';
INSERT INTO mapping_evidence (subject_type, subject_id, kind, tmdb_kind, tmdb_id, source, imdb_id)
SELECT 'netflix', id, 'netflix_id', tmdb_kind, tmdb_id, 'netflix_id', NULLIF(imdb_id, '')
FROM netflix_titles
WHERE match_status <> 'manual' AND COALESCE(tmdb_id, '') <> '' AND match_source = 'netflix_id';
-- Other Netflix matches, except mappings taken over a link.
INSERT INTO mapping_evidence (subject_type, subject_id, kind, tmdb_kind, tmdb_id, source, imdb_id, via_title_id)
SELECT 'netflix', n.id, 'match', n.tmdb_kind, n.tmdb_id, n.match_source, NULLIF(n.imdb_id, ''),
       CASE WHEN n.match_source = 'flixpatrol_title' THEN n.title_id END
FROM netflix_titles n
WHERE n.match_status <> 'manual' AND COALESCE(n.tmdb_id, '') <> '' AND COALESCE(n.match_source, '') NOT IN ('netflix_id', 'netflix_title')
  AND NOT (n.match_source = 'flixpatrol_title'
           AND EXISTS (SELECT 1 FROM title_links l WHERE l.netflix_title_id = n.id AND l.title_id = n.title_id));

-- 3. The IDs the titles kept themselves (now on tmdb_titles or the evidence)
--    are kept one release for rollback under legacy_ names; nothing reads or
--    writes them. The RT backoff moved to tmdb_titles.
ALTER TABLE titles RENAME COLUMN imdb_id TO legacy_imdb_id;
ALTER TABLE titles RENAME COLUMN rt_url TO legacy_rt_url;
ALTER TABLE titles RENAME COLUMN justwatch_id TO legacy_justwatch_id;
ALTER TABLE titles RENAME COLUMN wikidata_id TO legacy_wikidata_id;
ALTER TABLE titles RENAME COLUMN rt_attempts TO legacy_rt_attempts;
ALTER TABLE titles RENAME COLUMN next_rt_at TO legacy_next_rt_at;
ALTER TABLE netflix_titles RENAME COLUMN imdb_id TO legacy_imdb_id;
CREATE INDEX idx_tmdb_titles_rt_due ON tmdb_titles(rt_id, next_rt_at);

-- +goose Down
DROP INDEX IF EXISTS idx_tmdb_titles_rt_due;
ALTER TABLE netflix_titles RENAME COLUMN legacy_imdb_id TO imdb_id;
ALTER TABLE titles RENAME COLUMN legacy_next_rt_at TO next_rt_at;
ALTER TABLE titles RENAME COLUMN legacy_rt_attempts TO rt_attempts;
ALTER TABLE titles RENAME COLUMN legacy_wikidata_id TO wikidata_id;
ALTER TABLE titles RENAME COLUMN legacy_justwatch_id TO justwatch_id;
ALTER TABLE titles RENAME COLUMN legacy_rt_url TO rt_url;
ALTER TABLE titles RENAME COLUMN legacy_imdb_id TO imdb_id;
DROP INDEX IF EXISTS idx_mapping_evidence_via;
DROP INDEX IF EXISTS idx_mapping_evidence_ref;
DROP TABLE IF EXISTS mapping_evidence;
ALTER TABLE tmdb_titles ADD COLUMN imdb_manual INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tmdb_titles ADD COLUMN wikidata_manual INTEGER NOT NULL DEFAULT 0;
UPDATE tmdb_titles SET imdb_manual = CASE WHEN imdb_source = 'manual' THEN 1 ELSE 0 END,
                       wikidata_manual = CASE WHEN wikidata_source = 'manual' THEN 1 ELSE 0 END;
ALTER TABLE tmdb_titles DROP COLUMN next_rt_at;
ALTER TABLE tmdb_titles DROP COLUMN rt_attempts;
ALTER TABLE tmdb_titles DROP COLUMN justwatch_source;
ALTER TABLE tmdb_titles DROP COLUMN netflix_source;
ALTER TABLE tmdb_titles DROP COLUMN letterboxd_source;
ALTER TABLE tmdb_titles DROP COLUMN metacritic_source;
ALTER TABLE tmdb_titles DROP COLUMN rt_source;
ALTER TABLE tmdb_titles DROP COLUMN wikidata_source;
ALTER TABLE tmdb_titles DROP COLUMN imdb_source;
