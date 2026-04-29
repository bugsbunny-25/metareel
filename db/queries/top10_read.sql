-- name: ListTop10ByProvider :many
SELECT
    r.ranked_on,
    r.country,
    r.streaming_provider,
    r.category,
    r.rank,
    t.slug,
    t.name,
    t.kind,
    t.tmdb_id,
    t.imdb_id,
    t.rt_url
FROM rankings r
JOIN titles t ON t.id = r.title_id
WHERE r.ranked_on = ?
  AND r.country = ?
  AND r.streaming_provider = ?
  AND r.category = ?
ORDER BY r.rank ASC;

-- name: ListTop10AllProviders :many
SELECT
    r.ranked_on,
    r.country,
    r.streaming_provider,
    r.category,
    r.rank,
    t.slug,
    t.name,
    t.kind,
    t.tmdb_id,
    t.imdb_id,
    t.rt_url
FROM rankings r
JOIN titles t ON t.id = r.title_id
WHERE r.ranked_on = ?
  AND r.country = ?
  AND r.category = ?
ORDER BY r.streaming_provider ASC, r.rank ASC;

