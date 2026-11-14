-- Push notifications: registered devices, what was sent, pending receipts.

-- name: UpsertUserDevice :one
-- Registers a device, or moves an existing token to this user and session
-- (a phone signed in again) and enables it again.
INSERT INTO user_devices (user_id, session_id, platform, push_provider, token, app_version)
VALUES (sqlc.arg(user_id), sqlc.narg(session_id), sqlc.arg(platform), 'expo', sqlc.arg(token), sqlc.narg(app_version))
ON CONFLICT(token) DO UPDATE SET
    user_id = excluded.user_id, session_id = excluded.session_id, platform = excluded.platform,
    app_version = excluded.app_version, last_seen_at = CURRENT_TIMESTAMP, disabled_at = NULL
RETURNING id;

-- name: ListUserDevices :many
SELECT id, platform, CAST(COALESCE(app_version, '') AS TEXT) AS app_version, created_at, last_seen_at,
    CAST(COALESCE(disabled_at, '') AS TEXT) AS disabled_at, session_id
FROM user_devices WHERE user_id = sqlc.arg(user_id) ORDER BY last_seen_at DESC, id DESC;

-- name: DeleteUserDevice :execrows
DELETE FROM user_devices WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: ListActiveDeviceTokens :many
SELECT id, token FROM user_devices WHERE user_id = sqlc.arg(user_id) AND disabled_at IS NULL ORDER BY id;

-- name: DisableUserDevice :exec
UPDATE user_devices SET disabled_at = CURRENT_TIMESTAMP WHERE id = sqlc.arg(id) AND disabled_at IS NULL;

-- name: ListArrivalWatchers :many
-- Users with the title on their watchlist and a device to notify, with their
-- preferences (JSON) for the country / services / opt-in checks.
SELECT ut.user_id, CAST(COALESCE(p.data, '{}') AS TEXT) AS preferences
FROM user_titles ut
LEFT JOIN user_preferences p ON p.user_id = ut.user_id
WHERE ut.tmdb_kind = sqlc.arg(tmdb_kind) AND ut.tmdb_id = sqlc.arg(tmdb_id) AND ut.status = 'watchlist'
  AND EXISTS (SELECT 1 FROM user_devices d WHERE d.user_id = ut.user_id AND d.disabled_at IS NULL)
ORDER BY ut.user_id;

-- name: InsertNotificationLog :execrows
-- Records a notification; 0 rows means it was already sent.
INSERT INTO notification_log (user_id, kind, ref) VALUES (sqlc.arg(user_id), sqlc.arg(kind), sqlc.arg(ref))
ON CONFLICT(user_id, kind, ref) DO NOTHING;

-- name: DeleteNotificationLog :exec
DELETE FROM notification_log WHERE user_id = sqlc.arg(user_id) AND kind = sqlc.arg(kind) AND ref = sqlc.arg(ref);

-- name: InsertPushTicket :exec
INSERT INTO push_tickets (ticket_id, device_id) VALUES (sqlc.arg(ticket_id), sqlc.arg(device_id))
ON CONFLICT(ticket_id) DO NOTHING;

-- name: ListDuePushTickets :many
-- Tickets old enough for their receipt (a datetime modifier such as '-15 minutes').
SELECT ticket_id, device_id FROM push_tickets
WHERE created_at <= datetime('now', CAST(sqlc.arg(age) AS TEXT))
ORDER BY created_at, ticket_id
LIMIT sqlc.arg(limit);

-- name: DeletePushTickets :exec
DELETE FROM push_tickets WHERE ticket_id IN (sqlc.slice(ticket_ids));

-- name: DeleteStalePushTickets :execrows
-- Receipts are kept by Expo for about a day; older tickets can't be checked.
DELETE FROM push_tickets WHERE created_at < datetime('now', '-1 day');
