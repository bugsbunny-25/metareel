-- +goose Up
-- The service's own page for each watch provider offer (JustWatch's
-- standardWebURL), next to TMDB's per-country watch page in link.
ALTER TABLE tmdb_watch_providers ADD COLUMN url TEXT;
-- When JustWatch offers were last looked up for the title's providers
-- (NULL = never: due once in the next titles.metadata run).
ALTER TABLE tmdb_titles ADD COLUMN offers_fetched_at DATETIME;

-- +goose Down
ALTER TABLE tmdb_titles DROP COLUMN offers_fetched_at;
ALTER TABLE tmdb_watch_providers DROP COLUMN url;
