package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// Title match statuses (titles.match_status).
const (
	MatchStatusPending   = "pending"
	MatchStatusMatched   = "matched"
	MatchStatusUnmatched = "unmatched"
	MatchStatusManual    = "manual"
)

// ListTitlesByIDs returns the titles with these IDs (missing IDs are skipped).
func (r *FlixPatrolRepository) ListTitlesByIDs(ctx context.Context, ids []int64) ([]sqlc.Title, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.q.ListTitlesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list titles by ids: %w", err)
	}
	return rows, nil
}

// ListTitlesDueForMatch returns up to limit unmatched titles whose retry
// backoff has expired.
func (r *FlixPatrolRepository) ListTitlesDueForMatch(ctx context.Context, limit int64) ([]sqlc.Title, error) {
	rows, err := r.q.ListTitlesDueForMatch(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list titles due for match: %w", err)
	}
	return rows, nil
}

// TitleMatchResult is the outcome of one matching attempt.
type TitleMatchResult struct {
	Kind        string // movie | tv_show
	TmdbID      string // empty = not matched
	ImdbID      string
	JustWatchID string
	Source      string
	MatchedName string
	MatchedYear int
	// RetryAfter is when to try again if unmatched, as a SQLite datetime
	// modifier ("+3 days"); ignored when matched.
	RetryAfter string
}

// FlixPatrolDetails is what the FlixPatrol title page says about a title.
type FlixPatrolDetails struct {
	Name         string
	Kind         string
	PremiereDate string // YYYY-MM-DD, or "" if unknown
	Country      string
}

func (r *FlixPatrolRepository) SaveFlixPatrolDetails(ctx context.Context, titleID int64, d FlixPatrolDetails) error {
	return r.q.SaveFlixPatrolDetails(ctx, sqlc.SaveFlixPatrolDetailsParams{
		FpName: nullIfEmpty(d.Name), FpKind: nullIfEmpty(d.Kind), FpPremiereDate: nullIfEmpty(d.PremiereDate),
		FpCountry: nullIfEmpty(d.Country), ID: titleID,
	})
}

// CountTitlesByMatchStatus returns the number of titles per match status.
func (r *FlixPatrolRepository) CountTitlesByMatchStatus(ctx context.Context) (map[string]int64, error) {
	rows, err := r.q.CountTitlesByMatchStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("count titles by match status: %w", err)
	}
	out := map[string]int64{}
	for _, row := range rows {
		out[row.MatchStatus] = row.Titles
	}
	return out, nil
}

// IsNoRows reports whether err is sql.ErrNoRows.
func IsNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }

// nullIfEmpty maps "" to NULL.
func nullIfEmpty(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

// LatestChart returns the country, provider and category of the most recent
// chart a title appeared on ("" values if it never charted).
func (r *FlixPatrolRepository) LatestChart(ctx context.Context, titleID int64) (country, provider, category string, err error) {
	row, err := r.q.GetLatestChartForTitle(ctx, titleID)
	if IsNoRows(err) {
		return "", "", "", nil
	}
	if err != nil {
		return "", "", "", fmt.Errorf("latest chart for title: %w", err)
	}
	return row.Country, row.StreamingProvider, row.Category, nil
}

// TmdbRefsChartingSince lists matched titles on any chart since since.
func (r *FlixPatrolRepository) TmdbRefsChartingSince(ctx context.Context, since time.Time, limit int64) ([]TmdbRef, error) {
	rows, err := r.q.ListTmdbRefsChartingSince(ctx, sqlc.ListTmdbRefsChartingSinceParams{Since: FormatDate(since), Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("list charting tmdb refs: %w", err)
	}
	out := make([]TmdbRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID})
	}
	return out, nil
}
