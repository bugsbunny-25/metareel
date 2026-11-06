package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository/sqlc"
)

// WebhookRepository stores webhook subscriptions and their delivery queue,
// and reads the event sources they follow.
type WebhookRepository struct {
	q *sqlc.Queries
}

func NewWebhookRepository(db *sql.DB) *WebhookRepository {
	return &WebhookRepository{q: sqlc.New(db)}
}

// Webhook is a subscription. Secret is only filled where it is needed (the
// delivery loop) or shown once (created / rotated).
type Webhook struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	URL            string     `json:"url"`
	Events         []string   `json:"events"`
	Enabled        bool       `json:"enabled"`
	LastDeliveryAt *time.Time `json:"last_delivery_at"`
	LastStatus     *string    `json:"last_status"`
	LastError      *string    `json:"last_error"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Secret         string     `json:"-"`
}

func toWebhook(w sqlc.Webhook) Webhook {
	out := Webhook{ID: w.ID, Name: w.Name, URL: w.Url, Enabled: w.Enabled, Secret: w.Secret,
		LastStatus: nullStringPtr(w.LastStatus), LastError: nullStringPtr(w.LastError),
		CreatedAt: w.CreatedAt.UTC(), UpdatedAt: w.UpdatedAt.UTC()}
	if err := json.Unmarshal([]byte(w.Events), &out.Events); err != nil || out.Events == nil {
		out.Events = []string{}
	}
	if w.LastDeliveryAt.Valid {
		t := w.LastDeliveryAt.Time.UTC()
		out.LastDeliveryAt = &t
	}
	return out
}

func eventsJSON(events []string) string {
	b, _ := json.Marshal(events)
	return string(b)
}

func (r *WebhookRepository) List(ctx context.Context) ([]Webhook, error) {
	rows, err := r.q.ListWebhooks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	out := make([]Webhook, 0, len(rows))
	for _, w := range rows {
		out = append(out, toWebhook(w))
	}
	return out, nil
}

// ListEnabled returns the enabled webhooks (with secrets).
func (r *WebhookRepository) ListEnabled(ctx context.Context) ([]Webhook, error) {
	rows, err := r.q.ListEnabledWebhooks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled webhooks: %w", err)
	}
	out := make([]Webhook, 0, len(rows))
	for _, w := range rows {
		out = append(out, toWebhook(w))
	}
	return out, nil
}

// Get returns a webhook (with its secret), or sql.ErrNoRows.
func (r *WebhookRepository) Get(ctx context.Context, id int64) (Webhook, error) {
	w, err := r.q.GetWebhook(ctx, id)
	if err != nil {
		return Webhook{}, err
	}
	return toWebhook(w), nil
}

func (r *WebhookRepository) Create(ctx context.Context, name, url, secret string, events []string, enabled bool) (Webhook, error) {
	w, err := r.q.CreateWebhook(ctx, sqlc.CreateWebhookParams{Name: name, Url: url, Secret: secret, Events: eventsJSON(events), Enabled: enabled})
	if err != nil {
		return Webhook{}, fmt.Errorf("create webhook: %w", err)
	}
	return toWebhook(w), nil
}

// Update replaces a webhook's settings (sql.ErrNoRows when it is gone).
func (r *WebhookRepository) Update(ctx context.Context, id int64, name, url string, events []string, enabled bool) (Webhook, error) {
	w, err := r.q.UpdateWebhook(ctx, sqlc.UpdateWebhookParams{ID: id, Name: name, Url: url, Events: eventsJSON(events), Enabled: enabled})
	if err != nil {
		return Webhook{}, err
	}
	return toWebhook(w), nil
}

func (r *WebhookRepository) SetSecret(ctx context.Context, id int64, secret string) (Webhook, error) {
	w, err := r.q.SetWebhookSecret(ctx, sqlc.SetWebhookSecretParams{ID: id, Secret: secret})
	if err != nil {
		return Webhook{}, err
	}
	return toWebhook(w), nil
}

// Delete removes a webhook and its deliveries; false when there was none.
func (r *WebhookRepository) Delete(ctx context.Context, id int64) (bool, error) {
	n, err := r.q.DeleteWebhook(ctx, id)
	if err != nil {
		return false, fmt.Errorf("delete webhook: %w", err)
	}
	return n > 0, nil
}

// Queue queues an event for a webhook; false when it was already queued.
func (r *WebhookRepository) Queue(ctx context.Context, webhookID int64, eventID, eventType, payload string) (bool, error) {
	n, err := r.q.QueueWebhookDelivery(ctx, sqlc.QueueWebhookDeliveryParams{WebhookID: webhookID, EventID: eventID, EventType: eventType, Payload: payload})
	if err != nil {
		return false, fmt.Errorf("queue webhook delivery: %w", err)
	}
	return n > 0, nil
}

// DueDelivery is a pending delivery to attempt now.
type DueDelivery struct {
	ID, WebhookID        int64
	EventID, EventType   string
	Payload, URL, Secret string
	Attempts             int64
}

func (r *WebhookRepository) Due(ctx context.Context, limit int64) ([]DueDelivery, error) {
	rows, err := r.q.ListDueWebhookDeliveries(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list due webhook deliveries: %w", err)
	}
	out := make([]DueDelivery, 0, len(rows))
	for _, d := range rows {
		out = append(out, DueDelivery{ID: d.ID, WebhookID: d.WebhookID, EventID: d.EventID, EventType: d.EventType,
			Payload: d.Payload, URL: d.Url, Secret: d.Secret, Attempts: d.Attempts})
	}
	return out, nil
}

func nullCode(code int) sql.NullInt64 { return sql.NullInt64{Int64: int64(code), Valid: code > 0} }

// Delivered records a successful attempt.
func (r *WebhookRepository) Delivered(ctx context.Context, d DueDelivery, statusCode int) error {
	if err := r.q.MarkWebhookDelivered(ctx, sqlc.MarkWebhookDeliveredParams{ID: d.ID, StatusCode: nullCode(statusCode)}); err != nil {
		return fmt.Errorf("mark webhook delivered: %w", err)
	}
	return r.q.RecordWebhookResult(ctx, sqlc.RecordWebhookResultParams{ID: d.WebhookID, Status: nullIfEmpty("delivered")})
}

// Failed records a failed attempt, retried after backoff (a datetime
// modifier) unless final.
func (r *WebhookRepository) Failed(ctx context.Context, d DueDelivery, statusCode int, msg, backoff string, final bool) error {
	f := int64(0)
	if final {
		f = 1
	}
	if err := r.q.MarkWebhookAttemptFailed(ctx, sqlc.MarkWebhookAttemptFailedParams{ID: d.ID, StatusCode: nullCode(statusCode),
		Error: sql.NullString{String: msg, Valid: true}, Final: f, Backoff: backoff}); err != nil {
		return fmt.Errorf("mark webhook attempt failed: %w", err)
	}
	return r.q.RecordWebhookResult(ctx, sqlc.RecordWebhookResultParams{ID: d.WebhookID, Status: nullIfEmpty("failed"), Error: sql.NullString{String: msg, Valid: true}})
}

// WebhookDelivery is a delivery as the admin API lists it.
type WebhookDelivery struct {
	ID             int64   `json:"id"`
	WebhookID      int64   `json:"webhook_id"`
	EventID        string  `json:"event_id"`
	EventType      string  `json:"event_type"`
	Status         string  `json:"status"`
	Attempts       int64   `json:"attempts"`
	NextAttemptAt  string  `json:"next_attempt_at,omitempty"` // UTC "YYYY-MM-DD HH:MM:SS" (pending)
	LastStatusCode *int64  `json:"last_status_code"`
	LastError      *string `json:"last_error"`
	CreatedAt      string  `json:"created_at"`
	DeliveredAt    string  `json:"delivered_at,omitempty"`
}

// Record stores a delivery made right away (a test ping) and returns it.
func (r *WebhookRepository) Record(ctx context.Context, webhookID int64, eventID, eventType, payload, status string, statusCode int, msg string) (WebhookDelivery, error) {
	d, err := r.q.InsertWebhookDelivery(ctx, sqlc.InsertWebhookDeliveryParams{WebhookID: webhookID, EventID: eventID, EventType: eventType,
		Payload: payload, Status: status, LastStatusCode: nullCode(statusCode), LastError: nullIfEmpty(msg)})
	if err != nil {
		return WebhookDelivery{}, fmt.Errorf("record webhook delivery: %w", err)
	}
	res := sqlc.RecordWebhookResultParams{ID: webhookID, Status: nullIfEmpty(status), Error: nullIfEmpty(msg)}
	if err := r.q.RecordWebhookResult(ctx, res); err != nil {
		return WebhookDelivery{}, fmt.Errorf("record webhook result: %w", err)
	}
	out := WebhookDelivery{ID: d.ID, WebhookID: d.WebhookID, EventID: d.EventID, EventType: d.EventType, Status: d.Status,
		Attempts: d.Attempts, LastStatusCode: nullInt64Ptr(d.LastStatusCode), LastError: nullStringPtr(d.LastError),
		CreatedAt: d.CreatedAt.UTC().Format("2006-01-02 15:04:05")}
	if d.DeliveredAt.Valid {
		out.DeliveredAt = d.DeliveredAt.Time.UTC().Format("2006-01-02 15:04:05")
	}
	return out, nil
}

// Deliveries lists a webhook's latest deliveries, newest first.
func (r *WebhookRepository) Deliveries(ctx context.Context, webhookID, limit int64) ([]WebhookDelivery, error) {
	rows, err := r.q.ListWebhookDeliveries(ctx, sqlc.ListWebhookDeliveriesParams{WebhookID: webhookID, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("list webhook deliveries: %w", err)
	}
	out := make([]WebhookDelivery, 0, len(rows))
	for _, d := range rows {
		item := WebhookDelivery{ID: d.ID, WebhookID: d.WebhookID, EventID: d.EventID, EventType: d.EventType, Status: d.Status,
			Attempts: d.Attempts, LastStatusCode: nullInt64Ptr(d.LastStatusCode), LastError: nullStringPtr(d.LastError),
			CreatedAt: d.CreatedAt, DeliveredAt: d.DeliveredAt}
		if d.Status == "pending" {
			item.NextAttemptAt = d.NextAttemptAt
		}
		out = append(out, item)
	}
	return out, nil
}

// DeliveryPayload returns a delivery's JSON body (sql.ErrNoRows if none).
func (r *WebhookRepository) DeliveryPayload(ctx context.Context, id int64) (string, error) {
	return r.q.GetWebhookDeliveryPayload(ctx, id)
}

// Retry queues a finished delivery again; false when there is none (or it
// is still pending).
func (r *WebhookRepository) Retry(ctx context.Context, id int64) (bool, error) {
	n, err := r.q.RetryWebhookDelivery(ctx, id)
	if err != nil {
		return false, fmt.Errorf("retry webhook delivery: %w", err)
	}
	return n > 0, nil
}

// DeliveryCounts returns deliveries per webhook and status.
func (r *WebhookRepository) DeliveryCounts(ctx context.Context) (map[int64]map[string]int64, error) {
	rows, err := r.q.CountWebhookDeliveriesByStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("count webhook deliveries: %w", err)
	}
	out := map[int64]map[string]int64{}
	for _, row := range rows {
		if out[row.WebhookID] == nil {
			out[row.WebhookID] = map[string]int64{}
		}
		out[row.WebhookID][row.Status] = row.N
	}
	return out, nil
}

// Prune deletes finished deliveries older than window ('-30 days').
func (r *WebhookRepository) Prune(ctx context.Context, window string) (int64, error) {
	n, err := r.q.PruneWebhookDeliveries(ctx, window)
	if err != nil {
		return 0, fmt.Errorf("prune webhook deliveries: %w", err)
	}
	return n, nil
}

// ChangedChart is a chart stored or changed (an event source).
type ChangedChart struct {
	ID                                int64
	Date, Country, Provider, Category string
	Entries                           int64
	ChangedAt                         string // UTC "YYYY-MM-DD HH:MM:SS"
}

// ChartsChangedSince lists charts stored or changed at or after since.
func (r *WebhookRepository) ChartsChangedSince(ctx context.Context, since string, limit int64) ([]ChangedChart, error) {
	rows, err := r.q.ListChartsChangedSince(ctx, sqlc.ListChartsChangedSinceParams{Since: since, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("list changed charts: %w", err)
	}
	out := make([]ChangedChart, 0, len(rows))
	for _, c := range rows {
		out = append(out, ChangedChart{ID: c.ID, Date: c.RankedOn, Country: c.Country, Provider: c.StreamingProvider,
			Category: c.Category, Entries: c.EntryCount, ChangedAt: c.ChangedAt})
	}
	return out, nil
}

// LatestChartChange returns the newest changed_at ("" when there are no
// charts) and the highest chart row id.
func (r *WebhookRepository) LatestChartChange(ctx context.Context) (string, int64, error) {
	row, err := r.q.MaxChartChangedAt(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("latest chart change: %w", err)
	}
	return row.ChangedAt, row.MaxID, nil
}

// OfferEventsAfter lists watch provider events after an id.
func (r *WebhookRepository) OfferEventsAfter(ctx context.Context, afterID, limit int64) ([]OfferEvent, error) {
	rows, err := r.q.ListOfferEventsAfter(ctx, sqlc.ListOfferEventsAfterParams{AfterID: afterID, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("list offer events: %w", err)
	}
	out := make([]OfferEvent, 0, len(rows))
	for _, e := range rows {
		out = append(out, OfferEvent{ID: e.ID, TmdbKind: e.TmdbKind, TmdbID: e.TmdbID, Title: e.Title, Country: e.Country,
			Monetization: e.Monetization, ProviderID: e.ProviderID, ProviderName: e.ProviderName, Event: e.Event,
			PreviousCheckAt: e.PreviousCheckAt, ObservedAt: e.ObservedAt})
	}
	return out, nil
}

// LatestOfferEventID returns the newest watch provider event id (0 = none).
func (r *WebhookRepository) LatestOfferEventID(ctx context.Context) (int64, error) {
	return r.q.MaxOfferEventID(ctx)
}

// IsNotFound reports whether err means the webhook or delivery is gone.
func IsNotFound(err error) bool { return errors.Is(err, sql.ErrNoRows) }
