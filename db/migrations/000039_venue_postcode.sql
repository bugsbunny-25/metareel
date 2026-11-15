-- +goose Up
-- A cinema's postcode (ZIP, PIN, postcode): from OSM addr:postcode on import,
-- or typed in. Searchable, and it tells two same-named cinemas apart.
ALTER TABLE venues ADD COLUMN postcode TEXT;

-- +goose Down
ALTER TABLE venues DROP COLUMN postcode;
