package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// OpsRepository backs maintenance and alerting.
type OpsRepository struct {
	db *sql.DB
	q  *sqlc.Queries
}

func NewOpsRepository(db *sql.DB) *OpsRepository {
	return &OpsRepository{db: db, q: sqlc.New(db)}
}

// Setting returns an app setting ("" if unset).
func (r *OpsRepository) Setting(ctx context.Context, key string) (string, error) {
	v, err := r.q.GetAppSetting(ctx, key)
	if IsNoRows(err) {
		return "", nil
	}
	return v, err
}

func (r *OpsRepository) SetSetting(ctx context.Context, key, value string) error {
	return r.q.SetAppSetting(ctx, sqlc.SetAppSettingParams{Key: key, Value: value})
}

// ChartFreshness is the latest stored chart of a country × provider.
type ChartFreshness struct {
	Country       string
	Provider      string
	LatestDate    time.Time
	LastScrapedAt *time.Time
}

func (r *OpsRepository) LatestChartDates(ctx context.Context) ([]ChartFreshness, error) {
	rows, err := r.q.ListLatestChartDates(ctx)
	if err != nil {
		return nil, fmt.Errorf("list latest chart dates: %w", err)
	}
	out := make([]ChartFreshness, 0, len(rows))
	for _, row := range rows {
		d, err := time.Parse(DateLayout, row.LatestDate)
		if err != nil {
			continue
		}
		f := ChartFreshness{Country: row.Country, Provider: row.StreamingProvider, LatestDate: d}
		if t, err := time.Parse("2006-01-02 15:04:05", row.LastScrapedAt); err == nil {
			f.LastScrapedAt = &t
		}
		out = append(out, f)
	}
	return out, nil
}

// Optimize lets SQLite refresh query-planner statistics and checkpoints the
// WAL. (PRAGMAs are outside what sqlc generates, hence the raw statements.)
func (r *OpsRepository) Optimize(ctx context.Context) error {
	for _, stmt := range []string{"PRAGMA optimize", "PRAGMA wal_checkpoint(TRUNCATE)"} {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}

// CoverageWeek is mapping coverage of chart entries in one week.
type CoverageWeek struct {
	Week      string `json:"week"` // YYYY-WW
	FirstDate string `json:"first_date"`
	Entries   int64  `json:"entries"`
	Mapped    int64  `json:"mapped"`
	WithRT    int64  `json:"with_rt"`
}

func (r *OpsRepository) WeeklyMappingCoverage(ctx context.Context, since time.Time) ([]CoverageWeek, error) {
	rows, err := r.q.WeeklyMappingCoverage(ctx, FormatDate(since))
	if err != nil {
		return nil, fmt.Errorf("weekly mapping coverage: %w", err)
	}
	out := make([]CoverageWeek, 0, len(rows))
	for _, row := range rows {
		out = append(out, CoverageWeek{Week: toString(row.Week), FirstDate: dateText(row.FirstDate), Entries: row.Entries, Mapped: row.Mapped, WithRT: row.WithRt})
	}
	return out, nil
}

// CountryCount is a count per country.
type CountryCount struct {
	Country string `json:"country"`
	Titles  int64  `json:"titles"`
}

func (r *OpsRepository) UnmatchedTitlesByCountry(ctx context.Context, since time.Time) ([]CountryCount, error) {
	rows, err := r.q.UnmatchedTitlesByCountry(ctx, FormatDate(since))
	if err != nil {
		return nil, fmt.Errorf("unmatched titles by country: %w", err)
	}
	out := make([]CountryCount, 0, len(rows))
	for _, row := range rows {
		out = append(out, CountryCount{Country: row.Country, Titles: row.Titles})
	}
	return out, nil
}

// dateText renders a date column value (time.Time or text) as YYYY-MM-DD.
func dateText(v any) string {
	switch x := v.(type) {
	case time.Time:
		return FormatDate(x)
	case string:
		if len(x) >= 10 {
			return x[:10]
		}
		return x
	}
	return toString(v)
}
