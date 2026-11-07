-- +goose Up
-- Outbound webhooks (service/webhooks.go): subscriptions and their
-- deliveries. Events come from chart_snapshots and watch_provider_events,
-- followed by cursors in app_settings (webhooks.cursor.*).
CREATE TABLE webhooks (
    id               INTEGER PRIMARY KEY,
    name             TEXT NOT NULL,
    url              TEXT NOT NULL,
    secret           TEXT NOT NULL,             -- HMAC-SHA256 key for signatures; returned only when created / rotated
    events           TEXT NOT NULL,             -- JSON list of event types; ["*"] = every type
    enabled          BOOLEAN NOT NULL DEFAULT 1,
    last_delivery_at DATETIME,
    last_status      TEXT,                      -- delivered | failed (last attempt)
    last_error       TEXT,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE webhook_deliveries (
    id               INTEGER PRIMARY KEY,
    webhook_id       INTEGER NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_id         TEXT NOT NULL,             -- stable per event, so an event is queued once per webhook
    event_type       TEXT NOT NULL,
    payload          TEXT NOT NULL,             -- the JSON body
    status           TEXT NOT NULL DEFAULT 'pending', -- pending | delivered | failed (validated in Go)
    attempts         INTEGER NOT NULL DEFAULT 0,
    next_attempt_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_status_code INTEGER,
    last_error       TEXT,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    delivered_at     DATETIME,
    UNIQUE(webhook_id, event_id)
);
CREATE INDEX idx_webhook_deliveries_due ON webhook_deliveries(status, next_attempt_at);
CREATE INDEX idx_webhook_deliveries_webhook ON webhook_deliveries(webhook_id, id);
CREATE INDEX idx_chart_snapshots_changed_at ON chart_snapshots(changed_at);

-- +goose Down
DROP INDEX IF EXISTS idx_chart_snapshots_changed_at;
DROP INDEX IF EXISTS idx_webhook_deliveries_webhook;
DROP INDEX IF EXISTS idx_webhook_deliveries_due;
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhooks;
DELETE FROM app_settings WHERE key LIKE 'webhooks.cursor.%';
