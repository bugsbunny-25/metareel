package handler

import (
	"net/http"
	"net/url"

	"github.com/labstack/echo/v5"
)

// SameOriginWrites refuses requests that change data (anything but GET,
// HEAD and OPTIONS) when a browser says they come from another site: the
// admin API is unauthenticated (LAN only), so a web page a LAN user opens
// must not be able to send it requests (cross-site form posts and fetches
// with no preflight, e.g. a bodiless POST). Browsers mark the origin with
// Sec-Fetch-Site and Origin; clients that send neither (curl, scripts) are
// allowed. Behind a reverse proxy that rewrites Host, X-Forwarded-Host is
// compared instead.
func SameOriginWrites(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return next(c)
		}
		switch r.Header.Get("Sec-Fetch-Site") {
		case "", "same-origin", "none":
		default:
			return c.JSON(http.StatusForbidden, map[string]any{"error": "cross-site requests to the admin API are not allowed"})
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "null" {
			host := r.Host
			if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
				host = fwd
			}
			if u, err := url.Parse(origin); err != nil || u.Host != host {
				return c.JSON(http.StatusForbidden, map[string]any{"error": "cross-site requests to the admin API are not allowed"})
			}
		} else if origin == "null" {
			return c.JSON(http.StatusForbidden, map[string]any{"error": "cross-site requests to the admin API are not allowed"})
		}
		return next(c)
	}
}
