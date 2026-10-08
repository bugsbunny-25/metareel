package client

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/go-resty/resty/v2"
)

// Rotten Tomatoes has no public API. Like Seerr
// (server/api/rating/rottentomatoes.ts), we use the Algolia search index the
// rottentomatoes.com site itself queries; the app ID and search-only key are
// the public ones embedded in their web client.
const (
	rtAlgoliaURL    = "https://79frdp12pn-dsn.algolia.net/1/indexes/*/queries"
	rtAlgoliaAppID  = "79FRDP12PN"
	rtAlgoliaAPIKey = "175588f6e5f8319b27702e4cc4013561"
)

// RTKind is the Rotten Tomatoes content type.
type RTKind string

const (
	RTKindMovie RTKind = "movie"
	RTKindTV    RTKind = "tv"
)

// slugPrefix is the URL path prefix RT uses for the kind: /m/<vanity> or /tv/<vanity>.
func (k RTKind) slugPrefix() string {
	if k == RTKindTV {
		return "tv"
	}
	return "m"
}

// RTRating is a Rotten Tomatoes title with its scores. Score pointers are nil
// when RT has no score yet (e.g. no reviews).
type RTRating struct {
	Slug           string // "m/<vanity>" or "tv/<vanity>", as stored in titles.rt_url
	Title          string
	Year           int
	CriticsScore   *int64 // Tomatometer
	CertifiedFresh *bool  // movies only
	AudienceScore  *int64 // Popcornmeter
}

func (r RTRating) URL() string { return "https://www.rottentomatoes.com/" + r.Slug }

type RottenTomatoesClient struct {
	http *resty.Client
}

func NewRottenTomatoes(timeout time.Duration) *RottenTomatoesClient {
	return &RottenTomatoesClient{
		http: resty.New().
			SetTimeout(timeout).
			SetHeader("x-algolia-application-id", rtAlgoliaAppID).
			SetHeader("x-algolia-api-key", rtAlgoliaAPIKey).
			SetHeader("Content-Type", "application/json").
			SetHeader("Accept", "application/json"),
	}
}

type rtAlgoliaResponse struct {
	Results []struct {
		Index string         `json:"index"`
		Hits  []rtAlgoliaHit `json:"hits"`
	} `json:"results"`
}

type rtAlgoliaHit struct {
	Title          string   `json:"title"`
	Titles         []string `json:"titles"`
	Aka            []string `json:"aka"`
	Type           string   `json:"type"`
	ReleaseYear    int      `json:"releaseYear"`
	Vanity         string   `json:"vanity"`
	RottenTomatoes *struct {
		CriticsScore   *int64 `json:"criticsScore"`
		AudienceScore  *int64 `json:"audienceScore"`
		CertifiedFresh *bool  `json:"certifiedFresh"`
	} `json:"rottenTomatoes"`
}

func (c *RottenTomatoesClient) search(ctx context.Context, kind RTKind, query string) ([]rtAlgoliaHit, error) {
	filters := url.QueryEscape(fmt.Sprintf(`isEmsSearchable=1 AND type:"%s"`, kind))
	body := map[string]any{
		"requests": []map[string]any{{
			"indexName": "content_rt",
			"query":     query,
			"params":    "filters=" + filters + "&hitsPerPage=20",
		}},
	}
	var out rtAlgoliaResponse
	resp, err := c.http.R().SetContext(ctx).SetBody(body).SetResult(&out).Post(rtAlgoliaURL)
	if err != nil {
		return nil, fmt.Errorf("rotten tomatoes search: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("rotten tomatoes search status %d", resp.StatusCode())
	}
	for _, r := range out.Results {
		if r.Index == "content_rt" {
			return r.Hits, nil
		}
	}
	return nil, nil
}

// BySlug looks a title up by its RT slug ("m/<vanity>" or "tv/<vanity>"):
// searching the index for the vanity returns that title, which we pick by
// exact vanity match. Returns nil if RT no longer has it.
func (c *RottenTomatoesClient) BySlug(ctx context.Context, slug string) (*RTRating, error) {
	prefix, vanity, ok := strings.Cut(strings.Trim(slug, "/"), "/")
	if !ok || vanity == "" {
		return nil, fmt.Errorf("invalid rotten tomatoes slug %q", slug)
	}
	kind := RTKindMovie
	if prefix == "tv" {
		kind = RTKindTV
	}
	hits, err := c.search(ctx, kind, vanity)
	if err != nil {
		return nil, err
	}
	for i := range hits {
		if hits[i].Vanity == vanity {
			r := hits[i].rating(kind)
			return &r, nil
		}
	}
	return nil, nil
}

var rtTheWordRe = regexp.MustCompile(`(?i)\bthe\b ?`)

// Search finds the best RT match for name + year (year 0 = unknown) using
// Seerr's scoring. Returns nil if nothing scores high enough or the best
// match has no scores.
func (c *RottenTomatoesClient) Search(ctx context.Context, kind RTKind, name string, year int) (*RTRating, error) {
	query := name
	if kind == RTKindMovie {
		// As Seerr does: "the" hurts movie search relevance.
		query = rtTheWordRe.ReplaceAllString(name, "")
	}
	hits, err := c.search(ctx, kind, query)
	if err != nil {
		return nil, err
	}
	best := bestRTHit(hits, name, year)
	if best == nil || best.RottenTomatoes == nil {
		return nil, nil
	}
	r := best.rating(kind)
	return &r, nil
}

// Candidates returns RT's search hits for name, best match first (Seerr
// scoring with year 0 = any), for picking a slug by hand.
func (c *RottenTomatoesClient) Candidates(ctx context.Context, kind RTKind, name string, year int) ([]RTRating, error) {
	hits, err := c.search(ctx, kind, name)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(hits, func(i, j int) bool {
		return rtTitleScore(&hits[i], name)*rtYearScore(&hits[i], year) > rtTitleScore(&hits[j], name)*rtYearScore(&hits[j], year)
	})
	out := make([]RTRating, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.rating(kind))
	}
	return out, nil
}

func (h rtAlgoliaHit) rating(kind RTKind) RTRating {
	r := RTRating{Slug: kind.slugPrefix() + "/" + h.Vanity, Title: h.Title, Year: h.ReleaseYear}
	if h.RottenTomatoes != nil {
		r.CriticsScore = h.RottenTomatoes.CriticsScore
		r.AudienceScore = h.RottenTomatoes.AudienceScore
		if kind == RTKindMovie {
			certified := h.RottenTomatoes.CertifiedFresh != nil && *h.RottenTomatoes.CertifiedFresh
			r.CertifiedFresh = &certified
		}
	}
	return r
}

// Seerr's tunables.
const (
	rtInexactTitleFactor   = 0.25
	rtAlternateTitleFactor = 0.8
	rtPerYearPenalty       = 0.4
	rtMinimumScore         = 0.175
)

// bestRTHit scores each hit as title similarity × year closeness × (1, or 0.5
// without ratings) and returns the best one above rtMinimumScore.
func bestRTHit(hits []rtAlgoliaHit, name string, year int) *rtAlgoliaHit {
	type scored struct {
		score float64
		hit   *rtAlgoliaHit
	}
	var candidates []scored
	for i := range hits {
		h := &hits[i]
		s := rtTitleScore(h, name) * rtYearScore(h, year)
		if h.RottenTomatoes == nil {
			s *= 0.5
		}
		if s > rtMinimumScore {
			candidates = append(candidates, scored{s, h})
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	return candidates[0].hit
}

// rtTitleScore is the best similarity between name and the hit's main or
// alternate titles; alternates are penalized.
func rtTitleScore(h *rtAlgoliaHit, name string) float64 {
	want := rtNorm(name)
	best := rtSimilarity(rtNorm(h.Title), want)
	for _, alt := range append(append([]string{}, h.Aka...), h.Titles...) {
		best = math.Max(best, rtSimilarity(rtNorm(alt), want)*rtAlternateTitleFactor)
	}
	return best
}

// rtYearScore: 0 years off -> 1.0, 1 -> 0.6, 2 -> 0.2, 3+ -> 0.
func rtYearScore(h *rtAlgoliaHit, year int) float64 {
	if year == 0 {
		return 1
	}
	return math.Max(0, 1-math.Abs(float64(h.ReleaseYear-year))*rtPerYearPenalty)
}

func rtSimilarity(a, b string) float64 {
	if a == b {
		return 1
	}
	return jaroSimilarity(a, b) * rtInexactTitleFactor
}

// rtNorm lowercases and keeps only letters, digits and spaces.
func rtNorm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == ' ' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// jaroSimilarity is the Jaro string similarity in [0, 1].
func jaroSimilarity(s1, s2 string) float64 {
	a, b := []rune(s1), []rune(s2)
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	window := max(len(a), len(b))/2 - 1
	if window < 0 {
		window = 0
	}
	aMatched := make([]bool, len(a))
	bMatched := make([]bool, len(b))
	matches := 0
	for i := range a {
		lo, hi := max(0, i-window), min(len(b)-1, i+window)
		for j := lo; j <= hi; j++ {
			if !bMatched[j] && a[i] == b[j] {
				aMatched[i], bMatched[j] = true, true
				matches++
				break
			}
		}
	}
	if matches == 0 {
		return 0
	}
	transpositions, k := 0, 0
	for i := range a {
		if !aMatched[i] {
			continue
		}
		for !bMatched[k] {
			k++
		}
		if a[i] != b[k] {
			transpositions++
		}
		k++
	}
	m := float64(matches)
	return (m/float64(len(a)) + m/float64(len(b)) + (m-float64(transpositions)/2)/m) / 3
}
