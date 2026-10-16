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

// TitleExternalIDs is every ID of one TMDB title (IMDb, Wikidata, Rotten
// Tomatoes, Metacritic, Letterboxd, Netflix, JustWatch: each from the
// strongest source that has one, see Sources), plus the FlixPatrol slugs and
// Netflix Top 10 names that map to it.
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
	// Where each ID came from: imdb, wikidata, rotten_tomatoes, metacritic,
	// letterboxd, netflix, justwatch → manual | tmdb | wikidata | match |
	// netflix (copied from a Netflix title mapped here).
	Sources map[string]string `json:"sources"`
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
	ids := TitleExternalIDs{FlixPatrolSlugs: []string{}, NetflixTop10: []string{}, Sources: map[string]string{}}
	if meta != nil {
		ids.IMDbID, ids.WikidataID, ids.RottenTomatoesID = meta.IMDbID, meta.WikidataID, meta.RottenTomatoesID
		ids.NetflixID, ids.JustWatchID, ids.MetacriticID, ids.LetterboxdID = meta.NetflixID, meta.JustWatchID, meta.MetacriticID, meta.LetterboxdID
		for k, v := range meta.IDSources {
			ids.Sources[k] = v
		}
	}
	for _, t := range fps {
		ids.FlixPatrolSlugs = append(ids.FlixPatrolSlugs, t.Slug)
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
// given TMDB title (fixed by hand). An IMDb or Wikidata ID it was given by
// is stored as set by hand on the new TMDB title. Mappings are then shared
// over links; TMDB details, IDs and seasons follow in a mapping refresh.
func (s *TitleMappingService) Remap(ctx context.Context, from TmdbRef, in RemapInput) (*TmdbMappingEntry, error) {
	var (
		wiki           *client.WikidataItem
		imdbID, wikiID string
	)
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
		if client.WikidataQIDRe.MatchString(strings.ToUpper(ext)) {
			wikiID = strings.ToUpper(ext)
		} else {
			imdbID = ext
		}
	}
	to, err := ParseTmdbRef(in.TmdbKind, in.TmdbID)
	if err != nil {
		return nil, err
	}
	if to == from {
		return nil, &ValidationError{Message: "the new TMDB title is the current one"}
	}
	s.EnsureDetails(ctx, to)
	if wiki != nil && (wiki.TmdbMovieID == to.ID || wiki.TmdbTVID == to.ID) {
		// Store what the item says now rather than waiting for the Wikidata job.
		if err := s.meta.SaveWikidata(ctx, to, client.WikidataIDs{WikidataID: wiki.WikidataID, IMDbID: wiki.IMDbID,
			RottenTomato: wiki.RottenTomato, NetflixID: wiki.NetflixID}); err != nil {
			return nil, err
		}
	}
	session := s.writer.Session(ActorAdmin)
	if err := s.SetManualIDs(ctx, session, to, optional(imdbID), optional(wikiID), nil); err != nil {
		return nil, err
	}
	titleIDs, netflixIDs, err := session.Remap(ctx, from, to)
	if err != nil {
		return nil, err
	}
	if len(titleIDs)+len(netflixIDs) == 0 {
		return nil, ErrTitleNotFound
	}
	s.log.Info("tmdb mapping remapped", slog.String("from", from.String()), slog.String("to", to.String()),
		slog.Int("flixpatrol_titles", len(titleIDs)), slog.Int("netflix_titles", len(netflixIDs)))
	session.Flush(ctx)
	return s.Get(ctx, to)
}

// SetManualIDs stores IMDb / Wikidata IDs and Rotten Tomatoes slugs of a
// TMDB title given by hand (nil = unchanged): TMDB, Wikidata and matching
// never replace them; "" clears one and hands it back to them. The TMDB
// title joins the session's follow-up, which fills cleared IDs again.
func (s *TitleMappingService) SetManualIDs(ctx context.Context, session *MappingSession, ref TmdbRef, imdbID, wikidataID, rtID *string) error {
	if s == nil || s.meta == nil || ref.ID == "" || (imdbID == nil && wikidataID == nil && rtID == nil) {
		return nil
	}
	set := repository.TmdbIDs{}
	clearIMDb, clearWikidata, clearRT := false, false, false
	for _, f := range []struct {
		v     *string
		dst   *string
		clear *bool
	}{{imdbID, &set.IMDbID, &clearIMDb}, {wikidataID, &set.WikidataID, &clearWikidata}, {rtID, &set.RTID, &clearRT}} {
		if f.v == nil {
			continue
		}
		if *f.v == "" {
			*f.clear = true
		} else {
			*f.dst = *f.v
		}
	}
	if err := s.meta.ClearIDs(ctx, ref, clearIMDb, clearWikidata, clearRT); err != nil {
		return err
	}
	if err := s.meta.SaveIDs(ctx, ref, set, repository.IDSourceManual); err != nil {
		return err
	}
	session.Touch(ref)
	return nil
}

// optional returns nil for "" (unchanged), else &v.
func optional(v string) *string {
	if v == "" {
		return nil
	}
	return &v
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

// MappingEventsQuery selects mapping history: subject is "flixpatrol:<id>"
// or "netflix:<id>", tmdb is "movie:<id>" or "tv:<id>"; empty = any.
type MappingEventsQuery struct {
	Subject string
	Tmdb    string
	Limit   int64
}

// Events lists recorded mapping changes, newest first.
func (s *TitleMappingService) Events(ctx context.Context, q MappingEventsQuery) ([]repository.MappingEvent, error) {
	f := repository.MappingEventFilter{Limit: q.Limit}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	f.Limit = min(f.Limit, 500)
	if q.Subject != "" {
		kind, id, _ := strings.Cut(q.Subject, ":")
		n, err := strconv.ParseInt(id, 10, 64)
		if (kind != repository.SubjectFlixPatrol && kind != repository.SubjectNetflix) || err != nil || n <= 0 {
			return nil, &ValidationError{Message: "subject must look like flixpatrol:12 or netflix:34"}
		}
		f.SubjectType, f.SubjectID = kind, n
	}
	if q.Tmdb != "" {
		kind, id, _ := strings.Cut(q.Tmdb, ":")
		ref, err := ParseTmdbRef(kind, id)
		if err != nil {
			return nil, err
		}
		f.Ref = ref
	}
	events, err := s.writer.Events(ctx, f)
	if events == nil {
		events = []repository.MappingEvent{}
	}
	return events, err
}

// Refs lists the mapping reference: every chart title with its TMDB title
// and every external ID (kind "" | movie | tv; mappedOnly leaves out
// unmapped titles).
func (s *TitleMappingService) Refs(ctx context.Context, kind string, mappedOnly bool) ([]repository.MappingRef, error) {
	switch kind {
	case "", "movie", "tv":
	default:
		return nil, &ValidationError{Message: "invalid tmdb_kind, expected movie or tv"}
	}
	return s.repo.Refs(ctx, kind, mappedOnly)
}

// TitleIDsOf returns the IMDb, RT, Wikidata and JustWatch IDs of a TMDB
// title (empty for an unmapped title, or when unknown).
func (s *TitleMappingService) TitleIDsOf(ctx context.Context, ref TmdbRef) repository.TitleIDs {
	if s == nil || s.meta == nil || ref.ID == "" {
		return repository.TitleIDs{}
	}
	m, err := s.meta.Get(ctx, ref)
	if err != nil {
		s.log.Warn("tmdb title ids failed", slog.String("ref", ref.String()), slog.Any("err", err))
		return repository.TitleIDs{}
	}
	if m == nil {
		return repository.TitleIDs{}
	}
	return repository.TitleIDs{IMDbID: m.IMDbID, RTID: m.RottenTomatoesID, WikidataID: m.WikidataID, JustWatchID: m.JustWatchID}
}
