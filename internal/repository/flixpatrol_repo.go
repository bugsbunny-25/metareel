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

// TitleListFilter narrows ListTitles / CountTitles; empty fields match all.
type TitleListFilter struct {
	Kind    string // "movie" | "tv_show"
	Search  string // name or slug substring
	Missing string // "tmdb" | "imdb" | "rt": only titles without that ID
	Sort    string // see ListTitles in db/queries/flixpatrol.sql
}

type TitleListItem struct {
	sqlc.Title
	LastRankedOn  *time.Time
	RankingsCount int64
}

func (r *FlixPatrolRepository) ListTitles(ctx context.Context, f TitleListFilter, limit, offset int64) ([]TitleListItem, error) {
	rows, err := r.q.ListTitles(ctx, sqlc.ListTitlesParams{
		Sort:    f.Sort,
		Kind:    f.Kind,
		Name:    f.Search,
		Missing: f.Missing,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list titles: %w", err)
	}
	out := make([]TitleListItem, 0, len(rows))
	for _, row := range rows {
		item := TitleListItem{
			Title: sqlc.Title{
				ID: row.ID, Slug: row.Slug, Name: row.Name, Kind: row.Kind,
				TmdbID: row.TmdbID, ImdbID: row.ImdbID, RtUrl: row.RtUrl,
				CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			},
			RankingsCount: row.RankingsCount,
		}
		item.LastRankedOn = parseStoredDate(row.LastRankedOn)
		out = append(out, item)
	}
	return out, nil
}

func (r *FlixPatrolRepository) CountTitles(ctx context.Context, f TitleListFilter) (int64, error) {
	return r.q.CountTitles(ctx, sqlc.CountTitlesParams{Kind: f.Kind, Name: f.Search, Missing: f.Missing})
}

type TitleStats struct {
	Total       int64
	MissingTmdb int64
	MissingImdb int64
	MissingRT   int64
	Rankings    int64
	LatestChart *time.Time
}

func (r *FlixPatrolRepository) TitleStats(ctx context.Context) (*TitleStats, error) {
	t, err := r.q.TitleMappingStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("title mapping stats: %w", err)
	}
	rk, err := r.q.RankingStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("ranking stats: %w", err)
	}
	return &TitleStats{
		Total: t.Total, MissingTmdb: t.MissingTmdb, MissingImdb: t.MissingImdb, MissingRT: t.MissingRt,
		Rankings: rk.Rankings, LatestChart: parseStoredDate(rk.LatestRankedOn),
	}, nil
}

// parseStoredDate parses a ranked_on value as SQLite returns it from an
// aggregate (the driver's text form of a time.Time).
func parseStoredDate(v string) *time.Time {
	if len(v) < 10 {
		return nil
	}
	t, err := time.Parse("2006-01-02", v[:10])
	if err != nil {
		return nil
	}
	return &t
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
	Kind   string // "movie" | "tv_show"; the TMDB namespace of TmdbID
	TmdbID sql.NullString
	ImdbID sql.NullString
	RtURL  sql.NullString
}

func (r *FlixPatrolRepository) UpdateTitleIDs(ctx context.Context, in UpdateTitleIDsInput) (sqlc.Title, error) {
	if in.Kind != "movie" && in.Kind != "tv_show" {
		return sqlc.Title{}, fmt.Errorf("invalid kind: %s", in.Kind)
	}
	return r.q.UpdateTitleIDs(ctx, sqlc.UpdateTitleIDsParams{
		ID:     in.ID,
		Kind:   in.Kind,
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

