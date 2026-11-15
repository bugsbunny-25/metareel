package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

const maxImportCandidates = 6

// matchItem matches one staged title to TMDB.
//
//   - A title with a TMDB id (Suite movies) is checked to exist.
//   - A title with a TheTVDB id (every show; some Suite movies) goes through
//     TMDB's /find.
//   - A movie the user already logged at exactly the same second (from
//     another import of the same history) is taken to be that movie.
//   - Otherwise it is searched for by name and only accepted on an exact
//     release date, the same name in the same year (one year either side),
//     or as the only hit released within a week.
//   - A movie named in a non-Latin script is looked for by its original
//     language, release date and runtime (discoverByRelease).
//
// Anything less certain is left unmatched with the hits as candidates.
//
// A matched show also has its episodes matched (mapEpisodes).
func (s *ImportService) matchItem(ctx context.Context, userID int64, it repository.UserImportItem) (repository.ImportItemMatch, error) {
	var d importData
	if err := json.Unmarshal([]byte(it.Data), &d); err != nil {
		return repository.ImportItemMatch{}, fmt.Errorf("decode import item %d: %w", it.ID, err)
	}
	year := 0
	if it.Year != nil {
		year = int(*it.Year)
	}
	releaseDate := ""
	if it.ReleaseDate != nil {
		releaseDate = *it.ReleaseDate
	}

	tmdbID, method := "", ""
	if ref, ok := strings.CutPrefix(it.SourceRef, "tmdb:"); ok && it.Kind == "movie" {
		if _, err := s.tracking.titles.Resolve(ctx, "movie", ref); err == nil {
			tmdbID, method = ref, "tmdb_id"
		} else if !errors.Is(err, ErrTrackedTitleUnknown) {
			return repository.ImportItemMatch{}, err
		}
	}
	if tmdbID == "" && d.TvdbID != "" {
		found, err := s.tmdb.FindByTVDB(ctx, d.TvdbID)
		if err != nil {
			return repository.ImportItemMatch{}, fmt.Errorf("%w: %v", ErrUpstream, err)
		}
		list := found.TV
		if it.Kind == "movie" {
			list = found.Movies
		}
		if len(list) > 0 {
			tmdbID, method = list[0].ID, "tvdb"
		}
	}

	// A movie the user already logged at exactly the same second is the
	// same movie: apps that import another app's history keep its
	// timestamps, and this finds titles a translated name cannot.
	if tmdbID == "" && it.Kind == "movie" && len(d.Watches) > 0 {
		times := make([]string, 0, len(d.Watches))
		for _, w := range d.Watches {
			times = append(times, repository.FormatTimestamp(time.Unix(w.At, 0)))
		}
		ids, err := s.repo.MoviesWatchedAt(ctx, userID, times)
		if err != nil {
			return repository.ImportItemMatch{}, err
		}
		if len(ids) == 1 {
			tmdbID, method = ids[0], "history"
		}
	}

	var hits []TitleSearchResult
	if tmdbID == "" {
		var err error
		hits, err = s.searchTitle(ctx, it.Kind, it.Title, year)
		if err != nil {
			return repository.ImportItemMatch{}, err
		}
		if best := pickMatch(hits, it.Title, year, releaseDate); best != nil {
			tmdbID, method = best.ID, "search"
		}
	}

	// A movie TV Time names in the app's language (Telugu, Tamil, Hindi...)
	// is often not findable by that name; its script gives the original
	// language, and with the release date and runtime that is usually one
	// film. Several are offered as candidates instead.
	if tmdbID == "" && it.Kind == "movie" && releaseDate != "" {
		if lang := scriptLanguage(it.Title); lang != "" {
			found, sure, err := s.discoverByRelease(ctx, lang, releaseDate, d.RuntimeMinutes)
			if err != nil {
				return repository.ImportItemMatch{}, err
			}
			if sure != nil {
				tmdbID, method = sure.ID, "release"
			}
			hits = append(found, hits...)
		}
	}

	m := repository.ImportItemMatch{Data: it.Data}
	unmatched := func() (repository.ImportItemMatch, error) {
		m.Status = ItemUnmatched
		if len(hits) > maxImportCandidates {
			hits = hits[:maxImportCandidates]
		}
		if len(hits) > 0 {
			c, err := json.Marshal(hits)
			if err != nil {
				return repository.ImportItemMatch{}, err
			}
			cs := string(c)
			m.Candidates = &cs
		}
		return m, nil
	}
	if tmdbID == "" {
		return unmatched()
	}

	// Cache the title (for the review list and the diary) and, for a show,
	// match its episodes.
	if _, err := s.tracking.titles.Resolve(ctx, it.Kind, tmdbID); err != nil {
		if errors.Is(err, ErrTrackedTitleUnknown) {
			return unmatched()
		}
		return repository.ImportItemMatch{}, err
	}
	if it.Kind == "tv" {
		mapped, err := s.mapEpisodes(ctx, userID, tmdbID, &d)
		if err != nil {
			return repository.ImportItemMatch{}, err
		}
		data, err := json.Marshal(d)
		if err != nil {
			return repository.ImportItemMatch{}, err
		}
		m.Data, m.EpisodesMapped = string(data), mapped
	}
	m.Status, m.TmdbID, m.Method = ItemMatched, &tmdbID, &method
	return m, nil
}

// searchTitle searches TMDB for a title: movies within their year first,
// then without it, because exports sometimes carry a regional release date.
func (s *ImportService) searchTitle(ctx context.Context, kind, title string, year int) ([]TitleSearchResult, error) {
	query, nameYear := splitTitleYear(title)
	if year == 0 {
		year = nameYear
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	var hits []client.TMDBSearchResult
	var err error
	if kind == "movie" {
		if year > 0 {
			hits, err = s.tmdb.SearchMovieYear(ctx, query, year)
		}
		if err == nil && len(hits) == 0 {
			hits, err = s.tmdb.SearchMovie(ctx, query)
		}
	} else {
		hits, err = s.tmdb.SearchTV(ctx, query)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: tmdb search: %v", ErrUpstream, err)
	}
	out := make([]TitleSearchResult, 0, len(hits))
	for _, h := range hits {
		out = append(out, TitleSearchResult{
			Kind: kind, ID: fmt.Sprint(h.ID), Title: h.Title, OriginalTitle: h.OriginalTitle,
			Year: h.Year, ReleaseDate: h.ReleaseDate, PosterPath: h.PosterPath,
		})
	}
	return out, nil
}

// pickMatch returns the search hit that is certainly the title, or nil.
func pickMatch(hits []TitleSearchResult, title string, year int, releaseDate string) *TitleSearchResult {
	name, nameYear := splitTitleYear(title)
	if year == 0 {
		year = nameYear
	}
	want := normalizeTitle(name)
	if releaseDate != "" {
		for i := range hits {
			if hits[i].ReleaseDate == releaseDate {
				return &hits[i]
			}
		}
	}
	if want == "" {
		return nearRelease(hits, releaseDate)
	}
	sameName := func(h TitleSearchResult) bool {
		return normalizeTitle(h.Title) == want || normalizeTitle(h.OriginalTitle) == want
	}
	if year == 0 {
		// No year to tell remakes apart: only a single hit of that name.
		var only *TitleSearchResult
		for i := range hits {
			if sameName(hits[i]) {
				if only != nil {
					return nil
				}
				only = &hits[i]
			}
		}
		return only
	}
	for _, tolerance := range []int{0, 1} {
		for i := range hits {
			if sameName(hits[i]) && hits[i].Year != 0 && absInt(hits[i].Year-year) <= tolerance {
				return &hits[i]
			}
		}
	}
	return nearRelease(hits, releaseDate)
}

// nearRelease returns the only hit released within a week of releaseDate,
// for names that differ in punctuation or numbering ("Part One", "2nd")
// where the date still pins the film down. Two hits that close is too close
// to call.
func nearRelease(hits []TitleSearchResult, releaseDate string) *TitleSearchResult {
	want, err := time.Parse("2006-01-02", releaseDate)
	if err != nil {
		return nil
	}
	var near *TitleSearchResult
	for i := range hits {
		got, err := time.Parse("2006-01-02", hits[i].ReleaseDate)
		if err != nil {
			continue
		}
		if d := got.Sub(want); d <= 7*24*time.Hour && d >= -7*24*time.Hour {
			if near != nil {
				return nil
			}
			near = &hits[i]
		}
	}
	return near
}

// mapEpisodes matches each episode watch of a show to a TMDB episode: by
// season and episode number when TMDB has that episode, else through the
// watch's TheTVDB episode id (for shows whose numbering differs). It returns
// how many episode watches were matched.
func (s *ImportService) mapEpisodes(ctx context.Context, userID int64, tmdbID string, d *importData) (int64, error) {
	if err := s.tracking.titles.ResolveEpisodes(ctx, tmdbID, false); err != nil {
		return 0, err
	}
	eps, err := s.repo.ShowEpisodes(ctx, userID, tmdbID, true)
	if err != nil {
		return 0, err
	}
	have := make(map[[2]int64]bool, len(eps))
	for _, e := range eps {
		have[[2]int64{e.SeasonNumber, e.EpisodeNumber}] = true
	}
	finds := 0
	byTVDB := map[string]*[2]int64{}
	mapped := int64(0)
	for i := range d.Watches {
		w := &d.Watches[i]
		w.TmdbSeason, w.TmdbEpisode = nil, nil
		if w.Season == nil && w.TvdbEpisodeID == "" {
			continue
		}
		if w.Season != nil && w.Episode != nil && have[[2]int64{*w.Season, *w.Episode}] {
			season, episode := *w.Season, *w.Episode
			w.TmdbSeason, w.TmdbEpisode = &season, &episode
			mapped++
			continue
		}
		if w.TvdbEpisodeID == "" {
			continue
		}
		hit, seen := byTVDB[w.TvdbEpisodeID]
		if !seen {
			if finds >= maxEpisodeFinds {
				continue
			}
			finds++
			found, err := s.tmdb.FindByTVDB(ctx, w.TvdbEpisodeID)
			if err != nil {
				return 0, fmt.Errorf("%w: %v", ErrUpstream, err)
			}
			for _, e := range found.Episodes {
				key := [2]int64{int64(e.SeasonNumber), int64(e.EpisodeNumber)}
				if e.ShowID == tmdbID && have[key] {
					hit = &key
					break
				}
			}
			byTVDB[w.TvdbEpisodeID] = hit
		}
		if hit != nil {
			season, episode := hit[0], hit[1]
			w.TmdbSeason, w.TmdbEpisode = &season, &episode
			mapped++
		}
	}
	return mapped, nil
}

// importResult is what writing one title did.
type importResult struct {
	Added      int `json:"added"`
	Duplicates int `json:"duplicates"`
	Unmapped   int `json:"unmapped"`
}

// commitItem writes one matched title: its watches (skipping any the user
// already has) and then its status and favourite flag.
func (s *ImportService) commitItem(ctx context.Context, imp repository.UserImport, it repository.UserImportItem) error {
	var d importData
	if err := json.Unmarshal([]byte(it.Data), &d); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	kind, tmdbID, userID := it.Kind, *it.TmdbID, imp.UserID

	snap, err := s.tracking.titles.Resolve(ctx, kind, tmdbID)
	if err != nil {
		return err
	}
	var episodes map[[2]int64]repository.TrackedEpisode
	if kind == "tv" {
		if err := s.tracking.titles.ResolveEpisodes(ctx, tmdbID, false); err != nil {
			return err
		}
		eps, err := s.repo.ShowEpisodes(ctx, userID, tmdbID, true)
		if err != nil {
			return err
		}
		episodes = make(map[[2]int64]repository.TrackedEpisode, len(eps))
		for _, e := range eps {
			episodes[[2]int64{e.SeasonNumber, e.EpisodeNumber}] = e
		}
	}

	prev, err := s.repo.UserTitle(ctx, userID, kind, tmdbID)
	if err != nil {
		return err
	}
	prevJSON := "null"
	if prev != nil {
		b, err := json.Marshal(savedState{Status: prev.Status, Favorite: prev.Favorite,
			WatchlistPosition: prev.WatchlistPosition, WatchlistNote: prev.WatchlistNote})
		if err != nil {
			return err
		}
		prevJSON = string(b)
	}

	existing, err := s.repo.TitleWatchTimes(ctx, userID, kind, tmdbID)
	if err != nil {
		return err
	}
	seen := map[[2]int64][]time.Time{}
	for _, w := range existing {
		t, err := repository.ParseTimestamp(w.EndedAt)
		if err != nil {
			continue
		}
		key := [2]int64{w.SeasonNumber, w.EpisodeNumber}
		seen[key] = append(seen[key], t)
	}

	// First viewings before rewatches, so a rewatch is judged against them.
	ordered := append([]importWatch(nil), d.Watches...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Rewatch != ordered[j].Rewatch {
			return !ordered[i].Rewatch
		}
		return ordered[i].At < ordered[j].At
	})

	res := importResult{}
	var watches []repository.NewWatch
	for _, w := range ordered {
		key := [2]int64{-1, -1}
		var ep *repository.TrackedEpisode
		if kind == "tv" {
			if w.TmdbSeason == nil || w.TmdbEpisode == nil {
				res.Unmapped++
				continue
			}
			key = [2]int64{*w.TmdbSeason, *w.TmdbEpisode}
			e, ok := episodes[key]
			if !ok {
				res.Unmapped++
				continue
			}
			ep = &e
		}
		at := time.Unix(w.At, 0).UTC()
		if isDuplicateWatch(seen[key], at, w.Rewatch) {
			res.Duplicates++
			continue
		}
		in := WatchInput{Kind: kind, TmdbID: tmdbID, EndedAt: at.Format(time.RFC3339), TZ: imp.TZ, Precision: PrecisionMinute}
		if w.Day {
			in.Precision = PrecisionDay
		}
		if ep != nil {
			in.SeasonNumber, in.EpisodeNumber = &ep.SeasonNumber, &ep.EpisodeNumber
		}
		nw, err := s.tracking.buildWatch(in, snap, ep)
		if err != nil {
			return err
		}
		importID := imp.ID
		nw.Source, nw.ImportID = imp.Source, &importID
		watches = append(watches, nw)
		seen[key] = append(seen[key], at)
		res.Added++
	}

	resultJSON, err := json.Marshal(res)
	if err != nil {
		return err
	}
	if err := s.repo.CommitImportItem(ctx, userID, it, watches, string(resultJSON), prevJSON); err != nil {
		return err
	}
	return s.applyImportedState(ctx, userID, kind, tmdbID, d)
}

// isDuplicateWatch reports whether a watch at `at` is one the user already
// has. A first viewing is a duplicate of any watch of the same movie or
// episode (another app may have stamped it differently); a rewatch only of
// one at nearly the same time.
func isDuplicateWatch(existing []time.Time, at time.Time, rewatch bool) bool {
	if !rewatch {
		return len(existing) > 0
	}
	for _, t := range existing {
		if d := t.Sub(at); d < rewatchWindow && d > -rewatchWindow {
			return true
		}
	}
	return false
}

// applyImportedState carries a title's flags over: favourite, and a status
// unless the user already chose one themselves (dropped, on hold, not
// interested), which an import never overrides.
func (s *ImportService) applyImportedState(ctx context.Context, userID int64, kind, tmdbID string, d importData) error {
	cur, err := s.repo.UserTitle(ctx, userID, kind, tmdbID)
	if err != nil {
		return err
	}
	st := repository.UserTitleState{}
	watched := int64(0)
	if cur != nil {
		st = repository.UserTitleState{Status: cur.Status, Favorite: cur.Favorite,
			WatchlistPosition: cur.WatchlistPosition, WatchlistNote: cur.WatchlistNote}
		watched = cur.WatchCount
	}
	auto := st.Status == StatusNone || st.Status == StatusWatchlist || st.Status == StatusWatching || st.Status == StatusCompleted

	want := st.Status
	switch {
	case !auto:
	case kind == "tv" && d.Stopped:
		want = StatusDropped
	case kind == "tv" && d.Paused:
		want = StatusOnHold
	case d.Watchlist && watched == 0 && st.Status == StatusNone:
		want = StatusWatchlist
	}
	changed := want != st.Status || (d.Favorite && !st.Favorite)
	if changed {
		if want == StatusWatchlist && st.Status != StatusWatchlist {
			pos, err := s.repo.NextWatchlistPosition(ctx, userID)
			if err != nil {
				return err
			}
			st.WatchlistPosition = &pos
		}
		if want != StatusWatchlist {
			st.WatchlistPosition = nil
		}
		st.Status = want
		st.Favorite = st.Favorite || d.Favorite
		if err := s.repo.SaveUserTitleState(ctx, userID, kind, tmdbID, st); err != nil {
			return err
		}
	}
	// Watched episodes move a show between watching and completed, as when
	// they are marked by hand.
	if kind == "tv" && watched > 0 && (want == StatusNone || want == StatusWatching || want == StatusCompleted || want == StatusWatchlist) {
		return s.tracking.advanceShowStatus(ctx, userID, tmdbID, false)
	}
	return nil
}

// scriptLanguage guesses a title's original language from its script, for
// scripts that belong to one language TMDB knows by that code. Scripts shared
// by many languages (Latin, Cyrillic, Arabic, Han) give "".
func scriptLanguage(title string) string {
	counts := map[string]int{}
	for _, r := range title {
		switch {
		case unicode.Is(unicode.Telugu, r):
			counts["te"]++
		case unicode.Is(unicode.Tamil, r):
			counts["ta"]++
		case unicode.Is(unicode.Malayalam, r):
			counts["ml"]++
		case unicode.Is(unicode.Kannada, r):
			counts["kn"]++
		case unicode.Is(unicode.Devanagari, r):
			counts["hi"]++
		case unicode.Is(unicode.Bengali, r):
			counts["bn"]++
		case unicode.Is(unicode.Gurmukhi, r):
			counts["pa"]++
		case unicode.Is(unicode.Gujarati, r):
			counts["gu"]++
		case unicode.Is(unicode.Thai, r):
			counts["th"]++
		case unicode.Is(unicode.Hangul, r):
			counts["ko"]++
		case unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r):
			counts["ja"]++
		}
	}
	best, n := "", 0
	for lang, c := range counts {
		if c > n {
			best, n = lang, c
		}
	}
	return best
}

// discoverByRelease finds movies in lang released (anywhere) within three
// days of releaseDate. With a runtime, the only one within ten minutes of it
// is returned as sure; the rest are candidates.
func (s *ImportService) discoverByRelease(ctx context.Context, lang, releaseDate string, runtime int) ([]TitleSearchResult, *TitleSearchResult, error) {
	day, err := time.Parse("2006-01-02", releaseDate)
	if err != nil {
		return nil, nil, nil
	}
	in := client.DiscoverMoviesInput{
		Language: lang,
		From:     day.AddDate(0, 0, -3).Format("2006-01-02"),
		To:       day.AddDate(0, 0, 3).Format("2006-01-02"),
	}
	toResults := func(hits []client.TMDBSearchResult) []TitleSearchResult {
		out := make([]TitleSearchResult, 0, len(hits))
		for _, h := range hits {
			out = append(out, TitleSearchResult{Kind: "movie", ID: fmt.Sprint(h.ID), Title: h.Title,
				OriginalTitle: h.OriginalTitle, Year: h.Year, ReleaseDate: h.ReleaseDate, PosterPath: h.PosterPath})
		}
		return out
	}
	var sure *TitleSearchResult
	if runtime > 0 {
		narrow := in
		narrow.RuntimeMin, narrow.RuntimeMax = max(runtime-10, 1), runtime+10
		hits, err := s.tmdb.DiscoverMovies(ctx, narrow)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: tmdb discover: %v", ErrUpstream, err)
		}
		if len(hits) == 1 {
			sure = &toResults(hits)[0]
		}
	}
	hits, err := s.tmdb.DiscoverMovies(ctx, in)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: tmdb discover: %v", ErrUpstream, err)
	}
	return toResults(hits), sure, nil
}
