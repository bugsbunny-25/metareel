package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// ErrRatingsUnavailable means a refresh failed at every provider and there
// was no cached data to fall back on.
var ErrRatingsUnavailable = errors.New("ratings providers unavailable")

// Rating providers, as stored in title_rating_sources.provider.
const (
	ProviderJustWatch      = "justwatch"
	ProviderMDBList        = "mdblist"
	ProviderRottenTomatoes = "rottentomatoes"
)

// Canonical source names shared across providers.
const (
	sourceIMDb              = "imdb"
	sourceTomatoes          = "tomatoes"           // RT critics (Tomatometer), 0-100
	sourcePopcorn           = "popcorn"            // RT audience (Popcornmeter), 0-100
	sourceTomatoesCertified = "tomatoes_certified" // RT Certified Fresh, 1 or 0
)

// justWatchCountry is the catalog used for JustWatch lookups; IMDb and RT
// scores do not vary by country.
const justWatchCountry = "US"

// Narrow views of the provider clients, so tests can fake them.
type justWatchRatings interface {
	GetTitleByID(ctx context.Context, id, country, language string) (*client.MovieOrShowFragment, error)
	GetTitlesByTopSearchPopular(ctx context.Context, searchQuery string, first int, country, language string, objectType client.ObjectType, packages []client.Package) (*client.GetTitlesByTopSearchPopularQuery, error)
}

type mdblistRatings interface {
	GetByTmdb(ctx context.Context, kind, tmdbID string) (*client.MDBListMedia, error)
}

type rottenTomatoesRatings interface {
	BySlug(ctx context.Context, slug string) (*client.RTRating, error)
	Search(ctx context.Context, kind client.RTKind, name string, year int) (*client.RTRating, error)
}

type tmdbDetails interface {
	Enabled() bool
	Details(ctx context.Context, kind, tmdbID string) (string, int, error)
}

type RatingsService struct {
	log       *slog.Logger
	repo      *repository.RatingsRepository
	titles    *repository.FlixPatrolRepository
	justWatch justWatchRatings
	mdblist   mdblistRatings // nil when MDBLIST_API_KEY is not set
	rt        rottenTomatoesRatings
	tmdb      tmdbDetails
	ttl       time.Duration
	preferred string // provider whose IMDb / RT values win when both have one
	now       func() time.Time
	refreshes singleflight.Group
}

type RatingsServiceConfig struct {
	TTL               time.Duration
	PreferredProvider string // ProviderJustWatch | ProviderMDBList
}

func NewRatingsService(log *slog.Logger, repo *repository.RatingsRepository, titles *repository.FlixPatrolRepository, jw *client.JustWatchClient, mdb *client.MDBListClient, rt *client.RottenTomatoesClient, tmdb *client.TMDBClient, cfg RatingsServiceConfig) *RatingsService {
	s := &RatingsService{
		log:       log,
		repo:      repo,
		titles:    titles,
		ttl:       cfg.TTL,
		preferred: cfg.PreferredProvider,
		now:       time.Now,
	}
	// Assign only non-nil clients so the interface fields stay nil-comparable.
	if jw != nil {
		s.justWatch = jw
	}
	if mdb != nil {
		s.mdblist = mdb
	}
	if rt != nil {
		s.rt = rt
	}
	if tmdb != nil {
		s.tmdb = tmdb
	}
	if s.preferred != ProviderMDBList || s.mdblist == nil {
		s.preferred = ProviderJustWatch
	}
	return s
}

// PreferredProvider is the provider that wins for headline values.
func (s *RatingsService) PreferredProvider() string { return s.preferred }

type RatingsResponse struct {
	Kind           string                `json:"kind"` // movie | tv
	TmdbID         string                `json:"tmdb_id"`
	ImdbID         *string               `json:"imdb_id"`
	Title          *string               `json:"title"`
	Year           *int64                `json:"year"`
	IMDb           *IMDbRating           `json:"imdb"`
	RottenTomatoes *RottenTomatoesRating `json:"rotten_tomatoes"`
	Sources        []RatingSourceResult  `json:"sources"` // every rating each provider returned
	RefreshedAt    time.Time             `json:"refreshed_at"`
	Cached         bool                  `json:"cached"` // served from the database without refreshing
	Stale          bool                  `json:"stale"`  // a refresh was due but failed; data is older than the TTL
}

type IMDbRating struct {
	Rating float64 `json:"rating"` // 0-10
	Votes  *int64  `json:"votes"`
	URL    *string `json:"url"`
	Source string  `json:"source"` // provider the value came from
}

type RottenTomatoesRating struct {
	URL            *string `json:"url"`
	CriticsScore   *int64  `json:"critics_score"`  // Tomatometer, 0-100
	CriticsRating  *string `json:"critics_rating"` // Certified Fresh | Fresh | Rotten
	CertifiedFresh *bool   `json:"certified_fresh"`
	CriticsSource  *string `json:"critics_source"`  // provider the critics score came from
	AudienceScore  *int64  `json:"audience_score"`  // Popcornmeter, 0-100
	AudienceRating *string `json:"audience_rating"` // Upright | Spilled
	AudienceSource *string `json:"audience_source"`
}

type RatingSourceResult struct {
	Provider  string    `json:"provider"`
	Source    string    `json:"source"`
	Value     float64   `json:"value"` // on the provider's own scale for this source
	Votes     *int64    `json:"votes,omitempty"`
	URL       *string   `json:"url,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
}

// GetRatings returns cached ratings if they were refreshed within the TTL,
// otherwise refreshes them from the providers. force skips the cache. If a
// refresh fails and older ratings exist, those are returned with Stale set.
func (s *RatingsService) GetRatings(ctx context.Context, ref TmdbRef, force bool) (*RatingsResponse, error) {
	ident, sources, err := s.repo.GetTitleRatings(ctx, ref.Kind, ref.ID)
	if err != nil {
		return nil, err
	}
	if ident != nil && !force && s.now().Sub(ident.RefreshedAt) < s.ttl {
		return s.buildResponse(ref, ident, sources, true, false), nil
	}

	// Collapse concurrent refreshes of the same title, and don't let one
	// caller hanging up abort a refresh others are waiting on.
	v, err, _ := s.refreshes.Do(ref.String(), func() (any, error) {
		refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		return s.refresh(refreshCtx, ref, ident)
	})
	if err != nil {
		if ident != nil && !errors.Is(err, ErrTitleNotFound) {
			s.log.Warn("ratings refresh failed, serving stale ratings", slog.String("title", ref.String()), slog.Any("err", err))
			return s.buildResponse(ref, ident, sources, true, true), nil
		}
		return nil, err
	}
	res := v.(refreshResult)
	return s.buildResponse(ref, res.ident, res.sources, false, false), nil
}

type refreshResult struct {
	ident   *repository.TitleRatings
	sources []repository.RatingSource
}

func (s *RatingsService) refresh(ctx context.Context, ref TmdbRef, prev *repository.TitleRatings) (refreshResult, error) {
	start := s.now()
	ident := repository.TitleRatings{TmdbKind: ref.Kind, TmdbID: ref.ID, RefreshedAt: start}
	if prev != nil {
		ident.Title, ident.Year, ident.ImdbID, ident.JustWatchID, ident.RTSlug = prev.Title, prev.Year, prev.ImdbID, prev.JustWatchID, prev.RTSlug
	}
	titleKind := titleKindForTmdb(ref.Kind)

	// Scraped titles mapped to this TMDB ID give us a name, IMDb ID and RT slug.
	scraped, err := s.titles.ListTitlesByTmdbIDs(ctx, []string{ref.ID})
	if err != nil {
		return refreshResult{}, err
	}
	for _, t := range scraped {
		if t.Kind != titleKind {
			continue
		}
		setIfEmpty(&ident.Title, t.Name)
		if t.ImdbID.Valid {
			setIfEmpty(&ident.ImdbID, t.ImdbID.String)
		}
		if t.RtUrl.Valid {
			setIfEmpty(&ident.RTSlug, t.RtUrl.String)
		}
	}

	sourcesByProvider := map[string][]repository.RatingSource{}
	var errs []error
	var rtSlugHints []string

	if s.mdblist != nil {
		m, err := s.mdblist.GetByTmdb(ctx, ref.Kind, ref.ID)
		switch {
		case err != nil:
			errs = append(errs, err)
			s.log.Warn("mdblist ratings failed", slog.String("title", ref.String()), slog.Any("err", err))
		case m == nil:
			sourcesByProvider[ProviderMDBList] = nil
		default:
			setIfEmpty(&ident.Title, m.Title)
			if m.Year > 0 && ident.Year == nil {
				y := int64(m.Year)
				ident.Year = &y
			}
			setIfEmpty(&ident.ImdbID, m.ImdbID)
			var hint string
			sourcesByProvider[ProviderMDBList], hint = mdblistSources(m, start)
			if hint != "" {
				rtSlugHints = append(rtSlugHints, hint)
			}
		}
	}

	if s.tmdb != nil && s.tmdb.Enabled() && ident.Title == nil {
		name, year, err := s.tmdb.Details(ctx, ref.Kind, ref.ID)
		if err != nil {
			s.log.Warn("tmdb details failed", slog.String("title", ref.String()), slog.Any("err", err))
		}
		setIfEmpty(&ident.Title, name)
		if year > 0 && ident.Year == nil {
			y := int64(year)
			ident.Year = &y
		}
	}

	if s.justWatch != nil {
		frag, err := s.justWatchTitle(ctx, ref, &ident)
		switch {
		case err != nil:
			errs = append(errs, err)
			s.log.Warn("justwatch ratings failed", slog.String("title", ref.String()), slog.Any("err", err))
		case frag == nil:
			if ident.Title != nil {
				sourcesByProvider[ProviderJustWatch] = nil
			}
		default:
			setIfEmpty(&ident.Title, frag.Content.Title)
			if frag.Content.OriginalReleaseYear != nil && ident.Year == nil {
				y := int64(*frag.Content.OriginalReleaseYear)
				ident.Year = &y
			}
			if frag.Content.ExternalIds.ImdbId != nil {
				setIfEmpty(&ident.ImdbID, *frag.Content.ExternalIds.ImdbId)
			}
			id := frag.Id
			ident.JustWatchID = &id
			sourcesByProvider[ProviderJustWatch] = justWatchSources(frag, ident.ImdbID, start)
		}
	}

	// With no name and no ratings there is nothing to search RT with.
	if ident.Title == nil && !hasRatings(sourcesByProvider) {
		if len(errs) > 0 {
			return refreshResult{}, fmt.Errorf("%w: %w", ErrRatingsUnavailable, errors.Join(errs...))
		}
		return refreshResult{}, ErrTitleNotFound
	}

	if s.rt != nil {
		r, err := s.rottenTomatoes(ctx, ref, &ident, rtSlugHints)
		switch {
		case err != nil:
			errs = append(errs, err)
			s.log.Warn("rotten tomatoes ratings failed", slog.String("title", ref.String()), slog.Any("err", err))
		case r == nil:
			sourcesByProvider[ProviderRottenTomatoes] = nil
		default:
			slug := r.Slug
			ident.RTSlug = &slug
			sourcesByProvider[ProviderRottenTomatoes] = rtSources(r, start)
		}
	}

	// Don't cache an empty result caused by provider errors; retry next time
	// (or serve the previous ratings as stale).
	if !hasRatings(sourcesByProvider) && len(errs) > 0 {
		return refreshResult{}, fmt.Errorf("%w: %w", ErrRatingsUnavailable, errors.Join(errs...))
	}

	saved, sources, err := s.repo.SaveTitleRatings(ctx, ident, sourcesByProvider)
	if err != nil {
		return refreshResult{}, err
	}
	if err := s.repo.FillTitleExternalIDs(ctx, titleKind, ref.ID, saved.ImdbID, saved.RTSlug); err != nil {
		s.log.Warn("back-filling title ids failed", slog.String("title", ref.String()), slog.Any("err", err))
	}
	s.log.Info("ratings refreshed",
		slog.String("title", ref.String()),
		slog.String("name", stringValue(saved.Title)),
		slog.Int("sources", len(sources)),
		slog.Int("provider_errors", len(errs)),
		slog.Int64("duration_ms", s.now().Sub(start).Milliseconds()))
	return refreshResult{ident: saved, sources: sources}, nil
}

// justWatchTitle finds the JustWatch title by its stored node ID, or else by
// searching the name and matching the TMDB ID. Returns nil if not found.
func (s *RatingsService) justWatchTitle(ctx context.Context, ref TmdbRef, ident *repository.TitleRatings) (*client.MovieOrShowFragment, error) {
	if ident.JustWatchID != nil {
		frag, err := s.justWatch.GetTitleByID(ctx, *ident.JustWatchID, justWatchCountry, "en")
		if err != nil {
			return nil, fmt.Errorf("justwatch node %s: %w", *ident.JustWatchID, err)
		}
		if frag != nil && frag.Content.ExternalIds.TmdbId != nil && *frag.Content.ExternalIds.TmdbId == ref.ID {
			return frag, nil
		}
	}
	if ident.Title == nil {
		return nil, nil
	}
	objectType := client.ObjectTypeMovie
	if ref.Kind == "tv" {
		objectType = client.ObjectTypeTVShow
	}
	res, err := s.justWatch.GetTitlesByTopSearchPopular(ctx, *ident.Title, 10, justWatchCountry, "en", objectType, nil)
	if err != nil {
		return nil, fmt.Errorf("justwatch search %q: %w", *ident.Title, err)
	}
	for i := range res.PoupularTitles.Edges {
		frag := &res.PoupularTitles.Edges[i].Node
		if frag.Content.ExternalIds.TmdbId != nil && *frag.Content.ExternalIds.TmdbId == ref.ID {
			return frag, nil
		}
	}
	return nil, nil
}

// rottenTomatoes looks the title up on RT: by known slug first (stored, from
// the scraped title, or MDBList's RT link), else by Seerr-style name + year
// search. Returns nil if RT has no match.
func (s *RatingsService) rottenTomatoes(ctx context.Context, ref TmdbRef, ident *repository.TitleRatings, slugHints []string) (*client.RTRating, error) {
	var slugs []string
	if ident.RTSlug != nil {
		slugs = append(slugs, *ident.RTSlug)
	}
	for _, h := range slugHints {
		if !containsString(slugs, h) {
			slugs = append(slugs, h)
		}
	}
	for _, slug := range slugs {
		r, err := s.rt.BySlug(ctx, slug)
		if err != nil {
			return nil, err
		}
		if r != nil {
			return r, nil
		}
		s.log.Debug("rotten tomatoes slug not found", slog.String("title", ref.String()), slog.String("slug", slug))
	}

	if ident.Title == nil {
		return nil, nil
	}
	kind := client.RTKindMovie
	if ref.Kind == "tv" {
		kind = client.RTKindTV
	}
	year := 0
	if ident.Year != nil {
		year = int(*ident.Year)
	}
	r, err := s.rt.Search(ctx, kind, *ident.Title, year)
	if err != nil {
		return nil, err
	}
	if r != nil {
		s.log.Info("rotten tomatoes slug found by search", slog.String("title", ref.String()), slog.String("name", *ident.Title), slog.Int("year", year), slog.String("slug", r.Slug), slog.String("rt_title", r.Title), slog.Int("rt_year", r.Year))
	}
	return r, nil
}

func justWatchSources(f *client.MovieOrShowFragment, imdbID *string, at time.Time) []repository.RatingSource {
	sc := f.Content.Scoring
	var out []repository.RatingSource
	add := func(source string, value float64, votes *int64, u *string) {
		out = append(out, repository.RatingSource{Provider: ProviderJustWatch, Source: source, Value: value, Votes: votes, URL: u, FetchedAt: at})
	}
	if sc.ImdbScore != nil {
		var votes *int64
		if sc.ImdbVotes != nil {
			v := int64(*sc.ImdbVotes)
			votes = &v
		}
		add(sourceIMDb, *sc.ImdbScore, votes, imdbURL(imdbID))
	}
	if sc.TomatoMeter != nil {
		add(sourceTomatoes, float64(*sc.TomatoMeter), nil, nil)
		if sc.CertifiedFresh != nil {
			add(sourceTomatoesCertified, boolValue(*sc.CertifiedFresh), nil, nil)
		}
	}
	if sc.TmdbScore != nil {
		add("tmdb", *sc.TmdbScore, nil, nil)
	}
	if sc.TmdbPopularity != nil {
		add("tmdb_popularity", *sc.TmdbPopularity, nil, nil)
	}
	if sc.JwRating != nil {
		add("justwatch", *sc.JwRating, nil, nil)
	}
	return out
}

// mdblistSources converts every MDBList rating, plus its own scores, and
// returns the RT slug from its Rotten Tomatoes link if it has one.
func mdblistSources(m *client.MDBListMedia, at time.Time) ([]repository.RatingSource, string) {
	var out []repository.RatingSource
	var rtSlug string
	seen := map[string]bool{}
	for _, r := range m.Ratings {
		if r.Value == nil || r.Source == "" {
			continue
		}
		source := r.Source
		if source == "tomatoesaudience" {
			source = sourcePopcorn
		}
		if seen[source] {
			continue
		}
		seen[source] = true
		rs := repository.RatingSource{Provider: ProviderMDBList, Source: source, Value: *r.Value, FetchedAt: at}
		if r.Votes != nil {
			v := int64(*r.Votes)
			rs.Votes = &v
		}
		if r.URL != "" {
			u := r.URL
			rs.URL = &u
			if source == sourceTomatoes && rtSlug == "" {
				rtSlug = rtSlugFromURL(r.URL)
			}
		}
		out = append(out, rs)
	}
	if m.Score != nil {
		out = append(out, repository.RatingSource{Provider: ProviderMDBList, Source: "mdblist", Value: *m.Score, FetchedAt: at})
	}
	if m.ScoreAverage != nil {
		out = append(out, repository.RatingSource{Provider: ProviderMDBList, Source: "mdblist_average", Value: *m.ScoreAverage, FetchedAt: at})
	}
	return out, rtSlug
}

func rtSources(r *client.RTRating, at time.Time) []repository.RatingSource {
	u := r.URL()
	var out []repository.RatingSource
	if r.CriticsScore != nil {
		out = append(out, repository.RatingSource{Provider: ProviderRottenTomatoes, Source: sourceTomatoes, Value: float64(*r.CriticsScore), URL: &u, FetchedAt: at})
	}
	if r.AudienceScore != nil {
		out = append(out, repository.RatingSource{Provider: ProviderRottenTomatoes, Source: sourcePopcorn, Value: float64(*r.AudienceScore), URL: &u, FetchedAt: at})
	}
	if r.CertifiedFresh != nil {
		out = append(out, repository.RatingSource{Provider: ProviderRottenTomatoes, Source: sourceTomatoesCertified, Value: boolValue(*r.CertifiedFresh), URL: &u, FetchedAt: at})
	}
	return out
}

// rtSlugFromURL extracts "m/<vanity>" or "tv/<vanity>" from an RT link or path.
func rtSlugFromURL(raw string) string {
	path := raw
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		path = u.Path
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if (parts[i] == "m" || parts[i] == "tv") && parts[i+1] != "" {
			return parts[i] + "/" + parts[i+1]
		}
	}
	return ""
}

func (s *RatingsService) buildResponse(ref TmdbRef, ident *repository.TitleRatings, sources []repository.RatingSource, cached, stale bool) *RatingsResponse {
	out := &RatingsResponse{
		Kind:        ref.Kind,
		TmdbID:      ref.ID,
		ImdbID:      ident.ImdbID,
		Title:       ident.Title,
		Year:        ident.Year,
		Sources:     make([]RatingSourceResult, 0, len(sources)),
		RefreshedAt: ident.RefreshedAt,
		Cached:      cached,
		Stale:       stale,
	}
	for _, src := range sources {
		out.Sources = append(out.Sources, RatingSourceResult{Provider: src.Provider, Source: src.Source, Value: src.Value, Votes: src.Votes, URL: src.URL, FetchedAt: src.FetchedAt})
	}
	out.IMDb = resolveIMDb(sources, s.providerOrder(false), ident.ImdbID)
	out.RottenTomatoes = resolveRottenTomatoes(sources, s.providerOrder(true), ident.RTSlug, ref.Kind == "movie")
	return out
}

// providerOrder lists providers by precedence for headline values. RT's own
// numbers win for RT scores; otherwise the preferred provider goes first.
func (s *RatingsService) providerOrder(rt bool) []string {
	order := []string{ProviderJustWatch, ProviderMDBList}
	if s.preferred == ProviderMDBList {
		order = []string{ProviderMDBList, ProviderJustWatch}
	}
	if rt {
		order = append([]string{ProviderRottenTomatoes}, order...)
	}
	return order
}

func findSource(sources []repository.RatingSource, order []string, source string) *repository.RatingSource {
	for _, p := range order {
		for i := range sources {
			if sources[i].Provider == p && sources[i].Source == source {
				return &sources[i]
			}
		}
	}
	return nil
}

func resolveIMDb(sources []repository.RatingSource, order []string, imdbID *string) *IMDbRating {
	src := findSource(sources, order, sourceIMDb)
	if src == nil {
		return nil
	}
	return &IMDbRating{Rating: src.Value, Votes: src.Votes, URL: imdbURL(imdbID), Source: src.Provider}
}

func resolveRottenTomatoes(sources []repository.RatingSource, order []string, slug *string, isMovie bool) *RottenTomatoesRating {
	critics := findSource(sources, order, sourceTomatoes)
	audience := findSource(sources, order, sourcePopcorn)
	if critics == nil && audience == nil {
		return nil
	}
	out := &RottenTomatoesRating{}
	if slug != nil {
		u := "https://www.rottentomatoes.com/" + *slug
		out.URL = &u
	}
	if critics != nil {
		score := int64(critics.Value)
		out.CriticsScore = &score
		out.CriticsSource = &critics.Provider
		rating := "Rotten"
		if score >= 60 {
			rating = "Fresh"
		}
		// Certified Fresh is a movie-only RT badge; take it from the same
		// provider as the score when possible.
		if isMovie {
			certified := findSource(sources, append([]string{critics.Provider}, order...), sourceTomatoesCertified)
			if certified != nil {
				c := certified.Value >= 1
				out.CertifiedFresh = &c
				if c {
					rating = "Certified Fresh"
				}
			}
		}
		out.CriticsRating = &rating
	}
	if audience != nil {
		score := int64(audience.Value)
		out.AudienceScore = &score
		out.AudienceSource = &audience.Provider
		rating := "Spilled"
		if score >= 60 {
			rating = "Upright"
		}
		out.AudienceRating = &rating
	}
	return out
}

func imdbURL(imdbID *string) *string {
	if imdbID == nil || *imdbID == "" {
		return nil
	}
	u := "https://www.imdb.com/title/" + *imdbID + "/"
	return &u
}

func hasRatings(byProvider map[string][]repository.RatingSource) bool {
	for _, s := range byProvider {
		if len(s) > 0 {
			return true
		}
	}
	return false
}

func setIfEmpty(dst **string, v string) {
	if *dst == nil && strings.TrimSpace(v) != "" {
		v = strings.TrimSpace(v)
		*dst = &v
	}
}

func stringValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func boolValue(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
