package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

type FlixPatrolRepository struct {
	q  *sqlc.Queries
	db *sql.DB
}

func NewFlixPatrolRepository(db *sql.DB) *FlixPatrolRepository {
	return &FlixPatrolRepository{q: sqlc.New(db), db: db}
}

// EnsureTitle records a FlixPatrol slug seen on a chart, creating the title
// (with the chart's kind) if new. An existing title keeps its kind and IDs.
func (r *FlixPatrolRepository) EnsureTitle(ctx context.Context, slug, name, kind string) (sqlc.Title, error) {
	if slug == "" {
		return sqlc.Title{}, fmt.Errorf("slug is required")
	}
	if name == "" {
		return sqlc.Title{}, fmt.Errorf("name is required")
	}
	if kind != "movie" && kind != "tv_show" {
		return sqlc.Title{}, fmt.Errorf("invalid kind: %s", kind)
	}
	return r.q.EnsureTitle(ctx, sqlc.EnsureTitleParams{Slug: slug, Name: name, Kind: kind})
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
	// MatchStatus: pending | matched | unmatched | manual
	MatchStatus string
	Sort        string // see ListTitles in db/queries/flixpatrol.sql
}

type TitleListItem struct {
	sqlc.Title
	LastRankedOn  *time.Time
	RankingsCount int64
}

func (r *FlixPatrolRepository) ListTitles(ctx context.Context, f TitleListFilter, limit, offset int64) ([]TitleListItem, error) {
	rows, err := r.q.ListTitles(ctx, sqlc.ListTitlesParams{
		Sort:        f.Sort,
		Kind:        f.Kind,
		Name:        f.Search,
		Missing:     f.Missing,
		MatchStatus: f.MatchStatus,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list titles: %w", err)
	}
	out := make([]TitleListItem, 0, len(rows))
	for _, row := range rows {
		item := TitleListItem{Title: row.Title, RankingsCount: row.RankingsCount}
		item.LastRankedOn = parseStoredDate(row.LastRankedOn)
		out = append(out, item)
	}
	return out, nil
}

func (r *FlixPatrolRepository) CountTitles(ctx context.Context, f TitleListFilter) (int64, error) {
	return r.q.CountTitles(ctx, sqlc.CountTitlesParams{Kind: f.Kind, Name: f.Search, Missing: f.Missing, MatchStatus: f.MatchStatus})
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

// ChartEntry is one row of a chart to store.
type ChartEntry struct {
	TitleID      int64
	Slug         string
	Rank         int64 // 1..10
	SeasonNumber *int64
}

// ChartWrite is one chart (date × country × provider × category).
type ChartWrite struct {
	RankedOn          time.Time
	Country           string // ISO 3166-1 alpha-2
	StreamingProvider string
	Category          string // "movies" | "tv_shows"
	Source            string // e.g. "flixpatrol"
	RunID             int64  // 0 if not part of a task run
	Entries           []ChartEntry
}

// ChartSignature is a chart's content as stored in chart_snapshots.signature:
// "1:slug|2:slug|...", in rank order.
func ChartSignature(entries []ChartEntry) string {
	sorted := append([]ChartEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Rank < sorted[j].Rank })
	parts := make([]string, 0, len(sorted))
	for _, e := range sorted {
		parts = append(parts, strconv.FormatInt(e.Rank, 10)+":"+e.Slug)
	}
	return strings.Join(parts, "|")
}

// SaveChart replaces one chart's rankings and records its snapshot, in one
// transaction, so a chart is never half written and ranks that disappeared
// from a re-scraped chart don't linger.
func (r *FlixPatrolRepository) SaveChart(ctx context.Context, in ChartWrite) error {
	if len(in.Entries) == 0 {
		return fmt.Errorf("chart has no entries")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := r.q.WithTx(tx)

	date := FormatDate(in.RankedOn)
	if err := q.DeleteChartRankings(ctx, sqlc.DeleteChartRankingsParams{
		RankedOn: date, Country: in.Country, StreamingProvider: in.StreamingProvider, Category: in.Category,
	}); err != nil {
		return fmt.Errorf("delete chart rankings: %w", err)
	}
	for _, e := range in.Entries {
		var season sql.NullInt64
		if e.SeasonNumber != nil {
			season = sql.NullInt64{Int64: *e.SeasonNumber, Valid: true}
		}
		if err := q.InsertRanking(ctx, sqlc.InsertRankingParams{
			TitleID: e.TitleID, RankedOn: date, Country: in.Country, StreamingProvider: in.StreamingProvider,
			Category: in.Category, Rank: e.Rank, SeasonNumber: season,
		}); err != nil {
			return fmt.Errorf("insert ranking %s rank %d: %w", e.Slug, e.Rank, err)
		}
	}
	source := in.Source
	if source == "" {
		source = "flixpatrol"
	}
	if err := q.UpsertChartSnapshot(ctx, sqlc.UpsertChartSnapshotParams{
		RankedOn: date, Country: in.Country, StreamingProvider: in.StreamingProvider, Category: in.Category,
		EntryCount: int64(len(in.Entries)), Signature: ChartSignature(in.Entries), Source: source,
		RunID: sql.NullInt64{Int64: in.RunID, Valid: in.RunID > 0},
	}); err != nil {
		return fmt.Errorf("upsert chart snapshot: %w", err)
	}
	return tx.Commit()
}

// ChartSnapshot is the stored state of one chart.
type ChartSnapshot struct {
	RankedOn          time.Time
	Country           string
	StreamingProvider string
	Category          string
	EntryCount        int64
	Signature         string
	ScrapedAt         time.Time
	ChangedAt         time.Time
}

func toChartSnapshot(s sqlc.ChartSnapshot) ChartSnapshot {
	d, _ := time.Parse(DateLayout, s.RankedOn)
	return ChartSnapshot{
		RankedOn: d, Country: s.Country, StreamingProvider: s.StreamingProvider, Category: s.Category,
		EntryCount: s.EntryCount, Signature: s.Signature, ScrapedAt: s.ScrapedAt, ChangedAt: s.ChangedAt,
	}
}

// ChartSnapshotsForDate returns the stored charts (by category) of one
// country × provider on a date.
func (r *FlixPatrolRepository) ChartSnapshotsForDate(ctx context.Context, date time.Time, country, provider string) (map[string]ChartSnapshot, error) {
	rows, err := r.q.ListChartSnapshotsForDate(ctx, sqlc.ListChartSnapshotsForDateParams{
		RankedOn: FormatDate(date), Country: country, StreamingProvider: provider,
	})
	if err != nil {
		return nil, fmt.Errorf("list chart snapshots: %w", err)
	}
	out := make(map[string]ChartSnapshot, len(rows))
	for _, row := range rows {
		out[row.Category] = toChartSnapshot(row)
	}
	return out, nil
}

// PreviousChartSnapshot returns the latest stored chart before date, or nil.
func (r *FlixPatrolRepository) PreviousChartSnapshot(ctx context.Context, date time.Time, country, provider, category string) (*ChartSnapshot, error) {
	row, err := r.q.GetPreviousChartSnapshot(ctx, sqlc.GetPreviousChartSnapshotParams{
		Country: country, StreamingProvider: provider, Category: category, RankedOn: FormatDate(date),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("previous chart snapshot: %w", err)
	}
	snap := toChartSnapshot(row)
	return &snap, nil
}

// StoredChartDates returns the dates in [from, to] on which both categories
// of a country × provider chart are stored.
func (r *FlixPatrolRepository) StoredChartDates(ctx context.Context, country, provider string, from, to time.Time) (map[string]bool, error) {
	rows, err := r.q.ListChartSnapshotDates(ctx, sqlc.ListChartSnapshotDatesParams{
		Country: country, StreamingProvider: provider, FromDate: FormatDate(from), ToDate: FormatDate(to),
	})
	if err != nil {
		return nil, fmt.Errorf("list chart snapshot dates: %w", err)
	}
	out := make(map[string]bool, len(rows))
	for _, d := range rows {
		out[d] = true
	}
	return out, nil
}
