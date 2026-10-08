-- +goose Up
-- API keys for the public (non-admin) API. Only a SHA-256 hash of each key
-- is stored; the key itself is shown once, when created or rotated.
CREATE TABLE api_keys (
    id            INTEGER PRIMARY KEY,
    name          TEXT NOT NULL,
    key_prefix    TEXT NOT NULL,          -- first characters, to recognise a key in the UI
    key_hash      TEXT NOT NULL UNIQUE,   -- hex SHA-256 of the full key
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    rotated_at    DATETIME,
    last_used_at  DATETIME
);

-- Runtime settings editable from the admin UI.
CREATE TABLE app_settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS app_settings;
DROP TABLE IF EXISTS api_keys;
