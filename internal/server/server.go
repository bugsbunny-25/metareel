package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"
	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/config"
	"github.com/bugsbunny-25/metareel/internal/handler"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/router"
	"github.com/bugsbunny-25/metareel/internal/scheduler"
	"github.com/bugsbunny-25/metareel/internal/service"
	taskruntime "github.com/bugsbunny-25/metareel/internal/tasks/runtime"
)

// Run bootstraps every dependency (DB, cache, HTTP client, scraper),
// registers routes, starts the Echo server, and blocks until a SIGINT /
// SIGTERM triggers a graceful shutdown.
func Run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	// --- Database ---------------------------------------------------------
	db, err := repository.Open(cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() {
		// WAL keeps recent writes in metareel.db-wal until checkpointed; merge
		// into the main file on shutdown so CLI/GUIs that open only metareel.db
		// see the same data the server had.
		if _, err := db.Exec(`PRAGMA wal_checkpoint(FULL)`); err != nil {
			log.Warn("sqlite wal checkpoint", slog.Any("err", err))
		}
		if err := db.Close(); err != nil {
			log.Warn("sqlite close", slog.Any("err", err))
		}
	}()

	if err := repository.Migrate(db, cfg.Database.MigrationsDir); err != nil {
		return fmt.Errorf("migrate db: %w", err)
	}
	log.Info("database ready", slog.String("dsn", cfg.Database.URL))

	taskBootstrap := taskruntime.Build(log, cfg, db)
	flixRepo := repository.NewFlixPatrolRepository(db)
	taskScheduleRepo := repository.NewTaskScheduleRepository(db)
	top10Read := service.NewTop10ReadService(flixRepo)

	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}
	asynqClient := asynq.NewClient(redisOpt)
	defer asynqClient.Close()

	taskScheduleAdmin := service.NewTaskScheduleAdminService(taskScheduleRepo, asynqClient, cfg.Asynq.Queue)

	worker := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.Asynq.Concurrency,
		Queues:      map[string]int{cfg.Asynq.Queue: 1},
	})

	workerErr := make(chan error, 1)
	go func() {
		if err := worker.Run(taskBootstrap.Mux); err != nil {
			workerErr <- err
		}
		close(workerErr)
	}()

	periodicManager, err := scheduler.NewPeriodicTaskManager(redisOpt, taskBootstrap.ConfigProvider)
	if err != nil {
		return fmt.Errorf("create periodic task manager: %w", err)
	}
	schedulerErr := make(chan error, 1)
	go func() {
		if err := periodicManager.Run(); err != nil {
			schedulerErr <- err
		}
		close(schedulerErr)
	}()

	titlesAdmin := service.NewTitleAdminService(flixRepo)
	h := handler.New(top10Read, taskScheduleAdmin, titlesAdmin)

	// --- HTTP server ------------------------------------------------------
	e := echo.New()
	router.Register(e, h, cfg)

	httpServer := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      e,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Run server in a goroutine so we can handle shutdown signals.
	serverErr := make(chan error, 1)
	go func() {
		log.Info("server listening", slog.String("addr", cfg.Server.Address()))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Block on either a terminal signal or an unrecoverable server error.
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-sigCtx.Done():
		log.Info("shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
	case err := <-workerErr:
		if err != nil {
			return fmt.Errorf("asynq worker error: %w", err)
		}
	case err := <-schedulerErr:
		if err != nil {
			return fmt.Errorf("asynq scheduler error: %w", err)
		}
	}

	periodicManager.Shutdown()
	worker.Shutdown()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	log.Info("server stopped cleanly")
	return nil
}
