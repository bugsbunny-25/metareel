package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// Every change to what a FlixPatrol or Netflix title is mapped to goes
// through a MappingTx (MappingRepository.Write): evidence is written, the
// mappings of the titles it affects are computed from their evidence and
// their links (by the service) and applied, and each change is recorded in
// mapping_events, all in one transaction. Nothing else writes titles.tmdb_id
// or netflix_titles.tmdb_id.

// Mapping subjects (mapping_events.subject_type, mapping_evidence.subject_type).
const (
	SubjectFlixPatrol = "flixpatrol" // titles
	SubjectNetflix    = "netflix"    // netflix_titles
)

// Evidence kinds (mapping_evidence.kind), strongest first.
const (
	EvidenceManual    = "manual"     // fixed by hand (possibly as unmapped)
	EvidenceNetflixID = "netflix_id" // a Netflix title's Netflix ID
	EvidenceMatch     = "match"      // automatic match
)

// Mapping change actions (mapping_events.action).
const (
	MappingActionMatch   = "match"   // automatic match
	MappingActionManual  = "manual"  // fixed by hand
	MappingActionAdopt   = "adopt"   // took a linked title's mapping
	MappingActionRemap   = "remap"   // moved with everything on its TMDB title
	MappingActionClear   = "clear"   // match forgotten by hand
	MappingActionUnshare = "unshare" // a linked title's mapping it had taken went away (back to its own, or unmapped)
	MappingActionRequeue = "requeue" // queued for matching again (lifts a manual lock)
)

// Subject is a FlixPatrol or Netflix title.
type Subject struct {
	Type string // SubjectFlixPatrol | SubjectNetflix
	ID   int64
}

func (s Subject) String() string { return fmt.Sprintf("%s:%d", s.Type, s.ID) }

// FlixPatrolSubject / NetflixSubject build subjects.
func FlixPatrolSubject(id int64) Subject { return Subject{Type: SubjectFlixPatrol, ID: id} }
func NetflixSubject(id int64) Subject    { return Subject{Type: SubjectNetflix, ID: id} }

// Evidence is what one source says a title is.
type Evidence struct {
	Subject
	Kind       string  // EvidenceManual | EvidenceNetflixID | EvidenceMatch
	Ref        TmdbRef // zero: fixed by hand as unmapped
	Source     string  // e.g. justwatch_path, wikidata_netflix, netflix_id, manual
	IMDbID     string  // the IMDb ID it came with
	ViaTitleID int64   // a Netflix match through a same-name FlixPatrol title
}

// Mapping is a title's stored (computed) mapping.
type Mapping struct {
	Ref        TmdbRef
	Status     string // match_status; "" keeps the stored one
	Source     string // match_source
	ViaTitleID int64  // Netflix titles: the FlixPatrol title it was matched through
	OwnMatch   bool   // FlixPatrol titles: from its own automatic match (keeps the matched name)
}

// MappingInputs is what computing mappings needs: the evidence of the
// subjects and of everything linked to them, all links, and the subjects'
// stored mappings.
type MappingInputs struct {
	Evidence map[Subject][]Evidence
	Links    []TitleLink
	Current  map[Subject]Mapping
}

// Neighbours returns the subjects linked to s.
func (in MappingInputs) Neighbours(s Subject) []Subject {
	var out []Subject
	for _, l := range in.Links {
		switch {
		case s.Type == SubjectNetflix && l.NetflixTitleID == s.ID:
			out = append(out, FlixPatrolSubject(l.TitleID))
		case s.Type == SubjectFlixPatrol && l.TitleID == s.ID:
			out = append(out, NetflixSubject(l.NetflixTitleID))
		}
	}
	return out
}

// MappingChange is one title's mapping change. From / To are zero while
// unmapped.
type MappingChange struct {
	SubjectType string
	SubjectID   int64
	Action      string
	From, To    TmdbRef
	Source      string // match source after the change
}

// Changed reports whether the TMDB title changed.
func (c MappingChange) Changed() bool { return c.From != c.To }

// recorded reports whether a change is worth an event: automatic changes
// only when the TMDB title changed, admin actions always.
func (c MappingChange) recorded() bool {
	switch c.Action {
	case MappingActionMatch, MappingActionAdopt, MappingActionUnshare:
		return c.Changed()
	}
	return true
}

// MappingTx writes mapping evidence and computed mappings in one transaction.
type MappingTx struct {
	ctx     context.Context
	q       *sqlc.Queries
	actor   string
	changes []MappingChange
	titles  map[int64]sqlc.Title // rows read or written in this transaction
	netflix map[int64]sqlc.NetflixTitle
}

// Write runs fn in a transaction and returns the mapping changes it
// recorded.
func (r *MappingRepository) Write(ctx context.Context, actor string, fn func(tx *MappingTx) error) ([]MappingChange, error) {
	sqlTx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	tx := &MappingTx{ctx: ctx, q: r.q.WithTx(sqlTx), actor: actor, titles: map[int64]sqlc.Title{}, netflix: map[int64]sqlc.NetflixTitle{}}
	if err := fn(tx); err != nil {
		return nil, err
	}
	if err := sqlTx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return tx.changes, nil
}

// --- evidence --------------------------------------------------------------------

// PutEvidence stores (replaces) one piece of evidence, making sure the TMDB
// title it names has a row.
func (tx *MappingTx) PutEvidence(e Evidence) error {
	if e.Ref.ID != "" {
		if err := tx.q.EnsureTmdbTitle(tx.ctx, sqlc.EnsureTmdbTitleParams{TmdbKind: e.Ref.Kind, TmdbID: e.Ref.ID}); err != nil {
			return fmt.Errorf("ensure tmdb title %s: %w", e.Ref, err)
		}
	}
	if err := tx.q.UpsertMappingEvidence(tx.ctx, sqlc.UpsertMappingEvidenceParams{
		SubjectType: e.Type, SubjectID: e.ID, Kind: e.Kind,
		TmdbKind: nullIfEmpty(e.Ref.Kind), TmdbID: nullIfEmpty(e.Ref.ID), Source: nullIfEmpty(e.Source),
		ImdbID: nullIfEmpty(e.IMDbID), ViaTitleID: sql.NullInt64{Int64: e.ViaTitleID, Valid: e.ViaTitleID > 0},
	}); err != nil {
		return fmt.Errorf("put %s evidence of %s: %w", e.Kind, e.Subject, err)
	}
	return nil
}

// DeleteEvidence deletes one kind of a subject's evidence, or all of it
// (kind ""). Reports whether anything was deleted.
func (tx *MappingTx) DeleteEvidence(s Subject, kind string) (bool, error) {
	n, err := tx.q.DeleteMappingEvidence(tx.ctx, sqlc.DeleteMappingEvidenceParams{SubjectType: s.Type, SubjectID: s.ID, Kind: kind})
	if err != nil {
		return false, fmt.Errorf("delete evidence of %s: %w", s, err)
	}
	return n > 0, nil
}

// Evidence returns a subject's evidence.
func (tx *MappingTx) Evidence(s Subject) ([]Evidence, error) {
	rows, err := tx.q.ListMappingEvidence(tx.ctx, sqlc.ListMappingEvidenceParams{SubjectType: s.Type, Ids: []int64{s.ID}})
	if err != nil {
		return nil, fmt.Errorf("list evidence of %s: %w", s, err)
	}
	out := make([]Evidence, 0, len(rows))
	for _, e := range rows {
		out = append(out, evidenceFromRow(e))
	}
	return out, nil
}

// DeleteNetflixEvidenceVia deletes Netflix matches made through a FlixPatrol
// title, returning the Netflix titles they belonged to.
func (tx *MappingTx) DeleteNetflixEvidenceVia(titleID int64) ([]Subject, error) {
	ids, err := tx.q.DeleteNetflixEvidenceVia(tx.ctx, sql.NullInt64{Int64: titleID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("delete netflix evidence via %d: %w", titleID, err)
	}
	out := make([]Subject, 0, len(ids))
	for _, id := range ids {
		out = append(out, NetflixSubject(id))
	}
	return out, nil
}

// SaveTmdbIDs stores IDs of a TMDB title found by matching (see
// MetadataRepository.SaveIDs).
func (tx *MappingTx) SaveTmdbIDs(ref TmdbRef, ids TmdbIDs, source string) error {
	return saveTmdbIDs(tx.ctx, tx.q, ref, ids, source)
}

// --- matching bookkeeping ----------------------------------------------------------

// RecordTitleMatchAttempt records an automatic matching attempt of a
// FlixPatrol title. sql.ErrNoRows when the title is manual (or gone).
func (tx *MappingTx) RecordTitleMatchAttempt(titleID int64, found bool, matchedName string, matchedYear int, retryAfter string) (sqlc.Title, error) {
	t, err := tx.q.RecordTitleMatchAttempt(tx.ctx, sqlc.RecordTitleMatchAttemptParams{
		Found: boolInt(found), MatchedName: nullIfEmpty(matchedName),
		MatchedYear: sql.NullInt64{Int64: int64(matchedYear), Valid: matchedYear > 0}, RetryAfter: retryAfter, ID: titleID,
	})
	if err == nil {
		tx.titles[t.ID] = t
	}
	return t, err
}

// RecordNetflixMatchAttempt records an automatic matching attempt of a
// Netflix title (manual titles are left alone).
func (tx *MappingTx) RecordNetflixMatchAttempt(id int64, found bool, retryAfter string) error {
	delete(tx.netflix, id)
	return tx.q.RecordNetflixMatchAttempt(tx.ctx, sqlc.RecordNetflixMatchAttemptParams{Found: boolInt(found), RetryAfter: retryAfter, ID: id})
}

// Requeue makes a title due for matching now; clear also forgets its
// matching attempts.
func (tx *MappingTx) Requeue(s Subject, clear bool) error {
	if s.Type == SubjectNetflix {
		delete(tx.netflix, s.ID)
		return tx.q.RequeueNetflixTitleMatch(tx.ctx, sqlc.RequeueNetflixTitleMatchParams{Clear: boolInt(clear), ID: s.ID})
	}
	delete(tx.titles, s.ID)
	return tx.q.RequeueTitleMatch(tx.ctx, sqlc.RequeueTitleMatchParams{Clear: boolInt(clear), ID: s.ID})
}

// --- reading -------------------------------------------------------------------

// Title returns a FlixPatrol title (sql.ErrNoRows if missing).
func (tx *MappingTx) Title(id int64) (sqlc.Title, error) {
	if t, ok := tx.titles[id]; ok {
		return t, nil
	}
	t, err := tx.q.GetTitleByID(tx.ctx, id)
	if err == nil {
		tx.titles[id] = t
	}
	return t, err
}

// NetflixTitle returns a Netflix title (sql.ErrNoRows if missing).
func (tx *MappingTx) NetflixTitle(id int64) (sqlc.NetflixTitle, error) {
	if n, ok := tx.netflix[id]; ok {
		return n, nil
	}
	n, err := tx.q.GetNetflixTitleRow(tx.ctx, id)
	if err == nil {
		tx.netflix[id] = n
	}
	return n, err
}

// SubjectsOnRef returns the titles now mapped to ref.
func (tx *MappingTx) SubjectsOnRef(ref TmdbRef) ([]Subject, error) {
	fp, err := tx.q.ListTitleIDsOnRef(tx.ctx, sqlc.ListTitleIDsOnRefParams{Kind: titleKindOf(ref.Kind), TmdbID: nullIfEmpty(ref.ID)})
	if err != nil {
		return nil, err
	}
	nf, err := tx.q.ListNetflixTitleIDsOnRef(tx.ctx, sqlc.ListNetflixTitleIDsOnRefParams{TmdbKind: nullIfEmpty(ref.Kind), TmdbID: nullIfEmpty(ref.ID)})
	if err != nil {
		return nil, err
	}
	var out []Subject
	for _, id := range fp {
		out = append(out, FlixPatrolSubject(id))
	}
	for _, id := range nf {
		out = append(out, NetflixSubject(id))
	}
	return out, nil
}

// Links returns every link.
func (tx *MappingTx) Links() ([]TitleLink, error) {
	rows, err := tx.q.ListTitleLinks(tx.ctx)
	if err != nil {
		return nil, fmt.Errorf("list title links: %w", err)
	}
	out := make([]TitleLink, 0, len(rows))
	for _, l := range rows {
		out = append(out, TitleLink{NetflixTitleID: l.NetflixTitleID, TitleID: l.TitleID})
	}
	return out, nil
}

// Changes returns the changes recorded so far.
func (tx *MappingTx) Changes() []MappingChange { return tx.changes }

// LinkedSubjects returns every title that has a link.
func (r *MappingRepository) LinkedSubjects(ctx context.Context) ([]Subject, error) {
	links, err := r.q.ListTitleLinks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list title links: %w", err)
	}
	seen := map[Subject]bool{}
	var out []Subject
	for _, l := range links {
		for _, s := range []Subject{NetflixSubject(l.NetflixTitleID), FlixPatrolSubject(l.TitleID)} {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out, nil
}

// Inputs loads what computing the mappings of subjects needs. Subjects
// that do not exist are left out of Current.
func (tx *MappingTx) Inputs(subjects []Subject) (MappingInputs, error) {
	in := MappingInputs{Evidence: map[Subject][]Evidence{}, Current: map[Subject]Mapping{}}
	links, err := tx.q.ListTitleLinks(tx.ctx)
	if err != nil {
		return in, fmt.Errorf("list title links: %w", err)
	}
	for _, l := range links {
		in.Links = append(in.Links, TitleLink{NetflixTitleID: l.NetflixTitleID, TitleID: l.TitleID})
	}
	need := map[Subject]bool{}
	for _, s := range subjects {
		need[s] = true
		for _, n := range in.Neighbours(s) {
			need[n] = true
		}
	}
	ids := map[string][]int64{}
	for s := range need {
		ids[s.Type] = append(ids[s.Type], s.ID)
	}
	for typ, list := range ids {
		sort.Slice(list, func(i, j int) bool { return list[i] < list[j] })
		rows, err := tx.q.ListMappingEvidence(tx.ctx, sqlc.ListMappingEvidenceParams{SubjectType: typ, Ids: list})
		if err != nil {
			return in, fmt.Errorf("list mapping evidence: %w", err)
		}
		for _, e := range rows {
			ev := evidenceFromRow(e)
			in.Evidence[ev.Subject] = append(in.Evidence[ev.Subject], ev)
		}
	}
	var fpIDs, nfIDs []int64
	for _, s := range subjects {
		if s.Type == SubjectNetflix {
			nfIDs = append(nfIDs, s.ID)
		} else {
			fpIDs = append(fpIDs, s.ID)
		}
	}
	if len(fpIDs) > 0 {
		rows, err := tx.q.ListTitlesByIDs(tx.ctx, fpIDs)
		if err != nil {
			return in, fmt.Errorf("list titles: %w", err)
		}
		for _, t := range rows {
			tx.titles[t.ID] = t
			in.Current[FlixPatrolSubject(t.ID)] = titleMapping(t)
		}
	}
	if len(nfIDs) > 0 {
		rows, err := tx.q.ListNetflixTitleRowsByIDs(tx.ctx, nfIDs)
		if err != nil {
			return in, fmt.Errorf("list netflix titles: %w", err)
		}
		for _, n := range rows {
			tx.netflix[n.ID] = n
			in.Current[NetflixSubject(n.ID)] = netflixMapping(n)
		}
	}
	return in, nil
}

// --- applying ------------------------------------------------------------------

// Apply stores a subject's computed mapping when it differs from the stored
// one, and records the change under action.
func (tx *MappingTx) Apply(s Subject, cur, m Mapping, action string) error {
	if m.Status == "" {
		m.Status = cur.Status
	}
	same := m.Ref == cur.Ref && m.Status == cur.Status && m.Source == cur.Source && m.ViaTitleID == cur.ViaTitleID
	if same && (action == MappingActionMatch || action == MappingActionAdopt || action == MappingActionUnshare) {
		return nil
	}
	if !same {
		if s.Type == SubjectNetflix {
			n, err := tx.q.ApplyNetflixMapping(tx.ctx, sqlc.ApplyNetflixMappingParams{
				TmdbKind: nullIfEmpty(m.Ref.Kind), TmdbID: nullIfEmpty(m.Ref.ID),
				TitleID:     sql.NullInt64{Int64: m.ViaTitleID, Valid: m.ViaTitleID > 0},
				MatchStatus: m.Status, MatchSource: nullIfEmpty(m.Source), ID: s.ID,
			})
			if err != nil {
				return fmt.Errorf("apply mapping of %s: %w", s, err)
			}
			tx.netflix[n.ID] = n
		} else {
			kind := ""
			if m.Ref.ID != "" {
				kind = titleKindOf(m.Ref.Kind)
			}
			t, err := tx.q.ApplyTitleMapping(tx.ctx, sqlc.ApplyTitleMappingParams{
				Kind: kind, TmdbID: nullIfEmpty(m.Ref.ID), MatchStatus: m.Status, MatchSource: nullIfEmpty(m.Source),
				OwnMatch: boolInt(m.OwnMatch), ID: s.ID,
			})
			if err != nil {
				return fmt.Errorf("apply mapping of %s: %w", s, err)
			}
			tx.titles[t.ID] = t
		}
	}
	c := MappingChange{SubjectType: s.Type, SubjectID: s.ID, Action: action, From: cur.Ref, To: m.Ref, Source: m.Source}
	if !c.recorded() {
		return nil
	}
	if err := tx.q.InsertMappingEvent(tx.ctx, sqlc.InsertMappingEventParams{
		SubjectType: c.SubjectType, SubjectID: c.SubjectID, Action: c.Action,
		FromTmdbKind: nullIfEmpty(c.From.Kind), FromTmdbID: nullIfEmpty(c.From.ID),
		ToTmdbKind: nullIfEmpty(c.To.Kind), ToTmdbID: nullIfEmpty(c.To.ID),
		Source: nullIfEmpty(c.Source), Actor: tx.actor,
	}); err != nil {
		return fmt.Errorf("record mapping event: %w", err)
	}
	tx.changes = append(tx.changes, c)
	return nil
}

// --- conversions -----------------------------------------------------------------

// TitleRef is the TMDB title a FlixPatrol title is mapped to (zero if none).
func TitleRef(t sqlc.Title) TmdbRef {
	if t.TmdbID.String == "" {
		return TmdbRef{}
	}
	kind := "movie"
	if t.Kind == "tv_show" {
		kind = "tv"
	}
	return TmdbRef{Kind: kind, ID: t.TmdbID.String}
}

// NetflixTitleRef is the TMDB title a Netflix title is mapped to (zero if none).
func NetflixTitleRef(n sqlc.NetflixTitle) TmdbRef {
	if n.TmdbID.String == "" || (n.TmdbKind.String != "movie" && n.TmdbKind.String != "tv") {
		return TmdbRef{}
	}
	return TmdbRef{Kind: n.TmdbKind.String, ID: n.TmdbID.String}
}

func titleMapping(t sqlc.Title) Mapping {
	return Mapping{Ref: TitleRef(t), Status: t.MatchStatus, Source: t.MatchSource.String}
}

func netflixMapping(n sqlc.NetflixTitle) Mapping {
	return Mapping{Ref: NetflixTitleRef(n), Status: n.MatchStatus, Source: n.MatchSource.String, ViaTitleID: n.TitleID.Int64}
}

func evidenceFromRow(e sqlc.MappingEvidence) Evidence {
	ev := Evidence{Subject: Subject{Type: e.SubjectType, ID: e.SubjectID}, Kind: e.Kind, Source: e.Source.String,
		IMDbID: e.ImdbID.String, ViaTitleID: e.ViaTitleID.Int64}
	if e.TmdbID.String != "" && (e.TmdbKind.String == "movie" || e.TmdbKind.String == "tv") {
		ev.Ref = TmdbRef{Kind: e.TmdbKind.String, ID: e.TmdbID.String}
	}
	return ev
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// --- evidence and history (reads) --------------------------------------------

// EvidenceOf returns a subject's evidence.
func (r *MappingRepository) EvidenceOf(ctx context.Context, s Subject) ([]Evidence, error) {
	rows, err := r.q.ListMappingEvidence(ctx, sqlc.ListMappingEvidenceParams{SubjectType: s.Type, Ids: []int64{s.ID}})
	if err != nil {
		return nil, fmt.Errorf("list evidence of %s: %w", s, err)
	}
	out := make([]Evidence, 0, len(rows))
	for _, e := range rows {
		out = append(out, evidenceFromRow(e))
	}
	return out, nil
}

// MappingEventFilter selects mapping events: one subject and/or one TMDB
// title (as the old or the new mapping); zero values = any.
type MappingEventFilter struct {
	SubjectType string
	SubjectID   int64
	Ref         TmdbRef
	Limit       int64
}

// MappingEvent is one recorded mapping change.
type MappingEvent struct {
	ID          int64     `json:"id"`
	SubjectType string    `json:"subject_type"`
	SubjectID   int64     `json:"subject_id"`
	SubjectName string    `json:"subject_name"`
	Action      string    `json:"action"`
	FromKind    *string   `json:"from_tmdb_kind"`
	FromID      *string   `json:"from_tmdb_id"`
	ToKind      *string   `json:"to_tmdb_kind"`
	ToID        *string   `json:"to_tmdb_id"`
	Source      *string   `json:"source"`
	Actor       string    `json:"actor"`
	CreatedAt   time.Time `json:"created_at"`
}

// MappingEvents lists mapping events, newest first.
func (r *MappingRepository) MappingEvents(ctx context.Context, f MappingEventFilter) ([]MappingEvent, error) {
	rows, err := r.q.ListMappingEvents(ctx, sqlc.ListMappingEventsParams{
		SubjectType: f.SubjectType, SubjectID: f.SubjectID, TmdbKind: f.Ref.Kind, TmdbID: f.Ref.ID, Limit: f.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list mapping events: %w", err)
	}
	out := make([]MappingEvent, 0, len(rows))
	for _, e := range rows {
		out = append(out, MappingEvent{
			ID: e.ID, SubjectType: e.SubjectType, SubjectID: e.SubjectID, SubjectName: e.SubjectName, Action: e.Action,
			FromKind: nullStringPtr(e.FromTmdbKind), FromID: nullStringPtr(e.FromTmdbID),
			ToKind: nullStringPtr(e.ToTmdbKind), ToID: nullStringPtr(e.ToTmdbID),
			Source: nullStringPtr(e.Source), Actor: e.Actor, CreatedAt: e.CreatedAt.UTC(),
		})
	}
	return out, nil
}

// IMDbMismatch is a mapped title whose deciding evidence came with an IMDb
// ID other than its TMDB title's.
type IMDbMismatch struct {
	SubjectType  string `json:"subject_type"`
	SubjectID    int64  `json:"subject_id"`
	Name         string `json:"name"`
	TmdbKind     string `json:"tmdb_kind"`
	TmdbID       string `json:"tmdb_id"`
	IMDbID       string `json:"imdb_id"`      // what the match / fix came with
	TmdbIMDbID   string `json:"tmdb_imdb_id"` // the TMDB title's
	EvidenceKind string `json:"evidence_kind"`
	Source       string `json:"source"`
}

// IMDbMismatches lists up to limit titles whose evidence's IMDb ID disagrees
// with their TMDB title's.
func (r *MappingRepository) IMDbMismatches(ctx context.Context, limit int64) ([]IMDbMismatch, error) {
	rows, err := r.q.ListIMDbMismatches(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list imdb mismatches: %w", err)
	}
	out := make([]IMDbMismatch, 0, len(rows))
	for _, m := range rows {
		out = append(out, IMDbMismatch{SubjectType: m.SubjectType, SubjectID: m.SubjectID, Name: m.Name, TmdbKind: m.TmdbKind,
			TmdbID: m.TmdbID, IMDbID: m.ImdbID, TmdbIMDbID: m.TmdbImdbID, EvidenceKind: m.EvidenceKind, Source: m.Source})
	}
	return out, nil
}

// NetflixIDMismatch is a Netflix title whose own Netflix ID (Tudum) differs
// from its TMDB title's (from a stronger source than a copied one).
type NetflixIDMismatch struct {
	NetflixTitleID    int64  `json:"netflix_title_id"`
	ShowTitle         string `json:"show_title"`
	List              string `json:"list"` // Netflix's list: movie (Films) | tv_show (TV)
	TmdbKind          string `json:"tmdb_kind"`
	TmdbID            string `json:"tmdb_id"`
	TmdbTitle         string `json:"tmdb_title"`
	NetflixID         string `json:"netflix_id"`          // the Netflix title's own
	TmdbNetflixID     string `json:"tmdb_netflix_id"`     // the TMDB title's
	TmdbNetflixSource string `json:"tmdb_netflix_source"` // manual | tmdb | wikidata | match
	MatchStatus       string `json:"match_status"`
	MatchSource       string `json:"match_source"`
	// Another Netflix title mapped to the same TMDB title has its Netflix ID:
	// this one is likely a special or part of that show, not a wrong match.
	OtherTitleHasID bool `json:"other_title_has_id"`
}

// NetflixIDMismatches lists up to limit Netflix titles whose Netflix ID
// disagrees with their TMDB title's: likely wrong mappings first, titles
// fixed by hand last.
func (r *MappingRepository) NetflixIDMismatches(ctx context.Context, limit int64) ([]NetflixIDMismatch, error) {
	rows, err := r.q.ListNetflixIDMismatches(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list netflix id mismatches: %w", err)
	}
	out := make([]NetflixIDMismatch, 0, len(rows))
	for _, m := range rows {
		out = append(out, NetflixIDMismatch{NetflixTitleID: m.ID, ShowTitle: m.ShowTitle, List: m.Kind, TmdbKind: m.TmdbKind,
			TmdbID: m.TmdbID, TmdbTitle: m.TmdbTitle, NetflixID: m.NetflixID, TmdbNetflixID: m.TmdbNetflixID,
			TmdbNetflixSource: m.TmdbNetflixSource, MatchStatus: m.MatchStatus, MatchSource: m.MatchSource,
			OtherTitleHasID: m.OtherTitleHasID == 1})
	}
	return out, nil
}

// ContradictedNetflixLabelMatches lists up to limit Netflix titles mapped
// only by a Wikidata label match whose own Netflix ID differs from their
// TMDB title's (the label matched another title of the same name).
func (r *MappingRepository) ContradictedNetflixLabelMatches(ctx context.Context, limit int64) ([]int64, error) {
	ids, err := r.q.ListContradictedNetflixLabelMatches(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list contradicted netflix label matches: %w", err)
	}
	return ids, nil
}
