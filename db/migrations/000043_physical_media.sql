-- +goose Up
-- Physical media: the discs users own.
--
-- Two levels, like venues and watches:
--
--   disc_releases  an edition as it was published (a barcode, a format, its
--                  discs, specs, special features and artwork). Shared
--                  reference data: the second user to scan a barcode gets the
--                  release the first one filled in.
--   disc_copies    one user's copy of a release: what they paid, where, when
--                  and in what condition. A user may own two copies.
--
-- A box set is a release whose parts are releases too (parent_id): the outer
-- box has its own artwork and barcode, and each part (a Steelbook, a keepcase,
-- a season) has its own discs, specs and artwork. A copy always points at the
-- top-level release. A simple box set needs no parts: one release can carry
-- several titles, and each disc says which title it holds.
--
-- What is on a release is linked to TMDB (disc_release_titles), never to the
-- chart domain, for the same reason as the tracking tables.
--
-- Formats, packaging and per-disc specs are validated in Go, so a new value
-- needs no table rebuild. discs and special_features are JSON documents:
-- they are always read and written with their release and never queried on
-- their own.
CREATE TABLE disc_releases (
    id               INTEGER PRIMARY KEY,
    parent_id        INTEGER REFERENCES disc_releases(id) ON DELETE CASCADE,
    position         INTEGER NOT NULL DEFAULT 0,      -- order inside the parent box
    barcode          TEXT,                            -- GTIN-13 digits (a UPC-A gets a leading 0)
    title            TEXT NOT NULL,                   -- as printed on the release
    edition          TEXT NOT NULL DEFAULT '',        -- Steelbook, Collector's Edition, ...
    media_format     TEXT NOT NULL,                   -- uhd_bluray | bluray | bluray_3d | dvd | hd_dvd | other
    packaging        TEXT NOT NULL DEFAULT '',        -- keepcase | steelbook | digipak | digibook | mediabook | box_set | other
    has_slipcover    INTEGER NOT NULL DEFAULT 0,
    region           TEXT NOT NULL DEFAULT '',        -- A / B / C / ABC, 1..6, free
    country          TEXT,                            -- ISO 3166 code of the market
    studio           TEXT NOT NULL DEFAULT '',        -- distributor
    label            TEXT,                            -- boutique label: a watch_formats slug (criterion, arrow, ...)
    release_date     TEXT,                            -- YYYY-MM-DD
    discs            TEXT NOT NULL DEFAULT '[]',      -- JSON: per-disc specs
    special_features TEXT NOT NULL DEFAULT '[]',      -- JSON: [{title, minutes, disc}]
    notes            TEXT,
    source           TEXT NOT NULL DEFAULT 'manual',  -- manual | upcitemdb
    created_by       INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at       TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_disc_releases_barcode ON disc_releases(barcode) WHERE barcode IS NOT NULL;
CREATE INDEX idx_disc_releases_parent ON disc_releases(parent_id, position) WHERE parent_id IS NOT NULL;

-- What is on a release, in order: one movie, a trilogy, a show's seasons.
-- season_number NULL means the whole movie or show.
CREATE TABLE disc_release_titles (
    release_id    INTEGER NOT NULL REFERENCES disc_releases(id) ON DELETE CASCADE,
    position      INTEGER NOT NULL,
    tmdb_kind     TEXT NOT NULL,                      -- movie | tv
    tmdb_id       TEXT NOT NULL,
    season_number INTEGER,
    PRIMARY KEY (release_id, position)
) WITHOUT ROWID;
CREATE INDEX idx_disc_release_titles_tmdb ON disc_release_titles(tmdb_kind, tmdb_id);

-- Artwork and photos of a release. The files live under MEDIA_DIR, named by
-- the content hash of the stored (re-encoded) file, so a URL never changes
-- meaning and can be cached for good. Every upload is re-encoded, which also
-- drops a phone photo's EXIF (its GPS position).
CREATE TABLE disc_release_images (
    id          INTEGER PRIMARY KEY,
    release_id  INTEGER NOT NULL REFERENCES disc_releases(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,                        -- front | back | slip_front | slip_back | spine | disc | booklet | other
    position    INTEGER NOT NULL DEFAULT 0,
    sha256      TEXT NOT NULL,                        -- of the stored, re-encoded file; also its name
    content_type TEXT NOT NULL DEFAULT 'image/jpeg', -- image/png when the picture has transparency
    width       INTEGER NOT NULL,
    height      INTEGER NOT NULL,
    bytes       INTEGER NOT NULL,
    source      TEXT NOT NULL,                        -- upload | upcitemdb | tmdb
    source_url  TEXT,
    uploaded_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_disc_release_images_release ON disc_release_images(release_id, kind, position);
CREATE INDEX idx_disc_release_images_sha ON disc_release_images(sha256);

-- A user's copy. Money is integer minor units with its ISO 4217 currency,
-- like ticket prices.
CREATE TABLE disc_copies (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    release_id   INTEGER NOT NULL REFERENCES disc_releases(id),
    purchased_on TEXT,                                -- YYYY-MM-DD
    price_minor  INTEGER,
    currency     TEXT,
    retailer     TEXT NOT NULL DEFAULT '',
    condition    TEXT NOT NULL DEFAULT '',            -- '' | sealed | new | used
    notes        TEXT,
    created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_disc_copies_user ON disc_copies(user_id, id);
CREATE INDEX idx_disc_copies_release ON disc_copies(release_id);

-- What a barcode lookup service answered, so a scan never spends the daily
-- quota twice. found = 0 is a cached miss.
CREATE TABLE disc_barcode_lookups (
    barcode    TEXT NOT NULL,
    provider   TEXT NOT NULL,                         -- upcitemdb
    found      INTEGER NOT NULL,
    payload    TEXT NOT NULL DEFAULT '{}',
    fetched_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (barcode, provider)
) WITHOUT ROWID;

-- +goose Down
DROP TABLE IF EXISTS disc_barcode_lookups;
DROP TABLE IF EXISTS disc_copies;
DROP TABLE IF EXISTS disc_release_images;
DROP TABLE IF EXISTS disc_release_titles;
DROP TABLE IF EXISTS disc_releases;
