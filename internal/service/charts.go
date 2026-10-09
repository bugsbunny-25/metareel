package service

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// ChartsService serves chart catalogs, ranges, movers, leaderboards, the
// change feed and exports. Points are FlixPatrol-style: rank 1 = 10 points
// … rank 10 = 1 point, per chart per day.
type ChartsService struct {
	repo *repository.AnalyticsRepository
	now  func() time.Time
}

func NewChartsService(repo *repository.AnalyticsRepository) *ChartsService {
	return &ChartsService{repo: repo, now: time.Now}
}

// Points for a rank (rank 1 = 10 … rank 10 = 1).
func Points(rank int64) int64 {
	if rank < 1 || rank > 10 {
		return 0
	}
	return 11 - rank
}

// ChartTitle identifies the title of a chart entry.
type ChartTitle struct {
	TitleID int64   `json:"title_id"`
	Slug    string  `json:"slug"`
	Name    string  `json:"name"`
	Kind    string  `json:"kind"` // movie | tv_show
	TmdbID  *string `json:"tmdb_id"`
	IMDbID  *string `json:"imdb_id"`
}

func chartTitle(e repository.ChartEntryRow) ChartTitle {
	return ChartTitle{TitleID: e.TitleID, Slug: e.Slug, Name: e.Name, Kind: e.Kind, TmdbID: e.TmdbID, IMDbID: e.ImdbID}
}

// --- validation helpers -----------------------------------------------------

func validCountry(raw string, required bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if required {
			return "", &ValidationError{Message: "country is required"}
		}
		return "", nil
	}
	code, err := flixpatrol.GetCountryCodeFromISO(raw)
	if err != nil {
		return "", &ValidationError{Message: fmt.Sprintf("invalid country code: %s", raw)}
	}
	return string(code), nil
}

func validProvider(raw string) (string, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return "", nil
	}
	for _, r := range raw {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return "", &ValidationError{Message: fmt.Sprintf("invalid provider: %s", raw)}
		}
	}
	return raw, nil
}

// validCategory accepts movies | tv_shows | tv-shows ("" = both).
func validCategory(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return "", nil
	case "movies", "movie":
		return "movies", nil
	case "tv_shows", "tv-shows", "tv":
		return "tv_shows", nil
	}
	return "", &ValidationError{Message: "invalid category, expected movies or tv_shows"}
}

func validKind(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return "", nil
	case "movie", "movies":
		return "movie", nil
	case "tv_show", "tv", "tv_shows", "tv-shows":
		return "tv_show", nil
	}
	return "", &ValidationError{Message: "invalid kind, expected movie or tv_show"}
}

// dateRange resolves optional from/to (default: the last defaultDays days up
// to the current chart date) and enforces maxDays.
func (s *ChartsService) dateRange(from, to *time.Time, defaultDays, maxDays int) (time.Time, time.Time, error) {
	end := flixpatrol.EffectiveTop10Date(s.now())
	if to != nil {
		end = *to
	}
	start := end.AddDate(0, 0, -(defaultDays - 1))
	if from != nil {
		start = *from
	}
	if start.After(end) {
		return start, end, &ValidationError{Message: "from must be on or before to"}
	}
	if days := int(end.Sub(start).Hours()/24) + 1; days > maxDays {
		return start, end, &ValidationError{Message: fmt.Sprintf("range too long: at most %d days", maxDays)}
	}
	return start, end, nil
}

// --- catalog ------------------------------------------------------------------

// ChartInfo describes one stored chart.
type ChartInfo struct {
	Country       string     `json:"country"`
	Provider      string     `json:"provider"`
	Category      string     `json:"category"`
	FirstDate     string     `json:"first_date"`
	LatestDate    string     `json:"latest_date"`
	Dates         int64      `json:"dates"`
	LastScrapedAt *time.Time `json:"last_scraped_at"`
	// Stale: the latest stored date is older than the previous chart date.
	Stale bool `json:"stale"`
}

type ChartCatalogResponse struct {
	CurrentChartDate string      `json:"current_chart_date"` // the date FlixPatrol shows now
	Items            []ChartInfo `json:"items"`
}

func (s *ChartsService) Catalog(ctx context.Context, country, provider string) (*ChartCatalogResponse, error) {
	country, err := validCountry(country, false)
	if err != nil {
		return nil, err
	}
	if provider, err = validProvider(provider); err != nil {
		return nil, err
	}
	rows, err := s.repo.ChartCatalog(ctx)
	if err != nil {
		return nil, err
	}
	current := flixpatrol.EffectiveTop10Date(s.now())
	staleBefore := repository.FormatDate(current.AddDate(0, 0, -1))
	out := &ChartCatalogResponse{CurrentChartDate: repository.FormatDate(current), Items: []ChartInfo{}}
	for _, r := range rows {
		if (country != "" && r.Country != country) || (provider != "" && r.Provider != provider) {
			continue
		}
		out.Items = append(out.Items, ChartInfo{
			Country: r.Country, Provider: r.Provider, Category: r.Category, FirstDate: r.FirstDate, LatestDate: r.LatestDate,
			Dates: r.Dates, LastScrapedAt: r.LastScrapedAt, Stale: r.LatestDate < staleBefore,
		})
	}
	return out, nil
}

type ChartDatesResponse struct {
	Country  string                 `json:"country"`
	Provider string                 `json:"provider"`
	Items    []repository.ChartDate `json:"items"`
}

func (s *ChartsService) Dates(ctx context.Context, country, provider, category string, limit int64) (*ChartDatesResponse, error) {
	country, err := validCountry(country, true)
	if err != nil {
		return nil, err
	}
	if provider, err = validProvider(provider); err != nil || provider == "" {
		return nil, &ValidationError{Message: "invalid provider"}
	}
	if category, err = validCategory(category); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 366
	}
	items, err := s.repo.ChartDates(ctx, country, provider, category, limit)
	if err != nil {
		return nil, err
	}
	return &ChartDatesResponse{Country: country, Provider: provider, Items: items}, nil
}

// --- history ------------------------------------------------------------------

// HistoryQuery selects one chart over a date range.
type HistoryQuery struct {
	Country  string
	Provider string
	Category string // movies | tv_shows
	From, To *time.Time
}

type ChartDay struct {
	Date  string           `json:"date"`
	Items []ChartDayResult `json:"items"`
}

type ChartDayResult struct {
	Rank         int64  `json:"rank"`
	SeasonNumber *int64 `json:"season_number,omitempty"`
	ChartTitle
}

type ChartHistoryResponse struct {
	Country  string     `json:"country"`
	Provider string     `json:"provider"`
	Category string     `json:"category"`
	From     string     `json:"from"`
	To       string     `json:"to"`
	Days     []ChartDay `json:"days"` // stored dates only, oldest first
}

const maxHistoryDays = 92

func (s *ChartsService) History(ctx context.Context, q HistoryQuery) (*ChartHistoryResponse, error) {
	country, err := validCountry(q.Country, true)
	if err != nil {
		return nil, err
	}
	provider, err := validProvider(q.Provider)
	if err != nil || provider == "" {
		return nil, &ValidationError{Message: "invalid provider"}
	}
	category, err := validCategory(q.Category)
	if err != nil || category == "" {
		return nil, &ValidationError{Message: "invalid category, expected movies or tv_shows"}
	}
	from, to, err := s.dateRange(q.From, q.To, 7, maxHistoryDays)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ChartEntries(ctx, repository.ChartEntryFilter{From: from, To: to, Country: country, Provider: provider, Category: category})
	if err != nil {
		return nil, err
	}
	out := &ChartHistoryResponse{Country: country, Provider: provider, Category: category,
		From: repository.FormatDate(from), To: repository.FormatDate(to), Days: []ChartDay{}}
	for _, r := range rows {
		if n := len(out.Days); n == 0 || out.Days[n-1].Date != r.Date {
			out.Days = append(out.Days, ChartDay{Date: r.Date})
		}
		day := &out.Days[len(out.Days)-1]
		day.Items = append(day.Items, ChartDayResult{Rank: r.Rank, SeasonNumber: r.SeasonNumber, ChartTitle: chartTitle(r)})
	}
	return out, nil
}

// --- movers -------------------------------------------------------------------

type MoversQuery struct {
	Country  string
	Provider string
	Category string     // "" = both
	Date     *time.Time // nil = latest stored date per chart
}

type Mover struct {
	Category     string `json:"category"`
	Rank         *int64 `json:"rank"`          // null for exits
	PreviousRank *int64 `json:"previous_rank"` // null for new entries
	Change       int64  `json:"change"`        // positions climbed (negative = fell); 0 for debuts, re-entries and exits
	ChartTitle
}

type MoversChart struct {
	Category     string  `json:"category"`
	Date         string  `json:"date"`
	PreviousDate *string `json:"previous_date"`
	Climbers     []Mover `json:"climbers"`
	Fallers      []Mover `json:"fallers"`
	Debuts       []Mover `json:"debuts"`     // first time on this chart in the last 90 days
	ReEntries    []Mover `json:"re_entries"` // back after dropping out
	Exits        []Mover `json:"exits"`      // on the previous date, gone now
}

type MoversResponse struct {
	Country  string        `json:"country"`
	Provider string        `json:"provider"`
	Charts   []MoversChart `json:"charts"`
}

func (s *ChartsService) Movers(ctx context.Context, q MoversQuery) (*MoversResponse, error) {
	country, err := validCountry(q.Country, true)
	if err != nil {
		return nil, err
	}
	provider, err := validProvider(q.Provider)
	if err != nil || provider == "" {
		return nil, &ValidationError{Message: "invalid provider"}
	}
	category, err := validCategory(q.Category)
	if err != nil {
		return nil, err
	}
	end := flixpatrol.EffectiveTop10Date(s.now())
	if q.Date != nil {
		end = *q.Date
	}
	rows, err := s.repo.ChartEntries(ctx, repository.ChartEntryFilter{
		From: end.AddDate(0, 0, -90), To: end, Country: country, Provider: provider, Category: category,
	})
	if err != nil {
		return nil, err
	}
	out := &MoversResponse{Country: country, Provider: provider, Charts: []MoversChart{}}
	byCategory := map[string][]repository.ChartEntryRow{}
	for _, r := range rows {
		byCategory[r.Category] = append(byCategory[r.Category], r)
	}
	for _, cat := range []string{"movies", "tv_shows"} {
		if entries := byCategory[cat]; len(entries) > 0 {
			out.Charts = append(out.Charts, movers(cat, entries))
		}
	}
	if len(out.Charts) == 0 {
		return nil, ErrTop10NotFound
	}
	return out, nil
}

// movers compares a chart's latest date in entries (date-ordered) with the
// stored date before it.
func movers(category string, entries []repository.ChartEntryRow) MoversChart {
	var dates []string
	byDate := map[string][]repository.ChartEntryRow{}
	for _, e := range entries {
		if _, ok := byDate[e.Date]; !ok {
			dates = append(dates, e.Date)
		}
		byDate[e.Date] = append(byDate[e.Date], e)
	}
	latest := dates[len(dates)-1]
	mc := MoversChart{Category: category, Date: latest, Climbers: []Mover{}, Fallers: []Mover{}, Debuts: []Mover{}, ReEntries: []Mover{}, Exits: []Mover{}}
	prevRank := map[int64]int64{}
	seenBefore := map[int64]bool{}
	if len(dates) > 1 {
		prev := dates[len(dates)-2]
		mc.PreviousDate = &prev
		for _, e := range byDate[prev] {
			prevRank[e.TitleID] = e.Rank
		}
		for _, d := range dates[:len(dates)-1] {
			for _, e := range byDate[d] {
				seenBefore[e.TitleID] = true
			}
		}
	}
	current := map[int64]bool{}
	for _, e := range byDate[latest] {
		current[e.TitleID] = true
		rank := e.Rank
		m := Mover{Category: category, Rank: &rank, ChartTitle: chartTitle(e)}
		if p, ok := prevRank[e.TitleID]; ok {
			pr := p
			m.PreviousRank, m.Change = &pr, p-rank
			switch {
			case m.Change > 0:
				mc.Climbers = append(mc.Climbers, m)
			case m.Change < 0:
				mc.Fallers = append(mc.Fallers, m)
			}
			continue
		}
		if mc.PreviousDate == nil {
			continue
		}
		if seenBefore[e.TitleID] {
			mc.ReEntries = append(mc.ReEntries, m)
		} else {
			mc.Debuts = append(mc.Debuts, m)
		}
	}
	if mc.PreviousDate != nil {
		for _, e := range byDate[*mc.PreviousDate] {
			if !current[e.TitleID] {
				pr := e.Rank
				mc.Exits = append(mc.Exits, Mover{Category: category, PreviousRank: &pr, ChartTitle: chartTitle(e)})
			}
		}
	}
	sort.SliceStable(mc.Climbers, func(i, j int) bool { return mc.Climbers[i].Change > mc.Climbers[j].Change })
	sort.SliceStable(mc.Fallers, func(i, j int) bool { return mc.Fallers[i].Change < mc.Fallers[j].Change })
	return mc
}

// --- leaderboards -----------------------------------------------------------

type LeaderboardQuery struct {
	Period   string     // day | week | month | custom (from/to)
	Date     *time.Time // end of the period; default the current chart date
	From, To *time.Time // custom period
	Country  string     // "" = every country
	Provider string     // "" = every provider
	Category string     // "" = both
	Kind     string     // "" | movie | tv_show
	Limit    int
}

type LeaderboardEntry struct {
	Position     int      `json:"position"`
	Points       int64    `json:"points"`
	DaysOnCharts int64    `json:"days_on_charts"` // distinct dates
	Appearances  int64    `json:"appearances"`    // chart × date entries
	BestRank     int64    `json:"best_rank"`
	Countries    []string `json:"countries"`
	Providers    []string `json:"providers"`
	ChartTitle
	// Slugs: every FlixPatrol title aggregated into this entry (one TMDB title).
	Slugs []string `json:"slugs"`
}

type LeaderboardResponse struct {
	Period   string             `json:"period"`
	From     string             `json:"from"`
	To       string             `json:"to"`
	Country  *string            `json:"country"`
	Provider *string            `json:"provider"`
	Category *string            `json:"category"`
	Kind     *string            `json:"kind"`
	Items    []LeaderboardEntry `json:"items"`
}

const maxLeaderboardDays = 366

func (s *ChartsService) Leaderboard(ctx context.Context, q LeaderboardQuery) (*LeaderboardResponse, error) {
	country, err := validCountry(q.Country, false)
	if err != nil {
		return nil, err
	}
	provider, err := validProvider(q.Provider)
	if err != nil {
		return nil, err
	}
	category, err := validCategory(q.Category)
	if err != nil {
		return nil, err
	}
	kind, err := validKind(q.Kind)
	if err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	period := strings.ToLower(strings.TrimSpace(q.Period))
	end := flixpatrol.EffectiveTop10Date(s.now())
	if q.Date != nil {
		end = *q.Date
	}
	var from, to time.Time
	switch period {
	case "", "week":
		period, from, to = "week", end.AddDate(0, 0, -6), end
	case "day":
		from, to = end, end
	case "month":
		from, to = end.AddDate(0, 0, -29), end
	case "custom":
		if q.From == nil || q.To == nil {
			return nil, &ValidationError{Message: "period=custom needs from and to"}
		}
		if from, to, err = s.dateRange(q.From, q.To, 7, maxLeaderboardDays); err != nil {
			return nil, err
		}
	default:
		return nil, &ValidationError{Message: "invalid period, expected day, week, month or custom"}
	}

	rows, err := s.repo.ChartEntries(ctx, repository.ChartEntryFilter{From: from, To: to, Country: country, Provider: provider, Category: category})
	if err != nil {
		return nil, err
	}
	out := &LeaderboardResponse{Period: period, From: repository.FormatDate(from), To: repository.FormatDate(to),
		Country: optStr(country), Provider: optStr(provider), Category: optStr(category), Kind: optStr(kind)}
	out.Items = aggregateLeaderboard(rows, kind, limit)
	return out, nil
}

// aggregateLeaderboard sums points per title; titles mapped to the same TMDB
// title are one entry.
func aggregateLeaderboard(rows []repository.ChartEntryRow, kind string, limit int) []LeaderboardEntry {
	type acc struct {
		entry     LeaderboardEntry
		dates     map[string]bool
		countries map[string]bool
		providers map[string]bool
		slugs     map[string]bool
	}
	byKey := map[string]*acc{}
	var order []string
	for _, r := range rows {
		if kind != "" && r.Kind != kind {
			continue
		}
		key := "id:" + strconv.FormatInt(r.TitleID, 10)
		if r.TmdbID != nil && *r.TmdbID != "" {
			key = r.Kind + ":" + *r.TmdbID
		}
		a := byKey[key]
		if a == nil {
			a = &acc{entry: LeaderboardEntry{ChartTitle: chartTitle(r), BestRank: r.Rank},
				dates: map[string]bool{}, countries: map[string]bool{}, providers: map[string]bool{}, slugs: map[string]bool{}}
			byKey[key] = a
			order = append(order, key)
		}
		a.entry.Points += Points(r.Rank)
		a.entry.Appearances++
		if r.Rank < a.entry.BestRank {
			a.entry.BestRank = r.Rank
		}
		a.dates[r.Date] = true
		a.countries[r.Country] = true
		a.providers[r.Provider] = true
		a.slugs[r.Slug] = true
	}
	items := make([]LeaderboardEntry, 0, len(order))
	for _, k := range order {
		a := byKey[k]
		a.entry.DaysOnCharts = int64(len(a.dates))
		a.entry.Countries, a.entry.Providers, a.entry.Slugs = sortedKeys(a.countries), sortedKeys(a.providers), sortedKeys(a.slugs)
		items = append(items, a.entry)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Points != items[j].Points {
			return items[i].Points > items[j].Points
		}
		return items[i].BestRank < items[j].BestRank
	})
	if len(items) > limit {
		items = items[:limit]
	}
	for i := range items {
		items[i].Position = i + 1
	}
	return items
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// --- combined top 10 ----------------------------------------------------------

// Top10Combined is every provider's latest (or dated) movie and TV chart of a
// country in one response.
type Top10Combined struct {
	Country string            `json:"country"`
	Movies  []Top10Result     `json:"movies"`
	TVShows []Top10Result     `json:"tv_shows"`
	Dates   map[string]string `json:"dates"` // provider → chart date served
}

// GetCountryTop10 combines GetMoviesAllProviders and GetTVShowsAllProviders.
func (s *Top10ReadService) GetCountryTop10(ctx context.Context, q Top10Query) (*Top10Combined, error) {
	movies, errM := s.GetMoviesAllProviders(ctx, q)
	tv, errT := s.GetTVShowsAllProviders(ctx, q)
	if errM != nil && errT != nil {
		return nil, errM
	}
	out := &Top10Combined{Movies: []Top10Result{}, TVShows: []Top10Result{}, Dates: map[string]string{}}
	if movies != nil {
		out.Country, out.Movies = movies.Country, movies.Items
	}
	if tv != nil {
		out.Country, out.TVShows = tv.Country, tv.Items
	}
	for _, list := range [][]Top10Result{out.Movies, out.TVShows} {
		for _, it := range list {
			if it.Date > out.Dates[it.Provider] {
				out.Dates[it.Provider] = it.Date
			}
		}
	}
	return out, nil
}

// --- change feed --------------------------------------------------------------

type ChartChangesResponse struct {
	Since string                   `json:"since"`
	Next  string                   `json:"next"` // pass as since to continue
	Items []repository.ChartChange `json:"items"`
}

func (s *ChartsService) Changes(ctx context.Context, since time.Time, limit int64) (*ChartChangesResponse, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	items, err := s.repo.ChartChangesSince(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := &ChartChangesResponse{Since: since.UTC().Format(time.RFC3339), Next: since.UTC().Format(time.RFC3339), Items: items}
	if n := len(items); n > 0 {
		out.Next = items[n-1].ScrapedAt.UTC().Format(time.RFC3339)
	}
	return out, nil
}

// --- export ---------------------------------------------------------------------

type ExportQuery struct {
	From, To                    *time.Time
	Country, Provider, Category string
	Format                      string // csv | ndjson
}

const maxExportDays = 366

// ExportFormat validates the format and returns its content type.
func ExportFormat(format string) (string, error) {
	switch strings.ToLower(format) {
	case "", "csv":
		return "text/csv; charset=utf-8", nil
	case "ndjson", "jsonl":
		return "application/x-ndjson", nil
	}
	return "", &ValidationError{Message: "invalid format, expected csv or ndjson"}
}

// ValidateExport checks an export query before the response starts.
func (s *ChartsService) ValidateExport(q ExportQuery) error {
	_, err := s.exportFilter(q)
	return err
}

func (s *ChartsService) exportFilter(q ExportQuery) (repository.ChartEntryFilter, error) {
	var f repository.ChartEntryFilter
	var err error
	if f.Country, err = validCountry(q.Country, false); err != nil {
		return f, err
	}
	if f.Provider, err = validProvider(q.Provider); err != nil {
		return f, err
	}
	if f.Category, err = validCategory(q.Category); err != nil {
		return f, err
	}
	f.From, f.To, err = s.dateRange(q.From, q.To, 30, maxExportDays)
	return f, err
}

// Export writes chart entries in the range to w.
func (s *ChartsService) Export(ctx context.Context, q ExportQuery, w io.Writer) error {
	f, err := s.exportFilter(q)
	if err != nil {
		return err
	}
	rows, err := s.repo.ChartEntries(ctx, f)
	if err != nil {
		return err
	}
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	if strings.EqualFold(q.Format, "ndjson") || strings.EqualFold(q.Format, "jsonl") {
		enc := json.NewEncoder(w)
		for _, r := range rows {
			if err := enc.Encode(map[string]any{
				"date": r.Date, "country": r.Country, "provider": r.Provider, "category": r.Category, "rank": r.Rank,
				"season_number": r.SeasonNumber, "title_id": r.TitleID, "slug": r.Slug, "name": r.Name, "kind": r.Kind,
				"tmdb_id": r.TmdbID, "imdb_id": r.ImdbID,
			}); err != nil {
				return err
			}
		}
		return nil
	}
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"date", "country", "provider", "category", "rank", "season_number", "title_id", "slug", "name", "kind", "tmdb_id", "imdb_id"})
	for _, r := range rows {
		season := ""
		if r.SeasonNumber != nil {
			season = strconv.FormatInt(*r.SeasonNumber, 10)
		}
		if err := cw.Write([]string{r.Date, r.Country, r.Provider, r.Category, strconv.FormatInt(r.Rank, 10), season,
			strconv.FormatInt(r.TitleID, 10), r.Slug, r.Name, r.Kind, str(r.TmdbID), str(r.ImdbID)}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// Countries lists the countries FlixPatrol charts can be scraped for.
func (s *ChartsService) Countries() []flixpatrol.Country { return flixpatrol.Countries() }

// ProviderInfo is a chart provider and the countries it has stored charts in.
type ProviderInfo struct {
	Slug      string   `json:"slug"`
	Countries []string `json:"countries"` // with stored charts
}

// Providers lists known providers plus any others with stored charts.
func (s *ChartsService) Providers(ctx context.Context) ([]ProviderInfo, error) {
	rows, err := s.repo.ChartCatalog(ctx)
	if err != nil {
		return nil, err
	}
	countries := map[string]map[string]bool{}
	for _, p := range flixpatrol.KnownProviders {
		countries[string(p)] = map[string]bool{}
	}
	for _, r := range rows {
		if countries[r.Provider] == nil {
			countries[r.Provider] = map[string]bool{}
		}
		countries[r.Provider][r.Country] = true
	}
	out := make([]ProviderInfo, 0, len(countries))
	for p, cs := range countries {
		out = append(out, ProviderInfo{Slug: p, Countries: sortedKeys(cs)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}
