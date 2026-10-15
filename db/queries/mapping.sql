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

-- name: AdoptNetflixTitleMapping :execrows
-- Shares a linked FlixPatrol title's mapping (never touches manual titles).
-- The IMDb ID falls back to TMDB's.
UPDATE netflix_titles
SET tmdb_kind     = sqlc.arg(tmdb_kind),
    tmdb_id       = sqlc.arg(tmdb_id),
    imdb_id       = COALESCE(NULLIF(CAST(sqlc.arg(imdb_id) AS TEXT), ''),
                             (SELECT m.imdb_id FROM tmdb_titles m WHERE m.tmdb_kind = sqlc.arg(tmdb_kind) AND m.tmdb_id = sqlc.arg(tmdb_id))),
    title_id      = COALESCE(sqlc.narg(title_id), netflix_titles.title_id),
    match_status  = 'matched',
    match_source  = sqlc.arg(match_source),
    next_match_at = NULL,
    updated_at    = CURRENT_TIMESTAMP
WHERE netflix_titles.id = sqlc.arg(id) AND netflix_titles.match_status <> 'manual';

-- name: AdoptFlixPatrolTitleMapping :execrows
-- Shares a linked Netflix title's mapping (never touches manual titles).
-- IDs that belonged to the previous match (RT, JustWatch, Wikidata) are
-- dropped when the TMDB title changes, and the RT lookup is due again.
UPDATE titles
SET kind         = sqlc.arg(kind),
    tmdb_id      = sqlc.arg(tmdb_id),
    imdb_id      = COALESCE(NULLIF(CAST(sqlc.arg(imdb_id) AS TEXT), ''),
                            (SELECT m.imdb_id FROM tmdb_titles m
                             WHERE m.tmdb_kind = CASE sqlc.arg(kind) WHEN 'tv_show' THEN 'tv' ELSE 'movie' END
                               AND m.tmdb_id = sqlc.arg(tmdb_id))),
    rt_url       = CASE WHEN titles.kind = sqlc.arg(kind) AND COALESCE(titles.tmdb_id, '') = sqlc.arg(tmdb_id) THEN titles.rt_url END,
    justwatch_id = CASE WHEN titles.kind = sqlc.arg(kind) AND COALESCE(titles.tmdb_id, '') = sqlc.arg(tmdb_id) THEN titles.justwatch_id END,
    wikidata_id  = CASE WHEN titles.kind = sqlc.arg(kind) AND COALESCE(titles.tmdb_id, '') = sqlc.arg(tmdb_id) THEN titles.wikidata_id END,
    matched_name = NULL,
    matched_year = NULL,
    rt_attempts  = 0,
    next_rt_at   = NULL,
    match_status = 'matched',
    match_source = 'netflix_title',
    next_match_at = NULL,
    updated_at   = CURRENT_TIMESTAMP
WHERE titles.id = sqlc.arg(id) AND titles.match_status <> 'manual';

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

-- name: RemapFlixPatrolTitles :many
-- Moves every FlixPatrol title on one TMDB title to another (fixed by hand).
UPDATE titles
SET kind          = sqlc.arg(new_kind),
    tmdb_id       = sqlc.arg(new_tmdb_id),
    imdb_id       = (SELECT m.imdb_id FROM tmdb_titles m
                     WHERE m.tmdb_kind = CASE sqlc.arg(new_kind) WHEN 'tv_show' THEN 'tv' ELSE 'movie' END
                       AND m.tmdb_id = sqlc.arg(new_tmdb_id)),
    rt_url        = NULL,
    justwatch_id  = NULL,
    wikidata_id   = NULL,
    matched_name  = NULL,
    matched_year  = NULL,
    rt_attempts   = 0,
    next_rt_at    = NULL,
    match_status  = 'manual',
    match_source  = 'manual',
    next_match_at = NULL,
    updated_at    = CURRENT_TIMESTAMP
WHERE titles.kind = sqlc.arg(old_kind) AND titles.tmdb_id = sqlc.arg(old_tmdb_id)
RETURNING id;

-- name: RemapNetflixTitles :many
-- Moves every Netflix title on one TMDB title to another (fixed by hand).
UPDATE netflix_titles
SET tmdb_kind     = sqlc.arg(new_tmdb_kind),
    tmdb_id       = sqlc.arg(new_tmdb_id),
    imdb_id       = (SELECT m.imdb_id FROM tmdb_titles m WHERE m.tmdb_kind = sqlc.arg(new_tmdb_kind) AND m.tmdb_id = sqlc.arg(new_tmdb_id)),
    match_status  = 'manual',
    match_source  = 'manual',
    next_match_at = NULL,
    updated_at    = CURRENT_TIMESTAMP
WHERE netflix_titles.tmdb_kind = sqlc.arg(old_tmdb_kind) AND netflix_titles.tmdb_id = sqlc.arg(old_tmdb_id)
RETURNING id;

-- name: ClearNetflixTitlesAdoptedFrom :many
-- After a FlixPatrol title's match is cleared, linked Netflix titles that only
-- took their mapping from it are cleared too (else they would hand it back).
UPDATE netflix_titles
SET tmdb_kind = NULL, tmdb_id = NULL, imdb_id = NULL, match_status = 'pending', match_source = NULL,
    next_match_at = NULL, updated_at = CURRENT_TIMESTAMP
WHERE netflix_titles.match_source = 'flixpatrol_title' AND netflix_titles.match_status <> 'manual'
  AND netflix_titles.id IN (SELECT l.netflix_title_id FROM title_links l WHERE l.title_id = sqlc.arg(title_id))
RETURNING id;

-- name: ClearFlixPatrolTitlesAdoptedFrom :exec
-- The same for FlixPatrol titles that took a cleared Netflix title's mapping.
UPDATE titles
SET tmdb_id = NULL, imdb_id = NULL, rt_url = NULL, justwatch_id = NULL, wikidata_id = NULL,
    match_status = 'pending', match_source = NULL, matched_name = NULL, matched_year = NULL,
    next_match_at = NULL, rt_attempts = 0, next_rt_at = NULL, updated_at = CURRENT_TIMESTAMP
WHERE titles.match_source = 'netflix_title' AND titles.match_status <> 'manual'
  AND titles.id IN (SELECT l.title_id FROM title_links l WHERE l.netflix_title_id = sqlc.arg(netflix_title_id));
