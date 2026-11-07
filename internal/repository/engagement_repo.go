package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// EngagementRepository stores Netflix's "What We Watched" engagement reports.
type EngagementRepository struct {
	db *sql.DB
	q  *sqlc.Queries
}

func NewEngagementRepository(db *sql.DB) *EngagementRepository {
	return &EngagementRepository{db: db, q: sqlc.New(db)}
}

// EngagementRow is one title of a report.
type EngagementRow struct {
	PeriodStart       string `json:"period_start"`
	PeriodEnd         string `json:"period_end"`
	Kind              string `json:"kind"` // movie | tv_show
	Title             string `json:"title"`
	AvailableGlobally *bool  `json:"available_globally"`
	ReleaseDate       string `json:"release_date,omitempty"` // Netflix originals only
	HoursViewed       *int64 `json:"hours_viewed"`
	RuntimeMinutes    *int64 `json:"runtime_minutes"`
	Views             *int64 `json:"views"`
	SeasonNumber      *int64 `json:"season_number,omitempty"`
	NetflixTitleID    *int64 `json:"-"`
}

func nullBool(p *bool) sql.NullBool {
	if p == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: *p, Valid: true}
}

// SavePeriod replaces a report period's rows in one transaction.
func (r *EngagementRepository) SavePeriod(ctx context.Context, start, end string, rows []EngagementRow) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)
	if err := q.DeleteNetflixEngagementPeriod(ctx, start); err != nil {
		return fmt.Errorf("clear engagement period %s: %w", start, err)
	}
	for _, e := range rows {
		if err := q.InsertNetflixEngagement(ctx, sqlc.InsertNetflixEngagementParams{
			PeriodStart: start, PeriodEnd: end, Kind: e.Kind, Title: e.Title,
			AvailableGlobally: nullBool(e.AvailableGlobally), ReleaseDate: nullIfEmpty(e.ReleaseDate),
			HoursViewed: toNullInt64(e.HoursViewed), RuntimeMinutes: toNullInt64(e.RuntimeMinutes), Views: toNullInt64(e.Views),
			NetflixTitleID: toNullInt64(e.NetflixTitleID), SeasonNumber: toNullInt64(e.SeasonNumber),
		}); err != nil {
			return fmt.Errorf("insert engagement row %q: %w", e.Title, err)
		}
	}
	return tx.Commit()
}

// NetflixName is a Netflix Top 10 title or season title by name.
type NetflixName struct {
	NetflixTitleID int64
	Kind           string // movie | tv_show
	Name           string
	SeasonNumber   int64 // 0 = unknown / not a season
	IsSeason       bool
}

func (r *EngagementRepository) NetflixNames(ctx context.Context) ([]NetflixName, error) {
	rows, err := r.q.ListNetflixTitleNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("list netflix names: %w", err)
	}
	out := make([]NetflixName, 0, len(rows))
	for _, row := range rows {
		out = append(out, NetflixName{NetflixTitleID: row.ID, Kind: row.Kind, Name: row.Name, SeasonNumber: row.SeasonNumber, IsSeason: row.IsSeason == 1})
	}
	return out, nil
}

// ForTmdb returns the report rows of the Netflix titles mapped to a TMDB
// title, newest period first.
func (r *EngagementRepository) ForTmdb(ctx context.Context, ref TmdbRef) ([]EngagementRow, error) {
	rows, err := r.q.ListNetflixEngagementForTmdb(ctx, sqlc.ListNetflixEngagementForTmdbParams{TmdbKind: sql.NullString{String: ref.Kind, Valid: true}, TmdbID: sql.NullString{String: ref.ID, Valid: true}})
	if err != nil {
		return nil, fmt.Errorf("list engagement of %s: %w", ref, err)
	}
	out := make([]EngagementRow, 0, len(rows))
	for _, e := range rows {
		row := EngagementRow{PeriodStart: e.PeriodStart, PeriodEnd: e.PeriodEnd, Kind: e.Kind, Title: e.Title, ReleaseDate: e.ReleaseDate,
			HoursViewed: nullInt64Ptr(e.HoursViewed), RuntimeMinutes: nullInt64Ptr(e.RuntimeMinutes), Views: nullInt64Ptr(e.Views),
			SeasonNumber: nullInt64Ptr(e.SeasonNumber)}
		if e.AvailableGlobally.Valid {
			v := e.AvailableGlobally.Bool
			row.AvailableGlobally = &v
		}
		out = append(out, row)
	}
	return out, nil
}

// EngagementPeriod is an imported report.
type EngagementPeriod struct {
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	Titles      int64  `json:"titles"`
	Linked      int64  `json:"linked"` // rows linked to a Netflix Top 10 title
}

func (r *EngagementRepository) Periods(ctx context.Context) ([]EngagementPeriod, error) {
	rows, err := r.q.CountNetflixEngagement(ctx)
	if err != nil {
		return nil, fmt.Errorf("count engagement: %w", err)
	}
	out := make([]EngagementPeriod, 0, len(rows))
	for _, p := range rows {
		out = append(out, EngagementPeriod{PeriodStart: p.PeriodStart, PeriodEnd: p.PeriodEnd, Titles: p.Titles, Linked: p.Linked})
	}
	return out, nil
}

// ResetNetflixPremieres clears every TMDB title's Netflix premiere and sets
// it again from report rows linked to Netflix Top 10 titles (one
// transaction); it returns how many titles got one.
func (r *EngagementRepository) ResetNetflixPremieres(ctx context.Context) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)
	if err := q.ClearNetflixPremieres(ctx); err != nil {
		return 0, fmt.Errorf("clear netflix premieres: %w", err)
	}
	n, err := q.SetLinkedNetflixPremieres(ctx)
	if err != nil {
		return 0, fmt.Errorf("set linked netflix premieres: %w", err)
	}
	return n, tx.Commit()
}

// ReportOriginal is a Netflix original in a report not linked to a Netflix
// Top 10 title.
type ReportOriginal struct {
	Kind, Title, ReleaseDate string
}

func (r *EngagementRepository) UnlinkedOriginals(ctx context.Context) ([]ReportOriginal, error) {
	rows, err := r.q.ListUnlinkedNetflixOriginals(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unlinked originals: %w", err)
	}
	out := make([]ReportOriginal, 0, len(rows))
	for _, row := range rows {
		out = append(out, ReportOriginal{Kind: row.Kind, Title: row.Title, ReleaseDate: row.ReleaseDate})
	}
	return out, nil
}

// TmdbName is a stored TMDB title's names and release date.
type TmdbName struct {
	Ref                  TmdbRef
	Title, OriginalTitle string
	ReleaseDate          string
}

// TmdbTitlesWithoutPremiere lists stored TMDB titles without a Netflix
// premiere.
func (r *EngagementRepository) TmdbTitlesWithoutPremiere(ctx context.Context) ([]TmdbName, error) {
	rows, err := r.q.ListTmdbTitleNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tmdb title names: %w", err)
	}
	out := make([]TmdbName, 0, len(rows))
	for _, row := range rows {
		out = append(out, TmdbName{Ref: TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID}, Title: row.Title, OriginalTitle: row.OriginalTitle, ReleaseDate: row.ReleaseDate})
	}
	return out, nil
}

// SetNetflixPremiere records a TMDB title's Netflix premiere (keeping an
// earlier one).
func (r *EngagementRepository) SetNetflixPremiere(ctx context.Context, ref TmdbRef, premiere, source string) error {
	if err := r.q.SetNetflixPremiere(ctx, sqlc.SetNetflixPremiereParams{TmdbKind: ref.Kind, TmdbID: ref.ID, Premiere: premiere, Source: nullIfEmpty(source)}); err != nil {
		return fmt.Errorf("set netflix premiere of %s: %w", ref, err)
	}
	return nil
}
