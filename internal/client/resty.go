package client

import (
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/bugsbunny-25/metareel/internal/config"
)

// New builds a pre-configured Resty client suitable for calling downstream
// REST APIs. It applies sane defaults: timeout, retries with exponential
// backoff, a User-Agent header, and a JSON Accept header.
func New(cfg config.HTTPClient) *resty.Client {
	c := resty.New().
		SetTimeout(cfg.Timeout).
		SetRetryCount(cfg.RetryCount).
		SetRetryWaitTime(500 * time.Millisecond).
		SetRetryMaxWaitTime(5 * time.Second).
		SetHeader("User-Agent", cfg.UserAgent).
		SetHeader("Accept", "application/json")

	// Retry on transient failures and 5xx responses.
	c.AddRetryCondition(func(r *resty.Response, err error) bool {
		if err != nil {
			return true
		}
		return r.StatusCode() >= 500
	})

	return c
}
