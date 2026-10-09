package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Downloader fetches public data files (Netflix Top 10 TSVs, IMDb datasets)
// and lets the caller skip a file whose Last-Modified / ETag is unchanged.
type Downloader struct {
	http      *http.Client
	userAgent string
}

func NewDownloader(timeout time.Duration, userAgent string) *Downloader {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	if userAgent == "" {
		userAgent = "metareel/0.1 (+https://github.com/bugsbunny-25/metareel)"
	}
	return &Downloader{http: &http.Client{Timeout: timeout}, userAgent: userAgent}
}

// FileVersion identifies a version of a remote file.
type FileVersion struct {
	ETag         string
	LastModified string
}

// Same reports whether v and o are known to be the same version.
func (v FileVersion) Same(o FileVersion) bool {
	if v.ETag != "" && o.ETag != "" {
		return v.ETag == o.ETag
	}
	return v.LastModified != "" && v.LastModified == o.LastModified
}

// Download is an open download; Body must be closed.
type Download struct {
	Body    io.ReadCloser
	Version FileVersion
	Size    int64
}

// Open starts downloading url. When the response's version equals prev, it
// closes the body without reading it and returns nil (unchanged): some hosts
// (Netflix) refuse HEAD and ignore If-Modified-Since, so this is the cheap
// way to check.
func (d *Downloader) Open(ctx context.Context, url string, prev FileVersion) (*Download, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", d.userAgent)
	if prev.ETag != "" {
		req.Header.Set("If-None-Match", prev.ETag)
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if resp.StatusCode == http.StatusNotModified {
		resp.Body.Close()
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		resp.Body.Close()
		return nil, fmt.Errorf("download %s: status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	v := FileVersion{ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified")}
	if v.Same(prev) {
		resp.Body.Close()
		return nil, nil
	}
	return &Download{Body: resp.Body, Version: v, Size: resp.ContentLength}, nil
}
