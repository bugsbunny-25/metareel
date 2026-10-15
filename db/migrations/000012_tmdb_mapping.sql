-- +goose Up
-- One TMDB-keyed mapping across sources: TMDB titles are the hub, FlixPatrol
-- titles and Netflix titles both point at them, and a Netflix title and a
-- FlixPatrol title that are the same thing are linked so a mapping found or
-- fixed on either side is shared with the other.

-- netflix_titles.kind stays Netflix's list (Films | TV, part of the unique
-- key); tmdb_kind is the TMDB namespace of tmdb_id (movie | tv), which can
-- differ (a stand-up special in Netflix's TV list is a TMDB movie). NULL
-- while unmapped.
ALTER TABLE netflix_titles ADD COLUMN tmdb_kind TEXT;
UPDATE netflix_titles SET tmdb_kind = CASE kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END
WHERE COALESCE(tmdb_id, '') <> '';
DROP INDEX IF EXISTS idx_netflix_titles_tmdb;
CREATE INDEX idx_netflix_titles_tmdb_ref ON netflix_titles(tmdb_kind, tmdb_id);

-- A Netflix title and a FlixPatrol title that are the same title: same
-- normalized name, same Netflix list (Films / TV) and on Netflix's charts in
-- the same or an adjacent week. Recomputed by the mapping sync.
CREATE TABLE title_links (
    netflix_title_id INTEGER NOT NULL REFERENCES netflix_titles(id) ON DELETE CASCADE,
    title_id         INTEGER NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (netflix_title_id, title_id)
);
CREATE INDEX idx_title_links_title ON title_links(title_id);

INSERT INTO title_links (netflix_title_id, title_id)
SELECT id, title_id FROM netflix_titles WHERE title_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- Every external ID of a TMDB title in one place: tmdb_titles already has
-- IMDb, Wikidata, Rotten Tomatoes (rt_id), Metacritic and Letterboxd; add
-- Netflix's title ID (Wikidata P1874, netflix.com/title/<id>) and JustWatch's
-- (from matching, e.g. tm123 / ts123). FlixPatrol slugs and Netflix Top 10
-- names are the titles / netflix_titles rows mapped to it.
ALTER TABLE tmdb_titles ADD COLUMN netflix_id TEXT;
ALTER TABLE tmdb_titles ADD COLUMN justwatch_id TEXT;
CREATE INDEX idx_tmdb_titles_netflix_id ON tmdb_titles(netflix_id);
UPDATE tmdb_titles SET justwatch_id = (
    SELECT t.justwatch_id FROM titles t
    WHERE t.tmdb_id = tmdb_titles.tmdb_id AND COALESCE(t.justwatch_id, '') <> ''
      AND CASE t.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END = tmdb_titles.tmdb_kind
    ORDER BY t.id LIMIT 1);
-- Look every title up on Wikidata again for its Netflix ID.
UPDATE tmdb_titles SET wikidata_fetched_at = NULL;

-- Netflix's own IDs of Netflix Top 10 titles (the show / film) and of TV
-- seasons, from Tudum's Top 10 search; checked_at backs off the lookup.
ALTER TABLE netflix_titles ADD COLUMN netflix_id TEXT;
ALTER TABLE netflix_titles ADD COLUMN netflix_id_checked_at DATETIME;
ALTER TABLE netflix_seasons ADD COLUMN netflix_id TEXT;
CREATE INDEX idx_netflix_titles_netflix_id ON netflix_titles(netflix_id);

-- +goose Down
DROP INDEX IF EXISTS idx_netflix_titles_netflix_id;
ALTER TABLE netflix_seasons DROP COLUMN netflix_id;
ALTER TABLE netflix_titles DROP COLUMN netflix_id_checked_at;
ALTER TABLE netflix_titles DROP COLUMN netflix_id;
DROP INDEX IF EXISTS idx_tmdb_titles_netflix_id;
ALTER TABLE tmdb_titles DROP COLUMN justwatch_id;
ALTER TABLE tmdb_titles DROP COLUMN netflix_id;
DROP INDEX IF EXISTS idx_title_links_title;
DROP TABLE IF EXISTS title_links;
DROP INDEX IF EXISTS idx_netflix_titles_tmdb_ref;
CREATE INDEX idx_netflix_titles_tmdb ON netflix_titles(kind, tmdb_id);
ALTER TABLE netflix_titles DROP COLUMN tmdb_kind;
