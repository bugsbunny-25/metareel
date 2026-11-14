-- Cinema subscriptions of a user and the watches they covered.

-- name: ListCinemaSubscriptions :many
SELECT id, user_id, plan, chain, price_minor, currency, period, started_on, ended_on, note, created_at, updated_at
FROM cinema_subscriptions WHERE user_id = sqlc.arg(user_id)
ORDER BY COALESCE(ended_on, '9999-12-31') DESC, started_on DESC, id DESC;

-- name: GetCinemaSubscription :one
SELECT id, user_id, plan, chain, price_minor, currency, period, started_on, ended_on, note, created_at, updated_at
FROM cinema_subscriptions WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: CreateCinemaSubscription :one
INSERT INTO cinema_subscriptions (user_id, plan, chain, price_minor, currency, period, started_on, ended_on, note)
VALUES (sqlc.arg(user_id), sqlc.arg(plan), sqlc.narg(chain), sqlc.arg(price_minor), sqlc.arg(currency), sqlc.arg(period),
        sqlc.arg(started_on), sqlc.narg(ended_on), sqlc.narg(note))
RETURNING id;

-- name: UpdateCinemaSubscription :execrows
UPDATE cinema_subscriptions SET
    plan = sqlc.arg(plan), chain = sqlc.narg(chain), price_minor = sqlc.arg(price_minor), currency = sqlc.arg(currency),
    period = sqlc.arg(period), started_on = sqlc.arg(started_on), ended_on = sqlc.narg(ended_on), note = sqlc.narg(note),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: DeleteCinemaSubscription :execrows
DELETE FROM cinema_subscriptions WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- SubscriptionCinemaWatches lists a user's cinema watches with what they
-- cost, for the savings analysis: covered ones and paid ones (paid tickets
-- price the covered ones that have no face value, and feed the what-if).
-- name: SubscriptionCinemaWatches :many
SELECT w.id, CAST(w.watched_on AS TEXT) AS watched_on, w.subscription_id, w.face_value_minor, w.price_minor, w.fees_minor,
       w.currency, w.tickets_count, w.venue_id,
       CAST(COALESCE(v.chain, '') AS TEXT) AS venue_chain,
       CAST(COALESCE(v.name, w.venue_name_at_watch, '') AS TEXT) AS venue_name
FROM watch_entries w
LEFT JOIN venues v ON v.id = w.venue_id
WHERE w.user_id = sqlc.arg(user_id) AND (w.venue_kind = 'theater' OR w.subscription_id IS NOT NULL)
ORDER BY w.watched_on, w.id;
