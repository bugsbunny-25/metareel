-- +goose Up
-- The earliest Netflix premiere date of a TMDB title according to Netflix's
-- engagement reports (which give a release date for Netflix originals only):
-- set by netflix.engagement.import from report rows linked to Netflix Top 10
-- titles ('linked') or matched by a unique name and year ('name'). The origin
-- classifier counts it as a Netflix original.
ALTER TABLE tmdb_titles ADD COLUMN netflix_premiere TEXT;
ALTER TABLE tmdb_titles ADD COLUMN netflix_premiere_source TEXT;

-- +goose Down
ALTER TABLE tmdb_titles DROP COLUMN netflix_premiere_source;
ALTER TABLE tmdb_titles DROP COLUMN netflix_premiere;
