package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// TimestampLayout is how tracking stores instants: UTC, in the same format as
// SQLite's CURRENT_TIMESTAMP, so stored values sort and compare against
// datetime('now'). Binding a time.Time directly would write Go's own format.
const TimestampLayout = "2006-01-02 15:04:05"

// FormatTimestamp renders t as a stored UTC timestamp.
func FormatTimestamp(t time.Time) string { return t.UTC().Format(TimestampLayout) }

// ParseTimestamp reads a stored UTC timestamp.
func ParseTimestamp(v string) (time.Time, error) {
	return time.ParseInLocation(TimestampLayout, v, time.UTC)
}

// TrackingRepository stores a user's watchlist, statuses, watch log and tags.
// Construct it over the writer for writes and over the read-only pool for
// list and diary reads. Every method is scoped to one user.
type TrackingRepository struct {
	db *sql.DB
	q  *sqlc.Queries
}

func NewTrackingRepository(db *sql.DB) *TrackingRepository {
	return &TrackingRepository{db: db, q: sqlc.New(db)}
}

// TrackedTitle is the minimal TMDB snapshot cached for a tracked title.
type TrackedTitle struct {
	Kind             string // movie | tv
	ID               string
	Title            string
	OriginalTitle    string
	ReleaseDate      string // YYYY-MM-DD, "" if unknown
	Year             int
	PosterPath       string
	Runtime          int
	NumberOfSeasons  int
	NumberOfEpisodes int
}

// TitleSnapshot is what the API shows for a title: the rich tmdb_titles row
// of a charted title when there is one, else the on-demand cache.
type TitleSnapshot struct {
	Kind             string
	ID               string
	Title            string
	ReleaseDate      string
	PosterPath       string
	Runtime          int
	NumberOfSeasons  int
	NumberOfEpisodes int
	CachedAt         string // "" when nothing is cached
	Charted          bool   // metareel knows this title from a chart
}

// Known reports whether anything is known about the title.
func (s TitleSnapshot) Known() bool { return s.Title != "" }

// SaveTrackedTitle caches a title's display metadata.
func (r *TrackingRepository) SaveTrackedTitle(ctx context.Context, t TrackedTitle) error {
	err := r.q.UpsertTrackedTitle(ctx, sqlc.UpsertTrackedTitleParams{
		TmdbKind:         t.Kind,
		TmdbID:           t.ID,
		Title:            t.Title,
		OriginalTitle:    t.OriginalTitle,
		ReleaseDate:      nullIfEmpty(t.ReleaseDate),
		Year:             nullIfZero(t.Year),
		PosterPath:       nullIfEmpty(t.PosterPath),
		Runtime:          nullIfZero(t.Runtime),
		NumberOfSeasons:  nullIfZero(t.NumberOfSeasons),
		NumberOfEpisodes: nullIfZero(t.NumberOfEpisodes),
	})
	if err != nil {
		return fmt.Errorf("cache tracked title %s/%s: %w", t.Kind, t.ID, err)
	}
	return nil
}

// TitleSnapshot returns what is known about a title. The query always returns
// one row, so an unknown title comes back with Known() false, not ErrNoRows.
func (r *TrackingRepository) TitleSnapshot(ctx context.Context, kind, id string) (TitleSnapshot, error) {
	row, err := r.q.GetTitleSnapshot(ctx, sqlc.GetTitleSnapshotParams{TmdbKind: kind, TmdbID: id})
	if err != nil {
		return TitleSnapshot{}, fmt.Errorf("title snapshot %s/%s: %w", kind, id, err)
	}
	return TitleSnapshot{
		Kind: kind, ID: id,
		Title:            row.Title,
		ReleaseDate:      row.ReleaseDate,
		PosterPath:       row.PosterPath,
		Runtime:          int(row.Runtime),
		NumberOfSeasons:  int(row.NumberOfSeasons),
		NumberOfEpisodes: int(row.NumberOfEpisodes),
		CachedAt:         row.CachedAt,
		Charted:          row.Charted != 0,
	}, nil
}

// UserTitle is a user's state for one title.
type UserTitle struct {
	Kind              string  `json:"tmdb_kind"`
	ID                string  `json:"tmdb_id"`
	Status            string  `json:"status"`
	Rating            *int64  `json:"rating,omitempty"`
	Favorite          bool    `json:"favorite"`
	WatchlistPosition *int64  `json:"watchlist_position,omitempty"`
	WatchlistNote     *string `json:"watchlist_note,omitempty"`
	FirstWatchedAt    *string `json:"first_watched_at,omitempty"`
	LastWatchedAt     *string `json:"last_watched_at,omitempty"`
	WatchCount        int64   `json:"watch_count"`
	// EpisodesWatched counts distinct episodes of a show seen at least once,
	// so a rewatch is not extra progress. 0 for a movie.
	EpisodesWatched int64  `json:"episodes_watched"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	// Display metadata, filled by list reads.
	Title       string `json:"title,omitempty"`
	ReleaseDate string `json:"release_date,omitempty"`
	PosterPath  string `json:"poster_path,omitempty"`
	Runtime     int    `json:"runtime,omitempty"`
}

// UserTitle returns the user's state for a title, or nil when they have none.
func (r *TrackingRepository) UserTitle(ctx context.Context, userID int64, kind, id string) (*UserTitle, error) {
	row, err := r.q.GetUserTitle(ctx, sqlc.GetUserTitleParams{UserID: userID, TmdbKind: kind, TmdbID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("user title %s/%s: %w", kind, id, err)
	}
	return &UserTitle{
		Kind: row.TmdbKind, ID: row.TmdbID, Status: row.Status,
		Rating:            nullInt64Ptr(row.Rating),
		Favorite:          row.Favorite != 0,
		WatchlistPosition: nullInt64Ptr(row.WatchlistPosition),
		WatchlistNote:     nullStringPtr(row.WatchlistNote),
		FirstWatchedAt:    nullStringPtr(row.FirstWatchedAt),
		LastWatchedAt:     nullStringPtr(row.LastWatchedAt),
		WatchCount:        row.WatchCount,
		EpisodesWatched:   row.EpisodesWatched,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}, nil
}

// UserTitleState is the part of a user's title state the client owns.
type UserTitleState struct {
	Status            string
	Favorite          bool
	WatchlistPosition *int64
	WatchlistNote     *string
}

// SaveUserTitleState writes the client-owned state and drops the row again if
// nothing worth keeping is left (no status, rating, favourite or watches).
func (r *TrackingRepository) SaveUserTitleState(ctx context.Context, userID int64, kind, id string, st UserTitleState) error {
	favorite := int64(0)
	if st.Favorite {
		favorite = 1
	}
	if err := r.q.SetUserTitleState(ctx, sqlc.SetUserTitleStateParams{
		UserID: userID, TmdbKind: kind, TmdbID: id,
		Status:            st.Status,
		Favorite:          favorite,
		WatchlistPosition: toNullInt64(st.WatchlistPosition),
		WatchlistNote:     toNullString(st.WatchlistNote),
	}); err != nil {
		return fmt.Errorf("save user title state %s/%s: %w", kind, id, err)
	}
	if _, err := r.q.DeleteEmptyUserTitle(ctx, sqlc.DeleteEmptyUserTitleParams{
		UserID: userID, TmdbKind: kind, TmdbID: id}); err != nil {
		return fmt.Errorf("prune empty user title %s/%s: %w", kind, id, err)
	}
	return nil
}

// DeleteUserTitle removes a user's state for a title. Watches are untouched.
func (r *TrackingRepository) DeleteUserTitle(ctx context.Context, userID int64, kind, id string) (bool, error) {
	n, err := r.q.DeleteUserTitle(ctx, sqlc.DeleteUserTitleParams{UserID: userID, TmdbKind: kind, TmdbID: id})
	if err != nil {
		return false, fmt.Errorf("delete user title %s/%s: %w", kind, id, err)
	}
	return n > 0, nil
}

// NextWatchlistPosition is the position to give a new watchlist entry.
func (r *TrackingRepository) NextWatchlistPosition(ctx context.Context, userID int64) (int64, error) {
	n, err := r.q.NextWatchlistPosition(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("next watchlist position: %w", err)
	}
	return n, nil
}

// UserTitleFilter selects and sorts a user's tracked titles.
type UserTitleFilter struct {
	Status       string // "" = every tracked title
	Kind         string // "" = both
	FavoriteOnly bool
	Sort         string // position_asc | rating_desc | watched_desc | title_asc | ""
	Limit        int64
	Offset       int64
}

// ListUserTitles returns a page of tracked titles and the total match count.
func (r *TrackingRepository) ListUserTitles(ctx context.Context, userID int64, f UserTitleFilter) ([]UserTitle, int64, error) {
	favorite := int64(0)
	if f.FavoriteOnly {
		favorite = 1
	}
	rows, err := r.q.ListUserTitles(ctx, sqlc.ListUserTitlesParams{
		Sort: f.Sort, UserID: userID, Status: f.Status, TmdbKind: f.Kind,
		FavoriteOnly: favorite, Limit: f.Limit, Offset: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list user titles: %w", err)
	}
	total, err := r.q.CountUserTitles(ctx, sqlc.CountUserTitlesParams{
		UserID: userID, Status: f.Status, TmdbKind: f.Kind, FavoriteOnly: favorite})
	if err != nil {
		return nil, 0, fmt.Errorf("count user titles: %w", err)
	}
	out := make([]UserTitle, 0, len(rows))
	for _, row := range rows {
		out = append(out, UserTitle{
			Kind: row.TmdbKind, ID: row.TmdbID, Status: row.Status,
			Rating:            nullInt64Ptr(row.Rating),
			Favorite:          row.Favorite != 0,
			WatchlistPosition: nullInt64Ptr(row.WatchlistPosition),
			WatchlistNote:     nullStringPtr(row.WatchlistNote),
			FirstWatchedAt:    nullStringPtr(row.FirstWatchedAt),
			LastWatchedAt:     nullStringPtr(row.LastWatchedAt),
			WatchCount:        row.WatchCount,
			EpisodesWatched:   row.EpisodesWatched,
			CreatedAt:         row.CreatedAt,
			UpdatedAt:         row.UpdatedAt,
			Title:             row.Title,
			ReleaseDate:       row.ReleaseDate,
			PosterPath:        row.PosterPath,
			Runtime:           int(row.Runtime),
		})
	}
	return out, total, nil
}

// WatchEntry is one watch. A rewatch is another entry, with its own rating
// and tags.
type WatchEntry struct {
	ID            int64   `json:"id"`
	ClientKey     *string `json:"client_key,omitempty"`
	SubjectKind   string  `json:"subject_kind"`
	Kind          string  `json:"tmdb_kind"`
	TmdbID        string  `json:"tmdb_id"`
	SeasonNumber  *int64  `json:"season_number,omitempty"`
	EpisodeNumber *int64  `json:"episode_number,omitempty"`
	TmdbEpisodeID *string `json:"tmdb_episode_id,omitempty"`

	EndedAt            string  `json:"ended_at"`
	StartedAt          *string `json:"started_at,omitempty"`
	RuntimeMinutesUsed *int64  `json:"runtime_minutes_used,omitempty"`
	RuntimeSource      *string `json:"runtime_source,omitempty"`
	Tz                 *string `json:"tz,omitempty"`
	UTCOffsetMinutes   *int64  `json:"utc_offset_minutes,omitempty"`
	WatchedOn          string  `json:"watched_on"`
	TimePrecision      string  `json:"time_precision"`

	Rating      *int64   `json:"rating,omitempty"`
	Liked       bool     `json:"liked"`
	IsRewatch   bool     `json:"is_rewatch"`
	WatchNumber int64    `json:"watch_number"`
	Notes       *string  `json:"notes,omitempty"`
	VenueKind   *string  `json:"venue_kind,omitempty"`
	Source      string   `json:"source"`
	Tags        []string `json:"tags"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at,omitempty"`

	// Where and how it was watched. Embedded so it flattens into the JSON
	// rather than nesting a half-empty object inside every entry.
	WatchContext
	Formats []WatchFormat `json:"formats"`

	// Display metadata, filled by diary reads.
	Title       string `json:"title,omitempty"`
	ReleaseDate string `json:"release_date,omitempty"`
	PosterPath  string `json:"poster_path,omitempty"`
}

// NewWatch is a watch to record.
type NewWatch struct {
	ClientKey          *string
	SubjectKind        string
	Kind               string
	TmdbID             string
	SeasonNumber       *int64
	EpisodeNumber      *int64
	TmdbEpisodeID      *string
	EndedAt            string
	StartedAt          *string
	RuntimeMinutesUsed *int64
	RuntimeSource      *string
	Tz                 *string
	UTCOffsetMinutes   *int64
	WatchedOn          string
	TimePrecision      string
	Rating             *int64
	Liked              bool
	Notes              *string
	VenueKind          *string
	Source             string
	Tags               []string
	Context            *WatchContext
	FormatIDs          []int64
}

// CreateWatch records a watch, attaches its tags and refreshes the title's
// roll-up (watch numbers included), all in one transaction.
func (r *TrackingRepository) CreateWatch(ctx context.Context, userID int64, w NewWatch) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)

	liked := int64(0)
	if w.Liked {
		liked = 1
	}
	id, err := q.InsertWatchEntry(ctx, sqlc.InsertWatchEntryParams{
		UserID: userID, ClientKey: toNullString(w.ClientKey), SubjectKind: w.SubjectKind,
		TmdbKind: w.Kind, TmdbID: w.TmdbID,
		SeasonNumber: toNullInt64(w.SeasonNumber), EpisodeNumber: toNullInt64(w.EpisodeNumber),
		TmdbEpisodeID: toNullString(w.TmdbEpisodeID),
		EndedAt:       w.EndedAt, StartedAt: toNullString(w.StartedAt),
		RuntimeMinutesUsed: toNullInt64(w.RuntimeMinutesUsed), RuntimeSource: toNullString(w.RuntimeSource),
		Tz: toNullString(w.Tz), UtcOffsetMinutes: toNullInt64(w.UTCOffsetMinutes),
		WatchedOn: w.WatchedOn, TimePrecision: w.TimePrecision,
		// Placeholder numbering; rollup renumbers the title in time order below.
		Rating: toNullInt64(w.Rating), Liked: liked, IsRewatch: 0, WatchNumber: 1,
		Notes: toNullString(w.Notes), VenueKind: toNullString(w.VenueKind), Source: w.Source,
	})
	if err != nil {
		return 0, fmt.Errorf("insert watch: %w", err)
	}
	if err := setTags(ctx, q, userID, id, w.Tags); err != nil {
		return 0, err
	}
	if err := setContext(ctx, q, userID, id, w.Context); err != nil {
		return 0, err
	}
	if err := setFormats(ctx, q, id, w.FormatIDs); err != nil {
		return 0, err
	}
	if err := rollup(ctx, q, userID, w.Kind, w.TmdbID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit watch: %w", err)
	}
	return id, nil
}

// WatchUpdate is the editable part of a watch.
type WatchUpdate struct {
	EndedAt            string
	StartedAt          *string
	RuntimeMinutesUsed *int64
	RuntimeSource      *string
	Tz                 *string
	UTCOffsetMinutes   *int64
	WatchedOn          string
	TimePrecision      string
	Rating             *int64
	Liked              bool
	Notes              *string
	VenueKind          *string
	Tags               []string
	Context            *WatchContext
	FormatIDs          []int64
}

// UpdateWatch edits a watch and refreshes the title's roll-up. It reports
// false when the entry does not exist or belongs to another user.
func (r *TrackingRepository) UpdateWatch(ctx context.Context, userID, id int64, kind, tmdbID string, u WatchUpdate) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)

	liked := int64(0)
	if u.Liked {
		liked = 1
	}
	n, err := q.UpdateWatchEntry(ctx, sqlc.UpdateWatchEntryParams{
		ID: id, UserID: userID,
		EndedAt: u.EndedAt, StartedAt: toNullString(u.StartedAt),
		RuntimeMinutesUsed: toNullInt64(u.RuntimeMinutesUsed), RuntimeSource: toNullString(u.RuntimeSource),
		Tz: toNullString(u.Tz), UtcOffsetMinutes: toNullInt64(u.UTCOffsetMinutes),
		WatchedOn: u.WatchedOn, TimePrecision: u.TimePrecision,
		Rating: toNullInt64(u.Rating), Liked: liked,
		Notes: toNullString(u.Notes), VenueKind: toNullString(u.VenueKind),
	})
	if err != nil {
		return false, fmt.Errorf("update watch %d: %w", id, err)
	}
	if n == 0 {
		return false, nil
	}
	if err := setTags(ctx, q, userID, id, u.Tags); err != nil {
		return false, err
	}
	if err := setContext(ctx, q, userID, id, u.Context); err != nil {
		return false, err
	}
	if err := setFormats(ctx, q, id, u.FormatIDs); err != nil {
		return false, err
	}
	if err := rollup(ctx, q, userID, kind, tmdbID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit watch update: %w", err)
	}
	return true, nil
}

// DeleteWatch removes a watch for good and refreshes the title's roll-up.
func (r *TrackingRepository) DeleteWatch(ctx context.Context, userID, id int64, kind, tmdbID string) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)

	n, err := q.DeleteWatchEntry(ctx, sqlc.DeleteWatchEntryParams{ID: id, UserID: userID})
	if err != nil {
		return false, fmt.Errorf("delete watch %d: %w", id, err)
	}
	if n == 0 {
		return false, nil
	}
	if err := rollup(ctx, q, userID, kind, tmdbID); err != nil {
		return false, err
	}
	if _, err := q.DeleteEmptyUserTitle(ctx, sqlc.DeleteEmptyUserTitleParams{
		UserID: userID, TmdbKind: kind, TmdbID: tmdbID}); err != nil {
		return false, fmt.Errorf("prune empty user title: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit watch delete: %w", err)
	}
	return true, nil
}

// Watch returns one of the user's watches, or sql.ErrNoRows.
func (r *TrackingRepository) Watch(ctx context.Context, userID, id int64) (WatchEntry, error) {
	row, err := r.q.GetWatchEntry(ctx, sqlc.GetWatchEntryParams{ID: id, UserID: userID})
	if err != nil {
		return WatchEntry{}, err
	}
	e := watchFromRow(row)
	tags, err := r.tagsFor(ctx, userID, []int64{id})
	if err != nil {
		return WatchEntry{}, err
	}
	e.Tags = tags[id]
	if e.Tags == nil {
		e.Tags = []string{}
	}
	formats, err := r.formatsFor(ctx, userID, []int64{id})
	if err != nil {
		return WatchEntry{}, err
	}
	if f := formats[id]; f != nil {
		e.Formats = f
	}
	return e, nil
}

// WatchByClientKey finds a watch by the idempotency key the client sent.
func (r *TrackingRepository) WatchByClientKey(ctx context.Context, userID int64, key string) (*WatchEntry, error) {
	row, err := r.q.GetWatchEntryByClientKey(ctx, sqlc.GetWatchEntryByClientKeyParams{UserID: userID, ClientKey: nullIfEmpty(key)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("watch by client key: %w", err)
	}
	e := watchFromRow(row)
	return &e, nil
}

// DiaryFilter selects a page of the watch log.
type DiaryFilter struct {
	Kind        string
	TmdbID      string
	SubjectKind string
	FromDay     string // YYYY-MM-DD
	ToDay       string // YYYY-MM-DD
	RatedOnly   bool
	CursorOn    string // "" starts at the newest entry
	CursorID    int64
	Limit       int64
}

// ListWatches returns a page of the diary, newest local day first, with each
// entry's tags.
func (r *TrackingRepository) ListWatches(ctx context.Context, userID int64, f DiaryFilter) ([]WatchEntry, error) {
	rated := int64(0)
	if f.RatedOnly {
		rated = 1
	}
	rows, err := r.q.ListWatchEntries(ctx, sqlc.ListWatchEntriesParams{
		UserID: userID, TmdbKind: f.Kind, TmdbID: f.TmdbID, SubjectKind: f.SubjectKind,
		FromDay: f.FromDay, ToDay: f.ToDay, RatedOnly: rated,
		CursorOn: f.CursorOn, CursorID: f.CursorID, Limit: f.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list watches: %w", err)
	}
	out := make([]WatchEntry, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		e := WatchEntry{
			ID: row.ID, SubjectKind: row.SubjectKind, Kind: row.TmdbKind, TmdbID: row.TmdbID,
			SeasonNumber: nullInt64Ptr(row.SeasonNumber), EpisodeNumber: nullInt64Ptr(row.EpisodeNumber),
			EndedAt: row.EndedAt, StartedAt: nullStringPtr(row.StartedAt),
			RuntimeMinutesUsed: nullInt64Ptr(row.RuntimeMinutesUsed), RuntimeSource: nullStringPtr(row.RuntimeSource),
			Tz: nullStringPtr(row.Tz), UTCOffsetMinutes: nullInt64Ptr(row.UtcOffsetMinutes),
			WatchedOn: row.WatchedOn, TimePrecision: row.TimePrecision,
			Rating: nullInt64Ptr(row.Rating), Liked: row.Liked != 0, IsRewatch: row.IsRewatch != 0,
			WatchNumber: row.WatchNumber, Notes: nullStringPtr(row.Notes), VenueKind: nullStringPtr(row.VenueKind),
			Source: row.Source, CreatedAt: row.CreatedAt, Tags: []string{}, Formats: []WatchFormat{},
			Title: row.Title, ReleaseDate: row.ReleaseDate, PosterPath: row.PosterPath,
			WatchContext: WatchContext{
				VenueID: nullInt64Ptr(row.VenueID), VenueNameAtWatch: nullStringPtr(row.VenueNameAtWatch),
				VenueCityAtWatch: nullStringPtr(row.VenueCityAtWatch), ScreeningType: nullStringPtr(row.ScreeningType),
				PriceMinor: nullInt64Ptr(row.PriceMinor), Currency: nullStringPtr(row.Currency),
				ProviderName: nullStringPtr(row.ProviderName), Media: nullStringPtr(row.Media),
				Dimension: nullStringPtr(row.Dimension),
			},
		}
		out = append(out, e)
		ids = append(ids, row.ID)
	}
	if len(ids) == 0 {
		return out, nil
	}
	tags, err := r.tagsFor(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	formats, err := r.formatsFor(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if t := tags[out[i].ID]; t != nil {
			out[i].Tags = t
		}
		if f := formats[out[i].ID]; f != nil {
			out[i].Formats = f
		}
	}
	return out, nil
}

func (r *TrackingRepository) tagsFor(ctx context.Context, userID int64, ids []int64) (map[int64][]string, error) {
	rows, err := r.q.ListTagsForWatchEntries(ctx, sqlc.ListTagsForWatchEntriesParams{UserID: userID, EntryIds: ids})
	if err != nil {
		return nil, fmt.Errorf("list tags for watches: %w", err)
	}
	out := make(map[int64][]string, len(ids))
	for _, row := range rows {
		out[row.EntryID] = append(out[row.EntryID], row.Name)
	}
	return out, nil
}

// UserTag is one of a user's tags and how many watches carry it.
type UserTag struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	WatchCount int64  `json:"watch_count"`
	CreatedAt  string `json:"created_at"`
}

// ListTags returns the user's tags, by name.
func (r *TrackingRepository) ListTags(ctx context.Context, userID int64) ([]UserTag, error) {
	rows, err := r.q.ListUserTags(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	out := make([]UserTag, 0, len(rows))
	for _, row := range rows {
		out = append(out, UserTag{ID: row.ID, Name: row.Name, WatchCount: row.WatchCount, CreatedAt: row.CreatedAt})
	}
	return out, nil
}

// DeleteTag removes one of the user's tags and detaches it from their watches.
func (r *TrackingRepository) DeleteTag(ctx context.Context, userID, id int64) (bool, error) {
	n, err := r.q.DeleteUserTag(ctx, sqlc.DeleteUserTagParams{ID: id, UserID: userID})
	if err != nil {
		return false, fmt.Errorf("delete tag %d: %w", id, err)
	}
	return n > 0, nil
}

// setTags replaces a watch's tags, creating any tag the user does not have.
func setTags(ctx context.Context, q *sqlc.Queries, userID, entryID int64, names []string) error {
	if err := q.ClearWatchEntryTags(ctx, entryID); err != nil {
		return fmt.Errorf("clear tags of watch %d: %w", entryID, err)
	}
	for _, name := range names {
		tag, err := q.UpsertUserTag(ctx, sqlc.UpsertUserTagParams{UserID: userID, Name: name})
		if err != nil {
			return fmt.Errorf("upsert tag %q: %w", name, err)
		}
		if err := q.AddWatchEntryTag(ctx, sqlc.AddWatchEntryTagParams{EntryID: entryID, TagID: tag.ID}); err != nil {
			return fmt.Errorf("tag watch %d with %q: %w", entryID, name, err)
		}
	}
	return nil
}

// rollup recomputes everything derived from one title's watches. It runs in
// the transaction of every write that adds, removes or moves a watch:
//   - watch_number and is_rewatch, renumbered in time order (per episode for a
//     show), so a watch logged late for an earlier date is still the first
//     viewing and a delete or edit closes the gaps;
//   - the user's state row (created if missing) and its roll-up: rating,
//     first/last watched, watch count.
func rollup(ctx context.Context, q *sqlc.Queries, userID int64, kind, tmdbID string) error {
	if err := q.RenumberWatches(ctx, sqlc.RenumberWatchesParams{UserID: userID, TmdbKind: kind, TmdbID: tmdbID}); err != nil {
		return fmt.Errorf("renumber watches %s/%s: %w", kind, tmdbID, err)
	}
	if err := q.SyncRewatchFlags(ctx, sqlc.SyncRewatchFlagsParams{UserID: userID, TmdbKind: kind, TmdbID: tmdbID}); err != nil {
		return fmt.Errorf("sync rewatch flags %s/%s: %w", kind, tmdbID, err)
	}
	if err := q.EnsureUserTitle(ctx, sqlc.EnsureUserTitleParams{UserID: userID, TmdbKind: kind, TmdbID: tmdbID}); err != nil {
		return fmt.Errorf("ensure user title %s/%s: %w", kind, tmdbID, err)
	}
	if err := q.RollupUserTitle(ctx, sqlc.RollupUserTitleParams{UserID: userID, TmdbKind: kind, TmdbID: tmdbID}); err != nil {
		return fmt.Errorf("rollup user title %s/%s: %w", kind, tmdbID, err)
	}
	return nil
}

func watchFromRow(row sqlc.WatchEntry) WatchEntry {
	return WatchEntry{
		ID: row.ID, ClientKey: nullStringPtr(row.ClientKey), SubjectKind: row.SubjectKind,
		Kind: row.TmdbKind, TmdbID: row.TmdbID,
		SeasonNumber: nullInt64Ptr(row.SeasonNumber), EpisodeNumber: nullInt64Ptr(row.EpisodeNumber),
		TmdbEpisodeID: nullStringPtr(row.TmdbEpisodeID),
		EndedAt:       row.EndedAt, StartedAt: nullStringPtr(row.StartedAt),
		RuntimeMinutesUsed: nullInt64Ptr(row.RuntimeMinutesUsed), RuntimeSource: nullStringPtr(row.RuntimeSource),
		Tz: nullStringPtr(row.Tz), UTCOffsetMinutes: nullInt64Ptr(row.UtcOffsetMinutes),
		WatchedOn: row.WatchedOn, TimePrecision: row.TimePrecision,
		Rating: nullInt64Ptr(row.Rating), Liked: row.Liked != 0, IsRewatch: row.IsRewatch != 0,
		WatchNumber: row.WatchNumber, Notes: nullStringPtr(row.Notes), VenueKind: nullStringPtr(row.VenueKind),
		Source: row.Source, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Tags: []string{},
		WatchContext: contextFrom(row), Formats: []WatchFormat{},
	}
}
