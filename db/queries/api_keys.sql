-- API keys for the public API, and admin-editable settings.

-- name: ListAPIKeys :many
SELECT id, name, key_prefix, key_hash, created_at, rotated_at, last_used_at
FROM api_keys
ORDER BY id;

-- name: CreateAPIKey :one
INSERT INTO api_keys (name, key_prefix, key_hash)
VALUES (?, ?, ?)
RETURNING id, name, key_prefix, key_hash, created_at, rotated_at, last_used_at;

-- name: RotateAPIKey :one
UPDATE api_keys
SET key_prefix = ?, key_hash = ?, rotated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING id, name, key_prefix, key_hash, created_at, rotated_at, last_used_at;

-- name: RenameAPIKey :one
UPDATE api_keys
SET name = ?
WHERE id = ?
RETURNING id, name, key_prefix, key_hash, created_at, rotated_at, last_used_at;

-- name: DeleteAPIKey :execrows
DELETE FROM api_keys WHERE id = ?;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: ListAppSettings :many
SELECT key, value, updated_at FROM app_settings;

-- name: UpsertAppSetting :exec
INSERT INTO app_settings (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP;
