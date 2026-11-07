-- Discovery for tides (service/discover.go): ratings, offers and offer
-- changes per country across the titles metareel knows.

-- name: ListIMDbRatedTitles :many
-- TMDB titles with an IMDb rating from IMDb's dataset.
SELECT m.tmdb_kind, m.tmdb_id, CAST(COALESCE(m.title, '') AS TEXT) AS title,
       CAST(COALESCE(m.release_date, '') AS TEXT) AS release_date,
       CAST(COALESCE(m.poster_path, '') AS TEXT) AS poster_path, m.genres,
       r.average_rating, r.num_votes
FROM tmdb_titles m
JOIN imdb_ratings r ON r.imdb_id = m.imdb_id
WHERE m.imdb_id IS NOT NULL AND m.imdb_id <> '';

-- name: ListCountryOffers :many
-- Every offer in a country (any monetization) of the titles metareel knows.
SELECT w.tmdb_kind, w.tmdb_id, w.provider_id, w.provider_name, w.monetization
FROM tmdb_watch_providers w
WHERE w.country = sqlc.arg(country)
ORDER BY w.tmdb_kind, w.tmdb_id, w.display_priority, w.provider_id;

-- name: ListCountryOfferEvents :many
-- Subscription / free / ads offers that arrived or left in a country since
-- a time ('YYYY-MM-DD HH:MM:SS' UTC).
SELECT e.tmdb_kind, e.tmdb_id, e.provider_id, e.provider_name, e.monetization, e.event,
       CAST(e.observed_at AS TEXT) AS observed_at, CAST(COALESCE(m.title, '') AS TEXT) AS title
FROM watch_provider_events e
LEFT JOIN tmdb_titles m ON m.tmdb_kind = e.tmdb_kind AND m.tmdb_id = e.tmdb_id
WHERE e.country = sqlc.arg(country) AND e.observed_at >= CAST(sqlc.arg(since) AS TEXT)
  AND e.monetization IN ('flatrate', 'free', 'ads')
ORDER BY e.observed_at DESC, e.id DESC;
