package handler

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"
	emw "github.com/labstack/echo/v5/middleware"
)

// redactedQueryParams are query parameters whose values never reach the
// request log: a phone's position (GET /api/v1/me/venues/nearby).
var redactedQueryParams = []string{"lat", "lon"}

// RequestLogger is echo's default request logger (same fields and messages)
// with the values of redactedQueryParams replaced in the logged URI.
func RequestLogger() echo.MiddlewareFunc {
	return emw.RequestLoggerWithConfig(emw.RequestLoggerConfig{
		LogLatency:       true,
		LogRemoteIP:      true,
		LogHost:          true,
		LogMethod:        true,
		LogURI:           true,
		LogRequestID:     true,
		LogUserAgent:     true,
		LogStatus:        true,
		LogContentLength: true,
		LogResponseSize:  true,
		HandleError:      true,
		LogValuesFunc: func(c *echo.Context, v emw.RequestLoggerValues) error {
			attrs := []slog.Attr{
				slog.String("method", v.Method),
				slog.String("uri", redactURI(v.URI)),
				slog.Int("status", v.Status),
				slog.Duration("latency", v.Latency),
				slog.String("host", v.Host),
				slog.String("bytes_in", v.ContentLength),
				slog.Int64("bytes_out", v.ResponseSize),
				slog.String("user_agent", v.UserAgent),
				slog.String("remote_ip", v.RemoteIP),
				slog.String("request_id", v.RequestID),
			}
			if v.Error == nil {
				c.Logger().LogAttrs(context.Background(), slog.LevelInfo, "REQUEST", attrs...)
				return nil
			}
			attrs = append(attrs, slog.String("error", v.Error.Error()))
			c.Logger().LogAttrs(context.Background(), slog.LevelError, "REQUEST_ERROR", attrs...)
			return nil
		},
	})
}

// redactURI replaces the values of redactedQueryParams in a request URI. A
// query that doesn't parse is dropped entirely rather than logged.
func redactURI(uri string) string {
	path, query, ok := strings.Cut(uri, "?")
	if !ok || query == "" {
		return uri
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return path + "?REDACTED"
	}
	changed := false
	for _, p := range redactedQueryParams {
		if _, ok := values[p]; ok {
			values[p] = []string{"REDACTED"}
			changed = true
		}
	}
	if !changed {
		return uri
	}
	return path + "?" + values.Encode()
}
