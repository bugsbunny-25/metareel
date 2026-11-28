package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// TitleFacts are a chart title's stored TMDB facts, sent on every chart row
// so clients need no TMDB lookup per title. Fields are omitted when the
// title has no TMDB metadata yet. Ratings are set only with include=ratings.
type TitleFacts struct {
	TmdbKind     string   `json:"tmdb_kind,omitempty"` // movie | tv: the namespace TMDB-keyed URLs use
	BackdropPath *string  `json:"backdrop_path,omitempty"`
	Overview     *string  `json:"overview,omitempty"`
	ReleaseDate  *string  `json:"release_date,omitempty"` // first air date for TV
	Year         *int     `json:"year,omitempty"`
	Runtime      *int64   `json:"runtime,omitempty"`      // minutes; TV: episode runtime
	SeasonCount  *int64   `json:"season_count,omitempty"` // TV only
	Genres       []string `json:"genres,omitempty"`
	// LogoPath: the title's logo image (TMDB path; English, else the
	// original language). Certification: the age certification in the
	// chart's country (omitted on lists spanning countries).
	LogoPath      *string `json:"logo_path,omitempty"`
	Certification *string `json:"certification,omitempty"`
	// Ratings are the stored IMDb and Rotten Tomatoes ratings (never fetched
	// for the request; the ratings.prewarm job keeps charting titles fresh).
	Ratings *InlineRatings `json:"ratings,omitempty"`
}

// InlineRatings are a title's stored headline ratings.
type InlineRatings struct {
	IMDb           *IMDbRating           `json:"imdb"`
	RottenTomatoes *RottenTomatoesRating `json:"rotten_tomatoes"`
	RefreshedAt    string                `json:"refreshed_at"` // RFC 3339
	Stale          bool                  `json:"stale"`        // older than the ratings TTL
}

// FactsTarget is one row to fill: its titles.kind (movie | tv_show) or TMDB
// kind (movie | tv), its TMDB id and where the facts go.
type FactsTarget struct {
	Kind    string
	TmdbID  *string
	Country string // the row's chart country, for certification ("" = none)
	Facts   *TitleFacts
}

// FactsHolder is a response whose rows carry TitleFacts.
type FactsHolder interface {
	FactsTargets() []FactsTarget
}

// TitleFactsService fills TitleFacts from stored metadata (one query per
// response) and, on request, stored ratings.
type TitleFactsService struct {
	metadata *repository.MetadataRepository
	ratings  *RatingsService
}

func NewTitleFactsService(metadata *repository.MetadataRepository, ratings *RatingsService) *TitleFactsService {
	return &TitleFactsService{metadata: metadata, ratings: ratings}
}

// ParseFactsInclude parses a chart endpoint's include= ("ratings" is the
// only optional section).
func ParseFactsInclude(raw string) (withRatings bool, err error) {
	for p := range strings.SplitSeq(raw, ",") {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "":
		case "ratings":
			withRatings = true
		default:
			return false, &ValidationError{Message: fmt.Sprintf("invalid include %q, expected ratings", strings.TrimSpace(p))}
		}
	}
	return withRatings, nil
}

// Fill sets the facts of every row of h. A nil h or service is a no-op.
func (s *TitleFactsService) Fill(ctx context.Context, h FactsHolder, withRatings bool) error {
	if s == nil || h == nil {
		return nil
	}
	targets := h.FactsTargets()
	refs := make([]repository.TmdbRef, 0, len(targets))
	seen := map[string]bool{}
	for _, t := range targets {
		if t.TmdbID == nil || *t.TmdbID == "" || t.Facts == nil {
			continue
		}
		ref := repository.TmdbRef{Kind: factsKind(t.Kind), ID: *t.TmdbID}
		if !seen[ref.String()] {
			seen[ref.String()] = true
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return nil
	}
	meta, err := s.metadata.GetMany(ctx, refs)
	if err != nil {
		return err
	}
	var ratings map[string]*InlineRatings
	if withRatings && s.ratings != nil {
		if ratings, err = s.ratings.storedRatings(ctx, refs); err != nil {
			return err
		}
	}
	for _, t := range targets {
		if t.TmdbID == nil || *t.TmdbID == "" || t.Facts == nil {
			continue
		}
		ref := repository.TmdbRef{Kind: factsKind(t.Kind), ID: *t.TmdbID}
		f := factsFromMetadata(ref.Kind, t.Country, meta[ref.String()])
		f.Ratings = ratings[ref.String()]
		*t.Facts = f
	}
	return nil
}

// factsKind maps titles.kind (movie | tv_show) or a TMDB kind to the TMDB kind.
func factsKind(kind string) string {
	if kind == "tv_show" || kind == "tv" {
		return "tv"
	}
	return "movie"
}

func factsFromMetadata(kind, country string, m *repository.TmdbMetadata) TitleFacts {
	f := TitleFacts{TmdbKind: kind}
	if m == nil {
		return f
	}
	f.BackdropPath = nonEmpty(m.BackdropPath)
	f.Overview = nonEmpty(m.Overview)
	f.ReleaseDate = nonEmpty(m.ReleaseDate)
	if f.ReleaseDate != nil && len(*f.ReleaseDate) >= 4 {
		if y, err := strconv.Atoi((*f.ReleaseDate)[:4]); err == nil {
			f.Year = &y
		}
	}
	if m.Runtime != nil && *m.Runtime > 0 {
		f.Runtime = m.Runtime
	}
	if kind == "tv" && m.NumberOfSeasons != nil && *m.NumberOfSeasons > 0 {
		f.SeasonCount = m.NumberOfSeasons
	}
	if len(m.Genres) > 0 {
		f.Genres = m.Genres
	}
	f.LogoPath = nonEmpty(m.LogoPath)
	if c, ok := m.Certifications[country]; ok && country != "" && c != "" {
		f.Certification = &c
	}
	return f
}

// storedRatings returns the stored ratings of refs keyed by "kind:id",
// without refreshing any of them.
func (s *RatingsService) storedRatings(ctx context.Context, refs []repository.TmdbRef) (map[string]*InlineRatings, error) {
	out := make(map[string]*InlineRatings, len(refs))
	for _, ref := range refs {
		r := TmdbRef{Kind: ref.Kind, ID: ref.ID}
		ident, sources, err := s.repo.GetTitleRatings(ctx, r.Kind, r.ID)
		if err != nil {
			return nil, err
		}
		if ident == nil {
			continue
		}
		stale := s.now().Sub(ident.RefreshedAt) >= s.ttl
		full := s.withDataset(ctx, s.buildResponse(r, ident, sources, true, stale))
		if full.IMDb == nil && full.RottenTomatoes == nil {
			continue
		}
		out[ref.String()] = &InlineRatings{IMDb: full.IMDb, RottenTomatoes: full.RottenTomatoes,
			RefreshedAt: ident.RefreshedAt.UTC().Format("2006-01-02T15:04:05Z07:00"), Stale: stale}
	}
	return out, nil
}

// --- holders -------------------------------------------------------------------

func chartTitleTarget(t *ChartTitle, country string) FactsTarget {
	return FactsTarget{Kind: t.Kind, TmdbID: t.TmdbID, Country: country, Facts: &t.TitleFacts}
}

func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (r *Top10Response) FactsTargets() []FactsTarget { return top10Targets(r.Items, r.Country) }

func top10Targets(items []Top10Result, country string) []FactsTarget {
	out := make([]FactsTarget, 0, len(items))
	for i := range items {
		it := &items[i]
		out = append(out, FactsTarget{Kind: it.Kind, TmdbID: it.TMDBID, Country: country, Facts: &it.TitleFacts})
	}
	return out
}

func (r *Top10Combined) FactsTargets() []FactsTarget {
	return append(top10Targets(r.Movies, r.Country), top10Targets(r.TVShows, r.Country)...)
}

func (r *ChartHistoryResponse) FactsTargets() []FactsTarget {
	var out []FactsTarget
	for d := range r.Days {
		for i := range r.Days[d].Items {
			out = append(out, chartTitleTarget(&r.Days[d].Items[i].ChartTitle, r.Country))
		}
	}
	return out
}

func (r *MoversResponse) FactsTargets() []FactsTarget {
	var out []FactsTarget
	for c := range r.Charts {
		ch := &r.Charts[c]
		for _, list := range [][]Mover{ch.Climbers, ch.Fallers, ch.Debuts, ch.ReEntries, ch.Exits} {
			for i := range list {
				out = append(out, chartTitleTarget(&list[i].ChartTitle, r.Country))
			}
		}
	}
	return out
}

func (r *LeaderboardResponse) FactsTargets() []FactsTarget {
	out := make([]FactsTarget, 0, len(r.Items))
	for i := range r.Items {
		out = append(out, chartTitleTarget(&r.Items[i].ChartTitle, strOr(r.Country)))
	}
	return out
}

func (r *BreakoutsResponse) FactsTargets() []FactsTarget {
	out := make([]FactsTarget, 0, len(r.Items))
	for i := range r.Items {
		out = append(out, chartTitleTarget(&r.Items[i].ChartTitle, ""))
	}
	return out
}

func (r *NetflixWeekResponse) FactsTargets() []FactsTarget {
	out := make([]FactsTarget, 0, len(r.Items))
	for i := range r.Items {
		it := &r.Items[i]
		out = append(out, FactsTarget{Kind: it.Kind, TmdbID: it.TmdbID, Country: strOr(r.Country), Facts: &it.TitleFacts})
	}
	return out
}
