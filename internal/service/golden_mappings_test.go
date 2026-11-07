package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/config"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// Golden mappings: chart titles that are known to be hard to map (each was
// fixed or confirmed by hand in the admin UI), run through the matchers
// against recorded JustWatch and TMDB responses. A case either maps to its
// right TMDB title (want) or is a known miss whose current result is pinned,
// so any change in what the matchers do, better or worse, fails here.
//
// Record fresh responses (live requests; needs TMDB_API_KEY) with:
//
//	GOLDEN_RECORD=1 TMDB_API_KEY=... go test -run TestGoldenMappings ./internal/service/
//
// then check the printed results against the cases and update known misses.

const goldenDir = "testdata/golden_mappings"

type goldenRef struct {
	Kind   string `json:"kind"` // movie | tv
	TmdbID string `json:"tmdb_id"`
}

func (r *goldenRef) String() string {
	if r == nil {
		return "no match"
	}
	return r.Kind + ":" + r.TmdbID
}

type goldenMiss struct {
	Got *goldenRef `json:"got"` // the current (wrong or missing) result
	Why string     `json:"why"`
}

type goldenFlixPatrol struct {
	Slug      string `json:"slug"`
	ChartName string `json:"chart_name"`
	ChartKind string `json:"chart_kind"` // movie | tv_show
	Details   *struct {
		Name     string `json:"name"`
		Kind     string `json:"kind"`
		Premiere string `json:"premiere"` // YYYY-MM-DD
	} `json:"details,omitempty"`
	Provider  string      `json:"provider"`
	Country   string      `json:"country"`
	Note      string      `json:"note"`
	Want      *goldenRef  `json:"want"`
	KnownMiss *goldenMiss `json:"known_miss,omitempty"`
}

type goldenNetflix struct {
	Name      string      `json:"name"`
	List      string      `json:"list"` // movie | tv_show
	ChartYear int         `json:"chart_year"`
	Note      string      `json:"note"`
	Want      *goldenRef  `json:"want"`
	KnownMiss *goldenMiss `json:"known_miss,omitempty"`
}

type goldenCases struct {
	FlixPatrol []goldenFlixPatrol `json:"flixpatrol"`
	Netflix    []goldenNetflix    `json:"netflix"`
}

// recordedResponse is one upstream answer.
type recordedResponse struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body,omitempty"`
	Text   string          `json:"text,omitempty"` // a non-JSON body
}

// replayTransport answers requests from recorded responses, or (recording)
// forwards them and records the answers. Keys drop the api_key query
// parameter, so keys never reach the fixture.
type replayTransport struct {
	mu        sync.Mutex
	record    bool
	next      http.RoundTripper
	responses map[string]recordedResponse
	missing   []string
}

func requestKey(r *http.Request, body []byte) string {
	u := *r.URL
	q := u.Query()
	q.Del("api_key")
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+strings.Join(q[k], ","))
	}
	key := r.Method + " " + u.Host + u.Path
	if len(parts) > 0 {
		key += "?" + strings.Join(parts, "&")
	}
	if len(body) > 0 {
		sum := sha256.Sum256(body)
		key += " body:" + hex.EncodeToString(sum[:8])
	}
	return key
}

func (t *replayTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	key := requestKey(r, body)
	t.mu.Lock()
	rec, ok := t.responses[key]
	t.mu.Unlock()
	if !ok && t.record {
		resp, err := t.next.RoundTrip(r)
		if err != nil {
			return nil, err
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		rec = recordedResponse{Status: resp.StatusCode}
		if json.Valid(b) {
			var compact bytes.Buffer
			_ = json.Compact(&compact, b)
			rec.Body = compact.Bytes()
		} else {
			rec.Text = string(b)
		}
		t.mu.Lock()
		t.responses[key] = rec
		t.mu.Unlock()
		ok = true
	}
	if !ok {
		t.mu.Lock()
		t.missing = append(t.missing, key)
		t.mu.Unlock()
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{"Content-Type": {"application/json"}}, Request: r}, nil
	}
	b := []byte(rec.Body)
	if rec.Text != "" {
		b = []byte(rec.Text)
	}
	return &http.Response{StatusCode: rec.Status, Body: io.NopCloser(bytes.NewReader(b)), Header: http.Header{"Content-Type": {"application/json"}},
		ContentLength: int64(len(b)), Request: r}, nil
}

func TestGoldenMappings(t *testing.T) {
	record := os.Getenv("GOLDEN_RECORD") == "1"
	raw, err := os.ReadFile(filepath.Join(goldenDir, "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases goldenCases
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("cases.json: %v", err)
	}
	rt := &replayTransport{record: record, next: http.DefaultTransport, responses: map[string]recordedResponse{}}
	httpPath := filepath.Join(goldenDir, "http.json")
	if !record {
		b, err := os.ReadFile(httpPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &rt.responses); err != nil {
			t.Fatalf("http.json: %v", err)
		}
	}
	apiKey := "test"
	if record {
		if apiKey = os.Getenv("TMDB_API_KEY"); apiKey == "" {
			t.Fatal("GOLDEN_RECORD needs TMDB_API_KEY")
		}
	}
	resty := client.New(config.HTTPClient{Timeout: 20 * time.Second, UserAgent: "metareel-golden-test"}).SetTransport(rt)
	tmdb := client.NewTMDB(resty, apiKey)
	jw := client.NewJustWatchClient(&http.Client{Transport: rt, Timeout: 20 * time.Second})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	matcher := NewTitleMatcher(log, nil, tmdb, jw, nil)
	matcher.jwDelay = 0
	if record {
		matcher.jwDelay = 300 * time.Millisecond // be polite to JustWatch
	}
	ctx := context.Background()

	check := func(name string, got, want *goldenRef, miss *goldenMiss) {
		t.Helper()
		if record {
			t.Logf("%-45s got %-15s want %s", name, got, want)
			return
		}
		same := func(a, b *goldenRef) bool { return a.String() == b.String() }
		switch {
		case same(got, want) && miss != nil:
			t.Errorf("%s: now maps right (%s); drop its known_miss", name, got)
		case same(got, want):
		case miss == nil:
			t.Errorf("%s: got %s, want %s", name, got, want)
		case !same(got, miss.Got):
			t.Errorf("%s: known miss changed: got %s, was %s (want %s)", name, got, miss.Got, want)
		}
	}

	for _, c := range cases.FlixPatrol {
		in := MatchInput{Slug: c.Slug, ChartName: c.ChartName, ChartKind: flixpatrol.TitleKind(c.ChartKind),
			Provider: flixpatrol.Provider(c.Provider), Country: c.Country}
		if c.Details != nil {
			d := &flixpatrol.TitleDetails{Name: c.Details.Name, TitleKind: flixpatrol.TitleKind(c.Details.Kind), Premiere: c.Details.Premiere}
			if len(c.Details.Premiere) >= 4 {
				_, _ = fmt.Sscanf(c.Details.Premiere[:4], "%d", &d.Year)
			}
			in.Details = d
		}
		m, err := matcher.Match(ctx, in)
		if err != nil {
			t.Fatalf("%s: %v", c.Slug, err)
		}
		var got *goldenRef
		if m.TmdbID != "" {
			got = &goldenRef{Kind: tmdbKindOf(m.Kind), TmdbID: m.TmdbID}
		}
		check("flixpatrol "+c.Slug, got, c.Want, c.KnownMiss)
	}

	nm := &netflixTMDBMatcher{tmdb: tmdb, log: log}
	for _, c := range cases.Netflix {
		cand, kind, ok := nm.match(ctx, c.Name, c.List, c.ChartYear)
		var got *goldenRef
		if ok {
			got = &goldenRef{Kind: kind, TmdbID: cand.TmdbID}
		}
		check(fmt.Sprintf("netflix %q (%s)", c.Name, c.List), got, c.Want, c.KnownMiss)
	}

	if record {
		// One response per line, sorted, so a re-recording diffs readably.
		keys := make([]string, 0, len(rt.responses))
		for k := range rt.responses {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var out bytes.Buffer
		out.WriteString("{\n")
		for i, k := range keys {
			kb, _ := json.Marshal(k)
			vb, err := json.Marshal(rt.responses[k])
			if err != nil {
				t.Fatal(err)
			}
			out.Write(kb)
			out.WriteString(": ")
			out.Write(vb)
			if i < len(keys)-1 {
				out.WriteString(",")
			}
			out.WriteString("\n")
		}
		out.WriteString("}\n")
		if bytes.Contains(out.Bytes(), []byte(apiKey)) {
			t.Fatal("the TMDB API key would be written to the fixture; not saving it")
		}
		if err := os.WriteFile(httpPath, out.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded %d responses to %s", len(rt.responses), httpPath)
		return
	}
	if len(rt.missing) > 0 {
		t.Errorf("requests without a recorded response (re-record with GOLDEN_RECORD=1): %v", rt.missing)
	}
}
