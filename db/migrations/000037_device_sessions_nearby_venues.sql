-- +goose Up
-- Phone sign-ins (docs/mobile-app-plan.md): a session can carry a device
-- name the user recognises in Settings -> Sessions, and "remember this
-- device" sessions get their own, longer lifetime and idle timeout
-- (AUTH_DEVICE_SESSION_TTL / AUTH_DEVICE_SESSION_IDLE_TIMEOUT).
ALTER TABLE sessions ADD COLUMN device_name TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN remember_device BOOLEAN NOT NULL DEFAULT 0;

-- Cinemas near a point: a latitude range scan, then longitude and the exact
-- distance in Go.
CREATE INDEX idx_venues_lat_lon ON venues(latitude, longitude);

-- +goose Down
DROP INDEX IF EXISTS idx_venues_lat_lon;
ALTER TABLE sessions DROP COLUMN remember_device;
ALTER TABLE sessions DROP COLUMN device_name;
