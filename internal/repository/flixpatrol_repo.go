package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

type FlixPatrolRepository struct {
	q *sqlc.Queries
}

func NewFlixPatrolRepository(db *sql.DB) *FlixPatrolRepository {
	return &FlixPatrolRepository{q: sqlc.New(db)}
}

type UpsertTitleInput struct {
	Slug   string
	Name   string
	Kind   string // "movie" | "tv_show"
	TmdbID string
	ImdbID string
	RtURL  string
}

func (r *FlixPatrolRepository) UpsertTitle(ctx context.Context, in UpsertTitleInput) (sqlc.Title, error) {
	if in.Slug == "" {
		return sqlc.Title{}, fmt.Errorf("slug is required")
	}
	if in.Name == "" {
		return sqlc.Title{}, fmt.Errorf("name is required")
	}
	if in.Kind != "movie" && in.Kind != "tv_show" {
		return sqlc.Title{}, fmt.Errorf("invalid kind: %s", in.Kind)
	}

	return r.q.UpsertTitle(ctx, sqlc.UpsertTitleParams{
		Slug:   in.Slug,
		Name:   in.Name,
		Kind:   in.Kind,
		TmdbID: sql.NullString{String: in.TmdbID, Valid: in.TmdbID != ""},
		ImdbID: sql.NullString{String: in.ImdbID, Valid: in.ImdbID != ""},
		RtUrl:  sql.NullString{String: in.RtURL, Valid: in.RtURL != ""},
	})
}

func (r *FlixPatrolRepository) GetTitleBySlug(ctx context.Context, slug string) (*sqlc.Title, error) {
	if slug == "" {
		return nil, fmt.Errorf("slug is required")
	}
	t, err := r.q.GetTitleBySlug(ctx, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

func (r *FlixPatrolRepository) ListTitles(ctx context.Context, kind, search string, limit, offset int64) ([]sqlc.Title, error) {
	return r.q.ListTitles(ctx, sqlc.ListTitlesParams{
		Kind:   kind,
		Name:   search,
		Limit:  limit,
		Offset: offset,
	})
}

func (r *FlixPatrolRepository) CountTitles(ctx context.Context, kind, search string) (int64, error) {
	return r.q.CountTitles(ctx, sqlc.CountTitlesParams{Kind: kind, Name: search})
}

func (r *FlixPatrolRepository) GetTitleByID(ctx context.Context, id int64) (*sqlc.Title, error) {
	t, err := r.q.GetTitleByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

type UpdateTitleIDsInput struct {
	ID     int64
	TmdbID sql.NullString
	ImdbID sql.NullString
	RtURL  sql.NullString
}

func (r *FlixPatrolRepository) UpdateTitleIDs(ctx context.Context, in UpdateTitleIDsInput) (sqlc.Title, error) {
	return r.q.UpdateTitleIDs(ctx, sqlc.UpdateTitleIDsParams{
		ID:     in.ID,
		TmdbID: in.TmdbID,
		ImdbID: in.ImdbID,
		RtUrl:  in.RtURL,
	})
}

type UpsertRankingInput struct {
	TitleID           int64
	RankedOn          time.Time
	Country           string // ISO 3166-1 alpha-2
	StreamingProvider string
	Category          string // "movies" | "tv_shows"
	Rank              int64  // 1..10
	SeasonNumber      *int64
}

func (r *FlixPatrolRepository) UpsertRanking(ctx context.Context, in UpsertRankingInput) (sqlc.Ranking, error) {
	var season sql.NullInt64
	if in.SeasonNumber != nil {
		season = sql.NullInt64{Int64: *in.SeasonNumber, Valid: true}
	}

	return r.q.UpsertRanking(ctx, sqlc.UpsertRankingParams{
		TitleID:           in.TitleID,
		RankedOn:          in.RankedOn,
		Country:           in.Country,
		StreamingProvider: in.StreamingProvider,
		Category:          in.Category,
		Rank:              in.Rank,
		SeasonNumber:      season,
	})
}

