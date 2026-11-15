package client

import (
	"context"
	"fmt"
	"regexp"
)

// tmdbImageBase is TMDB's image CDN; images need no API key.
const tmdbImageBase = "https://image.tmdb.org/t/p/"

// maxTMDBImageBytes caps a downloaded image (the smallest poster size is a
// few kilobytes).
const maxTMDBImageBytes = 512 << 10

var (
	tmdbImagePathRe = regexp.MustCompile(`^/[A-Za-z0-9_-]{1,64}\.(jpg|jpeg|png)$`)
	tmdbImageSizeRe = regexp.MustCompile(`^(w[0-9]{2,4}|original)$`)
)

// ValidTMDBImagePath reports whether path looks like a TMDB image path
// ("/abc123.jpg"), so nothing else is ever requested from the CDN.
func ValidTMDBImagePath(path string) bool { return tmdbImagePathRe.MatchString(path) }

// Image downloads a TMDB image (size e.g. "w92", path e.g. "/abc.jpg").
func (c *TMDBClient) Image(ctx context.Context, size, path string) ([]byte, error) {
	if !tmdbImageSizeRe.MatchString(size) || !ValidTMDBImagePath(path) {
		return nil, fmt.Errorf("tmdb image: invalid size or path")
	}
	resp, err := c.http.R().SetContext(ctx).SetDoNotParseResponse(true).Get(tmdbImageBase + size + path)
	if err != nil {
		return nil, fmt.Errorf("tmdb image: %w", err)
	}
	body := resp.RawBody()
	defer body.Close()
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("tmdb image: HTTP %d", resp.StatusCode())
	}
	buf := make([]byte, 0, 16<<10)
	tmp := make([]byte, 32<<10)
	for {
		n, err := body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if len(buf) > maxTMDBImageBytes {
			return nil, fmt.Errorf("tmdb image: larger than %d bytes", maxTMDBImageBytes)
		}
		if err != nil {
			break
		}
	}
	return buf, nil
}
