package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Netflix IDs: Netflix's Top 10 files carry no IDs, but Tudum's Top 10
// search returns each charted title's Netflix ID (a season's own ID for TV,
// whose public title page redirects to the show). A title's ID is the one
// whose name matches (a season title for TV) in the same list and that
// charted in one of the title's weeks. Netflix ID → TMDB is then exact:
// tmdb_titles.netflix_id, else Wikidata (P1874).

// MatchSourceNetflixID marks a Netflix title mapped through its Netflix ID.
const MatchSourceNetflixID = "netflix_id"

const (
	netflixIDRecheck   = "-30 days" // retry titles the search did not find
	netflixIDBacklog   = 200        // titles looked up per matching job beyond the due ones
	netflixIDDueLimit  = 500        // due titles looked up per run
	netflixSearchPause = 500 * time.Millisecond
)

// NetflixIDReport summarises a Netflix ID pass.
type NetflixIDReport struct {
	Searched int `json:"searched"`
	Found    int `json:"found"`
	Mapped   int `json:"mapped"` // titles (re)mapped to TMDB by their Netflix ID
}

// NetflixSearcher is Tudum's Top 10 search (client.NetflixTop10Search).
type NetflixSearcher interface {
	Search(ctx context.Context, term string) ([]client.NetflixSearchItem, error)
	ShowID(ctx context.Context, netflixID string) (string, error)
}

// WithNetflixSearch looks titles' Netflix IDs up on Tudum's Top 10 search.
func (n *NetflixImporter) WithNetflixSearch(search NetflixSearcher) *NetflixImporter {
	n.search = search
	return n
}

// resolveNetflixIDs looks up the Netflix IDs of these titles (ids), or of
// up to limit titles without one, then maps those not fixed by hand to the
// TMDB title of their ID, replacing matches made by name. It returns the
// titles it mapped.
func (n *NetflixImporter) resolveNetflixIDs(ctx context.Context, session *MappingSession, ids []int64, limit int64, rep *NetflixIDReport, logf func(level, format string, args ...any)) (map[int64]bool, error) {
	mapped := map[int64]bool{}
	if n.search == nil {
		return mapped, nil
	}
	// TV titles are found by their season titles; new imports have no rows yet.
	if err := n.repo.EnsureNetflixSeasons(ctx); err != nil {
		return mapped, err
	}
	cands, err := n.repo.NetflixTitlesNeedingNetflixID(ctx, ids, netflixIDRecheck, limit)
	if err != nil {
		return mapped, err
	}
	if len(ids) > 0 && int64(len(cands)) > limit {
		cands = cands[:limit]
	}
	found := map[int64]string{}
	searched := 0
	for _, c := range cands {
		if err := ctx.Err(); err != nil {
			return mapped, err
		}
		if c.NetflixID != "" {
			found[c.ID] = c.NetflixID
			continue
		}
		if searched > 0 {
			if err := sleepCtx(ctx, netflixSearchPause); err != nil {
				return mapped, err
			}
		}
		searched++
		rep.Searched++
		id, err := n.netflixIDOf(ctx, c)
		if err != nil {
			// The search itself failing (e.g. Netflix changed the query) stops the pass.
			n.log.Warn("netflix top 10 search failed", slog.String("show_title", c.ShowTitle), slog.Any("err", err))
			logf("warn", "Netflix ID lookup stopped: %s", truncateErr(err, 200))
			break
		}
		if err := n.repo.SetNetflixTitleNetflixID(ctx, c.ID, id); err != nil {
			return mapped, err
		}
		if id != "" {
			rep.Found++
			found[c.ID] = id
		}
	}
	if len(found) == 0 {
		return mapped, nil
	}

	refs, err := n.tmdbForNetflixIDs(ctx, found)
	if err != nil {
		n.log.Warn("netflix id to tmdb failed", slog.Any("err", err))
	}
	for _, c := range cands {
		nfID, ok := found[c.ID]
		hit, ok2 := refs[nfID]
		if !ok || !ok2 || c.MatchStatus == repository.MatchStatusManual {
			continue
		}
		ref := repository.TmdbRef{Kind: hit.TmdbKind, ID: hit.TmdbID}
		if c.TmdbID == ref.ID && c.TmdbKind == ref.Kind && c.MatchSource == MatchSourceNetflixID {
			continue
		}
		changed, err := session.MatchNetflixByNetflixID(ctx, c.ID, ref, hit.IMDbID)
		if err != nil {
			return mapped, err
		}
		if !changed {
			continue
		}
		mapped[c.ID] = true
		rep.Mapped++
		if c.TmdbID != "" && (c.TmdbID != ref.ID || c.TmdbKind != ref.Kind) {
			logf("info", "%q: Netflix ID %s is TMDB %s %s (was %s %s via %s)", c.ShowTitle, nfID, ref.Kind, ref.ID, c.TmdbKind, c.TmdbID, c.MatchSource)
		}
		if n.metadata != nil { // TMDB details follow in the mapping refresh
			if err := n.metadata.SaveMatchedIDs(ctx, ref, repository.TmdbIDs{NetflixID: nfID, WikidataID: hit.WikidataID, IMDbID: hit.IMDbID}); err != nil {
				return mapped, err
			}
		}
	}
	logf("info", "Netflix IDs: %d looked up on Tudum, %d found, %d title(s) mapped by Netflix ID", rep.Searched, rep.Found, rep.Mapped)
	// Titles mapped by name keep their mapping; their TMDB titles still take the ID.
	if _, err := n.metadata.SyncNetflixIDs(ctx); err != nil {
		return mapped, err
	}
	return mapped, nil
}

// netflixIDOf searches Tudum's Top 10 for a title's Netflix ID ("" when no
// result matches by name, list and week).
func (n *NetflixImporter) netflixIDOf(ctx context.Context, c repository.NetflixIDCandidate) (string, error) {
	items, err := n.search.Search(ctx, c.ShowTitle)
	if err != nil {
		return "", err
	}
	weeks, err := n.repo.NetflixTitleWeeks(ctx, c.ID)
	if err != nil {
		return "", err
	}
	charted := map[string]bool{}
	for _, w := range weeks {
		charted[w] = true
	}
	overlaps := func(it client.NetflixSearchItem) bool {
		for _, w := range it.Weeks() {
			if charted[w] {
				return true
			}
		}
		return false
	}
	category := "FILMS"
	if c.Kind == "tv_show" {
		category = "TV"
	}
	names := map[string]int64{normalizeTitle(c.ShowTitle): 0} // normalized name → season row (0 = the show itself)
	if c.Kind == "tv_show" {
		seasons, err := n.repo.NetflixSeasonTitles(ctx, c.ID)
		if err != nil {
			return "", err
		}
		for _, s := range seasons {
			names[normalizeTitle(s.SeasonTitle)] = s.ID
		}
	}
	var ids []string
	seen := map[string]bool{}
	for _, it := range items {
		seasonID, ok := names[normalizeTitle(it.Title)]
		if !ok || it.Category != category || !overlaps(it) {
			continue
		}
		if seasonID > 0 {
			if err := n.repo.SetNetflixSeasonNetflixID(ctx, seasonID, it.NetflixID); err != nil {
				return "", err
			}
		}
		if !seen[it.NetflixID] {
			seen[it.NetflixID] = true
			ids = append(ids, it.NetflixID)
		}
	}
	if len(ids) == 0 {
		return "", nil
	}
	if c.Kind != "tv_show" {
		if len(ids) > 1 {
			return "", nil // two films of this name charted in its weeks
		}
		return ids[0], nil
	}
	// Seasons: their public title pages redirect to the show.
	show, err := n.search.ShowID(ctx, ids[0])
	if err != nil {
		n.log.Debug("netflix show id lookup failed", slog.String("netflix_id", ids[0]), slog.Any("err", err))
		return "", nil
	}
	return show, nil
}

// tmdbForNetflixIDs maps Netflix IDs (by title) to a single TMDB title each:
// a TMDB title already holding the ID, else Wikidata's items with it.
func (n *NetflixImporter) tmdbForNetflixIDs(ctx context.Context, byTitle map[int64]string) (map[string]client.WikidataNetflixHit, error) {
	out := map[string]client.WikidataNetflixHit{}
	var ask []string
	for _, nfID := range byTitle {
		if _, done := out[nfID]; done {
			continue
		}
		refs, err := n.metadata.RefsByNetflixID(ctx, nfID, false)
		if err != nil {
			return out, err
		}
		if len(refs) == 1 {
			out[nfID] = client.WikidataNetflixHit{NetflixID: nfID, TmdbKind: refs[0].Kind, TmdbID: refs[0].ID}
			continue
		}
		ask = append(ask, nfID)
	}
	if len(ask) == 0 {
		return out, nil
	}
	hits, err := n.metadata.FindNetflixIDsOnWikidata(ctx, ask)
	if err != nil {
		return out, fmt.Errorf("wikidata netflix ids: %w", err)
	}
	for nfID, hs := range hits {
		if len(hs) == 1 {
			out[nfID] = hs[0]
		}
	}
	return out, nil
}
