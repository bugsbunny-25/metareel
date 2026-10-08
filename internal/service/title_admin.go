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
	repo *repository.FlixPatrolRepository
}

func NewTitleAdminService(repo *repository.FlixPatrolRepository) *TitleAdminService {
	return &TitleAdminService{repo: repo}
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
	// Set on list responses only.
	LastRankedOn  *string `json:"last_ranked_on,omitempty"`
	RankingsCount *int64  `json:"rankings_count,omitempty"`
}

// TitlesQuery filters and sorts ListTitlesPage.
type TitlesQuery struct {
	Kind    string // "" | movie | tv_show
	Search  string // name or slug substring
	Missing string // "" | tmdb | imdb | rt
	Sort    string // see titleSorts
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
	if !titleSorts[q.Sort] {
		return nil, &ValidationError{Message: "invalid sort"}
	}
	f := repository.TitleListFilter{Kind: q.Kind, Search: strings.TrimSpace(q.Search), Missing: q.Missing, Sort: q.Sort}
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
		resp := titleToResponse(t.Title)
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
	return titleToResponse(*t), nil
}

// PatchTitleIDs applies a partial update to a title's external IDs.
// Returns sql.ErrNoRows if the title does not exist.
func (s *TitleAdminService) PatchTitleIDs(ctx context.Context, titleID int64, in PatchTitleIDsInput) (*TitleResponse, error) {
	existing, err := s.repo.GetTitleByID(ctx, titleID)
	if err != nil {
		return nil, err
	}

	merged := repository.UpdateTitleIDsInput{
		ID:     titleID,
		Kind:   existing.Kind,
		TmdbID: existing.TmdbID,
		ImdbID: existing.ImdbID,
		RtURL:  existing.RtUrl,
	}
	if in.Kind != nil {
		if *in.Kind != "movie" && *in.Kind != "tv_show" {
			return nil, &ValidationError{Message: "kind must be movie or tv_show"}
		}
		merged.Kind = *in.Kind
	}
	if in.TmdbID != nil {
		merged.TmdbID = sql.NullString{String: *in.TmdbID, Valid: *in.TmdbID != ""}
	}
	if in.ImdbID != nil {
		merged.ImdbID = sql.NullString{String: *in.ImdbID, Valid: *in.ImdbID != ""}
	}
	if in.RtURL != nil {
		merged.RtURL = sql.NullString{String: *in.RtURL, Valid: *in.RtURL != ""}
	}

	updated, err := s.repo.UpdateTitleIDs(ctx, merged)
	if err != nil {
		return nil, err
	}
	return titleToResponse(updated), nil
}

func titleToResponse(t sqlc.Title) *TitleResponse {
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
	if t.ImdbID.Valid {
		resp.ImdbID = &t.ImdbID.String
	}
	if t.RtUrl.Valid {
		resp.RtURL = &t.RtUrl.String
	}
	return resp
}
