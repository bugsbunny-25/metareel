package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
	"github.com/bugsbunny-25/metareel/internal/tasks"
)

// TitleMappingService keeps one TMDB-keyed mapping across sources. TMDB
// titles are the hub; FlixPatrol titles and Netflix's official Top 10 titles
// both point at one. A Netflix title and a FlixPatrol title that are the same
// title (same name, same Netflix list, on Netflix's charts in the same or an
// adjacent week) are linked, and a mapping found or fixed on one side is
// shared with the other:
//
//   - a title fixed by hand wins (two that disagree are a conflict and left
//     for review);
//   - else a Netflix title mapped through its Netflix ID;
//   - else FlixPatrol's automatic match (made with the title page's year);
//   - else Netflix's automatic match.
//
// Each title weighs only itself and the titles directly linked to it.
//
// The TMDB kind comes with the mapping, so a show in Netflix's TV list can
// map to a TMDB movie (a stand-up special) and vice versa.
type TitleMappingService struct {
	log      *slog.Logger
	repo     *repository.MappingRepository
	writer   *MappingWriter
	meta     *repository.MetadataRepository
	metadata *TitleMetadataService // optional: TMDB details / Wikidata for newly mapped titles
	seasons  *SeasonMapper         // optional: remaps seasons of Netflix titles that changed
}

// NewTitleMappingService builds the mapping service around writer. Without a
// refresh queue on writer, follow-ups of mapping changes run inline.
func NewTitleMappingService(log *slog.Logger, repo *repository.MappingRepository, writer *MappingWriter, meta *repository.MetadataRepository, metadata *TitleMetadataService, seasons *SeasonMapper) *TitleMappingService {
	s := &TitleMappingService{log: log, repo: repo, writer: writer, meta: meta, metadata: metadata, seasons: seasons}
	if writer.queue == nil {
		writer.queue = func(ctx context.Context, r MappingRefresh) error {
			_, err := s.Refresh(ctx, r, nil)
			return err
		}
	}
	return s
}

// Writer is the mapping writer every mapping change goes through.
func (s *TitleMappingService) Writer() *MappingWriter { return s.writer }

// MappingSyncReport summarises a sync.
type MappingSyncReport struct {
	Links             int `json:"links"`
	LinksAdded        int `json:"links_added"`
	LinksRemoved      int `json:"links_removed"`
	NetflixUpdated    int `json:"netflix_titles_updated"`    // took a linked title's mapping
	FlixPatrolUpdated int `json:"flixpatrol_titles_updated"` // took a linked title's mapping
	Conflicts         int `json:"conflicts"`                 // titles whose linked titles disagree (they keep their own match)
}

// Match sources of mappings shared over a link.
const (
	MatchSourceFlixPatrolTitle = "flixpatrol_title" // a Netflix title took a linked FlixPatrol title's mapping
	MatchSourceNetflixTitle    = "netflix_title"    // a FlixPatrol title took a linked Netflix title's mapping
)

// Sync recomputes the links (when refreshLinks) and the mappings of every
// linked title (and of titles that lost a link) from their evidence. It
// only touches the database; TMDB details and season numbers of what
// changed follow in a mapping refresh.
func (s *TitleMappingService) Sync(ctx context.Context, refreshLinks bool, runLog func(level, msg string)) (*MappingSyncReport, error) {
	if s == nil {
		return &MappingSyncReport{}, nil
	}
	session := s.writer.Session(ActorMappingSync)
	defer session.Flush(ctx)
	return s.sync(ctx, refreshLinks, runLog, session)
}

// SyncIn is Sync writing through the caller's session (one follow-up for
// the caller's changes and what they spread to).
func (s *TitleMappingService) SyncIn(ctx context.Context, session *MappingSession, refreshLinks bool, runLog func(level, msg string)) (*MappingSyncReport, error) {
	if s == nil {
		return &MappingSyncReport{}, nil
	}
	return s.sync(ctx, refreshLinks, runLog, session)
}

func (s *TitleMappingService) sync(ctx context.Context, refreshLinks bool, runLog func(level, msg string), session *MappingSession) (*MappingSyncReport, error) {
	rep := &MappingSyncReport{}
	var removed []repository.TitleLink
	if refreshLinks {
		var err error
		if removed, err = s.refreshLinks(ctx, rep); err != nil {
			return rep, err
		}
	}
	subjects, err := s.repo.LinkedSubjects(ctx)
	if err != nil {
		return rep, err
	}
	for _, l := range removed {
		subjects = append(subjects, repository.NetflixSubject(l.NetflixTitleID), repository.FlixPatrolSubject(l.TitleID))
	}
	st, err := session.Recompute(ctx, subjects)
	if err != nil {
		return rep, err
	}
	rep.NetflixUpdated, rep.FlixPatrolUpdated, rep.Conflicts = st.NetflixAdopted, st.FlixPatrolAdopted, st.Conflicts
	if runLog != nil && (refreshLinks || rep.NetflixUpdated+rep.FlixPatrolUpdated+rep.Conflicts > 0) {
		msg := fmt.Sprintf("Mapping: %d Netflix titles and %d FlixPatrol titles took a linked title's TMDB mapping", rep.NetflixUpdated, rep.FlixPatrolUpdated)
		if refreshLinks {
			msg += fmt.Sprintf("; %d links (%d new, %d removed)", rep.Links, rep.LinksAdded, rep.LinksRemoved)
		}
		if rep.Conflicts > 0 {
			msg += fmt.Sprintf("; %d title(s) kept their own match because their linked titles disagree (see TMDB mapping → conflicts)", rep.Conflicts)
		}
		runLog("info", msg)
	}
	return rep, nil
}

// MappingRefreshReport summarises a mapping.refresh run.
type MappingRefreshReport struct {
	TmdbTitles      int                `json:"tmdb_titles"`
	DetailsFetched  int                `json:"details_fetched"`
	DetailsFresh    int                `json:"details_fresh"` // fetched recently, skipped
	DetailsFailed   int                `json:"details_failed"`
	WikidataChecked int                `json:"wikidata_checked"` // never looked up before
	WikidataFound   int                `json:"wikidata_found"`
	NetflixTitles   int                `json:"netflix_titles"` // whose seasons were remapped
	Sync            *MappingSyncReport `json:"sync,omitempty"` // run by hand: mappings recomputed
}

// detailsFreshFor is how long fetched TMDB details count as current for a
// mapping refresh (the metadata job refreshes them on its own schedule).
const detailsFreshFor = 24 * time.Hour

// Refresh does the follow-up of mapping changes (the mapping.refresh job):
// fetches TMDB details and Wikidata IDs of the TMDB titles now mapped to,
// remaps the seasons of Netflix titles that changed and re-tags recent TV
// rankings. Run with nothing to follow up, it recomputes every linked
// title's mapping first.
func (s *TitleMappingService) Refresh(ctx context.Context, r MappingRefresh, runLog func(level, msg string)) (*MappingRefreshReport, error) {
	rep := &MappingRefreshReport{}
	logf := func(level, format string, args ...any) {
		if runLog != nil {
			runLog(level, fmt.Sprintf(format, args...))
		}
	}
	// Run by hand (nothing to follow up): recompute every linked title's
	// mapping, and refresh what that changes.
	session := s.writer.Session(tasks.TypeMappingRefresh)
	if r.Empty() {
		sr, err := s.sync(ctx, false, runLog, session)
		rep.Sync = sr
		if err != nil {
			return rep, err
		}
	}
	shared := session.Take()
	for _, ref := range r.Refs {
		session.Touch(ref)
	}
	for _, ref := range shared.Refs {
		session.Touch(ref)
	}
	refs := session.Take().Refs
	netflixIDs := mergeIDs(r.NetflixIDs, shared.NetflixIDs)
	rep.TmdbTitles = len(refs)

	if s.metadata != nil && len(refs) > 0 {
		have, err := s.meta.GetMany(ctx, refs)
		if err != nil {
			return rep, err
		}
		fetched := 0
		for _, ref := range refs {
			if err := ctx.Err(); err != nil {
				return rep, err
			}
			// Recent details are kept, unless the IMDb ID is missing (e.g. a
			// hand-set one was just handed back to TMDB).
			if m := have[ref.String()]; m != nil && m.DetailsFetchedAt != nil && time.Since(*m.DetailsFetchedAt) < detailsFreshFor && m.IMDbID != nil {
				rep.DetailsFresh++
				continue
			}
			if fetched > 0 {
				if err := sleepCtx(ctx, tmdbRequestGap); err != nil {
					return rep, err
				}
			}
			fetched++
			if _, err := s.metadata.RefreshDetails(ctx, ref); err != nil {
				rep.DetailsFailed++
				s.log.Warn("tmdb details for mapping refresh failed", slog.String("ref", ref.String()), slog.Any("err", err))
				if rep.DetailsFailed <= 5 {
					logf("warn", "%s: TMDB details failed: %s", ref, truncateErr(err, 200))
				}
				continue
			}
			rep.DetailsFetched++
		}
		// Wikidata only for TMDB titles never looked up: the metadata job
		// rechecks the rest, and Wikidata's endpoint throttles bursts.
		var newRefs []repository.TmdbRef
		for _, ref := range refs {
			if m := have[ref.String()]; m == nil || !m.WikidataLookedUp {
				newRefs = append(newRefs, ref)
			}
		}
		found, err := s.metadata.LookupWikidata(ctx, newRefs)
		if err != nil {
			logf("warn", "Wikidata lookup failed: %s", truncateErr(err, 200))
		}
		rep.WikidataChecked, rep.WikidataFound = len(newRefs), len(found)
		logf("info", "TMDB titles: %d to refresh, %d details fetched (%d fresh, %d failed); Wikidata: %d new looked up, %d found",
			len(refs), rep.DetailsFetched, rep.DetailsFresh, rep.DetailsFailed, rep.WikidataChecked, rep.WikidataFound)
	}

	if s.seasons != nil && (len(refs) > 0 || len(netflixIDs) > 0) {
		// Netflix titles on the refreshed TMDB titles: their season list may have changed.
		_, nfs, err := s.repo.TitlesForTmdbIDs(ctx, refIDs(refs))
		if err != nil {
			return rep, err
		}
		want := map[repository.TmdbRef]bool{}
		for _, ref := range refs {
			want[ref] = true
		}
		for _, n := range nfs {
			if want[repository.NetflixTitleRef(n.NetflixTitle)] {
				netflixIDs = mergeIDs(netflixIDs, []int64{n.ID})
			}
		}
		srep := &SeasonMapReport{}
		for _, id := range netflixIDs {
			if err := s.seasons.MapNetflixSeasons(ctx, id, srep); err != nil {
				s.log.Warn("remap seasons after mapping change failed", slog.Int64("netflix_title_id", id), slog.Any("err", err))
			}
		}
		rep.NetflixTitles = len(netflixIDs)
		if err := s.seasons.TagRankings(ctx, time.Now().UTC().AddDate(-1, 0, 0), srep); err != nil {
			s.log.Warn("tag ranking seasons after mapping change failed", slog.Any("err", err))
		}
		logf("info", "Seasons: %d Netflix title(s) remapped", rep.NetflixTitles)
	}
	return rep, nil
}

// EnsureDetails fetches a TMDB title's details when none are stored, so an
// admin fix can show (and fill) them straight away; the rest of the
// follow-up runs in the mapping refresh.
func (s *TitleMappingService) EnsureDetails(ctx context.Context, ref repository.TmdbRef) {
	if s == nil || s.metadata == nil || ref.ID == "" {
		return
	}
	if m, err := s.meta.Get(ctx, ref); err == nil && m != nil && m.DetailsFetchedAt != nil {
		return
	}
	if _, err := s.metadata.RefreshDetails(ctx, ref); err != nil {
		s.log.Warn("tmdb details after manual mapping failed", slog.String("ref", ref.String()), slog.Any("err", err))
	}
}

func refIDs(refs []repository.TmdbRef) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range refs {
		if !seen[r.ID] {
			seen[r.ID] = true
			out = append(out, r.ID)
		}
	}
	return out
}

func mergeIDs(a, b []int64) []int64 {
	seen := make(map[int64]bool, len(a))
	out := make([]int64, 0, len(a)+len(b))
	for _, list := range [][]int64{a, b} {
		for _, id := range list {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

// --- links ------------------------------------------------------------------

// linkWeekSlack is how far apart (in weeks) the two sources may have charted a
// title: FlixPatrol's daily charts often show a title the week before or after
// it is in Netflix's weekly lists.
const linkWeekSlack = 1

func (s *TitleMappingService) refreshLinks(ctx context.Context, rep *MappingSyncReport) ([]repository.TitleLink, error) {
	fpWeeks, err := s.repo.FlixPatrolNetflixWeeks(ctx)
	if err != nil {
		return nil, err
	}
	want := map[repository.TitleLink]bool{}
	if len(fpWeeks) > 0 {
		from := fpWeeks[0].Week
		for _, w := range fpWeeks {
			if w.Week < from {
				from = w.Week
			}
		}
		// Netflix weeks only matter from the first week FlixPatrol has.
		titles, weeks, err := s.repo.NetflixLinkInputs(ctx, shiftWeek(from, -linkWeekSlack))
		if err != nil {
			return nil, err
		}
		for _, l := range ComputeTitleLinks(titles, weeks, fpWeeks) {
			want[l] = true
		}
	}
	have, err := s.repo.Links(ctx)
	if err != nil {
		return nil, err
	}
	var add, remove []repository.TitleLink
	for _, l := range have {
		if !want[l] {
			remove = append(remove, l)
		}
		delete(want, l)
	}
	for l := range want {
		add = append(add, l)
	}
	if err := s.repo.ReplaceLinks(ctx, add, remove); err != nil {
		return nil, err
	}
	rep.Links = len(have) - len(remove) + len(add)
	rep.LinksAdded, rep.LinksRemoved = len(add), len(remove)
	return remove, nil
}

// ComputeTitleLinks links Netflix titles to FlixPatrol titles with the same
// normalized name (FlixPatrol's chart or page name, with or without a
// ": Season N" suffix) that were on Netflix's chart of the same list in the
// same or an adjacent week.
func ComputeTitleLinks(titles []repository.LinkingNetflixTitle, weeks []repository.NetflixLinkWeek, fpWeeks []repository.FlixPatrolNetflixWeek) []repository.TitleLink {
	type fpKey struct{ category, name string }
	fp := map[fpKey]map[int64]map[string]bool{} // → title → weeks
	for _, w := range fpWeeks {
		for _, name := range linkNames(w.Name, w.FpName) {
			k := fpKey{w.Category, name}
			if fp[k] == nil {
				fp[k] = map[int64]map[string]bool{}
			}
			if fp[k][w.TitleID] == nil {
				fp[k][w.TitleID] = map[string]bool{}
			}
			fp[k][w.TitleID][w.Week] = true
		}
	}
	nWeeks := map[int64]map[string]bool{}
	for _, w := range weeks {
		if nWeeks[w.NetflixTitleID] == nil {
			nWeeks[w.NetflixTitleID] = map[string]bool{}
		}
		nWeeks[w.NetflixTitleID][w.Week] = true
	}
	var out []repository.TitleLink
	for _, n := range titles {
		category := "movies"
		if n.Kind == "tv_show" {
			category = "tv_shows"
		}
		cands := fp[fpKey{category, normalizeTitle(n.ShowTitle)}]
		if len(cands) == 0 || len(nWeeks[n.ID]) == 0 {
			continue
		}
		ids := make([]int64, 0, len(cands))
		for id := range cands {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			if weeksOverlap(cands[id], nWeeks[n.ID]) {
				out = append(out, repository.TitleLink{NetflixTitleID: n.ID, TitleID: id})
			}
		}
	}
	return out
}

// linkNames returns the distinct normalized names a FlixPatrol title is known by.
func linkNames(names ...string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if n := normalizeTitle(s); n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		add(name)
		if base, season := flixpatrol.SeasonFromName(name); season > 0 {
			add(base)
		}
	}
	return out
}

func weeksOverlap(fpWeeks, netflixWeeks map[string]bool) bool {
	for w := range fpWeeks {
		for d := -linkWeekSlack; d <= linkWeekSlack; d++ {
			if netflixWeeks[shiftWeek(w, d)] {
				return true
			}
		}
	}
	return false
}

// shiftWeek moves a YYYY-MM-DD date by n weeks (unparsable dates are kept).
func shiftWeek(week string, n int) string {
	if n == 0 {
		return week
	}
	t, err := time.Parse("2006-01-02", week)
	if err != nil {
		return week
	}
	return t.AddDate(0, 0, 7*n).Format("2006-01-02")
}

// --- sharing mappings ---------------------------------------------------------

// mappingMember is one title's strongest evidence in a linked group.
type mappingMember struct {
	netflix bool
	id      int64
	ref     repository.TmdbRef // zero when unmapped
	manual  bool
	matched bool // has a mapping and is not manual
	exact   bool // mapped through its Netflix ID
}

// groupAnchor picks the mapping a linked group shares: a title fixed by hand,
// else a Netflix title mapped through its Netflix ID, else FlixPatrol's
// match, else Netflix's. ok is false when there is none; conflict when
// titles of the deciding tier disagree.
func groupAnchor(members []mappingMember) (anchor mappingMember, ok, conflict bool) {
	tiers := []func(m mappingMember) bool{
		func(m mappingMember) bool { return m.manual && m.ref.ID != "" },
		func(m mappingMember) bool { return m.exact },
		func(m mappingMember) bool { return m.matched && !m.netflix },
		func(m mappingMember) bool { return m.matched && m.netflix },
	}
	for _, in := range tiers {
		var found []mappingMember
		for _, m := range members {
			if in(m) {
				found = append(found, m)
			}
		}
		if len(found) == 0 {
			continue
		}
		a := found[0]
		for _, m := range found[1:] {
			if m.ref != a.ref {
				return a, false, true
			}
		}
		return a, true, false
	}
	return mappingMember{}, false, false
}

// titleKindOf maps a TMDB kind (movie | tv) to titles.kind (movie | tv_show).
func titleKindOf(tmdbKind string) string {
	if tmdbKind == "tv" {
		return "tv_show"
	}
	return "movie"
}
