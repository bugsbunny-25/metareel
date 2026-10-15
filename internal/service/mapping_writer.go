package service

import (
	"context"
	"log/slog"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// Actors recorded on mapping events besides job types.
const (
	ActorAdmin       = "admin"        // admin API / UI
	ActorMappingSync = "mapping.sync" // sharing mappings over links
)

// MappingRefresh is the follow-up work after mapping changes: the TMDB
// titles newly mapped to (details, IMDb / Wikidata IDs) and the Netflix
// titles whose mapping changed (season numbers). The mapping.refresh job
// runs it (TitleMappingService.Refresh).
type MappingRefresh struct {
	Refs       []repository.TmdbRef `json:"refs"`
	NetflixIDs []int64              `json:"netflix_title_ids"`
	Reason     string               `json:"reason,omitempty"`
}

// Empty reports whether there is nothing to do.
func (r MappingRefresh) Empty() bool { return len(r.Refs)+len(r.NetflixIDs) == 0 }

// MappingWriter is the only way to change what a FlixPatrol or Netflix
// title is mapped to. Writes store evidence (what a source says a title is);
// the mappings of the titles they affect are then computed from evidence and
// links (mapping_compute.go), applied and recorded in mapping_events, all in
// one transaction (repository/mapping_write_repo.go). A session collects
// what the writes touched and hands it to the refresh queue once.
type MappingWriter struct {
	log   *slog.Logger
	repo  *repository.MappingRepository
	queue func(ctx context.Context, r MappingRefresh) error // nil: nothing follows
}

func NewMappingWriter(log *slog.Logger, repo *repository.MappingRepository) *MappingWriter {
	return &MappingWriter{log: log, repo: repo}
}

// WithRefreshQueue sets what runs the follow-up of mapping changes: in the
// server, enqueuing mapping.refresh. Without one, the TitleMappingService
// built with this writer runs it inline.
func (w *MappingWriter) WithRefreshQueue(queue func(ctx context.Context, r MappingRefresh) error) *MappingWriter {
	w.queue = queue
	return w
}

// MappingSession writes mapping changes on behalf of one actor (a job type,
// ActorAdmin or ActorMappingSync) and collects their follow-up.
type MappingSession struct {
	w       *MappingWriter
	actor   string
	pending MappingRefresh
	refs    map[repository.TmdbRef]bool
	netflix map[int64]bool
	changes int
}

func (w *MappingWriter) Session(actor string) *MappingSession {
	return &MappingSession{w: w, actor: actor, refs: map[repository.TmdbRef]bool{}, netflix: map[int64]bool{}}
}

// Changes is the number of recorded mapping changes so far.
func (s *MappingSession) Changes() int { return s.changes }

func (s *MappingSession) note(cs ...repository.MappingChange) {
	for _, c := range cs {
		s.changes++
		if c.To.ID != "" && (c.Changed() || c.Action == repository.MappingActionManual) {
			s.Touch(c.To)
		}
		if c.SubjectType == repository.SubjectNetflix && c.Changed() && !s.netflix[c.SubjectID] {
			s.netflix[c.SubjectID] = true
			s.pending.NetflixIDs = append(s.pending.NetflixIDs, c.SubjectID)
		}
	}
}

// Touch adds a TMDB title to the follow-up (e.g. its IDs were fixed by hand).
func (s *MappingSession) Touch(ref repository.TmdbRef) {
	if ref.ID == "" || s.refs[ref] {
		return
	}
	s.refs[ref] = true
	s.pending.Refs = append(s.pending.Refs, ref)
}

// Take returns the follow-up collected so far and forgets it.
func (s *MappingSession) Take() MappingRefresh {
	p := s.pending
	s.pending = MappingRefresh{}
	s.refs, s.netflix = map[repository.TmdbRef]bool{}, map[int64]bool{}
	return p
}

// Flush hands the collected follow-up to the refresh queue. A failure to
// queue it is logged, not returned: the writes are committed, and the
// nightly metadata job catches up.
func (s *MappingSession) Flush(ctx context.Context) {
	p := s.Take()
	if p.Empty() || s.w.queue == nil {
		return
	}
	p.Reason = "after mapping changes by " + s.actor
	if err := s.w.queue(ctx, p); err != nil {
		s.w.log.Warn("mapping refresh failed to start", slog.String("actor", s.actor), slog.Int("tmdb_titles", len(p.Refs)), slog.Any("err", err))
	}
}

// write runs fn in one mapping transaction and notes its changes.
func (s *MappingSession) write(ctx context.Context, fn func(tx *repository.MappingTx) error) error {
	changes, err := s.w.repo.Write(ctx, s.actor, fn)
	s.note(changes...)
	return err
}

// --- FlixPatrol titles ----------------------------------------------------------

// MatchTitle records an automatic matching attempt; what it found becomes
// the title's match evidence (and the IDs it came with the TMDB title's).
// sql.ErrNoRows when the title is manual (or gone).
func (s *MappingSession) MatchTitle(ctx context.Context, titleID int64, m repository.TitleMatchResult) (sqlc.Title, error) {
	var t sqlc.Title
	err := s.write(ctx, func(tx *repository.MappingTx) error {
		found := m.TmdbID != ""
		var err error
		if t, err = tx.RecordTitleMatchAttempt(titleID, found, m.MatchedName, m.MatchedYear, m.RetryAfter); err != nil || !found {
			return err
		}
		ref := repository.TmdbRef{Kind: tmdbKindOfTitle(m.Kind), ID: m.TmdbID}
		subj := repository.FlixPatrolSubject(titleID)
		if err := tx.PutEvidence(repository.Evidence{Subject: subj, Kind: repository.EvidenceMatch, Ref: ref, Source: m.Source, IMDbID: m.ImdbID}); err != nil {
			return err
		}
		if err := tx.SaveTmdbIDs(ref, repository.TmdbIDs{IMDbID: m.ImdbID, JustWatchID: m.JustWatchID}, repository.IDSourceMatch); err != nil {
			return err
		}
		if _, err := recompute(tx, []repository.Subject{subj}, repository.MappingActionMatch); err != nil {
			return err
		}
		t, err = tx.Title(titleID)
		return err
	})
	return t, err
}

// SetTitleMapping fixes a FlixPatrol title by hand (an empty tmdbID: as
// unmapped) and locks it against automatic matching; imdbID is what the fix
// came with. sql.ErrNoRows if missing.
func (s *MappingSession) SetTitleMapping(ctx context.Context, titleID int64, kind, tmdbID, imdbID string) (sqlc.Title, error) {
	var t sqlc.Title
	err := s.write(ctx, func(tx *repository.MappingTx) error {
		if _, err := tx.Title(titleID); err != nil {
			return err
		}
		subj := repository.FlixPatrolSubject(titleID)
		e := repository.Evidence{Subject: subj, Kind: repository.EvidenceManual, Source: repository.MatchStatusManual, IMDbID: imdbID}
		if tmdbID != "" {
			e.Ref = repository.TmdbRef{Kind: tmdbKindOfTitle(kind), ID: tmdbID}
		}
		if err := tx.PutEvidence(e); err != nil {
			return err
		}
		if _, err := recompute(tx, []repository.Subject{subj}, repository.MappingActionManual); err != nil {
			return err
		}
		var err error
		t, err = tx.Title(titleID)
		return err
	})
	return t, err
}

// RequeueTitle queues a title for matching now, lifting a manual lock;
// clear also forgets its evidence, and Netflix matches made through it.
// sql.ErrNoRows if missing.
func (s *MappingSession) RequeueTitle(ctx context.Context, titleID int64, clear bool) (sqlc.Title, error) {
	var t sqlc.Title
	err := s.write(ctx, func(tx *repository.MappingTx) error {
		if _, err := tx.Title(titleID); err != nil {
			return err
		}
		subj := repository.FlixPatrolSubject(titleID)
		action := repository.MappingActionRequeue
		var also []repository.Subject
		if clear {
			action = repository.MappingActionClear
			var err error
			if also, err = tx.DeleteNetflixEvidenceVia(titleID); err != nil {
				return err
			}
		}
		if err := forgetEvidence(tx, subj, clear); err != nil {
			return err
		}
		if err := tx.Requeue(subj, clear); err != nil {
			return err
		}
		if _, err := recompute(tx, []repository.Subject{subj}, action); err != nil {
			return err
		}
		if _, err := recompute(tx, also, ""); err != nil {
			return err
		}
		var err error
		t, err = tx.Title(titleID)
		return err
	})
	return t, err
}

// forgetEvidence forgets all of a subject's evidence (clear), or lifts its
// fix by hand: a fix that is all the title has stays as its match (as from
// no source) until matching finds another.
func forgetEvidence(tx *repository.MappingTx, subj repository.Subject, clear bool) error {
	if clear {
		_, err := tx.DeleteEvidence(subj, "")
		return err
	}
	list, err := tx.Evidence(subj)
	if err != nil {
		return err
	}
	var fix *repository.Evidence
	for i := range list {
		if list[i].Kind != repository.EvidenceManual {
			fix = nil
			break
		}
		fix = &list[i]
	}
	if fix != nil && fix.Ref.ID != "" {
		if err := tx.PutEvidence(repository.Evidence{Subject: subj, Kind: repository.EvidenceMatch, Ref: fix.Ref, IMDbID: fix.IMDbID}); err != nil {
			return err
		}
	}
	_, err = tx.DeleteEvidence(subj, repository.EvidenceManual)
	return err
}

// --- Netflix titles -------------------------------------------------------------

// MatchNetflixTitle records an automatic matching attempt of a Netflix title
// (manual titles are left alone).
func (s *MappingSession) MatchNetflixTitle(ctx context.Context, id int64, m repository.NetflixMatch) error {
	return s.write(ctx, func(tx *repository.MappingTx) error {
		n, err := tx.NetflixTitle(id)
		if err != nil || n.MatchStatus == repository.MatchStatusManual {
			return err
		}
		found := m.TmdbID != ""
		if err := tx.RecordNetflixMatchAttempt(id, found, m.RetryAfter); err != nil || !found {
			return err
		}
		ref := repository.TmdbRef{Kind: m.TmdbKind, ID: m.TmdbID}
		subj := repository.NetflixSubject(id)
		if err := tx.PutEvidence(repository.Evidence{Subject: subj, Kind: repository.EvidenceMatch, Ref: ref, Source: m.Source,
			IMDbID: m.ImdbID, ViaTitleID: m.TitleID}); err != nil {
			return err
		}
		if err := tx.SaveTmdbIDs(ref, repository.TmdbIDs{IMDbID: m.ImdbID}, repository.IDSourceMatch); err != nil {
			return err
		}
		_, err = recompute(tx, []repository.Subject{subj}, repository.MappingActionMatch)
		return err
	})
}

// MatchNetflixByNetflixID records that a Netflix title's Netflix ID belongs
// to ref (stronger than any match); reports whether its mapping changed.
// Manual titles are left alone.
func (s *MappingSession) MatchNetflixByNetflixID(ctx context.Context, id int64, ref repository.TmdbRef, imdbID string) (bool, error) {
	changed := false
	err := s.write(ctx, func(tx *repository.MappingTx) error {
		n, err := tx.NetflixTitle(id)
		if err != nil || n.MatchStatus == repository.MatchStatusManual {
			return err
		}
		subj := repository.NetflixSubject(id)
		if err := tx.PutEvidence(repository.Evidence{Subject: subj, Kind: repository.EvidenceNetflixID, Ref: ref,
			Source: MatchSourceNetflixID, IMDbID: imdbID}); err != nil {
			return err
		}
		if err := tx.SaveTmdbIDs(ref, repository.TmdbIDs{IMDbID: imdbID}, repository.IDSourceMatch); err != nil {
			return err
		}
		before := len(tx.Changes())
		if _, err := recompute(tx, []repository.Subject{subj}, repository.MappingActionMatch); err != nil {
			return err
		}
		for _, c := range tx.Changes()[before:] {
			if c.SubjectType == repository.SubjectNetflix && c.SubjectID == id {
				changed = true
			}
		}
		return nil
	})
	return changed, err
}

// SetNetflixMapping fixes a Netflix title by hand (empty tmdbID: as
// unmapped; tmdbKind is movie | tv) and locks it. sql.ErrNoRows if missing.
func (s *MappingSession) SetNetflixMapping(ctx context.Context, id int64, tmdbKind, tmdbID, imdbID string) error {
	return s.write(ctx, func(tx *repository.MappingTx) error {
		if _, err := tx.NetflixTitle(id); err != nil {
			return err
		}
		subj := repository.NetflixSubject(id)
		e := repository.Evidence{Subject: subj, Kind: repository.EvidenceManual, Source: repository.MatchStatusManual, IMDbID: imdbID}
		if tmdbID != "" {
			e.Ref = repository.TmdbRef{Kind: tmdbKind, ID: tmdbID}
		}
		if err := tx.PutEvidence(e); err != nil {
			return err
		}
		_, err := recompute(tx, []repository.Subject{subj}, repository.MappingActionManual)
		return err
	})
}

// RequeueNetflixTitle queues a Netflix title for matching, lifting a manual
// lock; clear also forgets its evidence. sql.ErrNoRows if missing.
func (s *MappingSession) RequeueNetflixTitle(ctx context.Context, id int64, clear bool) error {
	return s.write(ctx, func(tx *repository.MappingTx) error {
		if _, err := tx.NetflixTitle(id); err != nil {
			return err
		}
		subj := repository.NetflixSubject(id)
		action := repository.MappingActionRequeue
		if clear {
			action = repository.MappingActionClear
		}
		if err := forgetEvidence(tx, subj, clear); err != nil {
			return err
		}
		if err := tx.Requeue(subj, clear); err != nil {
			return err
		}
		_, err := recompute(tx, []repository.Subject{subj}, action)
		return err
	})
}

// --- both ----------------------------------------------------------------------

// Remap fixes everything now mapped to from onto to by hand, returning the
// FlixPatrol and Netflix titles moved.
func (s *MappingSession) Remap(ctx context.Context, from, to repository.TmdbRef) (titleIDs, netflixIDs []int64, err error) {
	err = s.write(ctx, func(tx *repository.MappingTx) error {
		subjects, err := tx.SubjectsOnRef(from)
		if err != nil {
			return err
		}
		for _, subj := range subjects {
			if err := tx.PutEvidence(repository.Evidence{Subject: subj, Kind: repository.EvidenceManual, Ref: to, Source: repository.MatchStatusManual}); err != nil {
				return err
			}
			if subj.Type == repository.SubjectNetflix {
				netflixIDs = append(netflixIDs, subj.ID)
			} else {
				titleIDs = append(titleIDs, subj.ID)
			}
		}
		_, err = recompute(tx, subjects, repository.MappingActionRemap)
		return err
	})
	return titleIDs, netflixIDs, err
}

// Recompute recomputes the mappings of subjects (and the titles linked to
// them), recording what changed as shared or withdrawn mappings.
func (s *MappingSession) Recompute(ctx context.Context, subjects []repository.Subject) (recomputeStats, error) {
	var st recomputeStats
	err := s.write(ctx, func(tx *repository.MappingTx) error {
		var err error
		st, err = recompute(tx, subjects, "")
		return err
	})
	return st, err
}

// Events lists recorded mapping changes, newest first.
func (w *MappingWriter) Events(ctx context.Context, f repository.MappingEventFilter) ([]repository.MappingEvent, error) {
	return w.repo.MappingEvents(ctx, f)
}
