-- Aggregates over one user's watch log: totals, the calendar, time of day,
-- ratings against the crowd, spend, formats, and what their watchlist is
-- doing on the charts.
--
-- How long a watch was is not stored on every row: runtime_minutes_used is
-- only set when a start was derived from it. These queries fall back to the
-- episode's runtime, then the title's, so an imprecise watch still counts.
-- Every statement is scoped by user_id.

-- name: StatsTotals :one
SELECT
    CAST(COUNT(*) AS INTEGER) AS watches,
    CAST(COALESCE(SUM(CASE WHEN w.subject_kind = 'movie' THEN 1 ELSE 0 END), 0) AS INTEGER) AS movie_watches,
    CAST(COALESCE(SUM(CASE WHEN w.subject_kind = 'episode' THEN 1 ELSE 0 END), 0) AS INTEGER) AS episode_watches,
    CAST(COALESCE(SUM(CASE WHEN w.is_rewatch = 1 THEN 1 ELSE 0 END), 0) AS INTEGER) AS rewatches,
    CAST(COUNT(DISTINCT w.tmdb_kind || ':' || w.tmdb_id) AS INTEGER) AS distinct_titles,
    CAST(COALESCE(SUM(COALESCE(w.runtime_minutes_used, e.runtime, t.runtime, c.runtime, 0)), 0) AS INTEGER) AS minutes,
    CAST(COUNT(DISTINCT w.watched_on) AS INTEGER) AS days,
    CAST(COALESCE(SUM(CASE WHEN w.rating IS NOT NULL THEN 1 ELSE 0 END), 0) AS INTEGER) AS rated,
    CAST(COALESCE(SUM(CASE WHEN w.liked = 1 THEN 1 ELSE 0 END), 0) AS INTEGER) AS liked
FROM watch_entries w
LEFT JOIN tracked_episodes e ON w.subject_kind = 'episode' AND e.tmdb_id = w.tmdb_id
     AND e.season_number = w.season_number AND e.episode_number = w.episode_number
LEFT JOIN tmdb_titles t ON t.tmdb_kind = w.tmdb_kind AND t.tmdb_id = w.tmdb_id
LEFT JOIN tracked_titles c ON c.tmdb_kind = w.tmdb_kind AND c.tmdb_id = w.tmdb_id
WHERE w.user_id = sqlc.arg(user_id)
  AND (CAST(sqlc.arg(from_day) AS TEXT) = '' OR w.watched_on >= CAST(sqlc.arg(from_day) AS TEXT))
  AND (CAST(sqlc.arg(to_day) AS TEXT) = '' OR w.watched_on <= CAST(sqlc.arg(to_day) AS TEXT));

-- StatsByVenueKind splits watching between the cinema, streaming and home.
-- name: StatsByVenueKind :many
SELECT
    CAST(COALESCE(w.venue_kind, '') AS TEXT) AS venue_kind,
    CAST(COUNT(*) AS INTEGER) AS watches,
    CAST(COALESCE(SUM(COALESCE(w.runtime_minutes_used, e.runtime, t.runtime, c.runtime, 0)), 0) AS INTEGER) AS minutes
FROM watch_entries w
LEFT JOIN tracked_episodes e ON w.subject_kind = 'episode' AND e.tmdb_id = w.tmdb_id
     AND e.season_number = w.season_number AND e.episode_number = w.episode_number
LEFT JOIN tmdb_titles t ON t.tmdb_kind = w.tmdb_kind AND t.tmdb_id = w.tmdb_id
LEFT JOIN tracked_titles c ON c.tmdb_kind = w.tmdb_kind AND c.tmdb_id = w.tmdb_id
WHERE w.user_id = sqlc.arg(user_id)
  AND (CAST(sqlc.arg(from_day) AS TEXT) = '' OR w.watched_on >= CAST(sqlc.arg(from_day) AS TEXT))
  AND (CAST(sqlc.arg(to_day) AS TEXT) = '' OR w.watched_on <= CAST(sqlc.arg(to_day) AS TEXT))
GROUP BY venue_kind
ORDER BY watches DESC;

-- StatsPerDay is the calendar heatmap, and what streaks are counted from.
-- name: StatsPerDay :many
SELECT
    w.watched_on,
    CAST(COUNT(*) AS INTEGER) AS watches,
    CAST(COALESCE(SUM(COALESCE(w.runtime_minutes_used, e.runtime, t.runtime, c.runtime, 0)), 0) AS INTEGER) AS minutes
FROM watch_entries w
LEFT JOIN tracked_episodes e ON w.subject_kind = 'episode' AND e.tmdb_id = w.tmdb_id
     AND e.season_number = w.season_number AND e.episode_number = w.episode_number
LEFT JOIN tmdb_titles t ON t.tmdb_kind = w.tmdb_kind AND t.tmdb_id = w.tmdb_id
LEFT JOIN tracked_titles c ON c.tmdb_kind = w.tmdb_kind AND c.tmdb_id = w.tmdb_id
WHERE w.user_id = sqlc.arg(user_id)
  AND (CAST(sqlc.arg(from_day) AS TEXT) = '' OR w.watched_on >= CAST(sqlc.arg(from_day) AS TEXT))
  AND (CAST(sqlc.arg(to_day) AS TEXT) = '' OR w.watched_on <= CAST(sqlc.arg(to_day) AS TEXT))
GROUP BY w.watched_on
ORDER BY w.watched_on;

-- StatsByHour buckets watching by the local hour it ended, using the offset
-- stored with each watch rather than UTC.
-- name: StatsByHour :many
SELECT
    CAST(strftime('%H', datetime(w.ended_at,
        CAST(COALESCE(w.utc_offset_minutes, 0) AS TEXT) || ' minutes')) AS TEXT) AS hour,
    CAST(COUNT(*) AS INTEGER) AS watches
FROM watch_entries w
WHERE w.user_id = sqlc.arg(user_id)
  AND w.time_precision = 'minute'
  AND (CAST(sqlc.arg(from_day) AS TEXT) = '' OR w.watched_on >= CAST(sqlc.arg(from_day) AS TEXT))
  AND (CAST(sqlc.arg(to_day) AS TEXT) = '' OR w.watched_on <= CAST(sqlc.arg(to_day) AS TEXT))
GROUP BY hour
ORDER BY hour;

-- StatsByWeekday buckets by the local day of the week (0 = Sunday).
-- name: StatsByWeekday :many
SELECT
    CAST(strftime('%w', w.watched_on) AS TEXT) AS weekday,
    CAST(COUNT(*) AS INTEGER) AS watches
FROM watch_entries w
WHERE w.user_id = sqlc.arg(user_id)
  AND (CAST(sqlc.arg(from_day) AS TEXT) = '' OR w.watched_on >= CAST(sqlc.arg(from_day) AS TEXT))
  AND (CAST(sqlc.arg(to_day) AS TEXT) = '' OR w.watched_on <= CAST(sqlc.arg(to_day) AS TEXT))
GROUP BY weekday
ORDER BY weekday;

-- name: StatsByMonth :many
SELECT
    CAST(substr(w.watched_on, 1, 7) AS TEXT) AS month,
    CAST(COUNT(*) AS INTEGER) AS watches,
    CAST(COALESCE(SUM(COALESCE(w.runtime_minutes_used, e.runtime, t.runtime, c.runtime, 0)), 0) AS INTEGER) AS minutes
FROM watch_entries w
LEFT JOIN tracked_episodes e ON w.subject_kind = 'episode' AND e.tmdb_id = w.tmdb_id
     AND e.season_number = w.season_number AND e.episode_number = w.episode_number
LEFT JOIN tmdb_titles t ON t.tmdb_kind = w.tmdb_kind AND t.tmdb_id = w.tmdb_id
LEFT JOIN tracked_titles c ON c.tmdb_kind = w.tmdb_kind AND c.tmdb_id = w.tmdb_id
WHERE w.user_id = sqlc.arg(user_id)
  AND (CAST(sqlc.arg(from_day) AS TEXT) = '' OR w.watched_on >= CAST(sqlc.arg(from_day) AS TEXT))
  AND (CAST(sqlc.arg(to_day) AS TEXT) = '' OR w.watched_on <= CAST(sqlc.arg(to_day) AS TEXT))
GROUP BY month
ORDER BY month;

-- name: StatsRatingDistribution :many
SELECT CAST(w.rating AS INTEGER) AS rating, CAST(COUNT(*) AS INTEGER) AS watches
FROM watch_entries w
WHERE w.user_id = sqlc.arg(user_id)
  AND w.rating IS NOT NULL
  AND (CAST(sqlc.arg(from_day) AS TEXT) = '' OR w.watched_on >= CAST(sqlc.arg(from_day) AS TEXT))
  AND (CAST(sqlc.arg(to_day) AS TEXT) = '' OR w.watched_on <= CAST(sqlc.arg(to_day) AS TEXT))
GROUP BY w.rating
ORDER BY w.rating;

-- StatsRatingComparison puts a user's rating next to the crowd's for every
-- title they rated. All three outside scores are normalised to 0-10, the
-- same scale as a rating in half-stars out of five.
-- name: StatsRatingComparison :many
SELECT
    u.tmdb_kind, u.tmdb_id,
    CAST(u.rating AS INTEGER) AS user_rating,
    CAST(COALESCE(NULLIF(t.title, ''), NULLIF(c.title, ''), '') AS TEXT) AS display_title,
    -- Left null when the crowd has no score: a missing rating is not a
    -- rating of zero. The Rotten Tomatoes value comes through a scalar
    -- subquery, which sqlc types as non-null, so it carries -1 as its
    -- "unknown" sentinel and the repository turns that back into nil.
    t.vote_average AS tmdb_rating,
    i.average_rating AS imdb_rating,
    CAST(COALESCE((SELECT s.value / 10.0 FROM title_rating_sources s
                   WHERE s.tmdb_kind = u.tmdb_kind AND s.tmdb_id = u.tmdb_id
                     AND s.source = 'tomatoes' ORDER BY s.fetched_at DESC LIMIT 1), -1) AS REAL) AS rt_rating
FROM user_titles u
LEFT JOIN tmdb_titles t ON t.tmdb_kind = u.tmdb_kind AND t.tmdb_id = u.tmdb_id
LEFT JOIN tracked_titles c ON c.tmdb_kind = u.tmdb_kind AND c.tmdb_id = u.tmdb_id
LEFT JOIN imdb_ratings i ON i.imdb_id = t.imdb_id
WHERE u.user_id = sqlc.arg(user_id) AND u.rating IS NOT NULL
ORDER BY u.rating DESC, display_title COLLATE NOCASE;

-- StatsWatchedTitles lists the distinct titles watched in a range with the
-- metadata the genre, decade and country breakdowns are built from in Go
-- (genres and origin countries are JSON arrays on tmdb_titles).
-- name: StatsWatchedTitles :many
SELECT
    w.tmdb_kind, w.tmdb_id,
    CAST(COUNT(*) AS INTEGER) AS watches,
    CAST(COALESCE(SUM(COALESCE(w.runtime_minutes_used, e.runtime, t.runtime, c.runtime, 0)), 0) AS INTEGER) AS minutes,
    CAST(COALESCE(NULLIF(t.title, ''), NULLIF(c.title, ''), '') AS TEXT) AS display_title,
    CAST(COALESCE(NULLIF(t.release_date, ''), NULLIF(c.release_date, ''), '') AS TEXT) AS release_date,
    CAST(COALESCE(t.genres, '') AS TEXT) AS genres,
    CAST(COALESCE(t.origin_countries, '') AS TEXT) AS origin_countries,
    CAST(COALESCE(t.original_language, '') AS TEXT) AS original_language
FROM watch_entries w
LEFT JOIN tracked_episodes e ON w.subject_kind = 'episode' AND e.tmdb_id = w.tmdb_id
     AND e.season_number = w.season_number AND e.episode_number = w.episode_number
LEFT JOIN tmdb_titles t ON t.tmdb_kind = w.tmdb_kind AND t.tmdb_id = w.tmdb_id
LEFT JOIN tracked_titles c ON c.tmdb_kind = w.tmdb_kind AND c.tmdb_id = w.tmdb_id
WHERE w.user_id = sqlc.arg(user_id)
  AND (CAST(sqlc.arg(from_day) AS TEXT) = '' OR w.watched_on >= CAST(sqlc.arg(from_day) AS TEXT))
  AND (CAST(sqlc.arg(to_day) AS TEXT) = '' OR w.watched_on <= CAST(sqlc.arg(to_day) AS TEXT))
GROUP BY w.tmdb_kind, w.tmdb_id
ORDER BY watches DESC, display_title COLLATE NOCASE;

-- StatsTopPeople ranks the people behind what a user watched. job '' counts
-- cast; any other value matches that crew job (Director, Screenplay, ...).
-- Only titles metareel has credits for can appear.
-- name: StatsTopPeople :many
SELECT
    p.person_id, p.name, CAST(COALESCE(p.profile_path, '') AS TEXT) AS profile_path,
    CAST(COUNT(DISTINCT w.tmdb_kind || ':' || w.tmdb_id) AS INTEGER) AS titles,
    CAST(COUNT(*) AS INTEGER) AS watches
FROM watch_entries w
JOIN tmdb_credits cr ON cr.tmdb_kind = w.tmdb_kind AND cr.tmdb_id = w.tmdb_id
JOIN tmdb_people p ON p.person_id = cr.person_id
WHERE w.user_id = sqlc.arg(user_id)
  AND ((CAST(sqlc.arg(job) AS TEXT) = '' AND cr.credit_type = 'cast')
       OR (CAST(sqlc.arg(job) AS TEXT) <> '' AND cr.job = CAST(sqlc.arg(job) AS TEXT)))
  AND (CAST(sqlc.arg(from_day) AS TEXT) = '' OR w.watched_on >= CAST(sqlc.arg(from_day) AS TEXT))
  AND (CAST(sqlc.arg(to_day) AS TEXT) = '' OR w.watched_on <= CAST(sqlc.arg(to_day) AS TEXT))
GROUP BY p.person_id, p.name, profile_path
ORDER BY titles DESC, watches DESC, p.name COLLATE NOCASE
LIMIT sqlc.arg(limit);

-- StatsSpendByCurrency totals money per currency. Amounts are never
-- converted between currencies: there is no rate to do it honestly with.
-- name: StatsSpendByCurrency :many
SELECT
    CAST(COALESCE(w.currency, '') AS TEXT) AS currency,
    CAST(COALESCE(SUM(w.price_minor), 0) AS INTEGER) AS price_minor,
    CAST(COALESCE(SUM(w.fees_minor), 0) AS INTEGER) AS fees_minor,
    CAST(COALESCE(SUM(w.concessions_minor), 0) AS INTEGER) AS concessions_minor,
    CAST(COUNT(*) AS INTEGER) AS watches,
    CAST(COALESCE(SUM(w.tickets_count), 0) AS INTEGER) AS tickets
FROM watch_entries w
WHERE w.user_id = sqlc.arg(user_id)
  AND w.currency IS NOT NULL
  AND (CAST(sqlc.arg(from_day) AS TEXT) = '' OR w.watched_on >= CAST(sqlc.arg(from_day) AS TEXT))
  AND (CAST(sqlc.arg(to_day) AS TEXT) = '' OR w.watched_on <= CAST(sqlc.arg(to_day) AS TEXT))
GROUP BY currency
ORDER BY price_minor DESC;

-- name: StatsSpendByVenue :many
SELECT
    CAST(COALESCE(w.venue_name_at_watch, '') AS TEXT) AS venue,
    CAST(COALESCE(w.currency, '') AS TEXT) AS currency,
    CAST(COUNT(*) AS INTEGER) AS watches,
    CAST(COALESCE(SUM(COALESCE(w.price_minor, 0) + COALESCE(w.fees_minor, 0)
                      + COALESCE(w.concessions_minor, 0)), 0) AS INTEGER) AS total_minor
FROM watch_entries w
WHERE w.user_id = sqlc.arg(user_id)
  AND w.venue_id IS NOT NULL
  AND (CAST(sqlc.arg(from_day) AS TEXT) = '' OR w.watched_on >= CAST(sqlc.arg(from_day) AS TEXT))
  AND (CAST(sqlc.arg(to_day) AS TEXT) = '' OR w.watched_on <= CAST(sqlc.arg(to_day) AS TEXT))
GROUP BY venue, currency
ORDER BY total_minor DESC, watches DESC;

-- StatsByFormat counts watches and time per premium format.
-- name: StatsByFormat :many
SELECT
    f.slug, f.label, f.category, f.logo_version,
    CAST(COUNT(*) AS INTEGER) AS watches,
    CAST(COALESCE(SUM(COALESCE(w.runtime_minutes_used, e.runtime, t.runtime, c.runtime, 0)), 0) AS INTEGER) AS minutes
FROM watch_entry_formats ef
JOIN watch_formats f ON f.id = ef.format_id
JOIN watch_entries w ON w.id = ef.entry_id
LEFT JOIN tracked_episodes e ON w.subject_kind = 'episode' AND e.tmdb_id = w.tmdb_id
     AND e.season_number = w.season_number AND e.episode_number = w.episode_number
LEFT JOIN tmdb_titles t ON t.tmdb_kind = w.tmdb_kind AND t.tmdb_id = w.tmdb_id
LEFT JOIN tracked_titles c ON c.tmdb_kind = w.tmdb_kind AND c.tmdb_id = w.tmdb_id
WHERE w.user_id = sqlc.arg(user_id)
  AND (CAST(sqlc.arg(from_day) AS TEXT) = '' OR w.watched_on >= CAST(sqlc.arg(from_day) AS TEXT))
  AND (CAST(sqlc.arg(to_day) AS TEXT) = '' OR w.watched_on <= CAST(sqlc.arg(to_day) AS TEXT))
GROUP BY f.slug, f.label, f.category, f.logo_version
ORDER BY minutes DESC, watches DESC;

-- WatchlistInCharts finds watchlist titles on a Top 10 right now. This is
-- the join that only works because the charts and the watchlist live in one
-- database.
-- name: WatchlistInCharts :many
SELECT
    u.tmdb_kind, u.tmdb_id,
    CAST(COALESCE(NULLIF(t.title, ''), NULLIF(c.title, ''), '') AS TEXT) AS display_title,
    r.country, r.streaming_provider, r.category,
    CAST(MIN(r.rank) AS INTEGER) AS best_rank,
    CAST(MAX(r.ranked_on) AS TEXT) AS ranked_on
FROM user_titles u
JOIN titles ti ON ti.tmdb_id = u.tmdb_id
     AND CASE ti.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END = u.tmdb_kind
JOIN rankings r ON r.title_id = ti.id
LEFT JOIN tmdb_titles t ON t.tmdb_kind = u.tmdb_kind AND t.tmdb_id = u.tmdb_id
LEFT JOIN tracked_titles c ON c.tmdb_kind = u.tmdb_kind AND c.tmdb_id = u.tmdb_id
WHERE u.user_id = sqlc.arg(user_id)
  AND u.status = 'watchlist'
  AND r.ranked_on >= CAST(sqlc.arg(since) AS TEXT)
  AND (CAST(sqlc.arg(country) AS TEXT) = '' OR r.country = sqlc.arg(country))
GROUP BY u.tmdb_kind, u.tmdb_id, display_title, r.country, r.streaming_provider, r.category
ORDER BY best_rank, ranked_on DESC
LIMIT sqlc.arg(limit);

-- WatchlistStreamingNow finds watchlist titles available in a country.
-- name: WatchlistStreamingNow :many
SELECT
    u.tmdb_kind, u.tmdb_id,
    CAST(COALESCE(NULLIF(t.title, ''), NULLIF(c.title, ''), '') AS TEXT) AS display_title,
    wp.provider_id, wp.provider_name, wp.monetization,
    CAST(COALESCE(wp.link, '') AS TEXT) AS link
FROM user_titles u
JOIN tmdb_watch_providers wp ON wp.tmdb_kind = u.tmdb_kind AND wp.tmdb_id = u.tmdb_id
LEFT JOIN tmdb_titles t ON t.tmdb_kind = u.tmdb_kind AND t.tmdb_id = u.tmdb_id
LEFT JOIN tracked_titles c ON c.tmdb_kind = u.tmdb_kind AND c.tmdb_id = u.tmdb_id
WHERE u.user_id = sqlc.arg(user_id)
  AND u.status = 'watchlist'
  AND wp.country = CAST(sqlc.arg(country) AS TEXT)
  AND (CAST(sqlc.arg(monetization) AS TEXT) = '' OR wp.monetization = sqlc.arg(monetization))
ORDER BY display_title COLLATE NOCASE, wp.provider_name
LIMIT sqlc.arg(limit);

-- WatchlistRecentlyLeft finds watchlist titles that came off a service in a
-- country. metareel records what has happened, not what is scheduled, so
-- this is what has already gone, not what is leaving soon.
-- name: WatchlistRecentlyLeft :many
SELECT
    u.tmdb_kind, u.tmdb_id,
    CAST(COALESCE(NULLIF(t.title, ''), NULLIF(c.title, ''), '') AS TEXT) AS display_title,
    ev.provider_name, ev.monetization, ev.country,
    CAST(ev.observed_at AS TEXT) AS observed_at
FROM user_titles u
JOIN watch_provider_events ev ON ev.tmdb_kind = u.tmdb_kind AND ev.tmdb_id = u.tmdb_id
LEFT JOIN tmdb_titles t ON t.tmdb_kind = u.tmdb_kind AND t.tmdb_id = u.tmdb_id
LEFT JOIN tracked_titles c ON c.tmdb_kind = u.tmdb_kind AND c.tmdb_id = u.tmdb_id
WHERE u.user_id = sqlc.arg(user_id)
  AND u.status = 'watchlist'
  AND ev.event = 'removed'
  AND ev.country = CAST(sqlc.arg(country) AS TEXT)
  AND ev.observed_at >= CAST(sqlc.arg(since) AS TEXT)
ORDER BY ev.observed_at DESC
LIMIT sqlc.arg(limit);
