package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// UserImport is an import of a user's history from another tracker.
type UserImport struct {
	ID          int64
	UserID      int64
	Source      string
	FileName    string
	TZ          *string
	Status      string
	Error       *string
	Summary     string // JSON
	CreatedAt   string
	UpdatedAt   string
	CommittedAt *string
	UndoneAt    *string
}

func importFrom(row sqlc.UserImport) UserImport {
	return UserImport{
		ID: row.ID, UserID: row.UserID, Source: row.Source, FileName: row.FileName,
		TZ: nullStringPtr(row.Tz), Status: row.Status, Error: nullStringPtr(row.Error),
		Summary: row.Summary, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		CommittedAt: nullStringPtr(row.CommittedAt), UndoneAt: nullStringPtr(row.UndoneAt),
	}
}

// UserImportItem is one title of an import.
type UserImportItem struct {
	ID             int64
	ImportID       int64
	Kind           string // movie | tv
	SourceRef      string
	Title          string
	Year           *int64
	ReleaseDate    *string
	Data           string // JSON
	MatchStatus    string
	MatchMethod    *string
	TmdbID         *string
	Candidates     *string // JSON
	EpisodesTotal  int64
	EpisodesMapped int64
	Result         *string // JSON
	PrevState      *string // JSON
	CommittedAt    *string

	// What is known about the matched TMDB title (review list only).
	TmdbTitle       string
	TmdbReleaseDate string
	TmdbPosterPath  string
}

func importItemFrom(row sqlc.UserImportItem) UserImportItem {
	return UserImportItem{
		ID: row.ID, ImportID: row.ImportID, Kind: row.Kind, SourceRef: row.SourceRef, Title: row.Title,
		Year: nullInt64Ptr(row.Year), ReleaseDate: nullStringPtr(row.ReleaseDate), Data: row.Data,
		MatchStatus: row.MatchStatus, MatchMethod: nullStringPtr(row.MatchMethod),
		TmdbID: nullStringPtr(row.TmdbID), Candidates: nullStringPtr(row.Candidates),
		EpisodesTotal: row.EpisodesTotal, EpisodesMapped: row.EpisodesMapped,
		Result: nullStringPtr(row.Result), PrevState: nullStringPtr(row.PrevState),
		CommittedAt: nullStringPtr(row.CommittedAt),
	}
}

func importItemsFrom(rows []sqlc.UserImportItem) []UserImportItem {
	out := make([]UserImportItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, importItemFrom(r))
	}
	return out
}

// NewImportItem is a parsed title to stage.
type NewImportItem struct {
	Kind          string
	SourceRef     string
	Title         string
	Year          int
	ReleaseDate   string
	Data          string // JSON
	EpisodesTotal int64
}

// CreateUserImport stages an import and its items in one transaction, in the
// given status.
func (r *TrackingRepository) CreateUserImport(ctx context.Context, userID int64, source, fileName string, tz *string,
	status, summary string, items []NewImportItem) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)

	id, err := q.CreateUserImport(ctx, sqlc.CreateUserImportParams{
		UserID: userID, Source: source, FileName: fileName, Tz: toNullString(tz), Status: status, Summary: summary,
	})
	if err != nil {
		return 0, fmt.Errorf("create import: %w", err)
	}
	for _, it := range items {
		if err := q.InsertUserImportItem(ctx, sqlc.InsertUserImportItemParams{
			ImportID: id, Kind: it.Kind, SourceRef: it.SourceRef, Title: it.Title,
			Year: nullIfZero(it.Year), ReleaseDate: nullIfEmpty(it.ReleaseDate),
			Data: it.Data, EpisodesTotal: it.EpisodesTotal,
		}); err != nil {
			return 0, fmt.Errorf("stage import item %s %s: %w", it.Kind, it.SourceRef, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit import: %w", err)
	}
	return id, nil
}

// UserImport returns one of a user's imports, or sql.ErrNoRows.
func (r *TrackingRepository) UserImport(ctx context.Context, userID, id int64) (UserImport, error) {
	row, err := r.q.GetUserImport(ctx, sqlc.GetUserImportParams{ID: id, UserID: userID})
	if err != nil {
		return UserImport{}, err
	}
	return importFrom(row), nil
}

// ListUserImports returns a user's most recent imports, newest first.
func (r *TrackingRepository) ListUserImports(ctx context.Context, userID int64) ([]UserImport, error) {
	rows, err := r.q.ListUserImports(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list imports: %w", err)
	}
	out := make([]UserImport, 0, len(rows))
	for _, row := range rows {
		out = append(out, importFrom(row))
	}
	return out, nil
}

// NextActiveUserImport returns the oldest import with background work left,
// or nil when there is none.
func (r *TrackingRepository) NextActiveUserImport(ctx context.Context) (*UserImport, error) {
	row, err := r.q.NextActiveUserImport(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("next active import: %w", err)
	}
	imp := importFrom(row)
	return &imp, nil
}

// SetUserImportStatus moves an import to status, with an error message for
// "failed".
func (r *TrackingRepository) SetUserImportStatus(ctx context.Context, id int64, status string, errMsg *string) error {
	return r.q.SetUserImportStatus(ctx, sqlc.SetUserImportStatusParams{ID: id, Status: status, Error: toNullString(errMsg)})
}

// MarkUserImportCommitted marks an import done.
func (r *TrackingRepository) MarkUserImportCommitted(ctx context.Context, id int64) error {
	return r.q.MarkUserImportCommitted(ctx, id)
}

// MarkUserImportUndone marks an import undone.
func (r *TrackingRepository) MarkUserImportUndone(ctx context.Context, id int64) error {
	return r.q.MarkUserImportUndone(ctx, id)
}

// DeleteUserImport forgets an import and its items. Watches it wrote stay.
func (r *TrackingRepository) DeleteUserImport(ctx context.Context, userID, id int64) (bool, error) {
	n, err := r.q.DeleteUserImport(ctx, sqlc.DeleteUserImportParams{ID: id, UserID: userID})
	return n > 0, err
}

// ImportItemCount is how many items of one kind are in one match status.
type ImportItemCount struct {
	Kind           string
	MatchStatus    string
	Items          int64
	EpisodesTotal  int64
	EpisodesMapped int64
	Committed      int64
}

// UserImportCounts returns an import's items counted by kind and status.
func (r *TrackingRepository) UserImportCounts(ctx context.Context, importID int64) ([]ImportItemCount, error) {
	rows, err := r.q.CountUserImportItems(ctx, importID)
	if err != nil {
		return nil, fmt.Errorf("count import items: %w", err)
	}
	out := make([]ImportItemCount, 0, len(rows))
	for _, row := range rows {
		out = append(out, ImportItemCount{
			Kind: row.Kind, MatchStatus: row.MatchStatus, Items: row.Items,
			EpisodesTotal: row.EpisodesTotal, EpisodesMapped: row.EpisodesMapped, Committed: row.Committed,
		})
	}
	return out, nil
}

// ImportResults is what an import wrote, added up over its items.
type ImportResults struct {
	Added      int64 `json:"watches_added"`
	Duplicates int64 `json:"duplicates_skipped"`
	Unmapped   int64 `json:"episodes_unmapped"`
}

// UserImportResults adds up the results of an import's committed items.
func (r *TrackingRepository) UserImportResults(ctx context.Context, importID int64) (ImportResults, error) {
	row, err := r.q.SumUserImportResults(ctx, importID)
	if err != nil {
		return ImportResults{}, fmt.Errorf("sum import results: %w", err)
	}
	return ImportResults{Added: row.Added, Duplicates: row.Duplicates, Unmapped: row.Unmapped}, nil
}

// ImportItemsToMatch returns up to limit items still waiting to be matched.
func (r *TrackingRepository) ImportItemsToMatch(ctx context.Context, importID, limit int64) ([]UserImportItem, error) {
	rows, err := r.q.ListUserImportItemsToMatch(ctx, sqlc.ListUserImportItemsToMatchParams{ImportID: importID, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("import items to match: %w", err)
	}
	return importItemsFrom(rows), nil
}

// ImportItemsToCommit returns up to limit matched items not yet written.
func (r *TrackingRepository) ImportItemsToCommit(ctx context.Context, importID, limit int64) ([]UserImportItem, error) {
	rows, err := r.q.ListUserImportItemsToCommit(ctx, sqlc.ListUserImportItemsToCommitParams{ImportID: importID, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("import items to commit: %w", err)
	}
	return importItemsFrom(rows), nil
}

// CommittedImportItems returns every item an import wrote.
func (r *TrackingRepository) CommittedImportItems(ctx context.Context, importID int64) ([]UserImportItem, error) {
	rows, err := r.q.ListCommittedUserImportItems(ctx, importID)
	if err != nil {
		return nil, fmt.Errorf("committed import items: %w", err)
	}
	return importItemsFrom(rows), nil
}

// UserImportItem returns one item of an import, or sql.ErrNoRows.
func (r *TrackingRepository) UserImportItem(ctx context.Context, importID, id int64) (UserImportItem, error) {
	row, err := r.q.GetUserImportItem(ctx, sqlc.GetUserImportItemParams{ID: id, ImportID: importID})
	if err != nil {
		return UserImportItem{}, err
	}
	return importItemFrom(row), nil
}

// ImportItemFilter selects a page of an import's items.
type ImportItemFilter struct {
	Kind        string
	MatchStatus string
	Limit       int64
	Offset      int64
}

// ListUserImportItems returns a page of an import's items and the total.
func (r *TrackingRepository) ListUserImportItems(ctx context.Context, importID int64, f ImportItemFilter) ([]UserImportItem, int64, error) {
	rows, err := r.q.ListUserImportItems(ctx, sqlc.ListUserImportItemsParams{
		ImportID: importID, Kind: f.Kind, MatchStatus: f.MatchStatus, Limit: f.Limit, Offset: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list import items: %w", err)
	}
	total, err := r.q.CountUserImportItemsFiltered(ctx, sqlc.CountUserImportItemsFilteredParams{
		ImportID: importID, Kind: f.Kind, MatchStatus: f.MatchStatus,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count import items: %w", err)
	}
	out := make([]UserImportItem, 0, len(rows))
	for _, row := range rows {
		it := importItemFrom(sqlc.UserImportItem{
			ID: row.ID, ImportID: row.ImportID, Kind: row.Kind, SourceRef: row.SourceRef, Title: row.Title,
			Year: row.Year, ReleaseDate: row.ReleaseDate, Data: row.Data, MatchStatus: row.MatchStatus,
			MatchMethod: row.MatchMethod, TmdbID: row.TmdbID, Candidates: row.Candidates,
			EpisodesTotal: row.EpisodesTotal, EpisodesMapped: row.EpisodesMapped, Result: row.Result,
			PrevState: row.PrevState, CommittedAt: row.CommittedAt,
		})
		it.TmdbTitle, it.TmdbReleaseDate, it.TmdbPosterPath = row.TmdbTitle, row.TmdbReleaseDate, row.TmdbPosterPath
		out = append(out, it)
	}
	return out, total, nil
}

// ImportItemMatch is how an item was matched.
type ImportItemMatch struct {
	Status         string
	Method         *string
	TmdbID         *string
	Candidates     *string
	Data           string
	EpisodesMapped int64
}

// SaveImportItemMatch stores an item's match.
func (r *TrackingRepository) SaveImportItemMatch(ctx context.Context, id int64, m ImportItemMatch) error {
	return r.q.SaveUserImportItemMatch(ctx, sqlc.SaveUserImportItemMatchParams{
		ID: id, MatchStatus: m.Status, MatchMethod: toNullString(m.Method), TmdbID: toNullString(m.TmdbID),
		Candidates: toNullString(m.Candidates), Data: m.Data, EpisodesMapped: m.EpisodesMapped,
	})
}

// WatchTime is when a user watched (an episode of) a title.
type WatchTime struct {
	SeasonNumber  int64 // -1 for a movie
	EpisodeNumber int64 // -1 for a movie
	EndedAt       string
}

// TitleWatchTimes returns every watch a user has of one title.
func (r *TrackingRepository) TitleWatchTimes(ctx context.Context, userID int64, kind, tmdbID string) ([]WatchTime, error) {
	rows, err := r.q.ListTitleWatchTimes(ctx, sqlc.ListTitleWatchTimesParams{UserID: userID, TmdbKind: kind, TmdbID: tmdbID})
	if err != nil {
		return nil, fmt.Errorf("watch times %s/%s: %w", kind, tmdbID, err)
	}
	out := make([]WatchTime, 0, len(rows))
	for _, row := range rows {
		out = append(out, WatchTime{SeasonNumber: row.SeasonNumber, EpisodeNumber: row.EpisodeNumber, EndedAt: row.EndedAt})
	}
	return out, nil
}

// CommitImportItem writes an item's watches, refreshes the title's roll-up and
// marks the item committed with its result and the state it replaced, all in
// one transaction: an import interrupted half way never writes a title twice.
func (r *TrackingRepository) CommitImportItem(ctx context.Context, userID int64, item UserImportItem, watches []NewWatch,
	result, prevState string) error {
	if item.TmdbID == nil {
		return fmt.Errorf("import item %d has no tmdb id", item.ID)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)

	for _, w := range watches {
		if err := insertWatch(ctx, q, userID, w); err != nil {
			return err
		}
	}
	if len(watches) > 0 {
		if err := rollup(ctx, q, userID, item.Kind, *item.TmdbID); err != nil {
			return err
		}
	}
	if err := q.SaveUserImportItemCommit(ctx, sqlc.SaveUserImportItemCommitParams{
		ID: item.ID, Result: sql.NullString{String: result, Valid: true},
		PrevState: sql.NullString{String: prevState, Valid: true},
	}); err != nil {
		return fmt.Errorf("mark import item %d committed: %w", item.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit import item %d: %w", item.ID, err)
	}
	return nil
}

// DeleteImportedWatches removes the watches one import wrote for one title
// and refreshes its roll-up.
func (r *TrackingRepository) DeleteImportedWatches(ctx context.Context, userID, importID int64, kind, tmdbID string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)
	n, err := q.DeleteImportedWatches(ctx, sqlc.DeleteImportedWatchesParams{
		UserID: userID, ImportID: sql.NullInt64{Int64: importID, Valid: true}, TmdbKind: kind, TmdbID: tmdbID,
	})
	if err != nil {
		return 0, fmt.Errorf("delete imported watches %s/%s: %w", kind, tmdbID, err)
	}
	if err := rollup(ctx, q, userID, kind, tmdbID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit undo %s/%s: %w", kind, tmdbID, err)
	}
	return n, nil
}

// insertWatch inserts one watch with placeholder numbering; the caller runs
// rollup afterwards, which renumbers the title in time order.
func insertWatch(ctx context.Context, q *sqlc.Queries, userID int64, w NewWatch) error {
	liked := int64(0)
	if w.Liked {
		liked = 1
	}
	if _, err := q.InsertWatchEntry(ctx, sqlc.InsertWatchEntryParams{
		UserID: userID, ClientKey: toNullString(w.ClientKey), SubjectKind: w.SubjectKind,
		TmdbKind: w.Kind, TmdbID: w.TmdbID,
		SeasonNumber: toNullInt64(w.SeasonNumber), EpisodeNumber: toNullInt64(w.EpisodeNumber),
		TmdbEpisodeID: toNullString(w.TmdbEpisodeID),
		EndedAt:       w.EndedAt, StartedAt: toNullString(w.StartedAt),
		RuntimeMinutesUsed: toNullInt64(w.RuntimeMinutesUsed), RuntimeSource: toNullString(w.RuntimeSource),
		Tz: toNullString(w.Tz), UtcOffsetMinutes: toNullInt64(w.UTCOffsetMinutes),
		WatchedOn: w.WatchedOn, TimePrecision: w.TimePrecision,
		Rating: toNullInt64(w.Rating), Liked: liked, IsRewatch: 0, WatchNumber: 1,
		Notes: toNullString(w.Notes), VenueKind: toNullString(w.VenueKind), Source: w.Source,
		ImportID: toNullInt64(w.ImportID),
	}); err != nil {
		return fmt.Errorf("insert watch %s/%s: %w", w.Kind, w.TmdbID, err)
	}
	return nil
}

// MoviesWatchedAt returns the TMDB ids of the movies a user logged at exactly
// one of the given stored instants.
func (r *TrackingRepository) MoviesWatchedAt(ctx context.Context, userID int64, endedAt []string) ([]string, error) {
	if len(endedAt) == 0 {
		return nil, nil
	}
	ids, err := r.q.MoviesWatchedAt(ctx, sqlc.MoviesWatchedAtParams{UserID: userID, EndedAt: endedAt})
	if err != nil {
		return nil, fmt.Errorf("movies watched at: %w", err)
	}
	return ids, nil
}
