package router

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/config"
	"github.com/bugsbunny-25/metareel/internal/handler"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/service"
)

// trackingEnv is an Echo app with the tracking API wired up, one API key and
// two users.
type trackingEnv struct {
	e      *echo.Echo
	apiKey string
}

// stubTMDB answers title lookups without a network call.
type stubTMDB struct{}

func (stubTMDB) Enabled() bool { return true }
func (stubTMDB) SearchMovie(context.Context, string) ([]client.TMDBSearchResult, error) {
	return []client.TMDBSearchResult{{ID: 27205, Title: "Inception", Year: 2010}}, nil
}
func (stubTMDB) SearchTV(context.Context, string) ([]client.TMDBSearchResult, error) { return nil, nil }
func (stubTMDB) TitleDetails(_ context.Context, kind, id string) (client.TMDBTitleDetails, error) {
	switch {
	case kind == "movie" && id == "27205":
		return client.TMDBTitleDetails{Title: "Inception", ReleaseDate: "2010-07-15", Year: 2010, Runtime: 148, Found: true}, nil
	case kind == "tv" && id == "1396":
		return client.TMDBTitleDetails{Title: "Breaking Bad", ReleaseDate: "2008-01-20", Year: 2008, Runtime: 47,
			NumberOfSeasons: 1, Seasons: []client.TMDBSeason{{Number: 1}}, Found: true}, nil
	}
	return client.TMDBTitleDetails{}, nil
}

func (stubTMDB) TVSeasonEpisodes(_ context.Context, tvID string, _ []int) (map[int][]client.TMDBEpisode, error) {
	if tvID != "1396" {
		return nil, nil
	}
	return map[int][]client.TMDBEpisode{1: {
		{ID: 901, SeasonNumber: 1, EpisodeNumber: 1, Name: "Pilot", AirDate: "2008-01-20", Runtime: 58},
		{ID: 902, SeasonNumber: 1, EpisodeNumber: 2, Name: "Cat in the Bag", AirDate: "2008-01-27", Runtime: 48},
	}}, nil
}

func newTrackingEnv(t *testing.T) trackingEnv {
	t.Helper()
	ctx := context.Background()
	db, err := repository.Open("file:" + filepath.Join(tempDBDir(t), "test.db") + "?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := repository.Migrate(db, "../../db/migrations"); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	keys := service.NewAPIKeyService(log, repository.NewAPIKeyRepository(db))
	if err := keys.Load(ctx); err != nil {
		t.Fatal(err)
	}
	key, err := keys.Create(ctx, "tides")
	if err != nil {
		t.Fatal(err)
	}
	userRepo := repository.NewUserRepository(db)
	sessions := service.NewSessionService(log, userRepo, service.SessionConfig{TTL: 24 * time.Hour, IdleTimeout: time.Hour})
	users := service.NewUserService(log, userRepo, sessions)
	for _, name := range []string{"alice", "bob"} {
		if _, err := users.Create(ctx, name, "long enough pw", service.RoleReader); err != nil {
			t.Fatal(err)
		}
	}

	repo := repository.NewTrackingRepository(db)
	titles := service.NewTrackedTitleService(log, stubTMDB{}, repo, repo)
	e := echo.New()
	Register(e, &handler.Handler{
		APIKeys: keys, Sessions: sessions, Users: users,
		Auth:     service.NewAuthService(keys, sessions),
		Tracking: service.NewTrackingService(log, repo, repo, titles),
	}, &config.Config{})
	return trackingEnv{e: e, apiKey: key.Key}
}

func (a trackingEnv) do(method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	a.e.ServeHTTP(rec, req)
	return rec
}

// login signs a user in and returns their session cookie header.
func (a trackingEnv) login(t *testing.T, username string) map[string]string {
	t.Helper()
	rec := a.do(http.MethodPost, "/api/v1/auth/login", `{"username":"`+username+`","password":"long enough pw"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", username, rec.Code, rec.Body)
	}
	return map[string]string{"Cookie": handler.SessionCookie + "=" + rec.Result().Cookies()[0].Value}
}

// The tracking API needs a signed-in user: an API key identifies an
// integration, not a person, so it is refused even though it opens the rest
// of the public API.
func TestTrackingNeedsASignedInUser(t *testing.T) {
	a := newTrackingEnv(t)

	for _, tc := range []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"no credentials", nil, http.StatusUnauthorized},
		{"api key in X-API-Key", map[string]string{"X-API-Key": a.apiKey}, http.StatusUnauthorized},
		{"api key as bearer", map[string]string{"Authorization": "Bearer " + a.apiKey}, http.StatusUnauthorized},
		{"a bogus session", map[string]string{"Cookie": handler.SessionCookie + "=mrs_bogus"}, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if rec := a.do(http.MethodGet, "/api/v1/me/watches", "", tc.headers); rec.Code != tc.want {
				t.Fatalf("GET /me/watches: %d, want %d (%s)", rec.Code, tc.want, rec.Body)
			}
		})
	}

	alice := a.login(t, "alice")
	if rec := a.do(http.MethodGet, "/api/v1/me/watches", "", alice); rec.Code != http.StatusOK {
		t.Fatalf("signed in: %d %s", rec.Code, rec.Body)
	}
}

// Per-user responses must never be cached or revalidated: the tracking group
// replaces the shared read routes' ETag with no-store.
func TestTrackingResponsesAreNotCacheable(t *testing.T) {
	a := newTrackingEnv(t)
	alice := a.login(t, "alice")

	rec := a.do(http.MethodGet, "/api/v1/me/watches", "", alice)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /me/watches: %d %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("Cache-Control = %q, want %q", got, "private, no-store")
	}
	if got := rec.Header().Get("Vary"); !strings.Contains(got, "Cookie") || !strings.Contains(got, "Authorization") {
		t.Errorf("Vary = %q, want Authorization and Cookie", got)
	}
	if got := rec.Header().Get("ETag"); got != "" {
		t.Errorf("ETag = %q, want none on a per-user response", got)
	}
}

// Writes are cookie-authenticated, so another site must not be able to make
// them from a visitor's browser.
func TestTrackingRefusesCrossSiteWrites(t *testing.T) {
	a := newTrackingEnv(t)
	alice := a.login(t, "alice")
	headers := map[string]string{"Cookie": alice["Cookie"], "Sec-Fetch-Site": "cross-site"}

	body := `{"tmdb_kind":"movie","tmdb_id":"27205","ended_at":"2026-11-10T01:30:00Z"}`
	if rec := a.do(http.MethodPost, "/api/v1/me/watches", body, headers); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site POST: %d, want 403 (%s)", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodPost, "/api/v1/me/watches", body, alice); rec.Code != http.StatusCreated {
		t.Fatalf("same-site POST: %d, want 201 (%s)", rec.Code, rec.Body)
	}
}

// The whole round trip: log a watch, read it back, find it in the diary and
// on the title, then delete it.
func TestTrackingRoundTrip(t *testing.T) {
	a := newTrackingEnv(t)
	alice := a.login(t, "alice")

	rec := a.do(http.MethodPost, "/api/v1/me/watches",
		`{"tmdb_kind":"movie","tmdb_id":"27205","ended_at":"2026-11-10T01:30:00Z","tz":"America/New_York","rating":9,"tags":["imax"],"client_key":"k1"}`, alice)
	if rec.Code != http.StatusCreated {
		t.Fatalf("log watch: %d %s", rec.Code, rec.Body)
	}
	var entry repository.WatchEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &entry); err != nil {
		t.Fatalf("decode watch: %v", err)
	}
	if entry.WatchedOn != "2026-11-09" {
		t.Errorf("watched_on = %q, want 2026-11-09 (local day of the start)", entry.WatchedOn)
	}
	if entry.Title != "" && entry.Title != "Inception" {
		t.Errorf("title = %q", entry.Title)
	}

	// Replaying the same client_key returns the stored watch, with 200.
	rec = a.do(http.MethodPost, "/api/v1/me/watches",
		`{"tmdb_kind":"movie","tmdb_id":"27205","ended_at":"2026-11-10T01:30:00Z","client_key":"k1"}`, alice)
	if rec.Code != http.StatusOK {
		t.Fatalf("replay: %d, want 200 (%s)", rec.Code, rec.Body)
	}

	// The diary has one entry, with its tag and its poster metadata.
	rec = a.do(http.MethodGet, "/api/v1/me/watches", "", alice)
	var diary struct {
		Watches []repository.WatchEntry `json:"watches"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &diary); err != nil {
		t.Fatalf("decode diary: %v", err)
	}
	if len(diary.Watches) != 1 {
		t.Fatalf("diary has %d entries, want 1: %s", len(diary.Watches), rec.Body)
	}
	if got := diary.Watches[0]; got.Title != "Inception" || len(got.Tags) != 1 || got.Tags[0] != "imax" {
		t.Errorf("diary entry = %+v", got)
	}

	// The title now carries the latest watch's rating.
	rec = a.do(http.MethodGet, "/api/v1/me/titles/movie/27205", "", alice)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"rating":9`) {
		t.Fatalf("title state: %d %s", rec.Code, rec.Body)
	}

	// Another user cannot see it, and gets a 404 rather than a 403.
	bob := a.login(t, "bob")
	if rec := a.do(http.MethodGet, "/api/v1/me/watches/"+itoa(entry.ID), "", bob); rec.Code != http.StatusNotFound {
		t.Errorf("bob reading alice's watch: %d, want 404 (%s)", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodDelete, "/api/v1/me/watches/"+itoa(entry.ID), "", bob); rec.Code != http.StatusNotFound {
		t.Errorf("bob deleting alice's watch: %d, want 404", rec.Code)
	}

	// Alice can, and the delete is permanent.
	if rec := a.do(http.MethodDelete, "/api/v1/me/watches/"+itoa(entry.ID), "", alice); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodGet, "/api/v1/me/watches/"+itoa(entry.ID), "", alice); rec.Code != http.StatusNotFound {
		t.Errorf("after delete: %d, want 404", rec.Code)
	}
}

func TestWatchlistEndpoints(t *testing.T) {
	a := newTrackingEnv(t)
	alice := a.login(t, "alice")

	if rec := a.do(http.MethodPut, "/api/v1/me/watchlist/movie/27205", "", alice); rec.Code != http.StatusOK {
		t.Fatalf("add to watchlist: %d %s", rec.Code, rec.Body)
	}
	rec := a.do(http.MethodGet, "/api/v1/me/watchlist", "", alice)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"tmdb_id":"27205"`) {
		t.Fatalf("watchlist: %d %s", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodDelete, "/api/v1/me/watchlist/movie/27205", "", alice); rec.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	rec = a.do(http.MethodGet, "/api/v1/me/watchlist", "", alice)
	if strings.Contains(rec.Body.String(), `"tmdb_id":"27205"`) {
		t.Errorf("still on the watchlist: %s", rec.Body)
	}

	// An id TMDB does not know is a 404, not a stored row.
	if rec := a.do(http.MethodPut, "/api/v1/me/watchlist/movie/424242", "", alice); rec.Code != http.StatusNotFound {
		t.Errorf("unknown title: %d, want 404 (%s)", rec.Code, rec.Body)
	}
}

// The admin console answers on /admin, its permanent home once the
// user-facing app takes over the root.
//
// The static dir must be relative to the working directory: echo binds its
// filesystem root when echo.New() runs, so c.File never resolves an absolute
// path. That is why UI_STATIC_DIR is "web/dist" and not an absolute path.
func TestAdminConsoleIsServedUnderAdmin(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("dist/assets", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("dist/index.html", []byte("<!doctype html><title>console</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("dist/assets/app.js", []byte("// bundle"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.UI.ServeStatic = true
	cfg.UI.StaticDir = "dist"
	e := echo.New()
	Register(e, &handler.Handler{}, cfg)

	for _, path := range []string{"/admin", "/admin/", "/admin/anything"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "console") {
			t.Errorf("GET %s: %d %q", path, rec.Code, rec.Body.String())
		}
	}

	// The console is built with base /admin/, so its real asset files come from
	// disk under /admin rather than the SPA fallback.
	req := httptest.NewRequest(http.MethodGet, "/admin/assets/app.js", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "bundle") {
		t.Errorf("GET /admin/assets/app.js: %d %q", rec.Code, rec.Body.String())
	}

	// No user app is built here (APP_STATIC_DIR is unset), so / sends people to the console.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/admin/" {
		t.Errorf("GET /: %d Location %q, want 302 /admin/", rec.Code, rec.Header().Get("Location"))
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

// The show page, a bulk mark and the up-next queue over HTTP.
func TestShowProgressAndUpNextEndpoints(t *testing.T) {
	a := newTrackingEnv(t)
	alice := a.login(t, "alice")

	rec := a.do(http.MethodGet, "/api/v1/me/shows/1396", "", alice)
	if rec.Code != http.StatusOK {
		t.Fatalf("show page: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"name":"Pilot"`) {
		t.Errorf("show page has no episodes: %s", rec.Body)
	}

	// Mark the first episode, then the queue should offer the second.
	rec = a.do(http.MethodPost, "/api/v1/me/shows/1396/watched",
		`{"scope":"episode","season_number":1,"episode_number":1,"ended_at":"2026-11-12T20:00:00Z"}`, alice)
	if rec.Code != http.StatusOK {
		t.Fatalf("mark episode: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"marked":1`) {
		t.Errorf("mark response = %s", rec.Body)
	}

	rec = a.do(http.MethodGet, "/api/v1/me/up-next", "", alice)
	var next struct {
		Episodes []struct {
			TmdbID        string `json:"tmdb_id"`
			SeasonNumber  int    `json:"season_number"`
			EpisodeNumber int    `json:"episode_number"`
			ShowTitle     string `json:"show_title"`
		} `json:"episodes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &next); err != nil {
		t.Fatalf("decode up-next: %v", err)
	}
	if len(next.Episodes) != 1 || next.Episodes[0].EpisodeNumber != 2 {
		t.Fatalf("up next = %+v, want S1E2", next.Episodes)
	}
	if next.Episodes[0].ShowTitle != "Breaking Bad" {
		t.Errorf("show_title = %q", next.Episodes[0].ShowTitle)
	}

	// Unmarking takes it back off.
	rec = a.do(http.MethodDelete, "/api/v1/me/shows/1396/watched?scope=season&season=1", "", alice)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"removed":1`) {
		t.Fatalf("unmark season: %d %s", rec.Code, rec.Body)
	}
	rec = a.do(http.MethodGet, "/api/v1/me/up-next", "", alice)
	if !strings.Contains(rec.Body.String(), `"episodes":[]`) {
		t.Errorf("up next after unmarking = %s, want empty", rec.Body)
	}

	// Episode routes need a signed-in user too.
	for _, path := range []string{"/api/v1/me/shows/1396", "/api/v1/me/up-next"} {
		if rec := a.do(http.MethodGet, path, "", map[string]string{"X-API-Key": a.apiKey}); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s with an api key: %d, want 401", path, rec.Code)
		}
	}
}

// Venues, screens and the premium-format taxonomy over HTTP.
func TestVenueAndFormatEndpoints(t *testing.T) {
	a := newTrackingEnv(t)
	alice := a.login(t, "alice")

	// The seeded taxonomy is there for users to pick from.
	rec := a.do(http.MethodGet, "/api/v1/me/formats", "", alice)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"slug":"imax"`) {
		t.Fatalf("formats: %d %s", rec.Code, rec.Body)
	}

	// Add a cinema and a screen.
	rec = a.do(http.MethodPost, "/api/v1/me/venues",
		`{"name":"PVR Phoenix","city":"Mumbai","country":"in","timezone":"Asia/Kolkata","refs":{"wikidata_id":"Q1"}}`, alice)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create venue: %d %s", rec.Code, rec.Body)
	}
	var created struct {
		Venue struct {
			ID      int64  `json:"id"`
			Country string `json:"country"`
		} `json:"venue"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode venue: %v", err)
	}
	if created.Venue.Country != "IN" {
		t.Errorf("country = %q, want IN", created.Venue.Country)
	}
	venueID := itoa(created.Venue.ID)

	rec = a.do(http.MethodPost, "/api/v1/me/venues/"+venueID+"/screens",
		`{"name":"Audi 1","number":1,"width":21,"height":16,"dim_unit":"m","aspect_ratio":"1.43:1"}`, alice)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create screen: %d %s", rec.Code, rec.Body)
	}
	var screen struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &screen); err != nil {
		t.Fatalf("decode screen: %v", err)
	}

	// Log a watch there, with formats and a price.
	body := `{"tmdb_kind":"movie","tmdb_id":"27205","ended_at":"2026-11-12T18:20:00Z",` +
		`"venue_id":` + venueID + `,"screen_id":` + itoa(screen.ID) + `,` +
		`"seat":"H14","screening_type":"regular","price_minor":45000,"currency":"inr",` +
		`"formats":["imax","dolby-atmos"]}`
	rec = a.do(http.MethodPost, "/api/v1/me/watches", body, alice)
	if rec.Code != http.StatusCreated {
		t.Fatalf("log watch at a venue: %d %s", rec.Code, rec.Body)
	}
	for _, want := range []string{`"venue_name_at_watch":"PVR Phoenix"`, `"currency":"INR"`, `"tz":"Asia/Kolkata"`, `"slug":"imax"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("watch body missing %s: %s", want, rec.Body)
		}
	}

	// The diary carries the venue and the formats.
	rec = a.do(http.MethodGet, "/api/v1/me/watches", "", alice)
	if !strings.Contains(rec.Body.String(), `"venue_name_at_watch":"PVR Phoenix"`) ||
		!strings.Contains(rec.Body.String(), `"slug":"imax"`) {
		t.Errorf("diary missing venue or formats: %s", rec.Body)
	}

	// A venue with watches at it cannot be deleted.
	if rec := a.do(http.MethodDelete, "/api/v1/me/venues/"+venueID, "", alice); rec.Code != http.StatusConflict {
		t.Errorf("delete a venue in use: %d, want 409 (%s)", rec.Code, rec.Body)
	}

	// Venue routes need a signed-in user, not an API key.
	if rec := a.do(http.MethodGet, "/api/v1/me/venues", "", map[string]string{"X-API-Key": a.apiKey}); rec.Code != http.StatusUnauthorized {
		t.Errorf("venues with an api key: %d, want 401", rec.Code)
	}
}

// The admin taxonomy: add a format, retire it, and see it leave the
// user-facing list while staying in the admin one.
func TestAdminWatchFormatEndpoints(t *testing.T) {
	a := newTrackingEnv(t)
	alice := a.login(t, "alice")

	rec := a.do(http.MethodPost, "/api/v1/admin/watch-formats",
		`{"slug":"imax-gt","label":"IMAX GT","category":"brand","sort_order":14}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create format: %d %s", rec.Code, rec.Body)
	}
	var f struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatalf("decode format: %v", err)
	}

	if rec := a.do(http.MethodPost, "/api/v1/admin/watch-formats",
		`{"slug":"imax-gt","label":"Again","category":"brand"}`, nil); rec.Code != http.StatusConflict {
		t.Errorf("duplicate slug: %d, want 409", rec.Code)
	}

	rec = a.do(http.MethodGet, "/api/v1/me/formats", "", alice)
	if !strings.Contains(rec.Body.String(), `"slug":"imax-gt"`) {
		t.Errorf("the new format is not offered: %s", rec.Body)
	}

	rec = a.do(http.MethodPatch, "/api/v1/admin/watch-formats/"+itoa(f.ID), `{"active":false}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("retire: %d %s", rec.Code, rec.Body)
	}
	rec = a.do(http.MethodGet, "/api/v1/me/formats", "", alice)
	if strings.Contains(rec.Body.String(), `"slug":"imax-gt"`) {
		t.Errorf("a retired format is still offered: %s", rec.Body)
	}
	rec = a.do(http.MethodGet, "/api/v1/admin/watch-formats", "", nil)
	if !strings.Contains(rec.Body.String(), `"slug":"imax-gt"`) {
		t.Errorf("the admin list should still show it: %s", rec.Body)
	}
}

// tempDBDir gives a test its own directory for the database.
//
// t.TempDir is not usable here: SessionService and APIKeyService record
// "last used" in a detached goroutine, so a write can still land after the
// test body returns. If it lands while t.TempDir is being removed, the
// removal fails with "directory not empty" and fails the test. Owning the
// directory lets the database close first and the removal be best-effort.
func tempDBDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "metareel-router-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// The stats endpoints over HTTP, including the watchlist crossover into
// metareel's own chart data.
func TestStatsEndpoints(t *testing.T) {
	a := newTrackingEnv(t)
	alice := a.login(t, "alice")

	// Two watches on different days, one of them rated and paid for.
	for _, body := range []string{
		`{"tmdb_kind":"movie","tmdb_id":"27205","ended_at":"2026-03-01T22:00:00Z","rating":9,"venue_kind":"theater","price_minor":45000,"currency":"INR","formats":["imax"]}`,
		`{"tmdb_kind":"movie","tmdb_id":"27205","ended_at":"2026-03-02T22:00:00Z","venue_kind":"streaming"}`,
	} {
		if rec := a.do(http.MethodPost, "/api/v1/me/watches", body, alice); rec.Code != http.StatusCreated {
			t.Fatalf("log watch: %d %s", rec.Code, rec.Body)
		}
	}

	rec := a.do(http.MethodGet, "/api/v1/me/stats/summary", "", alice)
	if rec.Code != http.StatusOK {
		t.Fatalf("summary: %d %s", rec.Code, rec.Body)
	}
	var summary struct {
		Totals struct {
			Watches int `json:"watches"`
			Minutes int `json:"minutes"`
		} `json:"totals"`
		Spend []struct {
			Currency   string `json:"currency"`
			TotalMinor int    `json:"total_minor"`
		} `json:"spend"`
		Formats []struct {
			Slug string `json:"slug"`
		} `json:"formats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.Totals.Watches != 2 || summary.Totals.Minutes != 296 {
		t.Errorf("totals = %+v, want 2 watches and 296 minutes", summary.Totals)
	}
	if len(summary.Spend) != 1 || summary.Spend[0].Currency != "INR" || summary.Spend[0].TotalMinor != 45000 {
		t.Errorf("spend = %+v", summary.Spend)
	}
	if len(summary.Formats) != 1 || summary.Formats[0].Slug != "imax" {
		t.Errorf("formats = %+v", summary.Formats)
	}

	// The calendar has both days and a two-day streak.
	rec = a.do(http.MethodGet, "/api/v1/me/stats/calendar", "", alice)
	if !strings.Contains(rec.Body.String(), `"longest":2`) {
		t.Errorf("calendar streaks = %s", rec.Body)
	}

	// Year in review defaults to the current year, and takes one explicitly.
	rec = a.do(http.MethodGet, "/api/v1/me/stats/year?year=2026", "", alice)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"year":2026`) {
		t.Fatalf("year: %d %s", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodGet, "/api/v1/me/stats/year?year=nope", "", alice); rec.Code != http.StatusBadRequest {
		t.Errorf("bad year: %d, want 400", rec.Code)
	}

	rec = a.do(http.MethodGet, "/api/v1/me/stats/ratings", "", alice)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"rated":1`) {
		t.Fatalf("ratings: %d %s", rec.Code, rec.Body)
	}

	// Watchlist insights need a country.
	if rec := a.do(http.MethodGet, "/api/v1/me/watchlist/insights", "", alice); rec.Code != http.StatusBadRequest {
		t.Errorf("insights without a country: %d, want 400", rec.Code)
	}
	rec = a.do(http.MethodGet, "/api/v1/me/watchlist/insights?country=IN", "", alice)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"country":"IN"`) {
		t.Fatalf("insights: %d %s", rec.Code, rec.Body)
	}

	// Stats are per-user and never cached.
	for _, path := range []string{"/api/v1/me/stats/summary", "/api/v1/me/stats/year", "/api/v1/me/watchlist/insights?country=IN"} {
		rec := a.do(http.MethodGet, path, "", alice)
		if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
			t.Errorf("GET %s Cache-Control = %q", path, got)
		}
		if rec := a.do(http.MethodGet, path, "", map[string]string{"X-API-Key": a.apiKey}); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s with an api key: %d, want 401", path, rec.Code)
		}
	}
}
