package client

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/bugsbunny-25/metareel/internal/config"
)

// resty logs transport errors itself, with the full URL; the key in the
// query string must not reach the logs.
func TestRestyLogsRedactKeys(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prev)

	c := New(config.HTTPClient{Timeout: time.Second, RetryCount: 1, UserAgent: "test"}) // resty logs only when retrying
	_, err := c.R().SetQueryParam("api_key", "sekrit123").Get("http://127.0.0.1:1/3/movie/1")
	if err == nil {
		t.Fatal("expected a connection error")
	}
	out := buf.String()
	if strings.Contains(out, "sekrit123") || !strings.Contains(out, "REDACTED") {
		t.Fatalf("logs = %q", out)
	}
}
