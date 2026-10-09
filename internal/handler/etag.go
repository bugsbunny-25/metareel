package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

// etagRecorder buffers a response so its ETag can be computed.
type etagRecorder struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (r *etagRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}

func (r *etagRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.buf.Write(b)
}

// Unwrap lets Echo find its own Response underneath.
func (r *etagRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// ETag adds a strong ETag to successful GET responses and answers
// If-None-Match with 304, so clients (tides) can revalidate chart data that
// changes about once a day without downloading it again. Responses stay
// "private" since the public API is keyed.
func ETag(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		req := c.Request()
		if req.Method != http.MethodGet || strings.Contains(req.URL.Path, "/export/") {
			return next(c)
		}
		orig := c.Response()
		rec := &etagRecorder{ResponseWriter: orig}
		c.SetResponse(rec)
		err := next(c)
		c.SetResponse(orig)
		if err != nil {
			// Let Echo's error handler write the error response.
			if rec.buf.Len() == 0 {
				return err
			}
		}
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		if status == http.StatusOK {
			sum := sha256.Sum256(rec.buf.Bytes())
			etag := `"` + hex.EncodeToString(sum[:12]) + `"`
			h := orig.Header()
			h.Set("ETag", etag)
			h.Set("Cache-Control", "private, no-cache")
			if matchETag(req.Header.Get("If-None-Match"), etag) {
				h.Del("Content-Type")
				h.Del("Content-Length")
				orig.WriteHeader(http.StatusNotModified)
				return nil
			}
		}
		orig.WriteHeader(status)
		_, werr := orig.Write(rec.buf.Bytes())
		return werr
	}
}

func matchETag(header, etag string) bool {
	for _, v := range strings.Split(header, ",") {
		v = strings.TrimSpace(v)
		if v == "*" || strings.TrimPrefix(v, "W/") == etag {
			return true
		}
	}
	return false
}
