package runtime

import (
	"database/sql"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/bugsbunny-25/metareel/internal/client"
	"github.com/bugsbunny-25/metareel/internal/config"
	"github.com/bugsbunny-25/metareel/internal/repository"
	"github.com/bugsbunny-25/metareel/internal/scheduler"
	"github.com/bugsbunny-25/metareel/internal/scraper"
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
	collector := scraper.NewCollector(cfg.Scraper)
	tmdbClient := client.NewTMDB(httpClient, cfg.TMDB.APIKey)
	justWatchClient := client.NewJustWatchClient(httpClient.GetClient())
	wikidataClient := client.NewWikidata(httpClient)
	flixJob := service.NewFlixPatrolJob(log, collector, flixRepo, tmdbClient, justWatchClient, wikidataClient)
	flixHandler := taskhandlers.NewFlixPatrolHandler(log, taskScheduleRepo, flixJob)

	return &Bootstrap{
		Mux:            taskhandlers.NewServeMux(flixHandler),
		ConfigProvider: scheduler.NewDBPeriodicTaskConfigProvider(taskScheduleRepo, cfg.Asynq.Queue),
	}
}
