package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// NetflixAdminService reviews and fixes how Netflix's official Top 10
// titles are mapped to TMDB / IMDb and how their season titles map to
// season numbers (the admin UI's Netflix titles page).
type NetflixAdminService struct {
	log        *slog.Logger
	repo       *repository.ImportsRepository
	writer     *MappingWriter
	meta       *repository.MetadataRepository
	metadata   *TitleMetadataService // optional: fetches TMDB details (seasons) after a manual fix
	seasons    *SeasonMapper
	candidates *TitleCandidatesService
	mapping    *TitleMappingService // optional: shares fixes with linked FlixPatrol titles
}

func NewNetflixAdminService(log *slog.Logger, repo *repository.ImportsRepository, writer *MappingWriter, meta *repository.MetadataRepository, metadata *TitleMetadataService, seasons *SeasonMapper, candidates *TitleCandidatesService) *NetflixAdminService {
	return &NetflixAdminService{log: log, repo: repo, writer: writer, meta: meta, metadata: metadata, seasons: seasons, candidates: candidates}
}

// WithMapping shares manual fixes with linked FlixPatrol titles.
func (s *NetflixAdminService) WithMapping(mapping *TitleMappingService) *NetflixAdminService {
	s.mapping = mapping
	return s
}

// ErrNetflixTitleNotFound / ErrNetflixSeasonNotFound map to 404.
var (
	ErrNetflixTitleNotFound  = errors.New("netflix title not found")
	ErrNetflixSeasonNotFound = errors.New("netflix season not found")
)

// NetflixTitleResponse is a Netflix title and its mapping.
type NetflixTitleResponse struct {
	ID            int64      `json:"id"`
	ShowTitle     string     `json:"show_title"`
	Kind          string     `json:"kind"`      // Netflix's list: movie (Films) | tv_show (TV)
	TmdbKind      *string    `json:"tmdb_kind"` // movie | tv: TMDB namespace of tmdb_id (can differ from the list)
	TmdbID        *string    `json:"tmdb_id"`
	ImdbID        *string    `json:"imdb_id"`      // the TMDB title's
	NetflixID     *string    `json:"netflix_id"`   // Netflix's title ID (netflix.com/title/<id>), from Tudum's Top 10 search
	MatchStatus   string     `json:"match_status"` // pending | matched | unmatched | manual
	MatchSource   *string    `json:"match_source,omitempty"`
	MatchAttempts int64      `json:"match_attempts"`
	NextMatchAt   *time.Time `json:"next_match_at,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at"`
	// TMDB title it is mapped to (once TMDB details are fetched).
	TmdbTitle *string `json:"tmdb_title,omitempty"`
	TmdbYear  *int    `json:"tmdb_year,omitempty"`
	// FlixPatrol title it was matched through, if any.
	FlixPatrol *NetflixFlixPatrolLink `json:"flixpatrol,omitempty"`
	// FlixPatrol titles linked to it (same title; they share one mapping).
	FlixPatrolLinks int64 `json:"flixpatrol_links"`
	// Stats.
	LastWeek        *string `json:"last_week"`
	WeeksGlobal     int64   `json:"weeks_in_global_top10"`
	BestGlobalRank  *int64  `json:"best_global_rank"`
	SeasonsTotal    int64   `json:"seasons"`
	SeasonsUnmapped int64   `json:"seasons_unmapped"`
}

type NetflixFlixPatrolLink struct {
	TitleID int64  `json:"title_id"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
}

// NetflixTitleDetail adds seasons, TMDB's season list and chart history.
type NetflixTitleDetail struct {
	NetflixTitleResponse
	Links       []NetflixLinkedTitle             `json:"linked_titles"`
	Seasons     []repository.NetflixSeasonStats  `json:"season_titles"`
	TmdbSeasons []repository.TmdbSeason          `json:"tmdb_seasons"`
	GlobalWeeks []repository.NetflixTitleWeek    `json:"global_weeks"`
	Countries   []repository.NetflixTitleCountry `json:"countries"`
}

// NetflixLinkedTitle is a FlixPatrol title linked to a Netflix title.
type NetflixLinkedTitle struct {
	TitleID     int64   `json:"title_id"`
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Kind        string  `json:"kind"` // movie | tv_show (TMDB namespace)
	TmdbID      *string `json:"tmdb_id"`
	MatchStatus string  `json:"match_status"`
}

type NetflixTitlesPage struct {
	Items  []NetflixTitleResponse `json:"items"`
	Total  int64                  `json:"total"`
	Limit  int64                  `json:"limit"`
	Offset int64                  `json:"offset"`
}

// NetflixTitlesQuery filters the admin list.
type NetflixTitlesQuery struct {
	Kind        string // "" | movie | tv_show
	Search      string // show title substring, or a TMDB ID
	MatchStatus string // "" | pending | matched | unmatched | manual
	Missing     string // "" | tmdb | season
	Flag        string // "" | cross_kind | linked
	Sort        string
}

var netflixTitleSorts = map[string]bool{
	"": true, "last_week_desc": true, "last_week_asc": true, "name_asc": true, "name_desc": true,
	"weeks_desc": true, "weeks_asc": true, "best_rank_asc": true, "id_desc": true,
}

func (s *NetflixAdminService) List(ctx context.Context, q NetflixTitlesQuery, limit, offset int64) (*NetflixTitlesPage, error) {
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 500)
	offset = max(offset, 0)
	switch q.Kind {
	case "", "movie", "tv_show":
	default:
		return nil, &ValidationError{Message: "invalid kind, expected movie or tv_show"}
	}
	switch q.MatchStatus {
	case "", repository.MatchStatusPending, repository.MatchStatusMatched, repository.MatchStatusUnmatched, repository.MatchStatusManual:
	default:
		return nil, &ValidationError{Message: "invalid match_status, expected pending, matched, unmatched or manual"}
	}
	switch q.Missing {
	case "", "tmdb", "season":
	default:
		return nil, &ValidationError{Message: "invalid missing, expected tmdb or season"}
	}
	switch q.Flag {
	case "", "cross_kind", "linked":
	default:
		return nil, &ValidationError{Message: "invalid flag, expected cross_kind or linked"}
	}
	if !netflixTitleSorts[q.Sort] {
		return nil, &ValidationError{Message: "invalid sort"}
	}
	f := repository.NetflixTitleFilter{Kind: q.Kind, Search: strings.TrimSpace(q.Search), MatchStatus: q.MatchStatus, Missing: q.Missing, Flag: q.Flag, Sort: q.Sort}
	rows, err := s.repo.ListNetflixTitles(ctx, f, limit, offset)
	if err != nil {
		return nil, err
	}
	total, err := s.repo.CountNetflixTitles(ctx, f)
	if err != nil {
		return nil, err
	}
	items := make([]NetflixTitleResponse, 0, len(rows))
	for _, r := range rows {
		items = append(items, netflixTitleResponse(r))
	}
	return &NetflixTitlesPage{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

// Get returns a title with its seasons and chart history.
func (s *NetflixAdminService) Get(ctx context.Context, id int64) (*NetflixTitleDetail, error) {
	row, err := s.repo.GetNetflixTitle(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNetflixTitleNotFound
	}
	if err != nil {
		return nil, err
	}
	out := &NetflixTitleDetail{NetflixTitleResponse: netflixTitleResponse(*row), TmdbSeasons: []repository.TmdbSeason{}, Links: []NetflixLinkedTitle{}}
	if s.mapping != nil {
		links, err := s.mapping.repo.FlixPatrolLinksOf(ctx, []int64{id})
		if err != nil {
			return nil, err
		}
		for _, l := range links {
			out.Links = append(out.Links, NetflixLinkedTitle{TitleID: l.TitleID, Slug: l.Slug, Name: l.Name, Kind: l.Kind, TmdbID: emptyToNil(l.TmdbID), MatchStatus: l.MatchStatus})
		}
	}
	if out.Seasons, err = s.repo.NetflixSeasonsForTitle(ctx, id); err != nil {
		return nil, err
	}
	if out.GlobalWeeks, out.Countries, err = s.repo.NetflixWeeksForTitle(ctx, id); err != nil {
		return nil, err
	}
	if row.TmdbKind.String == "tv" && row.TmdbID.Valid {
		m, err := s.meta.Get(ctx, repository.TmdbRef{Kind: "tv", ID: row.TmdbID.String})
		if err != nil {
			return nil, err
		}
		if m != nil && m.Seasons != nil {
			out.TmdbSeasons = m.Seasons
		}
	}
	return out, nil
}

// PatchNetflixTitleInput carries the patchable IDs: nil = unchanged, "" =
// clear. TmdbKind (movie | tv) is the TMDB namespace of tmdb_id; it defaults
// to the current one, else Netflix's list. Without a TMDB ID, an IMDb ID or
// a Wikidata ID (Q…) is resolved to one through TMDB / Wikidata.
type PatchNetflixTitleInput struct {
	TmdbKind   *string `json:"tmdb_kind"`
	TmdbID     *string `json:"tmdb_id"`
	ImdbID     *string `json:"imdb_id"`
	WikidataID *string `json:"wikidata_id"`
}

var tmdbNumericRe = regexp.MustCompile(`^\d{1,10}$`)

// Patch stores IDs fixed by hand and locks the title against automatic
// matching, then fetches the TMDB details (for its season list) and remaps
// the title's seasons.
func (s *NetflixAdminService) Patch(ctx context.Context, id int64, in PatchNetflixTitleInput) (*NetflixTitleDetail, error) {
	row, err := s.repo.GetNetflixTitle(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNetflixTitleNotFound
	}
	if err != nil {
		return nil, err
	}
	tmdbKind, tmdbID, imdbID := row.TmdbKind.String, row.TmdbID.String, ""
	if in.TmdbKind != nil {
		tmdbKind = strings.TrimSpace(*in.TmdbKind)
		if tmdbKind != "movie" && tmdbKind != "tv" {
			return nil, &ValidationError{Message: "tmdb_kind must be movie or tv"}
		}
	}
	if tmdbKind == "" {
		tmdbKind = tmdbKindOf(flixpatrol.TitleKind(row.Kind))
	}
	if in.TmdbID != nil {
		tmdbID = strings.TrimSpace(*in.TmdbID)
		if tmdbID != "" && !tmdbNumericRe.MatchString(tmdbID) {
			return nil, &ValidationError{Message: "tmdb_id must be a numeric TMDB ID"}
		}
	}
	if in.ImdbID != nil {
		imdbID = strings.TrimSpace(*in.ImdbID)
		if imdbID != "" && !imdbIDRe.MatchString(imdbID) {
			return nil, &ValidationError{Message: "imdb_id must look like tt1234567"}
		}
	}
	var wikidataID string
	if in.WikidataID != nil {
		wikidataID = strings.ToUpper(strings.TrimSpace(*in.WikidataID))
		if wikidataID != "" && !client.WikidataQIDRe.MatchString(wikidataID) {
			return nil, &ValidationError{Message: "wikidata_id must look like Q141461750"}
		}
	}
	if tmdbID == "" && (imdbID != "" || wikidataID != "") {
		if s.metadata == nil {
			return nil, &ValidationError{Message: "tmdb_id is required (resolving IMDb / Wikidata IDs needs TMDB)"}
		}
		ext := wikidataID
		if ext == "" {
			ext = imdbID
		}
		hint := ""
		if in.TmdbKind != nil {
			hint = tmdbKind
		}
		r, err := s.metadata.ResolveExternalID(ctx, ext, hint)
		if err != nil {
			return nil, err
		}
		tmdbKind, tmdbID = r.Ref.Kind, r.Ref.ID
	}
	session := s.writer.Session(ActorAdmin)
	if err := session.SetNetflixMapping(ctx, id, tmdbKind, tmdbID, imdbID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNetflixTitleNotFound
		}
		return nil, err
	}
	if tmdbID != "" {
		ref := repository.TmdbRef{Kind: tmdbKind, ID: tmdbID}
		s.mapping.EnsureDetails(ctx, ref)
		// IMDb / Wikidata IDs given here belong to the TMDB title: stored as
		// set by hand, so TMDB and Wikidata never replace them ("" hands
		// them back).
		var imdbSet, wikiSet *string
		if in.ImdbID != nil {
			imdbSet = &imdbID
		}
		if in.WikidataID != nil {
			wikiSet = &wikidataID
		}
		if err := s.mapping.SetManualIDs(ctx, session, ref, imdbSet, wikiSet, nil); err != nil {
			return nil, err
		}
	}
	if err := s.seasons.RefreshTitle(ctx, id); err != nil {
		return nil, err
	}
	session.Flush(ctx)
	return s.Get(ctx, id)
}

// Rematch puts a title back in the matching queue; clear also forgets its
// evidence (FlixPatrol titles that took its mapping lose it too). The caller
// enqueues netflix.titles.match for it.
func (s *NetflixAdminService) Rematch(ctx context.Context, id int64, clear bool) (*NetflixTitleDetail, error) {
	session := s.writer.Session(ActorAdmin)
	defer session.Flush(ctx)
	if err := session.RequeueNetflixTitle(ctx, id, clear); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNetflixTitleNotFound
		}
		return nil, err
	}
	if clear {
		if err := s.seasons.RefreshTitle(ctx, id); err != nil {
			return nil, err
		}
	}
	return s.Get(ctx, id)
}

// Candidates suggests TMDB / IMDb mappings for a Netflix title (Netflix gives
// no year; pass one to narrow Rotten Tomatoes).
func (s *NetflixAdminService) Candidates(ctx context.Context, id int64, q TitleCandidatesQuery) (*TitleCandidatesResponse, error) {
	row, err := s.repo.GetNetflixTitle(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNetflixTitleNotFound
	}
	if err != nil {
		return nil, err
	}
	query := CandidateQuery{Name: strings.TrimSpace(q.Name), Year: q.Year, Kind: q.Kind}
	if query.Name == "" {
		query.Name = row.ShowTitle
	}
	if query.Kind == "" {
		query.Kind = row.Kind
	}
	return s.candidates.SearchQuery(ctx, query)
}

// PatchNetflixSeasonInput fixes a season's number by hand, or (Auto) hands
// it back to automatic mapping.
type PatchNetflixSeasonInput struct {
	SeasonNumber *int64 `json:"season_number"`
	Auto         bool   `json:"auto"`
}

// PatchSeason fixes or resets a season mapping, re-tags the rankings that
// depend on it and returns the title's detail.
func (s *NetflixAdminService) PatchSeason(ctx context.Context, seasonID int64, in PatchNetflixSeasonInput) (*NetflixTitleDetail, error) {
	season, err := s.repo.GetNetflixSeason(ctx, seasonID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNetflixSeasonNotFound
	}
	if err != nil {
		return nil, err
	}
	switch {
	case in.Auto:
		if _, err := s.repo.ResetNetflixSeason(ctx, seasonID); err != nil {
			return nil, err
		}
	case in.SeasonNumber == nil:
		return nil, &ValidationError{Message: "season_number is required (or auto: true)"}
	case *in.SeasonNumber < 0 || *in.SeasonNumber > 999:
		return nil, &ValidationError{Message: "season_number must be between 0 and 999"}
	default:
		if _, err := s.repo.SetNetflixSeasonManual(ctx, seasonID, *in.SeasonNumber); err != nil {
			return nil, err
		}
	}
	if err := s.seasons.RefreshTitle(ctx, season.NetflixTitleID); err != nil {
		return nil, err
	}
	return s.Get(ctx, season.NetflixTitleID)
}

func netflixTitleResponse(r repository.NetflixTitleRow) NetflixTitleResponse {
	out := NetflixTitleResponse{
		ID: r.ID, ShowTitle: r.ShowTitle, Kind: r.Kind, TmdbKind: nullStr(r.TmdbKind), TmdbID: nullStr(r.TmdbID), ImdbID: r.IMDbID, NetflixID: nullStr(r.NetflixID),
		MatchStatus: r.MatchStatus, MatchSource: nullStr(r.MatchSource), MatchAttempts: r.MatchAttempts, UpdatedAt: r.UpdatedAt.UTC(),
		TmdbTitle: r.TmdbTitle, LastWeek: emptyToNil(r.LastWeek), WeeksGlobal: r.WeeksGlobal, BestGlobalRank: r.BestGlobalRank,
		SeasonsTotal: r.SeasonsTotal, SeasonsUnmapped: r.SeasonsUnmapped, FlixPatrolLinks: r.FlixPatrolLinks,
	}
	if r.NextMatchAt.Valid {
		v := r.NextMatchAt.Time.UTC()
		out.NextMatchAt = &v
	}
	if r.TmdbReleaseDate != nil && len(*r.TmdbReleaseDate) >= 4 {
		if y, err := strconv.Atoi((*r.TmdbReleaseDate)[:4]); err == nil {
			out.TmdbYear = &y
		}
	}
	if r.TitleID.Valid && r.FlixPatrolSlug != nil {
		out.FlixPatrol = &NetflixFlixPatrolLink{TitleID: r.TitleID.Int64, Slug: *r.FlixPatrolSlug, Name: derefStr(r.FlixPatrolName)}
	}
	return out
}

func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
