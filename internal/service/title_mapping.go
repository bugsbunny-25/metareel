package service

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
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
	meta     *repository.MetadataRepository
	metadata *TitleMetadataService // optional: TMDB details for newly shared mappings
	seasons  *SeasonMapper         // optional: remaps seasons of Netflix titles that changed
}

func NewTitleMappingService(log *slog.Logger, repo *repository.MappingRepository, meta *repository.MetadataRepository, metadata *TitleMetadataService, seasons *SeasonMapper) *TitleMappingService {
	return &TitleMappingService{log: log, repo: repo, meta: meta, metadata: metadata, seasons: seasons}
}

// MappingSyncReport summarises a sync.
type MappingSyncReport struct {
	Links             int `json:"links"`
	LinksAdded        int `json:"links_added"`
	LinksRemoved      int `json:"links_removed"`
	NetflixUpdated    int `json:"netflix_titles_updated"`
	FlixPatrolUpdated int `json:"flixpatrol_titles_updated"`
	Conflicts         int `json:"conflicts"` // titles whose linked titles disagree (left alone)
	IDsFilled         int `json:"tmdb_titles_ids_filled"`
}

// Match sources of mappings shared over a link.
const (
	MatchSourceFlixPatrolTitle = "flixpatrol_title" // a Netflix title took a linked FlixPatrol title's mapping
	MatchSourceNetflixTitle    = "netflix_title"    // a FlixPatrol title took a linked Netflix title's mapping
)

// metadataOnSync caps TMDB detail fetches per sync; the metadata job gets the rest.
const metadataOnSync = 25

// Sync recomputes the links (when refreshLinks) and shares mappings over
// them. Netflix titles whose mapping changed get their seasons remapped.
func (s *TitleMappingService) Sync(ctx context.Context, refreshLinks bool, runLog func(level, msg string)) (*MappingSyncReport, error) {
	rep := &MappingSyncReport{}
	if s == nil {
		return rep, nil
	}
	if refreshLinks {
		if err := s.refreshLinks(ctx, rep); err != nil {
			return rep, err
		}
	}
	changedNetflix, newRefs, err := s.propagate(ctx, rep)
	if err != nil {
		return rep, err
	}
	s.afterChange(ctx, changedNetflix, newRefs, rep.FlixPatrolUpdated > 0)
	if s.meta != nil {
		n, err := s.meta.ConsolidateExternalIDs(ctx)
		if err != nil {
			return rep, err
		}
		rep.IDsFilled = int(n)
	}
	if runLog != nil && (refreshLinks || rep.NetflixUpdated+rep.FlixPatrolUpdated+rep.Conflicts > 0) {
		msg := fmt.Sprintf("Mapping: %d Netflix titles and %d FlixPatrol titles took a linked title's TMDB mapping", rep.NetflixUpdated, rep.FlixPatrolUpdated)
		if refreshLinks {
			msg += fmt.Sprintf("; %d links (%d new, %d removed)", rep.Links, rep.LinksAdded, rep.LinksRemoved)
		}
		if rep.Conflicts > 0 {
			msg += fmt.Sprintf("; %d title(s) left alone because their linked titles disagree (see TMDB mapping → conflicts)", rep.Conflicts)
		}
		runLog("info", msg)
	}
	return rep, nil
}

// afterChange fetches TMDB details for newly shared mappings, remaps the
// seasons of Netflix titles that changed and re-tags recent TV rankings.
func (s *TitleMappingService) afterChange(ctx context.Context, netflixIDs []int64, refs []repository.TmdbRef, flixPatrolChanged bool) {
	if s.metadata != nil {
		for i, ref := range refs {
			if i >= metadataOnSync || ctx.Err() != nil {
				break
			}
			if _, err := s.metadata.RefreshDetails(ctx, ref); err != nil {
				s.log.Warn("tmdb details for shared mapping failed", slog.String("ref", ref.String()), slog.Any("err", err))
			}
		}
	}
	if s.seasons == nil || (len(netflixIDs) == 0 && !flixPatrolChanged) {
		return
	}
	rep := &SeasonMapReport{}
	for _, id := range netflixIDs {
		if err := s.seasons.MapNetflixSeasons(ctx, id, rep); err != nil {
			s.log.Warn("remap seasons after mapping sync failed", slog.Int64("netflix_title_id", id), slog.Any("err", err))
		}
	}
	if err := s.seasons.TagRankings(ctx, time.Now().UTC().AddDate(-1, 0, 0), rep); err != nil {
		s.log.Warn("tag ranking seasons after mapping sync failed", slog.Any("err", err))
	}
}

// --- links ------------------------------------------------------------------

// linkWeekSlack is how far apart (in weeks) the two sources may have charted a
// title: FlixPatrol's daily charts often show a title the week before or after
// it is in Netflix's weekly lists.
const linkWeekSlack = 1

func (s *TitleMappingService) refreshLinks(ctx context.Context, rep *MappingSyncReport) error {
	fpWeeks, err := s.repo.FlixPatrolNetflixWeeks(ctx)
	if err != nil {
		return err
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
			return err
		}
		for _, l := range ComputeTitleLinks(titles, weeks, fpWeeks) {
			want[l] = true
		}
	}
	have, err := s.repo.Links(ctx)
	if err != nil {
		return err
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
		return err
	}
	rep.Links = len(have) - len(remove) + len(add)
	rep.LinksAdded, rep.LinksRemoved = len(add), len(remove)
	return nil
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

// mappingMember is one side of a linked group.
type mappingMember struct {
	netflix bool
	id      int64
	ref     repository.TmdbRef // zero when unmapped
	imdbID  string
	manual  bool
	matched bool // has a mapping and is not manual
	exact   bool // mapped through its Netflix ID
}

func netflixMember(n sqlc.NetflixTitle) mappingMember {
	m := mappingMember{netflix: true, id: n.ID, imdbID: n.ImdbID.String, manual: n.MatchStatus == repository.MatchStatusManual}
	if n.TmdbID.String != "" && (n.TmdbKind.String == "movie" || n.TmdbKind.String == "tv") {
		m.ref = repository.TmdbRef{Kind: n.TmdbKind.String, ID: n.TmdbID.String}
	}
	m.matched = !m.manual && m.ref.ID != ""
	m.exact = m.matched && n.MatchSource.String == MatchSourceNetflixID
	return m
}

func flixPatrolMember(t sqlc.Title) mappingMember {
	m := mappingMember{id: t.ID, imdbID: t.ImdbID.String, manual: t.MatchStatus == repository.MatchStatusManual}
	if t.TmdbID.String != "" {
		m.ref = repository.TmdbRef{Kind: tmdbKindOf(flixpatrol.TitleKind(t.Kind)), ID: t.TmdbID.String}
	}
	m.matched = !m.manual && m.ref.ID != ""
	return m
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
			if a.imdbID == "" {
				a.imdbID = m.imdbID
			}
		}
		return a, true, false
	}
	return mappingMember{}, false, false
}

// propagatePasses bounds how far one sync spreads a mapping: each pass moves
// it one link (Netflix ↔ FlixPatrol) into titles that take it.
const propagatePasses = 3

// propagate shares mappings over links. Each title looks only at itself and
// the titles directly linked to it that are linked to nothing else, and
// takes the anchor of that set (see groupAnchor): a mapping never travels
// through an intermediary, nor from a title that links to several (one
// Netflix title for two same-name FlixPatrol titles). It returns the Netflix titles that changed and the TMDB
// titles newly mapped to.
func (s *TitleMappingService) propagate(ctx context.Context, rep *MappingSyncReport) ([]int64, []repository.TmdbRef, error) {
	var changedNetflix []int64
	var newRefs []repository.TmdbRef
	seenRef := map[repository.TmdbRef]bool{}
	for pass := 0; pass < propagatePasses; pass++ {
		changed, conflicts, err := s.propagateOnce(ctx, &changedNetflix, &newRefs, seenRef, rep)
		if err != nil {
			return changedNetflix, newRefs, err
		}
		rep.Conflicts = conflicts
		if changed == 0 {
			break
		}
	}
	return changedNetflix, newRefs, nil
}

func (s *TitleMappingService) propagateOnce(ctx context.Context, changedNetflix *[]int64, newRefs *[]repository.TmdbRef, seenRef map[repository.TmdbRef]bool, rep *MappingSyncReport) (int, int, error) {
	links, err := s.repo.Links(ctx)
	if err != nil || len(links) == 0 {
		return 0, 0, err
	}
	nts, fts, err := s.repo.LinkedTitles(ctx)
	if err != nil {
		return 0, 0, err
	}
	members := map[string]mappingMember{}
	for _, n := range nts {
		members[memberKey(true, n.ID)] = netflixMember(n)
	}
	for _, t := range fts {
		members[memberKey(false, t.ID)] = flixPatrolMember(t)
	}
	neighbours := map[string][]string{}
	for _, l := range links {
		a, b := memberKey(true, l.NetflixTitleID), memberKey(false, l.TitleID)
		neighbours[a] = append(neighbours[a], b)
		neighbours[b] = append(neighbours[b], a)
	}
	keys := make([]string, 0, len(members))
	for k := range members {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	changedCount, conflicts := 0, 0
	for _, k := range keys {
		if err := ctx.Err(); err != nil {
			return changedCount, conflicts, err
		}
		m := members[k]
		set := []mappingMember{m}
		for _, nk := range neighbours[k] {
			// A linked title also linked elsewhere (one Netflix title for a
			// show and its revival) says nothing certain about this one.
			if nm, ok := members[nk]; ok && len(neighbours[nk]) == 1 {
				set = append(set, nm)
			}
		}
		anchor, ok, conflict := groupAnchor(set)
		if conflict {
			conflicts++
			continue
		}
		if !ok || m.manual || m.ref == anchor.ref {
			continue
		}
		var changed bool
		if m.netflix {
			// The directly linked FlixPatrol title it is matched through.
			var via int64
			for _, nk := range neighbours[k] {
				if nm := members[nk]; !nm.netflix && nm.ref == anchor.ref {
					via = nm.id
					break
				}
			}
			source := MatchSourceFlixPatrolTitle
			if anchor.netflix {
				source = MatchSourceNetflixTitle
			}
			changed, err = s.repo.AdoptForNetflix(ctx, m.id, anchor.ref.Kind, anchor.ref.ID, anchor.imdbID, via, source)
			if changed {
				rep.NetflixUpdated++
				*changedNetflix = append(*changedNetflix, m.id)
			}
		} else {
			changed, err = s.repo.AdoptForFlixPatrol(ctx, m.id, titleKindOf(anchor.ref.Kind), anchor.ref.ID, anchor.imdbID)
			if changed {
				rep.FlixPatrolUpdated++
			}
		}
		if err != nil {
			return changedCount, conflicts, err
		}
		if changed {
			changedCount++
			if !seenRef[anchor.ref] {
				seenRef[anchor.ref] = true
				*newRefs = append(*newRefs, anchor.ref)
			}
		}
	}
	return changedCount, conflicts, nil
}

func memberKey(netflix bool, id int64) string {
	if netflix {
		return "n" + strconv.FormatInt(id, 10)
	}
	return "f" + strconv.FormatInt(id, 10)
}

// titleKindOf maps a TMDB kind (movie | tv) to titles.kind (movie | tv_show).
func titleKindOf(tmdbKind string) string {
	if tmdbKind == "tv" {
		return "tv_show"
	}
	return "movie"
}

// ForgetShared clears what linked titles took from a title whose match was
// just cleared (a FlixPatrol title by titleID, or a Netflix title by
// netflixTitleID), so the wrong mapping does not come back over the link;
// both sides are then matched afresh.
func (s *TitleMappingService) ForgetShared(ctx context.Context, titleID, netflixTitleID int64) error {
	if s == nil {
		return nil
	}
	if titleID > 0 {
		ids, err := s.repo.ClearAdoptedFromFlixPatrol(ctx, titleID)
		if err != nil {
			return err
		}
		s.afterChange(ctx, ids, nil, false)
	}
	if netflixTitleID > 0 {
		if err := s.repo.ClearAdoptedFromNetflix(ctx, netflixTitleID); err != nil {
			return err
		}
	}
	return nil
}
