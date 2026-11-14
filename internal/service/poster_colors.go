package service

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // TMDB posters are JPEGs
	_ "image/png"
	"log/slog"
	"time"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/repository"
)

// posterColorTimeout bounds fetching and decoding a poster on first use, so a
// slow CDN never holds up a title page for long; the colour is optional.
const posterColorTimeout = 2 * time.Second

// posterImageSource downloads a TMDB image.
type posterImageSource interface {
	Image(ctx context.Context, size, path string) ([]byte, error)
}

// PosterColorService gives the average colour of a TMDB poster, for
// artwork-tinted surfaces in the mobile app (the web app samples the image in
// the browser). Computed once per poster path from the smallest size and
// cached in poster_colors.
type PosterColorService struct {
	log  *slog.Logger
	repo *repository.TrackingRepository
	src  posterImageSource
}

func NewPosterColorService(log *slog.Logger, repo *repository.TrackingRepository, src posterImageSource) *PosterColorService {
	return &PosterColorService{log: log, repo: repo, src: src}
}

// Color returns "#rrggbb" for a poster path, or "" when there is no poster or
// it can't be fetched (logged; the caller just shows no tint).
func (s *PosterColorService) Color(ctx context.Context, posterPath string) string {
	if s == nil || !client.ValidTMDBImagePath(posterPath) {
		return ""
	}
	if c, err := s.repo.PosterColor(ctx, posterPath); err == nil {
		return c
	} else if !errors.Is(err, sql.ErrNoRows) {
		s.log.Warn("poster colour lookup failed", "poster_path", posterPath, "err", err)
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, posterColorTimeout)
	defer cancel()
	data, err := s.src.Image(ctx, "w92", posterPath)
	if err != nil {
		s.log.Warn("poster colour: fetching the poster failed", "poster_path", posterPath, "err", err)
		return ""
	}
	c, err := averageColor(data)
	if err != nil {
		s.log.Warn("poster colour: decoding the poster failed", "poster_path", posterPath, "err", err)
		return ""
	}
	if err := s.repo.SavePosterColor(ctx, posterPath, c); err != nil {
		s.log.Warn("poster colour: saving failed", "poster_path", posterPath, "err", err)
	}
	return c
}

// averageColor is the mean of every pixel's RGB, as "#rrggbb".
func averageColor(data []byte) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	b := img.Bounds()
	var r, g, bl, n uint64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			r += uint64(cr >> 8)
			g += uint64(cg >> 8)
			bl += uint64(cb >> 8)
			n++
		}
	}
	if n == 0 {
		return "", fmt.Errorf("empty image")
	}
	return fmt.Sprintf("#%02x%02x%02x", (r+n/2)/n, (g+n/2)/n, (bl+n/2)/n), nil
}
