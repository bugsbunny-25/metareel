-- Outbound webhooks: subscriptions, the event sources they follow and the
-- delivery queue.

-- name: ListWebhooks :many
SELECT * FROM webhooks ORDER BY id;

-- name: GetWebhook :one
SELECT * FROM webhooks WHERE id = ?;

-- name: CreateWebhook :one
INSERT INTO webhooks (name, url, secret, events, enabled)
VALUES (sqlc.arg(name), sqlc.arg(url), sqlc.arg(secret), sqlc.arg(events), sqlc.arg(enabled))
RETURNING *;

-- name: UpdateWebhook :one
UPDATE webhooks SET name = sqlc.arg(name), url = sqlc.arg(url), events = sqlc.arg(events), enabled = sqlc.arg(enabled),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SetWebhookSecret :one
UPDATE webhooks SET secret = sqlc.arg(secret), updated_at = CURRENT_TIMESTAMP WHERE id = sqlc.arg(id) RETURNING *;

-- name: DeleteWebhook :execrows
DELETE FROM webhooks WHERE id = ?;

-- name: ListEnabledWebhooks :many
SELECT * FROM webhooks WHERE enabled = 1 ORDER BY id;

-- name: QueueWebhookDelivery :execrows
-- Queues an event for a webhook once (a repeated event_id is ignored).
INSERT INTO webhook_deliveries (webhook_id, event_id, event_type, payload)
VALUES (sqlc.arg(webhook_id), sqlc.arg(event_id), sqlc.arg(event_type), sqlc.arg(payload))
ON CONFLICT(webhook_id, event_id) DO NOTHING;

-- name: InsertWebhookDelivery :one
-- A delivery recorded as it happens (test pings).
INSERT INTO webhook_deliveries (webhook_id, event_id, event_type, payload, status, attempts, last_status_code, last_error, delivered_at)
VALUES (sqlc.arg(webhook_id), sqlc.arg(event_id), sqlc.arg(event_type), sqlc.arg(payload), sqlc.arg(status), 1,
        sqlc.narg(last_status_code), sqlc.narg(last_error),
        CASE WHEN sqlc.arg(status) = 'delivered' THEN CURRENT_TIMESTAMP END)
RETURNING *;

-- name: ListDueWebhookDeliveries :many
-- Pending deliveries whose next attempt is due, of enabled webhooks.
SELECT d.id, d.webhook_id, d.event_id, d.event_type, d.payload, d.attempts, w.url, w.secret
FROM webhook_deliveries d
JOIN webhooks w ON w.id = d.webhook_id AND w.enabled = 1
WHERE d.status = 'pending' AND d.next_attempt_at <= datetime('now')
ORDER BY d.next_attempt_at, d.id
LIMIT sqlc.arg(limit);

-- name: MarkWebhookDelivered :exec
UPDATE webhook_deliveries SET status = 'delivered', attempts = attempts + 1, last_status_code = sqlc.narg(status_code),
    last_error = NULL, delivered_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: MarkWebhookAttemptFailed :exec
-- A failed attempt: retried after the backoff (a datetime modifier such as
-- '+5 minutes'), or failed for good when final = 1.
UPDATE webhook_deliveries SET attempts = attempts + 1, last_status_code = sqlc.narg(status_code), last_error = sqlc.arg(error),
    status = CASE WHEN CAST(sqlc.arg(final) AS INTEGER) = 1 THEN 'failed' ELSE 'pending' END,
    next_attempt_at = datetime('now', CAST(sqlc.arg(backoff) AS TEXT))
WHERE id = sqlc.arg(id);

-- name: RecordWebhookResult :exec
UPDATE webhooks SET last_delivery_at = CURRENT_TIMESTAMP, last_status = sqlc.arg(status), last_error = sqlc.narg(error)
WHERE id = sqlc.arg(id);

-- name: RetryWebhookDelivery :execrows
UPDATE webhook_deliveries SET status = 'pending', next_attempt_at = CURRENT_TIMESTAMP WHERE id = ? AND status <> 'pending';

-- name: ListWebhookDeliveries :many
SELECT id, webhook_id, event_id, event_type, status, attempts,
       CAST(next_attempt_at AS TEXT) AS next_attempt_at, last_status_code, last_error,
       CAST(created_at AS TEXT) AS created_at, CAST(COALESCE(delivered_at, '') AS TEXT) AS delivered_at
FROM webhook_deliveries
WHERE webhook_id = sqlc.arg(webhook_id)
ORDER BY id DESC
LIMIT sqlc.arg(limit);

-- name: GetWebhookDeliveryPayload :one
SELECT payload FROM webhook_deliveries WHERE id = ?;

-- name: CountWebhookDeliveriesByStatus :many
SELECT webhook_id, status, COUNT(*) AS n FROM webhook_deliveries GROUP BY webhook_id, status;

-- name: PruneWebhookDeliveries :execrows
-- Deletes finished deliveries older than a window ('-30 days').
DELETE FROM webhook_deliveries
WHERE status <> 'pending' AND created_at < datetime('now', CAST(sqlc.arg(window) AS TEXT));

-- name: ListChartsChangedAfter :many
-- Charts stored or changed after a position (changed_at
-- 'YYYY-MM-DD HH:MM:SS' UTC, then row id), oldest first: keyset paging, so
-- many charts sharing one changed_at are read page by page.
SELECT id, CAST(ranked_on AS TEXT) AS ranked_on, country, streaming_provider, category, entry_count,
       CAST(changed_at AS TEXT) AS changed_at
FROM chart_snapshots
WHERE changed_at >= CAST(sqlc.arg(after_at) AS TEXT)
  AND (changed_at > CAST(sqlc.arg(after_at) AS TEXT) OR id > sqlc.arg(after_id))
ORDER BY changed_at, id
LIMIT sqlc.arg(limit);

-- name: MaxChartChangedAt :one
SELECT CAST(COALESCE(MAX(changed_at), '') AS TEXT) AS changed_at, CAST(COALESCE(MAX(id), 0) AS INTEGER) AS max_id
FROM chart_snapshots;

-- name: ListOfferEventsAfter :many
-- Watch provider events after an id, oldest first, with the TMDB title.
SELECT e.id, e.tmdb_kind, e.tmdb_id, e.country, e.monetization, e.provider_id, e.provider_name, e.event,
       CAST(COALESCE(e.previous_check_at, '') AS TEXT) AS previous_check_at, CAST(e.observed_at AS TEXT) AS observed_at,
       CAST(COALESCE(m.title, '') AS TEXT) AS title
FROM watch_provider_events e
LEFT JOIN tmdb_titles m ON m.tmdb_kind = e.tmdb_kind AND m.tmdb_id = e.tmdb_id
WHERE e.id > sqlc.arg(after_id)
ORDER BY e.id
LIMIT sqlc.arg(limit);

-- name: MaxOfferEventID :one
SELECT CAST(COALESCE(MAX(id), 0) AS INTEGER) AS id FROM watch_provider_events;
