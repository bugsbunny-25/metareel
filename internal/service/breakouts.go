package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// Breakouts: titles rising on Top 10 charts in several countries at once.
// Each chart's latest stored date (on or a day before the date asked for)
// is compared with its stored date about `window` days earlier: a title is
// rising on the chart when it entered it since then, or climbed at least
// min_climb places. It is a breakout when it rises in at least
// min_countries countries.

const (
	breakoutDefaultWindow    = 3
	breakoutMaxWindow        = 14
	breakoutDefaultCountries = 2
	breakoutDefaultClimb     = 3
	breakoutDefaultLimit     = 50
	breakoutMaxLimit         = 200
	// A chart whose latest stored date is more than this many days before
	// the date asked for is stale and left out.
	breakoutStaleDays = 1
	// The base date may fall up to this many days before date - window
	// (when the chart wasn't stored on that exact day).
	breakoutBaseSlack = 7
)

// BreakoutService finds breakouts in stored charts.
type BreakoutService struct {
	repo *repository.AnalyticsRepository
	now  func() time.Time
}

func NewBreakoutService(repo *repository.AnalyticsRepository) *BreakoutService {
	return &BreakoutService{repo: repo, now: time.Now}
}

// BreakoutQuery filters breakouts; zero values take the defaults.
type BreakoutQuery struct {
	Date         *time.Time // nil = the current chart date
	Window       int        // days, 1-14 (default 3)
	MinCountries int        // default 2
	MinClimb     int        // places, 1-9 (default 3)
	Category     string     // movies | tv_shows | "" = both
	Provider     string     // "" = every provider
	Kind         string     // movie | tv_show | ""
	Limit        int        // default 50, at most 200
}

// BreakoutCountry is how a title rose in one country (its best chart there).
type BreakoutCountry struct {
	Country  string `json:"country"`
	Provider string `json:"provider"`
	Category string `json:"category"`
	Date     string `json:"date"`      // the chart's latest stored date
	BaseDate string `json:"base_date"` // the stored date compared with
	Rank     int64  `json:"rank"`
	BaseRank *int64 `json:"base_rank"` // null = not on the chart on base_date
	Movement string `json:"movement"`  // entered | climbed
	Gain     int64  `json:"gain"`      // chart points gained (rank 1 = 10 … rank 10 = 1)
}

// Breakout is a title rising in several countries.
type Breakout struct {
	ChartTitle
	RisingCountries   int               `json:"rising_countries"`
	ChartingCountries []string          `json:"charting_countries"` // on a chart there on its latest date, rising or not
	Score             int64             `json:"score"`              // sum of the rising countries' gains
	Countries         []BreakoutCountry `json:"countries"`          // rising countries, biggest gain first
	key               string
}

type BreakoutsResponse struct {
	Date         string  `json:"date"`
	Window       int     `json:"window"`
	MinCountries int     `json:"min_countries"`
	MinClimb     int     `json:"min_climb"`
	Category     *string `json:"category"`
	Provider     *string `json:"provider"`
	Kind         *string `json:"kind"`
	// CountriesChecked: countries with a current chart (not stale) that
	// could be compared.
	CountriesChecked []string   `json:"countries_checked"`
	Total            int        `json:"total"`
	Items            []Breakout `json:"items"`
	limit            int
}

func (q *BreakoutQuery) normalize() error {
	if q.Window == 0 {
		q.Window = breakoutDefaultWindow
	}
	if q.Window < 1 || q.Window > breakoutMaxWindow {
		return &ValidationError{Message: fmt.Sprintf("window must be 1-%d days", breakoutMaxWindow)}
	}
	if q.MinCountries == 0 {
		q.MinCountries = breakoutDefaultCountries
	}
	if q.MinCountries < 1 || q.MinCountries > 250 {
		return &ValidationError{Message: "min_countries must be 1-250"}
	}
	if q.MinClimb == 0 {
		q.MinClimb = breakoutDefaultClimb
	}
	if q.MinClimb < 1 || q.MinClimb > 9 {
		return &ValidationError{Message: "min_climb must be 1-9"}
	}
	if q.Limit == 0 {
		q.Limit = breakoutDefaultLimit
	}
	if q.Limit < 1 || q.Limit > breakoutMaxLimit {
		return &ValidationError{Message: fmt.Sprintf("limit must be 1-%d", breakoutMaxLimit)}
	}
	var err error
	if q.Category, err = validCategory(q.Category); err != nil {
		return err
	}
	if q.Provider, err = validProvider(q.Provider); err != nil {
		return err
	}
	q.Kind, err = validKind(q.Kind)
	return err
}

// Breakouts lists titles rising in at least MinCountries countries, the
// most countries first, then by score.
func (s *BreakoutService) Breakouts(ctx context.Context, q BreakoutQuery) (*BreakoutsResponse, error) {
	out, err := s.all(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(out.Items) > out.limit {
		out.Items = out.Items[:out.limit]
	}
	return out, nil
}

// all is Breakouts without the limit.
func (s *BreakoutService) all(ctx context.Context, q BreakoutQuery) (*BreakoutsResponse, error) {
	if err := q.normalize(); err != nil {
		return nil, err
	}
	date := flixpatrol.EffectiveTop10Date(s.now())
	if q.Date != nil {
		date = *q.Date
	}
	rows, err := s.repo.ChartEntries(ctx, repository.ChartEntryFilter{
		From: date.AddDate(0, 0, -(q.Window + breakoutStaleDays + breakoutBaseSlack)), To: date,
		Provider: q.Provider, Category: q.Category,
	})
	if err != nil {
		return nil, err
	}
	items, checked, err := findBreakouts(ctx, rows, date, q)
	if err != nil {
		return nil, err
	}
	out := &BreakoutsResponse{Date: repository.FormatDate(date), Window: q.Window, MinCountries: q.MinCountries, MinClimb: q.MinClimb,
		Category: optStr(q.Category), Provider: optStr(q.Provider), Kind: optStr(q.Kind),
		CountriesChecked: checked, Total: len(items), Items: items, limit: q.Limit}
	return out, nil
}

// findBreakouts works on chart entries (any order) up to date.
func findBreakouts(ctx context.Context, rows []repository.ChartEntryRow, date time.Time, q BreakoutQuery) ([]Breakout, []string, error) {
	type chartKey struct{ country, provider, category string }
	byChart := map[chartKey]map[string]map[string]int64{} // chart -> date -> title -> best rank
	titles := map[string]repository.ChartEntryRow{}
	for _, r := range rows {
		if q.Kind != "" && r.Kind != q.Kind {
			continue
		}
		k := chartKey{r.Country, r.Provider, r.Category}
		if byChart[k] == nil {
			byChart[k] = map[string]map[string]int64{}
		}
		if byChart[k][r.Date] == nil {
			byChart[k][r.Date] = map[string]int64{}
		}
		tk := titleKey(r)
		if cur, ok := byChart[k][r.Date][tk]; !ok || r.Rank < cur {
			byChart[k][r.Date][tk] = r.Rank
		}
		if prev, ok := titles[tk]; !ok || r.Date >= prev.Date {
			titles[tk] = r
		}
	}
	// Kind filtering above can hide a chart date whose entries are all of
	// the other kind; stored dates come from every row.
	stored := map[chartKey][]string{}
	seen := map[chartKey]map[string]bool{}
	for _, r := range rows {
		k := chartKey{r.Country, r.Provider, r.Category}
		if seen[k] == nil {
			seen[k] = map[string]bool{}
		}
		if !seen[k][r.Date] {
			seen[k][r.Date] = true
			stored[k] = append(stored[k], r.Date)
		}
	}
	dateStr := repository.FormatDate(date)
	staleBefore := repository.FormatDate(date.AddDate(0, 0, -breakoutStaleDays))

	type titleCountry struct{ title, country string }
	best := map[titleCountry]BreakoutCountry{}
	charting := map[string]map[string]bool{}
	checked := map[string]bool{}
	for k, dates := range stored {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		sort.Strings(dates)
		cur := latestOnOrBefore(dates, dateStr)
		if cur == "" || cur < staleBefore {
			continue
		}
		curDay := parseDay(cur)
		base := latestOnOrBefore(dates, repository.FormatDate(curDay.AddDate(0, 0, -q.Window)))
		if base == "" || base < repository.FormatDate(curDay.AddDate(0, 0, -(q.Window+breakoutBaseSlack))) {
			continue
		}
		checked[k.country] = true
		for tk, rank := range byChart[k][cur] {
			if charting[tk] == nil {
				charting[tk] = map[string]bool{}
			}
			charting[tk][k.country] = true
			c := BreakoutCountry{Country: k.country, Provider: k.provider, Category: k.category, Date: cur, BaseDate: base, Rank: rank}
			if baseRank, ok := byChart[k][base][tk]; ok {
				if baseRank-rank < int64(q.MinClimb) {
					continue
				}
				br := baseRank
				c.BaseRank, c.Movement, c.Gain = &br, "climbed", baseRank-rank
			} else {
				c.Movement, c.Gain = "entered", Points(rank)
			}
			tc := titleCountry{tk, k.country}
			if prev, ok := best[tc]; !ok || c.Gain > prev.Gain || (c.Gain == prev.Gain && c.Rank < prev.Rank) {
				best[tc] = c
			}
		}
	}
	rising := map[string][]BreakoutCountry{}
	for tc, c := range best {
		rising[tc.title] = append(rising[tc.title], c)
	}
	var out []Breakout
	for tk, cs := range rising {
		if len(cs) < q.MinCountries {
			continue
		}
		sort.Slice(cs, func(i, j int) bool {
			if cs[i].Gain != cs[j].Gain {
				return cs[i].Gain > cs[j].Gain
			}
			return cs[i].Country < cs[j].Country
		})
		b := Breakout{ChartTitle: chartTitle(titles[tk]), RisingCountries: len(cs), Countries: cs, key: tk}
		for _, c := range cs {
			b.Score += c.Gain
		}
		for country := range charting[tk] {
			b.ChartingCountries = append(b.ChartingCountries, country)
		}
		sort.Strings(b.ChartingCountries)
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.RisingCountries != b.RisingCountries {
			return a.RisingCountries > b.RisingCountries
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.key < b.key
	})
	if out == nil {
		out = []Breakout{}
	}
	countries := make([]string, 0, len(checked))
	for c := range checked {
		countries = append(countries, c)
	}
	sort.Strings(countries)
	return out, countries, nil
}

// latestOnOrBefore returns the last of sorted dates that is <= day, or "".
func latestOnOrBefore(sorted []string, day string) string {
	i := sort.SearchStrings(sorted, day)
	if i < len(sorted) && sorted[i] == day {
		return day
	}
	if i == 0 {
		return ""
	}
	return sorted[i-1]
}
