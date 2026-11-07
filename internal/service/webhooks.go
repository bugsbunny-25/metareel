package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
)

// Outbound webhooks: consumers (tides) subscribe a URL to event types and
// get a signed POST per event instead of polling /changes. The dispatcher
// (WebhookService.Run, a loop in the server process) follows the event
// sources with cursors in app_settings, queues one delivery per event and
// subscribed webhook (webhook_deliveries, deduplicated by event id) and
// delivers them with retries.

// Webhook event types.
const (
	EventChartUpdated        = "chart.updated"        // a chart was stored or its content changed
	EventTitleEntered        = "title.entered"        // a title on a chart that was not on its previous date
	EventTitleNumberOne      = "title.number_one"     // a new #1 on a chart
	EventAvailabilityAdded   = "availability.added"   // a title arrived on a service in a country
	EventAvailabilityRemoved = "availability.removed" // a title left a service in a country
	EventTitleBreakout       = "title.breakout"       // a title started rising in several countries at once
	EventPing                = "ping"                 // test delivery
)

// WebhookEventTypes lists the event types a webhook can subscribe to.
var WebhookEventTypes = []string{EventChartUpdated, EventTitleEntered, EventTitleNumberOne, EventAvailabilityAdded, EventAvailabilityRemoved, EventTitleBreakout}

var ErrWebhookNotFound = errors.New("webhook not found")

const (
	webhookInterval     = 30 * time.Second
	webhookTimeout      = 10 * time.Second
	webhookDueBatch     = 50
	webhookSourceBatch  = 500
	webhookLookback     = 2 * time.Minute // re-read charts changed just before the cursor (see produceCharts)
	webhookMaxAttempts  = 6
	webhookRetention    = "-30 days"
	webhookPruneEvery   = time.Hour
	cursorCharts        = "webhooks.cursor.charts"       // newest chart changed_at queued
	cursorChartsFloor   = "webhooks.cursor.charts_floor" // "<changed_at>|<max id>" when following started: charts up to it are never sent
	cursorOffers        = "webhooks.cursor.offers"       // newest watch_provider_events id queued
	webhookSecretPrefix = "whsec_"
	sqliteTimeLayout    = "2006-01-02 15:04:05"
)

// webhookBackoff is the wait after each failed attempt (SQLite modifiers).
var webhookBackoff = []string{"+1 minutes", "+5 minutes", "+30 minutes", "+2 hours", "+12 hours"}

// WebhookService manages webhook subscriptions and runs the dispatcher.
type WebhookService struct {
	log       *slog.Logger
	repo      *repository.WebhookRepository
	ops       *repository.OpsRepository
	top10     *Top10ReadService
	breakouts *BreakoutService // optional: title.breakout events
	http      *http.Client
	now       func() time.Time

	mu         sync.Mutex // one dispatcher pass at a time
	lastPruned time.Time
	// seenCharts: charts already turned into events (id -> changed_at) within
	// the lookback, so re-reading it skips them. In memory only: after a
	// restart the lookback is processed again (repeats are dropped by event
	// id).
	seenCharts map[int64]string
}

// WebhookOptions configures where deliveries may go.
type WebhookOptions struct {
	// AllowLoopback lets webhooks reach 127.0.0.0/8 and ::1 (a receiver on
	// the same machine, outside Docker). Off by default: inside a container
	// loopback is metareel itself.
	AllowLoopback bool
}

func NewWebhookService(log *slog.Logger, repo *repository.WebhookRepository, ops *repository.OpsRepository, top10 *Top10ReadService, opts WebhookOptions) *WebhookService {
	dialer := &net.Dialer{Timeout: 5 * time.Second, Control: webhookDialControl(opts.AllowLoopback)}
	return &WebhookService{log: log, repo: repo, ops: ops, top10: top10, now: time.Now,
		http: &http.Client{
			Timeout: webhookTimeout,
			Transport: &http.Transport{
				// No proxy: the address check below must see the receiver,
				// not a proxy.
				Proxy:                  nil,
				DialContext:            dialer.DialContext,
				TLSHandshakeTimeout:    5 * time.Second,
				ResponseHeaderTimeout:  webhookTimeout,
				MaxResponseHeaderBytes: 64 << 10,
				MaxIdleConns:           10,
				IdleConnTimeout:        90 * time.Second,
			},
			// A redirect is a failed delivery: the body would not be re-sent
			// and a redirect could point anywhere.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}}
}

// WithBreakouts enables title.breakout events.
func (s *WebhookService) WithBreakouts(b *BreakoutService) *WebhookService {
	s.breakouts = b
	return s
}

// errWebhookAddress: the receiver resolved to an address webhooks may not
// reach.
var errWebhookAddress = errors.New("address not allowed for webhooks (link-local, multicast, unspecified, or loopback unless WEBHOOK_ALLOW_LOOPBACK)")

// webhookDialControl checks each resolved address as it is dialled (so DNS
// rebinding is covered too): never link-local (169.254/16 incl. cloud
// metadata, fe80::/10), unspecified, multicast or broadcast; loopback only
// when allowed. LAN addresses are allowed: the intended receiver may be one.
func webhookDialControl(allowLoopback bool) func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return errWebhookAddress
		}
		ip := net.ParseIP(host)
		switch {
		case ip == nil, ip.IsUnspecified(), ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast(), ip.IsInterfaceLocalMulticast(),
			ip.IsMulticast(), ip.Equal(net.IPv4bcast), ip.IsLoopback() && !allowLoopback:
			return errWebhookAddress
		}
		return nil
	}
}

// --- admin ------------------------------------------------------------------------

// WebhookInfo is a webhook with its delivery counts per status.
type WebhookInfo struct {
	repository.Webhook
	Deliveries map[string]int64 `json:"deliveries"`
}

// WebhookCreated is a new (or re-keyed) webhook with its signing secret,
// shown this once.
type WebhookCreated struct {
	repository.Webhook
	Secret string `json:"secret"`
}

// WebhookInput creates a webhook (Enabled nil = true).
type WebhookInput struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Events  []string `json:"events"`
	Enabled *bool    `json:"enabled"`
}

// WebhookPatch changes some of a webhook's settings.
type WebhookPatch struct {
	Name    *string   `json:"name"`
	URL     *string   `json:"url"`
	Events  *[]string `json:"events"`
	Enabled *bool     `json:"enabled"`
}

// EventTypes lists the event types a webhook can subscribe to.
func (s *WebhookService) EventTypes() []string { return WebhookEventTypes }

func (s *WebhookService) List(ctx context.Context) ([]WebhookInfo, error) {
	hooks, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := s.repo.DeliveryCounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]WebhookInfo, 0, len(hooks))
	for _, w := range hooks {
		c := counts[w.ID]
		if c == nil {
			c = map[string]int64{}
		}
		out = append(out, WebhookInfo{Webhook: w, Deliveries: c})
	}
	return out, nil
}

func (s *WebhookService) Create(ctx context.Context, in WebhookInput) (*WebhookCreated, error) {
	name, err := validWebhookName(in.Name)
	if err != nil {
		return nil, err
	}
	u, err := validWebhookURL(in.URL)
	if err != nil {
		return nil, err
	}
	events, err := validWebhookEvents(in.Events)
	if err != nil {
		return nil, err
	}
	secret, err := newWebhookSecret()
	if err != nil {
		return nil, err
	}
	enabled := in.Enabled == nil || *in.Enabled
	w, err := s.repo.Create(ctx, name, u, secret, events, enabled)
	if err != nil {
		return nil, err
	}
	s.log.Info("webhook created", slog.Int64("webhook_id", w.ID), slog.String("host", webhookHost(u)), slog.Any("events", events))
	return &WebhookCreated{Webhook: w, Secret: secret}, nil
}

func (s *WebhookService) get(ctx context.Context, id int64) (repository.Webhook, error) {
	w, err := s.repo.Get(ctx, id)
	if repository.IsNotFound(err) {
		return w, ErrWebhookNotFound
	}
	return w, err
}

func (s *WebhookService) Update(ctx context.Context, id int64, p WebhookPatch) (*repository.Webhook, error) {
	w, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Name != nil {
		if w.Name, err = validWebhookName(*p.Name); err != nil {
			return nil, err
		}
	}
	if p.URL != nil {
		if w.URL, err = validWebhookURL(*p.URL); err != nil {
			return nil, err
		}
	}
	if p.Events != nil {
		if w.Events, err = validWebhookEvents(*p.Events); err != nil {
			return nil, err
		}
	}
	if p.Enabled != nil {
		w.Enabled = *p.Enabled
	}
	out, err := s.repo.Update(ctx, id, w.Name, w.URL, w.Events, w.Enabled)
	if repository.IsNotFound(err) {
		return nil, ErrWebhookNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update webhook: %w", err)
	}
	return &out, nil
}

func (s *WebhookService) Delete(ctx context.Context, id int64) error {
	ok, err := s.repo.Delete(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrWebhookNotFound
	}
	return nil
}

// RotateSecret gives a webhook a new signing secret (the old one stops
// working at once).
func (s *WebhookService) RotateSecret(ctx context.Context, id int64) (*WebhookCreated, error) {
	if _, err := s.get(ctx, id); err != nil {
		return nil, err
	}
	secret, err := newWebhookSecret()
	if err != nil {
		return nil, err
	}
	w, err := s.repo.SetSecret(ctx, id, secret)
	if err != nil {
		return nil, fmt.Errorf("rotate webhook secret: %w", err)
	}
	return &WebhookCreated{Webhook: w, Secret: secret}, nil
}

// WebhookTestResult is the outcome of a test ping.
type WebhookTestResult struct {
	Delivery   repository.WebhookDelivery `json:"delivery"`
	StatusCode int                        `json:"status_code,omitempty"`
	Error      string                     `json:"error,omitempty"`
	DurationMS int64                      `json:"duration_ms"`
}

// Test sends a ping to a webhook now (enabled or not) and records it.
func (s *WebhookService) Test(ctx context.Context, id int64) (*WebhookTestResult, error) {
	w, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	nonce, err := randomToken(8)
	if err != nil {
		return nil, err
	}
	eventID := "ping:" + nonce
	body, err := s.envelope(eventID, EventPing, map[string]any{"webhook_id": w.ID, "message": "Test delivery from metareel"})
	if err != nil {
		return nil, err
	}
	start := s.now()
	code, sendErr := s.send(ctx, w.URL, w.Secret, 0, eventID, EventPing, body)
	res := &WebhookTestResult{StatusCode: code, DurationMS: s.now().Sub(start).Milliseconds()}
	status := "delivered"
	if sendErr != nil {
		status, res.Error = "failed", sendErr.Error()
	}
	if res.Delivery, err = s.repo.Record(ctx, w.ID, eventID, EventPing, body, status, code, res.Error); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *WebhookService) Deliveries(ctx context.Context, id int64, limit int) ([]repository.WebhookDelivery, error) {
	if _, err := s.get(ctx, id); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.Deliveries(ctx, id, int64(limit))
}

// DeliveryPayload returns the JSON body of a delivery.
func (s *WebhookService) DeliveryPayload(ctx context.Context, deliveryID int64) (json.RawMessage, error) {
	p, err := s.repo.DeliveryPayload(ctx, deliveryID)
	if repository.IsNotFound(err) {
		return nil, ErrWebhookNotFound
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(p), nil
}

// Retry queues a delivered or failed delivery again.
func (s *WebhookService) Retry(ctx context.Context, deliveryID int64) error {
	ok, err := s.repo.Retry(ctx, deliveryID)
	if err != nil {
		return err
	}
	if !ok {
		return &ValidationError{Message: "no finished delivery with that id (pending deliveries are retried automatically)"}
	}
	return nil
}

func validWebhookName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return "", &ValidationError{Message: "name is required (at most 100 characters)"}
	}
	return name, nil
}

func validWebhookURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || len(raw) > 2000 {
		return "", &ValidationError{Message: "url must be an http(s) URL"}
	}
	return raw, nil
}

func validWebhookEvents(events []string) ([]string, error) {
	var out []string
	for _, e := range events {
		e = strings.TrimSpace(e)
		if e == "*" {
			return []string{"*"}, nil
		}
		if !slices.Contains(WebhookEventTypes, e) {
			return nil, &ValidationError{Message: fmt.Sprintf("unknown event %q, expected * or any of %s", e, strings.Join(WebhookEventTypes, ", "))}
		}
		if !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return nil, &ValidationError{Message: "events is required (* for every event)"}
	}
	return out, nil
}

func subscribed(w repository.Webhook, eventType string) bool {
	return slices.Contains(w.Events, "*") || slices.Contains(w.Events, eventType)
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func newWebhookSecret() (string, error) {
	t, err := randomToken(32)
	if err != nil {
		return "", err
	}
	return webhookSecretPrefix + t, nil
}

// webhookHost is the URL's host, for logs (paths and queries can hold
// tokens).
func webhookHost(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Host
	}
	return ""
}

// --- dispatcher ------------------------------------------------------------------

// Run follows the event sources and delivers queued events every 30 s until
// ctx is done.
func (s *WebhookService) Run(ctx context.Context) {
	t := time.NewTicker(webhookInterval)
	defer t.Stop()
	for {
		s.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Tick queues new events and makes due deliveries once.
func (s *WebhookService) Tick(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.produce(ctx); err != nil && ctx.Err() == nil {
		s.log.Warn("webhooks: queueing events failed", slog.Any("err", err))
	}
	if err := s.deliver(ctx); err != nil && ctx.Err() == nil {
		s.log.Warn("webhooks: delivering failed", slog.Any("err", err))
	}
	if s.now().Sub(s.lastPruned) >= webhookPruneEvery {
		s.lastPruned = s.now()
		if n, err := s.repo.Prune(ctx, webhookRetention); err != nil {
			s.log.Warn("webhooks: pruning deliveries failed", slog.Any("err", err))
		} else if n > 0 {
			s.log.Info("webhooks: pruned old deliveries", slog.Int64("deliveries", n))
		}
	}
}

// webhookEvent is one event to queue.
type webhookEvent struct {
	ID, Type string
	Data     any
}

// envelope is the JSON body of a delivery.
func (s *WebhookService) envelope(id, eventType string, data any) (string, error) {
	b, err := json.Marshal(map[string]any{"id": id, "type": eventType, "created_at": s.now().UTC().Format(time.RFC3339), "data": data})
	if err != nil {
		return "", fmt.Errorf("encode webhook event: %w", err)
	}
	return string(b), nil
}

func (s *WebhookService) queue(ctx context.Context, hooks []repository.Webhook, events []webhookEvent) error {
	for _, e := range events {
		var body string
		for _, w := range hooks {
			if !subscribed(w, e.Type) {
				continue
			}
			if body == "" {
				var err error
				if body, err = s.envelope(e.ID, e.Type, e.Data); err != nil {
					return err
				}
			}
			if _, err := s.repo.Queue(ctx, w.ID, e.ID, e.Type, body); err != nil {
				return err
			}
		}
	}
	return nil
}

// produce queues events from charts and watch provider changes since the
// cursors. A missing cursor starts at the newest source row: webhooks get
// what happens from now on, not the backlog.
func (s *WebhookService) produce(ctx context.Context) error {
	hooks, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return err
	}
	if err := s.produceCharts(ctx, hooks); err != nil {
		return err
	}
	return s.produceOffers(ctx, hooks)
}

// produceCharts reads charts changed since the cursor minus a lookback: a
// re-scraped chart keeps its row id and gets a new changed_at when its
// transaction commits, which can land just behind a cursor already moved by
// a concurrent read. Re-read events are deduplicated by event id.
func (s *WebhookService) produceCharts(ctx context.Context, hooks []repository.Webhook) error {
	cursor, err := s.ops.Setting(ctx, cursorCharts)
	if err != nil {
		return err
	}
	if cursor == "" {
		latest, maxID, err := s.repo.LatestChartChange(ctx)
		if err != nil {
			return err
		}
		if latest == "" {
			latest = s.now().UTC().Format(sqliteTimeLayout)
		}
		if err := s.ops.SetSetting(ctx, cursorChartsFloor, latest+"|"+strconv.FormatInt(maxID, 10)); err != nil {
			return err
		}
		return s.ops.SetSetting(ctx, cursorCharts, latest)
	}
	rawFloor, err := s.ops.Setting(ctx, cursorChartsFloor)
	if err != nil {
		return err
	}
	floorAt, floorIDRaw, _ := strings.Cut(rawFloor, "|")
	floorID, _ := strconv.ParseInt(floorIDRaw, 10, 64)
	// existedAtStart: the chart was already stored, unchanged since, when
	// following started (seconds are the timestamps' resolution, so ties
	// fall back to the row id; a chart re-scraped in that same second is
	// taken as old).
	existedAtStart := func(ch repository.ChangedChart) bool {
		return ch.ChangedAt < floorAt || (ch.ChangedAt == floorAt && ch.ID <= floorID)
	}
	c, err := time.Parse(sqliteTimeLayout, cursor)
	if err != nil {
		return fmt.Errorf("bad chart cursor %q: %w", cursor, err)
	}
	since := c.Add(-webhookLookback).Format(sqliteTimeLayout)
	if s.seenCharts == nil {
		s.seenCharts = map[int64]string{}
	}
	// Read from the lookback onwards page by page (keyset on changed_at, id),
	// skipping charts already handled, and turn at most webhookSourceBatch
	// charts into events per pass. Paging matters: more charts than a page
	// can share one changed_at (a backfill or bulk import), and a plain
	// LIMIT would re-read the same page forever without moving the cursor.
	newest := cursor
	changedDates := map[string]bool{}
	afterAt, afterID, processed := since, int64(0), 0
pages:
	for {
		charts, err := s.repo.ChartsChangedAfter(ctx, afterAt, afterID, webhookSourceBatch)
		if err != nil {
			return err
		}
		for _, ch := range charts {
			if s.seenCharts[ch.ID] == ch.ChangedAt {
				afterAt, afterID = ch.ChangedAt, ch.ID
				continue
			}
			if len(hooks) > 0 && !existedAtStart(ch) {
				if processed >= webhookSourceBatch {
					break pages // the rest next pass; the cursor stays behind them
				}
				processed++
				events, err := s.chartEvents(ctx, ch)
				if err != nil {
					return err
				}
				if err := s.queue(ctx, hooks, events); err != nil {
					return err
				}
				changedDates[ch.Date] = true
			}
			s.seenCharts[ch.ID] = ch.ChangedAt
			afterAt, afterID = ch.ChangedAt, ch.ID
			if ch.ChangedAt > newest {
				newest = ch.ChangedAt
			}
		}
		if len(charts) < webhookSourceBatch {
			break
		}
	}
	// Forget charts that fall out of the next pass's lookback (the cursor
	// never moves back).
	for id, at := range s.seenCharts {
		if at < since {
			delete(s.seenCharts, id)
		}
	}
	// Breakouts are derived and best effort: a failure costs at most one
	// announcement and must not hold up the chart cursor or other events.
	if err := s.produceBreakouts(ctx, hooks, changedDates); err != nil && ctx.Err() == nil {
		s.log.Warn("webhooks: queueing breakout events failed", slog.Any("err", err))
	}
	if newest != cursor {
		return s.ops.SetSetting(ctx, cursorCharts, newest)
	}
	return nil
}

// ChartRef names a chart in event data.
type ChartRef struct {
	Country  string `json:"country"`
	Provider string `json:"provider"`
	Category string `json:"category"` // movies | tv_shows
	Date     string `json:"date"`
}

// ChartUpdatedData is a chart.updated event.
type ChartUpdatedData struct {
	ChartRef
	ChangedAt string        `json:"changed_at"` // UTC "YYYY-MM-DD HH:MM:SS"
	Entries   []Top10Result `json:"entries"`
}

// TitleChartData is a title.entered / title.number_one event.
type TitleChartData struct {
	Chart     ChartRef    `json:"chart"`
	FirstTime bool        `json:"first_time"` // never on this chart before (title.entered)
	Entry     Top10Result `json:"entry"`
}

// chartEvents builds a changed chart's events from its stored entries.
func (s *WebhookService) chartEvents(ctx context.Context, ch repository.ChangedChart) ([]webhookEvent, error) {
	date, err := time.Parse(repository.DateLayout, ch.Date)
	if err != nil {
		return nil, nil
	}
	q := Top10Query{Date: date, CountryCode: ch.Country, Provider: ch.Provider}
	var res *Top10Response
	switch ch.Category {
	case "movies":
		res, err = s.top10.GetMoviesByProvider(ctx, q)
	case "tv_shows":
		res, err = s.top10.GetTVShowsByProvider(ctx, q)
	default:
		return nil, nil
	}
	var invalid *ValidationError
	if errors.Is(err, ErrTop10NotFound) || errors.As(err, &invalid) {
		// No entries, or a chart the read API can't serve (e.g. an unknown
		// country): nothing to send, and it must not block later charts.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ref := ChartRef{Country: ch.Country, Provider: ch.Provider, Category: ch.Category, Date: ch.Date}
	chartKey := ch.Country + ":" + ch.Provider + ":" + ch.Category + ":" + ch.Date
	events := []webhookEvent{{
		ID:   EventChartUpdated + ":" + strconv.FormatInt(ch.ID, 10) + ":" + ch.ChangedAt,
		Type: EventChartUpdated, Data: ChartUpdatedData{ChartRef: ref, ChangedAt: ch.ChangedAt, Entries: res.Items},
	}}
	for _, e := range res.Items {
		titleKey := chartKey + ":" + strconv.FormatInt(e.TitleID, 10)
		if e.PreviousRank == nil {
			events = append(events, webhookEvent{ID: EventTitleEntered + ":" + titleKey, Type: EventTitleEntered,
				Data: TitleChartData{Chart: ref, FirstTime: e.DaysInTop10 == 1, Entry: e}})
		}
		if e.Rank == 1 && (e.PreviousRank == nil || *e.PreviousRank != 1) {
			events = append(events, webhookEvent{ID: EventTitleNumberOne + ":" + titleKey, Type: EventTitleNumberOne,
				Data: TitleChartData{Chart: ref, FirstTime: e.DaysInTop10 == 1, Entry: e}})
		}
	}
	return events, nil
}

// BreakoutEventData is a title.breakout event: the title and how it rose
// (see BreakoutService; default window, min_countries and min_climb).
type BreakoutEventData struct {
	Date         string `json:"date"`
	Window       int    `json:"window"`
	MinCountries int    `json:"min_countries"`
	MinClimb     int    `json:"min_climb"`
	Breakout
}

// breakoutEventDays: only charts of the current chart date and the two
// before it raise title.breakout (a backfill of old dates doesn't).
const breakoutEventDays = 2

// produceBreakouts queues title.breakout for titles that are breakouts on a
// date whose charts changed but were not on the day before. A title that
// keeps rising is announced once; one that drops out and breaks out again
// later is announced again.
func (s *WebhookService) produceBreakouts(ctx context.Context, hooks []repository.Webhook, dates map[string]bool) error {
	if s.breakouts == nil || len(dates) == 0 || !slices.ContainsFunc(hooks, func(w repository.Webhook) bool { return subscribed(w, EventTitleBreakout) }) {
		return nil
	}
	today := flixpatrol.EffectiveTop10Date(s.now())
	oldest := repository.FormatDate(today.AddDate(0, 0, -breakoutEventDays))
	days := make([]string, 0, len(dates))
	for d := range dates {
		if d >= oldest && d <= repository.FormatDate(today) {
			days = append(days, d)
		}
	}
	sort.Strings(days)
	// Adjacent dates share results (a date is the day before the next).
	cache := map[string]*BreakoutsResponse{}
	breakoutsOn := func(day time.Time) (*BreakoutsResponse, error) {
		key := repository.FormatDate(day)
		if r, ok := cache[key]; ok {
			return r, nil
		}
		r, err := s.breakouts.all(ctx, BreakoutQuery{Date: &day})
		if err != nil {
			return nil, err
		}
		cache[key] = r
		return r, nil
	}
	for _, d := range days {
		day, err := time.Parse(repository.DateLayout, d)
		if err != nil {
			continue
		}
		cur, err := breakoutsOn(day)
		if err != nil {
			return err
		}
		if len(cur.Items) == 0 {
			continue
		}
		prev, err := breakoutsOn(day.AddDate(0, 0, -1))
		if err != nil {
			return err
		}
		before := map[string]bool{}
		for _, b := range prev.Items {
			before[b.key] = true
		}
		var events []webhookEvent
		for _, b := range cur.Items {
			if before[b.key] {
				continue
			}
			events = append(events, webhookEvent{ID: EventTitleBreakout + ":" + b.key + ":" + d, Type: EventTitleBreakout,
				Data: BreakoutEventData{Date: d, Window: cur.Window, MinCountries: cur.MinCountries, MinClimb: cur.MinClimb, Breakout: b}})
		}
		if err := s.queue(ctx, hooks, events); err != nil {
			return err
		}
	}
	return nil
}

func (s *WebhookService) produceOffers(ctx context.Context, hooks []repository.Webhook) error {
	cursor, err := s.ops.Setting(ctx, cursorOffers)
	if err != nil {
		return err
	}
	if cursor == "" {
		latest, err := s.repo.LatestOfferEventID(ctx)
		if err != nil {
			return err
		}
		return s.ops.SetSetting(ctx, cursorOffers, strconv.FormatInt(latest, 10))
	}
	after, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil {
		return fmt.Errorf("bad offer cursor %q: %w", cursor, err)
	}
	offers, err := s.repo.OfferEventsAfter(ctx, after, webhookSourceBatch)
	if err != nil || len(offers) == 0 {
		return err
	}
	events := make([]webhookEvent, 0, len(offers))
	for _, o := range offers {
		t := EventAvailabilityAdded
		if o.Event == repository.OfferRemoved {
			t = EventAvailabilityRemoved
		}
		events = append(events, webhookEvent{ID: "availability:" + strconv.FormatInt(o.ID, 10), Type: t, Data: o})
	}
	if err := s.queue(ctx, hooks, events); err != nil {
		return err
	}
	return s.ops.SetSetting(ctx, cursorOffers, strconv.FormatInt(offers[len(offers)-1].ID, 10))
}

// deliver attempts the due deliveries.
func (s *WebhookService) deliver(ctx context.Context) error {
	due, err := s.repo.Due(ctx, webhookDueBatch)
	if err != nil {
		return err
	}
	// A receiver that can't be reached (or times out) is skipped for the
	// rest of the pass, so one slow webhook doesn't hold up the others.
	unreachable := map[int64]bool{}
	for _, d := range due {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if unreachable[d.WebhookID] {
			continue
		}
		code, sendErr := s.send(ctx, d.URL, d.Secret, d.ID, d.EventID, d.EventType, d.Payload)
		if sendErr != nil && code == 0 {
			unreachable[d.WebhookID] = true
		}
		if sendErr == nil {
			if err := s.repo.Delivered(ctx, d, code); err != nil {
				return err
			}
			continue
		}
		attempt := int(d.Attempts) + 1
		final := attempt >= webhookMaxAttempts
		backoff := webhookBackoff[min(attempt-1, len(webhookBackoff)-1)]
		s.log.Warn("webhook delivery failed", slog.Int64("webhook_id", d.WebhookID), slog.Int64("delivery_id", d.ID),
			slog.String("event", d.EventType), slog.Int("attempt", attempt), slog.Bool("final", final), slog.Any("err", sendErr))
		if err := s.repo.Failed(ctx, d, code, sendErr.Error(), backoff, final); err != nil {
			return err
		}
	}
	return nil
}

// WebhookSignature signs a body as the X-Metareel-Signature header does:
// "t=<unix seconds>,v1=<hex HMAC-SHA256 of "<t>.<body>" with the secret>".
func WebhookSignature(secret string, ts int64, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	t := strconv.FormatInt(ts, 10)
	mac.Write([]byte(t + "." + body))
	return "t=" + t + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// send POSTs one delivery. It returns the status code (0 without a
// response) and an error unless the receiver answered 2xx. Errors never
// carry the URL (it can hold tokens) or the secret.
func (s *WebhookService) send(ctx context.Context, rawURL, secret string, deliveryID int64, eventID, eventType, body string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(body))
	if err != nil {
		return 0, errors.New("invalid webhook url")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "metareel-webhooks/1")
	req.Header.Set("X-Metareel-Event", eventType)
	req.Header.Set("X-Metareel-Event-Id", eventID)
	if deliveryID > 0 {
		req.Header.Set("X-Metareel-Delivery", strconv.FormatInt(deliveryID, 10))
	}
	req.Header.Set("X-Metareel-Signature", WebhookSignature(secret, s.now().Unix(), body))
	resp, err := s.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return 0, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(string(snippet))
		if msg != "" {
			return resp.StatusCode, fmt.Errorf("status %d: %s", resp.StatusCode, msg)
		}
		return resp.StatusCode, fmt.Errorf("status %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}
