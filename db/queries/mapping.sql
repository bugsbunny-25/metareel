-- One TMDB-keyed mapping across FlixPatrol and Netflix titles: links between
-- the two sources, sharing mappings over links, and the admin mapping page.

-- name: ListNetflixTitlesForLinking :many
SELECT id, show_title, kind FROM netflix_titles;

-- name: ListNetflixTitleWeeksFrom :many
-- (title, week) pairs from a week on, global and per country.
SELECT netflix_title_id, week FROM netflix_top10_global WHERE week >= CAST(sqlc.arg(from_week) AS TEXT)
UNION
SELECT netflix_title_id, week FROM netflix_top10_countries WHERE week >= CAST(sqlc.arg(from_week) AS TEXT);

-- name: ListFlixPatrolNetflixWeeks :many
-- FlixPatrol titles on Netflix charts with the Netflix lists (movies |
-- tv_shows) and Netflix weeks (ending on Sunday) they charted in.
SELECT DISTINCT t.id, t.name, CAST(COALESCE(t.fp_name, '') AS TEXT) AS fp_name, r.category,
       CAST(date(r.ranked_on, 'weekday 0') AS TEXT) AS week
FROM rankings r
JOIN titles t ON t.id = r.title_id
WHERE r.streaming_provider = 'netflix' AND r.category IN ('movies', 'tv_shows');

-- name: ListTitleLinks :many
SELECT netflix_title_id, title_id FROM title_links;

-- name: InsertTitleLink :exec
INSERT INTO title_links (netflix_title_id, title_id) VALUES (?, ?)
ON CONFLICT(netflix_title_id, title_id) DO NOTHING;

-- name: DeleteTitleLink :exec
DELETE FROM title_links WHERE netflix_title_id = ? AND title_id = ?;

-- name: ListLinkedNetflixTitles :many
SELECT n.* FROM netflix_titles n WHERE n.id IN (SELECT netflix_title_id FROM title_links);

-- name: ListLinkedFlixPatrolTitles :many
SELECT t.* FROM titles t WHERE t.id IN (SELECT title_id FROM title_links);

-- name: ListTmdbMappings :many
-- TMDB titles that FlixPatrol or Netflix titles are mapped to, with how many
-- of each and flags. flag: conflict (linked titles mapped elsewhere) |
-- cross_kind (a Netflix title from the other list, or a FlixPatrol page kind
-- that differs) | both (titles from both sources) | netflix_only |
-- flixpatrol_only | manual (a title fixed by hand). Newest activity first.
SELECT m.tmdb_kind, m.tmdb_id, m.flixpatrol_titles, m.netflix_titles, m.conflicts, m.cross_kind, m.manual,
       CAST(m.last_seen AS TEXT) AS last_seen,
       tm.title, tm.release_date, tm.poster_path, tm.imdb_id
FROM (
    SELECT r.tmdb_kind, r.tmdb_id,
           (SELECT COUNT(*) FROM titles t
            WHERE t.kind = CASE r.tmdb_kind WHEN 'tv' THEN 'tv_show' ELSE 'movie' END AND t.tmdb_id = r.tmdb_id) AS flixpatrol_titles,
           (SELECT COUNT(*) FROM netflix_titles n WHERE n.tmdb_kind = r.tmdb_kind AND n.tmdb_id = r.tmdb_id) AS netflix_titles,
           -- Links from a title on this TMDB title to one mapped elsewhere,
           -- or to a title that is also linked to one mapped elsewhere.
           (SELECT COUNT(*) FROM title_links l
            JOIN titles t ON t.id = l.title_id
            JOIN netflix_titles n ON n.id = l.netflix_title_id
            WHERE t.kind = CASE r.tmdb_kind WHEN 'tv' THEN 'tv_show' ELSE 'movie' END AND t.tmdb_id = r.tmdb_id
              AND ((COALESCE(n.tmdb_id, '') <> '' AND NOT (n.tmdb_kind = r.tmdb_kind AND n.tmdb_id = r.tmdb_id))
                OR EXISTS (SELECT 1 FROM title_links l2 JOIN titles t2 ON t2.id = l2.title_id
                           WHERE l2.netflix_title_id = l.netflix_title_id AND COALESCE(t2.tmdb_id, '') <> ''
                             AND NOT (t2.kind = t.kind AND t2.tmdb_id = t.tmdb_id))))
         + (SELECT COUNT(*) FROM title_links l
            JOIN netflix_titles n ON n.id = l.netflix_title_id
            JOIN titles t ON t.id = l.title_id
            WHERE n.tmdb_kind = r.tmdb_kind AND n.tmdb_id = r.tmdb_id
              AND ((COALESCE(t.tmdb_id, '') <> '' AND NOT (t.kind = CASE r.tmdb_kind WHEN 'tv' THEN 'tv_show' ELSE 'movie' END AND t.tmdb_id = r.tmdb_id))
                OR EXISTS (SELECT 1 FROM title_links l2 JOIN netflix_titles n2 ON n2.id = l2.netflix_title_id
                           WHERE l2.title_id = l.title_id AND COALESCE(n2.tmdb_id, '') <> ''
                             AND NOT (n2.tmdb_kind = n.tmdb_kind AND n2.tmdb_id = n.tmdb_id)))) AS conflicts,
           (SELECT COUNT(*) FROM netflix_titles n
            WHERE n.tmdb_kind = r.tmdb_kind AND n.tmdb_id = r.tmdb_id
              AND n.kind <> CASE r.tmdb_kind WHEN 'tv' THEN 'tv_show' ELSE 'movie' END)
         + (SELECT COUNT(*) FROM titles t
            WHERE t.kind = CASE r.tmdb_kind WHEN 'tv' THEN 'tv_show' ELSE 'movie' END AND t.tmdb_id = r.tmdb_id
              AND COALESCE(t.fp_kind, t.kind) <> t.kind) AS cross_kind,
           (SELECT COUNT(*) FROM titles t
            WHERE t.kind = CASE r.tmdb_kind WHEN 'tv' THEN 'tv_show' ELSE 'movie' END AND t.tmdb_id = r.tmdb_id AND t.match_status = 'manual')
         + (SELECT COUNT(*) FROM netflix_titles n
            WHERE n.tmdb_kind = r.tmdb_kind AND n.tmdb_id = r.tmdb_id AND n.match_status = 'manual') AS manual,
           max(
               COALESCE((SELECT MAX(CAST(rk.ranked_on AS TEXT)) FROM rankings rk JOIN titles t ON t.id = rk.title_id
                         WHERE t.kind = CASE r.tmdb_kind WHEN 'tv' THEN 'tv_show' ELSE 'movie' END AND t.tmdb_id = r.tmdb_id), ''),
               COALESCE((SELECT MAX(g.week) FROM netflix_top10_global g JOIN netflix_titles n ON n.id = g.netflix_title_id
                         WHERE n.tmdb_kind = r.tmdb_kind AND n.tmdb_id = r.tmdb_id), ''),
               COALESCE((SELECT MAX(c.week) FROM netflix_top10_countries c JOIN netflix_titles n ON n.id = c.netflix_title_id
                         WHERE n.tmdb_kind = r.tmdb_kind AND n.tmdb_id = r.tmdb_id), '')) AS last_seen
    FROM (
        SELECT CASE kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END AS tmdb_kind, tmdb_id
        FROM titles WHERE COALESCE(tmdb_id, '') <> ''
        UNION
        SELECT tmdb_kind, tmdb_id
        FROM netflix_titles WHERE COALESCE(tmdb_id, '') <> '' AND tmdb_kind IS NOT NULL
    ) r
    WHERE (CAST(sqlc.arg(tmdb_kind) AS TEXT) = '' OR r.tmdb_kind = sqlc.arg(tmdb_kind))
) m
LEFT JOIN tmdb_titles tm ON tm.tmdb_kind = m.tmdb_kind AND tm.tmdb_id = m.tmdb_id
WHERE (CAST(sqlc.arg(search) AS TEXT) = ''
    OR m.tmdb_id = sqlc.arg(search)
    OR COALESCE(tm.imdb_id, '') = sqlc.arg(search)
    OR COALESCE(tm.title, '') LIKE '%' || sqlc.arg(search) || '%'
    OR COALESCE(tm.original_title, '') LIKE '%' || sqlc.arg(search) || '%'
    OR EXISTS (SELECT 1 FROM titles t
               WHERE t.kind = CASE m.tmdb_kind WHEN 'tv' THEN 'tv_show' ELSE 'movie' END AND t.tmdb_id = m.tmdb_id
                 AND (t.name LIKE '%' || sqlc.arg(search) || '%' OR t.slug LIKE '%' || sqlc.arg(search) || '%'))
    OR EXISTS (SELECT 1 FROM netflix_titles n
               WHERE n.tmdb_kind = m.tmdb_kind AND n.tmdb_id = m.tmdb_id AND n.show_title LIKE '%' || sqlc.arg(search) || '%'))
  AND (CAST(sqlc.arg(flag) AS TEXT) = ''
    OR (sqlc.arg(flag) = 'conflict' AND m.conflicts > 0)
    OR (sqlc.arg(flag) = 'cross_kind' AND m.cross_kind > 0)
    OR (sqlc.arg(flag) = 'both' AND m.flixpatrol_titles > 0 AND m.netflix_titles > 0)
    OR (sqlc.arg(flag) = 'netflix_only' AND m.flixpatrol_titles = 0)
    OR (sqlc.arg(flag) = 'flixpatrol_only' AND m.netflix_titles = 0)
    OR (sqlc.arg(flag) = 'manual' AND m.manual > 0))
ORDER BY m.last_seen DESC, m.tmdb_kind, m.tmdb_id
LIMIT sqlc.arg(limit) OFFSET sqlc.arg(offset);

-- name: ListFlixPatrolTitlesByTmdbIDs :many
-- FlixPatrol titles mapped to these TMDB IDs (either kind; callers filter),
-- with their last Netflix-or-other ranking date.
SELECT sqlc.embed(t),
       CAST(COALESCE((SELECT MAX(CAST(r.ranked_on AS TEXT)) FROM rankings r WHERE r.title_id = t.id), '') AS TEXT) AS last_ranked_on
FROM titles t
WHERE t.tmdb_id IN (sqlc.slice(tmdb_ids));

-- name: ListNetflixTitlesByTmdbIDs :many
SELECT sqlc.embed(n),
       CAST(COALESCE(NULLIF(max(
           COALESCE((SELECT MAX(g.week) FROM netflix_top10_global g WHERE g.netflix_title_id = n.id), ''),
           COALESCE((SELECT MAX(c.week) FROM netflix_top10_countries c WHERE c.netflix_title_id = n.id), '')), ''), '') AS TEXT) AS last_week
FROM netflix_titles n
WHERE n.tmdb_id IN (sqlc.slice(tmdb_ids));

-- name: ListTitleLinksForNetflixIDs :many
SELECT l.netflix_title_id, l.title_id, t.name, t.slug, t.kind, t.tmdb_id, t.match_status
FROM title_links l
JOIN titles t ON t.id = l.title_id
WHERE l.netflix_title_id IN (sqlc.slice(ids));

-- name: ListTitleLinksForFlixPatrolIDs :many
SELECT l.netflix_title_id, l.title_id, n.show_title, n.kind, n.tmdb_kind, n.tmdb_id, n.match_status
FROM title_links l
JOIN netflix_titles n ON n.id = l.netflix_title_id
WHERE l.title_id IN (sqlc.slice(ids));

-- name: InsertMappingEvent :exec
INSERT INTO mapping_events (subject_type, subject_id, action, from_tmdb_kind, from_tmdb_id, to_tmdb_kind, to_tmdb_id, source, actor)
VALUES (sqlc.arg(subject_type), sqlc.arg(subject_id), sqlc.arg(action), sqlc.narg(from_tmdb_kind), sqlc.narg(from_tmdb_id),
        sqlc.narg(to_tmdb_kind), sqlc.narg(to_tmdb_id), sqlc.narg(source), sqlc.arg(actor));

-- name: ListMappingEvents :many
-- Newest first, optionally of one subject (flixpatrol | netflix + id) and/or
-- one TMDB title (as the old or the new mapping).
SELECT e.*,
       CAST(COALESCE(CASE e.subject_type
           WHEN 'flixpatrol' THEN (SELECT t.name FROM titles t WHERE t.id = e.subject_id)
           ELSE (SELECT n.show_title FROM netflix_titles n WHERE n.id = e.subject_id) END, '') AS TEXT) AS subject_name
FROM mapping_events e
WHERE (CAST(sqlc.arg(subject_type) AS TEXT) = ''
       OR (e.subject_type = sqlc.arg(subject_type) AND e.subject_id = CAST(sqlc.arg(subject_id) AS INTEGER)))
  AND (CAST(sqlc.arg(tmdb_kind) AS TEXT) = ''
       OR (e.to_tmdb_kind = sqlc.arg(tmdb_kind) AND e.to_tmdb_id = CAST(sqlc.arg(tmdb_id) AS TEXT))
       OR (e.from_tmdb_kind = sqlc.arg(tmdb_kind) AND e.from_tmdb_id = CAST(sqlc.arg(tmdb_id) AS TEXT)))
ORDER BY e.id DESC
LIMIT sqlc.arg(limit);

-- name: UpsertMappingEvidence :exec
-- What one source says a title is (one row per title and kind of evidence).
INSERT INTO mapping_evidence (subject_type, subject_id, kind, tmdb_kind, tmdb_id, source, imdb_id, via_title_id)
VALUES (sqlc.arg(subject_type), sqlc.arg(subject_id), sqlc.arg(kind), sqlc.narg(tmdb_kind), sqlc.narg(tmdb_id),
        sqlc.narg(source), sqlc.narg(imdb_id), sqlc.narg(via_title_id))
ON CONFLICT(subject_type, subject_id, kind) DO UPDATE SET
    tmdb_kind = excluded.tmdb_kind, tmdb_id = excluded.tmdb_id, source = excluded.source,
    imdb_id = excluded.imdb_id, via_title_id = excluded.via_title_id, updated_at = CURRENT_TIMESTAMP;

-- name: DeleteMappingEvidence :execrows
-- One kind of a title's evidence, or all of it (kind '').
DELETE FROM mapping_evidence
WHERE subject_type = sqlc.arg(subject_type) AND subject_id = sqlc.arg(subject_id)
  AND (CAST(sqlc.arg(kind) AS TEXT) = '' OR kind = sqlc.arg(kind));

-- name: DeleteNetflixEvidenceVia :many
-- Netflix matches made through a FlixPatrol title (whose match was cleared).
DELETE FROM mapping_evidence
WHERE subject_type = 'netflix' AND kind = 'match' AND via_title_id = sqlc.arg(title_id)
RETURNING subject_id;

-- name: ListMappingEvidence :many
SELECT * FROM mapping_evidence WHERE subject_type = sqlc.arg(subject_type) AND subject_id IN (sqlc.slice(ids));

-- name: ListAllMappingEvidence :many
SELECT * FROM mapping_evidence;

-- name: ListIMDbMismatches :many
-- Titles whose deciding evidence came with an IMDb ID other than their TMDB
-- title's: the match or TMDB's IMDb ID is wrong.
SELECT e.subject_type, e.subject_id,
       CAST(COALESCE(CASE e.subject_type
           WHEN 'flixpatrol' THEN (SELECT t.name FROM titles t WHERE t.id = e.subject_id)
           ELSE (SELECT n.show_title FROM netflix_titles n WHERE n.id = e.subject_id) END, '') AS TEXT) AS name,
       CAST(e.tmdb_kind AS TEXT) AS tmdb_kind, CAST(e.tmdb_id AS TEXT) AS tmdb_id, CAST(e.imdb_id AS TEXT) AS imdb_id,
       CAST(m.imdb_id AS TEXT) AS tmdb_imdb_id, e.kind AS evidence_kind, CAST(COALESCE(e.source, '') AS TEXT) AS source
FROM mapping_evidence e
JOIN tmdb_titles m ON m.tmdb_kind = e.tmdb_kind AND m.tmdb_id = e.tmdb_id
WHERE COALESCE(e.imdb_id, '') <> '' AND COALESCE(m.imdb_id, '') <> '' AND e.imdb_id <> m.imdb_id
  AND NOT EXISTS (
    SELECT 1 FROM mapping_evidence h
    WHERE h.subject_type = e.subject_type AND h.subject_id = e.subject_id
      AND (CASE h.kind WHEN 'manual' THEN 3 WHEN 'netflix_id' THEN 2 ELSE 1 END)
        > (CASE e.kind WHEN 'manual' THEN 3 WHEN 'netflix_id' THEN 2 ELSE 1 END))
ORDER BY 1, 3
LIMIT sqlc.arg(limit);
