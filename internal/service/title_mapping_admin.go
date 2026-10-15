package service

import (
	"context"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// TitleExternalIDs is every ID of one TMDB title: TMDB's own record (IMDb,
// Wikidata, Rotten Tomatoes, Metacritic, Letterboxd, Netflix via Wikidata,
// JustWatch) with gaps filled from the FlixPatrol titles mapped to it, plus
// the FlixPatrol slugs and Netflix Top 10 names that map to it.
type TitleExternalIDs struct {
	IMDbID           *string  `json:"imdb_id"`
	WikidataID       *string  `json:"wikidata_id"`
	RottenTomatoesID *string  `json:"rotten_tomatoes_id"` // m/<slug> | tv/<slug>
	NetflixID        *string  `json:"netflix_id"`         // netflix.com/title/<id>
	JustWatchID      *string  `json:"justwatch_id"`       // tm… | ts…
	MetacriticID     *string  `json:"metacritic_id"`
	LetterboxdID     *string  `json:"letterboxd_id"`
	FlixPatrolSlugs  []string `json:"flixpatrol_slugs"`
	NetflixTop10     []string `json:"netflix_top10_titles"` // names in Netflix's official Top 10
}

// MappingQuery filters the admin mapping list.
type MappingQuery struct {
	Kind   string // "" | movie | tv
	Search string // title, alias name / slug, TMDB or IMDb ID
	Flag   string // "" | conflict | cross_kind | both | netflix_only | flixpatrol_only | manual
}

var mappingFlags = map[string]bool{"": true, "conflict": true, "cross_kind": true, "both": true, "netflix_only": true, "flixpatrol_only": true, "manual": true}

// TmdbMappingEntry is one TMDB title and everything mapped to it.
type TmdbMappingEntry struct {
	TmdbKind   string                  `json:"tmdb_kind"`
	TmdbID     string                  `json:"tmdb_id"`
	Title      *string                 `json:"title"`
	Year       *int                    `json:"year"`
	PosterPath *string                 `json:"poster_path,omitempty"`
	LastSeen   *string                 `json:"last_seen"` // last chart date / Netflix week
	IDs        TitleExternalIDs        `json:"ids"`
	Conflicts  int64                   `json:"conflicts"`  // links to titles mapped elsewhere
	CrossKind  int64                   `json:"cross_kind"` // titles listed as the other kind by their source
	Manual     int64                   `json:"manual"`     // titles fixed by hand
	FlixPatrol []MappedFlixPatrolTitle `json:"flixpatrol_titles"`
	Netflix    []MappedNetflixTitle    `json:"netflix_titles"`
}

// MappedFlixPatrolTitle is a FlixPatrol title mapped to a TMDB title.
type MappedFlixPatrolTitle struct {
	ID           int64         `json:"id"`
	Slug         string        `json:"slug"`
	Name         string        `json:"name"`
	PageKind     *string       `json:"page_kind,omitempty"` // movie | tv_show per FlixPatrol's title page
	MatchStatus  string        `json:"match_status"`
	MatchSource  *string       `json:"match_source,omitempty"`
	LastRankedOn *string       `json:"last_ranked_on"`
	Links        []MappingLink `json:"links"` // linked Netflix titles
}

// MappedNetflixTitle is a Netflix Top 10 title mapped to a TMDB title.
type MappedNetflixTitle struct {
	ID          int64         `json:"id"`
	ShowTitle   string        `json:"show_title"`
	List        string        `json:"list"` // Netflix's list: movie (Films) | tv_show (TV)
	MatchStatus string        `json:"match_status"`
	MatchSource *string       `json:"match_source,omitempty"`
	LastWeek    *string       `json:"last_week"`
	Links       []MappingLink `json:"links"` // linked FlixPatrol titles
}

// MappingLink is a title of the other source linked to a mapped title.
// Conflict: it is mapped to a different TMDB title.
type MappingLink struct {
	Source      string  `json:"source"` // flixpatrol | netflix
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	TmdbKind    *string `json:"tmdb_kind"`
	TmdbID      *string `json:"tmdb_id"`
	MatchStatus string  `json:"match_status"`
	Conflict    bool    `json:"conflict"`
}

type TmdbMappingPage struct {
	Items  []TmdbMappingEntry `json:"items"`
	Total  int64              `json:"total"`
	Limit  int64              `json:"limit"`
	Offset int64              `json:"offset"`
}

// List pages the TMDB titles anything is mapped to, newest activity first.
func (s *TitleMappingService) List(ctx context.Context, q MappingQuery, limit, offset int64) (*TmdbMappingPage, error) {
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 500)
	offset = max(offset, 0)
	switch q.Kind {
	case "", "movie", "tv":
	default:
		return nil, &ValidationError{Message: "invalid kind, expected movie or tv"}
	}
	if !mappingFlags[q.Flag] {
		return nil, &ValidationError{Message: "invalid flag, expected conflict, cross_kind, both, netflix_only, flixpatrol_only or manual"}
	}
	// The list is a few thousand rows ordered by activity, which SQLite
	// computes in full anyway: fetch all, then page here for the total.
	rows, err := s.repo.ListMappings(ctx, repository.MappingFilter{TmdbKind: q.Kind, Search: strings.TrimSpace(q.Search), Flag: q.Flag}, -1, 0)
	if err != nil {
		return nil, err
	}
	total := int64(len(rows))
	end := min(offset+limit, total)
	if offset > total {
		offset = total
	}
	items, err := s.entries(ctx, rows[offset:end])
	if err != nil {
		return nil, err
	}
	return &TmdbMappingPage{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

// Get returns one TMDB title's mapping entry; ErrTitleNotFound when nothing
// is mapped to it.
func (s *TitleMappingService) Get(ctx context.Context, ref TmdbRef) (*TmdbMappingEntry, error) {
	rows, err := s.repo.ListMappings(ctx, repository.MappingFilter{TmdbKind: ref.Kind, Search: ref.ID}, -1, 0)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.Ref == ref {
			items, err := s.entries(ctx, []repository.MappingRow{r})
			if err != nil {
				return nil, err
			}
			return &items[0], nil
		}
	}
	return nil, ErrTitleNotFound
}

// entries loads the titles, links and IDs of mapping rows.
func (s *TitleMappingService) entries(ctx context.Context, rows []repository.MappingRow) ([]TmdbMappingEntry, error) {
	out := make([]TmdbMappingEntry, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	refs := make([]repository.TmdbRef, 0, len(rows))
	idSet := map[string]bool{}
	var ids []string
	for _, r := range rows {
		refs = append(refs, r.Ref)
		if !idSet[r.Ref.ID] {
			idSet[r.Ref.ID] = true
			ids = append(ids, r.Ref.ID)
		}
	}
	fps, nfs, err := s.repo.TitlesForTmdbIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	meta, err := s.meta.GetMany(ctx, refs)
	if err != nil {
		return nil, err
	}
	var fpIDs, nfIDs []int64
	for _, t := range fps {
		fpIDs = append(fpIDs, t.ID)
	}
	for _, n := range nfs {
		nfIDs = append(nfIDs, n.ID)
	}
	nLinks, err := s.repo.NetflixLinksOf(ctx, fpIDs)
	if err != nil {
		return nil, err
	}
	fLinks, err := s.repo.FlixPatrolLinksOf(ctx, nfIDs)
	if err != nil {
		return nil, err
	}

	for _, r := range rows {
		e := TmdbMappingEntry{
			TmdbKind: r.Ref.Kind, TmdbID: r.Ref.ID, Title: r.Title, Year: yearOf(r.ReleaseDate), PosterPath: r.PosterPath,
			LastSeen: emptyToNil(r.LastSeen), Conflicts: r.Conflicts, CrossKind: r.CrossKind, Manual: r.Manual,
			FlixPatrol: []MappedFlixPatrolTitle{}, Netflix: []MappedNetflixTitle{},
		}
		var fpTitles []repository.MappedFlixPatrolTitle
		for _, t := range fps {
			if t.TmdbID.String != r.Ref.ID || tmdbKindOfTitle(t.Kind) != r.Ref.Kind {
				continue
			}
			fpTitles = append(fpTitles, t)
			mt := MappedFlixPatrolTitle{ID: t.ID, Slug: t.Slug, Name: t.Name, PageKind: nullStr(t.FpKind), MatchStatus: t.MatchStatus,
				MatchSource: nullStr(t.MatchSource), LastRankedOn: emptyToNil(t.LastRankedOn), Links: []MappingLink{}}
			for _, l := range nLinks {
				if l.TitleID != t.ID {
					continue
				}
				mt.Links = append(mt.Links, MappingLink{Source: "netflix", ID: l.NetflixTitleID, Name: l.ShowTitle,
					TmdbKind: emptyToNil(l.TmdbKind), TmdbID: emptyToNil(l.TmdbID), MatchStatus: l.MatchStatus,
					Conflict: l.TmdbID != "" && (l.TmdbID != r.Ref.ID || l.TmdbKind != r.Ref.Kind)})
			}
			e.FlixPatrol = append(e.FlixPatrol, mt)
		}
		for _, n := range nfs {
			if n.TmdbID.String != r.Ref.ID || n.TmdbKind.String != r.Ref.Kind {
				continue
			}
			mn := MappedNetflixTitle{ID: n.ID, ShowTitle: n.ShowTitle, List: n.Kind, MatchStatus: n.MatchStatus,
				MatchSource: nullStr(n.MatchSource), LastWeek: emptyToNil(n.LastWeek), Links: []MappingLink{}}
			for _, l := range fLinks {
				if l.NetflixTitleID != n.ID {
					continue
				}
				lk := tmdbKindOfTitle(l.Kind)
				mn.Links = append(mn.Links, MappingLink{Source: "flixpatrol", ID: l.TitleID, Name: l.Name,
					TmdbKind: kindIfMapped(lk, l.TmdbID), TmdbID: emptyToNil(l.TmdbID), MatchStatus: l.MatchStatus,
					Conflict: l.TmdbID != "" && (l.TmdbID != r.Ref.ID || lk != r.Ref.Kind)})
			}
			e.Netflix = append(e.Netflix, mn)
			e.IDs.NetflixTop10 = append(e.IDs.NetflixTop10, n.ShowTitle)
		}
		e.IDs = externalIDs(meta[r.Ref.String()], fpTitles, e.IDs.NetflixTop10)
		out = append(out, e)
	}
	return out, nil
}

// ExternalIDs returns every ID known for a TMDB title (nil when nothing is
// known about it).
func (s *TitleMappingService) ExternalIDs(ctx context.Context, ref TmdbRef) (*TitleExternalIDs, error) {
	if s == nil {
		return nil, nil
	}
	fps, nfs, err := s.repo.TitlesForTmdbIDs(ctx, []string{ref.ID})
	if err != nil {
		return nil, err
	}
	meta, err := s.meta.Get(ctx, ref)
	if err != nil {
		return nil, err
	}
	var fpTitles []repository.MappedFlixPatrolTitle
	for _, t := range fps {
		if t.TmdbID.String == ref.ID && tmdbKindOfTitle(t.Kind) == ref.Kind {
			fpTitles = append(fpTitles, t)
		}
	}
	var names []string
	for _, n := range nfs {
		if n.TmdbID.String == ref.ID && n.TmdbKind.String == ref.Kind {
			names = append(names, n.ShowTitle)
		}
	}
	if meta == nil && len(fpTitles) == 0 && len(names) == 0 {
		return nil, nil
	}
	ids := externalIDs(meta, fpTitles, names)
	return &ids, nil
}

func externalIDs(meta *repository.TmdbMetadata, fps []repository.MappedFlixPatrolTitle, netflixNames []string) TitleExternalIDs {
	ids := TitleExternalIDs{FlixPatrolSlugs: []string{}, NetflixTop10: []string{}}
	if meta != nil {
		ids.IMDbID, ids.WikidataID, ids.RottenTomatoesID = meta.IMDbID, meta.WikidataID, meta.RottenTomatoesID
		ids.NetflixID, ids.JustWatchID, ids.MetacriticID, ids.LetterboxdID = meta.NetflixID, meta.JustWatchID, meta.MetacriticID, meta.LetterboxdID
	}
	// Titles fixed by hand first, so their IDs win the gaps.
	sort.SliceStable(fps, func(i, j int) bool {
		return fps[i].MatchStatus == repository.MatchStatusManual && fps[j].MatchStatus != repository.MatchStatusManual
	})
	for _, t := range fps {
		ids.FlixPatrolSlugs = append(ids.FlixPatrolSlugs, t.Slug)
		fill(&ids.IMDbID, t.ImdbID.String)
		fill(&ids.WikidataID, t.WikidataID.String)
		fill(&ids.RottenTomatoesID, t.RtUrl.String)
		fill(&ids.JustWatchID, t.JustwatchID.String)
	}
	sort.Strings(ids.FlixPatrolSlugs)
	seen := map[string]bool{}
	for _, n := range netflixNames {
		if !seen[n] {
			seen[n] = true
			ids.NetflixTop10 = append(ids.NetflixTop10, n)
		}
	}
	sort.Strings(ids.NetflixTop10)
	return ids
}

func fill(dst **string, v string) {
	if (*dst == nil || **dst == "") && v != "" {
		*dst = &v
	}
}

// RemapInput moves everything mapped to one TMDB title to another, given
// by TMDB kind + ID, or by an IMDb ID or Wikidata item (e.g. Q141461750)
// resolved to TMDB (tmdb_kind picks one when both a movie and a show match).
type RemapInput struct {
	TmdbKind   string `json:"tmdb_kind"` // movie | tv
	TmdbID     string `json:"tmdb_id"`
	IMDbID     string `json:"imdb_id"`
	WikidataID string `json:"wikidata_id"`
}

// Remap moves every FlixPatrol and Netflix title mapped to from onto the
// given TMDB title (fixed by hand), fetches its TMDB details first so the
// titles get its IMDb ID, then shares the fix over links and remaps seasons.
func (s *TitleMappingService) Remap(ctx context.Context, from TmdbRef, in RemapInput) (*TmdbMappingEntry, error) {
	var wiki *client.WikidataItem
	if strings.TrimSpace(in.TmdbID) == "" {
		ext := strings.TrimSpace(in.WikidataID)
		if ext == "" {
			ext = strings.TrimSpace(in.IMDbID)
		}
		if ext == "" {
			return nil, &ValidationError{Message: "give tmdb_kind + tmdb_id, imdb_id or wikidata_id"}
		}
		if s.metadata == nil {
			return nil, &ValidationError{Message: "resolving IMDb / Wikidata IDs needs TMDB or Wikidata; give tmdb_id"}
		}
		r, err := s.metadata.ResolveExternalID(ctx, ext, in.TmdbKind)
		if err != nil {
			return nil, err
		}
		in.TmdbKind, in.TmdbID, wiki = r.Ref.Kind, r.Ref.ID, r.Wiki
	}
	to, err := ParseTmdbRef(in.TmdbKind, in.TmdbID)
	if err != nil {
		return nil, err
	}
	if to == from {
		return nil, &ValidationError{Message: "the new TMDB title is the current one"}
	}
	if s.metadata != nil {
		if _, err := s.metadata.RefreshDetails(ctx, to); err != nil {
			s.log.Warn("tmdb details before remap failed", slog.String("ref", to.String()), slog.Any("err", err))
		}
	}
	if wiki != nil && (wiki.TmdbMovieID == to.ID || wiki.TmdbTVID == to.ID) {
		// Store what the item says now rather than waiting for the Wikidata job.
		if err := s.meta.SaveWikidata(ctx, to, client.WikidataIDs{WikidataID: wiki.WikidataID, IMDbID: wiki.IMDbID,
			RottenTomato: wiki.RottenTomato, NetflixID: wiki.NetflixID}); err != nil {
			return nil, err
		}
	}
	titleIDs, netflixIDs, err := s.repo.Remap(ctx, from, to)
	if err != nil {
		return nil, err
	}
	if len(titleIDs)+len(netflixIDs) == 0 {
		return nil, ErrTitleNotFound
	}
	s.log.Info("tmdb mapping remapped", slog.String("from", from.String()), slog.String("to", to.String()),
		slog.Int("flixpatrol_titles", len(titleIDs)), slog.Int("netflix_titles", len(netflixIDs)))
	s.afterChange(ctx, netflixIDs, nil, len(titleIDs) > 0)
	if _, err := s.Sync(ctx, false, nil); err != nil {
		return nil, err
	}
	return s.Get(ctx, to)
}

func tmdbKindOfTitle(kind string) string {
	if kind == "tv_show" {
		return "tv"
	}
	return "movie"
}

func kindIfMapped(kind, id string) *string {
	if id == "" {
		return nil
	}
	return &kind
}

func yearOf(date *string) *int {
	if date == nil || len(*date) < 4 {
		return nil
	}
	y, err := strconv.Atoi((*date)[:4])
	if err != nil {
		return nil
	}
	return &y
}
