-- +goose Up
-- The average colour of a TMDB poster (#rrggbb), for artwork-tinted hero
-- surfaces in the mobile app (docs/mobile-app-plan.md, B5). Keyed by poster
-- path: TMDB never changes the image behind a path, so an entry never goes
-- stale. Computed on first use from the smallest poster size.
CREATE TABLE poster_colors (
    poster_path TEXT PRIMARY KEY,
    color       TEXT NOT NULL,
    computed_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) WITHOUT ROWID;

-- +goose Down
DROP TABLE IF EXISTS poster_colors;
