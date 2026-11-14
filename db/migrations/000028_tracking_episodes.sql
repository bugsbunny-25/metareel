-- +goose Up
-- Episode tracking: the episodes of the shows a user tracks, and the episode
-- side of their watch log.
--
-- tmdb_episodes (the chart domain) deliberately holds only recent, upcoming
-- and latest seasons, which is right for the calendar but not for tracking:
-- marking a show watched needs every season. These rows are fetched on
-- demand for tracked shows only and are not touched by tmdb.episodes.refresh.
CREATE TABLE tracked_episodes (
    tmdb_id         TEXT NOT NULL,            -- TMDB TV show ID
    season_number   INTEGER NOT NULL,         -- 0 = specials
    episode_number  INTEGER NOT NULL,
    tmdb_episode_id TEXT,                     -- stable across TMDB renumbering
    name            TEXT NOT NULL DEFAULT '',
    air_date        TEXT,                     -- YYYY-MM-DD, NULL = TBA
    episode_type    TEXT,                     -- standard | finale | mid_season
    runtime         INTEGER,                  -- minutes
    PRIMARY KEY (tmdb_id, season_number, episode_number)
) WITHOUT ROWID;
CREATE INDEX idx_tracked_episodes_air ON tracked_episodes(tmdb_id, air_date);

-- When a tracked show's episodes were last fetched (NULL = never).
ALTER TABLE tracked_titles ADD COLUMN episodes_fetched_at TEXT;

-- How many distinct episodes of a show the user has watched at least once.
-- Kept next to watch_count (which counts watches, rewatches included) so a
-- list of shows needs no aggregate over the log.
ALTER TABLE user_titles ADD COLUMN episodes_watched INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE user_titles DROP COLUMN episodes_watched;
ALTER TABLE tracked_titles DROP COLUMN episodes_fetched_at;
DROP INDEX IF EXISTS idx_tracked_episodes_air;
DROP TABLE IF EXISTS tracked_episodes;
