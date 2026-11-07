-- Title search (GET /search): every name a mapped TMDB title goes by, built
-- into an in-memory index (service/search.go).

-- name: ListSearchNames :many
-- One row per (TMDB title, name): TMDB's title and original title, the
-- FlixPatrol chart / page names and Netflix Top 10 names mapped to it.
SELECT n.tmdb_kind, n.tmdb_id, n.name, n.source
FROM (
    SELECT tmdb_kind, tmdb_id, title AS name, 'tmdb' AS source FROM tmdb_titles WHERE COALESCE(title, '') <> ''
    UNION SELECT tmdb_kind, tmdb_id, original_title, 'tmdb' FROM tmdb_titles WHERE COALESCE(original_title, '') <> ''
    UNION SELECT CASE kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END, tmdb_id, name, 'flixpatrol' FROM titles WHERE COALESCE(tmdb_id, '') <> ''
    UNION SELECT CASE kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END, tmdb_id, fp_name, 'flixpatrol' FROM titles
        WHERE COALESCE(tmdb_id, '') <> '' AND COALESCE(fp_name, '') <> ''
    UNION SELECT tmdb_kind, tmdb_id, show_title, 'netflix' FROM netflix_titles WHERE COALESCE(tmdb_id, '') <> '' AND tmdb_kind IS NOT NULL
) n
WHERE EXISTS (SELECT 1 FROM titles t WHERE t.tmdb_id = n.tmdb_id AND CASE t.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END = n.tmdb_kind)
   OR EXISTS (SELECT 1 FROM netflix_titles x WHERE x.tmdb_id = n.tmdb_id AND x.tmdb_kind = n.tmdb_kind);

-- name: ListSearchTitles :many
-- The TMDB titles chart titles are mapped to, with what a result shows:
-- TMDB metadata and when it last charted (FlixPatrol date / Netflix week).
SELECT r.tmdb_kind, r.tmdb_id,
       CAST(COALESCE(m.title, '') AS TEXT) AS title,
       CAST(COALESCE(m.original_title, '') AS TEXT) AS original_title,
       CAST(COALESCE(m.release_date, '') AS TEXT) AS release_date,
       CAST(COALESCE(m.poster_path, '') AS TEXT) AS poster_path,
       CAST(COALESCE((SELECT MAX(rk.ranked_on) FROM titles t JOIN rankings rk ON rk.title_id = t.id
                      WHERE t.tmdb_id = r.tmdb_id AND CASE t.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END = r.tmdb_kind), '') AS TEXT) AS last_flixpatrol,
       CAST(COALESCE((SELECT MAX(g.week) FROM netflix_titles x JOIN netflix_top10_global g ON g.netflix_title_id = x.id
                      WHERE x.tmdb_id = r.tmdb_id AND x.tmdb_kind = r.tmdb_kind), '') AS TEXT) AS last_netflix
FROM (
    SELECT DISTINCT CASE kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END AS tmdb_kind, tmdb_id FROM titles WHERE COALESCE(tmdb_id, '') <> ''
    UNION SELECT tmdb_kind, tmdb_id FROM netflix_titles WHERE COALESCE(tmdb_id, '') <> '' AND tmdb_kind IS NOT NULL
) r
LEFT JOIN tmdb_titles m ON m.tmdb_kind = r.tmdb_kind AND m.tmdb_id = r.tmdb_id;

-- name: SearchIndexStamp :one
-- Changes whenever a name, a mapping or a chart changes, so the index is
-- rebuilt only then.
SELECT CAST(
    (SELECT COUNT(*) || ':' || COALESCE(MAX(updated_at), '') FROM tmdb_titles) || '|' ||
    (SELECT COUNT(*) || ':' || COALESCE(MAX(updated_at), '') FROM titles) || '|' ||
    (SELECT COUNT(*) || ':' || COALESCE(MAX(updated_at), '') FROM netflix_titles) || '|' ||
    (SELECT COALESCE(MAX(scraped_at), '') FROM chart_snapshots) || '|' ||
    (SELECT COALESCE(MAX(week), '') FROM netflix_top10_global)
AS TEXT) AS stamp;
