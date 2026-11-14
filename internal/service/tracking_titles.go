package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// ErrTrackedTitleUnknown is returned when TMDB has no such movie or show.
var ErrTrackedTitleUnknown = errors.New("tmdb title not found")

// trackedTitleTTL is how long a cached title snapshot is reused before it is
// fetched from TMDB again. Tracking does not mirror TMDB; it caches only
// enough to render a diary without a request per row.
const trackedTitleTTL = 7 * 24 * time.Hour

// trackedEpisodesTTL is how long a tracked show's episode list is reused.
// Shorter than the title TTL because a running show gains episodes, and an
// episode missing from the cache is an episode missing from up-next.
const trackedEpisodesTTL = 24 * time.Hour

var tmdbIDRe = regexp.MustCompile(`^[0-9]{1,12}$`)

// titleSource is the part of the TMDB client tracking needs.
type titleSource interface {
	Enabled() bool
	SearchMovie(ctx context.Context, query string) ([]client.TMDBSearchResult, error)
	SearchTV(ctx context.Context, query string) ([]client.TMDBSearchResult, error)
	TitleDetails(ctx context.Context, kind, tmdbID string) (client.TMDBTitleDetails, error)
	TVSeasonEpisodes(ctx context.Context, tvID string, seasons []int) (map[int][]client.TMDBEpisode, error)
}

// TrackedTitleService searches TMDB and caches the titles a user tracks.
//
// A user can track anything on TMDB, including titles that never appeared on
// a chart, so these snapshots live in tracked_titles rather than in
// tmdb_titles: nothing here may be picked up by the mapping or metadata jobs.
type TrackedTitleService struct {
	log   *slog.Logger
	tmdb  titleSource
	write *repository.TrackingRepository
	read  *repository.TrackingRepository
	now   func() time.Time
}

func NewTrackedTitleService(log *slog.Logger, tmdb titleSource, write, read *repository.TrackingRepository) *TrackedTitleService {
	if read == nil {
		read = write
	}
	return &TrackedTitleService{log: log, tmdb: tmdb, write: write, read: read, now: time.Now}
}

// TitleSearchResult is one TMDB search hit.
type TitleSearchResult struct {
	Kind          string `json:"tmdb_kind"`
	ID            string `json:"tmdb_id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title,omitempty"`
	Year          int    `json:"year,omitempty"`
	ReleaseDate   string `json:"release_date,omitempty"`
	PosterPath    string `json:"poster_path,omitempty"`
}

// Search looks a title up on TMDB. kind is "movie", "tv" or "" for both.
func (s *TrackedTitleService) Search(ctx context.Context, query, kind string, limit int) ([]TitleSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, &ValidationError{Message: "q is required"}
	}
	if kind != "" && kind != "movie" && kind != "tv" {
		return nil, &ValidationError{Message: "kind must be movie or tv"}
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if !s.tmdb.Enabled() {
		return nil, fmt.Errorf("%w: TMDB_API_KEY is not set", ErrUpstream)
	}

	out := []TitleSearchResult{}
	for _, k := range []string{"movie", "tv"} {
		if kind != "" && kind != k {
			continue
		}
		var hits []client.TMDBSearchResult
		var err error
		if k == "movie" {
			hits, err = s.tmdb.SearchMovie(ctx, query)
		} else {
			hits, err = s.tmdb.SearchTV(ctx, query)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: tmdb search: %v", ErrUpstream, err)
		}
		for _, h := range hits {
			out = append(out, TitleSearchResult{
				Kind: k, ID: fmt.Sprint(h.ID), Title: h.Title, OriginalTitle: h.OriginalTitle,
				Year: h.Year, ReleaseDate: h.ReleaseDate, PosterPath: h.PosterPath,
			})
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ValidateRef checks a TMDB reference without touching the network.
func ValidateRef(kind, id string) error {
	if kind != "movie" && kind != "tv" {
		return &ValidationError{Message: "kind must be movie or tv"}
	}
	if !tmdbIDRe.MatchString(id) {
		return &ValidationError{Message: "id must be a TMDB numeric id"}
	}
	return nil
}

// Resolve returns what is known about a title, fetching and caching it from
// TMDB when it is unknown or the cache has expired. ErrTrackedTitleUnknown
// means TMDB has no such title.
//
// With no TMDB key the stored snapshot is returned as it is, so tracking
// keeps working (with empty metadata) rather than failing.
func (s *TrackedTitleService) Resolve(ctx context.Context, kind, id string) (repository.TitleSnapshot, error) {
	if err := ValidateRef(kind, id); err != nil {
		return repository.TitleSnapshot{}, err
	}
	snap, err := s.read.TitleSnapshot(ctx, kind, id)
	if err != nil {
		return repository.TitleSnapshot{}, err
	}
	if snap.Known() && (snap.Charted || !s.stale(snap)) {
		return snap, nil
	}
	if !s.tmdb.Enabled() {
		return snap, nil
	}

	d, err := s.tmdb.TitleDetails(ctx, kind, id)
	if err != nil {
		if snap.Known() {
			// Keep serving what is cached when TMDB is briefly unavailable.
			s.log.Warn("tmdb title details failed, using cached snapshot", "kind", kind, "id", id, "err", err)
			return snap, nil
		}
		return repository.TitleSnapshot{}, fmt.Errorf("%w: tmdb title details: %v", ErrUpstream, err)
	}
	if !d.Found {
		if snap.Known() {
			return snap, nil
		}
		return repository.TitleSnapshot{}, ErrTrackedTitleUnknown
	}
	if err := s.write.SaveTrackedTitle(ctx, repository.TrackedTitle{
		Kind: kind, ID: id, Title: d.Title, OriginalTitle: d.OriginalTitle,
		ReleaseDate: d.ReleaseDate, Year: d.Year, PosterPath: d.PosterPath,
		Runtime: d.Runtime, NumberOfSeasons: d.NumberOfSeasons, NumberOfEpisodes: d.NumberOfEpisodes,
	}); err != nil {
		return repository.TitleSnapshot{}, err
	}
	return s.read.TitleSnapshot(ctx, kind, id)
}

// stale reports whether the cached snapshot should be fetched again.
func (s *TrackedTitleService) stale(snap repository.TitleSnapshot) bool {
	if snap.CachedAt == "" {
		return true
	}
	at, err := repository.ParseTimestamp(snap.CachedAt)
	if err != nil {
		return true
	}
	return s.now().UTC().Sub(at) > trackedTitleTTL
}

// ResolveEpisodes makes sure every season of a show is cached, fetching them
// from TMDB when the cache is empty or has expired.
//
// This is why tracked_episodes exists next to tmdb_episodes: the chart-side
// table deliberately holds only recent, upcoming and latest seasons, which
// is right for the calendar but useless for marking a show watched.
func (s *TrackedTitleService) ResolveEpisodes(ctx context.Context, tmdbID string, force bool) error {
	if err := ValidateRef("tv", tmdbID); err != nil {
		return err
	}
	if !force {
		at, err := s.read.EpisodesFetchedAt(ctx, tmdbID)
		if err != nil {
			return err
		}
		if at != "" {
			fetched, err := repository.ParseTimestamp(at)
			if err == nil && s.now().UTC().Sub(fetched) <= trackedEpisodesTTL {
				return nil
			}
		}
	}
	if !s.tmdb.Enabled() {
		return nil
	}

	// The show's own details carry the season list, season 0 included;
	// number_of_seasons leaves specials out.
	d, err := s.tmdb.TitleDetails(ctx, "tv", tmdbID)
	if err != nil {
		return fmt.Errorf("%w: tmdb show details: %v", ErrUpstream, err)
	}
	if !d.Found {
		return ErrTrackedTitleUnknown
	}
	if err := s.write.SaveTrackedTitle(ctx, repository.TrackedTitle{
		Kind: "tv", ID: tmdbID, Title: d.Title, OriginalTitle: d.OriginalTitle,
		ReleaseDate: d.ReleaseDate, Year: d.Year, PosterPath: d.PosterPath,
		Runtime: d.Runtime, NumberOfSeasons: d.NumberOfSeasons, NumberOfEpisodes: d.NumberOfEpisodes,
	}); err != nil {
		return err
	}

	seasons := make([]int, 0, len(d.Seasons))
	for _, season := range d.Seasons {
		seasons = append(seasons, season.Number)
	}
	if len(seasons) == 0 {
		// A show TMDB lists no seasons for still gets stamped, so we do not
		// ask again on every request.
		return s.write.SaveShowEpisodes(ctx, tmdbID, map[int64][]repository.TrackedEpisode{})
	}

	fetched, err := s.tmdb.TVSeasonEpisodes(ctx, tmdbID, seasons)
	if err != nil {
		return fmt.Errorf("%w: tmdb season episodes: %v", ErrUpstream, err)
	}
	out := make(map[int64][]repository.TrackedEpisode, len(fetched))
	for season, episodes := range fetched {
		rows := make([]repository.TrackedEpisode, 0, len(episodes))
		for _, e := range episodes {
			if e.EpisodeNumber <= 0 {
				continue
			}
			row := repository.TrackedEpisode{
				SeasonNumber: int64(season), EpisodeNumber: int64(e.EpisodeNumber), Name: e.Name,
			}
			if e.ID > 0 {
				id := strconv.Itoa(e.ID)
				row.TmdbEpisodeID = &id
			}
			if len(e.AirDate) == 10 {
				air := e.AirDate
				row.AirDate = &air
			}
			if e.EpisodeType != "" {
				t := e.EpisodeType
				row.EpisodeType = &t
			}
			if e.Runtime > 0 {
				rt := int64(e.Runtime)
				row.Runtime = &rt
			}
			rows = append(rows, row)
		}
		out[int64(season)] = rows
	}
	return s.write.SaveShowEpisodes(ctx, tmdbID, out)
}
