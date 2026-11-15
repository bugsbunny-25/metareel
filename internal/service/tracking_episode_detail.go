package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// episodeDetailer is the TMDB call behind an episode's details screen. The
// TMDB client has it; a source without it gets the cached episode only.
type episodeDetailer interface {
	TVEpisode(ctx context.Context, tvID string, season, episode int) (*client.TMDBEpisodeDetail, error)
}

// EpisodeShow is the show an episode belongs to.
type EpisodeShow struct {
	TmdbID     string `json:"tmdb_id"`
	Title      string `json:"title"`
	PosterPath string `json:"poster_path,omitempty"`
}

// EpisodeDetail is one episode for a details screen: TMDB's details when
// they can be fetched (synopsis, still, score, crew, guests), else what is
// cached, and the user's watches of it.
type EpisodeDetail struct {
	Show    EpisodeShow              `json:"show"`
	Episode client.TMDBEpisodeDetail `json:"episode"`
	Live    bool                     `json:"live"` // false: TMDB couldn't be reached, only the cached basics
	Watches []repository.WatchEntry  `json:"watches"`
}

// Upper bounds for season and episode numbers (well past any real show), so
// they fit an int anywhere.
const (
	maxSeasonNumber  = 1000
	maxEpisodeNumber = 100_000
)

// maxEpisodeWatchPages bounds the diary scan for one episode's watches (200 a page).
const maxEpisodeWatchPages = 10

// Episode returns one episode of a show with the user's watches of it.
func (s *TrackingService) Episode(ctx context.Context, userID int64, tmdbID string, season, episode int64) (*EpisodeDetail, error) {
	if err := ValidateRef("tv", tmdbID); err != nil {
		return nil, err
	}
	if season < 0 || season > maxSeasonNumber {
		return nil, &ValidationError{Message: "season must be 0 (specials) to 1000"}
	}
	if episode < 1 || episode > maxEpisodeNumber {
		return nil, &ValidationError{Message: "episode must be 1 to 100000"}
	}
	snap, err := s.titles.Resolve(ctx, "tv", tmdbID)
	if err != nil {
		return nil, err
	}
	out := &EpisodeDetail{Show: EpisodeShow{TmdbID: tmdbID, Title: snap.Title, PosterPath: snap.PosterPath}, Watches: []repository.WatchEntry{}}

	if d, ok := s.titles.tmdb.(episodeDetailer); ok && s.titles.tmdb.Enabled() {
		live, err := d.TVEpisode(ctx, tmdbID, int(season), int(episode))
		if err != nil {
			// The cached basics still make a screen; say it isn't live.
			s.log.Warn("tmdb episode fetch failed", "tmdb_id", tmdbID, "season", season, "episode", episode, "error", err)
		} else if live != nil {
			out.Episode, out.Live = *live, true
		}
	}
	if !out.Live {
		if err := s.titles.ResolveEpisodes(ctx, tmdbID, false); err != nil {
			return nil, err
		}
		ep, err := s.read.Episode(ctx, tmdbID, season, episode)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		if err != nil {
			return nil, err
		}
		out.Episode = client.TMDBEpisodeDetail{
			SeasonNumber: int(season), EpisodeNumber: int(episode), Name: ep.Name,
			Directors: []string{}, Writers: []string{}, GuestStars: []client.TMDBGuestStar{},
		}
		if ep.AirDate != nil {
			out.Episode.AirDate = *ep.AirDate
		}
		if ep.EpisodeType != nil {
			out.Episode.EpisodeType = *ep.EpisodeType
		}
		if ep.Runtime != nil {
			out.Episode.Runtime = int(*ep.Runtime)
		}
	}

	// The user's watches of this episode, newest first.
	cursorOn, cursorID := "", int64(0)
	for page := 0; page < maxEpisodeWatchPages; page++ {
		rows, err := s.read.ListWatches(ctx, userID, repository.DiaryFilter{
			Kind: "tv", TmdbID: tmdbID, SubjectKind: "episode", CursorOn: cursorOn, CursorID: cursorID, Limit: maxDiaryLimit,
		})
		if err != nil {
			return nil, err
		}
		for _, w := range rows {
			if w.SeasonNumber != nil && w.EpisodeNumber != nil && *w.SeasonNumber == season && *w.EpisodeNumber == episode {
				out.Watches = append(out.Watches, w)
			}
		}
		if int64(len(rows)) < maxDiaryLimit {
			break
		}
		last := rows[len(rows)-1]
		cursorOn, cursorID = last.WatchedOn, last.ID
	}
	return out, nil
}
