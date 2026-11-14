package router

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCinemaSubscriptions(t *testing.T) {
	a := newTrackingEnv(t)
	alice := a.login(t, "alice")
	bob := a.login(t, "bob")

	if rec := a.do(http.MethodGet, "/api/v1/me/subscriptions", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("no session: %d", rec.Code)
	}
	rec := a.do(http.MethodPost, "/api/v1/me/subscriptions",
		`{"plan":"AMC Stubs A-List","chain":"AMC","price_minor":2399,"currency":"usd","started_on":"2026-09-10"}`, alice)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var sub struct {
		ID       int64  `json:"id"`
		Currency string `json:"currency"`
		Period   string `json:"period"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)
	if sub.ID == 0 || sub.Currency != "USD" || sub.Period != "month" {
		t.Fatalf("created = %s", rec.Body)
	}
	id := itoa(sub.ID)
	for _, body := range []string{`{"plan":"","price_minor":1,"currency":"USD","started_on":"2026-01-01"}`,
		`{"plan":"X","currency":"USD","started_on":"2026-01-01"}`, `{"plan":"X","price_minor":1,"currency":"US","started_on":"2026-01-01"}`,
		`{"plan":"X","price_minor":1,"currency":"USD","started_on":"2026-01-01","period":"week"}`,
		`{"plan":"X","price_minor":1,"currency":"USD","started_on":"2026-02-01","ended_on":"2026-01-01"}`} {
		if rec := a.do(http.MethodPost, "/api/v1/me/subscriptions", body, alice); rec.Code != http.StatusBadRequest {
			t.Errorf("bad %s: %d", body, rec.Code)
		}
	}

	// A watch covered by it, and one Bob can't cover with Alice's plan.
	rec = a.do(http.MethodPost, "/api/v1/me/watches",
		`{"tmdb_kind":"movie","tmdb_id":"27205","ended_at":"2026-09-12T22:00:00Z","venue_kind":"theater","subscription_id":`+id+`,"face_value_minor":2200,"price_minor":500,"currency":"USD"}`, alice)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("covered watch: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"subscription_id":`+id) || !strings.Contains(rec.Body.String(), `"face_value_minor":2200`) {
		t.Fatalf("watch = %s", rec.Body)
	}
	if rec := a.do(http.MethodPost, "/api/v1/me/watches",
		`{"tmdb_kind":"movie","tmdb_id":"27205","ended_at":"2026-09-12T22:00:00Z","venue_kind":"theater","subscription_id":`+id+`}`, bob); rec.Code != http.StatusBadRequest {
		t.Errorf("bob using alice's plan: %d %s", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodPost, "/api/v1/me/watches",
		`{"tmdb_kind":"movie","tmdb_id":"27205","ended_at":"2026-09-12T22:00:00Z","face_value_minor":100}`, alice); rec.Code != http.StatusBadRequest {
		t.Errorf("face value without currency: %d", rec.Code)
	}

	rec = a.do(http.MethodGet, "/api/v1/me/subscriptions/savings", "", alice)
	var sav struct {
		Subscriptions []struct {
			Watches, Bills int
			PaidMinor      int64 `json:"paid_minor"`
			ValueMinor     int64 `json:"value_minor"`
			ExtrasMinor    int64 `json:"extras_minor"`
			SavedMinor     int64 `json:"saved_minor"`
		} `json:"subscriptions"`
		WhatIf []any `json:"what_if"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sav); err != nil || len(sav.Subscriptions) != 1 {
		t.Fatalf("savings: %d %s", rec.Code, rec.Body)
	}
	if s := sav.Subscriptions[0]; s.Watches != 1 || s.Bills < 3 || s.ValueMinor != 2200 || s.ExtrasMinor != 500 || s.SavedMinor != 2200-500-s.PaidMinor {
		t.Fatalf("savings = %s", rec.Body)
	}
	if rec := a.do(http.MethodGet, "/api/v1/me/subscriptions", "", bob); !strings.Contains(rec.Body.String(), `"subscriptions":[]`) {
		t.Errorf("bob sees alice's plans: %s", rec.Body)
	}
	if rec := a.do(http.MethodPut, "/api/v1/me/subscriptions/"+id, `{"plan":"X","price_minor":1,"currency":"USD","started_on":"2026-01-01"}`, bob); rec.Code != http.StatusNotFound {
		t.Errorf("bob edits alice's plan: %d", rec.Code)
	}
	rec = a.do(http.MethodPut, "/api/v1/me/subscriptions/"+id, `{"plan":"AMC Stubs A-List","chain":"AMC","price_minor":2399,"currency":"USD","started_on":"2026-09-10","ended_on":"2026-10-31"}`, alice)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ended_on":"2026-10-31"`) {
		t.Fatalf("end plan: %d %s", rec.Code, rec.Body)
	}
	// Deleting the plan keeps the watch, no longer covered.
	if rec := a.do(http.MethodDelete, "/api/v1/me/subscriptions/"+id, "", alice); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	rec = a.do(http.MethodGet, "/api/v1/me/watches/1", "", alice)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"subscription_id"`) || !strings.Contains(rec.Body.String(), `"face_value_minor":2200`) {
		t.Fatalf("watch after plan deleted: %s", rec.Body)
	}
}
