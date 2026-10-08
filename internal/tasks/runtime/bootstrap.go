package runtime

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/config"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scheduler"
	"github.com/bugsbunny-25/metareel/internal/scraper"
	"github.com/bugsbunny-25/metareel/internal/scraper/flixpatrol"
	"github.com/bugsbunny-25/metareel/internal/service"
	taskhandlers "github.com/bugsbunny-25/metareel/internal/tasks/handlers"
)

// Bootstrap wires task handlers and periodic config provider.
// Server startup can consume this as a single unit.
type Bootstrap struct {
	Mux            *asynq.ServeMux
	ConfigProvider asynq.PeriodicTaskConfigProvider
}

func Build(log *slog.Logger, cfg *config.Config, db *sql.DB) *Bootstrap {
	// Task schedule config source (DB-backed, shared by scheduler + handlers).
	taskScheduleRepo := repository.NewTaskScheduleRepository(db)

	// FlixPatrol task dependencies.
	flixRepo := repository.NewFlixPatrolRepository(db)
	httpClient := client.New(cfg.HTTP)
	var fetcher flixpatrol.Fetcher = flixpatrol.LoggingFetcher{
		Next: flixpatrol.CollyFetcher{Base: scraper.NewCollector(cfg.Scraper)},
		Via:  "colly",
		Log:  log,
	}
	if fs := client.NewFlareSolverr(cfg.FlareSolverr); fs != nil {
		fetcher = flixpatrol.LoggingFetcher{
			Next: flixpatrol.FlareSolverrFetcher{Client: fs, Log: log},
			Via:  "flaresolverr",
			Log:  log,
		}
		logFlareSolverrHealth(log, fs, cfg.FlareSolverr)
	} else {
		log.Info("flixpatrol pages will be fetched directly (FLARESOLVERR_URL not set)")
	}
	tmdbClient := client.NewTMDB(httpClient, cfg.TMDB.APIKey)
	justWatchClient := client.NewJustWatchClient(httpClient.GetClient())
	wikidataClient := client.NewWikidata(httpClient)
	flixJob := service.NewFlixPatrolJob(log, fetcher, flixRepo, tmdbClient, justWatchClient, wikidataClient, client.NewRottenTomatoes(cfg.HTTP.Timeout))
	flixHandler := taskhandlers.NewFlixPatrolHandler(log, taskScheduleRepo, flixJob)

	return &Bootstrap{
		Mux:            taskhandlers.NewServeMux(flixHandler),
		ConfigProvider: scheduler.NewDBPeriodicTaskConfigProvider(taskScheduleRepo, cfg.Asynq.Queue),
	}
}

// logFlareSolverrHealth records at startup whether FlareSolverr is reachable,
// so a misconfigured FLARESOLVERR_URL shows up before the first scheduled run.
func logFlareSolverrHealth(log *slog.Logger, fs *client.FlareSolverrClient, cfg config.FlareSolverr) {
	attrs := []any{
		slog.String("endpoint", fs.Endpoint()),
		slog.String("session", cfg.Session),
		slog.Duration("max_timeout", cfg.MaxTimeout),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	version, err := fs.Health(ctx)
	if err != nil {
		log.Warn("flixpatrol pages will be fetched via flaresolverr, but it is not reachable", append(attrs, slog.Any("err", err))...)
		return
	}
	log.Info("flixpatrol pages will be fetched via flaresolverr", append(attrs, slog.String("version", version))...)
}
