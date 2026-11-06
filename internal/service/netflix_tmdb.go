package service

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// netflixTMDB is the part of the TMDB client Netflix matching uses.
type netflixTMDB interface {
	SearchMovie(ctx context.Context, query string) ([]client.TMDBSearchResult, error)
	SearchTV(ctx context.Context, query string) ([]client.TMDBSearchResult, error)
	WatchProviderCountries(ctx context.Context, kind, tmdbID string, providerIDs []int) ([]string, error)
	AlternativeTitles(ctx context.Context, kind, tmdbID string) ([]string, error)
}

// Match sources of the TMDB step.
const (
	MatchSourceTMDBNetflix         = "tmdb_netflix"           // exact TMDB title, the only one streaming on Netflix
	MatchSourceTMDBNetflixAltTitle = "tmdb_netflix_alt_title" // the same through a TMDB alternative title
)

const (
	netflixTMDBSearchHits = 8 // search hits to consider
	netflixTMDBCandidates = 5 // exact-name hits checked for Netflix availability
	netflixTMDBAltLookups = 3 // top hits whose alternative titles are read when none is exact
	// netflixTMDBGap spaces this step's TMDB requests (TMDB allows about 50
	// a second; a title takes 2-15), so a 400-title backlog fits the job.
	netflixTMDBGap = 100 * time.Millisecond
)

// netflixTMDBMatcher matches Netflix Top 10 names on TMDB: Netflix gives a
// name, a list (films / TV) and the weeks it charted, but no year, so a name
// shared by several titles (Turbo, Borderlands) or a translated local film
// (Pee Nak 5) is settled by which TMDB title is actually on Netflix.
type netflixTMDBMatcher struct {
	tmdb netflixTMDB
	log  *slog.Logger
	gap  time.Duration // between TMDB requests
	n    int           // requests made
}

func (m *netflixTMDBMatcher) wait(ctx context.Context) bool {
	if m.n > 0 && m.gap > 0 {
		if err := sleepCtx(ctx, m.gap); err != nil {
			return false
		}
	}
	m.n++
	return ctx.Err() == nil
}

// match searches the list's TMDB kind, then the other (Netflix lists
// stand-up specials as TV, TMDB as movies; there only a single hit counts).
// Among hits whose title or original title is the name (else, among the top
// hits, an alternative title), released no later than the year the title
// first charted, it takes the only one streaming on Netflix in any country,
// or among several the only recent release. It returns the candidate and its
// TMDB kind.
func (m *netflixTMDBMatcher) match(ctx context.Context, name, listKind string, chartYear int) (titleCandidate, string, bool) {
	listTmdbKind := tmdbKindOf(flixpatrol.TitleKind(listKind))
	otherKind := "tv"
	if listTmdbKind == "tv" {
		otherKind = "movie"
	}
	for i, kind := range []string{listTmdbKind, otherKind} {
		onNetflix := m.onNetflix(ctx, name, kind, chartYear)
		switch {
		case len(onNetflix) == 1:
			return onNetflix[0], kind, true
		case len(onNetflix) > 1 && i == 0:
			if c, ok := pickRecentRelease(onNetflix, chartYear); ok {
				c.Source = MatchSourceTMDBNetflix
				return c, kind, true
			}
			return titleCandidate{}, "", false // ambiguous: don't try the other kind
		}
		if ctx.Err() != nil {
			break
		}
	}
	return titleCandidate{}, "", false
}

// onNetflix returns the exact-name candidates of one kind streaming on
// Netflix.
func (m *netflixTMDBMatcher) onNetflix(ctx context.Context, name, kind string, chartYear int) []titleCandidate {
	if !m.wait(ctx) {
		return nil
	}
	search := m.tmdb.SearchMovie
	if kind == "tv" {
		search = m.tmdb.SearchTV
	}
	hits, err := search(ctx, name)
	if err != nil {
		m.log.Debug("tmdb netflix search failed", slog.String("name", name), slog.Any("err", err))
		return nil
	}
	hits = hits[:min(len(hits), netflixTMDBSearchHits)]
	possible := func(h client.TMDBSearchResult) bool {
		return chartYear == 0 || h.Year == 0 || h.Year <= chartYear
	}
	want := normalizeTitle(name)
	var exact []titleCandidate
	for _, h := range hits {
		if possible(h) && (normalizeTitle(h.Title) == want || normalizeTitle(h.OriginalTitle) == want) {
			exact = append(exact, tmdbHitCandidate(h, MatchSourceTMDBNetflix))
		}
	}
	if len(exact) == 0 {
		for _, h := range hits[:min(len(hits), netflixTMDBAltLookups)] {
			if !possible(h) || !m.wait(ctx) {
				continue
			}
			alts, err := m.tmdb.AlternativeTitles(ctx, kind, strconv.Itoa(h.ID))
			if err != nil {
				m.log.Debug("tmdb alternative titles failed", slog.Int("tmdb_id", h.ID), slog.Any("err", err))
				continue
			}
			for _, a := range alts {
				if normalizeTitle(a) == want {
					exact = append(exact, tmdbHitCandidate(h, MatchSourceTMDBNetflixAltTitle))
					break
				}
			}
		}
	}
	var out []titleCandidate
	for _, c := range exact[:min(len(exact), netflixTMDBCandidates)] {
		if !m.wait(ctx) {
			return nil
		}
		countries, err := m.tmdb.WatchProviderCountries(ctx, kind, c.TmdbID, client.TMDBNetflixProviderIDs)
		if err != nil {
			m.log.Debug("tmdb watch providers failed", slog.String("tmdb_id", c.TmdbID), slog.Any("err", err))
			continue
		}
		if len(countries) > 0 {
			out = append(out, c)
		}
	}
	return out
}

func tmdbHitCandidate(h client.TMDBSearchResult, source string) titleCandidate {
	return titleCandidate{Title: h.Title, OriginalTitle: h.OriginalTitle, Year: h.Year, TmdbID: strconv.Itoa(h.ID), Source: source}
}
