-- +goose Up
-- Phones registered for push notifications (the mobile app). A device
-- belongs to the session that registered it, so signing out (deleting the
-- session) removes it. Tokens are Expo push tokens; never logged.
CREATE TABLE user_devices (
    id            INTEGER PRIMARY KEY,
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id    INTEGER REFERENCES sessions(id) ON DELETE CASCADE,
    platform      TEXT NOT NULL,                  -- ios | android (validated in Go)
    push_provider TEXT NOT NULL DEFAULT 'expo',
    token         TEXT NOT NULL UNIQUE,
    app_version   TEXT,
    created_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    disabled_at   TEXT                            -- set when the push service says the token is gone
);
CREATE INDEX idx_user_devices_user ON user_devices(user_id);
CREATE INDEX idx_user_devices_session ON user_devices(session_id);

-- One row per notification sent, so each is sent once (kind + ref, e.g.
-- watchlist_arrival + movie:603).
CREATE TABLE notification_log (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,
    ref        TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, kind, ref)
);

-- Push tickets waiting for their receipt (Expo publishes receipts after
-- ~15 minutes; a DeviceNotRegistered receipt disables the device).
CREATE TABLE push_tickets (
    ticket_id  TEXT PRIMARY KEY,
    device_id  INTEGER NOT NULL REFERENCES user_devices(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Hourly; does nothing unless PUSH_ENABLED is set.
INSERT INTO task_schedules (task_type, name, enabled, max_retries, request_delay_seconds)
VALUES ('notifications.dispatch', 'Push notifications', 1, 1, 0)
ON CONFLICT(name) DO NOTHING;
INSERT INTO task_schedule_run_times (schedule_id, run_time_utc)
WITH RECURSIVE h(n) AS (SELECT 0 UNION ALL SELECT n + 1 FROM h WHERE n < 23)
SELECT s.id, printf('%02d:15', h.n) FROM task_schedules s, h WHERE s.name = 'Push notifications'
ON CONFLICT(schedule_id, run_time_utc) DO NOTHING;

-- +goose Down
DELETE FROM task_schedules WHERE task_type = 'notifications.dispatch';
DROP TABLE push_tickets;
DROP TABLE notification_log;
DROP TABLE user_devices;
