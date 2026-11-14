package router

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A user's own account: their sessions, their password and their
// preferences, never anyone else's.
func TestAccountEndpoints(t *testing.T) {
	a := newTrackingEnv(t)
	laptop := a.login(t, "alice")
	phone := a.login(t, "alice")
	bob := a.login(t, "bob")

	type session struct {
		ID      int64 `json:"id"`
		Current bool  `json:"current"`
	}
	account := func(h map[string]string) (string, []session) {
		t.Helper()
		rec := a.do(http.MethodGet, "/api/v1/me/account", "", h)
		if rec.Code != http.StatusOK {
			t.Fatalf("account: %d %s", rec.Code, rec.Body)
		}
		var out struct {
			User struct {
				Username string `json:"username"`
			} `json:"user"`
			Sessions []session `json:"sessions"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.User.Username, out.Sessions
	}

	name, sessions := account(laptop)
	if name != "alice" || len(sessions) != 2 {
		t.Fatalf("alice's account = %s with %d sessions, want alice with 2 (not bob's)", name, len(sessions))
	}
	var phoneID int64
	current := 0
	for _, s := range sessions {
		if s.Current {
			current++
		} else {
			phoneID = s.ID
		}
	}
	if current != 1 || phoneID == 0 {
		t.Fatalf("sessions = %+v, want exactly one marked current", sessions)
	}

	// Bob can't sign alice's phone out; alice can, but not her own session.
	if rec := a.do(http.MethodDelete, "/api/v1/me/sessions/"+itoa(phoneID), "", bob); rec.Code != http.StatusNotFound {
		t.Errorf("bob ending alice's session: %d, want 404", rec.Code)
	}
	_, bobSessions := account(bob)
	if rec := a.do(http.MethodDelete, "/api/v1/me/sessions/"+itoa(bobSessions[0].ID), "", bob); rec.Code != http.StatusBadRequest {
		t.Errorf("ending your own current session here: %d, want 400", rec.Code)
	}
	if rec := a.do(http.MethodDelete, "/api/v1/me/sessions/"+itoa(phoneID), "", laptop); rec.Code != http.StatusNoContent {
		t.Fatalf("alice ending her phone's session: %d %s", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodGet, "/api/v1/me/account", "", phone); rec.Code != http.StatusUnauthorized {
		t.Errorf("the ended session still works: %d", rec.Code)
	}

	// Changing the password checks the current one, keeps this session and
	// ends the others.
	tablet := a.login(t, "alice")
	for _, tc := range []struct{ body, want string }{
		{`{"current_password":"wrong one here","new_password":"a brand new pw"}`, "current password"},
		{`{"current_password":"long enough pw","new_password":"long enough pw"}`, "same as the current"},
		{`{"current_password":"long enough pw","new_password":"short"}`, ""},
	} {
		rec := a.do(http.MethodPost, "/api/v1/me/password", tc.body, laptop)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("password %s: %d %s, want 400 mentioning %q", tc.body, rec.Code, rec.Body, tc.want)
		}
	}
	rec := a.do(http.MethodPost, "/api/v1/me/password", `{"current_password":"long enough pw","new_password":"a brand new pw"}`, laptop)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"other_sessions_ended":1`) {
		t.Fatalf("change password: %d %s", rec.Code, rec.Body)
	}
	if rec := a.do(http.MethodGet, "/api/v1/me/account", "", laptop); rec.Code != http.StatusOK {
		t.Errorf("the session that changed the password was signed out: %d", rec.Code)
	}
	if rec := a.do(http.MethodGet, "/api/v1/me/account", "", tablet); rec.Code != http.StatusUnauthorized {
		t.Errorf("another session survived the password change: %d", rec.Code)
	}
	if rec := a.do(http.MethodPost, "/api/v1/auth/login", `{"username":"alice","password":"a brand new pw"}`, nil); rec.Code != http.StatusOK {
		t.Errorf("signing in with the new password: %d", rec.Code)
	}

	// Sign out everywhere else (two sessions with the new password: the
	// sign-in just above and this one).
	_ = a.do(http.MethodPost, "/api/v1/auth/login", `{"username":"alice","password":"a brand new pw"}`, nil)
	rec = a.do(http.MethodDelete, "/api/v1/me/sessions", "", laptop)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ended":2`) {
		t.Errorf("sign out everywhere else: %d %s, want 2 ended", rec.Code, rec.Body)
	}

	// Preferences: validated, replaced as a whole, per user.
	rec = a.do(http.MethodGet, "/api/v1/me/preferences", "", laptop)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"preferences":{}`) {
		t.Fatalf("empty preferences: %d %s", rec.Code, rec.Body)
	}
	rec = a.do(http.MethodPut, "/api/v1/me/preferences",
		`{"country":"gb","provider":"netflix","currency":"gbp","screen_unit":"ft","stale_days":60}`, laptop)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"country":"GB"`) || !strings.Contains(rec.Body.String(), `"currency":"GBP"`) {
		t.Fatalf("save preferences: %d %s", rec.Code, rec.Body)
	}
	for _, bad := range []string{`{"country":"GBR"}`, `{"currency":"pounds"}`, `{"screen_unit":"cm"}`, `{"stale_days":3}`, `{"provider":"Net Flix"}`} {
		if rec := a.do(http.MethodPut, "/api/v1/me/preferences", bad, laptop); rec.Code != http.StatusBadRequest {
			t.Errorf("preferences %s: %d, want 400", bad, rec.Code)
		}
	}
	if rec := a.do(http.MethodGet, "/api/v1/me/preferences", "", bob); !strings.Contains(rec.Body.String(), `"preferences":{}`) {
		t.Errorf("bob sees alice's preferences: %s", rec.Body)
	}
	// The stale-days preference is the queue's default.
	if rec := a.do(http.MethodGet, "/api/v1/me/shows", "", laptop); !strings.Contains(rec.Body.String(), `"stale_days":60`) {
		t.Errorf("queue ignores the stale_days preference: %s", rec.Body)
	}
	rec = a.do(http.MethodPut, "/api/v1/me/preferences", `{"currency":"USD"}`, laptop)
	if strings.Contains(rec.Body.String(), `"country"`) {
		t.Errorf("a PUT keeps fields it didn't send: %s", rec.Body)
	}

	for _, path := range []string{"/api/v1/me/account", "/api/v1/me/preferences"} {
		if rec := a.do(http.MethodGet, path, "", map[string]string{"X-API-Key": a.apiKey}); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s with an api key: %d, want 401", path, rec.Code)
		}
	}
}
