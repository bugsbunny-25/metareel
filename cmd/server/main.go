package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/bugsbunny-25/metareel/internal/config"
	"github.com/bugsbunny-25/metareel/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", slog.Any("err", err))
		os.Exit(1)
	}

	log, closeLog := newLogger(cfg.Log.Level, cfg.Log.FilePath)
	defer closeLog()
	slog.SetDefault(log)

	if err := server.Run(context.Background(), cfg, log); err != nil {
		log.Error("server failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func newLogger(level string, filePath string) (*slog.Logger, func()) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	writer := io.Writer(os.Stdout)
	closeFn := func() {}
	if strings.TrimSpace(filePath) != "" {
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			slog.Error("create log directory failed", slog.String("path", filePath), slog.Any("err", err))
		} else {
			f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				slog.Error("open log file failed", slog.String("path", filePath), slog.Any("err", err))
			} else {
				writer = io.MultiWriter(os.Stdout, f)
				closeFn = func() { _ = f.Close() }
			}
		}
	}

	h := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: lvl})
	return slog.New(h), closeFn
}
