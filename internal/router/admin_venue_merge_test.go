package router

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A duplicate cinema (typed in by a user, then imported) folds into the one
// kept: watches, screens, former names and outside IDs move; the diary keeps
// the name the watch was logged with.
func TestAdminVenueMerge(t *testing.T) {
	a := newTrackingEnv(t)
	alice := a.login(t, "alice")
	create := func(body string) string {
		rec := a.do(http.MethodPost, "/api/v1/admin/venues", body, nil)
		var v struct {
			Venue struct {
				ID int64 `json:"id"`
			} `json:"venue"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &v)
		if v.Venue.ID == 0 {
			t.Fatalf("create venue: %d %s", rec.Code, rec.Body)
		}
		return itoa(v.Venue.ID)
	}
	dup := create(`{"name":"PVR ICON Phoenix Palladium","city":"Mumbai","country":"IN","timezone":"Asia/Kolkata","chain":"PVR INOX"}`)
	kept := create(`{"name":"PVR Cinemas","city":"Mumbai","country":"IN","latitude":18.9951,"longitude":72.8239}`)
	rec := a.do(http.MethodPost, "/api/v1/admin/venues/"+dup+"/screens", `{"name":"Audi 1","seats":120}`, nil)
	var screen struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &screen)
	a.do(http.MethodPost, "/api/v1/admin/venues/"+dup+"/aliases", `{"name":"PVR Phoenix"}`, nil)
	rec = a.do(http.MethodPost, "/api/v1/me/watches",
		`{"tmdb_kind":"movie","tmdb_id":"27205","ended_at":"2026-11-12T22:00:00Z","venue_kind":"theater","venue_id":`+dup+`,"screen_id":`+itoa(screen.ID)+`}`, alice)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("log a watch: %d %s", rec.Code, rec.Body)
	}

	// Users can't merge: it rewrites other people's watches.
	if rec := a.do(http.MethodPost, "/api/v1/me/venues/"+dup+"/merge", `{"into":`+kept+`}`, alice); rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("merge through /me: %d", rec.Code)
	}
	for _, body := range []string{`{"into":` + dup + `}`, `{}`, `{"into":99999}`} {
		if rec := a.do(http.MethodPost, "/api/v1/admin/venues/"+dup+"/merge", body, nil); rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
			t.Errorf("merge %s: %d %s", body, rec.Code, rec.Body)
		}
	}

	rec = a.do(http.MethodPost, "/api/v1/admin/venues/"+dup+"/merge", `{"into":`+kept+`}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("merge: %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Venue struct {
			Name     string  `json:"name"`
			Chain    *string `json:"chain"`
			Timezone *string `json:"timezone"`
		} `json:"venue"`
		Screens []struct{ Name string } `json:"screens"`
		Aliases []struct{ Name string } `json:"aliases"`
		Watches int                     `json:"watch_count"`
		Merged  struct {
			Watches, Screens, Aliases int
			Filled                    bool
		} `json:"merged"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Venue.Name != "PVR Cinemas" || out.Venue.Chain == nil || *out.Venue.Chain != "PVR INOX" || *out.Venue.Timezone != "Asia/Kolkata" ||
		len(out.Screens) != 1 || len(out.Aliases) != 2 || out.Merged.Watches != 1 || out.Merged.Screens != 1 || out.Merged.Aliases != 2 || !out.Merged.Filled {
		t.Fatalf("merged = %s", rec.Body)
	}
	if rec := a.do(http.MethodGet, "/api/v1/admin/venues/"+dup, "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("merged venue still there: %d", rec.Code)
	}
	// The watch now points at the kept cinema but still reads as logged.
	rec = a.do(http.MethodGet, "/api/v1/me/watches?limit=5", "", alice)
	if !strings.Contains(rec.Body.String(), `"venue_id":`+kept) || !strings.Contains(rec.Body.String(), "PVR ICON Phoenix Palladium") {
		t.Fatalf("watch after merge: %s", rec.Body)
	}
}
