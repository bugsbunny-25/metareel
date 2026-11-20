package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Import statuses. An import is matched in the background, waits in "ready"
// for the user to review it, and is written in the background.
const (
	ImportMatching  = "matching"
	ImportReady     = "ready"
	ImportImporting = "importing"
	ImportDone      = "done"
	ImportUndone    = "undone"
	ImportFailed    = "failed"
)

// Item match statuses.
const (
	ItemPending   = "pending"
	ItemMatched   = "matched"
	ItemUnmatched = "unmatched"
	ItemSkipped   = "skipped"
)

const (
	// MaxImportUploadBytes bounds an uploaded export.
	MaxImportUploadBytes = 64 << 20
	// importMatchWorkers is how many titles are matched at once; TMDB
	// allows far more, but an import is not in a hurry.
	importMatchWorkers = 4
	// maxEpisodeFinds bounds the per-show TheTVDB episode lookups used for
	// episodes whose numbers TMDB does not have.
	maxEpisodeFinds = 200
	// rewatchWindow: a rewatch within this long of a watch already logged is
	// the same viewing, recorded twice.
	rewatchWindow = 36 * time.Hour
	// importIdlePoll is how often the worker looks for work when nobody
	// has woken it, which also resumes imports after a restart.
	importIdlePoll = 30 * time.Second
)

// importTMDB is the part of the TMDB client imports need.
type importTMDB interface {
	Enabled() bool
	SearchMovie(ctx context.Context, query string) ([]client.TMDBSearchResult, error)
	SearchMovieYear(ctx context.Context, query string, year int) ([]client.TMDBSearchResult, error)
	SearchTV(ctx context.Context, query string) ([]client.TMDBSearchResult, error)
	FindByTVDB(ctx context.Context, tvdbID string) (client.TMDBTVDBFound, error)
	DiscoverMovies(ctx context.Context, in client.DiscoverMoviesInput) ([]client.TMDBSearchResult, error)
}

// ImportService brings a user's history in from TV Time and Suite exports.
//
// An upload is parsed and staged at once; matching titles to TMDB and
// writing the watches happen in the background (Run), driven entirely by the
// staged rows, so a restart picks an import up where it stopped. Watches are
// written through the same roll-up as hand-logged ones and are tagged with
// the import, so an import can be undone as a whole.
//
// Duplicates are never written: a first viewing is only added when the user
// has no watch of that movie or episode yet (from any source, this import's
// own earlier items included), and a rewatch only when no watch of it is
// within a day and a half. Importing the same history twice, or the same
// history from two apps, therefore only fills the gaps.
type ImportService struct {
	log      *slog.Logger
	repo     *repository.TrackingRepository
	tracking *TrackingService
	tmdb     importTMDB
	wake     chan struct{}
	now      func() time.Time
}

func NewImportService(log *slog.Logger, repo *repository.TrackingRepository, tracking *TrackingService, tmdb importTMDB) *ImportService {
	return &ImportService{log: log, repo: repo, tracking: tracking, tmdb: tmdb, wake: make(chan struct{}, 1), now: time.Now}
}

func (s *ImportService) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Upload parses an export and stages it; matching starts in the background.
// tz is the IANA zone the user watched in, which decides the local day of
// each watch.
func (s *ImportService) Upload(ctx context.Context, userID int64, fileName, tz string, body []byte) (map[string]any, error) {
	if len(body) == 0 {
		return nil, &ValidationError{Message: "the upload is empty; send the export zip as the request body"}
	}
	if len(body) > MaxImportUploadBytes {
		return nil, &ValidationError{Message: fmt.Sprintf("the upload is larger than %d MB", MaxImportUploadBytes>>20)}
	}
	var zone *string
	if tz = strings.TrimSpace(tz); tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			return nil, &ValidationError{Message: "tz must be an IANA time zone name, e.g. America/New_York"}
		}
		zone = &tz
	}
	if !s.tmdb.Enabled() {
		return nil, fmt.Errorf("%w: TMDB_API_KEY is not set, so titles cannot be matched", ErrUpstream)
	}
	parsed, err := parseExport(body)
	if err != nil {
		return nil, err
	}
	items := make([]repository.NewImportItem, 0, len(parsed.Titles))
	for _, t := range parsed.Titles {
		data, err := json.Marshal(t.Data)
		if err != nil {
			return nil, fmt.Errorf("encode import item: %w", err)
		}
		items = append(items, repository.NewImportItem{
			Kind: t.Kind, SourceRef: t.SourceRef, Title: t.Title, Year: t.Year, ReleaseDate: t.ReleaseDate,
			Data: string(data), EpisodesTotal: t.Data.episodeCount(),
		})
	}
	summary, err := json.Marshal(parsed.Summary)
	if err != nil {
		return nil, fmt.Errorf("encode import summary: %w", err)
	}
	fileName = strings.TrimSpace(fileName)
	if len(fileName) > 200 {
		fileName = fileName[:200]
	}
	id, err := s.repo.CreateUserImport(ctx, userID, parsed.Source, fileName, zone, ImportMatching, string(summary), items)
	if err != nil {
		return nil, err
	}
	s.log.Info("import staged", "import_id", id, "user_id", userID, "source", parsed.Source, "titles", len(items))
	s.poke()
	return s.Get(ctx, userID, id)
}

// List returns the user's recent imports.
func (s *ImportService) List(ctx context.Context, userID int64) (map[string]any, error) {
	imps, err := s.repo.ListUserImports(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(imps))
	for _, imp := range imps {
		p, err := s.payload(ctx, imp)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return map[string]any{"imports": out}, nil
}

// Get returns one import with its progress.
func (s *ImportService) Get(ctx context.Context, userID, id int64) (map[string]any, error) {
	imp, err := s.repo.UserImport(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	return s.payload(ctx, imp)
}

type importTally struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Matched   int64 `json:"matched"`
	Unmatched int64 `json:"unmatched"`
	Skipped   int64 `json:"skipped"`
	Imported  int64 `json:"imported"`
	// Shows only: episode watches in the export and how many of them were
	// matched to a TMDB episode.
	EpisodesTotal  int64 `json:"episodes_total,omitempty"`
	EpisodesMapped int64 `json:"episodes_mapped,omitempty"`
}

func (s *ImportService) payload(ctx context.Context, imp repository.UserImport) (map[string]any, error) {
	counts, err := s.repo.UserImportCounts(ctx, imp.ID)
	if err != nil {
		return nil, err
	}
	tallies := map[string]*importTally{"movie": {}, "tv": {}}
	for _, c := range counts {
		t := tallies[c.Kind]
		if t == nil {
			continue
		}
		t.Total += c.Items
		t.Imported += c.Committed
		t.EpisodesTotal += c.EpisodesTotal
		if c.MatchStatus == ItemMatched {
			t.EpisodesMapped += c.EpisodesMapped
		}
		switch c.MatchStatus {
		case ItemPending:
			t.Pending += c.Items
		case ItemMatched:
			t.Matched += c.Items
		case ItemUnmatched:
			t.Unmatched += c.Items
		case ItemSkipped:
			t.Skipped += c.Items
		}
	}
	out := map[string]any{
		"id": imp.ID, "source": imp.Source, "file_name": imp.FileName, "status": imp.Status,
		"created_at": imp.CreatedAt, "updated_at": imp.UpdatedAt,
		"movies": tallies["movie"], "shows": tallies["tv"],
	}
	if imp.TZ != nil {
		out["tz"] = *imp.TZ
	}
	if imp.Error != nil {
		out["error"] = *imp.Error
	}
	if imp.CommittedAt != nil {
		out["committed_at"] = *imp.CommittedAt
	}
	if imp.UndoneAt != nil {
		out["undone_at"] = *imp.UndoneAt
	}
	var summary map[string]any
	if json.Unmarshal([]byte(imp.Summary), &summary) == nil {
		out["summary"] = summary
	}
	if imp.Status == ImportImporting || imp.Status == ImportDone || imp.Status == ImportUndone {
		res, err := s.repo.UserImportResults(ctx, imp.ID)
		if err != nil {
			return nil, err
		}
		out["results"] = res
	}
	return out, nil
}

// ImportItemsInput selects a page of an import's items.
type ImportItemsInput struct {
	Kind   string
	Match  string
	Limit  int64
	Offset int64
}

// Items returns a page of an import's items, unmatched ones first.
func (s *ImportService) Items(ctx context.Context, userID, id int64, in ImportItemsInput) (map[string]any, error) {
	if _, err := s.repo.UserImport(ctx, userID, id); err != nil {
		return nil, err
	}
	if in.Kind != "" && in.Kind != "movie" && in.Kind != "tv" {
		return nil, &ValidationError{Message: "kind must be movie or tv"}
	}
	switch in.Match {
	case "", ItemPending, ItemMatched, ItemUnmatched, ItemSkipped:
	default:
		return nil, &ValidationError{Message: "match must be one of pending, matched, unmatched, skipped"}
	}
	if in.Limit <= 0 {
		in.Limit = 50
	}
	if in.Limit > 200 {
		in.Limit = 200
	}
	if in.Offset < 0 {
		in.Offset = 0
	}
	rows, total, err := s.repo.ListUserImportItems(ctx, id, repository.ImportItemFilter{
		Kind: in.Kind, MatchStatus: in.Match, Limit: in.Limit, Offset: in.Offset,
	})
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		items = append(items, itemPayload(r))
	}
	return map[string]any{"items": items, "total": total, "limit": in.Limit, "offset": in.Offset}, nil
}

func itemPayload(it repository.UserImportItem) map[string]any {
	var d importData
	_ = json.Unmarshal([]byte(it.Data), &d)
	watches, rewatches := 0, 0
	for _, w := range d.Watches {
		if w.Rewatch {
			rewatches++
		} else {
			watches++
		}
	}
	out := map[string]any{
		"id": it.ID, "kind": it.Kind, "title": it.Title, "match_status": it.MatchStatus,
		"watches": watches, "rewatches": rewatches,
		"watchlist": d.Watchlist, "favorite": d.Favorite, "stopped": d.Stopped, "paused": d.Paused,
	}
	if it.Year != nil {
		out["year"] = *it.Year
	}
	if it.ReleaseDate != nil {
		out["release_date"] = *it.ReleaseDate
	}
	if it.Kind == "tv" {
		out["episodes_total"] = it.EpisodesTotal
		out["episodes_mapped"] = it.EpisodesMapped
	}
	if it.MatchMethod != nil {
		out["match_method"] = *it.MatchMethod
	}
	if it.TmdbID != nil {
		tmdb := map[string]any{"tmdb_kind": it.Kind, "tmdb_id": *it.TmdbID, "title": it.TmdbTitle}
		if it.TmdbReleaseDate != "" {
			tmdb["release_date"] = it.TmdbReleaseDate
		}
		if it.TmdbPosterPath != "" {
			tmdb["poster_path"] = it.TmdbPosterPath
		}
		out["tmdb"] = tmdb
	}
	if it.Candidates != nil {
		var c []TitleSearchResult
		if json.Unmarshal([]byte(*it.Candidates), &c) == nil && len(c) > 0 {
			out["candidates"] = c
		}
	}
	if it.Result != nil {
		var r map[string]any
		if json.Unmarshal([]byte(*it.Result), &r) == nil {
			out["result"] = r
		}
	}
	return out
}

// ImportItemInput fixes one item during review: match it to a TMDB title,
// or skip it.
type ImportItemInput struct {
	TmdbID *string `json:"tmdb_id"`
	Skip   *bool   `json:"skip"`
}

// SetItem matches an item by hand or skips it (or un-skips it, which puts
// it back to unmatched). Only an import waiting for review can be changed.
func (s *ImportService) SetItem(ctx context.Context, userID, id, itemID int64, in ImportItemInput) (map[string]any, error) {
	imp, err := s.repo.UserImport(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if imp.Status != ImportReady {
		return nil, &ConflictError{Message: "items can only be changed while the import is waiting for review"}
	}
	it, err := s.repo.UserImportItem(ctx, id, itemID)
	if err != nil {
		return nil, err
	}
	var d importData
	if err := json.Unmarshal([]byte(it.Data), &d); err != nil {
		return nil, fmt.Errorf("decode import item %d: %w", it.ID, err)
	}
	m := repository.ImportItemMatch{Data: it.Data, Candidates: it.Candidates}
	switch {
	case in.TmdbID != nil:
		tmdbID := strings.TrimSpace(*in.TmdbID)
		if err := ValidateRef(it.Kind, tmdbID); err != nil {
			return nil, err
		}
		if _, err := s.tracking.titles.Resolve(ctx, it.Kind, tmdbID); err != nil {
			return nil, err
		}
		method := "manual"
		m.Status, m.Method, m.TmdbID = ItemMatched, &method, &tmdbID
		if it.Kind == "tv" {
			mapped, err := s.mapEpisodes(ctx, userID, tmdbID, &d)
			if err != nil {
				return nil, err
			}
			m.EpisodesMapped = mapped
		}
	case in.Skip != nil && *in.Skip:
		m.Status, m.Method, m.TmdbID, m.EpisodesMapped = ItemSkipped, it.MatchMethod, it.TmdbID, it.EpisodesMapped
	case in.Skip != nil:
		if it.TmdbID != nil {
			m.Status, m.Method, m.TmdbID, m.EpisodesMapped = ItemMatched, it.MatchMethod, it.TmdbID, it.EpisodesMapped
		} else {
			m.Status = ItemUnmatched
		}
	default:
		return nil, &ValidationError{Message: "send tmdb_id to match the item, or skip"}
	}
	data, err := json.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("encode import item: %w", err)
	}
	m.Data = string(data)
	if err := s.repo.SaveImportItemMatch(ctx, it.ID, m); err != nil {
		return nil, err
	}
	updated, err := s.repo.UserImportItem(ctx, id, it.ID)
	if err != nil {
		return nil, err
	}
	if updated.TmdbID != nil {
		snap, err := s.repo.TitleSnapshot(ctx, updated.Kind, *updated.TmdbID)
		if err != nil {
			return nil, err
		}
		updated.TmdbTitle, updated.TmdbReleaseDate, updated.TmdbPosterPath = snap.Title, snap.ReleaseDate, snap.PosterPath
	}
	return itemPayload(updated), nil
}

// Commit starts writing a reviewed import. Unmatched and skipped items are
// left out.
func (s *ImportService) Commit(ctx context.Context, userID, id int64) (map[string]any, error) {
	imp, err := s.repo.UserImport(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if imp.Status != ImportReady {
		return nil, &ConflictError{Message: "only an import waiting for review can be imported (this one is " + imp.Status + ")"}
	}
	if err := s.repo.SetUserImportStatus(ctx, id, ImportImporting, nil); err != nil {
		return nil, err
	}
	s.poke()
	return s.Get(ctx, userID, id)
}

// Retry resumes a failed import from where it stopped: matching when titles
// are still waiting for a match, writing when it had started writing, and
// otherwise back to review.
func (s *ImportService) Retry(ctx context.Context, userID, id int64) (map[string]any, error) {
	imp, err := s.repo.UserImport(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if imp.Status != ImportFailed {
		return nil, &ConflictError{Message: "only a failed import can be retried (this one is " + imp.Status + ")"}
	}
	counts, err := s.repo.UserImportCounts(ctx, id)
	if err != nil {
		return nil, err
	}
	next := ImportReady
	for _, c := range counts {
		if c.MatchStatus == ItemPending && c.Items > 0 {
			next = ImportMatching
			break
		}
		if c.Committed > 0 {
			next = ImportImporting
		}
	}
	if err := s.repo.SetUserImportStatus(ctx, id, next, nil); err != nil {
		return nil, err
	}
	s.poke()
	return s.Get(ctx, userID, id)
}

// Undo removes every watch an import wrote and puts back each title's state
// (status, favourite, watchlist place) as it was before. Watches logged by
// hand since are kept.
func (s *ImportService) Undo(ctx context.Context, userID, id int64) (map[string]any, error) {
	imp, err := s.repo.UserImport(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if imp.Status != ImportDone {
		return nil, &ConflictError{Message: "only a finished import can be undone (this one is " + imp.Status + ")"}
	}
	items, err := s.repo.CommittedImportItems(ctx, id)
	if err != nil {
		return nil, err
	}
	removed := int64(0)
	for _, it := range items {
		if it.TmdbID == nil {
			continue
		}
		n, err := s.repo.DeleteImportedWatches(ctx, userID, id, it.Kind, *it.TmdbID)
		if err != nil {
			return nil, err
		}
		removed += n
		if err := s.restoreState(ctx, userID, it); err != nil {
			return nil, err
		}
	}
	if err := s.repo.MarkUserImportUndone(ctx, id); err != nil {
		return nil, err
	}
	s.log.Info("import undone", "import_id", id, "user_id", userID, "watches_removed", removed)
	out, err := s.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	out["watches_removed"] = removed
	return out, nil
}

// savedState is a title's user state before an import changed it.
type savedState struct {
	Status            string  `json:"status"`
	Favorite          bool    `json:"favorite"`
	WatchlistPosition *int64  `json:"watchlist_position,omitempty"`
	WatchlistNote     *string `json:"watchlist_note,omitempty"`
}

func (s *ImportService) restoreState(ctx context.Context, userID int64, it repository.UserImportItem) error {
	var prev *savedState
	if it.PrevState != nil {
		if err := json.Unmarshal([]byte(*it.PrevState), &prev); err != nil {
			return fmt.Errorf("decode saved state of import item %d: %w", it.ID, err)
		}
	}
	st := repository.UserTitleState{}
	if prev != nil {
		st = repository.UserTitleState{Status: prev.Status, Favorite: prev.Favorite,
			WatchlistPosition: prev.WatchlistPosition, WatchlistNote: prev.WatchlistNote}
	}
	cur, err := s.repo.UserTitle(ctx, userID, it.Kind, *it.TmdbID)
	if err != nil || cur == nil {
		return err
	}
	// SaveUserTitleState drops the row when nothing is left in it.
	if err := s.repo.SaveUserTitleState(ctx, userID, it.Kind, *it.TmdbID, st); err != nil {
		return err
	}
	if it.Kind == "tv" && cur.WatchCount > 0 {
		return s.tracking.advanceShowStatus(ctx, userID, *it.TmdbID, false)
	}
	return nil
}

// Delete forgets an import. A finished import's watches are kept (undo it
// first to remove them); an import still being worked on cannot be deleted.
func (s *ImportService) Delete(ctx context.Context, userID, id int64) error {
	imp, err := s.repo.UserImport(ctx, userID, id)
	if err != nil {
		return err
	}
	if imp.Status == ImportMatching || imp.Status == ImportImporting {
		return &ConflictError{Message: "the import is still being worked on; wait for it to finish"}
	}
	ok, err := s.repo.DeleteUserImport(ctx, userID, id)
	if err != nil {
		return err
	}
	if !ok {
		return sql.ErrNoRows
	}
	return nil
}

// Run works through imports with background work left until ctx ends.
func (s *ImportService) Run(ctx context.Context) {
	t := time.NewTicker(importIdlePoll)
	defer t.Stop()
	for {
		for ctx.Err() == nil {
			imp, err := s.repo.NextActiveUserImport(ctx)
			if err != nil {
				s.log.Warn("import worker: looking for work failed", "err", err)
				break
			}
			if imp == nil {
				break
			}
			s.process(ctx, *imp)
		}
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-t.C:
		}
	}
}

func (s *ImportService) process(ctx context.Context, imp repository.UserImport) {
	var err error
	switch imp.Status {
	case ImportMatching:
		err = s.matchAll(ctx, imp)
	case ImportImporting:
		err = s.commitAll(ctx, imp)
	}
	if err == nil || ctx.Err() != nil {
		return
	}
	s.log.Error("import failed", "import_id", imp.ID, "status", imp.Status, "err", err)
	msg := "the import stopped because of an internal error; retry it"
	if errors.Is(err, ErrUpstream) {
		msg = "TMDB could not be reached; retry the import later"
	}
	if serr := s.repo.SetUserImportStatus(context.WithoutCancel(ctx), imp.ID, ImportFailed, &msg); serr != nil {
		s.log.Error("import: marking failed", "import_id", imp.ID, "err", serr)
	}
}

// matchAll matches every pending item of an import, a batch at a time.
func (s *ImportService) matchAll(ctx context.Context, imp repository.UserImport) error {
	for {
		items, err := s.repo.ImportItemsToMatch(ctx, imp.ID, 40)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			break
		}
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(importMatchWorkers)
		var mu sync.Mutex
		upstreamErrors := 0
		for _, it := range items {
			g.Go(func() error {
				m, err := s.matchItem(gctx, imp.UserID, it)
				if err != nil {
					if gctx.Err() != nil {
						return gctx.Err()
					}
					// One failed lookup leaves the title for the user to
					// match by hand rather than failing the import; many in
					// a row means TMDB is down.
					s.log.Warn("import: matching a title failed", "import_id", imp.ID, "item_id", it.ID, "err", err)
					mu.Lock()
					upstreamErrors++
					tooMany := upstreamErrors > 10
					mu.Unlock()
					if tooMany {
						return err
					}
					m = repository.ImportItemMatch{Status: ItemUnmatched, Data: it.Data}
				}
				return s.repo.SaveImportItemMatch(gctx, it.ID, m)
			})
		}
		if err := g.Wait(); err != nil {
			return err
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	s.log.Info("import matched", "import_id", imp.ID)
	return s.repo.SetUserImportStatus(ctx, imp.ID, ImportReady, nil)
}

// commitAll writes every matched item of an import, a batch at a time.
func (s *ImportService) commitAll(ctx context.Context, imp repository.UserImport) error {
	for {
		items, err := s.repo.ImportItemsToCommit(ctx, imp.ID, 50)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := s.commitItem(ctx, imp, it); err != nil {
				return fmt.Errorf("import item %d (%s %s): %w", it.ID, it.Kind, it.Title, err)
			}
		}
	}
	res, err := s.repo.UserImportResults(ctx, imp.ID)
	if err != nil {
		return err
	}
	s.log.Info("import written", "import_id", imp.ID, "user_id", imp.UserID,
		"watches_added", res.Added, "duplicates", res.Duplicates, "unmapped", res.Unmapped)
	return s.repo.MarkUserImportCommitted(ctx, imp.ID)
}
