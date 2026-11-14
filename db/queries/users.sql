-- User accounts and their sign-in sessions.

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: ListUsers :many
SELECT id, username, password_hash, role, disabled, created_at, updated_at, last_login_at
FROM users
ORDER BY id;

-- name: GetUserByUsername :one
SELECT id, username, password_hash, role, disabled, created_at, updated_at, last_login_at
FROM users
WHERE username = sqlc.arg(username);

-- name: CreateUser :one
INSERT INTO users (username, password_hash, role)
VALUES (sqlc.arg(username), sqlc.arg(password_hash), sqlc.arg(role))
RETURNING id, username, password_hash, role, disabled, created_at, updated_at, last_login_at;

-- name: SetUserPassword :execrows
UPDATE users SET password_hash = sqlc.arg(password_hash), updated_at = CURRENT_TIMESTAMP
WHERE username = sqlc.arg(username);

-- name: SetUserRole :execrows
UPDATE users SET role = sqlc.arg(role), updated_at = CURRENT_TIMESTAMP
WHERE username = sqlc.arg(username);

-- name: SetUserDisabled :execrows
UPDATE users SET disabled = sqlc.arg(disabled), updated_at = CURRENT_TIMESTAMP
WHERE username = sqlc.arg(username);

-- name: RecordUserLogin :exec
UPDATE users SET last_login_at = CURRENT_TIMESTAMP WHERE id = sqlc.arg(id);

-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, expires_at, user_agent, ip, device_name, remember_device)
VALUES (sqlc.arg(user_id), sqlc.arg(token_hash), datetime('now', CAST(sqlc.arg(ttl) AS TEXT)),
        sqlc.arg(user_agent), sqlc.arg(ip), sqlc.arg(device_name), sqlc.arg(remember_device))
RETURNING id, expires_at;

-- GetActiveSession returns a session that has not expired, was used within
-- its idle window and belongs to an enabled user. idle and device_idle are
-- negative modifiers (e.g. '-604800 seconds'); device_idle applies to
-- "remember this device" sessions.
-- name: GetActiveSession :one
SELECT s.id, s.user_id, s.expires_at, u.username, u.role
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = sqlc.arg(token_hash)
  AND u.disabled = 0
  AND s.expires_at > datetime('now')
  AND s.last_seen_at > datetime('now', CASE WHEN s.remember_device THEN CAST(sqlc.arg(device_idle) AS TEXT)
                                            ELSE CAST(sqlc.arg(idle) AS TEXT) END);

-- GetActiveSessionDigest returns the token digest of an active session by
-- id (same rules as GetActiveSession), for the local-URL probe.
-- name: GetActiveSessionDigest :one
SELECT s.token_hash
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.id = sqlc.arg(id)
  AND u.disabled = 0
  AND s.expires_at > datetime('now')
  AND s.last_seen_at > datetime('now', CASE WHEN s.remember_device THEN CAST(sqlc.arg(device_idle) AS TEXT)
                                            ELSE CAST(sqlc.arg(idle) AS TEXT) END);

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = CURRENT_TIMESTAMP WHERE id = sqlc.arg(id);

-- name: DeleteSessionByHash :one
DELETE FROM sessions WHERE token_hash = sqlc.arg(token_hash)
RETURNING id, user_id;

-- name: DeleteUserSessions :execrows
DELETE FROM sessions
WHERE user_id = (SELECT id FROM users WHERE username = sqlc.arg(username));

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions
WHERE expires_at <= datetime('now')
   OR last_seen_at <= datetime('now', CASE WHEN remember_device THEN CAST(sqlc.arg(device_idle) AS TEXT)
                                           ELSE CAST(sqlc.arg(idle) AS TEXT) END);

-- name: CountEnabledAdmins :one
SELECT COUNT(*) FROM users WHERE role = 'admin' AND disabled = 0;

-- name: DeleteUser :execrows
DELETE FROM users WHERE username = sqlc.arg(username);

-- ListActiveSessions lists unexpired sessions used within their idle window
-- (see GetActiveSession), newest use first.
-- name: ListActiveSessions :many
SELECT s.id, s.user_id, u.username, s.created_at, s.last_seen_at, s.expires_at, s.user_agent, s.ip,
       s.device_name, s.remember_device
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.expires_at > datetime('now')
  AND s.last_seen_at > datetime('now', CASE WHEN s.remember_device THEN CAST(sqlc.arg(device_idle) AS TEXT)
                                            ELSE CAST(sqlc.arg(idle) AS TEXT) END)
ORDER BY s.last_seen_at DESC, s.id DESC;

-- name: DeleteSessionByID :one
DELETE FROM sessions WHERE id = sqlc.arg(id)
RETURNING token_hash;

-- name: DeleteUserSessionsExcept :execrows
DELETE FROM sessions
WHERE user_id = (SELECT u.id FROM users u WHERE u.username = sqlc.arg(username))
  AND sessions.id <> sqlc.arg(keep_id);
