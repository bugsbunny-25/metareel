package router

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	if _, err := service.NewUserService(log, userRepo, sessions).Create(ctx, "alice", "long enough pw", service.RoleReader); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	Register(e, &handler.Handler{APIKeys: keys, Sessions: sessions, Auth: service.NewAuthService(keys, sessions)}, &config.Config{})
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
