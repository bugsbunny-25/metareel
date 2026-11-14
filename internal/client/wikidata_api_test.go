package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestWikidataClosedCinemas(t *testing.T) {
	var searches []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("action") {
		case "query":
			searches = append(searches, q.Get("srsort")+"@"+q.Get("sroffset"))
			if !strings.Contains(q.Get("srsearch"), "haswbstatement:P17=Q145") || !strings.Contains(q.Get("srsearch"), "P5817=Q104664889|P5817=Q56556915|P576") {
				t.Errorf("srsearch = %q", q.Get("srsearch"))
			}
			off, _ := strconv.Atoi(q.Get("sroffset"))
			// Three hits; the second page is short.
			var hits []map[string]string
			for i := off; i < 3 && i < off+500; i++ {
				hits = append(hits, map[string]string{"title": fmt.Sprintf("Q%d", 10+i)})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"searchinfo": map[string]int{"totalhits": 3}, "search": hits}})
		case "wbgetentities":
			if q.Get("ids") != "Q10|Q11|Q12" {
				t.Errorf("ids = %q", q.Get("ids"))
			}
			fmt.Fprint(w, `{"entities": {
			  "Q10": {"id": "Q10", "labels": {"en": {"value": " Boston Opera House "}}, "claims": {
			    "P625": [{"mainsnak": {"snaktype": "value", "datavalue": {"value": {"latitude": 42.34, "longitude": -71.09}}}, "rank": "normal"}],
			    "P6375": [{"mainsnak": {"snaktype": "value", "datavalue": {"value": {"text": "343 Huntington Avenue", "language": "en"}}}, "rank": "normal"}],
			    "P571": [{"mainsnak": {"snaktype": "value", "datavalue": {"value": {"time": "+1909-11-08T00:00:00Z", "precision": 11}}}, "rank": "normal"}],
			    "P576": [{"mainsnak": {"snaktype": "value", "datavalue": {"value": {"time": "+1957-00-00T00:00:00Z", "precision": 9}}}, "rank": "normal"}],
			    "P3999": [{"mainsnak": {"snaktype": "somevalue"}, "rank": "normal"}]}},
			  "Q11": {"id": "Q11", "labels": {}, "claims": {
			    "P1619": [{"mainsnak": {"snaktype": "value", "datavalue": {"value": {"time": "+1930-05-00T00:00:00Z", "precision": 10}}}, "rank": "normal"}],
			    "P5817": [{"mainsnak": {"snaktype": "value", "datavalue": {"value": {"id": "Q56556915"}}}, "rank": "normal"}]}},
			  "Q12": {"id": "Q12", "missing": ""}}}`)
		}
	}))
	defer srv.Close()
	c := NewWikidataAPI(0)
	c.endpoint, c.gap = srv.URL, 0
	ctx := context.Background()

	if q, err := c.CountryQID(ctx, "GB"); err != nil || q != "Q145" {
		t.Fatalf("country = %q, %v", q, err)
	}
	got, note, err := c.ClosedCinemas(ctx, "Q145")
	if err != nil {
		t.Fatal(err)
	}
	if len(searches) != 1 || note != "" || len(got) != 2 {
		t.Fatalf("searches %v, note %q, got %+v", searches, note, got)
	}
	b := got[0]
	if b.QID != "Q10" || b.Name != "Boston Opera House" || !b.HasCoord || b.Lat != 42.34 || b.Address != "343 Huntington Avenue" ||
		b.Opened != "1909-11-08" || !b.Opened.Full() || b.Closed != "1957" || b.Closed.Year() != "1957" || b.Demolished {
		t.Fatalf("boston = %+v", b)
	}
	if o := got[1]; o.Name != "" || o.HasCoord || o.Opened != "1930-05" || !o.Demolished {
		t.Fatalf("second = %+v", o)
	}
	if _, err := c.CountryQID(ctx, "GBR"); err == nil {
		t.Fatal("bad code accepted")
	}
}
