-- Top 10 chart reads. A chart is (country, streaming_provider, category).
-- ranked_on may be NULL to mean the chart's latest scraped date.
-- previous_rank is the title's rank on the chart's previous scraped date
-- (not necessarily the previous calendar day, since scrapes can be missed);
-- NULL means the title was not on that chart. days_in_top10 counts every
-- date the title has been on the chart up to and including ranked_on.

-- name: ListTop10ByProvider :many
SELECT
    r.ranked_on,
    r.country,
    r.streaming_provider,
    r.category,
    r.rank,
    t.id AS title_id,
    t.slug,
    t.name,
    t.kind,
    t.tmdb_id,
    t.imdb_id,
    t.rt_url,
    COALESCE((
        SELECT MIN(p.rank)
        FROM rankings p
        WHERE p.title_id = r.title_id
          AND p.country = r.country
          AND p.streaming_provider = r.streaming_provider
          AND p.category = r.category
          AND p.ranked_on = (
              SELECT MAX(x.ranked_on)
              FROM rankings x
              WHERE x.country = r.country
                AND x.streaming_provider = r.streaming_provider
                AND x.category = r.category
                AND x.ranked_on < r.ranked_on
          )
    ), 0) AS previous_rank, -- 0 = not on the previous chart
    (
        SELECT COUNT(DISTINCT d.ranked_on)
        FROM rankings d
        WHERE d.title_id = r.title_id
          AND d.country = r.country
          AND d.streaming_provider = r.streaming_provider
          AND d.category = r.category
          AND d.ranked_on <= r.ranked_on
    ) AS days_in_top10
FROM rankings r
JOIN titles t ON t.id = r.title_id
WHERE r.country = sqlc.arg(country)
  AND r.streaming_provider = sqlc.arg(streaming_provider)
  AND r.category = sqlc.arg(category)
  AND r.ranked_on = COALESCE(sqlc.narg(ranked_on), (
      SELECT MAX(l.ranked_on)
      FROM rankings l
      WHERE l.country = r.country
        AND l.streaming_provider = r.streaming_provider
        AND l.category = r.category
  ))
ORDER BY r.rank ASC;

-- With ranked_on NULL, each provider's own latest chart is returned, so
-- providers scraped at different times of day are all present.

-- name: ListTop10AllProviders :many
SELECT
    r.ranked_on,
    r.country,
    r.streaming_provider,
    r.category,
    r.rank,
    t.id AS title_id,
    t.slug,
    t.name,
    t.kind,
    t.tmdb_id,
    t.imdb_id,
    t.rt_url,
    COALESCE((
        SELECT MIN(p.rank)
        FROM rankings p
        WHERE p.title_id = r.title_id
          AND p.country = r.country
          AND p.streaming_provider = r.streaming_provider
          AND p.category = r.category
          AND p.ranked_on = (
              SELECT MAX(x.ranked_on)
              FROM rankings x
              WHERE x.country = r.country
                AND x.streaming_provider = r.streaming_provider
                AND x.category = r.category
                AND x.ranked_on < r.ranked_on
          )
    ), 0) AS previous_rank, -- 0 = not on the previous chart
    (
        SELECT COUNT(DISTINCT d.ranked_on)
        FROM rankings d
        WHERE d.title_id = r.title_id
          AND d.country = r.country
          AND d.streaming_provider = r.streaming_provider
          AND d.category = r.category
          AND d.ranked_on <= r.ranked_on
    ) AS days_in_top10
FROM rankings r
JOIN titles t ON t.id = r.title_id
WHERE r.country = sqlc.arg(country)
  AND r.category = sqlc.arg(category)
  AND r.ranked_on = COALESCE(sqlc.narg(ranked_on), (
      SELECT MAX(l.ranked_on)
      FROM rankings l
      WHERE l.country = r.country
        AND l.streaming_provider = r.streaming_provider
        AND l.category = r.category
  ))
ORDER BY r.streaming_provider ASC, r.rank ASC;

-- Ranking history by TMDB ID. Several FlixPatrol slugs can map to the same
-- TMDB title, so these return every matching title row.

-- name: ListTitlesByTmdbIDs :many
SELECT id, slug, name, kind, tmdb_id, imdb_id, rt_url, created_at, updated_at
FROM titles
WHERE tmdb_id IN (sqlc.slice(tmdb_ids))
ORDER BY tmdb_id, id;

-- name: ListRankingsByTitleIDs :many
SELECT title_id, ranked_on, country, streaming_provider, category, rank
FROM rankings
-- The slice must stay last: sqlc numbers the named params ?1..?3 and SQLite
-- numbers the expanded "?" list after them.
WHERE (sqlc.narg(country) IS NULL OR country = sqlc.narg(country))
  AND (sqlc.narg(from_date) IS NULL OR ranked_on >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date) IS NULL OR ranked_on <= sqlc.narg(to_date))
  AND title_id IN (sqlc.slice(title_ids))
ORDER BY ranked_on, country, streaming_provider, category, rank;
