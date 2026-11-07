package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// CreditsRepository stores TMDB cast and crew and reads them for the
// credits, people and people-analytics endpoints.
type CreditsRepository struct {
	db *sql.DB
	q  *sqlc.Queries
}

func NewCreditsRepository(db *sql.DB) *CreditsRepository {
	return &CreditsRepository{db: db, q: sqlc.New(db)}
}

// CreditTarget is a TMDB title whose credits may be fetched.
type CreditTarget struct {
	Ref       TmdbRef
	FetchedAt string // "YYYY-MM-DD HH:MM:SS" UTC, "" = never
}

// CreditTargets lists every TMDB title, never fetched first, then oldest.
func (r *CreditsRepository) CreditTargets(ctx context.Context) ([]CreditTarget, error) {
	rows, err := r.q.ListCreditTargets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credit targets: %w", err)
	}
	out := make([]CreditTarget, 0, len(rows))
	for _, row := range rows {
		out = append(out, CreditTarget{Ref: TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID}, FetchedAt: row.CreditsFetchedAt})
	}
	return out, nil
}

// Credit kinds.
const (
	CreditCast = "cast"
	CreditCrew = "crew"
)

// SaveCredits replaces a title's credits (cast, crew already filtered) and
// marks it fetched, in one transaction.
func (r *CreditsRepository) SaveCredits(ctx context.Context, ref TmdbRef, cast, crew []client.TMDBCredit) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)
	if err := q.DeleteTitleCredits(ctx, sqlc.DeleteTitleCreditsParams{TmdbKind: ref.Kind, TmdbID: ref.ID}); err != nil {
		return fmt.Errorf("delete credits of %s: %w", ref, err)
	}
	save := func(creditType string, list []client.TMDBCredit) error {
		for _, c := range list {
			if err := q.UpsertPerson(ctx, sqlc.UpsertPersonParams{PersonID: int64(c.PersonID), Name: c.Name,
				ProfilePath: nullIfEmpty(c.ProfilePath), KnownFor: nullIfEmpty(c.KnownFor)}); err != nil {
				return fmt.Errorf("save person %d: %w", c.PersonID, err)
			}
			if err := q.InsertCredit(ctx, sqlc.InsertCreditParams{TmdbKind: ref.Kind, TmdbID: ref.ID, PersonID: int64(c.PersonID),
				CreditType: creditType, Job: c.Job, Department: c.Department, Character: nullIfEmpty(c.Character),
				CreditOrder: int64(c.Order), Episodes: nullIfZero(c.Episodes)}); err != nil {
				return fmt.Errorf("save credit of %s: %w", ref, err)
			}
		}
		return nil
	}
	if err := save(CreditCast, cast); err != nil {
		return err
	}
	if err := save(CreditCrew, crew); err != nil {
		return err
	}
	if err := q.SetCreditsFetched(ctx, sqlc.SetCreditsFetchedParams{TmdbKind: ref.Kind, TmdbID: ref.ID}); err != nil {
		return fmt.Errorf("mark credits fetched of %s: %w", ref, err)
	}
	return tx.Commit()
}

// Credit is a stored credit with its person.
type Credit struct {
	PersonID    int64
	Name        string
	ProfilePath string
	Type        string // cast | crew
	Job         string
	Department  string
	Character   string
	Order       int64
	Episodes    *int64
}

// TitleCredits lists a title's credits, cast first by billing order.
func (r *CreditsRepository) TitleCredits(ctx context.Context, ref TmdbRef) ([]Credit, error) {
	rows, err := r.q.ListTitleCredits(ctx, sqlc.ListTitleCreditsParams{TmdbKind: ref.Kind, TmdbID: ref.ID})
	if err != nil {
		return nil, fmt.Errorf("list title credits: %w", err)
	}
	out := make([]Credit, 0, len(rows))
	for _, row := range rows {
		out = append(out, Credit{PersonID: row.PersonID, Name: row.Name, ProfilePath: row.ProfilePath, Type: row.CreditType, Job: row.Job,
			Department: row.Department, Character: row.Character, Order: row.CreditOrder, Episodes: nullInt64Ptr(row.Episodes)})
	}
	return out, nil
}

// Person is a TMDB person.
type Person struct {
	ID          int64
	Name        string
	ProfilePath string
	KnownFor    string
}

// GetPerson returns a person, or nil when unknown.
func (r *CreditsRepository) GetPerson(ctx context.Context, id int64) (*Person, error) {
	row, err := r.q.GetPerson(ctx, id)
	if IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get person: %w", err)
	}
	return &Person{ID: row.PersonID, Name: row.Name, ProfilePath: row.ProfilePath, KnownFor: row.KnownFor}, nil
}

// PersonCredit is a person's credit with its title.
type PersonCredit struct {
	Ref         TmdbRef
	Title       string
	ReleaseDate string
	PosterPath  string
	Credit
}

// PersonCredits lists a person's credits on the titles metareel knows,
// newest release first.
func (r *CreditsRepository) PersonCredits(ctx context.Context, id int64) ([]PersonCredit, error) {
	rows, err := r.q.ListPersonCredits(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list person credits: %w", err)
	}
	out := make([]PersonCredit, 0, len(rows))
	for _, row := range rows {
		out = append(out, PersonCredit{Ref: TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID}, Title: row.Title, ReleaseDate: row.ReleaseDate,
			PosterPath: row.PosterPath, Credit: Credit{PersonID: id, Type: row.CreditType, Job: row.Job, Department: row.Department,
				Character: row.Character, Order: row.CreditOrder, Episodes: nullInt64Ptr(row.Episodes)}})
	}
	return out, nil
}

// TitleChartStats are a title's FlixPatrol chart totals.
type TitleChartStats struct {
	Days, Points, BestRank, Countries int64
	FirstCharted, LastCharted         string
}

// PersonChartStats returns the chart totals of each title a person has a
// credit on, keyed by ref.String().
func (r *CreditsRepository) PersonChartStats(ctx context.Context, id int64) (map[string]TitleChartStats, error) {
	rows, err := r.q.ListPersonChartStats(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list person chart stats: %w", err)
	}
	out := make(map[string]TitleChartStats, len(rows))
	for _, row := range rows {
		out[TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID}.String()] = TitleChartStats{Days: row.Days, Points: row.Points, BestRank: row.BestRank,
			Countries: row.Countries, FirstCharted: row.FirstCharted, LastCharted: row.LastCharted}
	}
	return out, nil
}

// TypedCredit is a credit of one type with its title and person.
type TypedCredit struct {
	Ref         TmdbRef
	PersonID    int64
	Name        string
	ProfilePath string
	Job         string
	Character   string
	Order       int64
}

// CreditsOfType lists every cast or crew credit.
func (r *CreditsRepository) CreditsOfType(ctx context.Context, creditType string) ([]TypedCredit, error) {
	rows, err := r.q.ListCreditsOfType(ctx, creditType)
	if err != nil {
		return nil, fmt.Errorf("list credits: %w", err)
	}
	out := make([]TypedCredit, 0, len(rows))
	for _, row := range rows {
		out = append(out, TypedCredit{Ref: TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID}, PersonID: row.PersonID, Name: row.Name,
			ProfilePath: row.ProfilePath, Job: row.Job, Character: row.Character, Order: row.CreditOrder})
	}
	return out, nil
}
