package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// Where a season number came from (netflix_seasons.season_source and
// rankings.season_source).
const (
	SeasonSourceParsed     = "parsed"        // "Season 2", "S3", "Limited Series" in Netflix's season title
	SeasonSourceTmdbName   = "tmdb_name"     // the season title names a TMDB season ("Hashira Training Arc")
	SeasonSourceNumber     = "number"        // "Part 2", "Volume 4", "Stranger Things 5"
	SeasonSourceTmdbAir    = "tmdb_air_date" // the latest TMDB season aired by the time it charted
	SeasonSourceTmdbSingle = "tmdb_single"   // the TMDB show has a single season
	SeasonSourceManual     = "manual"        // fixed by hand in the admin UI
	SeasonSourceChart      = "chart"         // the FlixPatrol chart name said so ("Wednesday: Season 2")
	SeasonSourceNetflix    = "netflix"       // Netflix's official Top 10 of that week
)

// NetflixTitleFilter filters and sorts the admin list of Netflix titles.
type NetflixTitleFilter struct {
	Kind        string // "" | movie | tv_show
	Search      string // show title substring or exact TMDB ID
	MatchStatus string
	Missing     string // "" | tmdb | season
	Flag        string // "" | cross_kind | linked
	Sort        string
}

// NetflixTitleRow is a Netflix title with its chart stats and the names of
// what it is mapped to.
type NetflixTitleRow struct {
	sqlc.NetflixTitle
	LastWeek        string // YYYY-MM-DD or ""
	WeeksGlobal     int64
	BestGlobalRank  *int64
	SeasonsTotal    int64
	SeasonsUnmapped int64
	FlixPatrolLinks int64
	TmdbTitle       *string
	TmdbReleaseDate *string
	IMDbID          *string // the TMDB title's
	FlixPatrolSlug  *string
	FlixPatrolName  *string
}

func (r *ImportsRepository) ListNetflixTitles(ctx context.Context, f NetflixTitleFilter, limit, offset int64) ([]NetflixTitleRow, error) {
	rows, err := r.q.ListNetflixTitles(ctx, sqlc.ListNetflixTitlesParams{
		Sort: f.Sort, Kind: f.Kind, Name: f.Search, MatchStatus: f.MatchStatus, Missing: f.Missing, Flag: f.Flag, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list netflix titles: %w", err)
	}
	out := make([]NetflixTitleRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, NetflixTitleRow{
			NetflixTitle: row.NetflixTitle, LastWeek: row.LastWeek, WeeksGlobal: row.WeeksGlobal, BestGlobalRank: rankPtr(row.BestGlobalRank),
			SeasonsTotal: row.SeasonsTotal, SeasonsUnmapped: row.SeasonsUnmapped, FlixPatrolLinks: row.FlixpatrolLinks,
			TmdbTitle: nullStringPtr(row.TmdbTitle), TmdbReleaseDate: nullStringPtr(row.TmdbReleaseDate), IMDbID: nullStringPtr(row.TmdbImdbID),
			FlixPatrolSlug: emptyNil(row.FlixpatrolSlug), FlixPatrolName: emptyNil(row.FlixpatrolName),
		})
	}
	return out, nil
}

func (r *ImportsRepository) CountNetflixTitles(ctx context.Context, f NetflixTitleFilter) (int64, error) {
	n, err := r.q.CountNetflixTitles(ctx, sqlc.CountNetflixTitlesParams{Kind: f.Kind, Name: f.Search, MatchStatus: f.MatchStatus, Missing: f.Missing, Flag: f.Flag})
	if err != nil {
		return 0, fmt.Errorf("count netflix titles: %w", err)
	}
	return n, nil
}

// GetNetflixTitle returns one title with its stats; sql.ErrNoRows if missing.
func (r *ImportsRepository) GetNetflixTitle(ctx context.Context, id int64) (*NetflixTitleRow, error) {
	row, err := r.q.GetNetflixTitle(ctx, id)
	if err != nil {
		return nil, err
	}
	return &NetflixTitleRow{
		NetflixTitle: row.NetflixTitle, LastWeek: row.LastWeek, WeeksGlobal: row.WeeksGlobal, BestGlobalRank: rankPtr(row.BestGlobalRank),
		SeasonsTotal: row.SeasonsTotal, SeasonsUnmapped: row.SeasonsUnmapped, FlixPatrolLinks: row.FlixpatrolLinks,
		TmdbTitle: nullStringPtr(row.TmdbTitle), TmdbReleaseDate: nullStringPtr(row.TmdbReleaseDate), IMDbID: nullStringPtr(row.TmdbImdbID),
		FlixPatrolSlug: emptyNil(row.FlixpatrolSlug), FlixPatrolName: emptyNil(row.FlixpatrolName),
	}, nil
}

// NetflixTitlesForMatch returns these titles (with their first week) to match
// now, whatever their backoff.
func (r *ImportsRepository) NetflixTitlesForMatch(ctx context.Context, ids []int64) ([]NetflixTitleDue, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.q.ListNetflixTitlesForMatch(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list netflix titles for match: %w", err)
	}
	out := make([]NetflixTitleDue, 0, len(rows))
	for _, row := range rows {
		out = append(out, NetflixTitleDue{NetflixTitle: row.NetflixTitle, FirstWeek: row.FirstWeek})
	}
	return out, nil
}

// --- seasons ------------------------------------------------------------------

// EnsureNetflixSeasons adds a netflix_seasons row for every new season title.
func (r *ImportsRepository) EnsureNetflixSeasons(ctx context.Context) error {
	if err := r.q.EnsureNetflixSeasons(ctx); err != nil {
		return fmt.Errorf("ensure netflix seasons: %w", err)
	}
	return nil
}

// NetflixSeasonToMap is a season (not fixed by hand) and what mapping needs.
type NetflixSeasonToMap struct {
	ID             int64
	NetflixTitleID int64
	SeasonTitle    string
	SeasonNumber   *int64
	SeasonSource   *string
	ShowTitle      string
	TmdbKind       string // movie | tv | "" (unmapped)
	TmdbID         string
	FirstWeek      string       // YYYY-MM-DD or ""
	TmdbSeasons    []TmdbSeason // nil = not fetched (or not matched)
}

// NetflixSeasonsToMap lists seasons not fixed by hand, of one title or all
// (netflixTitleID 0).
func (r *ImportsRepository) NetflixSeasonsToMap(ctx context.Context, netflixTitleID int64) ([]NetflixSeasonToMap, error) {
	rows, err := r.q.ListNetflixSeasonsForMapping(ctx, netflixTitleID)
	if err != nil {
		return nil, fmt.Errorf("list netflix seasons for mapping: %w", err)
	}
	out := make([]NetflixSeasonToMap, 0, len(rows))
	for _, row := range rows {
		out = append(out, NetflixSeasonToMap{
			ID: row.ID, NetflixTitleID: row.NetflixTitleID, SeasonTitle: row.SeasonTitle,
			SeasonNumber: nullInt64Ptr(row.SeasonNumber), SeasonSource: nullStringPtr(row.SeasonSource),
			ShowTitle: row.ShowTitle, TmdbKind: row.TmdbKind.String, TmdbID: row.TmdbID.String, FirstWeek: row.FirstWeek, TmdbSeasons: DecodeTmdbSeasons(row.TmdbSeasons),
		})
	}
	return out, nil
}

// SeasonUpdate sets a season number (nil = unknown) and its source.
type SeasonUpdate struct {
	ID     int64
	Number *int64
	Source string
}

// SaveNetflixSeasonMappings stores automatic season mappings in one
// transaction; seasons fixed by hand are left alone.
func (r *ImportsRepository) SaveNetflixSeasonMappings(ctx context.Context, updates []SeasonUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	return r.inTx(ctx, func(q *sqlc.Queries) error {
		for _, u := range updates {
			if err := q.UpdateNetflixSeasonMapping(ctx, sqlc.UpdateNetflixSeasonMappingParams{
				ID: u.ID, SeasonNumber: int64PtrNull(u.Number), SeasonSource: nullIfEmpty(u.Source),
			}); err != nil {
				return fmt.Errorf("update netflix season %d: %w", u.ID, err)
			}
		}
		return nil
	})
}

// GetNetflixSeason returns one season; sql.ErrNoRows if missing.
func (r *ImportsRepository) GetNetflixSeason(ctx context.Context, id int64) (sqlc.NetflixSeason, error) {
	return r.q.GetNetflixSeason(ctx, id)
}

// SetNetflixSeasonManual fixes a season's number by hand.
func (r *ImportsRepository) SetNetflixSeasonManual(ctx context.Context, id, number int64) (sqlc.NetflixSeason, error) {
	return r.q.SetNetflixSeasonManual(ctx, sqlc.SetNetflixSeasonManualParams{ID: id, SeasonNumber: sql.NullInt64{Int64: number, Valid: true}})
}

// ResetNetflixSeason hands a season back to automatic mapping.
func (r *ImportsRepository) ResetNetflixSeason(ctx context.Context, id int64) (sqlc.NetflixSeason, error) {
	return r.q.ResetNetflixSeason(ctx, id)
}

// NetflixSeasonStats is a season of a Netflix title with its chart stats.
type NetflixSeasonStats struct {
	ID             int64   `json:"id"`
	SeasonTitle    string  `json:"season_title"`
	SeasonNumber   *int64  `json:"season_number"`
	SeasonSource   *string `json:"season_source"`
	WeeksGlobal    int64   `json:"weeks_in_global_top10"`
	BestGlobalRank *int64  `json:"best_global_rank"`
	TotalViews     int64   `json:"total_views"`
	TotalHours     int64   `json:"total_hours_viewed"`
	FirstWeek      *string `json:"first_week"`
	LastWeek       *string `json:"last_week"`
	Countries      int64   `json:"countries"`
	NetflixID      *string `json:"netflix_id"` // the season's own Netflix ID
}

func (r *ImportsRepository) NetflixSeasonsForTitle(ctx context.Context, netflixTitleID int64) ([]NetflixSeasonStats, error) {
	rows, err := r.q.ListNetflixSeasonsForTitle(ctx, netflixTitleID)
	if err != nil {
		return nil, fmt.Errorf("list netflix seasons for title: %w", err)
	}
	out := make([]NetflixSeasonStats, 0, len(rows))
	for _, row := range rows {
		out = append(out, NetflixSeasonStats{
			ID: row.ID, SeasonTitle: row.SeasonTitle, SeasonNumber: nullInt64Ptr(row.SeasonNumber), SeasonSource: nullStringPtr(row.SeasonSource),
			WeeksGlobal: row.WeeksGlobal, BestGlobalRank: rankPtr(row.BestGlobalRank), TotalViews: row.TotalViews, TotalHours: row.TotalHours,
			FirstWeek: emptyNil(row.FirstWeek), LastWeek: emptyNil(row.LastWeek), Countries: row.Countries,
			NetflixID: nullStringPtr(row.NetflixID),
		})
	}
	return out, nil
}

// NetflixTitleWeek is one global Top 10 week of a Netflix title.
type NetflixTitleWeek struct {
	Week            string  `json:"week"`
	Category        string  `json:"category"`
	Rank            int64   `json:"rank"`
	SeasonTitle     *string `json:"season_title"`
	SeasonNumber    *int64  `json:"season_number"`
	Views           *int64  `json:"views"`
	HoursViewed     *int64  `json:"hours_viewed"`
	CumulativeWeeks *int64  `json:"cumulative_weeks"`
}

// NetflixTitleCountry summarises a Netflix title's weeks in one country.
type NetflixTitleCountry struct {
	Country   string `json:"country"`
	Weeks     int64  `json:"weeks"`
	BestRank  int64  `json:"best_rank"`
	FirstWeek string `json:"first_week"`
	LastWeek  string `json:"last_week"`
}

func (r *ImportsRepository) NetflixWeeksForTitle(ctx context.Context, netflixTitleID int64) ([]NetflixTitleWeek, []NetflixTitleCountry, error) {
	g, err := r.q.ListNetflixGlobalWeeksForTitle(ctx, netflixTitleID)
	if err != nil {
		return nil, nil, fmt.Errorf("list netflix global weeks for title: %w", err)
	}
	c, err := r.q.ListNetflixCountryWeeksForTitle(ctx, netflixTitleID)
	if err != nil {
		return nil, nil, fmt.Errorf("list netflix country weeks for title: %w", err)
	}
	weeks := make([]NetflixTitleWeek, 0, len(g))
	for _, row := range g {
		weeks = append(weeks, NetflixTitleWeek{Week: row.Week, Category: row.Category, Rank: row.WeeklyRank,
			SeasonTitle: nullStringPtr(row.SeasonTitle), SeasonNumber: nullInt64Ptr(row.SeasonNumber),
			Views: nullInt64Ptr(row.WeeklyViews), HoursViewed: nullInt64Ptr(row.WeeklyHoursViewed), CumulativeWeeks: nullInt64Ptr(row.CumulativeWeeks)})
	}
	countries := make([]NetflixTitleCountry, 0, len(c))
	for _, row := range c {
		countries = append(countries, NetflixTitleCountry{Country: row.Country, Weeks: row.Weeks, BestRank: row.BestRank,
			FirstWeek: row.FirstWeek, LastWeek: row.LastWeek})
	}
	return weeks, countries, nil
}

// CountNetflixSeasonsBySource counts seasons per season_source ("unmapped"
// for none).
func (r *ImportsRepository) CountNetflixSeasonsBySource(ctx context.Context) (map[string]int64, error) {
	rows, err := r.q.CountNetflixSeasonsBySource(ctx)
	if err != nil {
		return nil, fmt.Errorf("count netflix seasons: %w", err)
	}
	out := map[string]int64{}
	for _, row := range rows {
		out[row.Source] = row.Seasons
	}
	return out, nil
}

// --- season numbers of FlixPatrol TV rankings ------------------------------------

// TVRankingSeason is a TV ranking whose season was not given by its chart.
type TVRankingSeason struct {
	ID           int64
	Date         string // YYYY-MM-DD
	Country      string
	Provider     string
	TmdbID       string // "" when the title is not matched
	SeasonNumber *int64
	SeasonSource string
}

func (r *ImportsRepository) TVRankingsForSeasons(ctx context.Context, from time.Time) ([]TVRankingSeason, error) {
	rows, err := r.q.ListTVRankingsForSeasons(ctx, FormatDate(from))
	if err != nil {
		return nil, fmt.Errorf("list tv rankings for seasons: %w", err)
	}
	out := make([]TVRankingSeason, 0, len(rows))
	for _, row := range rows {
		out = append(out, TVRankingSeason{ID: row.ID, Date: dateText(row.RankedOn), Country: row.Country, Provider: row.StreamingProvider,
			TmdbID: row.TmdbID.String, SeasonNumber: nullInt64Ptr(row.SeasonNumber), SeasonSource: row.SeasonSource.String})
	}
	return out, nil
}

// SaveRankingSeasons stores inferred season numbers in one transaction.
func (r *ImportsRepository) SaveRankingSeasons(ctx context.Context, updates []SeasonUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	return r.inTx(ctx, func(q *sqlc.Queries) error {
		for _, u := range updates {
			if err := q.UpdateRankingSeason(ctx, sqlc.UpdateRankingSeasonParams{
				ID: u.ID, SeasonNumber: int64PtrNull(u.Number), SeasonSource: nullIfEmpty(u.Source),
			}); err != nil {
				return fmt.Errorf("update ranking %d season: %w", u.ID, err)
			}
		}
		return nil
	})
}

// NetflixSeasonRank is a mapped season of a TV show in a Netflix weekly Top
// 10 (Country "" = global).
type NetflixSeasonRank struct {
	TmdbID  string
	Country string
	Week    string
	Rank    int64
	Season  int64
}

// NetflixSeasonRanks returns mapped seasons in the global and per-country
// weekly Top 10 from fromWeek, plus the (country, week) pairs imported.
func (r *ImportsRepository) NetflixSeasonRanks(ctx context.Context, fromWeek string) ([]NetflixSeasonRank, map[string]bool, error) {
	g, err := r.q.ListNetflixGlobalSeasonRanks(ctx, fromWeek)
	if err != nil {
		return nil, nil, fmt.Errorf("list netflix global season ranks: %w", err)
	}
	c, err := r.q.ListNetflixCountrySeasonRanks(ctx, fromWeek)
	if err != nil {
		return nil, nil, fmt.Errorf("list netflix country season ranks: %w", err)
	}
	weeks, err := r.q.ListNetflixCountryWeeksImported(ctx, fromWeek)
	if err != nil {
		return nil, nil, fmt.Errorf("list netflix country weeks: %w", err)
	}
	out := make([]NetflixSeasonRank, 0, len(g)+len(c))
	for _, row := range g {
		out = append(out, NetflixSeasonRank{TmdbID: row.TmdbID.String, Week: row.Week, Rank: row.WeeklyRank, Season: row.SeasonNumber.Int64})
	}
	for _, row := range c {
		out = append(out, NetflixSeasonRank{TmdbID: row.TmdbID.String, Country: row.Country, Week: row.Week, Rank: row.WeeklyRank, Season: row.SeasonNumber.Int64})
	}
	imported := make(map[string]bool, len(weeks))
	for _, w := range weeks {
		imported[w.Country+"|"+w.Week] = true
	}
	return out, imported, nil
}

// TmdbTVSeasons returns the stored season lists of TV shows by TMDB ID.
func (r *ImportsRepository) TmdbTVSeasons(ctx context.Context) (map[string][]TmdbSeason, error) {
	rows, err := r.q.ListTmdbTVSeasons(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tmdb tv seasons: %w", err)
	}
	out := make(map[string][]TmdbSeason, len(rows))
	for _, row := range rows {
		out[row.TmdbID] = DecodeTmdbSeasons(row.Seasons)
	}
	return out, nil
}

func emptyNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// --- Netflix IDs --------------------------------------------------------------

// NetflixIDCandidate is a Netflix title still without its Netflix ID.
type NetflixIDCandidate struct {
	ID          int64
	ShowTitle   string
	Kind        string // Netflix's list: movie | tv_show
	MatchStatus string
	MatchSource string
	TmdbKind    string
	TmdbID      string
	NetflixID   string // known already (looked up before)
}

// NetflixTitlesNeedingNetflixID returns these titles (ids, with their
// Netflix ID when known), or up to limit titles without a Netflix ID not
// looked up within window (newest charting first).
func (r *ImportsRepository) NetflixTitlesNeedingNetflixID(ctx context.Context, ids []int64, window string, limit int64) ([]NetflixIDCandidate, error) {
	var out []NetflixIDCandidate
	add := func(id int64, show, kind, status string, source, tmdbKind, tmdbID sql.NullString, netflixID string) {
		out = append(out, NetflixIDCandidate{ID: id, ShowTitle: show, Kind: kind, MatchStatus: status,
			MatchSource: source.String, TmdbKind: tmdbKind.String, TmdbID: tmdbID.String, NetflixID: netflixID})
	}
	if len(ids) > 0 {
		rows, err := r.q.ListNetflixTitlesForNetflixIDByIDs(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("list netflix titles for netflix id: %w", err)
		}
		for _, row := range rows {
			add(row.ID, row.ShowTitle, row.Kind, row.MatchStatus, row.MatchSource, row.TmdbKind, row.TmdbID, row.NetflixID)
		}
		return out, nil
	}
	rows, err := r.q.ListNetflixTitlesForNetflixID(ctx, sqlc.ListNetflixTitlesForNetflixIDParams{Window: window, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("list netflix titles for netflix id: %w", err)
	}
	for _, row := range rows {
		add(row.ID, row.ShowTitle, row.Kind, row.MatchStatus, row.MatchSource, row.TmdbKind, row.TmdbID, row.NetflixID)
	}
	return out, nil
}

// NetflixTitleWeeks returns every week (YYYY-MM-DD) a title charted.
func (r *ImportsRepository) NetflixTitleWeeks(ctx context.Context, id int64) ([]string, error) {
	weeks, err := r.q.ListNetflixTitleWeeks(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list netflix title weeks: %w", err)
	}
	return weeks, nil
}

// NetflixSeasonTitle is a season title row of a Netflix title.
type NetflixSeasonTitle struct {
	ID          int64
	SeasonTitle string
}

func (r *ImportsRepository) NetflixSeasonTitles(ctx context.Context, netflixTitleID int64) ([]NetflixSeasonTitle, error) {
	rows, err := r.q.ListNetflixSeasonTitles(ctx, netflixTitleID)
	if err != nil {
		return nil, fmt.Errorf("list netflix season titles: %w", err)
	}
	out := make([]NetflixSeasonTitle, 0, len(rows))
	for _, s := range rows {
		out = append(out, NetflixSeasonTitle{ID: s.ID, SeasonTitle: s.SeasonTitle})
	}
	return out, nil
}

// SetNetflixTitleNetflixID records a title's Netflix ID ("" = looked up,
// not found) and when it was looked up.
func (r *ImportsRepository) SetNetflixTitleNetflixID(ctx context.Context, id int64, netflixID string) error {
	if err := r.q.SetNetflixTitleNetflixID(ctx, sqlc.SetNetflixTitleNetflixIDParams{ID: id, NetflixID: nullIfEmpty(netflixID)}); err != nil {
		return fmt.Errorf("set netflix id of netflix title %d: %w", id, err)
	}
	return nil
}

func (r *ImportsRepository) SetNetflixSeasonNetflixID(ctx context.Context, seasonID int64, netflixID string) error {
	if err := r.q.SetNetflixSeasonNetflixID(ctx, sqlc.SetNetflixSeasonNetflixIDParams{ID: seasonID, NetflixID: nullIfEmpty(netflixID)}); err != nil {
		return fmt.Errorf("set netflix id of season %d: %w", seasonID, err)
	}
	return nil
}
