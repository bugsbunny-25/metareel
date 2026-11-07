package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// Spread lag: how long after a title first charts in one country it charts
// in the others. A title's debut is its first chart appearance in any
// country, in the range, with none from 30 days before the range; the
// countries it debuted in are its origins. Each other country is followed for `window`
// days after the debut.
//
// A country only counts for a title when its charts were stored on at least
// 80% of the 30 days before the debut (otherwise an earlier appearance there
// could be missing, e.g. before scraping started or during a gap); a title
// whose origins include a country without that history is left out. A
// country is "not reached" only when its charts were stored on 80% of the
// window's days; with a gap it is left out for that title.

const (
	spreadLookback      = 30
	spreadMinCoverage   = 0.8 // share of days with stored charts
	spreadDefaultWindow = 30
	spreadMinWindow     = 7
	spreadMaxWindow     = 90
	spreadMaxItems      = 200
	spreadMaxDays       = 180 // the range; rows are read for 30 + window days more
)

// SpreadArrival is a title's first appearance in a country.
type SpreadArrival struct {
	Country   string `json:"country"`
	Provider  string `json:"provider"` // chart of the first appearance (best rank that day)
	Category  string `json:"category"`
	FirstDate string `json:"first_date"`
	LagDays   int    `json:"lag_days"` // days after the debut (0 = an origin)
	Rank      int64  `json:"rank"`     // rank on first_date
}

// SpreadItem is one title's spread.
type SpreadItem struct {
	ChartTitle
	DebutDate  string          `json:"debut_date"`
	Origins    []string        `json:"origins"`     // countries it charted in on the debut date
	Arrivals   []SpreadArrival `json:"arrivals"`    // origins and later arrivals within the window, by lag
	NotReached []string        `json:"not_reached"` // followed for the whole window without charting
	Pending    []string        `json:"pending"`     // not charted yet, window not over
}

// SpreadCountry sums up one country.
type SpreadCountry struct {
	Country string `json:"country"`
	// Origin: titles that debuted here (alone or with other countries).
	Origin int `json:"origin"`
	// Arrived: titles that charted here later than their debut elsewhere,
	// within the window, and their lags.
	Arrived    int        `json:"arrived"`
	ArrivalLag LagSummary `json:"arrival_lag"`
	NotReached int        `json:"not_reached"`
}

// SpreadPair is a single-origin route: titles that debuted only in From and
// were followed in To.
type SpreadPair struct {
	From       string     `json:"from"`
	To         string     `json:"to"`
	Reached    int        `json:"reached"`
	NotReached int        `json:"not_reached"`
	Lag        LagSummary `json:"lag"`
}

type SpreadLagResponse struct {
	AnalyticsEcho
	Window int `json:"window"`
	// Titles: debuts in the range with at least one other country followed.
	Titles int `json:"titles"`
	// MultiCountry: of those, titles that charted in two or more countries
	// within the window; Simultaneous: that debuted in several countries on
	// the same day.
	MultiCountry int `json:"multi_country"`
	Simultaneous int `json:"simultaneous"`
	// Summary: lags of every arrival after the first country (0 for the
	// extra origins of a simultaneous debut).
	Summary    LagSummary      `json:"summary"`
	Countries  []SpreadCountry `json:"countries"`
	Pairs      []SpreadPair    `json:"pairs"`
	ItemsTotal int             `json:"items_total"`
	Items      []SpreadItem    `json:"items"` // multi-country titles, newest debut first (at most 200)
}

func parseSpreadWindow(raw string) (int, error) {
	if raw == "" {
		return spreadDefaultWindow, nil
	}
	w, err := strconv.Atoi(raw)
	if err != nil || w < spreadMinWindow || w > spreadMaxWindow {
		return 0, &ValidationError{Message: fmt.Sprintf("window must be %d-%d days", spreadMinWindow, spreadMaxWindow)}
	}
	return w, nil
}

// SpreadLag measures how titles that debuted in the range spread between
// countries. country (optional) keeps titles that debuted there.
func (s *AnalyticsService) SpreadLag(ctx context.Context, q AnalyticsQuery) (*SpreadLagResponse, error) {
	f, err := s.filter(q, 90)
	if err != nil {
		return nil, err
	}
	if days := int(f.to.Sub(f.from).Hours()/24) + 1; days > spreadMaxDays {
		return nil, &ValidationError{Message: fmt.Sprintf("range too long: at most %d days", spreadMaxDays)}
	}
	window, err := parseSpreadWindow(q.Window)
	if err != nil {
		return nil, err
	}
	loadTo := f.to.AddDate(0, 0, window)
	if cur := flixpatrol.EffectiveTop10Date(s.now()); cur.Before(loadTo) {
		loadTo = cur
	}
	rows, err := s.repo.ChartEntries(ctx, repository.ChartEntryFilter{
		From: f.from.AddDate(0, 0, -spreadLookback), To: loadTo, Provider: f.provider, Category: f.category,
	})
	if err != nil {
		return nil, err
	}
	out, err := computeSpread(ctx, rows, f, window)
	if err != nil {
		return nil, err
	}
	out.AnalyticsEcho = f.echo()
	out.Window = window
	return out, nil
}

func computeSpread(ctx context.Context, rows []repository.ChartEntryRow, f analyticsFilter, window int) (*SpreadLagResponse, error) {
	storedSet := map[string]map[string]bool{}
	lastStored := map[string]string{}
	type titleState struct {
		title repository.ChartEntryRow
		first map[string]SpreadArrival // country -> first appearance
	}
	titles := map[string]*titleState{}
	var order []string
	for _, r := range rows {
		if storedSet[r.Country] == nil {
			storedSet[r.Country] = map[string]bool{}
		}
		storedSet[r.Country][r.Date] = true
		if r.Date > lastStored[r.Country] {
			lastStored[r.Country] = r.Date
		}
		if f.kind != "" && r.Kind != f.kind {
			continue
		}
		k := titleKey(r)
		t := titles[k]
		if t == nil {
			t = &titleState{title: r, first: map[string]SpreadArrival{}}
			titles[k] = t
			order = append(order, k)
		}
		a, ok := t.first[r.Country]
		if !ok || r.Date < a.FirstDate || (r.Date == a.FirstDate && r.Rank < a.Rank) {
			t.first[r.Country] = SpreadArrival{Country: r.Country, Provider: r.Provider, Category: r.Category, FirstDate: r.Date, Rank: r.Rank}
		}
	}
	countries := make([]string, 0, len(storedSet))
	stored := map[string][]string{}
	for c, set := range storedSet {
		countries = append(countries, c)
		for d := range set {
			stored[c] = append(stored[c], d)
		}
		sort.Strings(stored[c])
	}
	sort.Strings(countries)
	// covered: charts of c were stored on at least 80% of the days from..to.
	covered := func(c string, from, to time.Time) bool {
		days := int(to.Sub(from).Hours()/24) + 1
		lo := sort.SearchStrings(stored[c], repository.FormatDate(from))
		hi := sort.SearchStrings(stored[c], repository.FormatDate(to.AddDate(0, 0, 1)))
		return float64(hi-lo) >= spreadMinCoverage*float64(days)
	}

	from, to := repository.FormatDate(f.from), repository.FormatDate(f.to)
	out := &SpreadLagResponse{Countries: []SpreadCountry{}, Pairs: []SpreadPair{}, Items: []SpreadItem{}}
	var allLags []float64
	type pairKey struct{ from, to string }
	pairLags := map[pairKey][]float64{}
	pairMissed := map[pairKey]int{}
	byCountry := map[string]*SpreadCountry{}
	arrivalLags := map[string][]float64{}
	for _, c := range countries {
		byCountry[c] = &SpreadCountry{Country: c}
	}
	for i, k := range order {
		if i%1000 == 0 && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		t := titles[k]
		debut := ""
		for _, a := range t.first {
			if debut == "" || a.FirstDate < debut {
				debut = a.FirstDate
			}
		}
		if debut < from || debut > to {
			continue
		}
		item := SpreadItem{ChartTitle: chartTitle(t.title), DebutDate: debut, Arrivals: []SpreadArrival{}, NotReached: []string{}, Pending: []string{}}
		debutDay := parseDay(debut)
		observed := func(c string) bool {
			return covered(c, debutDay.AddDate(0, 0, -spreadLookback), debutDay.AddDate(0, 0, -1))
		}
		censored := false
		for _, c := range countries {
			if a, ok := t.first[c]; ok && a.FirstDate == debut {
				item.Origins = append(item.Origins, c)
				if !observed(c) {
					censored = true
				}
			}
		}
		if censored || (f.country != "" && !containsStr(item.Origins, f.country)) {
			continue
		}
		windowEnd := repository.FormatDate(debutDay.AddDate(0, 0, window))
		followed := 0
		for _, c := range countries {
			a, ok := t.first[c]
			if ok && a.FirstDate == debut {
				item.Arrivals = append(item.Arrivals, a)
				continue
			}
			if !observed(c) {
				continue // no history before the debut: unknown
			}
			followed++
			switch {
			case ok && a.FirstDate <= windowEnd:
				a.LagDays = int(parseDay(a.FirstDate).Sub(debutDay).Hours() / 24)
				item.Arrivals = append(item.Arrivals, a)
			case lastStored[c] >= windowEnd:
				if !covered(c, debutDay, parseDay(windowEnd)) {
					followed-- // a gap in the window: unknown
					continue
				}
				item.NotReached = append(item.NotReached, c)
			default:
				item.Pending = append(item.Pending, c)
			}
		}
		if followed == 0 && len(item.Origins) < 2 {
			continue // nothing to compare with
		}
		sort.SliceStable(item.Arrivals, func(i, j int) bool {
			if item.Arrivals[i].LagDays != item.Arrivals[j].LagDays {
				return item.Arrivals[i].LagDays < item.Arrivals[j].LagDays
			}
			return item.Arrivals[i].Country < item.Arrivals[j].Country
		})
		out.Titles++
		if len(item.Origins) > 1 {
			out.Simultaneous++
		}
		for i := 1; i < len(item.Arrivals); i++ {
			allLags = append(allLags, float64(item.Arrivals[i].LagDays))
		}
		for _, c := range item.Origins {
			byCountry[c].Origin++
		}
		for _, a := range item.Arrivals {
			if a.LagDays > 0 {
				byCountry[a.Country].Arrived++
				arrivalLags[a.Country] = append(arrivalLags[a.Country], float64(a.LagDays))
			}
		}
		for _, c := range item.NotReached {
			byCountry[c].NotReached++
		}
		if len(item.Origins) == 1 {
			origin := item.Origins[0]
			for _, a := range item.Arrivals[1:] {
				pk := pairKey{origin, a.Country}
				pairLags[pk] = append(pairLags[pk], float64(a.LagDays))
			}
			for _, c := range item.NotReached {
				pairMissed[pairKey{origin, c}]++
			}
		}
		if len(item.Arrivals) >= 2 {
			out.MultiCountry++
			out.Items = append(out.Items, item)
		}
	}
	out.Summary = lagSummary(allLags)
	for _, c := range countries {
		sc := byCountry[c]
		sc.ArrivalLag = lagSummary(arrivalLags[c])
		if sc.Origin+sc.Arrived+sc.NotReached > 0 {
			out.Countries = append(out.Countries, *sc)
		}
	}
	seen := map[pairKey]bool{}
	for pk := range pairLags {
		seen[pk] = true
	}
	for pk := range pairMissed {
		seen[pk] = true
	}
	for pk := range seen {
		out.Pairs = append(out.Pairs, SpreadPair{From: pk.from, To: pk.to, Reached: len(pairLags[pk]), NotReached: pairMissed[pk], Lag: lagSummary(pairLags[pk])})
	}
	sort.Slice(out.Pairs, func(i, j int) bool {
		a, b := out.Pairs[i], out.Pairs[j]
		if a.Reached+a.NotReached != b.Reached+b.NotReached {
			return a.Reached+a.NotReached > b.Reached+b.NotReached
		}
		if a.From != b.From {
			return a.From < b.From
		}
		return a.To < b.To
	})
	sort.SliceStable(out.Items, func(i, j int) bool { return out.Items[i].DebutDate > out.Items[j].DebutDate })
	out.ItemsTotal = len(out.Items)
	if len(out.Items) > spreadMaxItems {
		out.Items = out.Items[:spreadMaxItems]
	}
	return out, nil
}
