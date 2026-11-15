package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

type fakePosters struct {
	calls int
	data  []byte
	err   error
}

func (f *fakePosters) Image(_ context.Context, size, _ string) ([]byte, error) {
	f.calls++
	if size != "w92" {
		return nil, errors.New("unexpected size " + size)
	}
	return f.data, f.err
}

// A 2x1 image, one red and one blue pixel, averages to purple.
func twoPixelPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{R: 200, G: 0, B: 0, A: 255})
	img.Set(1, 0, color.RGBA{R: 0, G: 0, B: 100, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPosterColor(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewTrackingRepository(newTestDB(t))
	src := &fakePosters{data: twoPixelPNG(t)}
	s := NewPosterColorService(slog.New(slog.NewTextHandler(io.Discard, nil)), repo, src)

	if got := s.Color(ctx, "/poster1.jpg"); got != "#640032" {
		t.Fatalf("colour = %q, want #640032", got)
	}
	// Cached: no second download.
	if got := s.Color(ctx, "/poster1.jpg"); got != "#640032" || src.calls != 1 {
		t.Errorf("second call = %q after %d downloads, want the cached colour after 1", got, src.calls)
	}
	// Anything that isn't a TMDB image path is never fetched.
	for _, p := range []string{"", "../etc/passwd", "/a/b.jpg", "https://example.com/x.jpg", "/x.gif"} {
		if got := s.Color(ctx, p); got != "" {
			t.Errorf("Color(%q) = %q, want empty", p, got)
		}
	}
	if src.calls != 1 {
		t.Errorf("invalid paths were fetched (%d downloads)", src.calls)
	}
	// A failed download or an undecodable image gives no colour and caches nothing.
	src.err = errors.New("cdn down")
	if got := s.Color(ctx, "/poster2.jpg"); got != "" {
		t.Errorf("failed download = %q, want empty", got)
	}
	src.err, src.data = nil, []byte("not an image")
	if got := s.Color(ctx, "/poster2.jpg"); got != "" {
		t.Errorf("bad image = %q, want empty", got)
	}
	src.data = twoPixelPNG(t)
	if got := s.Color(ctx, "/poster2.jpg"); got != "#640032" {
		t.Errorf("after recovery = %q, want #640032", got)
	}
	var nilService *PosterColorService
	if got := nilService.Color(ctx, "/poster1.jpg"); got != "" {
		t.Errorf("nil service = %q", got)
	}
}
