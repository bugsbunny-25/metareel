# metareel — Architecture

## Overview

metareel is a Go backend service that scrapes daily Top 10 streaming rankings from [FlixPatrol](https://flixpatrol.com), enriches each title with TMDB and IMDb IDs (via the TMDB API and the JustWatch GraphQL API), persists the data in a SQLite database, and exposes a JSON REST API for querying rankings by country and streaming provider.

The HTTP contract is defined by embedded OpenAPI specs (`/openapi/public.yaml` and `/openapi/admin.yaml`) and served through Swagger UI (`/docs` and `/docs/admin`).

The service runs three concurrent subsystems in a single process:

| Subsystem | Role |
|---|---|
| **Echo HTTP server** | Serves the read API and admin endpoints |
| **Asynq worker** | Processes background scrape jobs pulled from Redis |
| **Asynq periodic task manager** | Reads cron schedules from the DB and enqueues jobs automatically |

---

## Component Architecture

```mermaid
graph TB
    subgraph process["metareel process"]
        subgraph http["HTTP layer"]
            Echo["Echo v5\nHTTP Server\n:8080"]
            MW["Middleware\n(Recover, RequestID,\nLogger, Gzip, CORS,\nBodyLimit)"]
            Handler["Handler\n(Top10, Admin)"]
            OpenAPI["OpenAPI module\n(spec + Swagger UI assets)"]
        end

        subgraph taskinfra["Task infrastructure"]
            Worker["Asynq Worker\n(goroutine)"]
            Scheduler["Asynq PeriodicTaskManager\n(goroutine, syncs DB every 1 min)"]
            AsynqClient["Asynq Client\n(enqueue)"]
        end

        subgraph services["Service layer"]
            Top10Svc["Top10ReadService"]
            AdminSvc["TaskScheduleAdminService"]
            FlixJob["FlixPatrolJob"]
        end

        subgraph clients["External clients"]
            Resty["Resty HTTP Client\n(timeout + retry)"]
            Colly["Colly Collector\n(async scraper)"]
            TMDB["TMDB Client\n(REST)"]
            JustWatch["JustWatch Client\n(GraphQL)"]
        end

        subgraph repo["Repository layer  (sqlc-generated)"]
            FlixRepo["FlixPatrolRepository"]
            ScheduleRepo["TaskScheduleRepository"]
            TaskRunRepo["TaskRunRepository"]
        end

        SQLite[("SQLite\n(WAL mode)\ndata/metareel.db")]
    end

    Redis[("Redis\n(Asynq queue)")]
    FlixPatrolSite["flixpatrol.com"]
    TMDBAPI["api.themoviedb.org"]
    JustWatchAPI["apis.justwatch.com\n(GraphQL)"]
    Client(["HTTP client\n(browser / curl)"])
    WebUI["web/dist\n(static SPA)"]

    Client -->|REST + docs request| Echo
    Echo --> MW --> Handler
    Echo -->|/docs + /openapi/*.yaml| OpenAPI
    Handler --> Top10Svc
    Handler --> AdminSvc
    AdminSvc --> AsynqClient
    AsynqClient -->|enqueue task| Redis

    Scheduler -->|reads schedules every 1 min| ScheduleRepo
    Scheduler -->|enqueues cron tasks| Redis
    Redis -->|dequeue| Worker
    Worker -->|route by task type| FlixJob

    Top10Svc --> FlixRepo
    FlixJob --> Colly
    FlixJob --> TMDB
    FlixJob --> JustWatch
    FlixJob --> FlixRepo
    FlixJob --> TaskRunRepo

    Colly -->|HTTP GET| FlixPatrolSite
    TMDB --> Resty -->|HTTPS| TMDBAPI
    JustWatch -->|GraphQL POST| JustWatchAPI

    FlixRepo --> SQLite
    ScheduleRepo --> SQLite
    TaskRunRepo --> SQLite

    Echo -.->|static fallback| WebUI
```

---

## Startup & Initialization Flow

```mermaid
sequenceDiagram
    participant main
    participant config
    participant server
    participant SQLite
    participant Goose
    participant Redis
    participant AsynqWorker
    participant AsynqScheduler
    participant Echo

    main->>config: Load() — parse env / .env file
    main->>server: Run(ctx, cfg, log)

    server->>SQLite: repository.Open(DSN)\nSetMaxOpenConns(1) — serialize writes
    server->>Goose: repository.Migrate() — apply pending goose migrations
    server->>Redis: asynq.NewClient(redisOpt)
    server->>Redis: asynq.NewServer(redisOpt, concurrency=10)

    Note over server: runtime.Build() wires all task dependencies
    server->>AsynqWorker: worker.Run(mux) — goroutine
    server->>AsynqScheduler: scheduler.NewPeriodicTaskManager()\n+ periodicManager.Run() — goroutine
    server->>Echo: echo.New() + router.Register() + httpServer.ListenAndServe() — goroutine

    Note over server: Block on SIGINT/SIGTERM or fatal goroutine error

    server->>AsynqScheduler: periodicManager.Shutdown()
    server->>AsynqWorker: worker.Shutdown()
    server->>Echo: httpServer.Shutdown(10s timeout)
    server->>SQLite: PRAGMA wal_checkpoint(FULL) then db.Close()
```

---

## Scrape Job Flow (the core data pipeline)

This is the primary data ingestion path. It can be triggered in two ways:
- **Automatically** by the Asynq PeriodicTaskManager on configured cron schedules
- **Manually** via `POST /api/v1/admin/task-schedules/:id/run-now`

```mermaid
sequenceDiagram
    participant Trigger as Trigger\n(Scheduler or Admin API)
    participant Redis
    participant Worker as Asynq Worker
    participant FPHandler as FlixPatrolHandler
    participant TaskRunRepo as TaskRunRepo\n(SQLite)
    participant FlixJob as FlixPatrolJob
    participant Colly as Colly Scraper
    participant FlixPatrol as flixpatrol.com
    participant JustWatch as JustWatch\n(GraphQL)
    participant TMDB as TMDB API
    participant FlixRepo as FlixPatrolRepo\n(SQLite)

    Trigger->>Redis: Enqueue task\n(FlixPatrolTop10Payload JSON)
    Redis->>Worker: Dequeue task
    Worker->>FPHandler: ProcessTask(ctx, task)

    FPHandler->>TaskRunRepo: CreateTaskRun (status=started)
    FPHandler->>TaskRunRepo: CreateTaskRunLog "task started"

    loop for each target (country × provider)
        FPHandler->>FlixJob: RunTargets(ctx, targets, date, opts)

        FlixJob->>Colly: Build URL:\nflixpatrol.com/top10/{provider}/{country}/{date}/
        Colly->>FlixPatrol: HTTP GET (with User-Agent, robots.txt check)
        FlixPatrol-->>Colly: HTML page
        Colly->>Colly: goquery parse:\nextract TOP 10 Movies + TV Shows tables
        Colly-->>FlixJob: []Entry{Rank, Name, Slug, Kind}

        loop for each Entry
            FlixJob->>FlixRepo: GetTitleBySlug(slug)

            alt title has both TMDB ID + IMDb ID
                FlixJob->>FlixRepo: UpsertRanking (skip ID lookup)
            else needs ID lookup
                FlixJob->>JustWatch: GetTitlesByPath(/{country}/{type}/{slug})
                JustWatch-->>FlixJob: ExternalIds{ImdbId, TmdbId}

                alt JustWatch path lookup failed
                    FlixJob->>JustWatch: GetTitlesByTopSearchPopular\n(slug, country, objectType, package)
                    JustWatch-->>FlixJob: ExternalIds
                end

                alt TMDB ID still missing && TMDB_API_KEY set
                    FlixJob->>TMDB: SearchMovie/SearchTV(name)
                    TMDB-->>FlixJob: tmdbID
                    FlixJob->>TMDB: MovieExternalIDs/TVExternalIDs(tmdbID)
                    TMDB-->>FlixJob: imdbID
                end

                FlixJob->>FlixRepo: UpsertTitle(slug, name, kind, tmdbID, imdbID)
                FlixJob->>FlixRepo: UpsertRanking(titleID, date, country, provider, rank)
            end
        end

        Note over FlixJob: Wait request_delay_seconds between targets
    end

    FPHandler->>TaskRunRepo: CompleteTaskRun (status=succeeded/failed)
    FPHandler->>TaskRunRepo: CreateTaskRunLog "task succeeded/error"
```

---

## Read API Flow

```mermaid
sequenceDiagram
    participant Client
    participant Echo
    participant Handler
    participant Top10Svc as Top10ReadService
    participant FlixRepo as FlixPatrolRepository
    participant SQLite

    Client->>Echo: GET /api/v1/top10/movies/{country}?date=YYYY-MM-DD
    Echo->>Handler: GetTop10MoviesAllProviders(c)
    Handler->>Handler: parseDate(date) — validate YYYY-MM-DD
    Handler->>Top10Svc: GetMoviesAllProviders(ctx, Top10Query)
    Top10Svc->>Top10Svc: validateQuery — check country ISO code
    Top10Svc->>FlixRepo: ListTop10AllProviders(date, country, "movies")
    FlixRepo->>SQLite: SELECT rankings JOIN titles WHERE ranked_on=? AND country=? AND category=?
    SQLite-->>FlixRepo: rows
    FlixRepo-->>Top10Svc: []Top10Item
    Top10Svc-->>Handler: *Top10Response{items: [{rank, provider, slug, name, tmdb_id, imdb_id}]}
    Handler-->>Client: 200 JSON
```

---

## Periodic Schedule Sync Flow

The PeriodicTaskManager polls the DB every minute and adjusts the Asynq cron schedule dynamically, without restarts.

```mermaid
sequenceDiagram
    participant AsynqScheduler as Asynq\nPeriodicTaskManager
    participant DBProvider as DBPeriodicTaskConfigProvider
    participant ScheduleRepo as TaskScheduleRepository
    participant SQLite
    participant Redis

    loop every 1 minute (SyncInterval)
        AsynqScheduler->>DBProvider: GetConfigs()
        DBProvider->>ScheduleRepo: ListFlixPatrolTop10TaskScheduleConfigs()
        ScheduleRepo->>SQLite: SELECT task_schedules JOIN targets JOIN run_times\nWHERE enabled = 1
        SQLite-->>ScheduleRepo: []FlixPatrolTaskSchedule
        ScheduleRepo-->>DBProvider: schedules with targets and HH:MM run times
        DBProvider->>DBProvider: Convert HH:MM → cron spec "MM HH * * *"
        DBProvider-->>AsynqScheduler: []*PeriodicTaskConfig{Cronspec, Task, MaxRetry, Queue}
        AsynqScheduler->>Redis: Register/update cron entries in Asynq
    end

    Note over AsynqScheduler,Redis: At cron fire time, Asynq enqueues the task
```

---

## Database Schema

Six tables in a single SQLite file (`data/metareel.db`, WAL mode).

```mermaid
erDiagram
    titles {
        INTEGER id PK
        TEXT slug UK "flixpatrol slug e.g. roommates-2026"
        TEXT name
        TEXT kind "movie | tv_show"
        TEXT tmdb_id
        TEXT imdb_id
        TEXT rt_url
        DATETIME created_at
        DATETIME updated_at
    }

    rankings {
        INTEGER id PK
        INTEGER title_id FK
        DATE ranked_on
        CHAR2 country "ISO 3166-1 alpha-2"
        TEXT streaming_provider "netflix | hbo-max | ..."
        TEXT category "movies | tv_shows | overall"
        INTEGER rank "1–10"
        INTEGER season_number
        DATETIME scraped_at
    }

    task_schedules {
        INTEGER id PK
        TEXT task_type "flixpatrol.top10.scrape"
        TEXT name UK
        BOOLEAN enabled
        INTEGER max_retries
        INTEGER request_delay_seconds
        BOOLEAN respect_robots
        TEXT user_agent
        DATETIME created_at
        DATETIME updated_at
    }

    task_schedule_targets {
        INTEGER id PK
        INTEGER schedule_id FK
        TEXT country_slug "flixpatrol slug e.g. united-states"
        TEXT provider_slug "netflix | hbo-max | ..."
    }

    task_schedule_run_times {
        INTEGER id PK
        INTEGER schedule_id FK
        TEXT run_time_utc "HH:MM"
    }

    task_runs {
        INTEGER id PK
        INTEGER schedule_id FK
        TEXT task_type
        TEXT asynq_task_id
        TEXT status "started | succeeded | failed"
        INTEGER retry_count
        INTEGER max_retry
        DATETIME started_at
        DATETIME finished_at
        TEXT error_message
    }

    task_run_logs {
        INTEGER id PK
        INTEGER run_id FK
        TEXT level "debug | info | warn | error"
        TEXT message
        DATETIME created_at
    }

    titles ||--o{ rankings : "has many"
    task_schedules ||--o{ task_schedule_targets : "has many"
    task_schedules ||--o{ task_schedule_run_times : "has many"
    task_schedules ||--o{ task_runs : "has many"
    task_runs ||--o{ task_run_logs : "has many"
```

Key constraints:
- `rankings` has a unique index on `(ranked_on, country, streaming_provider, category, rank)` — upserts replace stale title references cleanly.
- `titles` is keyed on `slug` (FlixPatrol URL slug). TMDB/IMDb IDs are filled in lazily and only overwrite `NULL`.
- `task_schedules.name` is unique — the admin API returns `409 Conflict` on duplicates.
- SQLite connection pool is capped at 1 (`SetMaxOpenConns(1)`) to serialize writes; WAL mode allows concurrent reads alongside the single writer.

---

## API Contract

The API reference is OpenAPI-first and is not duplicated in this architecture document.

- Public spec (YAML): `GET /openapi/public.yaml`
- Admin spec (YAML): `GET /openapi/admin.yaml`
- Public Swagger UI: `GET /docs`
- Admin Swagger UI: `GET /docs/admin`

Implementation note: both YAML specs and Swagger UI HTML are embedded from `internal/openapi/` into the server binary.

---

## Direct Dependencies

### Runtime dependencies (`go.mod` direct requires)

| Package | Version | Role |
|---|---|---|
| `github.com/labstack/echo/v5` | v5.1.0 | HTTP server and router |
| `github.com/hibiken/asynq` | v0.26.0 | Redis-backed background task queue and scheduler |
| `github.com/redis/go-redis/v9` | v9.18.0 | Redis client (used by `asynq` and the cache wrapper) |
| `github.com/gocolly/colly/v2` | v2.3.0 | Web scraping framework (async, robots.txt, rate limiting) |
| `github.com/PuerkitoBio/goquery` | v1.11.0 | jQuery-style HTML parsing (used inside Colly callbacks) |
| `github.com/go-resty/resty/v2` | v2.17.2 | HTTP client with retry and timeout for external REST APIs |
| `github.com/hasura/go-graphql-client` | v0.16.0 | GraphQL client for the JustWatch API |
| `modernc.org/sqlite` | v1.49.1 | Pure-Go SQLite driver (no CGO — enables fully static binary) |
| `github.com/pressly/goose/v3` | v3.27.0 | SQL migration runner (up/down, versioned files) |
| `github.com/caarlos0/env/v11` | v11.4.0 | Struct-tag-based environment variable parsing |
| `github.com/joho/godotenv` | v1.5.1 | Loads `.env` file into the process environment at startup |

### Dev-only dependencies (`tools/go.mod`)

| Package | Role |
|---|---|
| `github.com/air-verse/air` | Hot-reload server (`make dev-server-air`) |
| `github.com/go-delve/delve` | Local debugger backend for attach/launch workflows |
| `github.com/hibiken/asynqmon` | Web UI for inspecting Asynq queues (`make dev-asynqmon`) |
| `github.com/alicebob/miniredis/v2` | In-process Redis for local dev (`make dev-redis-up`) |

### Code generation tools (installed via `make tools`)

| Tool | Role |
|---|---|
| `github.com/sqlc-dev/sqlc` | Generates type-safe Go from `db/queries/*.sql` |
| `github.com/pressly/goose/v3` CLI | Runs migrations from the terminal (`make migrate-*`) |

### Standard library packages used

`context`, `database/sql`, `encoding/json`, `log/slog`, `net/http`, `os`, `os/signal`, `sync`, `syscall`, `time`

---

## Configuration Reference

All config is loaded from environment variables (with `.env` auto-loaded in development). See `.env.example` for the complete list.

| Variable | Default | Description |
|---|---|---|
| `APP_ENV` | `development` | Environment tag |
| `SERVER_HOST` | `0.0.0.0` | Bind address |
| `SERVER_PORT` | `8080` | Listen port |
| `SERVER_READ_TIMEOUT` | `15s` | Echo read timeout |
| `SERVER_WRITE_TIMEOUT` | `15s` | Echo write timeout |
| `SERVER_SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown window |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `DATABASE_URL` | `file:./data/metareel.db?...` | SQLite DSN (WAL + busy timeout + FK enforcement) |
| `DATABASE_MIGRATIONS_DIR` | `db/migrations` | Goose migrations directory |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `REDIS_PASSWORD` | _(empty)_ | Redis auth password |
| `REDIS_DB` | `0` | Redis logical DB index |
| `REDIS_DEFAULT_TTL` | `5m` | Default cache TTL |
| `ASYNQ_QUEUE` | `default` | Asynq queue name |
| `ASYNQ_CONCURRENCY` | `10` | Asynq worker goroutine count |
| `HTTP_CLIENT_TIMEOUT` | `20s` | Resty request timeout |
| `HTTP_CLIENT_RETRY_COUNT` | `2` | Resty auto-retry count on 5xx / network error |
| `HTTP_CLIENT_USER_AGENT` | `metareel/0.1` | User-Agent for TMDB + JustWatch requests |
| `SCRAPER_USER_AGENT` | `metareel-bot/0.1` | Colly User-Agent header |
| `SCRAPER_PARALLELISM` | `4` | Colly concurrent requests per domain |
| `SCRAPER_REQUEST_TIMEOUT` | `20s` | Colly per-request timeout |
| `UI_STATIC_DIR` | `web/dist` | Directory to serve the SPA from |
| `UI_SERVE_STATIC` | `true` | Enable / disable static file serving |
| `TMDB_API_KEY` | _(empty)_ | TMDB v3 API key — ID enrichment disabled when blank |

---

## Project Layout

```
metareel/
├── cmd/
│   └── server/
│       └── main.go               # Entrypoint — loads config, calls server.Run
├── internal/
│   ├── cache/
│   │   └── redis.go              # JSON get/set wrapper around go-redis
│   ├── client/
│   │   ├── resty.go              # Resty factory (timeout, retry, headers)
│   │   ├── tmdb.go               # TMDB REST client (search + external IDs)
│   │   ├── justwatch.go          # JustWatch GraphQL client (title ID lookup)
│   │   └── wikidata.go           # Wikidata lookup client
│   ├── config/
│   │   └── config.go             # Config struct + Load() via caarlos0/env
│   ├── constants/
│   │   └── iso3166.go            # ISO 3166-1 alpha-2 country code enum
│   ├── handler/
│   │   └── handler.go            # Echo handlers + request parsing + error mapping
│   ├── openapi/
│   │   ├── openapi.go            # Embedded OpenAPI YAML + Swagger UI helpers
│   │   ├── spec/                 # public/admin OpenAPI specs
│   │   └── swaggerui/            # Embedded Swagger UI HTML templates
│   ├── repository/
│   │   ├── repository.go         # DB open + Goose migration bootstrap
│   │   ├── flixpatrol_repo.go    # Title + ranking upserts
│   │   ├── task_schedule_repo.go # Schedule CRUD + target management
│   │   ├── task_run_repo.go      # Task run + log persistence
│   │   ├── top10_read_repo.go    # Top 10 read queries
│   │   └── sqlc/                 # sqlc-generated code (do not edit)
│   ├── router/
│   │   └── router.go             # Route registration + middleware + static SPA
│   ├── scheduler/
│   │   ├── manager.go            # Asynq PeriodicTaskManager factory
│   │   └── provider.go           # DBPeriodicTaskConfigProvider (DB → cron specs)
│   ├── scraper/
│   │   ├── colly.go              # Colly collector factory (rate limit, async)
│   │   └── flixpatrol/
│   │       ├── scraper.go        # Top10URL builder + ScrapeTop10 + HTML parser
│   │       └── country.go        # FlixPatrol slug ↔ ISO 3166-1 alpha-2 mapping
│   ├── server/
│   │   └── server.go             # Dependency wiring + graceful shutdown
│   ├── service/
│   │   ├── flixpatrol_job.go      # Core scrape + ID enrichment + DB persistence logic
│   │   ├── task_schedule_admin.go # Admin CRUD + manual task enqueue
│   │   ├── title_admin.go         # Title metadata patch/update service
│   │   └── top10_read.go          # Read-only Top 10 query service
│   └── tasks/
│       ├── definition.go          # Task type constants + payload structs + constructors
│       ├── handlers/
│       │   ├── flixpatrol.go      # Asynq handler: deserialise → run job → record run
│       │   └── mux.go             # Asynq ServeMux builder
│       └── runtime/
│           └── bootstrap.go       # Wires all task dependencies into a Bootstrap struct
├── db/
│   ├── migrations/
│   │   └── 000001_init.sql       # Full schema (titles, rankings, task_* tables)
│   └── queries/                  # sqlc input files (one per domain)
├── web/
│   └── dist/                     # Built SPA assets (not committed; mount or copy in)
├── tools/
│   ├── go.mod                    # Isolated dev tool deps (air, asynqmon, miniredis)
│   └── cmd/devredis/main.go      # In-process miniredis server for local dev
├── .air.run.toml                 # Air run profile (hot reload only)
├── .air.debug.toml               # Air debug profile (hot reload + Delve)
├── Dockerfile                    # Multi-stage; final stage: scratch + static binary
├── docker-compose.yml            # server + redis (128 MB / 160 MB memory caps)
├── Makefile                      # All developer workflows
└── sqlc.yaml                     # sqlc codegen config
```

---

## Key Design Decisions

**Pure-Go SQLite (`modernc.org/sqlite`)**
No CGO dependency means the final Docker image is built on `scratch` and is fully static (~15–20 MB). The trade-off is that `modernc` is slightly slower than `mattn/go-sqlite3` for high-throughput write workloads — acceptable here since rankings are written by background jobs, not hot paths.

**Single SQLite writer connection**
`SetMaxOpenConns(1)` serializes all writes. Combined with WAL mode (reads don't block the writer), this eliminates `SQLITE_BUSY` under concurrent API + worker load without needing a mutex.

**Asynq over a simpler cron package**
Asynq gives persistent, Redis-backed job state, retries with backoff, task deduplication, and the `PeriodicTaskManager` which can reload schedules from the DB every minute — meaning schedule changes take effect without a restart.

**JustWatch-first, TMDB-fallback ID enrichment**
JustWatch returns both TMDB and IMDb IDs in a single call. TMDB is only queried if JustWatch fails, reducing external API rate-limit exposure. Both are skipped entirely if a title already has both IDs in the DB.

**WAL checkpoint on shutdown**
`PRAGMA wal_checkpoint(FULL)` merges the write-ahead log into the main database file before closing. This ensures that any tool or backup script that opens only `metareel.db` (not the `-wal`/`-shm` side files) sees fully committed data.
