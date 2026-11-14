package router

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"strings"
	"testing"
)

// Admins upload a format logo; both the user API and the admin API hand out
// a versioned URL to it, served with a locked-down CSP.
func TestFormatLogos(t *testing.T) {
	a := newTrackingEnv(t)
	alice := a.login(t, "alice")

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	logo := buf.String()

	// Find IMAX's id in the admin list.
	rec := a.do(http.MethodGet, "/api/v1/admin/watch-formats", "", nil)
	var list struct {
		Formats []struct {
			ID      int64  `json:"id"`
			Slug    string `json:"slug"`
			LogoURL string `json:"logo_url"`
		} `json:"formats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("admin formats: %d %s", rec.Code, rec.Body)
	}
	var id string
	for _, f := range list.Formats {
		if f.Slug == "imax" {
			id = itoa(f.ID)
			if f.LogoURL != "" {
				t.Fatalf("imax has a logo before any upload: %q", f.LogoURL)
			}
		}
	}

	if rec := a.do(http.MethodPut, "/api/v1/admin/watch-formats/"+id+"/logo", "not an image", map[string]string{"Content-Type": "image/png"}); rec.Code != http.StatusBadRequest {
		t.Errorf("garbage claiming to be a PNG: %d, want 400", rec.Code)
	}
	if rec := a.do(http.MethodPut, "/api/v1/admin/watch-formats/999999/logo", logo, nil); rec.Code != http.StatusNotFound {
		t.Errorf("logo for an unknown format: %d, want 404", rec.Code)
	}
	rec = a.do(http.MethodPut, "/api/v1/admin/watch-formats/"+id+"/logo", logo, map[string]string{"Content-Type": "image/png"})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"logo_url":"/api/v1/admin/formats/imax/logo?v=`) {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}

	// The user API's format list points at the public route.
	rec = a.do(http.MethodGet, "/api/v1/me/formats", "", alice)
	var mine struct {
		Formats []struct {
			Slug    string `json:"slug"`
			LogoURL string `json:"logo_url"`
		} `json:"formats"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &mine)
	var url string
	for _, f := range mine.Formats {
		if f.Slug == "imax" {
			url = f.LogoURL
		}
	}
	if !strings.HasPrefix(url, "/api/v1/formats/imax/logo?v=") {
		t.Fatalf("user logo_url = %q", url)
	}

	rec = a.do(http.MethodGet, url, "", alice)
	h := rec.Header()
	if rec.Code != http.StatusOK || h.Get("Content-Type") != "image/png" || rec.Body.String() != logo {
		t.Fatalf("serve logo: %d %s", rec.Code, h.Get("Content-Type"))
	}
	if !strings.Contains(h.Get("Content-Security-Policy"), "sandbox") || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("logo headers: CSP %q, nosniff %q", h.Get("Content-Security-Policy"), h.Get("X-Content-Type-Options"))
	}
	if !strings.Contains(h.Get("Cache-Control"), "immutable") {
		t.Errorf("versioned logo Cache-Control = %q, want immutable", h.Get("Cache-Control"))
	}
	if rec := a.do(http.MethodGet, "/api/v1/formats/imax/logo?v=old", "", alice); !strings.Contains(rec.Header().Get("Cache-Control"), "no-cache") {
		t.Errorf("stale version Cache-Control = %q, want no-cache", rec.Header().Get("Cache-Control"))
	}
	if rec := a.do(http.MethodGet, "/api/v1/admin/formats/imax/logo", "", nil); rec.Code != http.StatusOK {
		t.Errorf("admin copy of the logo: %d", rec.Code)
	}
	if rec := a.do(http.MethodGet, "/api/v1/formats/imax/logo", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("public logo without credentials: %d, want 401", rec.Code)
	}
	if rec := a.do(http.MethodGet, "/api/v1/formats/dolby-atmos/logo", "", alice); rec.Code != http.StatusNotFound {
		t.Errorf("a format without a logo: %d, want 404", rec.Code)
	}

	// Removing it.
	rec = a.do(http.MethodDelete, "/api/v1/admin/watch-formats/"+id+"/logo", "", nil)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "logo_url") {
		t.Errorf("delete logo: %d %s", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodGet, "/api/v1/formats/imax/logo", "", alice); rec.Code != http.StatusNotFound {
		t.Errorf("logo after delete: %d, want 404", rec.Code)
	}
}
