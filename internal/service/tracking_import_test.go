package service

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// A TV Time export: Breaking Bad (TheTVDB 81189) with two episodes bulk
// marked plus a rewatch (whose season_number / episode_number read 0, as
// TV Time writes them; s_no / ep_no are right), an archived show, a for-later show, and a movie
// watched and one to watch.
func tvtimeExport(t *testing.T) []byte {
	return makeZip(t, map[string]string{
		"tracking-prod-records-v2.csv": strings.Join([]string{
			"episode_id,s_no,s_id,ep_id,created_at,episode_number,gsi,series_name,season_number,key,ep_no,is_unitary,is_followed,is_archived,is_for_later",
			"349232,1,81189,349232,2024-01-02 20:00:00,1,,Breaking Bad,1,watch-episode-a-1,1,false,,,",
			"349235,1,81189,349235,2024-01-02 20:00:00,2,,Breaking Bad,1,watch-episode-a-2,2,false,,,",
			"349240,2,81189,349240,2024-02-01 21:00:00,9,watch-episode-1706821200,Breaking Bad,2,watch-episode-a-3,9,true,,,",
			"349232,1,81189,349232,2024-03-01 20:00:00,0,,Breaking Bad,0,rewatch-episode-a-1-1,1,,,,",
			",,81189,,2024-01-01 00:00:00,,,Breaking Bad,,user-series-a,,,true,false,false",
			",,75340,,2024-01-01 00:00:00,,,Prison Break,,user-series-b,,,true,true,false",
			",,79126,,2024-01-01 00:00:00,,,The Wire,,user-series-c,,,true,false,true",
		}, "\n"),
		"tracking-prod-records.csv": strings.Join([]string{
			"type,created_at,uuid,type-uuid-n,movie_name,release_date,runtime,entity_type,watch_date_range_key",
			"watch,2023-05-01 18:00:00,u-1,watch-u-1-0,Inception,2010-07-16 00:00:00,8880,movie,watch-date-1682964000",
			"towatch,2023-05-01 18:00:00,u-2,towatch-u-2,The Dark Knight,2008-07-18 00:00:00,9120,movie,",
		}, "\n"),
		"user_tv_show_data.csv": "is_favorited,nb_episodes_seen,tv_show_name,user_id,tv_show_id,is_followed\n1,3,Breaking Bad,1,81189,1\n",
	})
}

// A Suite export of the same history: TMDB ids for movies, TheTVDB ids for
// shows and episodes, plus one episode TV Time never had. Season 1 was
// marked watched in one go, so its three episodes share a timestamp.
func suiteExport(t *testing.T) []byte {
	return makeZip(t, map[string]string{
		"manifest.json": `{"format":"suite-export","version":1}`,
		"movies.ndjson": `{"title":"Inception","year":2010,"tvdbId":12,"realTmdbId":27205,"watchedAt":1682964000000,"rewatchDates":[],"isFavorite":false}
{"title":"The Dark Knight","year":2008,"tvdbId":13,"realTmdbId":155,"rewatchDates":[],"isFavorite":false}
`,
		"shows.ndjson": `{"title":"Breaking Bad","tvdbId":81189,"isFavorite":false,"isStopped":false,"isWatchlisted":false,"watched":[` +
			`{"season":1,"number":1,"tvdbEpisodeId":349232,"watchedAt":1704225600000,"rewatchDates":[]},` +
			`{"season":1,"number":2,"tvdbEpisodeId":349235,"watchedAt":1704225600000,"rewatchDates":[]},` +
			`{"season":1,"number":3,"tvdbEpisodeId":349236,"watchedAt":1704225600000,"rewatchDates":[]}]}
`,
	})
}

func TestParseTVTimeExport(t *testing.T) {
	p, err := parseExport(tvtimeExport(t))
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != ImportSourceTVTime {
		t.Fatalf("source = %q", p.Source)
	}
	byRef := map[string]importedTitle{}
	for _, ti := range p.Titles {
		byRef[ti.SourceRef] = ti
	}
	bb := byRef["tvdb:81189"]
	if len(bb.Data.Watches) != 4 || !bb.Data.Favorite {
		t.Fatalf("breaking bad = %+v", bb)
	}
	days, rewatches := 0, 0
	for _, w := range bb.Data.Watches {
		if w.Day {
			days++
		}
		if w.Rewatch {
			rewatches++
		}
	}
	if days != 2 || rewatches != 1 {
		t.Fatalf("bulk marks = %d, rewatches = %d; want 2 and 1", days, rewatches)
	}
	if !byRef["tvdb:75340"].Data.Stopped {
		t.Fatal("archived show should be stopped")
	}
	if !byRef["tvdb:79126"].Data.Watchlist {
		t.Fatal("for-later show without episodes should be on the watchlist")
	}
	in := byRef["tvtime:u-1"]
	if in.ReleaseDate != "2010-07-16" || in.Year != 2010 || len(in.Data.Watches) != 1 || in.Data.Watches[0].At != 1682964000 {
		t.Fatalf("inception = %+v", in)
	}
	if !byRef["tvtime:u-2"].Data.Watchlist {
		t.Fatal("towatch movie should be on the watchlist")
	}
}

func TestParseSuiteExport(t *testing.T) {
	p, err := parseExport(suiteExport(t))
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != ImportSourceSuite || len(p.Titles) != 3 {
		t.Fatalf("parse = %s %d titles", p.Source, len(p.Titles))
	}
	for _, ti := range p.Titles {
		if ti.SourceRef == "tmdb:155" && !ti.Data.Watchlist {
			t.Fatal("unwatched Suite movie should be on the watchlist")
		}
	}
}

func TestParseExportRejectsOtherFiles(t *testing.T) {
	if _, err := parseExport([]byte("not a zip")); err == nil {
		t.Fatal("want an error for a non-zip upload")
	}
	if _, err := parseExport(makeZip(t, map[string]string{"x.txt": "hi"})); err == nil {
		t.Fatal("want an error for an unknown zip")
	}
}

func TestScriptLanguage(t *testing.T) {
	for title, want := range map[string]string{
		"\u0c32\u0c15\u0c4d\u0c15\u0c40 \u0c2c\u0c3e\u0c38\u0c4d\u0c15\u0c30\u0c4d": "te", // Telugu
		"\u0932\u093f\u092f\u094b 2": "hi", // Devanagari
		"Leo":                        "",
		"\u041b\u0435\u043e":         "", // Cyrillic: many languages
	} {
		if got := scriptLanguage(title); got != want {
			t.Errorf("scriptLanguage(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestPickMatch(t *testing.T) {
	hits := []TitleSearchResult{
		{ID: "1", Title: "Dune", Year: 1984, ReleaseDate: "1984-12-14"},
		{ID: "2", Title: "Dune", Year: 2021, ReleaseDate: "2021-09-15"},
	}
	if m := pickMatch(hits, "Dune", 2021, "2021-10-22"); m == nil || m.ID != "2" {
		t.Fatalf("same name and year: %+v", m)
	}
	if m := pickMatch(hits, "Dune", 0, "1984-12-14"); m == nil || m.ID != "1" {
		t.Fatalf("exact release date: %+v", m)
	}
	if m := pickMatch(hits, "Dune", 0, ""); m != nil {
		t.Fatalf("two of the same name and no year must not match, got %+v", m)
	}
	if m := pickMatch(hits, "Dune Part Two", 2024, ""); m != nil {
		t.Fatalf("a different name must not match, got %+v", m)
	}
	// A different name, but the only film out within a week of the date.
	if m := pickMatch(hits, "Dune: Part One", 2021, "2021-09-17"); m == nil || m.ID != "2" {
		t.Fatalf("near release date: %+v", m)
	}
	crowded := append(hits, TitleSearchResult{ID: "3", Title: "Dune: The Making Of", Year: 2021, ReleaseDate: "2021-09-20"})
	if m := pickMatch(crowded, "Dune: Part One", 2021, "2021-09-17"); m != nil {
		t.Fatalf("two films that week must not match, got %+v", m)
	}
}

// fakeImportTMDB stands in for TMDB's search and /find.
type fakeImportTMDB struct{}

func (fakeImportTMDB) Enabled() bool { return true }
func (fakeImportTMDB) SearchMovie(_ context.Context, q string) ([]client.TMDBSearchResult, error) {
	return fakeImportTMDB{}.SearchMovieYear(context.Background(), q, 0)
}
func (fakeImportTMDB) SearchMovieYear(_ context.Context, q string, _ int) ([]client.TMDBSearchResult, error) {
	switch q {
	case "Inception":
		return []client.TMDBSearchResult{{ID: 27205, Title: "Inception", Year: 2010, ReleaseDate: "2010-07-15"}}, nil
	case "The Dark Knight":
		return []client.TMDBSearchResult{{ID: 155, Title: "The Dark Knight", Year: 2008, ReleaseDate: "2008-07-16"}}, nil
	}
	return nil, nil
}
func (fakeImportTMDB) SearchTV(context.Context, string) ([]client.TMDBSearchResult, error) {
	return nil, nil
}
func (fakeImportTMDB) DiscoverMovies(_ context.Context, in client.DiscoverMoviesInput) ([]client.TMDBSearchResult, error) {
	if in.Language != "te" || in.From > "2024-10-31" || in.To < "2024-10-31" {
		return nil, nil
	}
	lucky := client.TMDBSearchResult{ID: 1035048, Title: "Lucky Baskhar", Year: 2024, ReleaseDate: "2024-10-30"}
	if in.RuntimeMin > 0 && in.RuntimeMin <= 150 && in.RuntimeMax >= 150 {
		return []client.TMDBSearchResult{lucky}, nil
	}
	return []client.TMDBSearchResult{lucky, {ID: 1, Title: "KA", Year: 2024, ReleaseDate: "2024-10-31"}}, nil
}
func (fakeImportTMDB) FindByTVDB(_ context.Context, id string) (client.TMDBTVDBFound, error) {
	switch id {
	case "81189":
		return client.TMDBTVDBFound{TV: []client.TMDBFound{{Kind: "tv", ID: "1396", Title: "Breaking Bad"}}}, nil
	case "349240": // TheTVDB's S2E9 is TMDB's S2E2
		return client.TMDBTVDBFound{Episodes: []client.TMDBFoundEpisode{{ShowID: "1396", SeasonNumber: 2, EpisodeNumber: 2}}}, nil
	}
	return client.TMDBTVDBFound{}, nil
}

func newImportEnv(t *testing.T) (*ImportService, *TrackingService, int64, int64, *sql.DB) {
	t.Helper()
	tracking, src, alice, bob, db := newTrackingEnvDB(t)
	src.details["movie/1035048"] = client.TMDBTitleDetails{Title: "Lucky Baskhar", ReleaseDate: "2024-10-30", Year: 2024, Runtime: 150, Found: true}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewImportService(log, repository.NewTrackingRepository(db), tracking, fakeImportTMDB{}), tracking, alice, bob, db
}

// runImport uploads an export, matches it, imports it and returns the import.
func runImport(t *testing.T, s *ImportService, userID int64, body []byte) int64 {
	t.Helper()
	ctx := context.Background()
	out, err := s.Upload(ctx, userID, "export.zip", "America/New_York", body)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	id := out["id"].(int64)
	drain(t, s)
	if got := mustImport(t, s, userID, id)["status"]; got != ImportReady {
		t.Fatalf("after matching status = %v", got)
	}
	if _, err := s.Commit(ctx, userID, id); err != nil {
		t.Fatalf("commit: %v", err)
	}
	drain(t, s)
	if got := mustImport(t, s, userID, id)["status"]; got != ImportDone {
		t.Fatalf("after commit status = %v", got)
	}
	return id
}

func drain(t *testing.T, s *ImportService) {
	t.Helper()
	for i := 0; i < 10; i++ {
		imp, err := s.repo.NextActiveUserImport(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if imp == nil {
			return
		}
		s.process(context.Background(), *imp)
	}
	t.Fatal("import never settled")
}

func mustImport(t *testing.T, s *ImportService, userID, id int64) map[string]any {
	t.Helper()
	out, err := s.Get(context.Background(), userID, id)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func countWatches(t *testing.T, db *sql.DB, userID int64, kind, id string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM watch_entries WHERE user_id = ? AND tmdb_kind = ? AND tmdb_id = ?`,
		userID, kind, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Importing the same history from Suite and then TV Time writes each
// viewing once: TV Time only adds what Suite was missing.
func TestImportDeduplicatesAcrossSources(t *testing.T) {
	s, tracking, alice, bob, db := newImportEnv(t)
	ctx := context.Background()

	suiteID := runImport(t, s, alice, suiteExport(t))
	res := mustImport(t, s, alice, suiteID)["results"].(repository.ImportResults)
	// Inception, S1E1-3.
	if res.Added != 4 || res.Duplicates != 0 {
		t.Fatalf("suite results = %+v", res)
	}
	if st, _ := tracking.read.UserTitle(ctx, alice, "movie", "155"); st == nil || st.Status != StatusWatchlist {
		t.Fatalf("dark knight state = %+v, want watchlist", st)
	}

	tvID := runImport(t, s, alice, tvtimeExport(t))
	res = mustImport(t, s, alice, tvID)["results"].(repository.ImportResults)
	// New: the S1E1 rewatch and S2E9 (mapped to S2E2 through TheTVDB).
	// Duplicates: Inception, S1E1, S1E2.
	if res.Added != 2 || res.Duplicates != 3 || res.Unmapped != 0 {
		t.Fatalf("tv time results = %+v", res)
	}
	if n := countWatches(t, db, alice, "tv", "1396"); n != 5 {
		t.Fatalf("breaking bad watches = %d, want 5", n)
	}
	if n := countWatches(t, db, alice, "movie", "27205"); n != 1 {
		t.Fatalf("inception watches = %d, want 1", n)
	}
	st, _ := tracking.read.UserTitle(ctx, alice, "tv", "1396")
	if st == nil || !st.Favorite || st.Status != StatusWatching {
		t.Fatalf("breaking bad state = %+v, want favourite and watching", st)
	}
	// Bulk-marked episodes keep only their day.
	var day int
	if err := db.QueryRow(`SELECT COUNT(*) FROM watch_entries WHERE user_id = ? AND time_precision = 'day'`, alice).Scan(&day); err != nil {
		t.Fatal(err)
	}
	if day != 3 {
		t.Fatalf("day-precision watches = %d, want 3 (Suite's bulk-marked season)", day)
	}

	// Importing the same file again adds nothing.
	againID := runImport(t, s, alice, tvtimeExport(t))
	if res := mustImport(t, s, alice, againID)["results"].(repository.ImportResults); res.Added != 0 {
		t.Fatalf("re-import added %d watches", res.Added)
	}

	// Another user's import is invisible.
	if _, err := s.Get(ctx, bob, tvID); err == nil {
		t.Fatal("bob can read alice's import")
	}
}

// A movie whose name TMDB can't find (TV Time exports names in the app's
// language) is matched to the movie the user already logged at the same
// second, which is how an app that copied the history recorded it.
func TestImportMatchesMoviesFromHistory(t *testing.T) {
	s, _, alice, _, db := newImportEnv(t)
	ctx := context.Background()
	runImport(t, s, alice, suiteExport(t))
	body := makeZip(t, map[string]string{
		"tracking-prod-records.csv": "type,created_at,uuid,type-uuid-n,movie_name,release_date,runtime,entity_type,watch_date_range_key\n" +
			"watch,2023-05-01 18:00:00,u-1,watch-u-1-0,\u0907\u0928\u094d\u0938\u0947\u092a\u094d\u0936\u0928,2010-07-16 00:00:00,8880,movie,watch-date-1682964000\n",
	})
	out, err := s.Upload(ctx, alice, "", "", body)
	if err != nil {
		t.Fatal(err)
	}
	id := out["id"].(int64)
	drain(t, s)
	page, err := s.Items(ctx, alice, id, ImportItemsInput{})
	if err != nil {
		t.Fatal(err)
	}
	it := page["items"].([]map[string]any)[0]
	if it["match_status"] != ItemMatched || it["match_method"] != "history" || it["tmdb"].(map[string]any)["tmdb_id"] != "27205" {
		t.Fatalf("item = %v", it)
	}
	if _, err := s.Commit(ctx, alice, id); err != nil {
		t.Fatal(err)
	}
	drain(t, s)
	if n := countWatches(t, db, alice, "movie", "27205"); n != 1 {
		t.Fatalf("inception watches = %d, want 1", n)
	}
}

// A movie named in Telugu is found by language, release date and runtime.
func TestImportFindsTranslatedTitlesByRelease(t *testing.T) {
	s, _, alice, _, _ := newImportEnv(t)
	ctx := context.Background()
	row := func(uuid, runtime string) string {
		return "watch,2024-11-02 18:00:00," + uuid + ",watch-" + uuid + "-0,\u0c32\u0c15\u0c4d\u0c15\u0c40,2024-10-31 00:00:00," + runtime + ",movie,watch-date-1730570400\n"
	}
	body := makeZip(t, map[string]string{
		"tracking-prod-records.csv": "type,created_at,uuid,type-uuid-n,movie_name,release_date,runtime,entity_type,watch_date_range_key\n" +
			row("u-1", "9000") + row("u-2", ""),
	})
	out, err := s.Upload(ctx, alice, "", "", body)
	if err != nil {
		t.Fatal(err)
	}
	id := out["id"].(int64)
	drain(t, s)
	page, err := s.Items(ctx, alice, id, ImportItemsInput{Kind: "movie"})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range page["items"].([]map[string]any) {
		switch it["match_status"] {
		case ItemMatched: // the one with a runtime (150 min) is certain
			if it["match_method"] != "release" || it["tmdb"].(map[string]any)["tmdb_id"] != "1035048" {
				t.Fatalf("matched item = %v", it)
			}
		case ItemUnmatched: // without one, two films that week are candidates
			if c := it["candidates"].([]TitleSearchResult); len(c) != 2 {
				t.Fatalf("candidates = %v", c)
			}
		default:
			t.Fatalf("item = %v", it)
		}
	}
}

// Undo removes what an import wrote and puts the title state back, but
// keeps watches logged by hand.
func TestImportUndo(t *testing.T) {
	s, tracking, alice, _, db := newImportEnv(t)
	ctx := context.Background()

	mustLog(t, tracking, alice, WatchInput{Kind: "movie", TmdbID: "155", EndedAt: "2020-01-01T20:00:00Z"})
	id := runImport(t, s, alice, suiteExport(t))
	if n := countWatches(t, db, alice, "movie", "27205"); n != 1 {
		t.Fatalf("inception watches = %d", n)
	}
	out, err := s.Undo(ctx, alice, id)
	if err != nil {
		t.Fatal(err)
	}
	if out["watches_removed"].(int64) != 4 || out["status"] != ImportUndone {
		t.Fatalf("undo = %v", out)
	}
	if n := countWatches(t, db, alice, "movie", "27205") + countWatches(t, db, alice, "tv", "1396"); n != 0 {
		t.Fatalf("%d imported watches left", n)
	}
	if n := countWatches(t, db, alice, "movie", "155"); n != 1 {
		t.Fatalf("hand-logged watch removed (%d left)", n)
	}
	for _, ref := range [][2]string{{"movie", "27205"}, {"tv", "1396"}} {
		if st, _ := tracking.read.UserTitle(ctx, alice, ref[0], ref[1]); st != nil {
			t.Fatalf("%s/%s state left behind: %+v", ref[0], ref[1], st)
		}
	}
	if _, err := s.Undo(ctx, alice, id); err == nil {
		t.Fatal("undoing twice should be refused")
	}
}

// Review: an unmatched title can be matched by hand or skipped.
func TestImportReviewFixesItems(t *testing.T) {
	s, _, alice, _, _ := newImportEnv(t)
	ctx := context.Background()
	body := makeZip(t, map[string]string{
		"tracking-prod-records.csv": "type,created_at,uuid,type-uuid-n,movie_name,release_date,runtime,entity_type,watch_date_range_key\n" +
			"watch,2023-05-01 18:00:00,u-9,watch-u-9-0,Some Obscure Film,2001-01-01 00:00:00,5400,movie,watch-date-1682964000\n",
	})
	out, err := s.Upload(ctx, alice, "", "", body)
	if err != nil {
		t.Fatal(err)
	}
	id := out["id"].(int64)
	drain(t, s)
	page, err := s.Items(ctx, alice, id, ImportItemsInput{Match: ItemUnmatched})
	if err != nil {
		t.Fatal(err)
	}
	items := page["items"].([]map[string]any)
	if len(items) != 1 {
		t.Fatalf("unmatched = %d, want 1", len(items))
	}
	itemID := items[0]["id"].(int64)
	got, err := s.SetItem(ctx, alice, id, itemID, ImportItemInput{TmdbID: tp("27205")})
	if err != nil {
		t.Fatal(err)
	}
	if got["match_status"] != ItemMatched || got["match_method"] != "manual" {
		t.Fatalf("after manual match = %v", got)
	}
	if _, err := s.SetItem(ctx, alice, id, itemID, ImportItemInput{Skip: tp(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Commit(ctx, alice, id); err != nil {
		t.Fatal(err)
	}
	drain(t, s)
	if res := mustImport(t, s, alice, id)["results"].(repository.ImportResults); res.Added != 0 {
		t.Fatalf("a skipped title was imported: %+v", res)
	}
	if _, err := s.SetItem(ctx, alice, id, itemID, ImportItemInput{Skip: tp(false)}); err == nil {
		t.Fatal("changing an item after the import should be refused")
	}
}
