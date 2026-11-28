package service

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Sources an import can come from.
const (
	ImportSourceTVTime = "tvtime"
	ImportSourceSuite  = "suite"
)

const (
	// maxImportFileBytes bounds what is read out of one file of an export,
	// so a zip bomb cannot exhaust memory. Real exports are far smaller.
	maxImportFileBytes = 128 << 20
	// maxImportTitles bounds how many titles one import may stage.
	maxImportTitles = 20000
	// bulkMarkMinEpisodes is how many episodes of one show have to share a
	// timestamp to the second before they count as one "mark as watched".
	bulkMarkMinEpisodes = 3
)

// importedTitle is one title read from an export, before it is matched.
type importedTitle struct {
	Kind        string // movie | tv
	SourceRef   string // tvdb:<id> | tmdb:<id> | tvtime:<uuid> | suite:<id>
	Title       string
	Year        int
	ReleaseDate string // YYYY-MM-DD
	Data        importData
}

// importData is what an export says about a title, kept as JSON on the
// staged item until the import is written.
type importData struct {
	Watches []importWatch `json:"watches,omitempty"`
	// Watchlist: wanted but not started. Favorite: marked as a favourite.
	// Stopped: the user gave up on it. Paused: TV Time's "for later" on a
	// show they had started.
	Watchlist bool `json:"watchlist,omitempty"`
	Favorite  bool `json:"favorite,omitempty"`
	Stopped   bool `json:"stopped,omitempty"`
	Paused    bool `json:"paused,omitempty"`
	// TvdbID is the title's TheTVDB id when the export has one.
	TvdbID         string `json:"tvdb_id,omitempty"`
	RuntimeMinutes int    `json:"runtime_minutes,omitempty"`
}

// importWatch is one watch from an export.
type importWatch struct {
	At int64 `json:"at"` // unix seconds, UTC
	// Day: the watch was part of a bulk mark, so only its day is known.
	Day     bool `json:"day,omitempty"`
	Rewatch bool `json:"rewatch,omitempty"`
	// The export's own episode numbering and TheTVDB episode id.
	Season        *int64 `json:"s,omitempty"`
	Episode       *int64 `json:"e,omitempty"`
	TvdbEpisodeID string `json:"tvdb_ep,omitempty"`
	// The TMDB episode it was matched to.
	TmdbSeason  *int64 `json:"tmdb_s,omitempty"`
	TmdbEpisode *int64 `json:"tmdb_e,omitempty"`
}

func (d importData) episodeCount() int64 {
	n := int64(0)
	for _, w := range d.Watches {
		if w.Season != nil || w.TvdbEpisodeID != "" {
			n++
		}
	}
	return n
}

// importParse is a parsed export.
type importParse struct {
	Source  string
	Titles  []importedTitle
	Summary map[string]any
}

// parseExport reads a TV Time (GDPR data request) or Suite export, both zip
// files, and works out which one it is from the files inside.
func parseExport(body []byte) (importParse, error) {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return importParse{}, &ValidationError{Message: "the upload is not a zip file; upload the export zip as you downloaded it"}
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := path.Base(f.Name)
		if strings.HasPrefix(name, "._") { // macOS resource forks
			continue
		}
		if _, dup := files[name]; !dup {
			files[name] = f
		}
	}
	var p importParse
	switch {
	case files["manifest.json"] != nil && (files["shows.ndjson"] != nil || files["movies.ndjson"] != nil):
		p, err = parseSuite(files)
	case files["tracking-prod-records-v2.csv"] != nil || files["tracking-prod-records.csv"] != nil:
		p, err = parseTVTime(files)
	default:
		return importParse{}, &ValidationError{Message: "this zip is neither a TV Time data export nor a Suite export"}
	}
	if err != nil {
		return importParse{}, err
	}
	if len(p.Titles) == 0 {
		return importParse{}, &ValidationError{Message: "the export has no movies or shows in it"}
	}
	if len(p.Titles) > maxImportTitles {
		return importParse{}, &ValidationError{Message: fmt.Sprintf("the export has %d titles; at most %d can be imported at once", len(p.Titles), maxImportTitles)}
	}
	sort.SliceStable(p.Titles, func(i, j int) bool {
		if p.Titles[i].Kind != p.Titles[j].Kind {
			return p.Titles[i].Kind < p.Titles[j].Kind
		}
		return p.Titles[i].SourceRef < p.Titles[j].SourceRef
	})
	return p, nil
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, &ValidationError{Message: fmt.Sprintf("can't open %s in the zip: %v", f.Name, err)}
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxImportFileBytes+1))
	if err != nil {
		return nil, &ValidationError{Message: fmt.Sprintf("can't read %s in the zip: %v", f.Name, err)}
	}
	if len(data) > maxImportFileBytes {
		return nil, &ValidationError{Message: fmt.Sprintf("%s is larger than %d MB", f.Name, maxImportFileBytes>>20)}
	}
	return data, nil
}

// readCSV reads a CSV file of the export into one map per row, keyed by the
// header. A file the export doesn't have reads as no rows.
func readCSV(files map[string]*zip.File, name string) ([]map[string]string, error) {
	f := files[name]
	if f == nil {
		return nil, nil
	}
	data, err := readZipFile(f)
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	header, err := r.Read()
	if errors.Is(err, io.EOF) {
		return nil, nil
	}
	if err != nil {
		return nil, &ValidationError{Message: fmt.Sprintf("%s is not a readable CSV file: %v", name, err)}
	}
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\ufeff")
	}
	var rows []map[string]string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, &ValidationError{Message: fmt.Sprintf("%s is not a readable CSV file: %v", name, err)}
		}
		row := make(map[string]string, len(header))
		for i, h := range header {
			if i < len(rec) {
				row[h] = strings.TrimSpace(rec[i])
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

var (
	tvtimeTimestampLayout = "2006-01-02 15:04:05"
	trailingYearRe        = regexp.MustCompile(`\s*\((\d{4})\)\s*$`)
	digitsRe              = regexp.MustCompile(`^[0-9]+$`)
)

// splitTitleYear splits "Bodyguard (2018)" into "Bodyguard" and 2018, the
// way TV Time and TheTVDB tell shows of the same name apart.
func splitTitleYear(name string) (string, int) {
	m := trailingYearRe.FindStringSubmatch(name)
	if m == nil {
		return strings.TrimSpace(name), 0
	}
	y, _ := strconv.Atoi(m[1])
	return strings.TrimSpace(strings.TrimSuffix(name, m[0])), y
}

func parseTVTimeTime(v string) int64 {
	t, err := time.ParseInLocation(tvtimeTimestampLayout, strings.TrimSpace(v), time.UTC)
	if err != nil {
		return 0
	}
	return t.Unix()
}

// unixSuffix reads the unix time at the end of a TV Time range key such as
// "watch-episode-1684012083" or "watch-date-1625290976".
func unixSuffix(v string) int64 {
	_, suffix, ok := strings.CutLast(v, "-")
	if !ok || suffix == "" {
		return 0
	}
	n, err := strconv.ParseInt(suffix, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	if n > 1e12 { // milliseconds
		n /= 1000
	}
	return n
}

func importIntPtr(v string) *int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || n < 0 {
		return nil
	}
	return &n
}

// numberPair reads an episode or season number from the first of two
// columns that holds a positive number, else the first that holds a number.
func numberPair(a, b string) *int64 {
	pa, pb := importIntPtr(a), importIntPtr(b)
	switch {
	case pa != nil && *pa > 0:
		return pa
	case pb != nil && *pb > 0:
		return pb
	case pa != nil:
		return pa
	}
	return pb
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func truthy(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "1" || v == "true" || v == "yes"
}

// parseTVTime reads TV Time's GDPR data export.
//
// Shows and episodes carry TheTVDB ids (tv_show_id / s_id, episode_id).
// Movies carry only TV Time's own uuid, a name and a release date, so they
// are matched by name later. Watch history is in tracking-prod-records-v2
// (episodes) and tracking-prod-records (movies, and episodes in older
// exports); the other files add follow, archive, for-later and favourite
// flags.
func parseTVTime(files map[string]*zip.File) (importParse, error) {
	v2, err := readCSV(files, "tracking-prod-records-v2.csv")
	if err != nil {
		return importParse{}, err
	}
	v1, err := readCSV(files, "tracking-prod-records.csv")
	if err != nil {
		return importParse{}, err
	}
	followed, err := readCSV(files, "followed_tv_show.csv")
	if err != nil {
		return importParse{}, err
	}
	special, err := readCSV(files, "user_show_special_status.csv")
	if err != nil {
		return importParse{}, err
	}
	showData, err := readCSV(files, "user_tv_show_data.csv")
	if err != nil {
		return importParse{}, err
	}

	type showAcc struct {
		title                     importedTitle
		followed, archived, later bool
		seen                      map[string]bool // episode id + rewatch flag + time
		episodesSeen              map[string]bool // first viewings, by episode id
		// Per watch: TV Time's is_unitary flag (false = a bulk mark), and
		// whether the row had the flag at all.
		unitary, unitaryKnown []bool
	}
	shows := map[string]*showAcc{}
	show := func(id, name string) *showAcc {
		s := shows[id]
		if s == nil {
			title, year := splitTitleYear(name)
			s = &showAcc{
				title:        importedTitle{Kind: "tv", SourceRef: "tvdb:" + id, Title: title, Year: year, Data: importData{TvdbID: id}},
				seen:         map[string]bool{},
				episodesSeen: map[string]bool{},
			}
			shows[id] = s
		} else if s.title.Title == "" && name != "" {
			s.title.Title, s.title.Year = splitTitleYear(name)
		}
		return s
	}
	skipped := 0
	addEpisode := func(showID, name, episodeID string, season, episode *int64, at int64, rewatch bool, unitary string) {
		if !digitsRe.MatchString(showID) || at <= 0 {
			skipped++
			return
		}
		s := show(showID, name)
		key := fmt.Sprintf("%s|%t|%d", episodeID, rewatch, at)
		if s.seen[key] {
			return
		}
		// The same first viewing listed in both tracking files.
		if !rewatch && episodeID != "" && s.episodesSeen[episodeID] {
			return
		}
		s.seen[key] = true
		if !rewatch && episodeID != "" {
			s.episodesSeen[episodeID] = true
		}
		w := importWatch{At: at, Rewatch: rewatch, Season: season, Episode: episode}
		if digitsRe.MatchString(episodeID) && episodeID != "0" {
			w.TvdbEpisodeID = episodeID
		}
		s.title.Data.Watches = append(s.title.Data.Watches, w)
		s.unitary = append(s.unitary, strings.EqualFold(unitary, "true"))
		s.unitaryKnown = append(s.unitaryKnown, unitary != "")
	}

	for _, r := range v2 {
		key := r["key"]
		switch {
		case strings.HasPrefix(key, "user-series-"):
			if !digitsRe.MatchString(r["s_id"]) {
				continue
			}
			s := show(r["s_id"], r["series_name"])
			s.followed = s.followed || truthy(r["is_followed"])
			s.archived = s.archived || truthy(r["is_archived"])
			s.later = s.later || truthy(r["is_for_later"])
		case strings.HasPrefix(key, "watch-episode-"), strings.HasPrefix(key, "rewatch-episode-"):
			at := unixSuffix(r["gsi"])
			if at == 0 {
				at = parseTVTimeTime(r["created_at"])
			}
			// s_no / ep_no are the reliable pair: on some rows (rewatches,
			// newer shows) season_number and episode_number are 0.
			season := numberPair(r["s_no"], r["season_number"])
			episode := numberPair(r["ep_no"], r["episode_number"])
			addEpisode(r["s_id"], r["series_name"], firstNonEmpty(r["episode_id"], r["ep_id"]), season, episode, at,
				strings.HasPrefix(key, "rewatch-"), r["is_unitary"])
		}
	}

	type movieAcc struct {
		title importedTitle
		seen  map[int64]bool
	}
	movies := map[string]*movieAcc{}
	movie := func(r map[string]string) *movieAcc {
		id := r["uuid"]
		m := movies[id]
		if m == nil {
			rel := r["release_date"]
			if len(rel) >= 10 {
				rel = rel[:10]
			}
			if _, err := time.Parse("2006-01-02", rel); err != nil {
				rel = ""
			}
			year := 0
			if rel != "" {
				year, _ = strconv.Atoi(rel[:4])
			}
			m = &movieAcc{title: importedTitle{Kind: "movie", SourceRef: "tvtime:" + id, Title: r["movie_name"], Year: year, ReleaseDate: rel}, seen: map[int64]bool{}}
			if sec, err := strconv.Atoi(r["runtime"]); err == nil && sec > 0 {
				m.title.Data.RuntimeMinutes = (sec + 30) / 60
			}
			movies[id] = m
		}
		return m
	}
	for _, r := range v1 {
		switch {
		case r["type"] == "watch" && r["entity_type"] == "episode":
			at := unixSuffix(r["watch_date"])
			if at == 0 {
				if n, err := strconv.ParseInt(r["watch_date"], 10, 64); err == nil {
					at = n
				}
			}
			if at == 0 {
				at = parseTVTimeTime(r["created_at"])
			}
			addEpisode(r["series_id"], r["series_name"], r["episode_id"], importIntPtr(r["season_number"]), importIntPtr(r["episode_number"]), at, false, "")
		case r["type"] == "watch" && r["entity_type"] == "movie":
			if r["uuid"] == "" {
				skipped++
				continue
			}
			at := unixSuffix(r["watch_date_range_key"])
			if at == 0 {
				at = parseTVTimeTime(r["created_at"])
			}
			if at <= 0 {
				skipped++
				continue
			}
			m := movie(r)
			if m.seen[at] {
				continue
			}
			m.seen[at] = true
			// "watch-<uuid>-<n>": n counts the viewings, 0 being the first.
			rewatch := false
			if k := r["type-uuid-n"]; k != "" {
				rewatch = unixSuffix(k) > 0
			}
			m.title.Data.Watches = append(m.title.Data.Watches, importWatch{At: at, Rewatch: rewatch})
		case r["type"] == "towatch" && r["entity_type"] == "movie":
			if r["uuid"] == "" {
				continue
			}
			movie(r).title.Data.Watchlist = true
		}
	}

	for _, r := range followed {
		if !digitsRe.MatchString(r["tv_show_id"]) {
			continue
		}
		s := show(r["tv_show_id"], r["tv_show_name"])
		s.followed = s.followed || truthy(r["active"])
		s.archived = s.archived || truthy(r["archived"])
	}
	for _, r := range special {
		if !digitsRe.MatchString(r["tv_show_id"]) || r["status"] != "for_later" {
			continue
		}
		show(r["tv_show_id"], r["tv_show_name"]).later = true
	}
	for _, r := range showData {
		if !digitsRe.MatchString(r["tv_show_id"]) {
			continue
		}
		s := shows[r["tv_show_id"]]
		if s == nil {
			if !truthy(r["is_followed"]) {
				continue
			}
			s = show(r["tv_show_id"], r["tv_show_name"])
			s.followed = true
		}
		s.title.Data.Favorite = s.title.Data.Favorite || truthy(r["is_favorited"])
	}

	out := importParse{Source: ImportSourceTVTime}
	episodes, rewatches, movieWatches := 0, 0, 0
	for _, s := range shows {
		d := &s.title.Data
		hasWatches := len(d.Watches) > 0
		switch {
		case s.archived:
			d.Stopped = true
		case s.later && hasWatches:
			d.Paused = true
		case s.later || (s.followed && !hasWatches):
			d.Watchlist = true
		}
		if !hasWatches && !d.Watchlist && !d.Stopped && !d.Favorite {
			continue // unfollowed and never watched: nothing to bring over
		}
		// A watch TV Time flags as not "unitary" was a bulk mark; without the
		// flag, many episodes stamped with the same second were.
		marked := map[int64]int{}
		for _, w := range d.Watches {
			marked[w.At]++
		}
		for i := range d.Watches {
			if s.unitaryKnown[i] {
				d.Watches[i].Day = !s.unitary[i]
			} else {
				d.Watches[i].Day = marked[d.Watches[i].At] >= bulkMarkMinEpisodes
			}
			if d.Watches[i].Rewatch {
				rewatches++
			} else {
				episodes++
			}
		}
		sortWatches(d.Watches)
		out.Titles = append(out.Titles, s.title)
	}
	for _, m := range movies {
		if len(m.title.Data.Watches) > 0 {
			m.title.Data.Watchlist = false
		}
		movieWatches += len(m.title.Data.Watches)
		sortWatches(m.title.Data.Watches)
		out.Titles = append(out.Titles, m.title)
	}
	out.Summary = importSummary(out.Titles, map[string]any{
		"episode_watches": episodes, "episode_rewatches": rewatches, "movie_watches": movieWatches, "rows_skipped": skipped,
	})
	return out, nil
}

func sortWatches(ws []importWatch) {
	sort.SliceStable(ws, func(i, j int) bool { return ws[i].At < ws[j].At })
}

func importSummary(titles []importedTitle, extra map[string]any) map[string]any {
	movies, shows := 0, 0
	for _, t := range titles {
		if t.Kind == "movie" {
			movies++
		} else {
			shows++
		}
	}
	out := map[string]any{"movies": movies, "shows": shows}
	maps.Copy(out, extra)
	return out
}

// Suite export records (format "suite-export", version 1).
type suiteManifest struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
}

type suiteMovie struct {
	Title        string  `json:"title"`
	Year         int     `json:"year"`
	TvdbID       int64   `json:"tvdbId"`
	RealTmdbID   int64   `json:"realTmdbId"`
	WatchedAt    *int64  `json:"watchedAt"` // unix ms
	RewatchDates []int64 `json:"rewatchDates"`
	IsFavorite   bool    `json:"isFavorite"`
	IsHidden     bool    `json:"isHidden"`
}

type suiteShow struct {
	Title         string `json:"title"`
	TvdbID        int64  `json:"tvdbId"`
	IsFavorite    bool   `json:"isFavorite"`
	IsStopped     bool   `json:"isStopped"`
	IsWatchlisted bool   `json:"isWatchlisted"`
	Watched       []struct {
		Season        int64   `json:"season"`
		Number        int64   `json:"number"`
		TvdbEpisodeID int64   `json:"tvdbEpisodeId"`
		WatchedAt     int64   `json:"watchedAt"`
		RewatchDates  []int64 `json:"rewatchDates"`
	} `json:"watched"`
}

// readNDJSON calls fn with each non-empty line of a newline-delimited JSON
// file. A file the export doesn't have reads as no lines.
func readNDJSON(files map[string]*zip.File, name string, fn func(line []byte) error) error {
	f := files[name]
	if f == nil {
		return nil
	}
	data, err := readZipFile(f)
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	line := 0
	for sc.Scan() {
		line++
		b := bytes.TrimSpace(sc.Bytes())
		if len(b) == 0 {
			continue
		}
		if err := fn(b); err != nil {
			return &ValidationError{Message: fmt.Sprintf("%s line %d is not valid JSON: %v", name, line, err)}
		}
	}
	if err := sc.Err(); err != nil {
		return &ValidationError{Message: fmt.Sprintf("can't read %s: %v", name, err)}
	}
	return nil
}

func msToUnix(ms int64) int64 {
	if ms <= 0 {
		return 0
	}
	return ms / 1000
}

// parseSuite reads a Suite export: a manifest, movies.ndjson (with TMDB ids)
// and shows.ndjson (with TheTVDB show and episode ids).
func parseSuite(files map[string]*zip.File) (importParse, error) {
	raw, err := readZipFile(files["manifest.json"])
	if err != nil {
		return importParse{}, err
	}
	var man suiteManifest
	if err := json.Unmarshal(raw, &man); err != nil || man.Format != "suite-export" {
		return importParse{}, &ValidationError{Message: "manifest.json is not a Suite export manifest"}
	}
	if man.Version > 1 {
		return importParse{}, &ValidationError{Message: fmt.Sprintf("Suite export version %d is newer than this server understands (1)", man.Version)}
	}

	out := importParse{Source: ImportSourceSuite}
	movieWatches, episodes, rewatches, hidden := 0, 0, 0, 0
	err = readNDJSON(files, "movies.ndjson", func(line []byte) error {
		var m suiteMovie
		if err := json.Unmarshal(line, &m); err != nil {
			return err
		}
		t := importedTitle{Kind: "movie", Title: m.Title, Year: m.Year}
		switch {
		case m.RealTmdbID > 0:
			t.SourceRef = fmt.Sprintf("tmdb:%d", m.RealTmdbID)
		case m.TvdbID > 0:
			t.SourceRef = fmt.Sprintf("suite:tvdb:%d", m.TvdbID)
		default:
			t.SourceRef = "suite:" + strings.ToLower(strings.TrimSpace(m.Title)) + ":" + strconv.Itoa(m.Year)
		}
		if m.TvdbID > 0 {
			t.Data.TvdbID = strconv.FormatInt(m.TvdbID, 10)
		}
		t.Data.Favorite = m.IsFavorite
		if m.WatchedAt != nil && msToUnix(*m.WatchedAt) > 0 {
			t.Data.Watches = append(t.Data.Watches, importWatch{At: msToUnix(*m.WatchedAt)})
		}
		for _, ms := range m.RewatchDates {
			if at := msToUnix(ms); at > 0 {
				t.Data.Watches = append(t.Data.Watches, importWatch{At: at, Rewatch: true})
			}
		}
		if len(t.Data.Watches) == 0 {
			if m.IsHidden {
				hidden++
				return nil // hidden from the list and never watched
			}
			t.Data.Watchlist = true
		}
		movieWatches += len(t.Data.Watches)
		sortWatches(t.Data.Watches)
		out.Titles = append(out.Titles, t)
		return nil
	})
	if err != nil {
		return importParse{}, err
	}

	err = readNDJSON(files, "shows.ndjson", func(line []byte) error {
		var s suiteShow
		if err := json.Unmarshal(line, &s); err != nil {
			return err
		}
		if s.TvdbID <= 0 {
			return nil
		}
		id := strconv.FormatInt(s.TvdbID, 10)
		title, year := splitTitleYear(s.Title)
		t := importedTitle{Kind: "tv", SourceRef: "tvdb:" + id, Title: title, Year: year,
			Data: importData{TvdbID: id, Favorite: s.IsFavorite, Stopped: s.IsStopped}}
		marked := map[int64]int{}
		for _, w := range s.Watched {
			marked[w.WatchedAt]++
		}
		for _, w := range s.Watched {
			at := msToUnix(w.WatchedAt)
			if at <= 0 {
				continue
			}
			season, episode := w.Season, w.Number
			iw := importWatch{At: at, Day: marked[w.WatchedAt] >= bulkMarkMinEpisodes, Season: &season, Episode: &episode}
			if w.TvdbEpisodeID > 0 {
				iw.TvdbEpisodeID = strconv.FormatInt(w.TvdbEpisodeID, 10)
			}
			t.Data.Watches = append(t.Data.Watches, iw)
			episodes++
			for _, ms := range w.RewatchDates {
				if at := msToUnix(ms); at > 0 {
					rw := iw
					rw.At, rw.Day, rw.Rewatch = at, false, true
					t.Data.Watches = append(t.Data.Watches, rw)
					rewatches++
				}
			}
		}
		if len(t.Data.Watches) == 0 && !s.IsStopped {
			// Followed but not started.
			t.Data.Watchlist = true
		}
		sortWatches(t.Data.Watches)
		out.Titles = append(out.Titles, t)
		return nil
	})
	if err != nil {
		return importParse{}, err
	}
	out.Summary = importSummary(out.Titles, map[string]any{
		"episode_watches": episodes, "episode_rewatches": rewatches, "movie_watches": movieWatches, "hidden_skipped": hidden,
	})
	return out, nil
}
