-- +goose Up
-- 1. A Netflix title's own Netflix ID (from Tudum's Top 10 search) also
--    becomes its TMDB title's when that has none: source 'netflix', below
--    every other source (manual > tmdb > wikidata > match > netflix), so any
--    of them replaces it. Kept in step by the mapping refresh and the
--    Netflix ID lookup (SyncTmdbNetflixIDsFromNetflix).
UPDATE tmdb_titles SET
    netflix_id = (
        SELECT n.netflix_id FROM netflix_titles n
        WHERE n.tmdb_kind = tmdb_titles.tmdb_kind AND n.tmdb_id = tmdb_titles.tmdb_id AND COALESCE(n.netflix_id, '') <> ''
        ORDER BY n.match_source = 'netflix_id' DESC, n.match_status = 'manual' DESC,
                 CASE n.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END = n.tmdb_kind DESC, n.id
        LIMIT 1),
    netflix_source = 'netflix',
    updated_at = CURRENT_TIMESTAMP
WHERE netflix_id IS NULL AND EXISTS (
    SELECT 1 FROM netflix_titles n
    WHERE n.tmdb_kind = tmdb_titles.tmdb_kind AND n.tmdb_id = tmdb_titles.tmdb_id AND COALESCE(n.netflix_id, '') <> '');

-- 2. The mapping reference: one row per chart title (a FlixPatrol title,
--    paired with the Netflix title linked to it when both map to the same
--    TMDB title, or a Netflix title on its own) with its TMDB title and every
--    external ID. Unmapped titles are included with NULL TMDB columns.
--    Derived, never written: the mapping stays on titles / netflix_titles
--    (written through the MappingWriter) and the IDs on tmdb_titles. A
--    column used here can only be dropped after the view is recreated.
CREATE VIEW mapping_refs AS
WITH fp AS (
    SELECT t.id, t.slug, t.name, COALESCE(t.fp_kind, t.kind) AS flixpatrol_kind, t.match_status,
           NULLIF(t.tmdb_id, '') AS tmdb_id,
           CASE WHEN COALESCE(t.tmdb_id, '') = '' THEN NULL WHEN t.kind = 'tv_show' THEN 'tv' ELSE 'movie' END AS tmdb_kind
    FROM titles t
),
nf AS (
    SELECT n.id, n.show_title, n.kind AS netflix_kind, n.netflix_id, n.match_status,
           NULLIF(n.tmdb_id, '') AS tmdb_id,
           CASE WHEN COALESCE(n.tmdb_id, '') = '' THEN NULL ELSE n.tmdb_kind END AS tmdb_kind
    FROM netflix_titles n
),
pairs AS (
    SELECT fp.id AS title_id, nf.id AS netflix_title_id
    FROM fp
    JOIN title_links l ON l.title_id = fp.id
    JOIN nf ON nf.id = l.netflix_title_id AND nf.tmdb_kind IS fp.tmdb_kind AND nf.tmdb_id IS fp.tmdb_id
),
rows AS (
    SELECT fp.id AS title_id, p.netflix_title_id, fp.tmdb_kind, fp.tmdb_id
    FROM fp LEFT JOIN pairs p ON p.title_id = fp.id
    UNION ALL
    SELECT NULL, nf.id, nf.tmdb_kind, nf.tmdb_id
    FROM nf WHERE nf.id NOT IN (SELECT netflix_title_id FROM pairs)
)
SELECT
    r.tmdb_kind,
    r.tmdb_id,
    tt.title          AS tmdb_title,
    tt.imdb_id,
    tt.wikidata_id,
    tt.rt_id          AS rt_slug,
    tt.metacritic_id  AS metacritic_slug,
    tt.letterboxd_id  AS letterboxd_slug,
    tt.justwatch_id,
    COALESCE(tt.netflix_id, nf.netflix_id) AS netflix_id,
    fp.id             AS flixpatrol_title_id,
    fp.slug           AS flixpatrol_slug,
    fp.name           AS flixpatrol_name,
    fp.flixpatrol_kind,
    fp.match_status   AS flixpatrol_match_status,
    nf.id             AS netflix_title_id,
    nf.show_title     AS netflix_title,
    nf.netflix_kind,
    nf.netflix_id     AS netflix_title_netflix_id,
    nf.match_status   AS netflix_match_status
FROM rows r
LEFT JOIN fp ON fp.id = r.title_id
LEFT JOIN nf ON nf.id = r.netflix_title_id
LEFT JOIN tmdb_titles tt ON tt.tmdb_kind = r.tmdb_kind AND tt.tmdb_id = r.tmdb_id;

-- +goose Down
DROP VIEW IF EXISTS mapping_refs;
UPDATE tmdb_titles SET netflix_id = NULL, netflix_source = NULL WHERE netflix_source = 'netflix';
