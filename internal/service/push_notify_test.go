package service

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

type fakePushSender struct {
	sent     [][]client.PushMessage
	err      error
	tickets  func(msgs []client.PushMessage) []client.PushTicket
	receipts map[string]client.PushReceipt
	asked    []string
}

func (f *fakePushSender) Send(_ context.Context, msgs []client.PushMessage) ([]client.PushTicket, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.sent = append(f.sent, msgs)
	if f.tickets != nil {
		return f.tickets(msgs), nil
	}
	out := make([]client.PushTicket, len(msgs))
	for i := range msgs {
		out[i] = client.PushTicket{OK: true, ID: "ticket-" + msgs[i].To}
	}
	return out, nil
}

func (f *fakePushSender) Receipts(_ context.Context, ids []string) (map[string]client.PushReceipt, error) {
	f.asked = append(f.asked, ids...)
	return f.receipts, nil
}

type pushEnv struct {
	db     *sql.DB
	svc    *PushService
	sender *fakePushSender
	repo   *repository.PushRepository
}

func newPushEnv(t *testing.T, enabled bool) pushEnv {
	t.Helper()
	db := newTestDB(t)
	sender := &fakePushSender{}
	repo := repository.NewPushRepository(db)
	svc := NewPushService(slog.New(slog.NewTextHandler(io.Discard, nil)), repo, repository.NewWebhookRepository(db),
		repository.NewOpsRepository(db), sender, enabled)
	return pushEnv{db: db, svc: svc, sender: sender, repo: repo}
}

func (e pushEnv) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := e.db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// user adds a user with a session, preferences and a registered device.
func (e pushEnv) user(t *testing.T, id int64, prefs, token string) {
	t.Helper()
	e.exec(t, `INSERT INTO users (id, username, password_hash) VALUES (?, ?, 'x')`, id, "u"+token)
	e.exec(t, `INSERT INTO sessions (id, user_id, token_hash, expires_at) VALUES (?, ?, ?, datetime('now', '+1 day'))`, id, id, "h"+token)
	e.exec(t, `INSERT INTO user_preferences (user_id, data) VALUES (?, ?)`, id, prefs)
	sid := id
	if _, err := e.svc.RegisterDevice(context.Background(), id, &sid, DeviceInput{Platform: "ios", Token: "ExponentPushToken[" + token + "]"}); err != nil {
		t.Fatalf("register device: %v", err)
	}
}

func (e pushEnv) event(t *testing.T, kind, id, country, monetization string, provider int64, name, event string) {
	t.Helper()
	e.exec(t, `INSERT INTO watch_provider_events (tmdb_kind, tmdb_id, country, monetization, provider_id, provider_name, event)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, kind, id, country, monetization, provider, name, event)
}

func (e pushEnv) run(t *testing.T) *PushRunSummary {
	t.Helper()
	sum, err := e.svc.Run(context.Background(), func(string, string) {})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return sum
}

func TestPushDisabledDoesNothing(t *testing.T) {
	e := newPushEnv(t, false)
	if sum := e.run(t); sum.Skipped == "" {
		t.Fatalf("expected a skipped run, got %+v", sum)
	}
}

func TestRegisterDeviceValidates(t *testing.T) {
	e := newPushEnv(t, true)
	e.exec(t, `INSERT INTO users (id, username, password_hash) VALUES (1, 'a', 'x')`)
	for _, in := range []DeviceInput{
		{Platform: "web", Token: "ExponentPushToken[abcdefghij]"},
		{Platform: "ios", Token: "not-a-token"},
		{Platform: "android", Token: "ExponentPushToken[abcdefghij]", AppVersion: strings.Repeat("9", 40)},
	} {
		var v *ValidationError
		if _, err := e.svc.RegisterDevice(context.Background(), 1, nil, in); !errors.As(err, &v) {
			t.Errorf("%+v: expected a validation error, got %v", in, err)
		}
	}
	d, err := e.svc.RegisterDevice(context.Background(), 1, nil, DeviceInput{Platform: "Android", Token: "ExpoPushToken[abcdefghij]", AppVersion: "0.1.0"})
	if err != nil || d.Platform != "android" || d.AppVersion != "0.1.0" {
		t.Fatalf("register: %+v, %v", d, err)
	}
	// The same token again updates the same device.
	d2, err := e.svc.RegisterDevice(context.Background(), 1, nil, DeviceInput{Platform: "android", Token: "ExpoPushToken[abcdefghij]", AppVersion: "0.2.0"})
	if err != nil || d2.ID != d.ID || d2.AppVersion != "0.2.0" {
		t.Fatalf("re-register: %+v, %v", d2, err)
	}
	if err := e.svc.DeleteDevice(context.Background(), 2, d.ID); !IsDeviceNotFound(err) {
		t.Fatalf("another user's device: %v", err)
	}
	if err := e.svc.DeleteDevice(context.Background(), 1, d.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestDeviceGoesWithItsSession(t *testing.T) {
	e := newPushEnv(t, true)
	e.user(t, 1, `{}`, "aaaaaaaaaa")
	e.exec(t, `DELETE FROM sessions WHERE id = 1`)
	devices, err := e.svc.Devices(context.Background(), 1)
	if err != nil || len(devices) != 0 {
		t.Fatalf("devices after sign-out: %+v, %v", devices, err)
	}
}

func TestWatchlistArrivals(t *testing.T) {
	e := newPushEnv(t, true)
	ctx := context.Background()
	// Alice: opted in, US, subscribes to Netflix (8).
	e.user(t, 1, `{"country":"US","services":[8],"notify_watchlist_arrivals":true}`, "alice00000")
	// Bob: opted in, US, no services listed (any service counts).
	e.user(t, 2, `{"country":"us","notify_watchlist_arrivals":true}`, "bob0000000")
	// Carol: not opted in.
	e.user(t, 3, `{"country":"US"}`, "carol00000")
	// Dan: opted in, but in India.
	e.user(t, 4, `{"country":"IN","notify_watchlist_arrivals":true}`, "dan0000000")
	for _, u := range []int64{1, 2, 3, 4} {
		e.exec(t, `INSERT INTO user_titles (user_id, tmdb_kind, tmdb_id, status) VALUES (?, 'movie', '603', 'watchlist')`, u)
	}
	e.exec(t, `INSERT INTO tmdb_titles (tmdb_kind, tmdb_id, title) VALUES ('movie', '603', 'The Matrix')`)

	// History before the first run is never sent.
	e.event(t, "movie", "603", "US", "flatrate", 8, "Netflix", "added")
	if sum := e.run(t); len(e.sender.sent) != 0 || sum.Cursor == 0 {
		t.Fatalf("first run should only set the cursor: %+v, sent %v", sum, e.sender.sent)
	}

	// Rentals, removals and other countries' services don't count; Hulu (15)
	// in the US reaches Bob (any service) but not Alice (Netflix only).
	e.event(t, "movie", "603", "US", "rent", 2, "Apple TV", "added")
	e.event(t, "movie", "603", "US", "flatrate", 8, "Netflix", "removed")
	e.event(t, "movie", "603", "US", "flatrate", 15, "Hulu", "added")
	sum := e.run(t)
	if len(e.sender.sent) != 1 || len(e.sender.sent[0]) != 1 {
		t.Fatalf("expected one message, got %+v", e.sender.sent)
	}
	m := e.sender.sent[0][0]
	if m.To != "ExponentPushToken[bob0000000]" || m.Title != "Now on Hulu" || !strings.Contains(m.Body, "The Matrix") ||
		m.Data["url"] != "/title/movie/603" || m.ChannelID != "arrivals" {
		t.Fatalf("unexpected message %+v", m)
	}
	if sum.Notifications != 1 || sum.Events != 3 {
		t.Fatalf("summary %+v", sum)
	}

	// Netflix in the US now reaches Alice; Bob was already told.
	e.event(t, "movie", "603", "US", "flatrate", 8, "Netflix", "added")
	e.run(t)
	if len(e.sender.sent) != 2 || e.sender.sent[1][0].To != "ExponentPushToken[alice00000]" {
		t.Fatalf("expected Alice only, got %+v", e.sender.sent)
	}

	// A later arrival of the same title is not sent again.
	e.event(t, "movie", "603", "US", "free", 300, "Pluto TV", "added")
	e.run(t)
	if len(e.sender.sent) != 2 {
		t.Fatalf("no resend expected, got %+v", e.sender.sent)
	}

	// Tickets get their receipts on a later run; a gone device is disabled.
	e.exec(t, `UPDATE push_tickets SET created_at = datetime('now', '-20 minutes')`)
	devs, _ := e.repo.ActiveTokens(ctx, 1)
	e.sender.receipts = map[string]client.PushReceipt{
		"ticket-ExponentPushToken[alice00000]": {OK: false, Error: "DeviceNotRegistered"},
		"ticket-ExponentPushToken[bob0000000]": {OK: true},
	}
	sum = e.run(t)
	if sum.ReceiptsChecked != 2 || sum.DevicesDisabled != 1 {
		t.Fatalf("receipts summary %+v", sum)
	}
	if after, _ := e.repo.ActiveTokens(ctx, 1); len(devs) != 1 || len(after) != 0 {
		t.Fatalf("alice's device should be disabled: before %v after %v", devs, after)
	}
}

func TestArrivalRetriedAfterSendFailure(t *testing.T) {
	e := newPushEnv(t, true)
	e.user(t, 1, `{"country":"GB","notify_watchlist_arrivals":true}`, "erin000000")
	e.exec(t, `INSERT INTO user_titles (user_id, tmdb_kind, tmdb_id, status) VALUES (1, 'tv', '1399', 'watchlist')`)
	e.run(t) // sets the cursor
	e.event(t, "tv", "1399", "GB", "flatrate", 9, "Prime Video", "added")

	e.sender.err = errors.New("network down")
	if _, err := e.svc.Run(context.Background(), func(string, string) {}); err == nil {
		t.Fatal("expected the send error")
	}
	e.sender.err = nil
	e.run(t)
	if len(e.sender.sent) != 1 || e.sender.sent[0][0].Data["url"] != "/title/tv/1399" {
		t.Fatalf("expected the retry to send, got %+v", e.sender.sent)
	}
}

func TestDeviceNotRegisteredTicketDisablesDevice(t *testing.T) {
	e := newPushEnv(t, true)
	e.user(t, 1, `{"country":"US","notify_watchlist_arrivals":true}`, "gone000000")
	e.exec(t, `INSERT INTO user_titles (user_id, tmdb_kind, tmdb_id, status) VALUES (1, 'movie', '1', 'watchlist')`)
	e.run(t)
	e.event(t, "movie", "1", "US", "ads", 5, "Tubi", "added")
	e.sender.tickets = func(msgs []client.PushMessage) []client.PushTicket {
		return []client.PushTicket{{Error: "DeviceNotRegistered"}}
	}
	sum := e.run(t)
	if sum.DevicesDisabled != 1 || sum.Failed != 1 {
		t.Fatalf("summary %+v", sum)
	}
}

// B10: watch providers are refreshed for watchlisted titles that never
// charted, so their arrivals produce events at all.
func TestWatchlistTitlesAreDueForProviderRefresh(t *testing.T) {
	e := newPushEnv(t, true)
	e.exec(t, `INSERT INTO users (id, username, password_hash) VALUES (1, 'a', 'x')`)
	e.exec(t, `INSERT INTO user_titles (user_id, tmdb_kind, tmdb_id, status) VALUES (1, 'movie', '424242', 'watchlist')`)
	e.exec(t, `INSERT INTO user_titles (user_id, tmdb_kind, tmdb_id, status) VALUES (1, 'movie', '515151', 'completed')`)
	refs, err := repository.NewMetadataRepository(e.db).RefsDueForDetails(context.Background(), "-3 days", "-30 days", 100)
	if err != nil {
		t.Fatal(err)
	}
	var watchlisted, completed bool
	for _, r := range refs {
		watchlisted = watchlisted || (r.Kind == "movie" && r.ID == "424242")
		completed = completed || (r.Kind == "movie" && r.ID == "515151")
	}
	if !watchlisted || completed {
		t.Fatalf("refs %+v: want the watchlisted title only", refs)
	}
}
