package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// ErrUpstream wraps JustWatch failures.
var ErrUpstream = errors.New("justwatch request failed")

const (
	releasesPageSize = 100
	// releasesMaxItems caps one day's "new" list or the "upcoming" list.
	releasesMaxItems   = 1000
	releasesDefaultNew = 7  // days of "new" titles when from/to are omitted
	releasesMaxNewDays = 31 // widest "new" range: one JustWatch query per day
	packagesCacheTTL   = 24 * time.Hour
	justWatchWebURL    = "https://www.justwatch.com"
	justWatchImagesURL = "https://images.justwatch.com"
)

type justWatchReleases interface {
	GetPackages(ctx context.Context, country string) ([]client.JWPackage, error)
	GetNewTitles(ctx context.Context, pageType client.NewPageType, country, language, date, pack string, objectTypes []client.ObjectType, after string, first int) (*client.NewTitlesPage, error)
}

// ReleasesService lists titles new on, or coming to, a streaming service,
// straight from JustWatch. Nothing is stored; only JustWatch's per-country
// service list is cached in memory.
type ReleasesService struct {
	log *slog.Logger
	jw  justWatchReleases
	now func() time.Time

	mu       sync.Mutex
	packages map[string]packagesCacheEntry // by country
}

type packagesCacheEntry struct {
	packages  []client.JWPackage
	fetchedAt time.Time
}

func NewReleasesService(log *slog.Logger, jw *client.JustWatchClient) *ReleasesService {
	return &ReleasesService{log: log, jw: jw, now: time.Now, packages: map[string]packagesCacheEntry{}}
}

type ReleasesQuery struct {
	CountryCode string
	Service     string     // FlixPatrol slug (e.g. "netflix") or JustWatch package short / technical name
	From        *time.Time // inclusive
	To          *time.Time // inclusive
	Kind        string     // "", "movie" or "tv"
	Language    string     // default "en"
}

type ReleasesResponse struct {
	Type      string         `json:"type"` // new | upcoming
	Country   string         `json:"country"`
	Service   ReleaseService `json:"service"`
	From      *string        `json:"from"`
	To        *string        `json:"to"`
	Count     int            `json:"count"`
	Truncated bool           `json:"truncated"` // JustWatch had more than releasesMaxItems
	Items     []ReleaseItem  `json:"items"`
}

type ReleaseService struct {
	Requested        string `json:"requested"`
	JustWatchPackage string `json:"justwatch_package"` // e.g. "nfx"
	Name             string `json:"name"`              // e.g. "Netflix"
}

type ReleaseItem struct {
	// Date the title was added to the service ("new"), or its announced
	// release date on the service ("upcoming"; null if not yet dated).
	Date                *string       `json:"date"`
	Kind                string        `json:"kind"`  // movie | tv
	Title               string        `json:"title"` // show title for seasons
	SeasonNumber        *int          `json:"season_number,omitempty"`
	SeasonTitle         *string       `json:"season_title,omitempty"`
	Year                *int          `json:"year"`
	OriginalReleaseDate *string       `json:"original_release_date"`
	TmdbID              *string       `json:"tmdb_id"` // show ID for seasons
	ImdbID              *string       `json:"imdb_id"`
	JustWatchID         string        `json:"justwatch_id"`
	JustWatchURL        *string       `json:"justwatch_url"`
	Description         *string       `json:"description"`
	RuntimeMinutes      *int          `json:"runtime_minutes"`
	Genres              []string      `json:"genres"`
	PosterURL           *string       `json:"poster_url"`
	IMDbRating          *float64      `json:"imdb_rating"`
	IMDbVotes           *int          `json:"imdb_votes"`
	Tomatometer         *int          `json:"tomatometer"`
	CertifiedFresh      *bool         `json:"certified_fresh"`
	Offer               *ReleaseOffer `json:"offer,omitempty"`        // new only
	ReleaseType         *string       `json:"release_type,omitempty"` // upcoming only, e.g. DIGITAL
}

type ReleaseOffer struct {
	URL              *string `json:"url"` // deep link on the service
	MonetizationType *string `json:"monetization_type"`
	PresentationType *string `json:"presentation_type"`
}

type ServicesResponse struct {
	Country  string           `json:"country"`
	Services []ReleaseService `json:"services"`
}

// ListServices lists the streaming services JustWatch knows in a country.
func (s *ReleasesService) ListServices(ctx context.Context, countryCode string) (*ServicesResponse, error) {
	country, err := validateReleaseCountry(countryCode)
	if err != nil {
		return nil, err
	}
	pkgs, err := s.countryPackages(ctx, country)
	if err != nil {
		return nil, err
	}
	out := &ServicesResponse{Country: country, Services: make([]ReleaseService, 0, len(pkgs))}
	for _, p := range pkgs {
		out.Services = append(out.Services, ReleaseService{Requested: p.TechnicalName, JustWatchPackage: p.ShortName, Name: p.ClearName})
	}
	return out, nil
}

// GetNew lists titles added to the service on each day from From to To
// (default: the last 7 days), newest day first.
func (s *ReleasesService) GetNew(ctx context.Context, q ReleasesQuery) (*ReleasesResponse, error) {
	country, pkg, kind, lang, err := s.validate(ctx, q)
	if err != nil {
		return nil, err
	}
	to := s.today()
	if q.To != nil {
		to = q.To.UTC()
	}
	from := to.AddDate(0, 0, -(releasesDefaultNew - 1))
	if q.From != nil {
		from = q.From.UTC()
	}
	if from.After(to) {
		return nil, &ValidationError{Message: "from must be on or before to"}
	}
	days := int(to.Sub(from).Hours()/24) + 1
	if days > releasesMaxNewDays {
		return nil, &ValidationError{Message: fmt.Sprintf("date range too wide: at most %d days", releasesMaxNewDays)}
	}

	perDay := make([][]ReleaseItem, days)
	truncated := make([]bool, days)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	for i := 0; i < days; i++ {
		day := to.AddDate(0, 0, -i).Format("2006-01-02")
		g.Go(func() error {
			edges, trunc, err := s.fetchAll(gctx, client.NewPageTypeNew, country, lang, day, pkg.ShortName)
			if err != nil {
				return err
			}
			for _, e := range edges {
				item := releaseItemFromEdge(e, country)
				if kind != "" && item.Kind != kind {
					continue
				}
				d := day
				item.Date = &d
				if o := e.NewOffer; o != nil {
					item.Offer = &ReleaseOffer{URL: o.StandardWebURL, MonetizationType: o.MonetizationType, PresentationType: o.PresentationType}
				}
				perDay[i] = append(perDay[i], item)
			}
			truncated[i] = trunc
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	out := newReleasesResponse("new", country, q.Service, pkg, &from, &to)
	for i := range perDay {
		out.Items = append(out.Items, perDay[i]...)
		out.Truncated = out.Truncated || truncated[i]
	}
	out.Count = len(out.Items)
	return out, nil
}

// GetUpcoming lists titles announced for the service, soonest first,
// optionally limited to release dates from From to To. Titles without an
// announced date for the service come last, and only when no range is given.
func (s *ReleasesService) GetUpcoming(ctx context.Context, q ReleasesQuery) (*ReleasesResponse, error) {
	country, pkg, kind, lang, err := s.validate(ctx, q)
	if err != nil {
		return nil, err
	}
	if q.From != nil && q.To != nil && q.From.After(*q.To) {
		return nil, &ValidationError{Message: "from must be on or before to"}
	}
	// JustWatch ignores the date for upcoming listings.
	edges, truncated, err := s.fetchAll(ctx, client.NewPageTypeUpcoming, country, lang, s.today().Format("2006-01-02"), pkg.ShortName)
	if err != nil {
		return nil, err
	}

	var fromStr, toStr string
	if q.From != nil {
		fromStr = q.From.UTC().Format("2006-01-02")
	}
	if q.To != nil {
		toStr = q.To.UTC().Format("2006-01-02")
	}
	out := newReleasesResponse("upcoming", country, q.Service, pkg, q.From, q.To)
	out.Truncated = truncated
	for _, e := range edges {
		item := releaseItemFromEdge(e, country)
		if kind != "" && item.Kind != kind {
			continue
		}
		item.Date, item.ReleaseType = serviceReleaseDate(e, pkg.ShortName)
		if fromStr != "" || toStr != "" {
			if item.Date == nil || (fromStr != "" && *item.Date < fromStr) || (toStr != "" && *item.Date > toStr) {
				continue
			}
		}
		out.Items = append(out.Items, item)
	}
	sort.SliceStable(out.Items, func(i, j int) bool {
		a, b := out.Items[i].Date, out.Items[j].Date
		if a == nil || b == nil {
			return a != nil
		}
		return *a < *b
	})
	out.Count = len(out.Items)
	return out, nil
}

func newReleasesResponse(typ, country, requested string, pkg client.JWPackage, from, to *time.Time) *ReleasesResponse {
	out := &ReleasesResponse{
		Type:    typ,
		Country: country,
		Service: ReleaseService{Requested: requested, JustWatchPackage: pkg.ShortName, Name: pkg.ClearName},
		Items:   []ReleaseItem{},
	}
	if from != nil {
		f := from.UTC().Format("2006-01-02")
		out.From = &f
	}
	if to != nil {
		t := to.UTC().Format("2006-01-02")
		out.To = &t
	}
	return out
}

func (s *ReleasesService) validate(ctx context.Context, q ReleasesQuery) (country string, pkg client.JWPackage, kind, lang string, err error) {
	country, err = validateReleaseCountry(q.CountryCode)
	if err != nil {
		return
	}
	switch kind = strings.ToLower(strings.TrimSpace(q.Kind)); kind {
	case "", "movie", "tv":
	default:
		err = &ValidationError{Message: fmt.Sprintf("invalid type %q, expected movie or tv", q.Kind)}
		return
	}
	lang = strings.ToLower(strings.TrimSpace(q.Language))
	if lang == "" {
		lang = "en"
	}
	if len(lang) != 2 {
		err = &ValidationError{Message: fmt.Sprintf("invalid language %q, expected a 2-letter code", q.Language)}
		return
	}
	pkg, err = s.resolveService(ctx, country, q.Service)
	return
}

func validateReleaseCountry(raw string) (string, error) {
	code, err := flixpatrol.GetCountryCodeFromISO(strings.TrimSpace(raw))
	if err != nil {
		return "", &ValidationError{Message: fmt.Sprintf("invalid country code: %s", raw)}
	}
	return string(code), nil
}

// resolveService maps a FlixPatrol slug or JustWatch short / technical name
// to the JustWatch package in that country.
func (s *ReleasesService) resolveService(ctx context.Context, country, service string) (client.JWPackage, error) {
	name := strings.ToLower(strings.TrimSpace(service))
	if name == "" {
		return client.JWPackage{}, &ValidationError{Message: "service is required"}
	}
	pkgs, err := s.countryPackages(ctx, country)
	if err != nil {
		return client.JWPackage{}, err
	}
	// A FlixPatrol slug resolves to the first of its codes the country has.
	for _, code := range justWatchPackages[flixpatrol.Provider(name)] {
		for _, p := range pkgs {
			if p.ShortName == string(code) {
				return p, nil
			}
		}
	}
	for _, p := range pkgs {
		if strings.EqualFold(p.ShortName, name) || strings.EqualFold(p.TechnicalName, name) {
			return p, nil
		}
	}
	return client.JWPackage{}, &ValidationError{Message: fmt.Sprintf("unknown service %q in %s; see /api/v1/services/%s", service, country, country)}
}

func (s *ReleasesService) countryPackages(ctx context.Context, country string) ([]client.JWPackage, error) {
	s.mu.Lock()
	cached, ok := s.packages[country]
	s.mu.Unlock()
	if ok && s.now().Sub(cached.fetchedAt) < packagesCacheTTL {
		return cached.packages, nil
	}
	pkgs, err := s.jw.GetPackages(ctx, country)
	if err != nil {
		if ok {
			s.log.Warn("justwatch packages refresh failed, using cached list", slog.String("country", country), slog.Any("err", err))
			return cached.packages, nil
		}
		s.log.Warn("justwatch packages request failed", slog.String("country", country), slog.Any("err", err))
		return nil, fmt.Errorf("%w: packages for %s: %w", ErrUpstream, country, err)
	}
	s.mu.Lock()
	s.packages[country] = packagesCacheEntry{packages: pkgs, fetchedAt: s.now()}
	s.mu.Unlock()
	return pkgs, nil
}

// fetchAll pages through a JustWatch listing, up to releasesMaxItems.
func (s *ReleasesService) fetchAll(ctx context.Context, pageType client.NewPageType, country, lang, date, pkg string) ([]client.NewTitleEdge, bool, error) {
	var edges []client.NewTitleEdge
	after := ""
	for {
		page, err := s.jw.GetNewTitles(ctx, pageType, country, lang, date, pkg, nil, after, releasesPageSize)
		if err != nil {
			s.log.Warn("justwatch titles request failed", slog.String("page_type", string(pageType)), slog.String("country", country), slog.String("service", pkg), slog.String("date", date), slog.Any("err", err))
			return nil, false, fmt.Errorf("%w: %s titles for %s/%s on %s: %w", ErrUpstream, strings.ToLower(string(pageType)), country, pkg, date, err)
		}
		edges = append(edges, page.Edges...)
		if len(edges) >= releasesMaxItems {
			return edges[:releasesMaxItems], true, nil
		}
		if !page.PageInfo.HasNextPage || page.PageInfo.EndCursor == "" || len(page.Edges) == 0 {
			return edges, false, nil
		}
		after = page.PageInfo.EndCursor
	}
}

func (s *ReleasesService) today() time.Time {
	y, m, d := s.now().UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// releaseItemFromEdge flattens a JustWatch movie or season. For seasons the
// title, TMDB / IMDb IDs and (when the season has none) description, poster,
// genres and scores come from the show.
func releaseItemFromEdge(e client.NewTitleEdge, country string) ReleaseItem {
	n := e.Node.MovieOrSeason
	c := n.Content
	item := ReleaseItem{
		Kind:                "movie",
		Title:               c.Title,
		Year:                c.OriginalReleaseYear,
		OriginalReleaseDate: nonEmpty(c.OriginalReleaseDate),
		TmdbID:              nonEmpty(c.ExternalIds.TmdbId),
		ImdbID:              nonEmpty(c.ExternalIds.ImdbId),
		JustWatchID:         n.ID,
		JustWatchURL:        prefixURL(justWatchWebURL, c.FullPath),
		Description:         nonEmpty(c.ShortDescription),
		RuntimeMinutes:      positive(c.Runtime),
		Genres:              genreNames(c.Genres),
		PosterURL:           prefixURL(justWatchImagesURL, c.PosterURL),
	}
	scoring := c.Scoring

	if n.ObjectType == "SHOW_SEASON" || n.ObjectType == "SHOW" {
		item.Kind = "tv"
		if n.ObjectType == "SHOW_SEASON" {
			item.SeasonNumber = c.Season.SeasonNumber
			item.SeasonTitle = nonEmpty(&c.Title)
		}
		// A season's TMDB ID is "<show id>:<season>"; report the show's.
		item.TmdbID = showTmdbID(item.TmdbID)
		if show := n.Season.Show; show != nil {
			sc := show.Content
			item.Title = sc.Title
			if id := nonEmpty(sc.ExternalIds.TmdbId); id != nil {
				item.TmdbID = id
			}
			if item.ImdbID == nil {
				item.ImdbID = nonEmpty(sc.ExternalIds.ImdbId)
			}
			if item.Description == nil {
				item.Description = nonEmpty(sc.ShortDescription)
			}
			if item.PosterURL == nil {
				item.PosterURL = prefixURL(justWatchImagesURL, sc.PosterURL)
			}
			if len(item.Genres) == 0 {
				item.Genres = genreNames(sc.Genres)
			}
			if scoring.ImdbScore == nil && scoring.TomatoMeter == nil {
				scoring = sc.Scoring
			}
		}
	}

	item.IMDbRating = scoring.ImdbScore
	item.IMDbVotes = intPtr(scoring.ImdbVotes)
	item.Tomatometer = intPtr(scoring.TomatoMeter)
	item.CertifiedFresh = scoring.CertifiedFresh
	return item
}

// serviceReleaseDate returns the title's release date on pkg, preferring a
// digital (streaming) release, else the earliest one listed for pkg.
func serviceReleaseDate(e client.NewTitleEdge, pkg string) (*string, *string) {
	var date, typ *string
	for _, r := range e.Node.MovieOrSeason.Content.UpcomingReleases {
		if r.Package == nil || !strings.EqualFold(r.Package.ShortName, pkg) || r.ReleaseDate == nil || *r.ReleaseDate == "" {
			continue
		}
		digital := r.ReleaseType != nil && *r.ReleaseType == "DIGITAL"
		haveDigital := typ != nil && *typ == "DIGITAL"
		if date == nil || (digital && !haveDigital) || (digital == haveDigital && *r.ReleaseDate < *date) {
			date, typ = r.ReleaseDate, r.ReleaseType
		}
	}
	return date, typ
}

func showTmdbID(id *string) *string {
	if id == nil {
		return nil
	}
	show, _, _ := strings.Cut(*id, ":")
	return &show
}

func genreNames(genres []client.JWGenre) []string {
	out := make([]string, 0, len(genres))
	for _, g := range genres {
		if g.Translation != nil && *g.Translation != "" {
			out = append(out, *g.Translation)
		} else if g.ShortName != "" {
			out = append(out, g.ShortName)
		}
	}
	return out
}

func nonEmpty(p *string) *string {
	if p == nil || strings.TrimSpace(*p) == "" {
		return nil
	}
	return p
}

func positive(p *int) *int {
	if p == nil || *p <= 0 {
		return nil
	}
	return p
}

func prefixURL(base string, path *string) *string {
	if path == nil || *path == "" {
		return nil
	}
	if strings.HasPrefix(*path, "http") {
		return path
	}
	u := base + *path
	return &u
}

// intPtr rounds an optional JustWatch number to an int.
func intPtr(v *float64) *int {
	if v == nil {
		return nil
	}
	n := int(*v + 0.5)
	return &n
}
