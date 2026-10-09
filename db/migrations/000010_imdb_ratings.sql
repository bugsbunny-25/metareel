-- +goose Up
-- IMDb ratings from IMDb's daily non-commercial dataset (title.ratings.tsv.gz,
-- https://developer.imdb.com/non-commercial-datasets/; personal and
-- non-commercial use only). By default only titles metareel knows are kept.
CREATE TABLE imdb_ratings (
    imdb_id        TEXT PRIMARY KEY,        -- e.g. tt0137523
    average_rating REAL NOT NULL,
    num_votes      INTEGER NOT NULL,
    imported_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) WITHOUT ROWID;

-- +goose Down
DROP TABLE IF EXISTS imdb_ratings;
