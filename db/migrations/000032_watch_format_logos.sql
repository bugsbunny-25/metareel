-- +goose Up
-- Logos for the premium formats, uploaded by admins and shown in the apps.
-- The image lives in the database so it is backed up with everything else;
-- logo_version (a content hash) goes into the logo URL, so clients can cache
-- a logo for good and a new upload is a new URL.
ALTER TABLE watch_formats ADD COLUMN logo_version TEXT;

CREATE TABLE watch_format_logos (
    format_id    INTEGER PRIMARY KEY REFERENCES watch_formats(id) ON DELETE CASCADE,
    content_type TEXT NOT NULL,
    data         BLOB NOT NULL,
    updated_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE watch_format_logos;
ALTER TABLE watch_formats DROP COLUMN logo_version;
