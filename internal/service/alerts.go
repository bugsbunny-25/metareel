package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

// AlertService posts problems to a webhook (ALERT_WEBHOOK_URL), at most once
// per alert key per cooldown. The webhook URL can embed a secret token, so it
// is never logged.
type AlertService struct {
	log      *slog.Logger
	ops      *repository.OpsRepository
	url      string
	format   string // json | slack | discord | ntfy
	cooldown time.Duration
	http     *http.Client
	now      func() time.Time
}

func NewAlertService(log *slog.Logger, ops *repository.OpsRepository, url, format string, cooldown time.Duration) *AlertService {
	if cooldown <= 0 {
		cooldown = 12 * time.Hour
	}
	return &AlertService{log: log, ops: ops, url: strings.TrimSpace(url), format: strings.ToLower(strings.TrimSpace(format)),
		cooldown: cooldown, http: &http.Client{Timeout: 15 * time.Second}, now: time.Now}
}

// Enabled reports whether a webhook is configured.
func (a *AlertService) Enabled() bool { return a != nil && a.url != "" }

// Alert is one notification.
type Alert struct {
	Key     string // dedupe key, e.g. "run_failed:3" or "stale_chart:US:netflix"
	Title   string
	Message string
	Level   string // warn | error
}

// Send posts the alert unless the same key was sent within the cooldown.
// It reports whether it was sent.
func (a *AlertService) Send(ctx context.Context, al Alert) (bool, error) {
	if !a.Enabled() {
		return false, nil
	}
	settingKey := "alert.last_sent." + al.Key
	if last, _ := a.ops.Setting(ctx, settingKey); last != "" {
		if t, err := time.Parse(time.RFC3339, last); err == nil && a.now().Sub(t) < a.cooldown {
			return false, nil
		}
	}
	if err := a.post(ctx, al); err != nil {
		a.log.Warn("alert webhook failed", slog.String("alert", al.Key), slog.Any("err", err))
		return false, err
	}
	a.log.Info("alert sent", slog.String("alert", al.Key))
	return true, a.ops.SetSetting(ctx, settingKey, a.now().UTC().Format(time.RFC3339))
}

// Resolve forgets a key so the next occurrence alerts immediately.
func (a *AlertService) Resolve(ctx context.Context, key string) {
	if a.Enabled() {
		_ = a.ops.SetSetting(ctx, "alert.last_sent."+key, "")
	}
}

func (a *AlertService) post(ctx context.Context, al Alert) error {
	text := "[metareel] " + al.Title
	if al.Message != "" {
		text += "\n" + al.Message
	}
	var (
		body        []byte
		contentType = "application/json"
		err         error
	)
	switch a.format {
	case "slack":
		body, err = json.Marshal(map[string]string{"text": text})
	case "discord":
		body, err = json.Marshal(map[string]string{"content": text})
	case "ntfy":
		body, contentType = []byte(al.Message), "text/plain; charset=utf-8"
	default:
		body, err = json.Marshal(map[string]string{"source": "metareel", "key": al.Key, "level": al.Level, "title": al.Title, "message": al.Message, "time": a.now().UTC().Format(time.RFC3339)})
	}
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build alert request: invalid ALERT_WEBHOOK_URL")
	}
	req.Header.Set("Content-Type", contentType)
	if a.format == "ntfy" {
		req.Header.Set("Title", "metareel: "+al.Title)
		if al.Level == "error" {
			req.Header.Set("Priority", "high")
		}
	}
	resp, err := a.http.Do(req)
	if err != nil {
		// The error text includes the URL; report only the cause.
		return fmt.Errorf("post alert: %s", redactURLError(err))
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("post alert: status %d", resp.StatusCode)
	}
	return nil
}

// redactURLError drops the request URL from a net/http client error.
func redactURLError(err error) string {
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok && u.Unwrap() != nil {
		return u.Unwrap().Error()
	}
	return "request failed"
}
