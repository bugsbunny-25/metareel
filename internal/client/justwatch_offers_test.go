package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	graphql "github.com/hasura/go-graphql-client"
)

func TestJustWatchGetOffers(t *testing.T) {
	var gotQuery string
	var gotVars map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.Unmarshal(body, &req)
		gotQuery, gotVars = req.Query, req.Variables
		if req.Variables["id"] == "ts1" {
			_, _ = io.WriteString(w, `{"data":{"node":null}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"node":{"id":"ts274631",
		  "US":[{"monetizationType":"FLATRATE","standardWebURL":"https://www.netflix.com/title/81231974","package":{"packageId":8}},
		        {"monetizationType":"BUY","standardWebURL":"javascript:alert(1)","package":{"packageId":3}},
		        {"monetizationType":"RENT","standardWebURL":null,"package":{"packageId":2}}],
		  "IN":[]}}}`)
	}))
	defer srv.Close()
	c := &JustWatchClient{client: graphql.NewClient(srv.URL, srv.Client())}

	offers, found, err := c.GetOffers(context.Background(), "ts274631", []string{"US", "in", "US", "X1", "us){x}"})
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if gotVars["id"] != "ts274631" {
		t.Fatalf("vars = %v", gotVars)
	}
	if !strings.Contains(gotQuery, "US: offers(country: US, platform: WEB)") || !strings.Contains(gotQuery, "IN: offers(country: IN") ||
		strings.Count(gotQuery, "offers(") != 2 {
		t.Fatalf("query = %s", gotQuery)
	}
	if len(offers["US"]) != 1 || offers["US"][0] != (JWOffer{Monetization: "flatrate", PackageID: 8, URL: "https://www.netflix.com/title/81231974"}) {
		t.Fatalf("US offers = %+v", offers["US"])
	}
	if len(offers["IN"]) != 0 {
		t.Fatalf("IN offers = %+v", offers["IN"])
	}

	if _, found, err := c.GetOffers(context.Background(), "ts1", []string{"US"}); err != nil || found {
		t.Fatalf("missing node: found=%v err=%v", found, err)
	}
	if _, _, err := c.GetOffers(context.Background(), `ts1") { x }`, []string{"US"}); err == nil {
		t.Fatal("expected an error for a malformed node id")
	}
}
