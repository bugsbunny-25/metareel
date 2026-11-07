package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// Discovery for tides, per country, over the titles metareel knows
// (charting and Netflix Top 10 titles):
//   - hidden gems: highly rated (IMDb's dataset) titles streamable in the
//     country that were not in its Top 10 lately;
//   - elsewhere: titles charting in other countries that can't be streamed
//     in this one;
//   - rotation: which streaming services are worth paying for this month,
//     from what charts, arrives and airs on each.

// DiscoverService serves the discovery endpoints.
type DiscoverService struct {
	repo      *repository.DiscoverRepository
	analytics *repository.AnalyticsRepository
	episodes  *repository.EpisodesRepository
	metadata  *repository.MetadataRepository
	now       func() time.Time
}

func NewDiscoverService(repo *repository.DiscoverRepository, analytics *repository.AnalyticsRepository, episodes *repository.EpisodesRepository, metadata *repository.MetadataRepository) *DiscoverService {
	return &DiscoverService{repo: repo, analytics: analytics, episodes: episodes, metadata: metadata, now: time.Now}
}

// DiscoverOffer is a title's offer in the country.
type DiscoverOffer struct {
	ProviderID   int64  `json:"provider_id"`
	ProviderName string `json:"provider_name"`
	Monetization string `json:"monetization"`
}

// DiscoverTitle identifies a title in discovery results.
type DiscoverTitle struct {
	Kind       string   `json:"kind"` // movie | tv
	TmdbID     string   `json:"tmdb_id"`
	Title      string   `json:"title"`
	Year       *int     `json:"year"`
	PosterPath *string  `json:"poster_path"`
	IMDbRating *float64 `json:"imdb_rating"`
	IMDbVotes  *int64   `json:"imdb_votes"`
}

func isStreaming(monetization string) bool {
	return monetization == "flatrate" || monetization == "free" || monetization == "ads"
}

func intRange(name string, v, def, lo, hi int) (int, error) {
	if v == 0 {
		return def, nil
	}
	if v < lo || v > hi {
		return 0, &ValidationError{Message: fmt.Sprintf("%s must be %d-%d", name, lo, hi)}
	}
	return v, nil
}

func validTmdbKind(kind string) (string, error) {
	switch kind {
	case "", "movie", "tv":
		return kind, nil
	}
	return "", &ValidationError{Message: "invalid kind, expected movie or tv"}
}

// chartPoints sums chart points per TMDB title and country over a window.
type countryPoints struct {
	points    int64
	bestRank  int64
	providers map[string]bool
}

func (s *DiscoverService) chartPoints(ctx context.Context, from, to time.Time, country string) (map[string]map[string]*countryPoints, map[string]string, map[string]bool, error) {
	rows, err := s.analytics.ChartEntries(ctx, repository.ChartEntryFilter{From: from, To: to, Country: country})
	if err != nil {
		return nil, nil, nil, err
	}
	out := map[string]map[string]*countryPoints{}
	names := map[string]string{}
	countries := map[string]bool{}
	for _, r := range rows {
		countries[r.Country] = true
		if r.TmdbID == nil || *r.TmdbID == "" {
			continue
		}
		key := tmdbKindOf(flixpatrol.TitleKind(r.Kind)) + ":" + *r.TmdbID
		if out[key] == nil {
			out[key] = map[string]*countryPoints{}
		}
		cp := out[key][r.Country]
		if cp == nil {
			cp = &countryPoints{bestRank: r.Rank, providers: map[string]bool{}}
			out[key][r.Country] = cp
		}
		cp.points += Points(r.Rank)
		cp.bestRank = min(cp.bestRank, r.Rank)
		cp.providers[r.Provider] = true
		names[key] = r.Name
	}
	return out, names, countries, nil
}

func yearPtr(date string) *int {
	if y := dateYear(date); y > 0 {
		return &y
	}
	return nil
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// --- hidden gems ------------------------------------------------------------------

type HiddenGemsQuery struct {
	Country    string
	Kind       string // movie | tv | ""
	ProviderID int64  // only this TMDB provider (0 = any)
	MinRating  float64
	MinVotes   int64
	Days       int
	Limit      int
}

type HiddenGem struct {
	DiscoverTitle
	Genres []string `json:"genres"`
	// Score: the IMDb rating weighted by votes (a Bayesian average toward
	// 6.5 over 25,000 votes), so a 9.0 from 300 votes doesn't top a 8.4
	// from 200,000.
	Score           float64         `json:"score"`
	StreamingHere   []DiscoverOffer `json:"streaming_here"`
	PointsElsewhere int64           `json:"points_elsewhere"` // Top 10 points in other countries over the window
	ChartedIn       []string        `json:"charted_in"`       // other countries it charted in over the window
}

type HiddenGemsResponse struct {
	Country    string      `json:"country"`
	From       string      `json:"from"`
	To         string      `json:"to"`
	Kind       *string     `json:"kind"`
	ProviderID *int64      `json:"provider_id"`
	MinRating  float64     `json:"min_rating"`
	MinVotes   int64       `json:"min_votes"`
	Considered int         `json:"considered"` // rated and streamable in the country
	Total      int         `json:"total"`
	Items      []HiddenGem `json:"items"`
}

const (
	gemPriorVotes  = 25000
	gemPriorRating = 6.5
)

// HiddenGems lists highly rated titles streamable in a country (on a
// subscription, free or with ads) that were not in its Top 10 charts over
// the window, best first.
func (s *DiscoverService) HiddenGems(ctx context.Context, q HiddenGemsQuery) (*HiddenGemsResponse, error) {
	country, err := validCountry(q.Country, true)
	if err != nil {
		return nil, err
	}
	kind, err := validTmdbKind(q.Kind)
	if err != nil {
		return nil, err
	}
	days, err := intRange("days", q.Days, 90, 7, 180)
	if err != nil {
		return nil, err
	}
	limit, err := intRange("limit", q.Limit, 50, 1, 200)
	if err != nil {
		return nil, err
	}
	minRating := q.MinRating
	if minRating == 0 {
		minRating = 7.5
	}
	if !(minRating >= 1 && minRating <= 10) { // also rejects NaN
		return nil, &ValidationError{Message: "min_rating must be 1-10"}
	}
	minVotes := q.MinVotes
	if minVotes == 0 {
		minVotes = 5000
	}
	if minVotes < 0 || minVotes > 10_000_000 || q.ProviderID < 0 {
		return nil, &ValidationError{Message: "min_votes must be 0-10000000 and provider_id positive"}
	}
	to := flixpatrol.EffectiveTop10Date(s.now())
	from := to.AddDate(0, 0, -(days - 1))
	rated, err := s.repo.IMDbRatedTitles(ctx)
	if err != nil {
		return nil, err
	}
	offers, err := s.repo.CountryOffers(ctx, country)
	if err != nil {
		return nil, err
	}
	streaming := map[string][]DiscoverOffer{}
	for _, o := range offers {
		if isStreaming(o.Monetization) {
			streaming[o.Ref.String()] = append(streaming[o.Ref.String()], DiscoverOffer{ProviderID: o.ProviderID, ProviderName: o.ProviderName, Monetization: o.Monetization})
		}
	}
	points, _, _, err := s.chartPoints(ctx, from, to, "")
	if err != nil {
		return nil, err
	}
	out := &HiddenGemsResponse{Country: country, From: repository.FormatDate(from), To: repository.FormatDate(to), Kind: optStr(kind),
		MinRating: minRating, MinVotes: minVotes, Items: []HiddenGem{}}
	if q.ProviderID > 0 {
		p := q.ProviderID
		out.ProviderID = &p
	}
	for _, t := range rated {
		key := t.Ref.String()
		here := streaming[key]
		if (kind != "" && t.Ref.Kind != kind) || t.Rating < minRating || t.Votes < minVotes || len(here) == 0 {
			continue
		}
		if q.ProviderID > 0 && !hasProvider(here, q.ProviderID) {
			continue
		}
		out.Considered++
		byCountry := points[key]
		if byCountry[country] != nil {
			continue // charted here: not hidden
		}
		g := HiddenGem{DiscoverTitle: discoverTitle(t), Genres: t.Genres, StreamingHere: here, ChartedIn: []string{}}
		if g.Genres == nil {
			g.Genres = []string{}
		}
		for c, cp := range byCountry {
			g.PointsElsewhere += cp.points
			g.ChartedIn = append(g.ChartedIn, c)
		}
		sort.Strings(g.ChartedIn)
		v := float64(t.Votes)
		g.Score = round(v/(v+gemPriorVotes)*t.Rating+gemPriorVotes/(v+gemPriorVotes)*gemPriorRating, 2)
		out.Items = append(out.Items, g)
	}
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		return a.Kind+a.TmdbID < b.Kind+b.TmdbID
	})
	out.Total = len(out.Items)
	out.Items = out.Items[:min(len(out.Items), limit)]
	return out, nil
}

func hasProvider(offers []DiscoverOffer, id int64) bool {
	for _, o := range offers {
		if o.ProviderID == id {
			return true
		}
	}
	return false
}

func discoverTitle(t repository.RatedTitle) DiscoverTitle {
	r, v := t.Rating, t.Votes
	return DiscoverTitle{Kind: t.Ref.Kind, TmdbID: t.Ref.ID, Title: t.Title, Year: yearPtr(t.ReleaseDate), PosterPath: strPtrOrNil(t.PosterPath),
		IMDbRating: &r, IMDbVotes: &v}
}

// --- trending elsewhere, not streamable here ------------------------------------

type ElsewhereQuery struct {
	Country string
	Kind    string
	Days    int
	Limit   int
}

type ElsewhereCountry struct {
	Country   string   `json:"country"`
	Points    int64    `json:"points"`
	BestRank  int64    `json:"best_rank"`
	Providers []string `json:"providers"`
}

type ElsewhereItem struct {
	DiscoverTitle
	Points      int64              `json:"points"` // Top 10 points over the window in the other countries
	Countries   []ElsewhereCountry `json:"countries"`
	RentBuyHere []DiscoverOffer    `json:"rent_buy_here"` // it can still be rented or bought in the country
}

type ElsewhereResponse struct {
	Country string  `json:"country"`
	From    string  `json:"from"`
	To      string  `json:"to"`
	Kind    *string `json:"kind"`
	// CountriesChecked: other countries with charts in the window.
	CountriesChecked []string `json:"countries_checked"`
	// Skipped: charting elsewhere but streamable here, charting here too,
	// or with no TMDB offers known yet (details not fetched).
	Skipped map[string]int  `json:"skipped"`
	Total   int             `json:"total"`
	Items   []ElsewhereItem `json:"items"`
}

// Elsewhere lists titles in other countries' Top 10s over the window that
// have no subscription, free or ads offer in the country (TMDB's offers),
// the most charted first.
func (s *DiscoverService) Elsewhere(ctx context.Context, q ElsewhereQuery) (*ElsewhereResponse, error) {
	country, err := validCountry(q.Country, true)
	if err != nil {
		return nil, err
	}
	kind, err := validTmdbKind(q.Kind)
	if err != nil {
		return nil, err
	}
	days, err := intRange("days", q.Days, 7, 1, 30)
	if err != nil {
		return nil, err
	}
	limit, err := intRange("limit", q.Limit, 50, 1, 200)
	if err != nil {
		return nil, err
	}
	to := flixpatrol.EffectiveTop10Date(s.now())
	from := to.AddDate(0, 0, -(days - 1))
	points, names, countries, err := s.chartPoints(ctx, from, to, "")
	if err != nil {
		return nil, err
	}
	delete(countries, country)
	out := &ElsewhereResponse{Country: country, From: repository.FormatDate(from), To: repository.FormatDate(to), Kind: optStr(kind),
		CountriesChecked: sortedKeys(countries), Skipped: map[string]int{"available_here": 0, "charting_here": 0, "availability_unknown": 0},
		Items: []ElsewhereItem{}}
	var refs []repository.TmdbRef
	for key, byCountry := range points {
		k, id, _ := strings.Cut(key, ":")
		if kind != "" && k != kind {
			continue
		}
		if byCountry[country] != nil {
			out.Skipped["charting_here"]++
			continue
		}
		refs = append(refs, repository.TmdbRef{Kind: k, ID: id})
	}
	meta, err := s.metadata.GetMany(ctx, refs)
	if err != nil {
		return nil, err
	}
	offers, err := s.repo.CountryOffers(ctx, country)
	if err != nil {
		return nil, err
	}
	streamable := map[string]bool{}
	rentBuy := map[string][]DiscoverOffer{}
	for _, o := range offers {
		key := o.Ref.String()
		if isStreaming(o.Monetization) {
			streamable[key] = true
		} else {
			rentBuy[key] = append(rentBuy[key], DiscoverOffer{ProviderID: o.ProviderID, ProviderName: o.ProviderName, Monetization: o.Monetization})
		}
	}
	ratings, err := s.ratingsByRef(ctx)
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		key := ref.String()
		m := meta[key]
		if m == nil || m.DetailsFetchedAt == nil {
			out.Skipped["availability_unknown"]++
			continue
		}
		if streamable[key] {
			out.Skipped["available_here"]++
			continue
		}
		it := ElsewhereItem{DiscoverTitle: DiscoverTitle{Kind: ref.Kind, TmdbID: ref.ID, Title: names[key], PosterPath: m.PosterPath},
			RentBuyHere: rentBuy[key], Countries: []ElsewhereCountry{}}
		if m.Title != nil && *m.Title != "" {
			it.Title = *m.Title
		}
		if m.ReleaseDate != nil {
			it.Year = yearPtr(*m.ReleaseDate)
		}
		if r, ok := ratings[key]; ok {
			it.IMDbRating, it.IMDbVotes = &r.Rating, &r.Votes
		}
		if it.RentBuyHere == nil {
			it.RentBuyHere = []DiscoverOffer{}
		}
		for c, cp := range points[key] {
			it.Points += cp.points
			it.Countries = append(it.Countries, ElsewhereCountry{Country: c, Points: cp.points, BestRank: cp.bestRank, Providers: sortedKeys(cp.providers)})
		}
		sort.Slice(it.Countries, func(i, j int) bool { return it.Countries[i].Points > it.Countries[j].Points })
		out.Items = append(out.Items, it)
	}
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if a.Points != b.Points {
			return a.Points > b.Points
		}
		return a.Kind+a.TmdbID < b.Kind+b.TmdbID
	})
	out.Total = len(out.Items)
	out.Items = out.Items[:min(len(out.Items), limit)]
	return out, nil
}

func (s *DiscoverService) ratingsByRef(ctx context.Context) (map[string]repository.RatedTitle, error) {
	rated, err := s.repo.IMDbRatedTitles(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]repository.RatedTitle, len(rated))
	for _, t := range rated {
		out[t.Ref.String()] = t
	}
	return out, nil
}

// --- subscription rotation --------------------------------------------------------

type RotationQuery struct {
	Country    string
	Days       int
	Subscribed []int64 // TMDB provider IDs the viewer pays for now
	Slots      int     // how many services to pay for
	MinTitles  int
	// IncludeOther adds providers outside the main streaming services
	// (live-TV bundles, niche services), each on its own.
	IncludeOther bool
}

// rotationFamily groups a TMDB provider's plans and channels (e.g. Netflix
// Standard with Ads, HBO Max Amazon Channel) under its main service, using
// the FlixPatrol services' TMDB provider IDs. ok is false for other
// providers.
func rotationFamily(providerID int64) (id int64, name string, ok bool) {
	for slug, ids := range providerOffers {
		for _, pid := range ids {
			if pid == providerID {
				return rotationMainID[slug], providerNames[slug], true
			}
		}
	}
	return providerID, "", false
}

// rotationMainID is each service's subscription provider ID on TMDB.
var rotationMainID = map[string]int64{
	"netflix": 8, "amazon-prime": 9, "apple-tv": 350, "disney": 337, "hbo-max": 1899,
	"paramount-plus": 531, "peacock": 386, "hulu": 15,
}

type RotationTitle struct {
	Kind   string `json:"kind"`
	TmdbID string `json:"tmdb_id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

type RotationService struct {
	ProviderID   int64  `json:"provider_id"` // the service's main TMDB provider ID
	ProviderName string `json:"provider_name"`
	// ProviderIDs: every TMDB provider (plans, channels) counted in it.
	ProviderIDs []int64 `json:"provider_ids"`
	// Titles: titles metareel knows with a subscription or ads offer here.
	Titles int `json:"titles"`
	// ChartPoints: Top 10 points its titles earned over the past window
	// (in the country, or every country when the country has no charts).
	ChartPoints  int64 `json:"chart_points"`
	Charting     int   `json:"charting"`       // of its titles, how many charted
	Arrivals     int   `json:"arrivals"`       // titles that arrived over the past window
	Departures   int   `json:"departures"`     // titles that left over the past window
	UpcomingShow int   `json:"upcoming_shows"` // shows with episodes in the coming window
	TopRated     int   `json:"top_rated"`      // titles rated 7.5+ on IMDb (5,000+ votes)
	// Score = chart points + 10 per show with episodes coming + 3 per
	// arrival + 1 per top-rated title.
	Score          int64           `json:"score"`
	Recommendation string          `json:"recommendation"` // subscribe | skip, or with subscribed: keep | pause | add | skip
	Highlights     []RotationTitle `json:"highlights"`
}

type RotationResponse struct {
	Country string `json:"country"`
	// PastFrom..PastTo: the window for charts and offer changes;
	// UpcomingTo: the end of the coming window for episodes.
	PastFrom   string            `json:"past_from"`
	PastTo     string            `json:"past_to"`
	UpcomingTo string            `json:"upcoming_to"`
	ChartBasis string            `json:"chart_basis"` // country | all_countries
	Slots      int               `json:"slots"`
	Subscribed []int64           `json:"subscribed"`
	Services   []RotationService `json:"services"`
}

// Rotation ranks a country's subscription services by what charts, arrives
// and airs on each, and recommends which to pay for in the coming window.
func (s *DiscoverService) Rotation(ctx context.Context, q RotationQuery) (*RotationResponse, error) {
	country, err := validCountry(q.Country, true)
	if err != nil {
		return nil, err
	}
	days, err := intRange("days", q.Days, 30, 7, 90)
	if err != nil {
		return nil, err
	}
	slots, err := intRange("slots", q.Slots, 2, 1, 10)
	if err != nil {
		return nil, err
	}
	minTitles, err := intRange("min_titles", q.MinTitles, 10, 1, 10000)
	if err != nil {
		return nil, err
	}
	if len(q.Subscribed) > 20 {
		return nil, &ValidationError{Message: "at most 20 subscribed services"}
	}
	today := s.now().UTC().Truncate(24 * time.Hour)
	to := flixpatrol.EffectiveTop10Date(s.now())
	from := to.AddDate(0, 0, -(days - 1))
	upcomingTo := today.AddDate(0, 0, days-1)

	offers, err := s.repo.CountryOffers(ctx, country)
	if err != nil {
		return nil, err
	}
	type svc struct {
		RotationService
		titles map[string]bool
	}
	services := map[int64]*svc{}
	family := func(providerID int64, name string) (int64, string, bool) {
		id, famName, ok := rotationFamily(providerID)
		if !ok {
			return providerID, name, q.IncludeOther
		}
		return id, famName, true
	}
	for _, o := range offers {
		if o.Monetization != "flatrate" && o.Monetization != "ads" {
			continue // free services need no subscription; rent / buy are per title
		}
		id, name, ok := family(o.ProviderID, o.ProviderName)
		if !ok {
			continue
		}
		sv := services[id]
		if sv == nil {
			sv = &svc{RotationService: RotationService{ProviderID: id, ProviderName: name, ProviderIDs: []int64{}}, titles: map[string]bool{}}
			services[id] = sv
		}
		if !containsInt64(sv.ProviderIDs, o.ProviderID) {
			sv.ProviderIDs = append(sv.ProviderIDs, o.ProviderID)
		}
		sv.titles[o.Ref.String()] = true
	}
	// The country's charts; every country's only when it has none.
	basis := "country"
	points, names, countries, err := s.chartPoints(ctx, from, to, country)
	if err != nil {
		return nil, err
	}
	if !countries[country] {
		basis = "all_countries"
		if points, names, _, err = s.chartPoints(ctx, from, to, ""); err != nil {
			return nil, err
		}
	}
	titlePoints := func(key string) int64 {
		var n int64
		for c, cp := range points[key] {
			if basis == "all_countries" || c == country {
				n += cp.points
			}
		}
		return n
	}
	ratings, err := s.ratingsByRef(ctx)
	if err != nil {
		return nil, err
	}
	changes, err := s.repo.CountryOfferChanges(ctx, country, repository.FormatDate(from)+" 00:00:00")
	if err != nil {
		return nil, err
	}
	// A title arriving on two plans of one service counts once.
	arrived := map[int64][]repository.OfferChange{}
	seenChange := map[string]bool{}
	for _, ch := range changes {
		if ch.Monetization != "flatrate" && ch.Monetization != "ads" {
			continue
		}
		id, _, ok := family(ch.ProviderID, ch.ProviderName)
		sv := services[id]
		if !ok || sv == nil {
			continue
		}
		k := fmt.Sprintf("%d|%s|%s", id, ch.Ref.String(), ch.Event)
		if seenChange[k] {
			continue
		}
		seenChange[k] = true
		if ch.Event == "added" {
			sv.Arrivals++
			arrived[id] = append(arrived[id], ch)
		} else {
			sv.Departures++
		}
	}
	upcoming := map[int64]map[string]string{} // provider -> show tmdb id -> title
	if s.episodes != nil {
		eps, err := s.episodes.CalendarEpisodes(ctx, country, repository.FormatDate(today), repository.FormatDate(upcomingTo), 0)
		if err != nil {
			return nil, err
		}
		if len(eps) > 0 {
			calOffers, err := s.episodes.CalendarOffers(ctx, country, repository.FormatDate(today), repository.FormatDate(upcomingTo))
			if err != nil {
				return nil, err
			}
			for _, e := range eps {
				for _, o := range calOffers[e.TmdbID] {
					if o.Monetization != "flatrate" && o.Monetization != "ads" {
						continue
					}
					id, _, ok := family(o.ProviderID, o.ProviderName)
					if !ok {
						continue
					}
					if upcoming[id] == nil {
						upcoming[id] = map[string]string{}
					}
					upcoming[id][e.TmdbID] = e.Title
				}
			}
		}
	}
	subscribed := map[int64]bool{}
	for _, pid := range q.Subscribed {
		id, _, _ := rotationFamily(pid) // any plan's ID means the service
		subscribed[id] = true
	}
	out := &RotationResponse{Country: country, PastFrom: repository.FormatDate(from), PastTo: repository.FormatDate(to),
		UpcomingTo: repository.FormatDate(upcomingTo), ChartBasis: basis, Slots: slots, Subscribed: q.Subscribed, Services: []RotationService{}}
	if out.Subscribed == nil {
		out.Subscribed = []int64{}
	}
	var list []*svc
	for id, sv := range services {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		sv.Titles = len(sv.titles)
		if sv.Titles < minTitles && !subscribed[id] {
			continue
		}
		type charted struct {
			key    string
			points int64
		}
		var top []charted
		for key := range sv.titles {
			if p := titlePoints(key); p > 0 {
				sv.ChartPoints += p
				sv.Charting++
				top = append(top, charted{key, p})
			}
			if r, ok := ratings[key]; ok && r.Rating >= 7.5 && r.Votes >= 5000 {
				sv.TopRated++
			}
		}
		sv.UpcomingShow = len(upcoming[id])
		sv.Score = sv.ChartPoints + 10*int64(sv.UpcomingShow) + 3*int64(sv.Arrivals) + int64(sv.TopRated)
		sort.Slice(top, func(i, j int) bool {
			return top[i].points > top[j].points || (top[i].points == top[j].points && top[i].key < top[j].key)
		})
		sv.Highlights = []RotationTitle{}
		for _, c := range top[:min(len(top), 3)] {
			k, tid, _ := strings.Cut(c.key, ":")
			sv.Highlights = append(sv.Highlights, RotationTitle{Kind: k, TmdbID: tid, Title: titleName(c.key, names, ratings), Detail: fmt.Sprintf("%d chart points", c.points)})
		}
		showIDs := make([]string, 0, len(upcoming[id]))
		for tid := range upcoming[id] {
			showIDs = append(showIDs, tid)
		}
		sort.Strings(showIDs)
		for _, tid := range showIDs[:min(len(showIDs), 2)] {
			sv.Highlights = append(sv.Highlights, RotationTitle{Kind: "tv", TmdbID: tid, Title: upcoming[id][tid], Detail: "new episodes coming"})
		}
		for _, ch := range arrived[id][:min(len(arrived[id]), 2)] {
			sv.Highlights = append(sv.Highlights, RotationTitle{Kind: ch.Ref.Kind, TmdbID: ch.Ref.ID, Title: ch.Title, Detail: "arrived " + ch.ObservedAt[:min(10, len(ch.ObservedAt))]})
		}
		list = append(list, sv)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Score != list[j].Score {
			return list[i].Score > list[j].Score
		}
		return list[i].ProviderID < list[j].ProviderID
	})
	for i, sv := range list {
		inTop := i < slots && sv.Score > 0
		switch {
		case len(subscribed) == 0 && inTop:
			sv.Recommendation = "subscribe"
		case len(subscribed) == 0:
			sv.Recommendation = "skip"
		case subscribed[sv.ProviderID] && inTop:
			sv.Recommendation = "keep"
		case subscribed[sv.ProviderID]:
			sv.Recommendation = "pause"
		case inTop:
			sv.Recommendation = "add"
		default:
			sv.Recommendation = "skip"
		}
		out.Services = append(out.Services, sv.RotationService)
	}
	return out, nil
}

func titleName(key string, names map[string]string, ratings map[string]repository.RatedTitle) string {
	if r, ok := ratings[key]; ok && r.Title != "" {
		return r.Title
	}
	return names[key]
}

func containsInt64(list []int64, v int64) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
