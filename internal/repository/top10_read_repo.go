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
	TitleID           int64
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
	PreviousRank      *int64 // nil if not on the chart's previous scraped date
	DaysInTop10       int64
}

// ListTop10ByProvider returns one chart. A nil rankedOn means the chart's
// latest scraped date.
func (r *FlixPatrolRepository) ListTop10ByProvider(ctx context.Context, rankedOn *time.Time, countryCode string, provider string, category string) ([]Top10Item, error) {
	rows, err := r.q.ListTop10ByProvider(ctx, sqlc.ListTop10ByProviderParams{
		RankedOn:          nullTime(rankedOn),
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
			TitleID:           row.TitleID,
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
			PreviousRank:      rankPtr(row.PreviousRank),
			DaysInTop10:       row.DaysInTop10,
		})
	}
	return out, nil
}

// ListTop10AllProviders returns every provider's chart for a country. A nil
// rankedOn means each provider's own latest scraped date.
func (r *FlixPatrolRepository) ListTop10AllProviders(ctx context.Context, rankedOn *time.Time, countryCode string, category string) ([]Top10Item, error) {
	rows, err := r.q.ListTop10AllProviders(ctx, sqlc.ListTop10AllProvidersParams{
		RankedOn: nullTime(rankedOn),
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
			TitleID:           row.TitleID,
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
			PreviousRank:      rankPtr(row.PreviousRank),
			DaysInTop10:       row.DaysInTop10,
		})
	}
	return out, nil
}

// ListTitlesByTmdbIDs returns every title row mapped to one of tmdbIDs, of
// any kind; callers filter by kind since TMDB movie and TV IDs overlap.
func (r *FlixPatrolRepository) ListTitlesByTmdbIDs(ctx context.Context, tmdbIDs []string) ([]sqlc.Title, error) {
	if len(tmdbIDs) == 0 {
		return nil, nil
	}
	ids := make([]sql.NullString, 0, len(tmdbIDs))
	for _, id := range tmdbIDs {
		ids = append(ids, sql.NullString{String: id, Valid: true})
	}
	rows, err := r.q.ListTitlesByTmdbIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list titles by tmdb ids: %w", err)
	}
	return rows, nil
}

type RankingHistoryFilter struct {
	Country string     // ISO code; empty = all countries
	From    *time.Time // inclusive; nil = unbounded
	To      *time.Time // inclusive; nil = unbounded
}

type RankingHistoryItem struct {
	TitleID           int64
	RankedOn          time.Time
	Country           string
	StreamingProvider string
	Category          string
	Rank              int64
}

func (r *FlixPatrolRepository) ListRankingsByTitleIDs(ctx context.Context, titleIDs []int64, f RankingHistoryFilter) ([]RankingHistoryItem, error) {
	if len(titleIDs) == 0 {
		return nil, nil
	}
	params := sqlc.ListRankingsByTitleIDsParams{TitleIds: titleIDs}
	if f.Country != "" {
		params.Country = f.Country
	}
	if f.From != nil {
		params.FromDate = f.From.UTC()
	}
	if f.To != nil {
		params.ToDate = f.To.UTC()
	}
	rows, err := r.q.ListRankingsByTitleIDs(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list rankings by title ids: %w", err)
	}
	out := make([]RankingHistoryItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, RankingHistoryItem{
			TitleID:           row.TitleID,
			RankedOn:          row.RankedOn,
			Country:           toString(row.Country),
			StreamingProvider: row.StreamingProvider,
			Category:          row.Category,
			Rank:              row.Rank,
		})
	}
	return out, nil
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}

// rankPtr converts the queries' previous_rank (0 = not ranked) to a pointer.
func rankPtr(v any) *int64 {
	var n int64
	switch x := v.(type) {
	case int64:
		n = x
	case int:
		n = int64(x)
	case float64:
		n = int64(x)
	}
	if n <= 0 {
		return nil
	}
	return &n
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
