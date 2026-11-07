package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// Release windows: a movie's theatrical, digital (PVOD / EST), physical and
// TV dates per country from TMDB's release dates, and when it reached
// streaming (subscription, free or ads). TMDB has no streaming release type,
// so the streaming date is observed: the first time metareel saw a
// streaming offer arrive (watch_provider_events, at a metadata refresh) or
// the movie's first FlixPatrol chart day in the country, whichever is
// earlier. Both are upper bounds: the movie may have arrived a little
// before.

const (
	releaseWindowsDefaultDays = 365
	releaseWindowsMaxItems    = 500
	// A first chart day only counts as a streaming arrival when the chart was
	// stored on most of the days before it (not the chart's first stored
	// days, or after a gap).
	releaseChartHistoryDays = 14
	releaseChartMinCoverage = 0.8
)

// WindowThresholds are the window lengths (days) counted in within_days:
// 17 (the shortest PVOD deals), 31, 45 (the classic exclusive window) and 90.
var WindowThresholds = []int{17, 31, 45, 90}

// ReleaseWindowsService serves movie release dates and windows.
type ReleaseWindowsService struct {
	repo     *repository.ReleaseDatesRepository
	metadata *repository.MetadataRepository
	charts   *ChartsService
}

func NewReleaseWindowsService(repo *repository.ReleaseDatesRepository, metadata *repository.MetadataRepository, analytics *repository.AnalyticsRepository) *ReleaseWindowsService {
	return &ReleaseWindowsService{repo: repo, metadata: metadata, charts: NewChartsService(analytics)}
}

// StreamingArrival is when a movie was first seen streaming in a country.
type StreamingArrival struct {
	Date         string `json:"date"`
	Basis        string `json:"basis"`                  // offer_added | first_chart
	Provider     string `json:"provider"`               // TMDB provider name (offer_added) or FlixPatrol service (first_chart)
	ProviderID   *int64 `json:"provider_id,omitempty"`  // TMDB provider id (offer_added)
	Monetization string `json:"monetization,omitempty"` // flatrate | free | ads (offer_added)
}

// StreamingOfferRef is a current streaming offer.
type StreamingOfferRef struct {
	ProviderID   int64  `json:"provider_id"`
	ProviderName string `json:"provider_name"`
	Monetization string `json:"monetization"`
}

// ReleaseWindowDays are the days between releases (null when a date is
// missing; negative when the later kind came first).
type ReleaseWindowDays struct {
	// TheatricalBasis: wide (type 3) or limited (type 2, when there is no
	// wide date).
	TheatricalBasis       *string `json:"theatrical_basis"`
	TheatricalToDigital   *int    `json:"theatrical_to_digital"`
	TheatricalToStreaming *int    `json:"theatrical_to_streaming"`
	DigitalToStreaming    *int    `json:"digital_to_streaming"`
}

// CountryReleases are a movie's releases in one country.
type CountryReleases struct {
	Country           string              `json:"country"`
	Premiere          *string             `json:"premiere"`
	TheatricalLimited *string             `json:"theatrical_limited"`
	Theatrical        *string             `json:"theatrical"`
	Digital           *string             `json:"digital"`
	Physical          *string             `json:"physical"`
	TV                *string             `json:"tv"`
	Notes             map[string]string   `json:"notes,omitempty"` // TMDB notes by release kind (e.g. a festival)
	Streaming         *StreamingArrival   `json:"streaming"`
	StreamingNow      []StreamingOfferRef `json:"streaming_now"`
	Windows           ReleaseWindowDays   `json:"windows"`
}

var releaseKinds = map[int]string{
	client.ReleasePremiere: "premiere", client.ReleaseTheatricalLimited: "theatrical_limited", client.ReleaseTheatrical: "theatrical",
	client.ReleaseDigital: "digital", client.ReleasePhysical: "physical", client.ReleaseTV: "tv",
}

func (c *CountryReleases) set(d repository.ReleaseDate) {
	date := d.Date
	switch d.Type {
	case client.ReleasePremiere:
		c.Premiere = &date
	case client.ReleaseTheatricalLimited:
		c.TheatricalLimited = &date
	case client.ReleaseTheatrical:
		c.Theatrical = &date
	case client.ReleaseDigital:
		c.Digital = &date
	case client.ReleasePhysical:
		c.Physical = &date
	case client.ReleaseTV:
		c.TV = &date
	default:
		return
	}
	if d.Note != "" {
		if c.Notes == nil {
			c.Notes = map[string]string{}
		}
		c.Notes[releaseKinds[d.Type]] = d.Note
	}
}

// theatricalStart is the wide theatrical date, else the limited one.
func (c *CountryReleases) theatricalStart() (string, string) {
	if c.Theatrical != nil {
		return *c.Theatrical, "wide"
	}
	if c.TheatricalLimited != nil {
		return *c.TheatricalLimited, "limited"
	}
	return "", ""
}

func windowDays(from, to string) *int {
	a, err1 := time.Parse(repository.DateLayout, from)
	b, err2 := time.Parse(repository.DateLayout, to)
	if err1 != nil || err2 != nil {
		return nil
	}
	n := int(b.Sub(a).Hours() / 24)
	return &n
}

// observations holds the streaming evidence of a set of movies, keyed by
// "tmdb_id|country".
type observations struct {
	offers  map[string][]repository.StreamingOffer // first arrivals
	current map[string][]repository.StreamingOffer
	charts  map[string][]repository.FirstChart
}

func (s *ReleaseWindowsService) observe(ctx context.Context, country string, ids []string) (*observations, error) {
	o := &observations{offers: map[string][]repository.StreamingOffer{}, current: map[string][]repository.StreamingOffer{},
		charts: map[string][]repository.FirstChart{}}
	if len(ids) == 0 {
		return o, nil
	}
	offers, err := s.repo.FirstStreamingOffers(ctx, country, ids)
	if err != nil {
		return nil, err
	}
	for _, x := range offers {
		o.offers[x.TmdbID+"|"+x.Country] = append(o.offers[x.TmdbID+"|"+x.Country], x)
	}
	current, err := s.repo.CurrentStreamingOffers(ctx, country, ids)
	if err != nil {
		return nil, err
	}
	for _, x := range current {
		o.current[x.TmdbID+"|"+x.Country] = append(o.current[x.TmdbID+"|"+x.Country], x)
	}
	charts, err := s.repo.FirstMovieCharts(ctx, country, ids)
	if err != nil {
		return nil, err
	}
	for _, x := range charts {
		o.charts[x.TmdbID+"|"+x.Country] = append(o.charts[x.TmdbID+"|"+x.Country], x)
	}
	return o, nil
}

// fill adds a country's streaming evidence and windows.
func (o *observations) fill(tmdbID string, c *CountryReleases) {
	key := tmdbID + "|" + c.Country
	start, basis := c.theatricalStart()
	consider := func(a StreamingArrival) {
		if start != "" && a.Date < start {
			return // before the theatrical release: not this release's arrival
		}
		if c.Streaming == nil || a.Date < c.Streaming.Date {
			v := a
			c.Streaming = &v
		}
	}
	for _, x := range o.offers[key] {
		id := x.ProviderID
		consider(StreamingArrival{Date: x.FirstSeen, Basis: "offer_added", Provider: x.ProviderName, ProviderID: &id, Monetization: x.Monetization})
	}
	for _, x := range o.charts[key] {
		// The chart was stored on most of the 14 days before (counted in SQL).
		if float64(x.StoredBefore) >= releaseChartMinCoverage*releaseChartHistoryDays {
			consider(StreamingArrival{Date: x.Date, Basis: "first_chart", Provider: x.Provider})
		}
	}
	c.StreamingNow = []StreamingOfferRef{}
	for _, x := range o.current[key] {
		c.StreamingNow = append(c.StreamingNow, StreamingOfferRef{ProviderID: x.ProviderID, ProviderName: x.ProviderName, Monetization: x.Monetization})
	}
	if start != "" {
		b := basis
		c.Windows.TheatricalBasis = &b
		if c.Digital != nil {
			c.Windows.TheatricalToDigital = windowDays(start, *c.Digital)
		}
		if c.Streaming != nil {
			c.Windows.TheatricalToStreaming = windowDays(start, c.Streaming.Date)
		}
	}
	if c.Digital != nil && c.Streaming != nil {
		c.Windows.DigitalToStreaming = windowDays(*c.Digital, c.Streaming.Date)
	}
}

// --- one movie ----------------------------------------------------------------

type TitleReleasesResponse struct {
	Kind        string            `json:"kind"`
	TmdbID      string            `json:"tmdb_id"`
	Title       *string           `json:"title"`
	ReleaseDate *string           `json:"release_date"` // TMDB's primary release date
	Countries   []CountryReleases `json:"countries"`
}

// TitleReleases returns a movie's releases and windows per country
// (country "" = every country).
func (s *ReleaseWindowsService) TitleReleases(ctx context.Context, ref TmdbRef, country string) (*TitleReleasesResponse, error) {
	if ref.Kind != "movie" {
		return nil, &ValidationError{Message: "release dates exist only for movies (TMDB has none for TV)"}
	}
	country, err := validCountry(country, false)
	if err != nil {
		return nil, err
	}
	meta, err := s.metadata.Get(ctx, ref)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, ErrTitleNotFound
	}
	dates, err := s.repo.MovieReleaseDates(ctx, ref.ID)
	if err != nil {
		return nil, err
	}
	obs, err := s.observe(ctx, country, []string{ref.ID})
	if err != nil {
		return nil, err
	}
	byCountry := map[string]*CountryReleases{}
	get := func(c string) *CountryReleases {
		if byCountry[c] == nil {
			byCountry[c] = &CountryReleases{Country: c}
		}
		return byCountry[c]
	}
	for _, d := range dates {
		if country == "" || d.Country == country {
			get(d.Country).set(d)
		}
	}
	for key := range obs.offers {
		get(key[len(ref.ID)+1:])
	}
	for key := range obs.current {
		get(key[len(ref.ID)+1:])
	}
	for key := range obs.charts {
		get(key[len(ref.ID)+1:])
	}
	out := &TitleReleasesResponse{Kind: ref.Kind, TmdbID: ref.ID, Title: meta.Title, ReleaseDate: meta.ReleaseDate, Countries: []CountryReleases{}}
	for _, c := range byCountry {
		obs.fill(ref.ID, c)
		out.Countries = append(out.Countries, *c)
	}
	sort.Slice(out.Countries, func(i, j int) bool { return out.Countries[i].Country < out.Countries[j].Country })
	return out, nil
}

// --- windows analytic ---------------------------------------------------------

type ReleaseWindowsQuery struct {
	From, To *time.Time
	Country  string // default US
	Limit    int    // items, default and max 500
}

// WindowStats sums up one kind of window.
type WindowStats struct {
	LagSummary
	// WithinDays: how many of the movies with this window had it at most N
	// days long (N = 17, 31, 45, 90).
	WithinDays map[string]int `json:"within_days"`
	// Missing: movies in the range without the later date (not released
	// that way yet, or not known).
	Missing int `json:"missing"`
}

type ProviderWindow struct {
	Provider string     `json:"provider"`
	Basis    string     `json:"basis"`
	Lag      LagSummary `json:"lag"`
}

type ReleaseWindowItem struct {
	TmdbID     string  `json:"tmdb_id"`
	Title      string  `json:"title"`
	PosterPath *string `json:"poster_path"`
	CountryReleases
}

type ReleaseWindowsResponse struct {
	Country string `json:"country"`
	From    string `json:"from"`
	To      string `json:"to"`
	// Movies: movies metareel knows with a theatrical release (wide, else
	// limited) in the country in the range.
	Movies                int                 `json:"movies"`
	TheatricalToDigital   WindowStats         `json:"theatrical_to_digital"`
	TheatricalToStreaming WindowStats         `json:"theatrical_to_streaming"`
	DigitalToStreaming    WindowStats         `json:"digital_to_streaming"`
	StreamingByProvider   []ProviderWindow    `json:"streaming_by_provider"` // theatrical to streaming, per first service
	ItemsTotal            int                 `json:"items_total"`
	Items                 []ReleaseWindowItem `json:"items"` // newest theatrical release first
}

// Windows measures release windows of movies released in theaters in a
// country between two dates.
func (s *ReleaseWindowsService) Windows(ctx context.Context, q ReleaseWindowsQuery) (*ReleaseWindowsResponse, error) {
	if q.Country == "" {
		q.Country = "US"
	}
	country, err := validCountry(q.Country, true)
	if err != nil {
		return nil, err
	}
	from, to, err := s.charts.dateRange(q.From, q.To, releaseWindowsDefaultDays, maxAnalyticsDays)
	if err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit < 0 || limit > releaseWindowsMaxItems {
		return nil, &ValidationError{Message: fmt.Sprintf("limit must be 1-%d", releaseWindowsMaxItems)}
	}
	if limit == 0 {
		limit = releaseWindowsMaxItems
	}
	fromS, toS := repository.FormatDate(from), repository.FormatDate(to)
	rows, err := s.repo.TheatricalReleaseDates(ctx, country, fromS, toS)
	if err != nil {
		return nil, err
	}
	items := map[string]*ReleaseWindowItem{}
	var ids []string
	for _, r := range rows {
		it := items[r.TmdbID]
		if it == nil {
			it = &ReleaseWindowItem{TmdbID: r.TmdbID, Title: r.Title, CountryReleases: CountryReleases{Country: country}}
			if r.PosterPath != "" {
				p := r.PosterPath
				it.PosterPath = &p
			}
			items[r.TmdbID] = it
			ids = append(ids, r.TmdbID)
		}
		it.set(r)
	}
	obs, err := s.observe(ctx, country, ids)
	if err != nil {
		return nil, err
	}
	out := &ReleaseWindowsResponse{Country: country, From: fromS, To: toS, StreamingByProvider: []ProviderWindow{}, Items: []ReleaseWindowItem{}}
	var toDigital, toStreaming, digitalToStreaming []float64
	type providerKey struct{ provider, basis string }
	byProvider := map[providerKey][]float64{}
	for _, id := range ids {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		it := items[id]
		start, _ := it.theatricalStart()
		if start < fromS || start > toS {
			continue // the wide date is outside the range (only the limited one was in it)
		}
		obs.fill(id, &it.CountryReleases)
		out.Movies++
		w := it.Windows
		if w.TheatricalToDigital != nil {
			toDigital = append(toDigital, float64(*w.TheatricalToDigital))
		} else {
			out.TheatricalToDigital.Missing++
		}
		if w.TheatricalToStreaming != nil {
			toStreaming = append(toStreaming, float64(*w.TheatricalToStreaming))
			pk := providerKey{it.Streaming.Provider, it.Streaming.Basis}
			byProvider[pk] = append(byProvider[pk], float64(*w.TheatricalToStreaming))
		} else {
			out.TheatricalToStreaming.Missing++
		}
		if w.DigitalToStreaming != nil {
			digitalToStreaming = append(digitalToStreaming, float64(*w.DigitalToStreaming))
		} else if it.Digital != nil {
			out.DigitalToStreaming.Missing++
		}
		out.Items = append(out.Items, *it)
	}
	out.TheatricalToDigital.fill(toDigital)
	out.TheatricalToStreaming.fill(toStreaming)
	out.DigitalToStreaming.fill(digitalToStreaming)
	for pk, v := range byProvider {
		out.StreamingByProvider = append(out.StreamingByProvider, ProviderWindow{Provider: pk.provider, Basis: pk.basis, Lag: lagSummary(v)})
	}
	sort.Slice(out.StreamingByProvider, func(i, j int) bool {
		a, b := out.StreamingByProvider[i], out.StreamingByProvider[j]
		if a.Lag.Titles != b.Lag.Titles {
			return a.Lag.Titles > b.Lag.Titles
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		return a.Basis < b.Basis
	})
	sort.SliceStable(out.Items, func(i, j int) bool {
		a, _ := out.Items[i].theatricalStart()
		b, _ := out.Items[j].theatricalStart()
		if a != b {
			return a > b
		}
		return out.Items[i].TmdbID < out.Items[j].TmdbID
	})
	out.ItemsTotal = len(out.Items)
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
	}
	return out, nil
}

func (w *WindowStats) fill(v []float64) {
	w.WithinDays = map[string]int{}
	for _, t := range WindowThresholds {
		n := 0
		for _, x := range v {
			if x <= float64(t) {
				n++
			}
		}
		w.WithinDays[strconv.Itoa(t)] = n
	}
	w.LagSummary = lagSummary(v)
}
