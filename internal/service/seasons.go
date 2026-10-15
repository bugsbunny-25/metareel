package service

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// SeasonMapper maps Netflix season titles ("Wednesday: Season 2") to TMDB
// season numbers and infers the season of FlixPatrol TV rankings, whose
// charts name only the show.
type SeasonMapper struct {
	log  *slog.Logger
	repo *repository.ImportsRepository
}

func NewSeasonMapper(log *slog.Logger, repo *repository.ImportsRepository) *SeasonMapper {
	return &SeasonMapper{log: log, repo: repo}
}

// SeasonMapReport summarises a season mapping pass.
type SeasonMapReport struct {
	Seasons         int `json:"netflix_seasons"`
	SeasonsChanged  int `json:"netflix_seasons_changed"`
	SeasonsUnmapped int `json:"netflix_seasons_unmapped"`
	Rankings        int `json:"tv_rankings"`
	RankingsChanged int `json:"tv_rankings_changed"`
}

// MapNetflixSeasons (re)maps the seasons of one Netflix title, or of all
// (netflixTitleID 0, which also adds rows for new season titles). Seasons
// fixed by hand are never changed.
func (m *SeasonMapper) MapNetflixSeasons(ctx context.Context, netflixTitleID int64, rep *SeasonMapReport) error {
	if netflixTitleID == 0 {
		if err := m.repo.EnsureNetflixSeasons(ctx); err != nil {
			return err
		}
	}
	seasons, err := m.repo.NetflixSeasonsToMap(ctx, netflixTitleID)
	if err != nil {
		return err
	}
	var updates []repository.SeasonUpdate
	for _, s := range seasons {
		var (
			num    *int64
			source string
		)
		if s.TmdbKind == "movie" {
			// Listed by Netflix as TV but a TMDB movie (a special): no seasons.
			if s.SeasonNumber != nil || derefStr(s.SeasonSource) != "" {
				updates = append(updates, repository.SeasonUpdate{ID: s.ID})
			}
			continue
		}
		num, source = ResolveNetflixSeason(s.ShowTitle, s.SeasonTitle, s.FirstWeek, s.TmdbSeasons)
		if num == nil {
			rep.SeasonsUnmapped++
		}
		if !sameInt(num, s.SeasonNumber) || source != derefStr(s.SeasonSource) {
			updates = append(updates, repository.SeasonUpdate{ID: s.ID, Number: num, Source: source})
		}
	}
	rep.Seasons += len(seasons)
	rep.SeasonsChanged += len(updates)
	return m.repo.SaveNetflixSeasonMappings(ctx, updates)
}

// TagRankings sets the season of FlixPatrol TV rankings from `from` on
// whose chart did not name one: on Netflix charts the best-ranked mapped
// season of the show in Netflix's official Top 10 that week (the country's,
// else global); otherwise the latest TMDB season aired by that date.
func (m *SeasonMapper) TagRankings(ctx context.Context, from time.Time, rep *SeasonMapReport) error {
	rankings, err := m.repo.TVRankingsForSeasons(ctx, from)
	if err != nil {
		return err
	}
	if len(rankings) == 0 {
		return nil
	}
	tmdb, err := m.repo.TmdbTVSeasons(ctx)
	if err != nil {
		return err
	}
	ranks, imported, err := m.repo.NetflixSeasonRanks(ctx, repository.FormatDate(from))
	if err != nil {
		return err
	}
	best := map[string]repository.NetflixSeasonRank{} // tmdb|country|week
	for _, r := range ranks {
		k := r.TmdbID + "|" + r.Country + "|" + r.Week
		if b, ok := best[k]; !ok || r.Rank < b.Rank {
			best[k] = r
		}
	}

	var updates []repository.SeasonUpdate
	for _, r := range rankings {
		var (
			num    *int64
			source string
		)
		if r.TmdbID != "" {
			if r.Provider == "netflix" {
				week := netflixWeekOf(r.Date)
				if b, ok := best[r.TmdbID+"|"+r.Country+"|"+week]; ok {
					num, source = int64p(b.Season), repository.SeasonSourceNetflix
				} else if b, ok := best[r.TmdbID+"||"+week]; ok && !imported[r.Country+"|"+week] {
					// The country's own week is not imported: use the global list.
					num, source = int64p(b.Season), repository.SeasonSourceNetflix
				}
			}
			if num == nil {
				if n, ok := latestAiredSeason(tmdb[r.TmdbID], r.Date); ok {
					num, source = int64p(n), repository.SeasonSourceTmdbAir
				}
			}
		}
		if !sameInt(num, r.SeasonNumber) || source != r.SeasonSource {
			updates = append(updates, repository.SeasonUpdate{ID: r.ID, Number: num, Source: source})
		}
	}
	rep.Rankings += len(rankings)
	rep.RankingsChanged += len(updates)
	return m.repo.SaveRankingSeasons(ctx, updates)
}

// Refresh maps every Netflix season, then tags TV rankings from `from`
// (zero = all), logging a summary to runLog.
func (m *SeasonMapper) Refresh(ctx context.Context, from time.Time, runLog func(level, msg string)) (*SeasonMapReport, error) {
	rep := &SeasonMapReport{}
	if err := m.MapNetflixSeasons(ctx, 0, rep); err != nil {
		return rep, fmt.Errorf("map netflix seasons: %w", err)
	}
	if err := m.TagRankings(ctx, from, rep); err != nil {
		return rep, fmt.Errorf("tag ranking seasons: %w", err)
	}
	if runLog != nil {
		runLog("info", fmt.Sprintf("Seasons: %d Netflix season titles (%d updated, %d without a number); %d TV rankings checked, %d updated",
			rep.Seasons, rep.SeasonsChanged, rep.SeasonsUnmapped, rep.Rankings, rep.RankingsChanged))
	}
	return rep, nil
}

// RefreshTitle remaps one Netflix title's seasons and re-tags the rankings
// that may depend on them (the last year).
func (m *SeasonMapper) RefreshTitle(ctx context.Context, netflixTitleID int64) error {
	rep := &SeasonMapReport{}
	if err := m.MapNetflixSeasons(ctx, netflixTitleID, rep); err != nil {
		return err
	}
	return m.TagRankings(ctx, time.Now().UTC().AddDate(-1, 0, 0), rep)
}

var (
	// "Season 2", "Series 3", localised forms; anywhere in the suffix
	// ("Homicide: New York: Season 2", "The Uncanny Counter: Season 2: Counter Punch").
	seasonWordRe = regexp.MustCompile(`(?i)(?:^|[\s:])(?:season|series|temporada|saison|staffel|stagione|seizoen|sezon|sezonul|シーズン|시즌|第)\s*(\d{1,3})\b`)
	// "S3: Dallas"
	seasonShortRe = regexp.MustCompile(`(?i)^s(\d{1,2})\b`)
	// "Limited Series", "Miniseries"
	limitedRe = regexp.MustCompile(`(?i)^(?:limited series|miniseries|mini-series)$`)
	// "Part 2", "Volume 4", "Chapter 7", "Collection 14", "Class 2", "1st STAGE", "2.", "5"
	numberedRe = regexp.MustCompile(`(?i)^(?:(?:part|volume|vol\.?|chapter|collection|class|book|cour|stage|round)\s*(\d{1,3})\b|(\d{1,2})(?:st|nd|rd|th)?\b)`)
	romanRe    = regexp.MustCompile(`^(II|III|IV|V|VI|VII|VIII|IX|X)$`)
	// "September 28, 2026" (dated episodes such as WWE Raw)
	suffixDateRe = regexp.MustCompile(`(?i)\b(January|February|March|April|May|June|July|August|September|October|November|December) (\d{1,2}), (\d{4})\b`)
)

var romanNumbers = map[string]int64{"II": 2, "III": 3, "IV": 4, "V": 5, "VI": 6, "VII": 7, "VIII": 8, "IX": 9, "X": 10}

// seasonSuffix is a season title without its show title ("Wednesday:
// Season 2" → "Season 2"); the whole title if it does not start with it.
func seasonSuffix(showTitle, seasonTitle string) string {
	st := strings.TrimSpace(seasonTitle)
	show := strings.TrimSpace(showTitle)
	if show != "" && len(st) > len(show) && strings.EqualFold(st[:len(show)], show) && !startsWithWordChar(st[len(show):]) {
		rest := strings.TrimLeft(st[len(show):], " :-–—.")
		if rest != "" {
			return strings.TrimSpace(rest)
		}
	}
	return st
}

// startsWithWordChar reports whether s starts with a letter or digit (the
// show title was only a prefix of a longer word: "Monster" in "Monsters").
func startsWithWordChar(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// airedSeasonWindow bounds how long after a TMDB season aired an otherwise
// unrecognised season title is assumed to be that season.
const airedSeasonWindow = 120 * 24 * time.Hour

// ResolveNetflixSeason maps a Netflix season title to a season number, in
// order of confidence: an explicit "Season N" / "S N" / "Limited Series";
// a TMDB season of that name; "Part N" / "Volume N" / a bare number (when
// TMDB has that season or is unknown); the TMDB season that aired last
// before the title first charted (or before the date in the title); the only
// season of a single-season show. nil when nothing fits.
func ResolveNetflixSeason(showTitle, seasonTitle, firstWeek string, tmdb []repository.TmdbSeason) (*int64, string) {
	suffix := seasonSuffix(showTitle, seasonTitle)
	if m := seasonWordRe.FindStringSubmatch(suffix); m != nil {
		return atoi64p(m[1]), repository.SeasonSourceParsed
	}
	if m := seasonShortRe.FindStringSubmatch(suffix); m != nil {
		return atoi64p(m[1]), repository.SeasonSourceParsed
	}
	if limitedRe.MatchString(suffix) {
		return int64p(1), repository.SeasonSourceParsed
	}
	if n, ok := tmdbSeasonNamed(tmdb, suffix, seasonTitle); ok {
		return int64p(n), repository.SeasonSourceTmdbName
	}
	if n, ok := numberedSeason(suffix); ok && (tmdb == nil || hasSeason(tmdb, n)) {
		return int64p(n), repository.SeasonSourceNumber
	}
	if tmdb != nil {
		if m := suffixDateRe.FindStringSubmatch(suffix); m != nil {
			if d, err := time.Parse("January 2, 2006", m[1]+" "+m[2]+", "+m[3]); err == nil {
				if n, ok := latestAiredSeason(tmdb, repository.FormatDate(d)); ok {
					return int64p(n), repository.SeasonSourceTmdbAir
				}
			}
		}
		if n, ok := seasonAiredBefore(tmdb, firstWeek, airedSeasonWindow); ok {
			return int64p(n), repository.SeasonSourceTmdbAir
		}
		if regular := regularSeasons(tmdb); len(regular) == 1 {
			return int64p(int64(regular[0].Number)), repository.SeasonSourceTmdbSingle
		}
	}
	return nil, ""
}

// tmdbSeasonNamed finds the TMDB season named like the suffix (or the whole
// season title): an exact match, else the only season whose name contains
// it or is contained in it (names of at least 6 characters, not generic
// "Season N" names).
func tmdbSeasonNamed(tmdb []repository.TmdbSeason, suffix, full string) (int64, bool) {
	want, wantFull := normalizeTitle(suffix), normalizeTitle(full)
	if want == "" {
		return 0, false
	}
	for _, s := range tmdb {
		if n := normalizeTitle(s.Name); n != "" && (n == want || n == wantFull) {
			return int64(s.Number), true
		}
	}
	var found []int64
	for _, s := range regularSeasons(tmdb) {
		n := normalizeTitle(s.Name)
		if len(n) < 6 || seasonWordRe.MatchString(s.Name) || len(want) < 6 {
			continue
		}
		if strings.Contains(want, n) || strings.Contains(n, want) {
			found = append(found, int64(s.Number))
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return 0, false
}

func numberedSeason(suffix string) (int64, bool) {
	if m := numberedRe.FindStringSubmatch(suffix); m != nil {
		v := m[1]
		if v == "" {
			v = m[2]
		}
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n, true
		}
	}
	if m := romanRe.FindStringSubmatch(strings.TrimSpace(suffix)); m != nil {
		return romanNumbers[m[1]], true
	}
	return 0, false
}

func regularSeasons(tmdb []repository.TmdbSeason) []repository.TmdbSeason {
	out := make([]repository.TmdbSeason, 0, len(tmdb))
	for _, s := range tmdb {
		if s.Number > 0 {
			out = append(out, s)
		}
	}
	return out
}

func hasSeason(tmdb []repository.TmdbSeason, n int64) bool {
	for _, s := range tmdb {
		if int64(s.Number) == n {
			return true
		}
	}
	return false
}

// latestAiredSeason is the highest-numbered regular season that had aired by
// date (YYYY-MM-DD).
func latestAiredSeason(tmdb []repository.TmdbSeason, date string) (int64, bool) {
	var best int64
	for _, s := range regularSeasons(tmdb) {
		if s.AirDate != "" && s.AirDate <= date && int64(s.Number) > best {
			best = int64(s.Number)
		}
	}
	return best, best > 0
}

// seasonAiredBefore is the latest season aired by the end of the week the
// title first charted, if it aired at most window before.
func seasonAiredBefore(tmdb []repository.TmdbSeason, firstWeek string, window time.Duration) (int64, bool) {
	week, err := time.Parse(repository.DateLayout, firstWeek)
	if err != nil {
		return 0, false
	}
	n, ok := latestAiredSeason(tmdb, firstWeek)
	if !ok {
		return 0, false
	}
	for _, s := range tmdb {
		if int64(s.Number) != n {
			continue
		}
		aired, err := time.Parse(repository.DateLayout, s.AirDate)
		if err != nil || week.Sub(aired) > window {
			return 0, false
		}
	}
	return n, true
}

// netflixWeekOf returns the Sunday ending the Netflix week (Monday–Sunday)
// that contains date (YYYY-MM-DD).
func netflixWeekOf(date string) string {
	d, err := time.Parse(repository.DateLayout, date)
	if err != nil {
		return ""
	}
	return repository.FormatDate(d.AddDate(0, 0, (7-int(d.Weekday()))%7))
}

func atoi64p(s string) *int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

func int64p(n int64) *int64 { return &n }

func sameInt(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
