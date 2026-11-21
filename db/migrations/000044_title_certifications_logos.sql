-- +goose Up
-- Age certifications per country (movies: TMDB release_dates; TV:
-- content_ratings), JSON {"US": "PG-13"}, and the title's logo image (TMDB
-- images, English first, else the original language). Filled by the next
-- details refresh; NULL until then.
ALTER TABLE tmdb_titles ADD COLUMN certifications TEXT;
ALTER TABLE tmdb_titles ADD COLUMN logo_path TEXT;

-- +goose Down
ALTER TABLE tmdb_titles DROP COLUMN logo_path;
ALTER TABLE tmdb_titles DROP COLUMN certifications;
