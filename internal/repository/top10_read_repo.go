package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

type Top10Item struct {
	Rank              int64
	RankedOn          time.Time
	Country           string
	StreamingProvider string
	Category          string
	Slug              string
	Name              string
	Kind              string
	TMDBID            *string
	IMDbID            *string
	RottenTomatoesURL *string
}

func (r *FlixPatrolRepository) ListTop10ByProvider(ctx context.Context, rankedOn time.Time, countryCode string, provider string, category string) ([]Top10Item, error) {
	rows, err := r.q.ListTop10ByProvider(ctx, sqlc.ListTop10ByProviderParams{
		RankedOn:          rankedOn,
		Country:           countryCode,
		StreamingProvider: provider,
		Category:          category,
	})
	if err != nil {
		return nil, fmt.Errorf("list top10 by provider: %w", err)
	}
	out := make([]Top10Item, 0, len(rows))
	for _, row := range rows {
		out = append(out, Top10Item{
			Rank:              row.Rank,
			RankedOn:          row.RankedOn,
			Country:           toString(row.Country),
			StreamingProvider: row.StreamingProvider,
			Category:          row.Category,
			Slug:              row.Slug,
			Name:              row.Name,
			Kind:              row.Kind,
			TMDBID:            nullStringPtr(row.TmdbID),
			IMDbID:            nullStringPtr(row.ImdbID),
			RottenTomatoesURL: nullStringPtr(row.RtUrl),
		})
	}
	return out, nil
}

func (r *FlixPatrolRepository) ListTop10AllProviders(ctx context.Context, rankedOn time.Time, countryCode string, category string) ([]Top10Item, error) {
	rows, err := r.q.ListTop10AllProviders(ctx, sqlc.ListTop10AllProvidersParams{
		RankedOn: rankedOn,
		Country:  countryCode,
		Category: category,
	})
	if err != nil {
		return nil, fmt.Errorf("list top10 all providers: %w", err)
	}
	out := make([]Top10Item, 0, len(rows))
	for _, row := range rows {
		out = append(out, Top10Item{
			Rank:              row.Rank,
			RankedOn:          row.RankedOn,
			Country:           toString(row.Country),
			StreamingProvider: row.StreamingProvider,
			Category:          row.Category,
			Slug:              row.Slug,
			Name:              row.Name,
			Kind:              row.Kind,
			TMDBID:            nullStringPtr(row.TmdbID),
			IMDbID:            nullStringPtr(row.ImdbID),
			RottenTomatoesURL: nullStringPtr(row.RtUrl),
		})
	}
	return out, nil
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func nullInt64Ptr(v any) *int64 {
	switch x := v.(type) {
	case sql.NullInt64:
		if x.Valid {
			return &x.Int64
		}
	case struct {
		Int64 int64
		Valid bool
	}:
		if x.Valid {
			return &x.Int64
		}
	}
	return nil
}

func nullStringPtr(v any) *string {
	switch x := v.(type) {
	case sql.NullString:
		if x.Valid {
			return &x.String
		}
	case struct {
		String string
		Valid  bool
	}:
		if x.Valid {
			return &x.String
		}
	}
	return nil
}

