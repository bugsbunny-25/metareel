package service

import (
	"context"
	"database/sql"
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

// PatchTitleIDsInput carries the three patchable fields. A nil pointer means
// "leave unchanged"; a pointer to an empty string clears the field to NULL.
type PatchTitleIDsInput struct {
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
}

type TitlesPageResponse struct {
	Items  []TitleResponse `json:"items"`
	Total  int64           `json:"total"`
	Limit  int64           `json:"limit"`
	Offset int64           `json:"offset"`
}

func (s *TitleAdminService) ListTitlesPage(ctx context.Context, kind, search string, limit, offset int64) (*TitlesPageResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.repo.ListTitles(ctx, kind, search, limit, offset)
	if err != nil {
		return nil, err
	}
	total, err := s.repo.CountTitles(ctx, kind, search)
	if err != nil {
		return nil, err
	}
	items := make([]TitleResponse, 0, len(rows))
	for _, t := range rows {
		items = append(items, *titleToResponse(t))
	}
	return &TitlesPageResponse{Items: items, Total: total, Limit: limit, Offset: offset}, nil
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
		TmdbID: existing.TmdbID,
		ImdbID: existing.ImdbID,
		RtURL:  existing.RtUrl,
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
