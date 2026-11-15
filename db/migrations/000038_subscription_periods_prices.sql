-- +goose Up
-- Cinema subscriptions over time. A plan can be cancelled and taken up again
-- (each run is a period; the gaps between them are not billed) and its price
-- can change (each price applies from its date until the next). The
-- started_on, ended_on and price_minor columns on cinema_subscriptions stay as
-- a summary kept in step by the service: first start, last end (null while a
-- period is open) and the price in effect today.
CREATE TABLE cinema_subscription_periods (
    id              INTEGER PRIMARY KEY,
    subscription_id INTEGER NOT NULL REFERENCES cinema_subscriptions(id) ON DELETE CASCADE,
    started_on      TEXT NOT NULL,  -- YYYY-MM-DD, the period's first billing day
    ended_on        TEXT            -- YYYY-MM-DD, null while it runs (only the last period)
);
CREATE INDEX idx_cinema_subscription_periods ON cinema_subscription_periods(subscription_id, started_on);

CREATE TABLE cinema_subscription_prices (
    id              INTEGER PRIMARY KEY,
    subscription_id INTEGER NOT NULL REFERENCES cinema_subscriptions(id) ON DELETE CASCADE,
    effective_on    TEXT NOT NULL,  -- YYYY-MM-DD; the earliest price also covers bills before it
    price_minor     INTEGER NOT NULL,
    UNIQUE (subscription_id, effective_on)
);

INSERT INTO cinema_subscription_periods (subscription_id, started_on, ended_on)
SELECT id, started_on, ended_on FROM cinema_subscriptions;
INSERT INTO cinema_subscription_prices (subscription_id, effective_on, price_minor)
SELECT id, started_on, price_minor FROM cinema_subscriptions;

-- +goose Down
DROP TABLE IF EXISTS cinema_subscription_prices;
DROP INDEX IF EXISTS idx_cinema_subscription_periods;
DROP TABLE IF EXISTS cinema_subscription_periods;
