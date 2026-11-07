package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// SearchRepository reads what the title search indexes.
type SearchRepository struct {
	q *sqlc.Queries
}

func NewSearchRepository(db *sql.DB) *SearchRepository {
	return &SearchRepository{q: sqlc.New(db)}
}

// SearchName is one name a TMDB title goes by and where it comes from
// (tmdb | flixpatrol | netflix).
type SearchName struct {
	Ref    TmdbRef
	Name   string
	Source string
}

// SearchTitle is a mapped TMDB title as a search result shows it.
type SearchTitle struct {
	Ref            TmdbRef
	Title          string
	OriginalTitle  string
	ReleaseDate    string
	PosterPath     string
	LastFlixPatrol string // YYYY-MM-DD or ""
	LastNetflix    string // YYYY-MM-DD (week) or ""
}

// SearchNames lists every name of every mapped TMDB title.
func (r *SearchRepository) SearchNames(ctx context.Context) ([]SearchName, error) {
	rows, err := r.q.ListSearchNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("list search names: %w", err)
	}
	out := make([]SearchName, 0, len(rows))
	for _, row := range rows {
		if row.Name.Valid && row.Name.String != "" {
			out = append(out, SearchName{Ref: TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID}, Name: row.Name.String, Source: row.Source})
		}
	}
	return out, nil
}

// SearchTitles lists every mapped TMDB title.
func (r *SearchRepository) SearchTitles(ctx context.Context) ([]SearchTitle, error) {
	rows, err := r.q.ListSearchTitles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list search titles: %w", err)
	}
	out := make([]SearchTitle, 0, len(rows))
	for _, row := range rows {
		if !row.TmdbID.Valid || row.TmdbID.String == "" {
			continue
		}
		out = append(out, SearchTitle{Ref: TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID.String}, Title: row.Title,
			OriginalTitle: row.OriginalTitle, ReleaseDate: row.ReleaseDate, PosterPath: row.PosterPath,
			LastFlixPatrol: row.LastFlixpatrol, LastNetflix: row.LastNetflix})
	}
	return out, nil
}

// IndexStamp changes whenever a name, mapping or chart changes.
func (r *SearchRepository) IndexStamp(ctx context.Context) (string, error) {
	s, err := r.q.SearchIndexStamp(ctx)
	if err != nil {
		return "", fmt.Errorf("search index stamp: %w", err)
	}
	return s, nil
}
