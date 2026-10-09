package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// ImportsRepository stores file imports: Netflix's official Top 10 and the
// IMDb ratings dataset.
type ImportsRepository struct {
	db *sql.DB
	q  *sqlc.Queries
}

func NewImportsRepository(db *sql.DB) *ImportsRepository {
	return &ImportsRepository{db: db, q: sqlc.New(db)}
}

// ImportState is the last imported version of a source file.
type ImportState struct {
	Source     string             `json:"source"`
	Version    client.FileVersion `json:"-"`
	Rows       int64              `json:"rows"`
	ImportedAt time.Time          `json:"imported_at"`
}

// GetImport returns a source's last import, or a zero state if never imported.
func (r *ImportsRepository) GetImport(ctx context.Context, source string) (ImportState, error) {
	row, err := r.q.GetDataImport(ctx, source)
	if IsNoRows(err) {
		return ImportState{Source: source}, nil
	}
	if err != nil {
		return ImportState{}, fmt.Errorf("get data import: %w", err)
	}
	return ImportState{
		Source:     row.Source,
		Version:    client.FileVersion{ETag: row.Etag.String, LastModified: row.LastModified.String},
		Rows:       row.Rows,
		ImportedAt: row.ImportedAt.UTC(),
	}, nil
}

// ListImports returns every source's last import.
func (r *ImportsRepository) ListImports(ctx context.Context) ([]ImportState, error) {
	rows, err := r.q.ListDataImports(ctx)
	if err != nil {
		return nil, fmt.Errorf("list data imports: %w", err)
	}
	out := make([]ImportState, 0, len(rows))
	for _, row := range rows {
		out = append(out, ImportState{Source: row.Source, Rows: row.Rows, ImportedAt: row.ImportedAt.UTC(),
			Version: client.FileVersion{ETag: row.Etag.String, LastModified: row.LastModified.String}})
	}
	return out, nil
}

func (r *ImportsRepository) SaveImport(ctx context.Context, source string, v client.FileVersion, rows int64) error {
	return r.q.SaveDataImport(ctx, sqlc.SaveDataImportParams{
		Source: source, Etag: nullIfEmpty(v.ETag), LastModified: nullIfEmpty(v.LastModified), Rows: rows,
	})
}

// NetflixGlobalRow is one row of all-weeks-global.tsv.
type NetflixGlobalRow struct {
	Week            string
	Category        string
	Rank            int64
	ShowTitle       string
	SeasonTitle     string
	HoursViewed     *int64
	RuntimeHours    *float64
	Views           *int64
	CumulativeWeeks *int64
}

// NetflixCountryRow is one row of all-weeks-countries.tsv.
type NetflixCountryRow struct {
	Country         string
	Week            string
	Category        string
	Rank            int64
	ShowTitle       string
	SeasonTitle     string
	CumulativeWeeks *int64
}

// NetflixMostPopularRow is one row of most-popular.tsv.
type NetflixMostPopularRow struct {
	Category     string
	Rank         int64
	ShowTitle    string
	SeasonTitle  string
	HoursViewed  *int64
	RuntimeHours *float64
	Views        *int64
}

// NetflixKind maps a Netflix category ("Films (English)", "TV", …) to a title kind.
func NetflixKind(category string) string {
	if strings.HasPrefix(strings.ToLower(category), "tv") {
		return "tv_show"
	}
	return "movie"
}

// netflixTitleCache resolves show titles to netflix_titles IDs inside a tx.
type netflixTitleCache struct {
	q   *sqlc.Queries
	ids map[string]int64
}

func (c *netflixTitleCache) id(ctx context.Context, showTitle, kind string) (int64, error) {
	key := kind + "\x00" + showTitle
	if id, ok := c.ids[key]; ok {
		return id, nil
	}
	t, err := c.q.EnsureNetflixTitle(ctx, sqlc.EnsureNetflixTitleParams{ShowTitle: showTitle, Kind: kind})
	if err != nil {
		return 0, fmt.Errorf("ensure netflix title %q: %w", showTitle, err)
	}
	c.ids[key] = t.ID
	return t.ID, nil
}

// SaveNetflixGlobal upserts global weekly rows in one transaction.
func (r *ImportsRepository) SaveNetflixGlobal(ctx context.Context, rows []NetflixGlobalRow) error {
	return r.inTx(ctx, func(q *sqlc.Queries) error {
		titles := &netflixTitleCache{q: q, ids: map[string]int64{}}
		for _, row := range rows {
			id, err := titles.id(ctx, row.ShowTitle, NetflixKind(row.Category))
			if err != nil {
				return err
			}
			if err := q.UpsertNetflixGlobal(ctx, sqlc.UpsertNetflixGlobalParams{
				Week: row.Week, Category: row.Category, WeeklyRank: row.Rank, NetflixTitleID: id,
				ShowTitle: row.ShowTitle, SeasonTitle: nullIfEmpty(row.SeasonTitle),
				WeeklyHoursViewed: int64PtrNull(row.HoursViewed), RuntimeHours: floatPtrNull(row.RuntimeHours),
				WeeklyViews: int64PtrNull(row.Views), CumulativeWeeks: int64PtrNull(row.CumulativeWeeks),
			}); err != nil {
				return fmt.Errorf("upsert netflix global %s %s #%d: %w", row.Week, row.Category, row.Rank, err)
			}
		}
		return nil
	})
}

// SaveNetflixCountries upserts per-country weekly rows in one transaction.
func (r *ImportsRepository) SaveNetflixCountries(ctx context.Context, rows []NetflixCountryRow) error {
	return r.inTx(ctx, func(q *sqlc.Queries) error {
		titles := &netflixTitleCache{q: q, ids: map[string]int64{}}
		for _, row := range rows {
			id, err := titles.id(ctx, row.ShowTitle, NetflixKind(row.Category))
			if err != nil {
				return err
			}
			if err := q.UpsertNetflixCountry(ctx, sqlc.UpsertNetflixCountryParams{
				Country: row.Country, Week: row.Week, Category: row.Category, WeeklyRank: row.Rank, NetflixTitleID: id,
				ShowTitle: row.ShowTitle, SeasonTitle: nullIfEmpty(row.SeasonTitle), CumulativeWeeks: int64PtrNull(row.CumulativeWeeks),
			}); err != nil {
				return fmt.Errorf("upsert netflix country %s %s %s #%d: %w", row.Country, row.Week, row.Category, row.Rank, err)
			}
		}
		return nil
	})
}

// ReplaceNetflixMostPopular replaces the most-popular list.
func (r *ImportsRepository) ReplaceNetflixMostPopular(ctx context.Context, rows []NetflixMostPopularRow) error {
	return r.inTx(ctx, func(q *sqlc.Queries) error {
		if err := q.DeleteNetflixMostPopular(ctx); err != nil {
			return err
		}
		titles := &netflixTitleCache{q: q, ids: map[string]int64{}}
		for _, row := range rows {
			id, err := titles.id(ctx, row.ShowTitle, NetflixKind(row.Category))
			if err != nil {
				return err
			}
			if err := q.InsertNetflixMostPopular(ctx, sqlc.InsertNetflixMostPopularParams{
				Category: row.Category, Rank: row.Rank, NetflixTitleID: id, ShowTitle: row.ShowTitle,
				SeasonTitle: nullIfEmpty(row.SeasonTitle), HoursViewed91d: int64PtrNull(row.HoursViewed),
				RuntimeHours: floatPtrNull(row.RuntimeHours), Views91d: int64PtrNull(row.Views),
			}); err != nil {
				return fmt.Errorf("insert netflix most popular: %w", err)
			}
		}
		return nil
	})
}

// NetflixTitleDue is a Netflix title to match and the first week it charted.
type NetflixTitleDue struct {
	sqlc.NetflixTitle
	FirstWeek string // YYYY-MM-DD or ""
}

// NetflixTitlesDueForMatch lists Netflix titles to match (backoff expired).
func (r *ImportsRepository) NetflixTitlesDueForMatch(ctx context.Context, limit int64) ([]NetflixTitleDue, error) {
	rows, err := r.q.ListNetflixTitlesDueForMatch(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list netflix titles due for match: %w", err)
	}
	out := make([]NetflixTitleDue, 0, len(rows))
	for _, row := range rows {
		out = append(out, NetflixTitleDue{NetflixTitle: row.NetflixTitle, FirstWeek: row.FirstWeek})
	}
	return out, nil
}

// NetflixMatch is one matching attempt of a Netflix title.
type NetflixMatch struct {
	TmdbID     string // empty = not matched
	ImdbID     string
	TitleID    int64 // FlixPatrol title it matched through
	Source     string
	RetryAfter string // SQLite modifier, when unmatched
}

func (r *ImportsRepository) SaveNetflixTitleMatch(ctx context.Context, id int64, m NetflixMatch) error {
	status, retry := MatchStatusUnmatched, m.RetryAfter
	if m.TmdbID != "" {
		status, retry = MatchStatusMatched, ""
	}
	return r.q.SaveNetflixTitleMatch(ctx, sqlc.SaveNetflixTitleMatchParams{
		TmdbID: nullIfEmpty(m.TmdbID), ImdbID: nullIfEmpty(m.ImdbID),
		TitleID: sql.NullInt64{Int64: m.TitleID, Valid: m.TitleID > 0}, MatchStatus: status,
		MatchSource: nullIfEmpty(m.Source), RetryAfter: retry, ID: id,
	})
}

// FlixPatrolTitlesNamed returns matched FlixPatrol titles of a kind named
// name (or whose title-page name is name) that charted on Netflix.
func (r *ImportsRepository) FlixPatrolTitlesNamed(ctx context.Context, name, kind string) ([]sqlc.Title, error) {
	return r.q.FindFlixPatrolTitlesByName(ctx, sqlc.FindFlixPatrolTitlesByNameParams{Kind: kind, Name: name})
}

func (r *ImportsRepository) CountNetflixTitlesByMatchStatus(ctx context.Context) (map[string]int64, error) {
	rows, err := r.q.CountNetflixTitlesByMatchStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("count netflix titles: %w", err)
	}
	out := map[string]int64{}
	for _, row := range rows {
		out[row.MatchStatus] = row.Titles
	}
	return out, nil
}

// KnownIMDbIDs returns every IMDb ID referenced anywhere in the database.
func (r *ImportsRepository) KnownIMDbIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := r.q.ListKnownIMDbIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list known imdb ids: %w", err)
	}
	out := make(map[string]bool, len(rows))
	for _, id := range rows {
		if id.Valid {
			out[id.String] = true
		}
	}
	return out, nil
}

// IMDbRating is one row of the IMDb ratings dataset.
type IMDbRating struct {
	IMDbID     string    `json:"imdb_id"`
	Rating     float64   `json:"rating"`
	Votes      int64     `json:"votes"`
	ImportedAt time.Time `json:"imported_at"`
}

// SaveIMDbRatings upserts ratings in one transaction.
func (r *ImportsRepository) SaveIMDbRatings(ctx context.Context, rows []IMDbRating) error {
	return r.inTx(ctx, func(q *sqlc.Queries) error {
		for _, row := range rows {
			if err := q.UpsertIMDbRating(ctx, sqlc.UpsertIMDbRatingParams{ImdbID: row.IMDbID, AverageRating: row.Rating, NumVotes: row.Votes}); err != nil {
				return fmt.Errorf("upsert imdb rating %s: %w", row.IMDbID, err)
			}
		}
		return nil
	})
}

// IMDbRatings returns stored dataset ratings keyed by IMDb ID.
func (r *ImportsRepository) IMDbRatings(ctx context.Context, ids []string) (map[string]IMDbRating, error) {
	out := map[string]IMDbRating{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.q.ListIMDbRatings(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list imdb ratings: %w", err)
	}
	for _, row := range rows {
		out[row.ImdbID] = IMDbRating{IMDbID: row.ImdbID, Rating: row.AverageRating, Votes: row.NumVotes, ImportedAt: row.ImportedAt.UTC()}
	}
	return out, nil
}

func (r *ImportsRepository) CountIMDbRatings(ctx context.Context) (int64, error) {
	return r.q.CountIMDbRatings(ctx)
}

func (r *ImportsRepository) inTx(ctx context.Context, fn func(q *sqlc.Queries) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(r.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func int64PtrNull(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

func floatPtrNull(p *float64) sql.NullFloat64 {
	if p == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *p, Valid: true}
}
