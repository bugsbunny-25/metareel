-- +goose Up
-- Cinema subscriptions (AMC Stubs A-List, Regal Unlimited, ODEON Limitless,
-- ...): what a user pays for one and which watches it covered, for the
-- savings analysis. A covered watch keeps price_minor as what was paid on top
-- (an upcharge, fees) and face_value_minor as what the ticket would have cost.
CREATE TABLE cinema_subscriptions (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan        TEXT NOT NULL,             -- e.g. AMC Stubs A-List
    chain       TEXT,                      -- matched against a venue's chain or name, e.g. AMC
    price_minor INTEGER NOT NULL,          -- per period, minor units
    currency    TEXT NOT NULL,             -- ISO 4217
    period      TEXT NOT NULL DEFAULT 'month', -- month | year (validated in Go)
    started_on  TEXT NOT NULL,             -- YYYY-MM-DD, the first billing day
    ended_on    TEXT,                      -- YYYY-MM-DD, null while active
    note        TEXT,
    created_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_cinema_subscriptions_user ON cinema_subscriptions(user_id);

ALTER TABLE watch_entries ADD COLUMN subscription_id INTEGER REFERENCES cinema_subscriptions(id) ON DELETE SET NULL;
ALTER TABLE watch_entries ADD COLUMN face_value_minor INTEGER;
CREATE INDEX idx_watch_entries_subscription ON watch_entries(subscription_id);

-- +goose Down
DROP INDEX IF EXISTS idx_watch_entries_subscription;
ALTER TABLE watch_entries DROP COLUMN face_value_minor;
ALTER TABLE watch_entries DROP COLUMN subscription_id;
DROP INDEX IF EXISTS idx_cinema_subscriptions_user;
DROP TABLE IF EXISTS cinema_subscriptions;
