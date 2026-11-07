package router

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/config"
	"github.com/bugsbunny-25/metareel/internal/handler"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/service"
)

type authEnv struct {
	e      *echo.Echo
	apiKey string
}

func newAuthEnv(t *testing.T) authEnv {
	return newAuthEnvWith(t, false)
}

func newAuthEnvWith(t *testing.T, requireAdminLogin bool) authEnv {
	t.Helper()
	ctx := context.Background()
	db, err := repository.Open("file:" + filepath.Join(t.TempDir(), "test.db") + "?_pragma=foreign_keys(1)")
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
	if _, err := users.Create(ctx, "alice", "long enough pw", service.RoleReader); err != nil {
		t.Fatal(err)
	}
	if _, err := users.Create(ctx, "root", "long enough pw", service.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	Register(e, &handler.Handler{APIKeys: keys, Sessions: sessions, Users: users, Auth: service.NewAuthService(keys, sessions),
		AdminLoginRequired: requireAdminLogin}, &config.Config{})
	return authEnv{e: e, apiKey: key.Key}
}

func (a authEnv) do(method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
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

func TestUserAuthAndAPIKeysBothWork(t *testing.T) {
	a := newAuthEnv(t)

	// No credentials: 401 naming both schemes.
	rec := a.do(http.MethodGet, "/api/v1/test", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no credentials: %d, want 401", rec.Code)
	}
	if got := rec.Header().Values("WWW-Authenticate"); len(got) != 2 {
		t.Fatalf("WWW-Authenticate = %v, want ApiKey and Bearer", got)
	}

	// API keys keep working, in both headers.
	for _, h := range []map[string]string{{"X-API-Key": a.apiKey}, {"Authorization": "Bearer " + a.apiKey}} {
		if rec := a.do(http.MethodGet, "/api/v1/test", "", h); rec.Code != http.StatusOK {
			t.Fatalf("api key %v: %d", h, rec.Code)
		}
	}

	// Wrong password.
	if rec := a.do(http.MethodPost, "/api/v1/auth/login", `{"username":"alice","password":"nope nope nope"}`, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d, want 401", rec.Code)
	}
	// Cross-site browser login is refused.
	if rec := a.do(http.MethodPost, "/api/v1/auth/login", `{"username":"alice","password":"long enough pw"}`,
		map[string]string{"Sec-Fetch-Site": "cross-site"}); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site login: %d, want 403", rec.Code)
	}

	rec = a.do(http.MethodPost, "/api/v1/auth/login", `{"username":"alice","password":"long enough pw"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	var login service.LoginResult
	if err := json.Unmarshal(rec.Body.Bytes(), &login); err != nil || !strings.HasPrefix(login.Token, "mrs_") {
		t.Fatalf("login body = %s (%v)", rec.Body, err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != handler.SessionCookie || cookies[0].Value != login.Token ||
		!cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Secure {
		t.Fatalf("session cookie = %+v", cookies)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("login Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}

	cookie := map[string]string{"Cookie": handler.SessionCookie + "=" + login.Token}
	bearer := map[string]string{"Authorization": "Bearer " + login.Token}
	for name, h := range map[string]map[string]string{"cookie": cookie, "bearer": bearer} {
		if rec := a.do(http.MethodGet, "/api/v1/test", "", h); rec.Code != http.StatusOK {
			t.Fatalf("session via %s: %d", name, rec.Code)
		}
	}
	rec = a.do(http.MethodGet, "/api/v1/auth/me", "", bearer)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"kind":"user"`) || !strings.Contains(rec.Body.String(), `"name":"alice"`) {
		t.Fatalf("me (session): %d %s", rec.Code, rec.Body)
	}
	rec = a.do(http.MethodGet, "/api/v1/auth/me", "", map[string]string{"X-API-Key": a.apiKey})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"kind":"api_key"`) {
		t.Fatalf("me (key): %d %s", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodGet, "/api/v1/auth/me", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("me (none): %d", rec.Code)
	}

	// A bad header credential isn't rescued by a valid cookie.
	h := map[string]string{"Cookie": cookie["Cookie"], "X-API-Key": "mr_wrong"}
	if rec := a.do(http.MethodGet, "/api/v1/test", "", h); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key + valid cookie: %d, want 401", rec.Code)
	}

	// Logout ends the session and clears the cookie.
	rec = a.do(http.MethodPost, "/api/v1/auth/logout", "", cookie)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	if c := rec.Result().Cookies(); len(c) != 1 || c[0].MaxAge >= 0 {
		t.Fatalf("logout cookie = %+v, want deletion", c)
	}
	for name, h := range map[string]map[string]string{"cookie": cookie, "bearer": bearer} {
		if rec := a.do(http.MethodGet, "/api/v1/test", "", h); rec.Code != http.StatusUnauthorized {
			t.Fatalf("session via %s after logout: %d, want 401", name, rec.Code)
		}
	}
	// The admin API is unchanged by this (phase 1).
	if rec := a.do(http.MethodGet, "/api/v1/admin/test", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("admin test route: %d", rec.Code)
	}
}

func TestSessionCookieSecureBehindHTTPS(t *testing.T) {
	a := newAuthEnv(t)
	rec := a.do(http.MethodPost, "/api/v1/auth/login", `{"username":"alice","password":"long enough pw"}`,
		map[string]string{"X-Forwarded-Proto": "https"})
	if c := rec.Result().Cookies(); rec.Code != http.StatusOK || len(c) != 1 || !c[0].Secure {
		t.Fatalf("login behind https proxy: %d cookies %+v", rec.Code, c)
	}
}

// login signs a user in and returns the session cookie header.
func (a authEnv) login(t *testing.T, username string) map[string]string {
	t.Helper()
	rec := a.do(http.MethodPost, "/api/v1/auth/login", `{"username":"`+username+`","password":"long enough pw"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", username, rec.Code, rec.Body)
	}
	return map[string]string{"Cookie": handler.SessionCookie + "=" + rec.Result().Cookies()[0].Value}
}

func TestAdminLoginRequired(t *testing.T) {
	a := newAuthEnvWith(t, true)
	admin, reader := a.login(t, "root"), a.login(t, "alice")
	for _, tc := range []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"no session", nil, http.StatusUnauthorized},
		{"api key", map[string]string{"X-API-Key": a.apiKey}, http.StatusUnauthorized},
		{"bad session", map[string]string{"Cookie": handler.SessionCookie + "=mrs_bogus"}, http.StatusUnauthorized},
		{"reader", reader, http.StatusForbidden},
		{"admin cookie", admin, http.StatusOK},
	} {
		if rec := a.do(http.MethodGet, "/api/v1/admin/test", "", tc.headers); rec.Code != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, rec.Code, tc.want)
		}
	}
	// The session endpoint never 401s and tells the UI what to show.
	rec := a.do(http.MethodGet, "/api/v1/auth/session", "", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != `{"admin_login_required":true,"user":null}`+"\n" {
		t.Fatalf("session (none): %d %q", rec.Code, rec.Body)
	}
	rec = a.do(http.MethodGet, "/api/v1/auth/session", "", admin)
	if !strings.Contains(rec.Body.String(), `"username":"root","role":"admin"`) {
		t.Fatalf("session (admin): %s", rec.Body)
	}
	// Sign-in itself stays reachable.
	if rec := a.do(http.MethodPost, "/api/v1/auth/login", `{"username":"root","password":"nope nope nope"}`, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("login with admin login required: %d", rec.Code)
	}
}

func TestAdminOpenWithoutFlag(t *testing.T) {
	a := newAuthEnvWith(t, false)
	if rec := a.do(http.MethodGet, "/api/v1/admin/test", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("admin without flag: %d", rec.Code)
	}
	if rec := a.do(http.MethodGet, "/api/v1/admin/test", "", map[string]string{"Cookie": handler.SessionCookie + "=mrs_bogus"}); rec.Code != http.StatusOK {
		t.Fatalf("admin without flag, stale cookie: %d", rec.Code)
	}
	if rec := a.do(http.MethodGet, "/api/v1/auth/session", "", nil); !strings.Contains(rec.Body.String(), `"admin_login_required":false`) {
		t.Fatalf("session: %s", rec.Body)
	}
}

func TestAdminUserManagement(t *testing.T) {
	a := newAuthEnvWith(t, true)
	admin := a.login(t, "root")
	alice := a.login(t, "alice")
	call := func(method, path, body string) *httptest.ResponseRecorder { return a.do(method, path, body, admin) }

	if rec := call(http.MethodGet, "/api/v1/admin/users", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"username":"alice"`) {
		t.Fatalf("list users: %d %s", rec.Code, rec.Body)
	}
	if rec := call(http.MethodPost, "/api/v1/admin/users", `{"username":"bob","password":"long enough pw"}`); rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"role":"reader"`) {
		t.Fatalf("create bob: %d %s", rec.Code, rec.Body)
	}
	for _, tc := range []struct {
		name, method, path, body string
		want                     int
	}{
		{"duplicate", http.MethodPost, "/api/v1/admin/users", `{"username":"BOB","password":"long enough pw"}`, http.StatusConflict},
		{"short password", http.MethodPost, "/api/v1/admin/users", `{"username":"carol","password":"short"}`, http.StatusBadRequest},
		{"bad role", http.MethodPatch, "/api/v1/admin/users/bob", `{"role":"root"}`, http.StatusBadRequest},
		{"unknown user", http.MethodPatch, "/api/v1/admin/users/nobody", `{"disabled":true}`, http.StatusNotFound},
		{"demote self", http.MethodPatch, "/api/v1/admin/users/root", `{"role":"reader"}`, http.StatusBadRequest},
		{"disable self", http.MethodPatch, "/api/v1/admin/users/ROOT", `{"disabled":true}`, http.StatusBadRequest},
		{"delete self", http.MethodDelete, "/api/v1/admin/users/root", ``, http.StatusBadRequest},
		{"unknown session", http.MethodDelete, "/api/v1/admin/sessions/999999", ``, http.StatusNotFound},
	} {
		if rec := call(tc.method, tc.path, tc.body); rec.Code != tc.want {
			t.Errorf("%s: %d %s, want %d", tc.name, rec.Code, rec.Body, tc.want)
		}
	}

	// Sessions: the caller's own is marked current.
	rec := call(http.MethodGet, "/api/v1/admin/sessions", "")
	var sessions []service.SessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &sessions); err != nil || len(sessions) != 2 {
		t.Fatalf("sessions: %s (%v)", rec.Body, err)
	}
	var aliceSession int64
	current := 0
	for _, s := range sessions {
		if s.Current {
			current++
			if s.Username != "root" {
				t.Errorf("current session belongs to %s", s.Username)
			}
		}
		if s.Username == "alice" {
			aliceSession = s.ID
		}
	}
	if current != 1 {
		t.Fatalf("%d sessions marked current, want 1", current)
	}
	if rec := a.do(http.MethodGet, "/api/v1/test", "", alice); rec.Code != http.StatusOK {
		t.Fatalf("alice before revoke: %d", rec.Code)
	}
	if rec := call(http.MethodDelete, "/api/v1/admin/sessions/"+strconv.FormatInt(aliceSession, 10), ""); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke alice's session: %d", rec.Code)
	}
	if rec := a.do(http.MethodGet, "/api/v1/test", "", alice); rec.Code != http.StatusUnauthorized {
		t.Fatalf("alice after revoke: %d, want 401", rec.Code)
	}

	// Promoting alice gives her the console; her own password change keeps
	// her session, while an admin resetting it signs her out.
	if rec := call(http.MethodPatch, "/api/v1/admin/users/alice", `{"role":"admin"}`); rec.Code != http.StatusOK {
		t.Fatalf("promote alice: %d %s", rec.Code, rec.Body)
	}
	alice = a.login(t, "alice")
	if rec := a.do(http.MethodGet, "/api/v1/admin/test", "", alice); rec.Code != http.StatusOK {
		t.Fatalf("alice as admin: %d", rec.Code)
	}
	other := a.login(t, "alice")
	if rec := a.do(http.MethodPatch, "/api/v1/admin/users/alice", `{"password":"long enough pw"}`, alice); rec.Code != http.StatusOK {
		t.Fatalf("alice changes own password: %d %s", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodGet, "/api/v1/admin/test", "", alice); rec.Code != http.StatusOK {
		t.Fatalf("alice's own session after own password change: %d", rec.Code)
	}
	if rec := a.do(http.MethodGet, "/api/v1/admin/test", "", other); rec.Code != http.StatusUnauthorized {
		t.Fatalf("alice's other session after own password change: %d, want 401", rec.Code)
	}
	if rec := call(http.MethodPatch, "/api/v1/admin/users/alice", `{"password":"long enough pw"}`); rec.Code != http.StatusOK {
		t.Fatalf("reset alice's password: %d", rec.Code)
	}
	if rec := a.do(http.MethodGet, "/api/v1/admin/test", "", alice); rec.Code != http.StatusUnauthorized {
		t.Fatalf("alice after admin reset: %d, want 401", rec.Code)
	}

	// Sign a user out everywhere, disable, delete.
	bob := a.login(t, "bob")
	if rec := call(http.MethodPost, "/api/v1/admin/users/bob/logout", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sessions_ended":1`) {
		t.Fatalf("logout bob: %d %s", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodGet, "/api/v1/test", "", bob); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bob after logout: %d", rec.Code)
	}
	if rec := call(http.MethodPatch, "/api/v1/admin/users/bob", `{"disabled":true}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"disabled":true`) {
		t.Fatalf("disable bob: %d %s", rec.Code, rec.Body)
	}
	if rec := call(http.MethodDelete, "/api/v1/admin/users/bob", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete bob: %d", rec.Code)
	}
	if rec := call(http.MethodDelete, "/api/v1/admin/users/bob", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("delete bob twice: %d", rec.Code)
	}
}
