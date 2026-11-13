-- +goose Up
-- Where and how a watch happened: the premium-format taxonomy admins
-- maintain, the cinemas and screens a watch can point at, and the detail
-- columns on a watch itself.
--
-- Enums here (category, status, screening_type, media, hdr, ...) are
-- validated in Go rather than by CHECK constraints, so adding a value needs
-- no table rebuild and a column can still be dropped on the way down.

-- Premium formats, maintained by admins and referenced by watches for good.
-- Rows are deactivated, never deleted, so an old watch keeps its meaning.
-- category separates the independent axes a cinema format mixes: the brand,
-- the projection, the sound, the motion rig and 2D / 3D. Keeping them apart
-- is what makes "all my 70mm screenings" or "IMAX hours" answerable.
CREATE TABLE watch_formats (
    id         INTEGER PRIMARY KEY,
    slug       TEXT NOT NULL UNIQUE COLLATE NOCASE,
    label      TEXT NOT NULL,
    category   TEXT NOT NULL,            -- brand | projection | sound | motion | dimension
    note       TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    active     INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_watch_formats_active ON watch_formats(active, sort_order);

INSERT INTO watch_formats (slug, label, category, sort_order) VALUES
    ('imax',          'IMAX',            'brand',      10),
    ('imax-3d',       'IMAX 3D',         'brand',      11),
    ('imax-laser',    'IMAX with Laser', 'brand',      12),
    ('imax-70mm',     'IMAX 70mm',       'brand',      13),
    ('dolby-cinema',  'Dolby Cinema',    'brand',      20),
    ('4dx',           '4DX',             'motion',     30),
    ('d-box',         'D-BOX',           'motion',     31),
    ('screenx',       'ScreenX',         'brand',      40),
    ('cinemark-xd',   'Cinemark XD',     'brand',      41),
    ('pvr-playhouse', 'PVR Playhouse',   'brand',      42),
    ('inox-insignia', 'INOX Insignia',   'brand',      43),
    ('plf',           'Premium large format', 'brand', 44),
    ('reald-3d',      'RealD 3D',        'dimension',  50),
    ('dolby-atmos',   'Dolby Atmos',     'sound',      60),
    ('dts-x',         'DTS:X',           'sound',      61),
    ('70mm',          '70mm film',       'projection', 70),
    ('35mm',          '35mm film',       'projection', 71),
    ('laser',         'Laser projection','projection', 72),
    ('hfr',           'High frame rate', 'projection', 73);

CREATE TABLE watch_entry_formats (
    entry_id  INTEGER NOT NULL REFERENCES watch_entries(id) ON DELETE CASCADE,
    format_id INTEGER NOT NULL REFERENCES watch_formats(id) ON DELETE RESTRICT,
    PRIMARY KEY (entry_id, format_id)
);
CREATE INDEX idx_watch_entry_formats_format ON watch_entry_formats(format_id);

-- Cinemas. Shared reference data rather than per-user rows: a cinema is a
-- place, not an opinion, and this instance has a handful of trusted users.
-- A venue is never silently rewritten by history: opened_on / closed_on and
-- replaced_by_venue_id record what happened to it, aliases record what it
-- used to be called, and every watch keeps a snapshot of the name and city
-- as they were on the night.
CREATE TABLE venues (
    id                   INTEGER PRIMARY KEY,
    name                 TEXT NOT NULL,
    chain                TEXT,
    address              TEXT,
    city                 TEXT,
    region               TEXT,
    country              TEXT,            -- ISO 3166-1 alpha-2
    latitude             REAL,
    longitude            REAL,
    timezone             TEXT,            -- IANA zone, the default for watches here
    opened_on            TEXT,            -- YYYY-MM-DD
    closed_on            TEXT,
    status               TEXT NOT NULL DEFAULT 'open', -- open | closed | renamed | rebuilt
    replaced_by_venue_id INTEGER REFERENCES venues(id) ON DELETE SET NULL,
    note                 TEXT,
    created_by           INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at           TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_venues_country_city ON venues(country, city);
CREATE INDEX idx_venues_name ON venues(name COLLATE NOCASE);

-- What a venue used to be called, and when.
CREATE TABLE venue_aliases (
    id        INTEGER PRIMARY KEY,
    venue_id  INTEGER NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    name      TEXT NOT NULL,
    from_date TEXT,
    to_date   TEXT
);
CREATE INDEX idx_venue_aliases_venue ON venue_aliases(venue_id);

-- A venue's id in an outside source, the same pattern the title mapping uses.
-- Google place IDs may be stored indefinitely; the rest of Google's place
-- data may not, which is why only the id lives here.
CREATE TABLE venue_refs (
    venue_id INTEGER NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    source   TEXT NOT NULL,   -- google_place_id | osm_id | wikidata_id | fsq_id | overture_id
    ref      TEXT NOT NULL,
    PRIMARY KEY (venue_id, source),
    UNIQUE (source, ref)
);

-- One auditorium. Geometry belongs to the screen, not to each visit; a
-- screen that is retrofitted gets effective dates rather than a rewrite.
CREATE TABLE venue_screens (
    id             INTEGER PRIMARY KEY,
    venue_id       INTEGER NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    name           TEXT NOT NULL DEFAULT '',
    number         INTEGER,
    seats          INTEGER,
    width          REAL,            -- as entered, in dim_unit
    height         REAL,
    dim_unit       TEXT,            -- m | ft
    width_m        REAL,            -- normalised, so screens compare
    height_m       REAL,
    aspect_ratio   REAL,            -- e.g. 1.9 for 1.90:1
    effective_from TEXT,
    effective_to   TEXT,
    note           TEXT,
    created_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_venue_screens_venue ON venue_screens(venue_id);

-- Where and how the watch happened.
ALTER TABLE watch_entries ADD COLUMN venue_id INTEGER REFERENCES venues(id) ON DELETE SET NULL;
ALTER TABLE watch_entries ADD COLUMN screen_id INTEGER REFERENCES venue_screens(id) ON DELETE SET NULL;
-- The venue as it was that night, so a later rename or closure cannot
-- rewrite what the diary says you went to.
ALTER TABLE watch_entries ADD COLUMN venue_name_at_watch TEXT;
ALTER TABLE watch_entries ADD COLUMN venue_city_at_watch TEXT;
ALTER TABLE watch_entries ADD COLUMN seat TEXT;
ALTER TABLE watch_entries ADD COLUMN screening_type TEXT;  -- regular | preview | premiere | festival | press | rerelease | marathon
ALTER TABLE watch_entries ADD COLUMN event_name TEXT;
ALTER TABLE watch_entries ADD COLUMN with_qa INTEGER NOT NULL DEFAULT 0;

-- Money, in minor units so nothing is ever a float.
ALTER TABLE watch_entries ADD COLUMN price_minor INTEGER;
ALTER TABLE watch_entries ADD COLUMN fees_minor INTEGER;
ALTER TABLE watch_entries ADD COLUMN concessions_minor INTEGER;
ALTER TABLE watch_entries ADD COLUMN currency TEXT;        -- ISO 4217
ALTER TABLE watch_entries ADD COLUMN tickets_count INTEGER;
ALTER TABLE watch_entries ADD COLUMN booking_source TEXT;

-- Streaming and physical media.
ALTER TABLE watch_entries ADD COLUMN provider_id INTEGER;  -- TMDB / JustWatch provider id
ALTER TABLE watch_entries ADD COLUMN provider_name TEXT;
ALTER TABLE watch_entries ADD COLUMN monetization TEXT;    -- flatrate | free | ads | rent | buy
ALTER TABLE watch_entries ADD COLUMN media TEXT;           -- bluray_4k | bluray | dvd | vhs | digital_file | 35mm | 70mm
ALTER TABLE watch_entries ADD COLUMN edition TEXT;         -- theatrical | extended | directors | final | ...
ALTER TABLE watch_entries ADD COLUMN device TEXT;

-- How it looked and sounded.
ALTER TABLE watch_entries ADD COLUMN dimension TEXT;       -- 2d | 3d | 3d_hfr
ALTER TABLE watch_entries ADD COLUMN resolution TEXT;      -- sd | hd | fhd | 4k | 8k
ALTER TABLE watch_entries ADD COLUMN hdr TEXT;             -- none | hdr10 | hdr10_plus | dolby_vision | hlg
ALTER TABLE watch_entries ADD COLUMN audio_format TEXT;
ALTER TABLE watch_entries ADD COLUMN frame_rate REAL;
ALTER TABLE watch_entries ADD COLUMN audio_language TEXT;
ALTER TABLE watch_entries ADD COLUMN subtitle_language TEXT;

-- Aspect ratios. TMDB has none of this, so it is what the viewer says it
-- was: the text as written plus a number to compare, and a flag for the
-- films that change ratio part way through.
ALTER TABLE watch_entries ADD COLUMN content_aspect_ratio_text TEXT;
ALTER TABLE watch_entries ADD COLUMN content_aspect_ratio REAL;
ALTER TABLE watch_entries ADD COLUMN content_aspect_variable INTEGER NOT NULL DEFAULT 0;
ALTER TABLE watch_entries ADD COLUMN screen_aspect_ratio REAL;
ALTER TABLE watch_entries ADD COLUMN screen_width REAL;
ALTER TABLE watch_entries ADD COLUMN screen_height REAL;
ALTER TABLE watch_entries ADD COLUMN screen_dim_unit TEXT;
ALTER TABLE watch_entries ADD COLUMN screen_width_m REAL;
ALTER TABLE watch_entries ADD COLUMN screen_height_m REAL;

CREATE INDEX idx_watch_entries_venue ON watch_entries(venue_id);

-- +goose Down
DROP INDEX IF EXISTS idx_watch_entries_venue;
ALTER TABLE watch_entries DROP COLUMN screen_height_m;
ALTER TABLE watch_entries DROP COLUMN screen_width_m;
ALTER TABLE watch_entries DROP COLUMN screen_dim_unit;
ALTER TABLE watch_entries DROP COLUMN screen_height;
ALTER TABLE watch_entries DROP COLUMN screen_width;
ALTER TABLE watch_entries DROP COLUMN screen_aspect_ratio;
ALTER TABLE watch_entries DROP COLUMN content_aspect_variable;
ALTER TABLE watch_entries DROP COLUMN content_aspect_ratio;
ALTER TABLE watch_entries DROP COLUMN content_aspect_ratio_text;
ALTER TABLE watch_entries DROP COLUMN subtitle_language;
ALTER TABLE watch_entries DROP COLUMN audio_language;
ALTER TABLE watch_entries DROP COLUMN frame_rate;
ALTER TABLE watch_entries DROP COLUMN audio_format;
ALTER TABLE watch_entries DROP COLUMN hdr;
ALTER TABLE watch_entries DROP COLUMN resolution;
ALTER TABLE watch_entries DROP COLUMN dimension;
ALTER TABLE watch_entries DROP COLUMN device;
ALTER TABLE watch_entries DROP COLUMN edition;
ALTER TABLE watch_entries DROP COLUMN media;
ALTER TABLE watch_entries DROP COLUMN monetization;
ALTER TABLE watch_entries DROP COLUMN provider_name;
ALTER TABLE watch_entries DROP COLUMN provider_id;
ALTER TABLE watch_entries DROP COLUMN booking_source;
ALTER TABLE watch_entries DROP COLUMN tickets_count;
ALTER TABLE watch_entries DROP COLUMN currency;
ALTER TABLE watch_entries DROP COLUMN concessions_minor;
ALTER TABLE watch_entries DROP COLUMN fees_minor;
ALTER TABLE watch_entries DROP COLUMN price_minor;
ALTER TABLE watch_entries DROP COLUMN with_qa;
ALTER TABLE watch_entries DROP COLUMN event_name;
ALTER TABLE watch_entries DROP COLUMN screening_type;
ALTER TABLE watch_entries DROP COLUMN seat;
ALTER TABLE watch_entries DROP COLUMN venue_city_at_watch;
ALTER TABLE watch_entries DROP COLUMN venue_name_at_watch;
ALTER TABLE watch_entries DROP COLUMN screen_id;
ALTER TABLE watch_entries DROP COLUMN venue_id;
DROP INDEX IF EXISTS idx_venue_screens_venue;
DROP TABLE IF EXISTS venue_screens;
DROP TABLE IF EXISTS venue_refs;
DROP INDEX IF EXISTS idx_venue_aliases_venue;
DROP TABLE IF EXISTS venue_aliases;
DROP INDEX IF EXISTS idx_venues_name;
DROP INDEX IF EXISTS idx_venues_country_city;
DROP TABLE IF EXISTS venues;
DROP INDEX IF EXISTS idx_watch_entry_formats_format;
DROP TABLE IF EXISTS watch_entry_formats;
DROP INDEX IF EXISTS idx_watch_formats_active;
DROP TABLE IF EXISTS watch_formats;
