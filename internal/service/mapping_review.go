package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// The mapping review queue scores every automatic mapping (FlixPatrol and
// Netflix titles not fixed by hand) by how likely it is wrong, so the few
// suspicious ones can be checked first. Each signal adds points (capped at
// 100) and a reason.

// Review reason codes.
const (
	ReviewWeakSource    = "weak_source"     // matched by a weak rule
	ReviewNotOnProvider = "not_on_provider" // TMDB shows no offer from the service it charted on
	ReviewNoOffers      = "no_offers"       // TMDB shows no offers at all
	ReviewReleasedLater = "released_later"  // TMDB release year after the title first charted
	ReviewYearMismatch  = "year_mismatch"   // FlixPatrol's premiere year far from TMDB's (films)
	ReviewNameDiffers   = "name_differs"    // chart name is not TMDB's title
	ReviewIMDbMismatch  = "imdb_mismatch"   // the match came with another IMDb ID
	ReviewNoDetails     = "no_details"      // TMDB details not fetched yet
)

// sourceRisk is the base risk of each match source (unknown sources 15).
var sourceRisk = map[string]int{
	"netflix_id": 0, "justwatch_path": 0, MatchSourceFlixPatrolTitle: 5, "netflix_title": 5,
	"justwatch_search": 10, "justwatch_netflix": 10, "wikidata_netflix": 15, "legacy": 15,
	MatchSourceTMDBNetflix: 20, "justwatch_netflix_other_kind": 20, "tmdb_search": 25,
	MatchSourceTMDBNetflixAltTitle: 30, "justwatch_unique_name": 35, "justwatch_recent_release": 35,
}

// providerOffers are the TMDB watch provider IDs of each FlixPatrol provider
// (any plan, store or channel of the service).
var providerOffers = map[string][]int64{
	"netflix":        {8, 175, 1796},
	"amazon-prime":   {9, 10, 119, 613, 2100},
	"apple-tv":       {2, 350},
	"disney":         {122, 337, 390},
	"hbo-max":        {31, 118, 384, 1825, 1899},
	"paramount-plus": {531, 582, 633, 1770, 1853, 2303, 2304, 2616},
	"peacock":        {386, 387},
	"hulu":           {15},
}

var providerNames = map[string]string{
	"netflix": "Netflix", "amazon-prime": "Prime Video", "apple-tv": "Apple TV", "disney": "Disney+",
	"hbo-max": "HBO Max", "paramount-plus": "Paramount+", "peacock": "Peacock", "hulu": "Hulu",
}

// ReviewReason is one signal against a mapping.
type ReviewReason struct {
	Code   string `json:"code"`
	Text   string `json:"text"`
	Points int    `json:"points"`
}

// ReviewItem is a mapping in the review queue.
type ReviewItem struct {
	Subject     string         `json:"subject"` // flixpatrol | netflix
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	Kind        string         `json:"kind"`
	TmdbKind    string         `json:"tmdb_kind"`
	TmdbID      string         `json:"tmdb_id"`
	TmdbTitle   string         `json:"tmdb_title,omitempty"`
	TmdbYear    int            `json:"tmdb_year,omitempty"`
	MatchSource string         `json:"match_source,omitempty"`
	Providers   []string       `json:"providers"`
	FirstSeen   string         `json:"first_charted,omitempty"`
	Points      int64          `json:"points"` // chart points (FlixPatrol) / weeks in the global Top 10 (Netflix)
	Risk        int            `json:"risk"`   // 0-100
	Reasons     []ReviewReason `json:"reasons"`
}

// MappingReviewQuery filters the queue.
type MappingReviewQuery struct {
	Subject  string // flixpatrol | netflix | "" (both)
	Reason   string // a reason code, "" = any
	MinRisk  int    // default 25
	MinGiven bool   // MinRisk was set (0 lists everything)
}

// MappingReviewResponse is one page of the queue, riskiest first.
type MappingReviewResponse struct {
	Checked int            `json:"checked"` // automatic mappings scored
	Total   int            `json:"total"`   // matching the filters
	Reasons map[string]int `json:"reasons"` // mappings per reason code (before the reason filter)
	Items   []ReviewItem   `json:"items"`
}

const defaultReviewMinRisk = 25

// Review scores every automatic mapping and returns those at or above the
// minimum risk, riskiest (then most charted) first.
func (s *TitleMappingService) Review(ctx context.Context, q MappingReviewQuery, limit, offset int) (*MappingReviewResponse, error) {
	switch q.Subject {
	case "", "flixpatrol", "netflix":
	default:
		return nil, &ValidationError{Message: fmt.Sprintf("invalid subject %q, expected flixpatrol or netflix", q.Subject)}
	}
	if q.Reason != "" && !validReviewReason(q.Reason) {
		return nil, &ValidationError{Message: fmt.Sprintf("invalid reason %q", q.Reason)}
	}
	if q.MinRisk < 0 || q.MinRisk > 100 {
		return nil, &ValidationError{Message: "min_risk must be between 0 and 100"}
	}
	minRisk := q.MinRisk
	if !q.MinGiven {
		minRisk = defaultReviewMinRisk
	}
	candidates, err := s.repo.ReviewCandidates(ctx)
	if err != nil {
		return nil, err
	}
	offers, err := s.repo.WatchProviderIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := &MappingReviewResponse{Reasons: map[string]int{}, Items: []ReviewItem{}}
	var items []ReviewItem
	for _, c := range candidates {
		if q.Subject != "" && c.Subject != q.Subject {
			continue
		}
		out.Checked++
		it := scoreMapping(c, offers[c.TmdbKind+":"+c.TmdbID])
		if it.Risk < minRisk {
			continue
		}
		has := q.Reason == ""
		for _, r := range it.Reasons {
			out.Reasons[r.Code]++
			has = has || r.Code == q.Reason
		}
		if has {
			items = append(items, it)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Risk != b.Risk {
			return a.Risk > b.Risk
		}
		if a.Points != b.Points {
			return a.Points > b.Points
		}
		if a.Subject != b.Subject {
			return a.Subject < b.Subject
		}
		return a.ID < b.ID
	})
	out.Total = len(items)
	if offset < len(items) {
		out.Items = items[offset:min(len(items), offset+limit)]
	}
	return out, nil
}

func validReviewReason(code string) bool {
	switch code {
	case ReviewWeakSource, ReviewNotOnProvider, ReviewNoOffers, ReviewReleasedLater, ReviewYearMismatch,
		ReviewNameDiffers, ReviewIMDbMismatch, ReviewNoDetails:
		return true
	}
	return false
}

// chartNameSuffix strips ": Season 2", " - Part 1", ": Limited Series" etc. from
// chart names before comparing them with TMDB's show title.
var chartNameSuffix = regexp.MustCompile(`(?i)\s*[:\-–(]\s*(season|series|part|volume|vol\.|chapter|limited series)\b.*$`)

func scoreMapping(c repository.ReviewCandidate, offers map[int64]bool) ReviewItem {
	it := ReviewItem{Subject: c.Subject, ID: c.ID, Name: c.Name, Kind: c.Kind, TmdbKind: c.TmdbKind, TmdbID: c.TmdbID,
		TmdbTitle: c.TmdbTitle, TmdbYear: dateYear(c.TmdbReleaseDate), MatchSource: c.MatchSource, Providers: c.Providers,
		FirstSeen: c.FirstCharted, Points: c.Points, Reasons: []ReviewReason{}}
	if it.Providers == nil {
		it.Providers = []string{}
	}
	add := func(code string, points int, format string, args ...any) {
		it.Reasons = append(it.Reasons, ReviewReason{Code: code, Text: fmt.Sprintf(format, args...), Points: points})
		it.Risk += points
	}

	base, known := sourceRisk[c.MatchSource]
	if !known {
		base = 15
	}
	if base >= 20 || !known {
		src := c.MatchSource
		if src == "" {
			src = "unknown"
		}
		add(ReviewWeakSource, base, "matched by %s", src)
	} else {
		it.Risk += base
	}

	if !c.HasDetails {
		add(ReviewNoDetails, 5, "TMDB details not fetched yet")
	} else {
		if len(offers) == 0 {
			add(ReviewNoOffers, 15, "TMDB lists no streaming, rental or purchase offers")
		} else {
			for _, p := range c.Providers {
				ids, ok := providerOffers[p]
				if !ok || !chartedNear(c.LastCharted[p], c.DetailsFetchedAt) {
					continue // titles leave services: only check recent charts
				}
				found := false
				for _, id := range ids {
					if offers[id] {
						found = true
						break
					}
				}
				if !found {
					add(ReviewNotOnProvider, 35, "charted on %s, but TMDB shows no %s offer in any country", providerName(p), providerName(p))
					break
				}
			}
		}
		chartYear := dateYear(c.FirstCharted)
		if it.TmdbYear > 0 && chartYear > 0 && it.TmdbYear > chartYear {
			add(ReviewReleasedLater, 40, "TMDB release year %d is after it first charted (%s)", it.TmdbYear, c.FirstCharted)
		}
		if fp := dateYear(c.FPPremiereDate); c.TmdbKind == "movie" && fp > 0 && it.TmdbYear > 0 && absInt(fp-it.TmdbYear) > maxYearDrift {
			add(ReviewYearMismatch, 25, "FlixPatrol premiere year %d, TMDB %d", fp, it.TmdbYear)
		}
		if c.TmdbTitle != "" && !namesAgree(c.Name, c.TmdbTitle, c.TmdbOriginalTitle) {
			add(ReviewNameDiffers, 15, "TMDB calls it %q", c.TmdbTitle)
		}
	}
	if c.EvidenceIMDbID != "" && c.TmdbIMDbID != "" && !strings.EqualFold(c.EvidenceIMDbID, c.TmdbIMDbID) {
		add(ReviewIMDbMismatch, 30, "matched with IMDb %s, the TMDB title has %s", c.EvidenceIMDbID, c.TmdbIMDbID)
	}
	it.Risk = min(it.Risk, 100)
	return it
}

// offerWindowDays: a title that last charted on a service longer than this
// before its offers were fetched may simply have left it.
const offerWindowDays = 60

// chartedNear reports whether a title last charted (YYYY-MM-DD) within
// offerWindowDays before its offers were fetched (a datetime; "" = unknown).
func chartedNear(lastCharted, fetchedAt string) bool {
	last, err1 := time.Parse(repository.DateLayout, lastCharted)
	if len(fetchedAt) < 10 {
		return false
	}
	fetched, err2 := time.Parse(repository.DateLayout, fetchedAt[:10])
	if err1 != nil || err2 != nil {
		return false
	}
	return fetched.Sub(last) <= offerWindowDays*24*time.Hour
}

// namesAgree reports whether a chart name is (or contains, or is contained
// in) TMDB's title or original title, ignoring case, punctuation and season
// suffixes.
func namesAgree(chart string, tmdb ...string) bool {
	a := normalizeTitle(chartNameSuffix.ReplaceAllString(chart, ""))
	if a == "" {
		return true
	}
	for _, t := range tmdb {
		b := normalizeTitle(t)
		if b == "" {
			continue
		}
		if a == b || (len(a) >= 4 && len(b) >= 4 && (strings.Contains(a, b) || strings.Contains(b, a))) {
			return true
		}
	}
	return false
}

func providerName(slug string) string {
	if n := providerNames[slug]; n != "" {
		return n
	}
	return slug
}

func dateYear(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(date[:4])
	return y
}
