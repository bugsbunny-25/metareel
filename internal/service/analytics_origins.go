package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// OriginsResponse is the origin mix of each provider's charts: how much of
// its chart points come from its originals, its parent company's titles and
// licensed ones, over time, and who supplies the licensed ones. Points are
// within one provider's charts (rank 1 = 10 … rank 10 = 1); providers'
// charts are separate, so compare shares, not points, across providers.
type OriginsResponse struct {
	AnalyticsEcho
	Interval  string            `json:"interval"`
	Providers []ProviderOrigins `json:"providers"`
}

// ProviderOrigins is one provider's origin mix.
type ProviderOrigins struct {
	Provider  string                  `json:"provider"`
	Points    int64                   `json:"points"`
	Origins   map[string]*OriginShare `json:"origins"`
	Series    []OriginPeriod          `json:"series"`
	Suppliers []OriginSupplier        `json:"suppliers"` // owners of licensed titles, by points
	Top       map[string][]OriginItem `json:"top"`       // top titles per origin (up to 5)
}

// OriginShare is the part of a provider's points from one origin.
type OriginShare struct {
	Points int64   `json:"points"`
	Share  float64 `json:"share"`
	Titles int     `json:"titles"`
}

// OriginPeriod is a provider's origin shares in one period (start date).
type OriginPeriod struct {
	Period string             `json:"period"`
	Points int64              `json:"points"`
	Shares map[string]float64 `json:"shares"`
}

// OriginSupplier is a media group whose titles a provider licenses.
type OriginSupplier struct {
	Owner  string  `json:"owner"` // "" = independent or not recognised
	Name   string  `json:"name"`
	Points int64   `json:"points"`
	Share  float64 `json:"share"` // of the provider's licensed points
	Titles int     `json:"titles"`
}

// OriginItem is a title in a provider's origin mix.
type OriginItem struct {
	Name   string  `json:"name"`
	Kind   string  `json:"kind"`
	TmdbID *string `json:"tmdb_id,omitempty"`
	Points int64   `json:"points"`
	Owner  string  `json:"owner,omitempty"`
	Reason string  `json:"reason,omitempty"`
}

const maxOriginTop = 5

// periodStart returns the start of the week (Monday) or month of a date.
func periodStart(date, interval string) string {
	d, err := time.Parse(repository.DateLayout, date)
	if err != nil {
		return date
	}
	if interval == "month" {
		return d.Format("2006-01") + "-01"
	}
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7)).Format(repository.DateLayout)
}

// Origins splits each provider's chart points by origin (default range:
// the last 90 days; interval week | month).
func (s *AnalyticsService) Origins(ctx context.Context, q AnalyticsQuery) (*OriginsResponse, error) {
	interval := q.Interval
	switch interval {
	case "":
		interval = "week"
	case "week", "month":
	default:
		return nil, &ValidationError{Message: fmt.Sprintf("invalid interval %q, expected week or month", q.Interval)}
	}
	f, err := s.filter(q, 90)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ChartEntries(ctx, repository.ChartEntryFilter{From: f.from, To: f.to, Country: f.country, Provider: f.provider, Category: f.category})
	if err != nil {
		return nil, err
	}
	var refs []repository.TmdbRef
	seenRef := map[string]bool{}
	for _, r := range rows {
		if r.TmdbID != nil && *r.TmdbID != "" {
			ref := repository.TmdbRef{Kind: tmdbKindOfTitle(r.Kind), ID: *r.TmdbID}
			if !seenRef[ref.String()] {
				seenRef[ref.String()] = true
				refs = append(refs, ref)
			}
		}
	}
	meta, err := s.metadata.GetMany(ctx, refs)
	if err != nil {
		return nil, err
	}

	type titleAcc struct {
		item   OriginItem
		origin string
	}
	type providerAcc struct {
		out     *ProviderOrigins
		titles  map[string]*titleAcc // by title key
		periods map[string]map[string]int64
		classes map[string]OriginInfo // memo by title key
	}
	byProvider := map[string]*providerAcc{}
	for _, r := range rows {
		if f.kind != "" && r.Kind != f.kind {
			continue
		}
		pts := Points(r.Rank)
		if pts == 0 {
			continue
		}
		pa := byProvider[r.Provider]
		if pa == nil {
			pa = &providerAcc{out: &ProviderOrigins{Provider: r.Provider}, titles: map[string]*titleAcc{},
				periods: map[string]map[string]int64{}, classes: map[string]OriginInfo{}}
			byProvider[r.Provider] = pa
		}
		key := titleKey(r)
		info, ok := pa.classes[key]
		if !ok {
			var m *repository.TmdbMetadata
			if r.TmdbID != nil {
				m = meta[repository.TmdbRef{Kind: tmdbKindOfTitle(r.Kind), ID: *r.TmdbID}.String()]
			}
			info = ClassifyOrigin(r.Provider, r.Kind == "movie", m)
			pa.classes[key] = info
		}
		ta := pa.titles[key]
		if ta == nil {
			ta = &titleAcc{origin: info.Origin, item: OriginItem{Name: r.Name, Kind: r.Kind, TmdbID: r.TmdbID, Owner: info.Owner, Reason: info.Reason}}
			pa.titles[key] = ta
		}
		ta.item.Points += pts
		pa.out.Points += pts
		p := periodStart(r.Date, interval)
		if pa.periods[p] == nil {
			pa.periods[p] = map[string]int64{}
		}
		pa.periods[p][info.Origin] += pts
	}

	out := &OriginsResponse{AnalyticsEcho: f.echo(), Interval: interval, Providers: []ProviderOrigins{}}
	for _, pa := range byProvider {
		po := pa.out
		po.Origins = map[string]*OriginShare{}
		po.Top = map[string][]OriginItem{}
		for _, o := range Origins {
			po.Origins[o] = &OriginShare{}
		}
		suppliers := map[string]*OriginSupplier{}
		var licensedPoints int64
		for _, ta := range pa.titles {
			os := po.Origins[ta.origin]
			os.Points += ta.item.Points
			os.Titles++
			po.Top[ta.origin] = append(po.Top[ta.origin], ta.item)
			if ta.origin == OriginLicensed {
				licensedPoints += ta.item.Points
				sp := suppliers[ta.item.Owner]
				if sp == nil {
					name := MediaGroupName(ta.item.Owner)
					if name == "" {
						name = "Independent / other"
					}
					sp = &OriginSupplier{Owner: ta.item.Owner, Name: name}
					suppliers[ta.item.Owner] = sp
				}
				sp.Points += ta.item.Points
				sp.Titles++
			}
		}
		for _, os := range po.Origins {
			if po.Points > 0 {
				os.Share = round(float64(os.Points)/float64(po.Points), 3)
			}
		}
		for o, items := range po.Top {
			sort.Slice(items, func(i, j int) bool {
				if items[i].Points != items[j].Points {
					return items[i].Points > items[j].Points
				}
				return items[i].Name < items[j].Name
			})
			po.Top[o] = items[:min(len(items), maxOriginTop)]
		}
		po.Suppliers = []OriginSupplier{}
		for _, sp := range suppliers {
			if licensedPoints > 0 {
				sp.Share = round(float64(sp.Points)/float64(licensedPoints), 3)
			}
			po.Suppliers = append(po.Suppliers, *sp)
		}
		sort.Slice(po.Suppliers, func(i, j int) bool {
			if po.Suppliers[i].Points != po.Suppliers[j].Points {
				return po.Suppliers[i].Points > po.Suppliers[j].Points
			}
			return po.Suppliers[i].Owner < po.Suppliers[j].Owner
		})
		po.Series = []OriginPeriod{}
		for p, byOrigin := range pa.periods {
			op := OriginPeriod{Period: p, Shares: map[string]float64{}}
			for _, v := range byOrigin {
				op.Points += v
			}
			for _, o := range Origins {
				op.Shares[o] = round(float64(byOrigin[o])/float64(op.Points), 3)
			}
			po.Series = append(po.Series, op)
		}
		sort.Slice(po.Series, func(i, j int) bool { return po.Series[i].Period < po.Series[j].Period })
		out.Providers = append(out.Providers, *po)
	}
	sort.Slice(out.Providers, func(i, j int) bool { return out.Providers[i].Provider < out.Providers[j].Provider })
	return out, nil
}
