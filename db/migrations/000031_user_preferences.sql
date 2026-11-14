-- +goose Up
-- A user's preferences, as one JSON object validated in Go (country,
-- provider, currency, screen_unit, stale_days), so a new preference needs
-- no table rebuild. They follow the user to every device; things that
-- belong to one device (the theme) stay in the browser.
CREATE TABLE user_preferences (
    user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    data       TEXT NOT NULL DEFAULT '{}',
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE user_preferences;
