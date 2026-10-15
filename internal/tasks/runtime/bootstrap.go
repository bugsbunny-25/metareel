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
	"github.com/bugsbunny-25/metareel/internal/tasks"
	taskhandlers "github.com/bugsbunny-25/metareel/internal/tasks/handlers"
)

// Bootstrap wires task handlers and periodic config provider.
// Server startup can consume this as a single unit.
type Bootstrap struct {
	Mux            *asynq.ServeMux
	ConfigProvider asynq.PeriodicTaskConfigProvider
	// Shared with the HTTP side.
	Fetcher     flixpatrol.Fetcher
	Metadata    *service.TitleMetadataService
	Seasons     *service.SeasonMapper
	Mapping     *service.TitleMappingService
	Writer      *service.MappingWriter // every mapping change goes through it
	Maintenance *service.MaintenanceService
	Alerts      *service.AlertService
}

// Deps are built by the server and shared with the task handlers.
type Deps struct {
	Asynq   *asynq.Client // enqueues follow-up tasks
	Ratings *service.RatingsService
}

func Build(log *slog.Logger, cfg *config.Config, db *sql.DB, deps Deps) *Bootstrap {
	// Task schedule config source (DB-backed, shared by scheduler + handlers).
	taskScheduleRepo := repository.NewTaskScheduleRepository(db)
	flixRepo := repository.NewFlixPatrolRepository(db)
	importsRepo := repository.NewImportsRepository(db)
	opsRepo := repository.NewOpsRepository(db)
	httpClient := client.New(cfg.HTTP)

	var next flixpatrol.Fetcher = flixpatrol.LoggingFetcher{
		Next: flixpatrol.CollyFetcher{Base: scraper.NewCollector(cfg.Scraper)},
		Via:  "colly",
		Log:  log,
	}
	if fs := client.NewFlareSolverr(cfg.FlareSolverr); fs != nil {
		next = flixpatrol.LoggingFetcher{
			Next: flixpatrol.FlareSolverrFetcher{Client: fs, Log: log},
			Via:  "flaresolverr",
			Log:  log,
		}
		logFlareSolverrHealth(log, fs, cfg.FlareSolverr)
	} else {
		log.Info("flixpatrol pages will be fetched directly (FLARESOLVERR_URL not set)")
	}
	// One FlixPatrol fetch at a time across every job.
	fetcher := &flixpatrol.SerialFetcher{Next: next, MinInterval: cfg.Scraper.MinInterval}

	tmdbClient := client.NewTMDB(httpClient, cfg.TMDB.APIKey)
	justWatchClient := client.NewJustWatchClient(httpClient.GetClient())
	rtClient := client.NewRottenTomatoes(cfg.HTTP.Timeout)
	matcher := service.NewTitleMatcher(log, fetcher, tmdbClient, justWatchClient, rtClient)
	metadataRepo := repository.NewMetadataRepository(db)
	metadata := service.NewTitleMetadataService(log, metadataRepo, tmdbClient, client.NewWikidataSPARQL(cfg.HTTP.Timeout))
	downloader := client.NewDownloader(cfg.Imports.DownloadTimeout, "")
	alerts := service.NewAlertService(log, opsRepo, cfg.Alerts.WebhookURL, cfg.Alerts.Format, cfg.Alerts.Cooldown)
	maintenance := service.NewMaintenanceService(log, opsRepo, taskScheduleRepo, alerts, cfg.Maintenance.RunRetentionDays, cfg.Alerts.StaleChartHours)

	seasons := service.NewSeasonMapper(log, importsRepo)
	mappingRepo := repository.NewMappingRepository(db)
	writer := service.NewMappingWriter(log, mappingRepo)
	if deps.Asynq != nil {
		writer.WithRefreshQueue(mappingRefreshQueue(deps.Asynq, cfg.Asynq.Queue))
	}
	mapping := service.NewTitleMappingService(log, mappingRepo, writer, metadataRepo, metadata, seasons)
	netflix := service.NewNetflixImporter(log, downloader, importsRepo, writer, flixRepo, taskScheduleRepo, matcher, metadata, seasons, cfg.Imports.NetflixCountries).WithMapping(mapping)
	if cfg.Imports.NetflixSearch {
		netflix.WithNetflixSearch(client.NewNetflixTop10Search(cfg.HTTP.Timeout, cfg.Imports.NetflixSearchQueryID, cfg.Imports.NetflixSearchQueryVer))
	}

	runner := taskhandlers.NewRunner(log, taskScheduleRepo, alerts)
	flixHandler := taskhandlers.NewFlixPatrolHandler(log, runner,
		service.NewFlixPatrolJob(log, fetcher, flixRepo), seasons, deps.Asynq, cfg.Asynq.Queue)

	jobs := taskhandlers.NewJobHandler(runner).
		Handle(tasks.TypeTitlesEnrich, taskhandlers.EnrichJob(service.NewTitleEnricher(log, flixRepo, writer, matcher, metadata).WithMapping(mapping), seasons, cfg.Scraper.UserAgent)).
		Handle(tasks.TypeTitlesMetadata, taskhandlers.MetadataJob(metadata, seasons)).
		Handle(tasks.TypeNetflixTop10Import, taskhandlers.NetflixImportJob(netflix)).
		Handle(tasks.TypeNetflixTitlesMatch, taskhandlers.NetflixMatchJob(netflix)).
		Handle(tasks.TypeIMDbRatingsImport, taskhandlers.IMDbImportJob(service.NewIMDbImporter(log, downloader, importsRepo, cfg.Imports.IMDbScope == "all"))).
		Handle(tasks.TypeMappingRefresh, taskhandlers.MappingRefreshJob(mapping)).
		Handle(tasks.TypeMaintenance, taskhandlers.MaintenanceJob(maintenance))
	if deps.Ratings != nil {
		jobs.Handle(tasks.TypeRatingsPrewarm, taskhandlers.RatingsPrewarmJob(deps.Ratings, flixRepo))
	}

	if alerts.Enabled() {
		log.Info("alerts enabled", slog.String("format", cfg.Alerts.Format))
	}
	return &Bootstrap{
		Mux:            taskhandlers.NewServeMux(flixHandler, jobs),
		ConfigProvider: scheduler.NewDBPeriodicTaskConfigProvider(taskScheduleRepo, cfg.Asynq.Queue, log),
		Fetcher:        fetcher,
		Metadata:       metadata,
		Seasons:        seasons,
		Mapping:        mapping,
		Writer:         writer,
		Maintenance:    maintenance,
		Alerts:         alerts,
	}
}

// mappingRefreshQueue enqueues mapping.refresh for the follow-up of mapping
// changes. Not unique: a change made while a refresh runs gets its own run
// (cheap: details fetched within a day are skipped).
func mappingRefreshQueue(c *asynq.Client, queue string) func(ctx context.Context, r service.MappingRefresh) error {
	return func(ctx context.Context, r service.MappingRefresh) error {
		p := tasks.JobPayload{Reason: r.Reason, NetflixTitleIDs: r.NetflixIDs}
		for _, ref := range r.Refs {
			p.TmdbRefs = append(p.TmdbRefs, ref.String())
		}
		task, err := tasks.NewJobTask(tasks.TypeMappingRefresh, p)
		if err != nil {
			return err
		}
		opts := []asynq.Option{asynq.MaxRetry(2), asynq.Timeout(tasks.Timeout(tasks.TypeMappingRefresh))}
		if queue != "" {
			opts = append(opts, asynq.Queue(queue))
		}
		_, err = c.EnqueueContext(ctx, task, opts...)
		return err
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
