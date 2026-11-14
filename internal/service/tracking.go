package service

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Watch statuses a user can put on a title. Validated here rather than by a
// CHECK constraint so a new one needs no table rebuild.
const (
	StatusNone          = ""
	StatusWatchlist     = "watchlist"
	StatusWatching      = "watching"
	StatusCompleted     = "completed"
	StatusDropped       = "dropped"
	StatusOnHold        = "on_hold"
	StatusNotInterested = "not_interested"
)

var trackingStatuses = map[string]bool{
	StatusNone: true, StatusWatchlist: true, StatusWatching: true, StatusCompleted: true,
	StatusDropped: true, StatusOnHold: true, StatusNotInterested: true,
}

// How precisely a watch's time is known. An imprecise time is stored at the
// earliest instant it could mean (1 January, the 1st, 00:00 local).
const (
	PrecisionMinute = "minute"
	PrecisionDay    = "day"
	PrecisionMonth  = "month"
	PrecisionYear   = "year"
)

var trackingPrecisions = map[string]bool{
	PrecisionMinute: true, PrecisionDay: true, PrecisionMonth: true, PrecisionYear: true,
}

var trackingVenueKinds = map[string]bool{
	"theater": true, "streaming": true, "physical": true, "broadcast": true, "other": true,
}

const (
	maxTagsPerWatch = 20
	maxTagLength    = 50
	maxDiaryLimit   = 200
	maxListLimit    = 200
)

// TrackingService records what a user watchlists and watches.
//
// Every method takes the acting user's id and passes it to the repository,
// which scopes every statement by it; nothing here may read or write another
// user's rows.
type TrackingService struct {
	log    *slog.Logger
	write  *repository.TrackingRepository
	read   *repository.TrackingRepository
	titles *TrackedTitleService
	now    func() time.Time
}

func NewTrackingService(log *slog.Logger, write, read *repository.TrackingRepository, titles *TrackedTitleService) *TrackingService {
	if read == nil {
		read = write
	}
	return &TrackingService{log: log, write: write, read: read, titles: titles, now: time.Now}
}

// TitleStateInput is what a client sets on a title.
type TitleStateInput struct {
	Status        *string `json:"status"`
	Rating        *int64  `json:"rating"`
	Favorite      *bool   `json:"favorite"`
	WatchlistNote *string `json:"watchlist_note"`
}

// GetTitleState returns the user's state for a title plus what is known about
// it, resolving and caching the title from TMDB when it is new.
func (s *TrackingService) GetTitleState(ctx context.Context, userID int64, kind, id string) (map[string]any, error) {
	snap, err := s.titles.Resolve(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	st, err := s.read.UserTitle(ctx, userID, kind, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"title": titlePayload(snap), "state": st}, nil
}

// SetTitleState sets the status, favourite flag and watchlist note of a
// title. A title moved onto the watchlist is put at the end of it.
//
// A rating here is not accepted: "my rating" is the latest watch's rating, so
// ratings are set on a watch.
func (s *TrackingService) SetTitleState(ctx context.Context, userID int64, kind, id string, in TitleStateInput) (map[string]any, error) {
	if err := ValidateRef(kind, id); err != nil {
		return nil, err
	}
	if in.Rating != nil {
		return nil, &ValidationError{Message: "rating is set on a watch, not on a title; log a watch with a rating instead"}
	}
	current, err := s.write.UserTitle(ctx, userID, kind, id)
	if err != nil {
		return nil, err
	}

	state := repository.UserTitleState{}
	if current != nil {
		state.Status = current.Status
		state.Favorite = current.Favorite
		state.WatchlistPosition = current.WatchlistPosition
		state.WatchlistNote = current.WatchlistNote
	}
	if in.Status != nil {
		status := strings.TrimSpace(*in.Status)
		if !trackingStatuses[status] {
			return nil, &ValidationError{Message: "status must be one of watchlist, watching, completed, dropped, on_hold, not_interested, or empty"}
		}
		if status == StatusWatchlist && state.Status != StatusWatchlist {
			pos, err := s.write.NextWatchlistPosition(ctx, userID)
			if err != nil {
				return nil, err
			}
			state.WatchlistPosition = &pos
		}
		if status != StatusWatchlist {
			state.WatchlistPosition = nil
		}
		state.Status = status
	}
	if in.Favorite != nil {
		state.Favorite = *in.Favorite
	}
	if in.WatchlistNote != nil {
		note := strings.TrimSpace(*in.WatchlistNote)
		if note == "" {
			state.WatchlistNote = nil
		} else {
			state.WatchlistNote = &note
		}
	}

	// Resolve first, so a typo'd id is a 404 before anything is stored.
	snap, err := s.titles.Resolve(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	if err := s.write.SaveUserTitleState(ctx, userID, kind, id, state); err != nil {
		return nil, err
	}
	st, err := s.write.UserTitle(ctx, userID, kind, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"title": titlePayload(snap), "state": st}, nil
}

// DeleteTitleState clears a user's state for a title. Watches are kept, so
// the roll-up is recomputed afterwards.
func (s *TrackingService) DeleteTitleState(ctx context.Context, userID int64, kind, id string) error {
	if err := ValidateRef(kind, id); err != nil {
		return err
	}
	ok, err := s.write.DeleteUserTitle(ctx, userID, kind, id)
	if err != nil {
		return err
	}
	if !ok {
		return sql.ErrNoRows
	}
	return nil
}

// ListTitlesInput selects a page of a user's tracked titles.
type ListTitlesInput struct {
	Status       string
	Kind         string
	FavoriteOnly bool
	Sort         string
	Limit        int64
	Offset       int64
}

// ListTitles returns a page of the user's tracked titles.
func (s *TrackingService) ListTitles(ctx context.Context, userID int64, in ListTitlesInput) (map[string]any, error) {
	if in.Status != "" && !trackingStatuses[in.Status] {
		return nil, &ValidationError{Message: "unknown status"}
	}
	if in.Kind != "" && in.Kind != "movie" && in.Kind != "tv" {
		return nil, &ValidationError{Message: "kind must be movie or tv"}
	}
	switch in.Sort {
	case "", "position_asc", "rating_desc", "watched_desc", "title_asc":
	default:
		return nil, &ValidationError{Message: "sort must be one of position_asc, rating_desc, watched_desc, title_asc"}
	}
	if in.Limit <= 0 {
		in.Limit = 50
	}
	if in.Limit > maxListLimit {
		in.Limit = maxListLimit
	}
	if in.Offset < 0 {
		in.Offset = 0
	}
	if in.Status == StatusWatchlist && in.Sort == "" {
		in.Sort = "position_asc"
	}

	rows, total, err := s.read.ListUserTitles(ctx, userID, repository.UserTitleFilter{
		Status: in.Status, Kind: in.Kind, FavoriteOnly: in.FavoriteOnly,
		Sort: in.Sort, Limit: in.Limit, Offset: in.Offset,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"titles": rows, "total": total, "limit": in.Limit, "offset": in.Offset}, nil
}

// WatchInput is a watch to log or the new contents of one.
//
// The client sends UTC instants. EndedAt is when the watch ended, which is
// what clients log; StartedAt is optional and only stored when given. TZ is
// the IANA zone the watch happened in and decides which local day it lands
// on; the offset at that instant is derived from it and stored alongside.
type WatchInput struct {
	ClientKey     *string  `json:"client_key"`
	Kind          string   `json:"tmdb_kind"`
	TmdbID        string   `json:"tmdb_id"`
	SeasonNumber  *int64   `json:"season_number"`
	EpisodeNumber *int64   `json:"episode_number"`
	EndedAt       string   `json:"ended_at"`
	StartedAt     *string  `json:"started_at"`
	TZ            *string  `json:"tz"`
	OffsetMinutes *int64   `json:"utc_offset_minutes"`
	Precision     string   `json:"time_precision"`
	Rating        *int64   `json:"rating"`
	Liked         *bool    `json:"liked"`
	Notes         *string  `json:"notes"`
	VenueKind     *string  `json:"venue_kind"`
	Tags          []string `json:"tags"`
	// Formats are premium-format slugs (imax, dolby-atmos, 70mm, ...) from
	// GET /api/v1/me/formats. One watch can carry several, because a
	// screening mixes independent things: a brand, a projection, a sound
	// system, a motion rig.
	Formats []string `json:"formats"`
	// Where and how it was watched, flattened into this body.
	WatchContextInput
}

// LogWatch records a watch of a movie. Sending the same client_key twice
// returns the watch stored the first time instead of a duplicate, so a
// retried request is safe.
func (s *TrackingService) LogWatch(ctx context.Context, userID int64, in WatchInput) (repository.WatchEntry, bool, error) {
	if err := ValidateRef(in.Kind, in.TmdbID); err != nil {
		return repository.WatchEntry{}, false, err
	}
	if in.Kind == "tv" && (in.SeasonNumber == nil || in.EpisodeNumber == nil) {
		return repository.WatchEntry{}, false, &ValidationError{
			Message: "a show is watched one episode at a time: send season_number and episode_number, or mark a season with POST /api/v1/me/shows/{id}/watched"}
	}
	if in.Kind == "movie" && (in.SeasonNumber != nil || in.EpisodeNumber != nil) {
		return repository.WatchEntry{}, false, &ValidationError{Message: "a movie has no season or episode number"}
	}
	if in.ClientKey != nil {
		key := strings.TrimSpace(*in.ClientKey)
		if key != "" {
			existing, err := s.write.WatchByClientKey(ctx, userID, key)
			if err != nil {
				return repository.WatchEntry{}, false, err
			}
			if existing != nil {
				return *existing, false, nil
			}
		}
	}

	snap, err := s.titles.Resolve(ctx, in.Kind, in.TmdbID)
	if err != nil {
		return repository.WatchEntry{}, false, err
	}
	ep, err := s.episodeFor(ctx, in)
	if err != nil {
		return repository.WatchEntry{}, false, err
	}
	watchContext, formatIDs, venueTZ, err := s.resolveContext(ctx, in.WatchContextInput, in.Formats)
	if err != nil {
		return repository.WatchEntry{}, false, err
	}
	if err := s.checkSubscription(ctx, userID, watchContext); err != nil {
		return repository.WatchEntry{}, false, err
	}
	// A watch at a cinema takes that cinema's time zone unless the client
	// sends one, which is what makes the local day right when you are away.
	if in.TZ == nil && venueTZ != nil {
		in.TZ = venueTZ
	}
	w, err := s.buildWatch(in, snap, ep)
	if err != nil {
		return repository.WatchEntry{}, false, err
	}
	w.Context, w.FormatIDs = watchContext, formatIDs
	id, err := s.write.CreateWatch(ctx, userID, w)
	if err != nil {
		return repository.WatchEntry{}, false, err
	}
	entry, err := s.write.Watch(ctx, userID, id)
	if err != nil {
		return repository.WatchEntry{}, false, err
	}
	return entry, true, nil
}

// GetWatch returns one of the user's watches. Another user's watch is
// reported as missing, never as forbidden, so ids do not leak.
func (s *TrackingService) GetWatch(ctx context.Context, userID, id int64) (repository.WatchEntry, error) {
	return s.read.Watch(ctx, userID, id)
}

// UpdateWatch replaces the editable contents of a watch.
func (s *TrackingService) UpdateWatch(ctx context.Context, userID, id int64, in WatchInput) (repository.WatchEntry, error) {
	existing, err := s.write.Watch(ctx, userID, id)
	if err != nil {
		return repository.WatchEntry{}, err
	}
	in.Kind, in.TmdbID = existing.Kind, existing.TmdbID
	in.SeasonNumber, in.EpisodeNumber = existing.SeasonNumber, existing.EpisodeNumber
	snap, err := s.titles.Resolve(ctx, existing.Kind, existing.TmdbID)
	if err != nil {
		return repository.WatchEntry{}, err
	}
	ep, err := s.episodeFor(ctx, in)
	if err != nil {
		return repository.WatchEntry{}, err
	}
	watchContext, formatIDs, venueTZ, err := s.resolveContext(ctx, in.WatchContextInput, in.Formats)
	if err != nil {
		return repository.WatchEntry{}, err
	}
	if err := s.checkSubscription(ctx, userID, watchContext); err != nil {
		return repository.WatchEntry{}, err
	}
	if in.TZ == nil && venueTZ != nil {
		in.TZ = venueTZ
	}
	w, err := s.buildWatch(in, snap, ep)
	if err != nil {
		return repository.WatchEntry{}, err
	}
	ok, err := s.write.UpdateWatch(ctx, userID, id, existing.Kind, existing.TmdbID, repository.WatchUpdate{
		EndedAt: w.EndedAt, StartedAt: w.StartedAt,
		RuntimeMinutesUsed: w.RuntimeMinutesUsed, RuntimeSource: w.RuntimeSource,
		Tz: w.Tz, UTCOffsetMinutes: w.UTCOffsetMinutes,
		WatchedOn: w.WatchedOn, TimePrecision: w.TimePrecision,
		Rating: w.Rating, Liked: w.Liked, Notes: w.Notes, VenueKind: w.VenueKind, Tags: w.Tags,
		Context: watchContext, FormatIDs: formatIDs,
	})
	if err != nil {
		return repository.WatchEntry{}, err
	}
	if !ok {
		return repository.WatchEntry{}, sql.ErrNoRows
	}
	return s.write.Watch(ctx, userID, id)
}

// DeleteWatch removes a watch for good (there is no trash) and recomputes the
// title's roll-up.
func (s *TrackingService) DeleteWatch(ctx context.Context, userID, id int64) error {
	existing, err := s.write.Watch(ctx, userID, id)
	if err != nil {
		return err
	}
	ok, err := s.write.DeleteWatch(ctx, userID, id, existing.Kind, existing.TmdbID)
	if err != nil {
		return err
	}
	if !ok {
		return sql.ErrNoRows
	}
	return nil
}

// DiaryInput selects a page of the watch log.
type DiaryInput struct {
	Kind      string
	TmdbID    string
	From      string
	To        string
	RatedOnly bool
	Cursor    string
	Limit     int64
}

// ListWatches returns a page of the diary with a cursor for the next page.
func (s *TrackingService) ListWatches(ctx context.Context, userID int64, in DiaryInput) (map[string]any, error) {
	if in.Kind != "" && in.Kind != "movie" && in.Kind != "tv" {
		return nil, &ValidationError{Message: "kind must be movie or tv"}
	}
	if in.TmdbID != "" && !tmdbIDRe.MatchString(in.TmdbID) {
		return nil, &ValidationError{Message: "tmdb_id must be a TMDB numeric id"}
	}
	for name, v := range map[string]string{"from": in.From, "to": in.To} {
		if v == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", v); err != nil {
			return nil, &ValidationError{Message: name + " must be YYYY-MM-DD"}
		}
	}
	if in.Limit <= 0 {
		in.Limit = 50
	}
	if in.Limit > maxDiaryLimit {
		in.Limit = maxDiaryLimit
	}
	cursorOn, cursorID, err := parseDiaryCursor(in.Cursor)
	if err != nil {
		return nil, err
	}

	// One extra row tells us whether there is another page.
	rows, err := s.read.ListWatches(ctx, userID, repository.DiaryFilter{
		Kind: in.Kind, TmdbID: in.TmdbID, FromDay: in.From, ToDay: in.To,
		RatedOnly: in.RatedOnly, CursorOn: cursorOn, CursorID: cursorID, Limit: in.Limit + 1,
	})
	if err != nil {
		return nil, err
	}
	next := ""
	if int64(len(rows)) > in.Limit {
		rows = rows[:in.Limit]
		last := rows[len(rows)-1]
		next = fmt.Sprintf("%s:%d", last.WatchedOn, last.ID)
	}
	out := map[string]any{"watches": rows, "limit": in.Limit}
	if next != "" {
		out["next_cursor"] = next
	}
	return out, nil
}

// ListTags returns the user's tags with how many watches carry each.
func (s *TrackingService) ListTags(ctx context.Context, userID int64) (map[string]any, error) {
	tags, err := s.read.ListTags(ctx, userID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"tags": tags}, nil
}

// DeleteTag removes a tag and detaches it from the user's watches.
func (s *TrackingService) DeleteTag(ctx context.Context, userID, id int64) error {
	ok, err := s.write.DeleteTag(ctx, userID, id)
	if err != nil {
		return err
	}
	if !ok {
		return sql.ErrNoRows
	}
	return nil
}

// SearchTitles looks a title up on TMDB.
func (s *TrackingService) SearchTitles(ctx context.Context, query, kind string, limit int) (map[string]any, error) {
	results, err := s.titles.Search(ctx, query, kind, limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{"results": results}, nil
}

// buildWatch validates a watch and works out its stored instants, local day
// and runtime provenance.
func (s *TrackingService) buildWatch(in WatchInput, snap repository.TitleSnapshot, ep *repository.TrackedEpisode) (repository.NewWatch, error) {
	precision := in.Precision
	if precision == "" {
		precision = PrecisionMinute
	}
	if !trackingPrecisions[precision] {
		return repository.NewWatch{}, &ValidationError{Message: "time_precision must be one of minute, day, month, year"}
	}
	if in.Rating != nil && (*in.Rating < 1 || *in.Rating > 10) {
		return repository.NewWatch{}, &ValidationError{Message: "rating must be 1-10 half-stars (1 = half a star, 10 = five stars), or null"}
	}
	if in.VenueKind != nil && *in.VenueKind != "" && !trackingVenueKinds[*in.VenueKind] {
		return repository.NewWatch{}, &ValidationError{Message: "venue_kind must be one of theater, streaming, physical, broadcast, other"}
	}
	tags, err := normalizeTags(in.Tags)
	if err != nil {
		return repository.NewWatch{}, err
	}

	ended, err := parseInstant("ended_at", in.EndedAt)
	if err != nil {
		return repository.NewWatch{}, err
	}
	var started *time.Time
	if in.StartedAt != nil && strings.TrimSpace(*in.StartedAt) != "" {
		t, err := parseInstant("started_at", *in.StartedAt)
		if err != nil {
			return repository.NewWatch{}, err
		}
		if t.After(ended) {
			return repository.NewWatch{}, &ValidationError{Message: "started_at must not be after ended_at"}
		}
		started = &t
	}

	tz, offset, err := resolveZone(in.TZ, in.OffsetMinutes, ended)
	if err != nil {
		return repository.NewWatch{}, err
	}

	w := repository.NewWatch{
		ClientKey: trimmedPtr(in.ClientKey), SubjectKind: "movie", Kind: in.Kind, TmdbID: in.TmdbID,
		TimePrecision: precision, Rating: in.Rating, Notes: trimmedPtr(in.Notes),
		VenueKind: trimmedPtr(in.VenueKind), Source: "manual", Tags: tags,
	}
	// An episode's own runtime beats the show's typical one.
	runtime := snap.Runtime
	if ep != nil {
		w.SubjectKind = "episode"
		w.SeasonNumber = &ep.SeasonNumber
		w.EpisodeNumber = &ep.EpisodeNumber
		w.TmdbEpisodeID = ep.TmdbEpisodeID
		if ep.Runtime != nil && *ep.Runtime > 0 {
			runtime = int(*ep.Runtime)
		}
	}
	if in.Liked != nil {
		w.Liked = *in.Liked
	}
	if tz != nil {
		w.Tz = tz
	}
	if offset != 0 || tz != nil {
		off := offset
		w.UTCOffsetMinutes = &off
	}

	if precision == PrecisionMinute {
		w.EndedAt = repository.FormatTimestamp(ended)
		if started != nil {
			st := repository.FormatTimestamp(*started)
			w.StartedAt = &st
		}
		// The day a watch belongs to is the local day of its start: the one
		// the client sent, else ended_at minus the runtime. It is computed
		// here and frozen, so a later TMDB runtime change cannot move an
		// entry to a different day.
		effective := ended
		switch {
		case started != nil:
			effective = *started
		case runtime > 0:
			effective = ended.Add(-time.Duration(runtime) * time.Minute)
			used := int64(runtime)
			w.RuntimeMinutesUsed = &used
			src := "tmdb_title"
			if ep != nil && ep.Runtime != nil && *ep.Runtime > 0 {
				src = "tmdb_episode"
			}
			w.RuntimeSource = &src
		default:
			src := "unknown"
			w.RuntimeSource = &src
		}
		w.WatchedOn = localDay(effective, offset)
		return w, nil
	}

	// An imprecise time means the earliest instant it could be: 1 January,
	// the 1st, 00:00 in the user's own zone.
	local := ended.Add(time.Duration(offset) * time.Minute)
	y, m, d := local.Year(), local.Month(), local.Day()
	switch precision {
	case PrecisionYear:
		m, d = time.January, 1
	case PrecisionMonth:
		d = 1
	}
	localMidnight := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	w.WatchedOn = localMidnight.Format("2006-01-02")
	w.EndedAt = repository.FormatTimestamp(localMidnight.Add(-time.Duration(offset) * time.Minute))
	return w, nil
}

// parseInstant reads an RFC 3339 instant and converts it to UTC.
func parseInstant(field, v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, &ValidationError{Message: field + " is required (RFC 3339, e.g. 2026-11-12T23:00:00Z)"}
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, &ValidationError{Message: field + " must be an RFC 3339 instant, e.g. 2026-11-12T23:00:00Z"}
	}
	return t.UTC(), nil
}

// resolveZone works out the zone to record and its offset at instant. An
// IANA name wins: the offset is derived from it, because an offset alone
// cannot be recomputed after an edit that crosses a DST change.
func resolveZone(tz *string, offsetMinutes *int64, instant time.Time) (*string, int64, error) {
	if tz != nil && strings.TrimSpace(*tz) != "" {
		name := strings.TrimSpace(*tz)
		loc, err := time.LoadLocation(name)
		if err != nil {
			return nil, 0, &ValidationError{Message: "tz must be an IANA time zone name, e.g. America/New_York"}
		}
		_, offset := instant.In(loc).Zone()
		return &name, int64(offset / 60), nil
	}
	if offsetMinutes != nil {
		if *offsetMinutes < -12*60 || *offsetMinutes > 14*60 {
			return nil, 0, &ValidationError{Message: "utc_offset_minutes must be between -720 and 840"}
		}
		return nil, *offsetMinutes, nil
	}
	return nil, 0, nil
}

// localDay is the YYYY-MM-DD day instant falls on at the given offset.
func localDay(instant time.Time, offsetMinutes int64) string {
	return instant.Add(time.Duration(offsetMinutes) * time.Minute).Format("2006-01-02")
}

// normalizeTags trims, de-duplicates (ignoring case) and sorts tag names.
func normalizeTags(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if len(name) > maxTagLength {
			return nil, &ValidationError{Message: fmt.Sprintf("a tag may be at most %d characters", maxTagLength)}
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	if len(out) > maxTagsPerWatch {
		return nil, &ValidationError{Message: fmt.Sprintf("a watch may carry at most %d tags", maxTagsPerWatch)}
	}
	sort.Strings(out)
	return out, nil
}

// parseDiaryCursor reads a "YYYY-MM-DD:id" diary cursor.
func parseDiaryCursor(v string) (string, int64, error) {
	if strings.TrimSpace(v) == "" {
		return "", 0, nil
	}
	day, idPart, ok := strings.Cut(v, ":")
	if !ok {
		return "", 0, &ValidationError{Message: "cursor is not valid"}
	}
	if _, err := time.Parse("2006-01-02", day); err != nil {
		return "", 0, &ValidationError{Message: "cursor is not valid"}
	}
	var id int64
	if _, err := fmt.Sscanf(idPart, "%d", &id); err != nil || id <= 0 {
		return "", 0, &ValidationError{Message: "cursor is not valid"}
	}
	return day, id, nil
}

func trimmedPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}
	return &v
}

// titlePayload is the title metadata returned next to a user's state.
func titlePayload(snap repository.TitleSnapshot) map[string]any {
	return map[string]any{
		"tmdb_kind": snap.Kind, "tmdb_id": snap.ID, "title": snap.Title,
		"release_date": snap.ReleaseDate, "poster_path": snap.PosterPath,
		"runtime": snap.Runtime, "number_of_seasons": snap.NumberOfSeasons,
		"number_of_episodes": snap.NumberOfEpisodes, "charted": snap.Charted,
	}
}
