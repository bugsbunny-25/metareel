-- +goose Up
-- Distributors of a movie on Wikidata (P750, English labels as a JSON list),
-- one of the signals that tell a service's original films from licensed
-- ones (see service/origins.go). Filled by the titles.metadata job.
ALTER TABLE tmdb_titles ADD COLUMN distributors TEXT;
ALTER TABLE tmdb_titles ADD COLUMN distributors_fetched_at DATETIME;

-- +goose Down
ALTER TABLE tmdb_titles DROP COLUMN distributors_fetched_at;
ALTER TABLE tmdb_titles DROP COLUMN distributors;
