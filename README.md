# metareel

[![CI](https://github.com/bugsbunny-25/metareel/actions/workflows/ci.yml/badge.svg)](https://github.com/bugsbunny-25/metareel/actions/workflows/ci.yml)
[![CodeQL](https://github.com/bugsbunny-25/metareel/actions/workflows/codeql.yml/badge.svg)](https://github.com/bugsbunny-25/metareel/actions/workflows/codeql.yml)
[![Container](https://github.com/bugsbunny-25/metareel/actions/workflows/container.yml/badge.svg)](https://github.com/bugsbunny-25/metareel/actions/workflows/container.yml)

A small, batteries-included Go REST API server that calls downstream APIs and
scrapes web pages, backed by SQLite for persistence and Redis for caching.

## Stack

| Concern        | Library                                  |
| -------------- | ---------------------------------------- |
| HTTP server    | `labstack/echo/v5`                       |
| Job queue      | `hibiken/asynq` (Redis)                  |
| HTTP client    | `go-resty/resty/v2`                      |
| Web scraping   | `gocolly/colly/v2`                       |
| Database       | `modernc.org/sqlite` (pure-Go, no CGO)   |
| Migrations     | `pressly/goose/v3`                       |
| Query codegen  | `sqlc-dev/sqlc`                          |
| Config         | `caarlos0/env/v11` + `joho/godotenv`     |
| Logging        | `log/slog` (stdlib)                      |

Because `modernc/sqlite` is pure-Go, the final binary is fully static and
runs on `scratch` (~15–20 MB image, ~20–50 MB RSS at idle).

## Project layout

```
.
├── cmd/
│   └── server/              # main.go – composition root
├── internal/
│   ├── client/              # Resty HTTP client factory
│   ├── config/              # env-driven config
│   ├── handler/             # Echo HTTP handlers
│   ├── repository/          # DB handle + migrations bootstrapping
│   │   └── sqlc/            # sqlc-generated code (after `make sqlc`)
│   ├── router/              # route registration, static UI fallback
│   ├── scraper/             # Colly collector factory
│   ├── server/              # lifecycle: deps + graceful shutdown
│   └── service/             # business logic (fetcher)
├── db/
│   ├── migrations/          # goose up/down SQL files
│   └── queries/             # sqlc input queries
├── web/
│   └── dist/                # drop your built SPA here (served at /)
├── Dockerfile               # multi-stage, scratch-based
├── docker-compose.yml       # server + redis
├── Makefile
├── sqlc.yaml
└── .env.example
```

## Quick start

### Local

```bash
cp .env.example .env
make tools          # installs sqlc + migrate CLIs (one-time)
make sqlc           # generate internal/repository/sqlc from db/queries
make tidy
make run            # starts server on :8080 (requires local redis on :6379)
```

### Local dev tools submodule (isolated, no global installs)

```bash
make dev-up         # run profile: devredis + asynqmon + API hot-reload (no debugger)
make dev-up-debug   # debug profile: hot-reload + Delve on 127.0.0.1:2345
make dev-redis-up   # starts Go-based devredis on 127.0.0.1:6379
make dev-server-air-run   # backend-only run profile
make dev-server-air-debug # backend-only debug profile
make dev-asynqmon   # opens Asynq monitor on http://localhost:8081
make dev-doctor     # validates local toolchain, ports, and service state
make dev-down       # stops air + debugger + API + devredis + asynqmon services
make dev-redis-down # stops Go-based devredis
```

`make dev-up` is the default for everyday local development. Use `make dev-up-debug` when you need breakpoints.

For editor debugging, use the checked-in configuration:

- `Attach: dev-up-debug (Air + Delve)` to attach to `make dev-up-debug`
- `Launch: API (no Air)` for direct debugging without hot-reload

Dev-only dependencies are pinned in `tools/go.mod` so root `go.mod` stays production-focused. Local dev binaries are installed to `.tools/bin`.

`devredis` uses `miniredis` (pure Go) for local development/test convenience. For production-like Asynq behavior, use a real Redis server.

### Troubleshooting local run/debug

| Problem | Quick fix |
|---|---|
| `dlv command not available` in editor | Run `make dev-doctor` (installs `.tools/bin/dlv`) and reload the editor window |
| Breakpoints do not hit | Start `make dev-up-debug`, then use `Attach: dev-up-debug (Air + Delve)` |
| API endpoint unavailable while debugging | Ensure you are in debug profile with `--continue` (`make dev-up-debug`), then re-trigger request |
| Port already in use (`7080` or `2345`) | Run `make dev-down`, then `make dev-doctor` to re-check ports |
| Air output is noisy | Use the new run/debug profiles (`.air.run.toml` / `.air.debug.toml`) with scoped watchers |

### Docker

```bash
make up             # builds the image and brings up server + redis
curl localhost:8080/healthz
make logs
make down
```

## CI/CD (GitHub Actions)

### Workflows

- `ci.yml`
  - `go-quality`: `go mod tidy` check, `make vet`, `make build`, race tests with coverage, coverage artifact
  - `integration`: Redis service container + DB env wiring + test suite
  - `govulncheck`: vulnerability scan with `govulncheck ./...`
- `codeql.yml`
  - CodeQL analysis for Go on PRs, `main`, and weekly schedule
- `container.yml`
  - PR: Docker build + Trivy scan + SBOM artifact
  - `main`/tags: Docker build + push to GHCR + Trivy scan + SBOM artifact
- `staging-deploy.yml`
  - Manual (`workflow_dispatch`) deploy scaffold with smoke-test contract (`/healthz`, `/api/v1/test`)

### Coverage reporting

- Coverage is generated in CI as `coverage.out`.
- CI uploads `coverage.out` as an artifact and writes the total percentage into the GitHub job summary.

### Run CI checks locally

```bash
# Mirrors the go-quality job (tidy check, vet, build, race tests + coverage)
make ci-local

# Mirrors the integration job (expects Redis reachable at REDIS_ADDR, default 127.0.0.1:6379)
make integration-local

# Mirrors the govulncheck job
make security-local

# Optional: full local CodeQL (requires codeql CLI + packs)
make codeql-local

# Generate browsable local artifacts (coverage HTML, junit xml, test json, govulncheck output)
make reports-local
make reports-open
```

### Required branch protection checks (recommended)

- `go-quality`
- `integration`
- `govulncheck`
- `analyze` (CodeQL)
- `build-scan-publish` (container + Trivy)

### Operational notes

- Container images are built from `Dockerfile` and use `linux/amd64`.
- Published images use immutable SHA-based tags; tag pushes also publish version tags (`v*`).
- Trivy fails the workflow on `HIGH`/`CRITICAL` vulnerabilities.

## Endpoints

### API contract and docs

- **Public Swagger UI**: `GET /docs` (spec: `GET /openapi/public.yaml`)
- **Admin Swagger UI**: `GET /docs/admin` (spec: `GET /openapi/admin.yaml`)

### HTTP endpoints (high level)

| Method | Path                                        | Description                                 |
| ------ | ------------------------------------------- | ------------------------------------------- |
| GET    | `/healthz`                                  | Liveness |
| GET    | `/readyz`                                   | Readiness: DB + Redis (503 if down), FlareSolverr reported as `degraded` |
| GET    | `/docs`, `/docs/admin`                      | Swagger UI (public + admin)                 |
| GET    | `/openapi/public.yaml`, `/openapi/admin.yaml` | OpenAPI specs (YAML)                      |
| GET    | `/api/v1/top10/{movies\|tv-shows}/:country[/:provider]` | Top 10 charts (`date` optional = latest; entries carry `previous_rank`, `days_in_top10`, and for TV `season_number` + `season_source` when known) |
| GET    | `/api/v1/top10/:country`                    | Every provider's movie + TV chart of a country in one call |
| GET    | `/api/v1/top10/{movies\|tv-shows}/:country/:provider/history` | One chart over a range (`from`, `to`; ≤ 92 days) |
| GET    | `/api/v1/top10/global/:provider`            | A provider's titles across all countries by points (`date`, `category`, `kind`) |
| GET    | `/api/v1/charts`, `/api/v1/charts/:country/:provider/dates` | Stored charts with date ranges and freshness; stored dates |
| GET    | `/api/v1/changes?since=`                    | Charts stored / re-scraped since a time (poll this) |
| GET    | `/api/v1/movers`                            | Climbers, fallers, debuts, re-entries, exits (`country`, `provider`, `date`) |
| GET    | `/api/v1/leaderboards`                      | Titles by chart points (`period=day\|week\|month\|custom`, filters) |
| GET    | `/api/v1/export/rankings`                   | Chart entries as CSV / NDJSON (≤ 366 days) |
| GET    | `/api/v1/countries`, `/api/v1/providers`    | Supported countries; providers with stored charts |
| GET    | `/api/v1/titles`, `/api/v1/titles/:id`      | Scraped titles (paged; `match_status` filter); a single mapped title carries its TMDB title's `ids` |
| GET    | `/api/v1/titles/lookup?imdb=\|tmdb=\|slug=\|netflix=` | Find a title by any ID (`netflix` = the number in netflix.com/title/<id>) |
| GET    | `/api/v1/titles/tmdb/:kind/:id`             | Everything about a title: TMDB metadata, chart stats, current positions, ratings, where to watch, Netflix numbers (`include=`), and `ids` — every known ID (IMDb, Wikidata, Rotten Tomatoes, Netflix, JustWatch, FlixPatrol slugs, Netflix Top 10 names) |
| GET    | `/api/v1/titles/tmdb/:kind/:id/stats`       | Chart performance: points, peak, days at #1, debut, streak, spread across countries |
| GET    | `/api/v1/titles/tmdb/:kind/:id/availability` | Watch providers per country (TMDB, data by JustWatch) |
| GET    | `/api/v1/titles/tmdb/:kind/:id/netflix`     | Weeks in Netflix's official Top 10, with a per-season summary (`seasons`) for TV |
| GET    | `/api/v1/titles/tmdb/:kind/:id/rankings`    | Ranking history by TMDB ID (`kind` = movie or tv; `country`, `from`, `to` optional) |
| GET    | `/api/v1/titles/tmdb/rankings?ids=movie:425,tv:1` | Batch ranking history (max 50 ids)    |
| GET    | `/api/v1/titles/tmdb/:kind/:id/ratings`     | IMDb + Rotten Tomatoes ratings by TMDB ID, cached for `RATINGS_TTL` (`?refresh=true` to force); IMDb's own dataset wins for IMDb |
| GET    | `/api/v1/titles/tmdb/ratings?ids=`          | Batch ratings (max 50; refreshes up to 10 uncached) |
| GET    | `/api/v1/netflix/top10[/:country]`, `/api/v1/netflix/most-popular` | Netflix's official weekly Top 10 and most-popular lists (rows carry `season_number` / `season_source`) |
| GET    | `/api/v1/analytics/{decay,country-similarity,release-lag,ratings-vs-popularity,genres,netflix-calibration}` | Analytics over a range (`from`, `to`, `country`, `provider`, `category`, `kind`) |
| GET    | `/api/v1/titles/new/:country/:service`      | Titles added to a service per day, live from JustWatch (`from`, `to` default last 7 days; `type`) |
| GET    | `/api/v1/titles/upcoming/:country/:service` | Titles announced for a service, live from JustWatch (`from`, `to`, `type`) |
| GET    | `/api/v1/services/:country`                 | JustWatch service codes in a country (for `:service`) |
| GET/POST/PATCH/DELETE | `/api/v1/admin/...`           | Admin API: schedules, runs, stats, API keys, settings, title corrections (`PATCH /api/v1/admin/titles/:id`, `POST .../rematch`), `POST /jobs/:type/run`, `POST /backfill`, `GET /data-quality`, `GET /task-types`, and admin copies of the read routes above |
| GET/PATCH/POST | `/api/v1/admin/netflix/titles[/:id[/rematch\|/candidates]]` | Netflix official Top 10 titles: list (`q`, `kind`, `match_status`, `missing=tmdb\|season`, `flag=cross_kind\|linked`, `sort`), detail with season titles, weeks and linked FlixPatrol titles, fix by hand (`tmdb_kind` + `tmdb_id`, or an IMDb / Wikidata ID resolved to TMDB), rematch, candidate search |
| GET/POST | `/api/v1/admin/mappings[/sync\|/:kind/:id[/remap]]` | The TMDB-keyed mapping: every TMDB title with all its IDs and the FlixPatrol / Netflix titles mapped to it (`q`, `kind`, `flag=conflict\|cross_kind\|both\|netflix_only\|flixpatrol_only\|manual`); `sync` recomputes links now; `remap` moves everything on a TMDB title to another (by TMDB, IMDb or Wikidata ID) |
| GET    | `/api/v1/admin/mapping-refs?tmdb_kind=movie\|tv&mapped=true` | The mapping reference (view `mapping_refs`): one row per chart title with its TMDB title and every ID (IMDb, Wikidata, RT, Metacritic, Letterboxd, JustWatch, Netflix), FlixPatrol slug / kind and Netflix title / kind |
| GET    | `/api/v1/admin/mapping-events?subject=flixpatrol:12\|netflix:34&tmdb=movie:550&limit=` | History of mapping changes, newest first: action, TMDB title before / after, source and who made it (job, `mapping.sync`, `admin`) |
| PATCH  | `/api/v1/admin/netflix/seasons/:id`         | Set a Netflix season title's season number by hand (`{"season_number": 2}`) or hand it back (`{"auto": true}`) |
| GET    | `/` (and any unmatched path)                | Serves `web/dist/` SPA if present           |

GET responses under `/api/v1` carry an `ETag`; send it back as
`If-None-Match` to get `304 Not Modified` when nothing changed.

## Jobs

Every job is a schedule (admin UI → Schedules; run times in UTC) and can be run
on demand from Data health or `POST /api/v1/admin/jobs/:type/run`.

| Job | Default | What it does |
|---|---|---|
| `flixpatrol.top10.scrape` | your schedules | Scrapes each target's current chart, fills gaps from the last `backfill_days`, re-checks hourly for `recheck_hours` when FlixPatrol has not published the day's chart yet. A failed chart page does not stop the run (status `partial`); retries only redo failed pages. New titles are queued for matching right away. |
| `titles.enrich` | 03:00 | Matches titles to TMDB / IMDb (FlixPatrol title page → JustWatch → TMDB), retrying unmatched ones after 1, 3, 7, then every 30 days; finds RT slugs via Wikidata SPARQL, then RT search. Titles fixed in the admin UI (`manual`) are never re-matched, but their TMDB titles get missing RT slugs. |
| `titles.metadata` | 04:00 | TMDB details + watch providers (charting titles every 3 days, others monthly) and Wikidata IDs (RT, Metacritic, Letterboxd) of TMDB titles. Needs `TMDB_API_KEY`. |
| `ratings.prewarm` | 15:30 | Refreshes expired ratings of titles charting in the last 3 days. |
| `netflix.top10.import` | 21:00 | Netflix's official weekly Top 10 (global, per country, most popular); skipped when unchanged. Matches up to 400 new titles (see [TMDB mapping](#tmdb-mapping)) and maps season titles to season numbers. |
| `netflix.titles.match` | 09:00 | Works through the backlog of unmatched Netflix titles (retrying after 1, 3, 7, then every 30 days; `manual` titles are never touched), looks up Netflix IDs of up to 200 more titles (fixing earlier name matches), then remaps seasons. Also queued by "Match again" in the admin UI. |
| `imdb.ratings.import` | 06:00 | IMDb's daily ratings dataset for the IMDb IDs metareel knows. |
| `mapping.refresh` | — (queued) | Follows up every mapping change (matches, fixes, remaps, shared mappings): TMDB details and Wikidata IDs of the TMDB titles now mapped to, season numbers. Run by hand, it first recomputes every linked title's mapping. |
| `maintenance` | 02:00 | Run retention (`RUN_RETENTION_DAYS`), stale-chart alerts, SQLite optimize. |

### TMDB mapping

One mapping keyed by TMDB: TMDB titles are the hub, and FlixPatrol titles and
Netflix official Top 10 titles both point at one (`kind` / `tmdb_kind` is the
TMDB namespace, so a stand-up special in Netflix's TV list can be a TMDB
movie). Each TMDB title keeps every ID — IMDb, Wikidata, Rotten Tomatoes,
Metacritic, Letterboxd, Netflix (Wikidata P1874 or Tudum's Top 10 search),
JustWatch — and the FlixPatrol slugs and Netflix Top 10 names mapped to it
(admin UI → Mapping → TMDB mappings; public `ids` on `/titles/tmdb/:kind/:id`
and `/titles/:id`; `ids` on the admin Netflix title too). These IDs live only
on the TMDB title, so every title mapped to it shows the same ones — the
FlixPatrol, Netflix and TMDB mapping drawers show the same ID list and the
same Details / Top 10 history / Netflix history / Ratings tabs — and moving a
title to another TMDB title shows that title's. Each ID records where it came
from (`ids.sources`) and is only replaced by the same or a stronger source:
entered by hand > TMDB > Wikidata > an automatic match > a Netflix title
mapped to it (its Tudum Netflix ID fills the TMDB title's while no other
source has one). The whole mapping, one row per chart title with every ID,
is the database view `mapping_refs` (`GET /api/v1/admin/mapping-refs`, or
"Export reference" on the TMDB mappings tab). An IMDb, Wikidata or RT ID you enter on a title is stored as its TMDB
title's, set by hand (clear it to hand it back). Data health lists titles
whose deciding match or fix came with another IMDb ID than their TMDB
title's, and Netflix titles whose own Netflix ID (Tudum) differs from their
TMDB title's — usually a match to another title of the same name ("check
mapping"), unless it is a special or season of a show whose own Netflix title
has that ID ("same show").

A title's mapping is computed, never copied: matching, Netflix IDs and fixes
by hand store **evidence** (what each source says the title is), and the
mapping follows from the title's own evidence and its linked titles' (see
Links). Taking a match or fix back therefore also takes back what linked
titles had from it. "Match again" lifts a fix by hand: the title's own
earlier match applies again (or the fix stays as its match if it has none)
until matching finds another; "Clear and match again" forgets it.

Every mapping change is written by one writer and recorded with who made it
(a job, link sharing or the admin UI): see the History section of the title
and mapping drawers, or `GET /api/v1/admin/mapping-events`. The follow-up
(TMDB details, IDs, season numbers) runs in a `mapping.refresh` job, so admin
fixes return quickly.

- **Links.** A Netflix title and a FlixPatrol title with the same normalized
  name (with or without ": Season N") in the same Netflix list (Films / TV)
  that were on Netflix's charts in the same or an adjacent week are linked.
  Linked titles share one mapping; each title weighs itself and the titles
  directly linked to it (only those linked to nothing else): a title fixed by hand wins, then a Netflix title
  mapped by its Netflix ID, then FlixPatrol's match (made with the title
  page's year), then Netflix's. Linked titles that disagree at the deciding
  level (two hand fixes, or two same-name FlixPatrol titles such as a show and
  its revival) are a conflict left for review (Mapping → TMDB mappings → Conflicts filter). Links are refreshed by
  `titles.enrich`, `netflix.top10.import` and `netflix.titles.match` (or
  "Sync links now").
- **Netflix titles** are matched by: a linked or same-name FlixPatrol title;
  their **Netflix ID** — Tudum's Top 10 search (`netflix.com/tudum/top10`)
  gives each charted title's ID (a season's for TV, whose public title page
  redirects to the show), accepted when the name (a season title for TV),
  list and a charted week agree, then mapped to TMDB through
  `tmdb_titles.netflix_id` or Wikidata; Wikidata items with a Netflix ID whose
  English label is the name (the other TMDB kind only if released in the year
  it first charted or the one before; never an item with a Netflix ID other
  than the title's own — such earlier matches are redone by the next Netflix
  matching run); JustWatch's US Netflix catalogue as
  either kind; a single exact name in all of JustWatch US.
- **Fixing.** Fix a single title in its drawer (TMDB kind + ID, or an IMDb /
  Wikidata ID resolved through TMDB's `/find`, then Wikidata), or move
  everything on a wrong TMDB title at once from Mapping → TMDB mappings → Move all.

### Seasons

Netflix lists TV shows per season (`season_title`, e.g. "Wednesday: Season 2").
Each season title is mapped to a TMDB season number: from the title itself
("Season 2", "S3", "Limited Series"), a TMDB season of that name, "Part /
Volume / Chapter N", the TMDB season that aired just before it first charted
(or before the date in a dated title such as "Raw: September 28, 2026"), or
the only season of a single-season show. Unresolved ones show up under
Netflix titles → "Season without a number" for fixing by hand.

FlixPatrol charts name only the show, so TV rankings get a season too
(`season_source`): `chart` when the chart name says so, `netflix` on Netflix
charts from the best-ranked season of the show in Netflix's official Top 10
that week (the country's list, else global), otherwise `tmdb_air_date` (the
latest season aired by that date). They are refreshed after scrapes, matching,
TMDB metadata and Netflix jobs.

Only one FlixPatrol page is fetched at a time across all jobs
(`SCRAPER_MIN_INTERVAL` apart). A schedule cannot be queued twice (409).
Set `ALERT_WEBHOOK_URL` to get failed runs and stale charts as JSON, Slack,
Discord or ntfy messages.

### Data sources and terms

- **FlixPatrol** (charts, title pages) through your FlareSolverr.
- **Netflix Top 10** (`netflix.com/tudum/top10` TSV downloads), and the Top
  10 page's own search for Netflix IDs (an undocumented persisted GraphQL
  query, sent anonymously; `NETFLIX_TOP10_SEARCH=false` turns it off).
- **IMDb non-commercial datasets** (`title.ratings.tsv.gz`) — personal and
  non-commercial use only; see https://developer.imdb.com/non-commercial-datasets/.
- **TMDB** (search, details, watch providers — watch provider data is by
  JustWatch and must be attributed), **Wikidata** (SPARQL, CC0),
  **JustWatch** GraphQL, **Rotten Tomatoes** search index, **MDBList** (optional).

## Public API keys

Every `/api/v1` route outside `/api/v1/admin` needs an API key, sent as
`X-API-Key: <key>` (or `Authorization: Bearer <key>`). Health checks and
`/docs` stay open.

- Create, rotate, rename and revoke keys in the admin console under
  **Settings & API keys**. A key is shown once; only a PBKDF2-SHA256 digest of it is stored.
  Rotating replaces the secret immediately.
- Requests are rate limited per key (default 300/min, 0 = off); over the
  limit the API answers 429 with `Retry-After`.
- Key enforcement can be switched off in the same page (e.g. local dev);
  requests are then rate limited per client IP.
- The admin console and `/api/v1/admin` are **not** authenticated. Expose only
  the public API to the internet, e.g. block `/` and `/api/v1/admin` at the
  reverse proxy.

```bash
curl -H "X-API-Key: mr_..." http://localhost:8080/api/v1/top10/movies/US
```

## Adding a future UI

Build your SPA (Vite, Next export, SvelteKit static, etc.) into static files
and either:

1. **Copy into `web/dist/`** in the repo – the Dockerfile doesn't bake UI
   files in, so mount them via the `./web/dist:/app/web/dist:ro` volume
   commented out in `docker-compose.yml`, or
2. **Bake them into the image** – uncomment a `COPY web/dist /app/web/dist`
   line in the Dockerfile runtime stage.

`internal/router/router.go` already serves `/` from `UI_STATIC_DIR` and falls
back to `index.html` for unknown paths (standard SPA history routing).

## Migrations

```bash
make migrate-new MIGRATION_NAME=add_users   # scaffold new up/down pair
make migrate-up                             # apply pending
make migrate-down                           # revert one (goose \"down\")
make migrate-status
```

The server also runs pending migrations automatically at startup via
`repository.Migrate`.

## sqlc

Queries live in `db/queries/*.sql`. Regenerate typed Go code after any
changes:

```bash
make sqlc
```

Then import `github.com/bugsbunny-25/metareel/internal/repository/sqlc` and use
`sqlc.New(db)` in your services.

## Configuration

All config is env-driven – see `.env.example` for the full list. A `.env`
file in the working directory is auto-loaded in development.

## Image size / memory

* Final image: `scratch` + static Go binary + `db/migrations` + CA certs +
  tzdata ≈ **15–20 MB**.
* `docker-compose.yml` caps the server at **128 MB** and Redis at **160 MB**
  with an LRU eviction policy; tune via the `deploy.resources.limits` blocks.
