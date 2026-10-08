package flixpatrol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/config"
)

func newFakeFlareSolverr(t *testing.T, handle func(req map[string]any) (int, map[string]any)) Fetcher {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		status, body := handle(req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)

	fs := client.NewFlareSolverr(config.FlareSolverr{URL: srv.URL, MaxTimeout: 5 * time.Second, Session: "test", SessionTTL: 30 * time.Minute})
	return FlareSolverrFetcher{Client: fs}
}

func TestFlareSolverrFetcher_ScrapesTitlePage(t *testing.T) {
	html, err := os.ReadFile("testdata/title_bugonia.html")
	if err != nil {
		t.Fatal(err)
	}
	fetcher := newFakeFlareSolverr(t, func(req map[string]any) (int, map[string]any) {
		if req["cmd"] != "request.get" || req["url"] != "https://flixpatrol.com/title/bugonia/" || req["session"] != "test" {
			t.Errorf("unexpected request %v", req)
		}
		return http.StatusOK, map[string]any{
			"status": "ok",
			"solution": map[string]any{
				"url":      req["url"],
				"status":   200,
				"headers":  map[string]string{},
				"response": string(html),
			},
		}
	})

	got, err := ScrapeTitleDetails(context.Background(), fetcher, "bugonia", "", false)
	if err != nil {
		t.Fatalf("ScrapeTitleDetails error: %v", err)
	}
	if got.Name != "Bugonia" || got.Year != 2025 || got.TitleKind != TitleKindMovie {
		t.Fatalf("unexpected details %+v", got)
	}
}

func TestFlareSolverrFetcher_UnsolvedChallenge(t *testing.T) {
	fetcher := newFakeFlareSolverr(t, func(req map[string]any) (int, map[string]any) {
		return http.StatusOK, map[string]any{
			"status": "ok",
			"solution": map[string]any{
				"status":   200,
				"response": "<html><head><title>Just a moment...</title></head><body></body></html>",
			},
		}
	})

	_, err := ScrapeTitleDetails(context.Background(), fetcher, "bugonia", "", false)
	if err == nil || !strings.Contains(err.Error(), "unsolved cloudflare challenge") {
		t.Fatalf("expected unsolved challenge error, got %v", err)
	}
}

func TestFlareSolverrFetcher_Error(t *testing.T) {
	fetcher := newFakeFlareSolverr(t, func(req map[string]any) (int, map[string]any) {
		return http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"message": "Error: Error solving the challenge. Timeout after 5.0 seconds.",
		}
	})

	_, err := ScrapeTitleDetails(context.Background(), fetcher, "bugonia", "", false)
	if err == nil || !strings.Contains(err.Error(), "Timeout after 5.0 seconds") {
		t.Fatalf("expected flaresolverr error, got %v", err)
	}
}
