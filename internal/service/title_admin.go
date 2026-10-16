package service

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

type TitleAdminService struct {
	repo    *repository.FlixPatrolRepository
	writer  *MappingWriter
	mapping *TitleMappingService // optional: linked Netflix titles follow manual fixes
}

func NewTitleAdminService(repo *repository.FlixPatrolRepository, writer *MappingWriter) *TitleAdminService {
	return &TitleAdminService{repo: repo, writer: writer}
}

// WithMapping makes manual fixes also update the Netflix titles linked to the
// FlixPatrol title (and remap their seasons).
func (s *TitleAdminService) WithMapping(mapping *TitleMappingService) *TitleAdminService {
	s.mapping = mapping
	return s
}

// PatchTitleIDsInput carries the patchable fields. A nil pointer means
// "leave unchanged"; a pointer to an empty string clears the ID to NULL.
// Kind ("movie" or "tv_show") says which TMDB namespace tmdb_id belongs to
// and cannot be cleared.
type PatchTitleIDsInput struct {
	Kind   *string `json:"kind"`
	TmdbID *string `json:"tmdb_id"`
	ImdbID *string `json:"imdb_id"`
	RtURL  *string `json:"rt_url"`
}

type TitleResponse struct {
	ID        int64     `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	TmdbID    *string   `json:"tmdb_id"`
	ImdbID    *string   `json:"imdb_id"`
	RtURL     *string   `json:"rt_url"`
	UpdatedAt time.Time `json:"updated_at"`
	// Matching state: pending | matched | unmatched | manual.
	MatchStatus   string     `json:"match_status"`
	MatchSource   *string    `json:"match_source,omitempty"`
	MatchedName   *string    `json:"matched_name,omitempty"`
	MatchedYear   *int64     `json:"matched_year,omitempty"`
	MatchAttempts int64      `json:"match_attempts"`
	NextMatchAt   *time.Time `json:"next_match_at,omitempty"`
	JustWatchID   *string    `json:"justwatch_id,omitempty"`
	WikidataID    *string    `json:"wikidata_id,omitempty"`
	// From the FlixPatrol title page, once read.
	FlixPatrol *FlixPatrolTitleDetails `json:"flixpatrol,omitempty"`
	// Set on list responses only.
	LastRankedOn  *string `json:"last_ranked_on,omitempty"`
	RankingsCount *int64  `json:"rankings_count,omitempty"`
	// Netflix official Top 10 titles linked to this one (they share its
	// TMDB mapping); set on single-title responses only.
	NetflixLinks []TitleNetflixLink `json:"netflix_links,omitempty"`
	// Every ID of its TMDB title, with sources (the same as the TMDB
	// mapping's); single-title responses of mapped titles only.
	IDs *TitleExternalIDs `json:"ids,omitempty"`
}

// TitleNetflixLink is a Netflix title linked to a FlixPatrol title.
type TitleNetflixLink struct {
	NetflixTitleID int64   `json:"netflix_title_id"`
	ShowTitle      string  `json:"show_title"`
	List           string  `json:"list"` // Netflix's list: movie | tv_show
	TmdbKind       *string `json:"tmdb_kind"`
	TmdbID         *string `json:"tmdb_id"`
	MatchStatus    string  `json:"match_status"`
}

// FlixPatrolTitleDetails is what the FlixPatrol title page said.
type FlixPatrolTitleDetails struct {
	Name         *string `json:"name"`
	Kind         *string `json:"kind"`
	PremiereDate *string `json:"premiere_date"`
	Country      *string `json:"country"`
}

// TitlesQuery filters and sorts ListTitlesPage.
type TitlesQuery struct {
	Kind        string // "" | movie | tv_show
	Search      string // name or slug substring
	Missing     string // "" | tmdb | imdb | rt
	MatchStatus string // "" | pending | matched | unmatched | manual
	Sort        string // see titleSorts
}

var titleSorts = map[string]bool{
	"": true, "id_desc": true, "id_asc": true, "name_asc": true, "name_desc": true,
	"updated_desc": true, "updated_asc": true, "last_ranked_desc": true, "last_ranked_asc": true, "rankings_desc": true, "rankings_asc": true,
}

type TitlesPageResponse struct {
	Items  []TitleResponse `json:"items"`
	Total  int64           `json:"total"`
	Limit  int64           `json:"limit"`
	Offset int64           `json:"offset"`
}

func (s *TitleAdminService) ListTitlesPage(ctx context.Context, q TitlesQuery, limit, offset int64) (*TitlesPageResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	switch q.Kind {
	case "", "movie", "tv_show":
	default:
		return nil, &ValidationError{Message: "invalid kind, expected movie or tv_show"}
	}
	switch q.Missing {
	case "", "tmdb", "imdb", "rt":
	default:
		return nil, &ValidationError{Message: "invalid missing, expected tmdb, imdb or rt"}
	}
	switch q.MatchStatus {
	case "", repository.MatchStatusPending, repository.MatchStatusMatched, repository.MatchStatusUnmatched, repository.MatchStatusManual:
	default:
		return nil, &ValidationError{Message: "invalid match_status, expected pending, matched, unmatched or manual"}
	}
	if !titleSorts[q.Sort] {
		return nil, &ValidationError{Message: "invalid sort"}
	}
	f := repository.TitleListFilter{Kind: q.Kind, Search: strings.TrimSpace(q.Search), Missing: q.Missing, MatchStatus: q.MatchStatus, Sort: q.Sort}
	rows, err := s.repo.ListTitles(ctx, f, limit, offset)
	if err != nil {
		return nil, err
	}
	total, err := s.repo.CountTitles(ctx, f)
	if err != nil {
		return nil, err
	}
	items := make([]TitleResponse, 0, len(rows))
	for _, t := range rows {
		resp := titleToResponse(t.Title, t.IDs)
		count := t.RankingsCount
		resp.RankingsCount = &count
		if t.LastRankedOn != nil {
			d := t.LastRankedOn.Format("2006-01-02")
			resp.LastRankedOn = &d
		}
		items = append(items, *resp)
	}
	return &TitlesPageResponse{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

// GetTitle returns one title; sql.ErrNoRows if it does not exist.
func (s *TitleAdminService) GetTitle(ctx context.Context, id int64) (*TitleResponse, error) {
	t, err := s.repo.GetTitleByID(ctx, id)
	if err != nil {
		return nil, err
	}
	resp := titleToResponse(*t, s.mapping.TitleIDsOf(ctx, repository.TitleRef(*t)))
	if s.mapping != nil {
		if ref := repository.TitleRef(*t); ref.ID != "" {
			if resp.IDs, err = s.mapping.ExternalIDs(ctx, ref); err != nil {
				return nil, err
			}
		}
		links, err := s.mapping.repo.NetflixLinksOf(ctx, []int64{id})
		if err != nil {
			return nil, err
		}
		for _, l := range links {
			resp.NetflixLinks = append(resp.NetflixLinks, TitleNetflixLink{
				NetflixTitleID: l.NetflixTitleID, ShowTitle: l.ShowTitle, List: l.Kind,
				TmdbKind: emptyToNil(l.TmdbKind), TmdbID: emptyToNil(l.TmdbID), MatchStatus: l.MatchStatus,
			})
		}
	}
	return resp, nil
}

// PatchTitleIDs applies a partial update to a title's external IDs.
// Returns sql.ErrNoRows if the title does not exist.
func (s *TitleAdminService) PatchTitleIDs(ctx context.Context, titleID int64, in PatchTitleIDsInput) (*TitleResponse, error) {
	existing, err := s.repo.GetTitleByID(ctx, titleID)
	if err != nil {
		return nil, err
	}

	kind, tmdbID := existing.Kind, existing.TmdbID.String
	if in.Kind != nil {
		if *in.Kind != "movie" && *in.Kind != "tv_show" {
			return nil, &ValidationError{Message: "kind must be movie or tv_show"}
		}
		kind = *in.Kind
	}
	if in.TmdbID != nil {
		tmdbID = strings.TrimSpace(*in.TmdbID)
	}
	imdbGiven := ""
	if in.ImdbID != nil {
		imdbGiven = strings.TrimSpace(*in.ImdbID)
	}
	remapped := tmdbID != "" && (tmdbID != existing.TmdbID.String || kind != existing.Kind)

	// The fix is the title's manual evidence; IMDb / RT IDs given are the
	// TMDB title's, set by hand ("" hands them back to TMDB / Wikidata /
	// matching).
	session := s.writer.Session(ActorAdmin)
	updated, err := session.SetTitleMapping(ctx, titleID, kind, tmdbID, imdbGiven)
	if err != nil {
		return nil, err
	}
	if ref := repository.TitleRef(updated); ref.ID != "" {
		if remapped {
			s.mapping.EnsureDetails(ctx, ref)
		}
		if err := s.mapping.SetManualIDs(ctx, session, ref, in.ImdbID, nil, in.RtURL); err != nil {
			return nil, err
		}
	}
	session.Flush(ctx)
	return s.GetTitle(ctx, updated.ID)
}

// titleToResponse builds a title's response; ids are its TMDB title's.
func titleToResponse(t sqlc.Title, ids repository.TitleIDs) *TitleResponse {
	resp := &TitleResponse{
		ID:        t.ID,
		Slug:      t.Slug,
		Name:      t.Name,
		Kind:      t.Kind,
		UpdatedAt: t.UpdatedAt,
	}
	if t.TmdbID.Valid {
		resp.TmdbID = &t.TmdbID.String
	}
	resp.ImdbID, resp.RtURL, resp.JustWatchID, resp.WikidataID = ids.IMDbID, ids.RTID, ids.JustWatchID, ids.WikidataID
	resp.MatchStatus = t.MatchStatus
	resp.MatchSource = nullStr(t.MatchSource)
	resp.MatchedName = nullStr(t.MatchedName)
	if t.MatchedYear.Valid {
		resp.MatchedYear = &t.MatchedYear.Int64
	}
	resp.MatchAttempts = t.MatchAttempts
	if t.NextMatchAt.Valid {
		v := t.NextMatchAt.Time.UTC()
		resp.NextMatchAt = &v
	}
	if t.FpFetchedAt.Valid {
		resp.FlixPatrol = &FlixPatrolTitleDetails{
			Name: nullStr(t.FpName), Kind: nullStr(t.FpKind), PremiereDate: nullStr(t.FpPremiereDate), Country: nullStr(t.FpCountry),
		}
	}
	return resp
}

func nullStr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

// Rematch puts a title back in the matching queue. clear also forgets its
// evidence (for a wrong match), and what linked titles took from it;
// otherwise it keeps its match and only lifts a manual lock and the retry
// backoff. Returns sql.ErrNoRows if not found.
func (s *TitleAdminService) Rematch(ctx context.Context, titleID int64, clear bool) (*TitleResponse, error) {
	session := s.writer.Session(ActorAdmin)
	defer session.Flush(ctx)
	t, err := session.RequeueTitle(ctx, titleID, clear)
	if err != nil {
		return nil, err
	}
	return titleToResponse(t, s.mapping.TitleIDsOf(ctx, repository.TitleRef(t))), nil
}
