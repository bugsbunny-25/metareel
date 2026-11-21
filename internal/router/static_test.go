package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v5"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// staticServer builds the two app trees under a temp working directory (echo
// resolves c.File relative to the directory it was created in) and registers them.
func staticServer(t *testing.T) *echo.Echo {
	t.Helper()
	t.Chdir(t.TempDir())
	writeFile(t, "web/dist/index.html", "admin index")
	writeFile(t, "web/dist/assets/admin.js", "admin js")
	writeFile(t, "app/dist/index.html", "app index")
	writeFile(t, "app/dist/assets/app.js", "app js")
	e := echo.New()
	registerStatic(e, "web/dist", "app/dist")
	return e
}

func get(e *echo.Echo, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestRegisterStatic_TwoApps(t *testing.T) {
	e := staticServer(t)
	cases := []struct{ path, want string }{
		{"/", "app index"},
		{"/assets/app.js", "app js"},
		{"/charts", "app index"},                            // SPA fallback
		{"/admin", "admin index"},                           // console home, no trailing slash
		{"/admin/", "admin index"},                          // console home
		{"/admin/assets/admin.js", "admin js"},              // console assets (Vite base /admin/)
		{"/admin/titles", "admin index"},                    // console fallback
		{"/assets/admin.js", "app index"},                   // admin assets are not at the root any more
		{"/admin/../app/dist/assets/app.js", "admin index"}, // ".." can't climb out of the admin dir
	}
	for _, tc := range cases {
		rec := get(e, tc.path)
		if rec.Code != http.StatusOK || rec.Body.String() != tc.want {
			t.Errorf("GET %s = %d %q, want 200 %q", tc.path, rec.Code, rec.Body.String(), tc.want)
		}
	}
}

func TestRegisterStatic_NoTraversalOutOfAppDir(t *testing.T) {
	e := staticServer(t)
	writeFile(t, "secret.txt", "secret")
	for _, p := range []string{"/../secret.txt", "/admin/../../secret.txt", "/%2e%2e/secret.txt"} {
		if rec := get(e, p); rec.Body.String() == "secret" {
			t.Errorf("GET %s served a file outside the app directories", p)
		}
	}
}
