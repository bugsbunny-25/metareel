package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// AnalyticsRepository reads charts and Netflix data for the read API and
// analytics; aggregation happens in the service layer.
type AnalyticsRepository struct {
	q *sqlc.Queries
}

func NewAnalyticsRepository(db *sql.DB) *AnalyticsRepository {
	return &AnalyticsRepository{q: sqlc.New(db)}
}

// ChartEntryRow is one ranking with its title.
type ChartEntryRow struct {
	Date         string // YYYY-MM-DD
	Country      string
	Provider     string
	Category     string // movies | tv_shows
	Rank         int64
	SeasonNumber *int64
	TitleID      int64
	Slug         string
	Name         string
	Kind         string // movie | tv_show
	TmdbID       *string
	ImdbID       *string
}

// ChartEntryFilter narrows ChartEntries; empty strings match all.
type ChartEntryFilter struct {
	From, To                    time.Time
	Country, Provider, Category string
}

func (r *AnalyticsRepository) ChartEntries(ctx context.Context, f ChartEntryFilter) ([]ChartEntryRow, error) {
	rows, err := r.q.ListChartEntries(ctx, sqlc.ListChartEntriesParams{
		FromDate: FormatDate(f.From), ToDate: FormatDate(f.To), Country: f.Country, Provider: f.Provider, Category: f.Category,
	})
	if err != nil {
		return nil, fmt.Errorf("list chart entries: %w", err)
	}
	out := make([]ChartEntryRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, ChartEntryRow{
			Date: dateText(row.RankedOn), Country: row.Country, Provider: row.StreamingProvider, Category: row.Category,
			Rank: row.Rank, SeasonNumber: nullInt64Ptr(row.SeasonNumber), TitleID: row.TitleID, Slug: row.Slug,
			Name: row.Name, Kind: row.Kind, TmdbID: nullStringPtr(row.TmdbID), ImdbID: nullStringPtr(row.ImdbID),
		})
	}
	return out, nil
}

// ChartCatalogRow describes one stored chart.
type ChartCatalogRow struct {
	Country       string
	Provider      string
	Category      string
	FirstDate     string
	LatestDate    string
	Dates         int64
	LastScrapedAt *time.Time
}

func (r *AnalyticsRepository) ChartCatalog(ctx context.Context) ([]ChartCatalogRow, error) {
	rows, err := r.q.ListChartCatalog(ctx)
	if err != nil {
		return nil, fmt.Errorf("list chart catalog: %w", err)
	}
	out := make([]ChartCatalogRow, 0, len(rows))
	for _, row := range rows {
		c := ChartCatalogRow{Country: row.Country, Provider: row.StreamingProvider, Category: row.Category,
			FirstDate: row.FirstDate, LatestDate: row.LatestDate, Dates: row.Dates}
		if t, err := time.Parse("2006-01-02 15:04:05", row.LastScrapedAt); err == nil {
			c.LastScrapedAt = &t
		}
		out = append(out, c)
	}
	return out, nil
}

// ChartDate is one stored chart date.
type ChartDate struct {
	Date       string    `json:"date"`
	Category   string    `json:"category"`
	EntryCount int64     `json:"entries"`
	ScrapedAt  time.Time `json:"scraped_at"`
}

func (r *AnalyticsRepository) ChartDates(ctx context.Context, country, provider, category string, limit int64) ([]ChartDate, error) {
	rows, err := r.q.ListChartDates(ctx, sqlc.ListChartDatesParams{Country: country, Provider: provider, Category: category, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("list chart dates: %w", err)
	}
	out := make([]ChartDate, 0, len(rows))
	for _, row := range rows {
		out = append(out, ChartDate{Date: row.RankedOn, Category: row.Category, EntryCount: row.EntryCount, ScrapedAt: row.ScrapedAt.UTC()})
	}
	return out, nil
}

// ChartChange is a chart stored or re-scraped.
type ChartChange struct {
	ID        int64     `json:"id"`
	Date      string    `json:"date"`
	Country   string    `json:"country"`
	Provider  string    `json:"provider"`
	Category  string    `json:"category"`
	Entries   int64     `json:"entries"`
	ScrapedAt time.Time `json:"scraped_at"`
	ChangedAt time.Time `json:"changed_at"`
}

func (r *AnalyticsRepository) ChartChangesSince(ctx context.Context, since time.Time, limit int64) ([]ChartChange, error) {
	rows, err := r.q.ListChartChangesSince(ctx, sqlc.ListChartChangesSinceParams{Since: since.UTC().Format("2006-01-02 15:04:05"), Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("list chart changes: %w", err)
	}
	out := make([]ChartChange, 0, len(rows))
	for _, row := range rows {
		out = append(out, ChartChange{ID: row.ID, Date: row.RankedOn, Country: row.Country, Provider: row.StreamingProvider,
			Category: row.Category, Entries: row.EntryCount, ScrapedAt: row.ScrapedAt.UTC(), ChangedAt: row.ChangedAt.UTC()})
	}
	return out, nil
}

// TitlesByTmdb returns FlixPatrol titles mapped to a TMDB title (kind is
// movie | tv_show).
func (r *AnalyticsRepository) TitlesByTmdb(ctx context.Context, kind, tmdbID string) ([]sqlc.Title, error) {
	return r.q.ListTitlesByTmdbRef(ctx, sqlc.ListTitlesByTmdbRefParams{Kind: kind, TmdbID: sql.NullString{String: tmdbID, Valid: true}})
}

func (r *AnalyticsRepository) TitlesByIMDb(ctx context.Context, imdbID string) ([]sqlc.Title, error) {
	return r.q.FindTitlesByIMDb(ctx, sql.NullString{String: imdbID, Valid: true})
}

func (r *AnalyticsRepository) TmdbRefsByIMDb(ctx context.Context, imdbID string) ([]TmdbRef, error) {
	rows, err := r.q.FindTmdbRefsByIMDb(ctx, sql.NullString{String: imdbID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]TmdbRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, TmdbRef{Kind: row.TmdbKind, ID: row.TmdbID})
	}
	return out, nil
}

// NetflixWeekRow is a row of Netflix's weekly Top 10 (global or a country).
type NetflixWeekRow struct {
	Week            string   `json:"week"`
	Country         string   `json:"country,omitempty"`
	Category        string   `json:"category"`
	Rank            int64    `json:"rank"`
	ShowTitle       string   `json:"show_title"`
	SeasonTitle     *string  `json:"season_title"`
	HoursViewed     *int64   `json:"hours_viewed,omitempty"`
	RuntimeHours    *float64 `json:"runtime_hours,omitempty"`
	Views           *int64   `json:"views,omitempty"`
	CumulativeWeeks *int64   `json:"cumulative_weeks"`
	Kind            string   `json:"kind,omitempty"`
	TmdbID          *string  `json:"tmdb_id,omitempty"`
	IMDbID          *string  `json:"imdb_id,omitempty"`
}

func (r *AnalyticsRepository) NetflixGlobalWeek(ctx context.Context, week, category string) ([]NetflixWeekRow, error) {
	rows, err := r.q.ListNetflixGlobalWeek(ctx, sqlc.ListNetflixGlobalWeekParams{Week: week, Category: category})
	if err != nil {
		return nil, fmt.Errorf("list netflix global week: %w", err)
	}
	out := make([]NetflixWeekRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, NetflixWeekRow{Week: row.Week, Category: row.Category, Rank: row.WeeklyRank, ShowTitle: row.ShowTitle,
			SeasonTitle: nullStringPtr(row.SeasonTitle), HoursViewed: nullInt64Ptr(row.WeeklyHoursViewed), RuntimeHours: nullFloatPtr(row.RuntimeHours),
			Views: nullInt64Ptr(row.WeeklyViews), CumulativeWeeks: nullInt64Ptr(row.CumulativeWeeks), Kind: row.Kind,
			TmdbID: nullStringPtr(row.TmdbID), IMDbID: nullStringPtr(row.ImdbID)})
	}
	return out, nil
}

func (r *AnalyticsRepository) NetflixCountryWeek(ctx context.Context, country, week, category string) ([]NetflixWeekRow, error) {
	rows, err := r.q.ListNetflixCountryWeek(ctx, sqlc.ListNetflixCountryWeekParams{Country: country, Week: week, Category: category})
	if err != nil {
		return nil, fmt.Errorf("list netflix country week: %w", err)
	}
	out := make([]NetflixWeekRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, NetflixWeekRow{Week: row.Week, Country: row.Country, Category: row.Category, Rank: row.WeeklyRank,
			ShowTitle: row.ShowTitle, SeasonTitle: nullStringPtr(row.SeasonTitle), CumulativeWeeks: nullInt64Ptr(row.CumulativeWeeks),
			Kind: row.Kind, TmdbID: nullStringPtr(row.TmdbID), IMDbID: nullStringPtr(row.ImdbID)})
	}
	return out, nil
}

// LatestNetflixWeek returns the latest imported week (country "" = global).
func (r *AnalyticsRepository) LatestNetflixWeek(ctx context.Context, country string) (string, error) {
	if country == "" {
		return r.q.LatestNetflixGlobalWeek(ctx)
	}
	return r.q.LatestNetflixCountryWeek(ctx, country)
}

// NetflixPopularRow is a row of Netflix's most-popular list.
type NetflixPopularRow struct {
	Category     string   `json:"category"`
	Rank         int64    `json:"rank"`
	ShowTitle    string   `json:"show_title"`
	SeasonTitle  *string  `json:"season_title"`
	HoursViewed  *int64   `json:"hours_viewed_first_91_days"`
	RuntimeHours *float64 `json:"runtime_hours"`
	Views        *int64   `json:"views_first_91_days"`
	Kind         string   `json:"kind"`
	TmdbID       *string  `json:"tmdb_id"`
	IMDbID       *string  `json:"imdb_id"`
}

func (r *AnalyticsRepository) NetflixMostPopular(ctx context.Context, category string) ([]NetflixPopularRow, error) {
	rows, err := r.q.ListNetflixMostPopular(ctx, category)
	if err != nil {
		return nil, fmt.Errorf("list netflix most popular: %w", err)
	}
	out := make([]NetflixPopularRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, NetflixPopularRow{Category: row.Category, Rank: row.Rank, ShowTitle: row.ShowTitle,
			SeasonTitle: nullStringPtr(row.SeasonTitle), HoursViewed: nullInt64Ptr(row.HoursViewed91d),
			RuntimeHours: nullFloatPtr(row.RuntimeHours), Views: nullInt64Ptr(row.Views91d), Kind: row.Kind,
			TmdbID: nullStringPtr(row.TmdbID), IMDbID: nullStringPtr(row.ImdbID)})
	}
	return out, nil
}

// NetflixForTitle returns a TMDB title's Netflix global and country weeks
// (kind is movie | tv_show).
func (r *AnalyticsRepository) NetflixForTitle(ctx context.Context, kind, tmdbID string) ([]NetflixWeekRow, []NetflixWeekRow, error) {
	id := sql.NullString{String: tmdbID, Valid: true}
	g, err := r.q.ListNetflixGlobalByTmdb(ctx, sqlc.ListNetflixGlobalByTmdbParams{Kind: kind, TmdbID: id})
	if err != nil {
		return nil, nil, fmt.Errorf("list netflix global by tmdb: %w", err)
	}
	c, err := r.q.ListNetflixCountriesByTmdb(ctx, sqlc.ListNetflixCountriesByTmdbParams{Kind: kind, TmdbID: id})
	if err != nil {
		return nil, nil, fmt.Errorf("list netflix countries by tmdb: %w", err)
	}
	global := make([]NetflixWeekRow, 0, len(g))
	for _, row := range g {
		global = append(global, NetflixWeekRow{Week: row.Week, Category: row.Category, Rank: row.WeeklyRank, ShowTitle: row.ShowTitle,
			SeasonTitle: nullStringPtr(row.SeasonTitle), HoursViewed: nullInt64Ptr(row.WeeklyHoursViewed), RuntimeHours: nullFloatPtr(row.RuntimeHours),
			Views: nullInt64Ptr(row.WeeklyViews), CumulativeWeeks: nullInt64Ptr(row.CumulativeWeeks)})
	}
	countries := make([]NetflixWeekRow, 0, len(c))
	for _, row := range c {
		countries = append(countries, NetflixWeekRow{Week: row.Week, Country: row.Country, Category: row.Category, Rank: row.WeeklyRank,
			ShowTitle: row.ShowTitle, SeasonTitle: nullStringPtr(row.SeasonTitle), CumulativeWeeks: nullInt64Ptr(row.CumulativeWeeks)})
	}
	return global, countries, nil
}

// NetflixMatchedWeek is a global weekly row with its TMDB match.
type NetflixMatchedWeek struct {
	Week        string
	Category    string
	Rank        int64
	ShowTitle   string
	HoursViewed *int64
	Views       *int64
	TmdbID      *string
	Kind        string
}

func (r *AnalyticsRepository) NetflixGlobalRange(ctx context.Context, fromWeek, toWeek string) ([]NetflixMatchedWeek, error) {
	rows, err := r.q.ListNetflixGlobalRange(ctx, sqlc.ListNetflixGlobalRangeParams{FromWeek: fromWeek, ToWeek: toWeek})
	if err != nil {
		return nil, fmt.Errorf("list netflix global range: %w", err)
	}
	out := make([]NetflixMatchedWeek, 0, len(rows))
	for _, row := range rows {
		out = append(out, NetflixMatchedWeek{Week: row.Week, Category: row.Category, Rank: row.WeeklyRank, ShowTitle: row.ShowTitle,
			HoursViewed: nullInt64Ptr(row.WeeklyHoursViewed), Views: nullInt64Ptr(row.WeeklyViews), TmdbID: nullStringPtr(row.TmdbID), Kind: row.Kind})
	}
	return out, nil
}
