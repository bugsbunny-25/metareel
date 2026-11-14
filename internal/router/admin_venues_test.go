package router

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The console manages cinemas through the admin copies of the venue routes.
func TestAdminVenueEndpoints(t *testing.T) {
	a := newTrackingEnv(t)
	alice := a.login(t, "alice")

	rec := a.do(http.MethodPost, "/api/v1/admin/venues",
		`{"name":"Prince Charles Cinema","city":"London","country":"gb","timezone":"Europe/London"}`, nil)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create venue: %d %s", rec.Code, rec.Body)
	}
	var created struct {
		Venue struct {
			ID      int64  `json:"id"`
			Country string `json:"country"`
		} `json:"venue"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	id := itoa(created.Venue.ID)
	if created.Venue.ID == 0 || created.Venue.Country != "GB" {
		t.Fatalf("created = %s", rec.Body)
	}

	rec = a.do(http.MethodPut, "/api/v1/admin/venues/"+id,
		`{"name":"Prince Charles Cinema","chain":"Independent","city":"London","country":"GB","timezone":"Europe/London","status":"open","note":"Leicester Place"}`, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"chain":"Independent"`) {
		t.Fatalf("update venue: %d %s", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodPut, "/api/v1/admin/venues/"+id, `{"name":"X","timezone":"Mars/Olympus"}`, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad time zone: %d, want 400", rec.Code)
	}

	rec = a.do(http.MethodPost, "/api/v1/admin/venues/"+id+"/screens", `{"name":"Downstairs","seats":285,"width":12.5,"height":5.2,"dim_unit":"m","aspect_ratio":"2.39:1"}`, nil)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("add screen: %d %s", rec.Code, rec.Body)
	}
	var screen struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &screen)
	sid := itoa(screen.ID)
	rec = a.do(http.MethodPut, "/api/v1/admin/venues/"+id+"/screens/"+sid, `{"name":"Downstairs","seats":300,"dim_unit":"ft","width":41,"height":17}`, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"seats":300`) {
		t.Fatalf("update screen: %d %s", rec.Code, rec.Body)
	}
	rec = a.do(http.MethodPost, "/api/v1/admin/venues/"+id+"/aliases", `{"name":"Prince Charles","to_date":"1991-12-31"}`, nil)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("add alias: %d %s", rec.Code, rec.Body)
	}
	rec = a.do(http.MethodGet, "/api/v1/admin/venues/"+id, "", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"Downstairs"`) || !strings.Contains(rec.Body.String(), `"Prince Charles"`) {
		t.Fatalf("get venue: %d %s", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodGet, "/api/v1/admin/venues?q=prince", "", nil); !strings.Contains(rec.Body.String(), `"Prince Charles Cinema"`) {
		t.Errorf("search: %s", rec.Body)
	}

	// A cinema someone watched at can't be deleted; closing it is the way.
	rec = a.do(http.MethodPost, "/api/v1/me/watches",
		`{"tmdb_kind":"movie","tmdb_id":"27205","ended_at":"2026-11-12T22:00:00Z","venue_kind":"theater","venue_id":`+id+`,"screen_id":`+sid+`}`, alice)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("log a watch there: %d %s", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodDelete, "/api/v1/admin/venues/"+id, "", nil); rec.Code != http.StatusConflict {
		t.Errorf("delete a cinema with watches: %d, want 409", rec.Code)
	}
	rec = a.do(http.MethodPut, "/api/v1/admin/venues/"+id, `{"name":"Prince Charles Cinema","country":"GB","status":"closed","closed_on":"2030-01-01"}`, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"closed"`) {
		t.Errorf("close venue: %d %s", rec.Code, rec.Body)
	}

	// An unused cinema can go.
	rec = a.do(http.MethodPost, "/api/v1/admin/venues", `{"name":"Typo Cinema"}`, nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if rec := a.do(http.MethodDelete, "/api/v1/admin/venues/"+itoa(created.Venue.ID), "", nil); rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
		t.Errorf("delete an unused cinema: %d %s", rec.Code, rec.Body)
	}
}
