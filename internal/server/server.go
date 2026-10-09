package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/labstack/echo/v5"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/config"
	"github.com/bugsbunny-25/metareel/internal/handler"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/router"
	"github.com/bugsbunny-25/metareel/internal/scheduler"
	"github.com/bugsbunny-25/metareel/internal/service"
	"github.com/bugsbunny-25/metareel/internal/tasks"
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

	flixRepo := repository.NewFlixPatrolRepository(db)
	taskScheduleRepo := repository.NewTaskScheduleRepository(db)
	importsRepo := repository.NewImportsRepository(db)
	metadataRepo := repository.NewMetadataRepository(db)
	analyticsRepo := repository.NewAnalyticsRepository(db)
	top10Read := service.NewTop10ReadService(flixRepo)

	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}
	asynqClient := asynq.NewClient(redisOpt)
	defer asynqClient.Close()

	ratings := newRatingsService(log, cfg, db, flixRepo).WithIMDbDataset(importsRepo)
	taskBootstrap := taskruntime.Build(log, cfg, db, taskruntime.Deps{Asynq: asynqClient, Ratings: ratings})

	taskScheduleAdmin := service.NewTaskScheduleAdminService(taskScheduleRepo, asynqClient, cfg.Asynq.Queue)

	worker := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.Asynq.Concurrency,
		Queues:      map[string]int{cfg.Asynq.Queue: 1},
		Logger:      scheduler.NewAsynqLogger(log),
		LogLevel:    scheduler.AsynqLogLevel(cfg.Log.Level),
	})

	workerErr := make(chan error, 1)
	go func() {
		if err := worker.Run(taskBootstrap.Mux); err != nil {
			workerErr <- err
		}
		close(workerErr)
	}()

	go failStaleRuns(ctx, log, taskScheduleRepo)

	periodicManager, err := scheduler.NewPeriodicTaskManager(redisOpt, taskBootstrap.ConfigProvider, log, cfg.Log.Level)
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
	httpClient := client.New(cfg.HTTP)
	justWatch := client.NewJustWatchClient(httpClient.GetClient())
	apiKeys := service.NewAPIKeyService(log, repository.NewAPIKeyRepository(db))
	if err := apiKeys.Load(ctx); err != nil {
		return fmt.Errorf("load api keys: %w", err)
	}
	keyCount, _ := apiKeys.List(ctx)
	settings := apiKeys.Settings()
	log.Info("public api access",
		slog.Bool("require_api_key", settings.RequireAPIKey),
		slog.Int("api_keys", len(keyCount)),
		slog.Int("rate_limit_per_minute", settings.RateLimitPerMinute))
	if settings.RequireAPIKey && len(keyCount) == 0 {
		log.Warn("public api requires an API key but none exist yet; create one in the admin UI (Settings)")
	}
	h := &handler.Handler{
		Top10:      top10Read,
		AdminTasks: taskScheduleAdmin,
		Titles:     titlesAdmin,
		Ratings:    ratings,
		Releases:   service.NewReleasesService(log, justWatch),
		Candidates: service.NewTitleCandidatesService(log, flixRepo, justWatch, client.NewTMDB(httpClient, cfg.TMDB.APIKey), client.NewRottenTomatoes(cfg.HTTP.Timeout)),
		Stats:      service.NewAdminStatsService(taskScheduleRepo, flixRepo),
		APIKeys:    apiKeys,
		Health:     service.NewHealthService(db, asynqClient.Ping, taskBootstrap.Fetcher),

		Charts:    service.NewChartsService(analyticsRepo),
		Overview:  service.NewTitleOverviewService(analyticsRepo, flixRepo, metadataRepo, ratings),
		Analytics: service.NewAnalyticsService(analyticsRepo, metadataRepo, importsRepo, repository.NewRatingsRepository(db)),
		DataQuality: service.NewDataQualityService(flixRepo, repository.NewOpsRepository(db), importsRepo, metadataRepo,
			taskScheduleRepo, taskBootstrap.Maintenance),
	}

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

func newRatingsService(log *slog.Logger, cfg *config.Config, db *sql.DB, flixRepo *repository.FlixPatrolRepository) *service.RatingsService {
	httpClient := client.New(cfg.HTTP)
	mdb := client.NewMDBList(cfg.MDBList.APIKey, cfg.HTTP.Timeout)
	svc := service.NewRatingsService(log,
		repository.NewRatingsRepository(db),
		flixRepo,
		client.NewJustWatchClient(httpClient.GetClient()),
		mdb,
		client.NewRottenTomatoes(cfg.HTTP.Timeout),
		client.NewTMDB(httpClient, cfg.TMDB.APIKey),
		service.RatingsServiceConfig{TTL: cfg.Ratings.TTL, PreferredProvider: cfg.Ratings.PreferredProvider},
	)
	log.Info("ratings configured",
		slog.Duration("ttl", cfg.Ratings.TTL),
		slog.Bool("mdblist", mdb != nil),
		slog.Bool("tmdb", cfg.TMDB.APIKey != ""),
		slog.String("preferred_provider", svc.PreferredProvider()))
	return svc
}

// failStaleRuns marks runs that never finished (server restarted mid-run, or
// the task timed out) as failed, at startup and then hourly, so they don't
// show as running forever.
func failStaleRuns(ctx context.Context, log *slog.Logger, repo *repository.TaskScheduleRepository) {
	olderThan := fmt.Sprintf("-%d minutes", int(tasks.MaxTimeout.Minutes())+10)
	sweep := func() {
		n, err := repo.FailStaleTaskRuns(ctx, olderThan)
		if err != nil {
			log.Warn("failing stale task runs", slog.Any("err", err))
			return
		}
		if n > 0 {
			log.Warn("marked interrupted task runs as failed", slog.Int64("runs", n))
		}
	}
	sweep()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sweep()
		}
	}
}
