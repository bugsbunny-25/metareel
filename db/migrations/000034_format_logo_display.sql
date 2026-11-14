-- +goose Up
-- How a format's logo is shown: 'original' colours, or 'mono' (drawn white by
-- the client) for dark logos that would vanish on the apps' dark plate. The
-- uploaded file is never changed; this is a display hint served with it.
ALTER TABLE watch_formats ADD COLUMN logo_display TEXT NOT NULL DEFAULT 'original';

-- +goose Down
ALTER TABLE watch_formats DROP COLUMN logo_display;
