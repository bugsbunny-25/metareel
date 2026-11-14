package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

// The ETag middleware keeps a handler's private caching but never lets a
// keyed response be marked cacheable by shared caches.
func TestETagCacheControl(t *testing.T) {
	for _, tc := range []struct{ set, want string }{
		{"", "private, no-cache"},
		{"private, max-age=31536000, immutable", "private, max-age=31536000, immutable"},
		{"no-store", "no-store"},
		{"public, max-age=600", "private, no-cache"},
		{"max-age=600", "private, no-cache"},
	} {
		e := echo.New()
		e.GET("/x", ETag(func(c *echo.Context) error {
			if tc.set != "" {
				c.Response().Header().Set("Cache-Control", tc.set)
			}
			return c.String(http.StatusOK, "hello")
		}))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if got := rec.Header().Get("Cache-Control"); got != tc.want {
			t.Errorf("handler set %q: Cache-Control = %q, want %q", tc.set, got, tc.want)
		}
		if rec.Header().Get("ETag") == "" {
			t.Errorf("handler set %q: no ETag", tc.set)
		}
	}
}
