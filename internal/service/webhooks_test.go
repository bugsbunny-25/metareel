package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// receiver records webhook requests and answers with status.
type receiver struct {
	mu     sync.Mutex
	status int
	got    []*http.Request
	bodies []string
}

func (rc *receiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.got = append(rc.got, r)
	rc.bodies = append(rc.bodies, string(b))
	if rc.status == 0 {
		rc.status = http.StatusNoContent
	}
	w.WriteHeader(rc.status)
	if rc.status >= 300 {
		_, _ = io.WriteString(w, "nope")
	}
}

func (rc *receiver) events() []string {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	var out []string
	for _, r := range rc.got {
		out = append(out, r.Header.Get("X-Metareel-Event"))
	}
	return out
}

func newWebhookTest(t *testing.T) (*WebhookService, *repository.WebhookRepository, func(chart string, ranks ...string)) {
	t.Helper()
	db := newTestDB(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := repository.NewWebhookRepository(db)
	top := NewTop10ReadService(repository.NewFlixPatrolRepository(db)).WithMetadata(repository.NewMetadataRepository(db))
	svc := NewWebhookService(log, repo, repository.NewOpsRepository(db), top, WebhookOptions{AllowLoopback: true})
	// chart stores a US Netflix movies chart on a date with titles in rank order.
	chart := func(date string, slugs ...string) {
		var titles []seedTitle
		var ranks []seedRanking
		for i, s := range slugs {
			titles = append(titles, seedTitle{Slug: s, Name: strings.ToUpper(s), Kind: "movie"})
			ranks = append(ranks, seedRanking{s, date, "US", "netflix", "movies", int64(i + 1)})
		}
		seed(t, db, titles, ranks)
	}
	return svc, repo, chart
}

func TestWebhookValidation(t *testing.T) {
	svc, _, _ := newWebhookTest(t)
	ctx := context.Background()
	var v *ValidationError
	for _, in := range []WebhookInput{
		{Name: "", URL: "https://x.test/h", Events: []string{"*"}},
		{Name: "a", URL: "ftp://x.test/h", Events: []string{"*"}},
		{Name: "a", URL: "https:///nohost", Events: []string{"*"}},
		{Name: "a", URL: "https://x.test/h", Events: nil},
		{Name: "a", URL: "https://x.test/h", Events: []string{"chart.deleted"}},
	} {
		if _, err := svc.Create(ctx, in); !errors.As(err, &v) {
			t.Errorf("%+v: err = %v, want ValidationError", in, err)
		}
	}
	w, err := svc.Create(ctx, WebhookInput{Name: " tides ", URL: "https://x.test/h", Events: []string{EventChartUpdated, EventChartUpdated, EventTitleEntered}})
	if err != nil {
		t.Fatal(err)
	}
	if w.Name != "tides" || !strings.HasPrefix(w.Secret, "whsec_") || len(w.Events) != 2 || !w.Enabled {
		t.Fatalf("created = %+v", w)
	}
	// The secret is never serialised with the webhook itself.
	b, _ := json.Marshal(w.Webhook)
	if strings.Contains(string(b), w.Secret) {
		t.Fatalf("webhook JSON leaks the secret: %s", b)
	}
	if _, err := svc.Update(ctx, w.ID+100, WebhookPatch{}); !errors.Is(err, ErrWebhookNotFound) {
		t.Fatalf("update missing: %v", err)
	}
	off := false
	if u, err := svc.Update(ctx, w.ID, WebhookPatch{Enabled: &off, Events: &[]string{"*", EventChartUpdated}}); err != nil || u.Enabled || len(u.Events) != 1 || u.Events[0] != "*" {
		t.Fatalf("update = %+v, %v", u, err)
	}
	rot, err := svc.RotateSecret(ctx, w.ID)
	if err != nil || rot.Secret == w.Secret {
		t.Fatalf("rotate = %+v, %v", rot, err)
	}
	if err := svc.Delete(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, w.ID); !errors.Is(err, ErrWebhookNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}

func TestWebhookChartEventsDelivered(t *testing.T) {
	svc, repo, chart := newWebhookTest(t)
	ctx := context.Background()
	rc := &receiver{}
	srv := httptest.NewServer(rc)
	defer srv.Close()

	// Charts stored before the dispatcher first runs are not replayed.
	chart("2026-11-01", "a", "b")
	all, err := svc.Create(ctx, WebhookInput{Name: "all", URL: srv.URL + "/all", Events: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, WebhookInput{Name: "no1", URL: srv.URL + "/no1", Events: []string{EventTitleNumberOne}}); err != nil {
		t.Fatal(err)
	}
	svc.Tick(ctx)
	if n := len(rc.events()); n != 0 {
		t.Fatalf("backlog delivered: %v", rc.events())
	}

	// Next day: c enters at #1, a drops to #2, b leaves.
	chart("2026-11-02", "c", "a")
	svc.Tick(ctx)
	got := rc.events()
	count := map[string]int{}
	for _, e := range got {
		count[e]++
	}
	// all: chart.updated + title.entered (c) + title.number_one (c); no1: title.number_one.
	if count[EventChartUpdated] != 1 || count[EventTitleEntered] != 1 || count[EventTitleNumberOne] != 2 || len(got) != 4 {
		t.Fatalf("events = %v", got)
	}

	// Signatures verify with the webhook's secret; bodies carry the entries.
	full, _ := repo.Get(ctx, all.ID)
	rc.mu.Lock()
	for i, r := range rc.got {
		if r.URL.Path != "/all" {
			continue
		}
		sig := r.Header.Get("X-Metareel-Signature")
		ts, _ := strconv.ParseInt(strings.TrimPrefix(strings.Split(sig, ",")[0], "t="), 10, 64)
		if sig != WebhookSignature(full.Secret, ts, rc.bodies[i]) || r.Header.Get("X-Metareel-Delivery") == "" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("bad headers %v", r.Header)
		}
		var env struct {
			ID, Type string
			Data     json.RawMessage
		}
		if err := json.Unmarshal([]byte(rc.bodies[i]), &env); err != nil || env.ID == "" {
			t.Errorf("body %s: %v", rc.bodies[i], err)
		}
		if env.Type == EventChartUpdated {
			var d ChartUpdatedData
			_ = json.Unmarshal(env.Data, &d)
			if d.Country != "US" || d.Provider != "netflix" || d.Date != "2026-11-02" || len(d.Entries) != 2 || d.Entries[0].Name != "C" {
				t.Errorf("chart.updated data = %+v", d)
			}
		}
		if env.Type == EventTitleEntered {
			var d TitleChartData
			_ = json.Unmarshal(env.Data, &d)
			if d.Entry.Name != "C" || !d.FirstTime || d.Chart.Category != "movies" {
				t.Errorf("title.entered data = %+v", d)
			}
		}
	}
	rc.mu.Unlock()

	// Ticking again delivers nothing new (events are queued once).
	svc.Tick(ctx)
	if n := len(rc.events()); n != 4 {
		t.Fatalf("re-delivered: %v", rc.events())
	}
	ds, err := svc.Deliveries(ctx, all.ID, 10)
	if err != nil || len(ds) != 3 || ds[0].Status != "delivered" || ds[0].Attempts != 1 {
		t.Fatalf("deliveries = %+v, %v", ds, err)
	}
	if p, err := svc.DeliveryPayload(ctx, ds[0].ID); err != nil || !strings.Contains(string(p), `"type"`) {
		t.Fatalf("payload = %s, %v", p, err)
	}
}

func TestWebhookRetriesAndFailures(t *testing.T) {
	svc, repo, chart := newWebhookTest(t)
	ctx := context.Background()
	rc := &receiver{status: http.StatusInternalServerError}
	srv := httptest.NewServer(rc)
	defer srv.Close()
	w, err := svc.Create(ctx, WebhookInput{Name: "t", URL: srv.URL + "/x?token=secret-in-url", Events: []string{EventChartUpdated}})
	if err != nil {
		t.Fatal(err)
	}
	svc.Tick(ctx) // starts the cursors
	chart("2026-11-03", "a")
	svc.Tick(ctx)
	ds, _ := svc.Deliveries(ctx, w.ID, 10)
	if len(ds) != 1 || ds[0].Status != "pending" || ds[0].Attempts != 1 || ds[0].LastStatusCode == nil || *ds[0].LastStatusCode != 500 ||
		ds[0].LastError == nil || !strings.Contains(*ds[0].LastError, "nope") || ds[0].NextAttemptAt == "" {
		t.Fatalf("after a failure = %+v", ds)
	}
	// Not due again until the backoff passes.
	svc.Tick(ctx)
	if n := len(rc.events()); n != 1 {
		t.Fatalf("retried before the backoff: %d requests", n)
	}
	hook, _ := repo.Get(ctx, w.ID)
	if hook.LastStatus == nil || *hook.LastStatus != "failed" {
		t.Fatalf("webhook status = %+v", hook)
	}

	// The last allowed attempt marks it failed for good; retry requeues it.
	d := repository.DueDelivery{ID: ds[0].ID, WebhookID: w.ID}
	for i := 0; i < webhookMaxAttempts; i++ {
		if err := repo.Failed(ctx, d, 500, "x", "+0 seconds", i == webhookMaxAttempts-1); err != nil {
			t.Fatal(err)
		}
	}
	ds, _ = svc.Deliveries(ctx, w.ID, 10)
	if ds[0].Status != "failed" {
		t.Fatalf("after max attempts = %+v", ds[0])
	}
	rc.mu.Lock()
	rc.status = http.StatusOK
	rc.mu.Unlock()
	if err := svc.Retry(ctx, ds[0].ID); err != nil {
		t.Fatal(err)
	}
	svc.Tick(ctx)
	ds, _ = svc.Deliveries(ctx, w.ID, 10)
	if ds[0].Status != "delivered" {
		t.Fatalf("after retry = %+v", ds[0])
	}
	var v *ValidationError
	if err := svc.Retry(ctx, ds[0].ID+99); !errors.As(err, &v) {
		t.Fatalf("retry unknown: %v", err)
	}
}

func TestWebhookRedirectAndErrorsHideURL(t *testing.T) {
	svc, _, _ := newWebhookTest(t)
	ctx := context.Background()
	target := httptest.NewServer(&receiver{})
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()

	w, _ := svc.Create(ctx, WebhookInput{Name: "r", URL: redirect.URL, Events: []string{"*"}})
	res, err := svc.Test(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusFound || res.Delivery.Status != "failed" {
		t.Fatalf("redirect = %+v", res)
	}

	// A connection error names neither the URL nor its token.
	dead, _ := svc.Create(ctx, WebhookInput{Name: "d", URL: "http://127.0.0.1:1/hook?token=abc123", Events: []string{"*"}})
	res, err = svc.Test(ctx, dead.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Delivery.Status != "failed" || strings.Contains(res.Error, "abc123") || strings.Contains(res.Error, "127.0.0.1:1/hook") {
		t.Fatalf("dead = %+v", res)
	}

	ok, _ := svc.Create(ctx, WebhookInput{Name: "ok", URL: target.URL, Events: []string{"*"}})
	if res, err = svc.Test(ctx, ok.ID); err != nil || res.Delivery.Status != "delivered" || res.StatusCode != http.StatusNoContent {
		t.Fatalf("ping = %+v, %v", res, err)
	}
	if _, err := svc.Test(ctx, 9999); !errors.Is(err, ErrWebhookNotFound) {
		t.Fatalf("test missing: %v", err)
	}
}

func TestWebhookAvailabilityEvents(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	meta := repository.NewMetadataRepository(db)
	svc := NewWebhookService(log, repository.NewWebhookRepository(db), repository.NewOpsRepository(db),
		NewTop10ReadService(repository.NewFlixPatrolRepository(db)), WebhookOptions{AllowLoopback: true})
	rc := &receiver{}
	srv := httptest.NewServer(rc)
	defer srv.Close()
	if _, err := svc.Create(ctx, WebhookInput{Name: "a", URL: srv.URL, Events: []string{EventAvailabilityAdded, EventAvailabilityRemoved}}); err != nil {
		t.Fatal(err)
	}
	save := func(offers ...client.TMDBWatchProvider) {
		if err := meta.SaveDetails(ctx, &client.TMDBFullDetails{Kind: "tv", ID: "7", Title: "Show", WatchProviders: offers}, false); err != nil {
			t.Fatal(err)
		}
	}
	save(offer("US", "flatrate", 8, "Netflix"))
	svc.Tick(ctx)
	save(offer("US", "flatrate", 9, "Prime Video"))
	svc.Tick(ctx)
	got := rc.events()
	if len(got) != 2 || !(got[0] == EventAvailabilityAdded || got[0] == EventAvailabilityRemoved) {
		t.Fatalf("availability events = %v", got)
	}
	rc.mu.Lock()
	body := rc.bodies[0] + rc.bodies[1]
	rc.mu.Unlock()
	if !strings.Contains(body, `"provider_name":"Prime Video"`) || !strings.Contains(body, `"title":"Show"`) {
		t.Fatalf("bodies = %s", body)
	}
}

// TestWebhookAddressGuard: loopback only when allowed; link-local (cloud
// metadata) and unspecified never; LAN addresses always.
func TestWebhookAddressGuard(t *testing.T) {
	deny, allow := webhookDialControl(false), webhookDialControl(true)
	for addr, want := range map[string][2]bool{ // [deny-loopback ok, allow-loopback ok]
		"127.0.0.1:8080":     {false, true},
		"[::1]:80":           {false, true},
		"169.254.169.254:80": {false, false},
		"[fe80::1]:80":       {false, false},
		"0.0.0.0:80":         {false, false},
		"224.0.0.1:80":       {false, false},
		"255.255.255.255:80": {false, false},
		"192.168.1.20:5080":  {true, true},
		"10.0.0.5:443":       {true, true},
		"172.18.0.3:8191":    {true, true},
		"93.184.216.34:443":  {true, true},
		"not-an-address":     {false, false},
	} {
		if got := deny("tcp", addr, nil) == nil; got != want[0] {
			t.Errorf("deny-loopback %s: allowed = %v, want %v", addr, got, want[0])
		}
		if got := allow("tcp", addr, nil) == nil; got != want[1] {
			t.Errorf("allow-loopback %s: allowed = %v, want %v", addr, got, want[1])
		}
	}

	// End to end: a loopback receiver is refused without the option, and the
	// error says why without naming the URL.
	db := newTestDB(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewWebhookService(log, repository.NewWebhookRepository(db), repository.NewOpsRepository(db),
		NewTop10ReadService(repository.NewFlixPatrolRepository(db)), WebhookOptions{})
	srv := httptest.NewServer(&receiver{})
	defer srv.Close()
	ctx := context.Background()
	w, err := svc.Create(ctx, WebhookInput{Name: "local", URL: srv.URL + "/h?token=t0ken", Events: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Test(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Delivery.Status != "failed" || !strings.Contains(res.Error, "not allowed for webhooks") || strings.Contains(res.Error, "t0ken") {
		t.Fatalf("loopback = %+v", res)
	}
	var v *ValidationError
	if _, err := svc.Create(ctx, WebhookInput{Name: "x", URL: "http://:8191/", Events: []string{"*"}}); !errors.As(err, &v) {
		t.Fatalf("empty hostname accepted: %v", err)
	}
}

// TestWebhookUnreachableSkipped: after a receiver can't be reached, its other
// due deliveries wait for the next pass.
func TestWebhookUnreachableSkipped(t *testing.T) {
	svc, repo, chart := newWebhookTest(t)
	ctx := context.Background()
	w, _ := svc.Create(ctx, WebhookInput{Name: "down", URL: "http://127.0.0.1:1/h", Events: []string{"*"}})
	svc.Tick(ctx)
	chart("2026-11-04", "a", "b") // chart.updated + 2 entered + number_one = 4 events
	svc.Tick(ctx)
	ds, _ := svc.Deliveries(ctx, w.ID, 10)
	tried := 0
	for _, d := range ds {
		if d.Attempts > 0 {
			tried++
		}
	}
	if len(ds) != 4 || tried != 1 {
		t.Fatalf("deliveries = %d, attempted = %d (want 4, 1)", len(ds), tried)
	}
	_ = repo
	_ = time.Second
}

// More charts than a page sharing one changed_at (a backfill or bulk import)
// must not stall the chart cursor: the real chart stored after them is
// still delivered, within a few passes.
func TestWebhookChartCursorPagesLargeBatches(t *testing.T) {
	db := newTestDB(t)
	top := NewTop10ReadService(repository.NewFlixPatrolRepository(db)).WithMetadata(repository.NewMetadataRepository(db))
	svc := NewWebhookService(slog.New(slog.NewTextHandler(io.Discard, nil)), repository.NewWebhookRepository(db),
		repository.NewOpsRepository(db), top, WebhookOptions{AllowLoopback: true})
	chart := func(date string, slugs ...string) {
		var titles []seedTitle
		var ranks []seedRanking
		for i, s := range slugs {
			titles = append(titles, seedTitle{Slug: s, Name: strings.ToUpper(s), Kind: "movie"})
			ranks = append(ranks, seedRanking{s, date, "US", "netflix", "movies", int64(i + 1)})
		}
		seed(t, db, titles, ranks)
	}
	ctx := context.Background()
	rc := &receiver{}
	srv := httptest.NewServer(rc)
	defer srv.Close()
	if _, err := svc.Create(ctx, WebhookInput{Name: "all", URL: srv.URL, Events: []string{EventChartUpdated}}); err != nil {
		t.Fatal(err)
	}
	chart("2026-11-01", "a")
	svc.Tick(ctx) // starts following

	// 1,200 empty charts with the same changed_at (one with a country the
	// read API rejects), then a real chart.
	stamp := time.Now().UTC().Format(sqliteTimeLayout)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1200; i++ {
		d := day("2020-01-01").AddDate(0, 0, i).Format(repository.DateLayout)
		country := "US"
		if i == 700 {
			country = "ZZ"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO chart_snapshots (ranked_on, country, streaming_provider, category, entry_count, signature, changed_at)
			VALUES (?, ?, 'netflix', 'movies', 0, '', ?)`, d, country, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	chart("2026-11-02", "b", "a")
	if _, err := db.ExecContext(ctx, `UPDATE chart_snapshots SET changed_at = ? WHERE ranked_on = '2026-11-02'`, stamp); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 4 && len(rc.events()) == 0; i++ {
		svc.Tick(ctx)
	}
	if got := rc.events(); len(got) != 1 || got[0] != EventChartUpdated {
		t.Fatalf("events = %v (cursor stalled?)", got)
	}
	// Later passes skip what was handled and deliver nothing again.
	svc.Tick(ctx)
	svc.Tick(ctx)
	if got := rc.events(); len(got) != 1 {
		t.Fatalf("re-delivered: %v", got)
	}
	cursor, _ := repository.NewOpsRepository(db).Setting(ctx, cursorCharts)
	if cursor != stamp {
		t.Fatalf("cursor = %q, want %q", cursor, stamp)
	}
}
