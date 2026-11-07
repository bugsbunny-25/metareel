package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/config"
	"github.com/bugsbunny-25/metareel/internal/handler"
)

func newTestEcho() *echo.Echo {
	e := echo.New()
	Register(e, &handler.Handler{}, &config.Config{})
	return e
}

// TestCORSOnlyOnPublicAPI: other websites get CORS headers for the public
// API, never for the admin API.
func TestCORSOnlyOnPublicAPI(t *testing.T) {
	e := newTestEcho()
	preflight := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodOptions, path, nil)
		req.Header.Set("Origin", "https://evil.example")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		req.Header.Set("Access-Control-Request-Headers", "content-type")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	if got := preflight("/api/v1/top10/movies/US").Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("public preflight Allow-Origin = %q, want *", got)
	}
	for _, p := range []string{"/api/v1/admin/webhooks", "/api/v1/admin/webhooks/1/rotate-secret", "/api/v1/admin"} {
		if got := preflight(p).Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("admin preflight %s Allow-Origin = %q, want none", p, got)
		}
	}
}

// TestAdminRejectsCrossSiteWrites: a page on another site can't send
// state-changing requests to the admin API (even bodiless POSTs that need no
// preflight); same-origin browsers and non-browser clients can.
func TestAdminRejectsCrossSiteWrites(t *testing.T) {
	e := newTestEcho()
	do := func(method, path string, headers map[string]string) int {
		req := httptest.NewRequest(method, path, nil)
		req.Host = "metareel.lan:5080"
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		func() {
			defer func() { _ = recover() }() // allowed requests reach handlers with nil services
			e.ServeHTTP(rec, req)
		}()
		return rec.Code
	}
	for _, h := range []map[string]string{
		{"Sec-Fetch-Site": "cross-site"},
		{"Sec-Fetch-Site": "same-site"},
		{"Origin": "https://evil.example"},
		{"Origin": "null"},
		{"Origin": "http://metareel.lan:5081"},
	} {
		if code := do(http.MethodPost, "/api/v1/admin/webhooks/1/rotate-secret", h); code != http.StatusForbidden {
			t.Errorf("POST with %v = %d, want 403", h, code)
		}
		if code := do(http.MethodDelete, "/api/v1/admin/webhooks/1", h); code != http.StatusForbidden {
			t.Errorf("DELETE with %v = %d, want 403", h, code)
		}
	}
	for _, h := range []map[string]string{
		{},
		{"Sec-Fetch-Site": "same-origin", "Origin": "http://metareel.lan:5080"},
		{"Sec-Fetch-Site": "none"},
		{"Origin": "https://admin.example", "X-Forwarded-Host": "admin.example"},
	} {
		if code := do(http.MethodPost, "/api/v1/admin/webhooks/abc/rotate-secret", h); code == http.StatusForbidden {
			t.Errorf("POST with %v was refused", h)
		}
	}
	// Reads are never refused by the guard.
	if code := do(http.MethodGet, "/api/v1/admin/webhooks/abc/deliveries", map[string]string{"Sec-Fetch-Site": "cross-site"}); code == http.StatusForbidden {
		t.Error("cross-site GET refused")
	}
}
