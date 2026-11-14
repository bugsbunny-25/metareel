package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOverpassCinemas(t *testing.T) {
	status, body := 200, ""
	var gotQuery, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotQuery, gotUA = r.PostForm.Get("data"), r.Header.Get("User-Agent")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	defer srv.Close()
	c := NewOverpass(srv.URL)
	ctx := context.Background()

	body = `{"elements": [
	  {"type": "node", "id": 1, "lat": 51.511, "lon": -0.130, "tags": {"amenity": "cinema", "name": "Prince Charles Cinema"}},
	  {"type": "way", "id": 2, "center": {"lat": 19.0, "lon": 72.8}, "tags": {"amenity": "cinema", "name": "PVR"}},
	  {"type": "relation", "id": 3, "tags": {"name": "No position"}},
	  {"type": "node", "id": 4, "lat": 1, "lon": 2}]}`
	got, err := c.Cinemas(ctx, "GB")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Ref() != "node/1" || got[1].Ref() != "way/2" || got[1].Lat != 19.0 || got[0].Tags["name"] != "Prince Charles Cinema" || got[2].Tags == nil {
		t.Fatalf("places = %+v", got)
	}
	if !strings.Contains(gotQuery, `area["ISO3166-1"="GB"][admin_level=2]`) || !strings.Contains(gotQuery, `nwr["amenity"="cinema"]`) || !strings.Contains(gotQuery, "out center tags") {
		t.Fatalf("query = %s", gotQuery)
	}
	if !strings.HasPrefix(gotUA, "metareel/") {
		t.Fatalf("user agent = %q", gotUA)
	}

	if _, err := c.Towns(ctx, "GB"); err != nil || !strings.Contains(gotQuery, `node["place"~"^(city|town)$"](area.c)`) {
		t.Fatalf("towns query = %s, %v", gotQuery, err)
	}
	if _, err := c.Cinemas(ctx, `GB"];node(1);//`); err == nil {
		t.Fatal("country code not validated")
	}
	body = `{"remark": "runtime error: Query timed out in \"query\" at line 1 after 301 seconds.", "elements": []}`
	if _, err := c.Cinemas(ctx, "US"); !errors.Is(err, ErrOverpassBusy) {
		t.Fatalf("timed-out query: %v", err)
	}
	status, body = 429, ""
	if _, err := c.Cinemas(ctx, "US"); !errors.Is(err, ErrOverpassBusy) {
		t.Fatalf("429: %v", err)
	}
	status = 400
	if _, err := c.Cinemas(ctx, "US"); err == nil || errors.Is(err, ErrOverpassBusy) {
		t.Fatalf("400: %v", err)
	}
}
