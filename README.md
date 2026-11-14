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
  - `main`: Docker build + push to GHCR as `main` and `sha-<commit>` + Trivy scan + SBOM artifact,
    then `cleanup-main-images` deletes every older main image (only once the whole build passed),
    so GHCR holds a single main image. Release images are never touched.
- `release.yml`
  - Creates a GitHub release, pre-releases included: runs `ci.yml`, builds and pushes the image
    tagged with the version via `container.yml`, then creates the tag and release (generated
    notes, image digest, SBOM attached). Nothing is tagged if CI or the image fails.
  - Start it from Actions → release → Run workflow (`version`, `prerelease`, `ref`), or push a
    `v*` tag / create a release in the UI. A version with a `-suffix` (e.g. `v0.2.0-alpha`) is
    always a pre-release.
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
- Main images are tagged `main` + `sha-<commit>` and only the latest survives; release images are
  tagged with their version (`v*`) and kept. Cleanup uses the workflow's `GITHUB_TOKEN`, so the
  package's *Manage Actions access* must give this repository the Admin role.
- Tags and releases created by `release.yml` don't trigger other workflows (a `GITHUB_TOKEN`
  limit), which is why it calls `ci.yml` and `container.yml` itself.
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
| POST   | `/api/v1/auth/login`, `/api/v1/auth/logout` | Sign a user in (session cookie + token) or out ([access](#public-api-access)) |
| GET    | `/api/v1/auth/me`                           | Who the request is authenticated as (user or API key) |
| GET    | `/api/v1/auth/session`                      | For the console: whether admin sign-in is required and who is signed in |
| GET    | `/api/v1/top10/{movies\|tv-shows}/:country[/:provider]` | Top 10 charts (`date` optional = latest; entries carry `previous_rank`, `days_in_top10`, the TMDB `poster_path` once the title has metadata, and for TV `season_number` + `season_source` when known) |
| GET    | `/api/v1/top10/:country`                    | Every provider's movie + TV chart of a country in one call |
| GET    | `/api/v1/top10/{movies\|tv-shows}/:country/:provider/history` | One chart over a range (`from`, `to`; ≤ 92 days) |
| GET    | `/api/v1/top10/global/:provider`            | A provider's titles across all countries by points (`date`, `category`, `kind`) |
| GET    | `/api/v1/charts`, `/api/v1/charts/:country/:provider/dates` | Stored charts with date ranges and freshness; stored dates |
| GET    | `/api/v1/changes?since=`                    | Charts stored / re-scraped since a time (poll this) |
| GET    | `/api/v1/movers`                            | Climbers, fallers, debuts, re-entries, exits (`country`, `provider`, `date`). Like every chart-derived list (history, breakouts, leaderboards, analytics items), each title carries `poster_path` (TMDB path, null until it has metadata) |
| GET    | `/api/v1/discover/:country/hidden-gems`     | Highly rated titles (IMDb ≥ `min_rating` 7.5 from ≥ `min_votes` 5,000) streamable in the country that weren't in its Top 10 over `days` (90), ranked by a vote-weighted rating (`kind`, `provider_id`, `limit`) |
| GET    | `/api/v1/discover/:country/elsewhere`       | Titles in other countries' Top 10s over `days` (7) with no subscription / free / ads offer in the country, with where they chart and rent / buy offers here (`kind`, `limit`) |
| GET    | `/api/v1/discover/:country/rotation`        | Which subscription services to pay for (plans and channels grouped per service; `include_other=true` adds TV bundles and niche services): each scored on its titles' chart points, upcoming episodes, arrivals and top-rated titles over `days` (30), with `subscribe` / `skip` (or `keep` / `pause` / `add` given `subscribed=8,9`) for the best `slots` (2) |
| GET    | `/api/v1/breakouts`                         | Titles rising in several countries at once: entered a chart or climbed `min_climb`+ places (default 3) over `window` days (1–14, default 3) in at least `min_countries` countries (default 2); `date`, `category`, `provider`, `kind`, `limit` |
| GET    | `/api/v1/leaderboards`                      | Titles by chart points (`period=day\|week\|month\|custom`, filters) |
| GET    | `/api/v1/export/rankings`                   | Chart entries as CSV / NDJSON (≤ 366 days) |
| GET    | `/api/v1/countries`, `/api/v1/providers`    | Supported countries; providers with stored charts |
| GET    | `/api/v1/titles`, `/api/v1/titles/:id`      | Scraped titles (paged; `match_status` filter); a single mapped title carries its TMDB title's `ids` |
| GET    | `/api/v1/search?q=&kind=movie\|tv&limit=` | Find titles by any name (TMDB title / original title, FlixPatrol and Netflix names); accent- and punctuation-insensitive, word prefixes ("strang thin"), best match then most recently charted first |
| GET    | `/api/v1/titles/lookup?imdb=\|tmdb=\|slug=\|netflix=` | Find a title by any ID (`netflix` = the number in netflix.com/title/<id>) |
| GET    | `/api/v1/titles/tmdb/:kind/:id`             | Everything about a title: TMDB metadata, chart stats, current positions, ratings, where to watch, Netflix numbers (`include=`), and `ids` — every known ID (IMDb, Wikidata, Rotten Tomatoes, Netflix, JustWatch, FlixPatrol slugs, Netflix Top 10 names) |
| GET    | `/api/v1/titles/tmdb/:kind/:id/stats`       | Chart performance: points, peak, days at #1, debut, streak, spread across countries |
| GET    | `/api/v1/titles/tmdb/:kind/:id/availability` | Watch providers per country (TMDB, data by JustWatch), each with `url` to the title on that service |
| GET    | `/api/v1/titles/tmdb/:kind/:id/availability/history` | When the title arrived on / left each service per country (`added` / `removed` between two TMDB refreshes; `country` optional) |
| GET    | `/api/v1/titles/tmdb/:kind/:id/history`     | Ratings, TMDB popularity and English Wikipedia pageviews over time (`from`, `to`; default last 90 days, ≤ 366): IMDb dataset, each provider's rating sources (a point when a value changed) and daily TMDB popularity |
| GET    | `/api/v1/titles/tmdb/:kind/:id/credits`     | Top 25 cast and key crew (directors, writers, creators, composers, cinematographers) from TMDB; TV with episodes credited |
| GET    | `/api/v1/people/:id`                        | A TMDB person and their titles metareel knows, with roles and each title's chart record (points, days, best rank, countries) |
| GET    | `/api/v1/titles/tmdb/movie/:id/releases`    | A movie's premiere, theatrical (limited / wide), digital (PVOD), physical and TV dates per country from TMDB, the observed streaming arrival (first offer seen arriving, or first chart day) and the windows between them in days (`country`) |
| GET    | `/api/v1/titles/tmdb/:kind/:id/timeline`    | Everything that happened to a title, newest first: chart debuts, peaks, #1 runs and re-entries, availability added/removed, rating changes, Wikipedia pageview spikes, Netflix Top 10 weeks and engagement periods, season premieres, release (`types`, `country`, `from`, `to`, `limit`) |
| GET    | `/api/v1/calendar/:country`                 | Upcoming episodes of shows streamable in the country (subscription / free / ads), one entry per show, season and day with the services (`from`, `to`: default the next 14 days, ≤ 62; `provider_id`) |
| GET    | `/api/v1/availability/changes`              | Titles that arrived on / left a service across titles (`since` RFC 3339, default 7 days; `country`, `event=added\|removed`, `monetization`, `provider_id`, `limit`) |
| GET    | `/api/v1/titles/tmdb/:kind/:id/netflix`     | Weeks in Netflix's official Top 10, with a per-season summary (`seasons`) for TV |
| GET    | `/api/v1/titles/tmdb/:kind/:id/rankings`    | Ranking history by TMDB ID (`kind` = movie or tv; `country`, `from`, `to` optional) |
| GET    | `/api/v1/titles/tmdb/rankings?ids=movie:425,tv:1` | Batch ranking history (max 50 ids)    |
| GET    | `/api/v1/titles/tmdb/:kind/:id/ratings`     | IMDb + Rotten Tomatoes ratings by TMDB ID, cached for `RATINGS_TTL` (`?refresh=true` to force); IMDb's own dataset wins for IMDb |
| GET    | `/api/v1/titles/tmdb/ratings?ids=`          | Batch ratings (max 50; refreshes up to 10 uncached) |
| GET    | `/api/v1/netflix/top10[/:country]`, `/api/v1/netflix/most-popular` | Netflix's official weekly Top 10 and most-popular lists (rows carry `season_number` / `season_source`) |
| GET    | `/api/v1/analytics/{decay,country-similarity,release-lag,ratings-vs-popularity,genres,netflix-calibration}` | Analytics over a range (`from`, `to`, `country`, `provider`, `category`, `kind`) |
| GET    | `/api/v1/analytics/release-patterns`        | How long TV seasons stay in the Top 10 by release pattern (`binge` / `batch` / `weekly` / `daily` / `irregular`, from TMDB episode air dates), over `window` days (30–180, default 60) after each premiere; `from`, `to` = premiere range (default last 365 days), `country`, `provider` |
| GET    | `/api/v1/analytics/spread-lag`              | How many days after a title first charts in one country it charts in the others: overall and per-country lags, origin → destination routes, titles never reached within `window` days (7–90, default 30). Debuts between `from` and `to` (default last 90 days, ≤ 180); `country` = debuted there; `provider`, `category`, `kind` |
| GET    | `/api/v1/analytics/people`                  | People ranked by the Top 10 points of their titles in a range (`role` = actor (top-10 billed) / director / writer / creator / composer / cinematographer; `from`, `to`, `country`, `provider`, `category`, `kind`, `limit`) |
| GET    | `/api/v1/analytics/release-windows`         | Theatrical → digital (PVOD), theatrical → streaming and digital → streaming windows of movies released in theaters in `country` (default US) between `from` and `to` (default last 365 days): medians, quartiles, how many were ≤ 17 / 31 / 45 / 90 days, per streaming service, and each movie (`limit`, ≤ 500) |
| GET    | `/api/v1/analytics/origins`                 | Each service's chart points by origin — `original` / `in_house` (parent company's studios and channels) / `licensed` / `unknown` — per week or month (`interval`), with the owners of licensed titles and top titles. Chart entries carry the same `origin` |
| GET    | `/api/v1/titles/new/:country/:service`      | Titles added to a service per day, live from JustWatch (`from`, `to` default last 7 days; `type`) |
| GET    | `/api/v1/titles/upcoming/:country/:service` | Titles announced for a service, live from JustWatch (`from`, `to`, `type`) |
| GET    | `/api/v1/services/:country`                 | JustWatch service codes in a country (for `:service`) |
| GET/POST/PATCH/DELETE | `/api/v1/admin/...`           | Admin API (admin sign-in with `AUTH_REQUIRE_ADMIN_LOGIN`): schedules, runs, stats, API keys, users (`/users`, `/users/:username`, `/users/:username/logout`) and sessions (`/sessions`, `DELETE /sessions/:id`), settings, title corrections (`PATCH /api/v1/admin/titles/:id`, `POST .../rematch`), `POST /jobs/:type/run`, `POST /backfill`, `GET /data-quality`, `GET /task-types`, and admin copies of the read routes above |
| GET/PATCH/POST | `/api/v1/admin/netflix/titles[/:id[/rematch\|/candidates]]` | Netflix official Top 10 titles: list (`q`, `kind`, `match_status`, `missing=tmdb\|season`, `flag=cross_kind\|linked`, `sort`), detail with season titles, weeks and linked FlixPatrol titles, fix by hand (`tmdb_kind` + `tmdb_id`, or an IMDb / Wikidata ID resolved to TMDB), rematch, candidate search |
| GET/POST | `/api/v1/admin/mappings[/sync\|/:kind/:id[/remap]]` | The TMDB-keyed mapping: every TMDB title with all its IDs and the FlixPatrol / Netflix titles mapped to it (`q`, `kind`, `flag=conflict\|cross_kind\|both\|netflix_only\|flixpatrol_only\|manual`); `sync` recomputes links now; `remap` moves everything on a TMDB title to another (by TMDB, IMDb or Wikidata ID) |
| GET    | `/api/v1/admin/mappings/review/:subject/:id/suggestions` | Other TMDB titles a queued mapping could be, ranked (name, kind, release year against the first chart, an offer on the charted service), for the queue's one-click fix |
| GET    | `/api/v1/admin/mappings/review?subject=flixpatrol\|netflix&reason=&min_risk=25` | Mapping review queue: every automatic mapping scored 0–100 by how likely it is wrong (not on the service it charted on, released after it charted, IMDb / year / name disagreement, weak matching rule), riskiest first, with the reasons |
| GET    | `/api/v1/admin/mapping-refs?tmdb_kind=movie\|tv&mapped=true` | The mapping reference (view `mapping_refs`): one row per chart title with its TMDB title and every ID (IMDb, Wikidata, RT, Metacritic, Letterboxd, JustWatch, Netflix), FlixPatrol slug / kind and Netflix title / kind |
| GET    | `/api/v1/admin/mapping-events?subject=flixpatrol:12\|netflix:34&tmdb=movie:550&limit=` | History of mapping changes, newest first: action, TMDB title before / after, source and who made it (job, `mapping.sync`, `admin`) |
| PATCH  | `/api/v1/admin/netflix/seasons/:id`         | Set a Netflix season title's season number by hand (`{"season_number": 2}`) or hand it back (`{"auto": true}`) |
| GET/POST/PATCH | `/api/v1/admin/watch-formats[/:id]`  | The premium-format taxonomy (console: Premium formats). A format is retired with `active: false`, never deleted |
| GET/POST/PATCH/DELETE | `/api/v1/admin/webhooks[/:id[/test\|/rotate-secret\|/deliveries]]`, `/api/v1/admin/webhook-deliveries/:id[/retry]` | Outbound webhooks (see [Webhooks](#webhooks)) |
| GET    | `/api/v1/me/titles/search?q=&kind=`         | Search TMDB for anything to track (not only titles metareel has charted); needs a signed-in user |
| GET    | `/api/v1/me/titles`                         | The user's tracked titles (`status`, `kind`, `favorite`, `sort`, `limit`, `offset`) |
| GET/PUT/DELETE | `/api/v1/me/titles/{movie\|tv}/:id`  | One title: its TMDB metadata plus the user's status, favourite flag, watchlist note and watch roll-up |
| GET    | `/api/v1/me/watchlist`                      | The watchlist, in list order |
| PUT/DELETE | `/api/v1/me/watchlist/{movie\|tv}/:id` | Put a title on the watchlist or take it off (watches and favourites are kept) |
| GET/POST | `/api/v1/me/watches`                      | The watch log (diary), newest local day first (cursor paged); log a watch |
| GET/PUT/DELETE | `/api/v1/me/watches/:id`            | One watch: read, replace, or delete it for good |
| GET    | `/api/v1/me/tags`                           | The user's tags with how many watches carry each |
| DELETE | `/api/v1/me/tags/:id`                       | Delete a tag and detach it from every watch |
| GET    | `/api/v1/me/shows`                          | Every show you track, sorted into stages (`watching`, `stalled` after `stale_days` (30) without a watch, `not_started`, `on_hold`, `dropped`, `caught_up`) with progress, the next episode and the next air date |
| GET    | `/api/v1/me/upcoming`                       | Episodes still to air of your shows, soonest first (`days`: 90, ≤ 365); on hold, dropped and unwanted shows left out |
| GET    | `/api/v1/me/shows/:id`                      | A show with the user's progress: seasons, episodes, watch counts (`include_specials`) |
| POST   | `/api/v1/me/shows/:id/watched`              | Mark episodes watched: `scope` = `episode` / `season` / `up_to` / `all` |
| DELETE | `/api/v1/me/shows/:id/watched`              | Unmark episodes (`scope`, `season`, `episode`, `include_specials`); deletes their watches |
| GET    | `/api/v1/me/up-next`                        | The next episode to watch of each show in progress |
| GET    | `/api/v1/me/formats`                        | The premium formats a watch can carry (IMAX, Dolby Cinema, 4DX, 70mm, Atmos…) |
| GET/POST | `/api/v1/me/venues`                       | Search cinemas (`q`, `country`, `include_closed`); add one |
| GET/PUT/DELETE | `/api/v1/me/venues/:id`             | One cinema with its screens, former names and outside ids; delete is refused while watches point at it |
| POST/DELETE | `/api/v1/me/venues/:id/aliases[/:alias_id]` | Record or remove a name a cinema used to have |
| POST   | `/api/v1/me/venues/:id/screens`             | Add an auditorium (size, seats, aspect ratio) |
| PUT/DELETE | `/api/v1/me/venues/:id/screens/:screen_id` | Edit or remove an auditorium |
| GET    | `/api/v1/me/stats/summary`                  | Totals, where you watched, formats, spend (`from`, `to`) |
| GET    | `/api/v1/me/stats/calendar`                 | Day-by-day heatmap and streaks |
| GET    | `/api/v1/me/stats/year`                     | Year in review (`year`, default this year) |
| GET    | `/api/v1/me/stats/ratings`                  | Your rating distribution and how it compares with TMDB, IMDb and RT |
| GET    | `/api/v1/me/stats/spend`                    | Money per currency, per cinema, per format |
| GET    | `/api/v1/me/stats/people`                   | Top cast or crew (`job=Director`, `limit`) |
| GET    | `/api/v1/me/watchlist/insights`             | What your watchlist is doing on the charts and on streaming (`country`, `days`) |
| GET    | `/admin` (and `/admin/...`)                 | The admin console SPA from `web/dist/` (`UI_STATIC_DIR`) |
| GET    | `/` (and any unmatched path)                | The user app SPA from `app/dist/` (`APP_STATIC_DIR`); redirects to `/admin/` when the app isn't built |

GET responses under `/api/v1` carry an `ETag`; send it back as
`If-None-Match` to get `304 Not Modified` when nothing changed. The one
exception is `/api/v1/me/...`: those responses are per-user, so they carry
`Cache-Control: private, no-store` and no `ETag` instead.

## Jobs

Every job is a schedule (admin UI → Schedules; run times in UTC) and can be run
on demand from Data health or `POST /api/v1/admin/jobs/:type/run`.

| Job | Default | What it does |
|---|---|---|
| `flixpatrol.top10.scrape` | your schedules | Scrapes each target's current chart, fills gaps from the last `backfill_days`, re-checks hourly for `recheck_hours` when FlixPatrol has not published the day's chart yet. A failed chart page does not stop the run (status `partial`); retries only redo failed pages. New titles are queued for matching right away. |
| `titles.enrich` | 03:00 | Matches titles to TMDB / IMDb (FlixPatrol title page → JustWatch → TMDB), retrying unmatched ones after 1, 3, 7, then every 30 days; finds RT slugs via Wikidata SPARQL, then RT search. Titles fixed in the admin UI (`manual`) are never re-matched, but their TMDB titles get missing RT slugs. |
| `titles.metadata` | 04:00 | TMDB details + watch providers with each service's link from JustWatch (charting titles every 3 days, others monthly), Wikidata IDs (RT, Metacritic, Letterboxd) of TMDB titles films' Wikidata distributors (for origins) and English Wikipedia articles (for pageviews), monthly. Needs `TMDB_API_KEY`. |
| `ratings.prewarm` | 15:30 | Refreshes expired ratings of titles charting in the last 3 days. |
| `netflix.top10.import` | 21:00 | Netflix's official weekly Top 10 (global, per country, most popular); skipped when unchanged. Matches up to 400 new titles (see [TMDB mapping](#tmdb-mapping)) and maps season titles to season numbers. |
| `netflix.titles.match` | 09:00 | Works through the backlog of unmatched Netflix titles (retrying after 1, 3, 7, then every 30 days; `manual` titles are never touched), looks up Netflix IDs of up to 200 more titles (fixing earlier name matches), then remaps seasons. Also queued by "Match again" in the admin UI. |
| `imdb.ratings.import` | 06:00 | IMDb's daily ratings dataset for the IMDb IDs metareel knows (also kept as history: a row whenever a title's rating or votes changed). |
| `tmdb.credits.refresh` | 05:30 | TMDB cast (top 25) and key crew of the titles metareel knows: never-fetched titles first, each again after 60 days, at most 1,000 a run (one request per title). Feeds `/titles/tmdb/:kind/:id/credits`, `/people/:id` and `/analytics/people`. |
| `tmdb.episodes.refresh` | 05:00 | TMDB episodes (air dates, names, finales) of the TV shows metareel knows: seasons from the last three years, undated ones and the latest, one request per show (20 seasons each). Airing shows daily, returning shows weekly, ended shows monthly; at most 1,500 shows a run. Feeds `/calendar/:country` and `/analytics/release-patterns`. |
| `wikipedia.pageviews.import` | 10:00 | Daily English Wikipedia user pageviews of every title with an article (the metadata job finds it on Wikidata), since the last stored day (90 days for a new title), for `/history`. Paced for Wikimedia's 200 requests/minute limit (about 15 minutes for 2,500 titles). |
| `netflix.engagement.import` | 07:00 | Netflix's "What We Watched" reports (hours viewed, views, runtime and the premiere date of originals for every title watched 50k+ hours in six months) from the list in `app_settings` `netflix.engagement.urls` (seeded with H2 2025 and H1 2026; skipped when unchanged). Run it with `{"url": "https://…xlsx"}` to add a new report. Rows are linked to Netflix Top 10 titles by name and shown in `/titles/tmdb/:kind/:id/netflix` (`engagement`); their premiere dates mark Netflix originals for the chart `origin` and `/analytics/origins`. |
| `tmdb.popularity.import` | 09:30 | TMDB's daily ID exports (no API key): popularity of every TMDB title metareel knows, one row per title per day, for `/history`. Skipped when unchanged; yesterday's export is used until today's is published (~08:00 UTC). |
| `mapping.refresh` | — (queued) | Follows up every mapping change (matches, fixes, remaps, shared mappings): TMDB details and Wikidata IDs of the TMDB titles now mapped to, season numbers. Run by hand, it first recomputes every linked title's mapping. |
| `maintenance` | 02:00 | Run retention (`RUN_RETENTION_DAYS`), stale-chart alerts, deletes TMDB people left without credits, SQLite optimize. |

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
- **Netflix "What We Watched" engagement reports** (`.xlsx` downloads linked
  from about.netflix.com), and **Wikimedia pageviews** (REST API, no key;
  identified by User-Agent, 200 requests a minute).
- **IMDb non-commercial datasets** (`title.ratings.tsv.gz`) — personal and
  non-commercial use only; see https://developer.imdb.com/non-commercial-datasets/.
- **TMDB** (search, details, watch providers — watch provider data is by
  JustWatch and must be attributed; daily ID exports for popularity), **Wikidata** (SPARQL, CC0; IDs and film distributors),
  **JustWatch** GraphQL, **Rotten Tomatoes** search index, **MDBList** (optional).

## Public API access

Every `/api/v1` route outside `/api/v1/admin` and `/api/v1/auth` needs
credentials: an **API key** or a **signed-in user's session**. Both work on
every route. Health checks and `/docs` stay open.

| Credential | How to send it |
| --- | --- |
| API key (`mr_…`) | `X-API-Key: <key>` or `Authorization: Bearer <key>` |
| Session token (`mrs_…`) | `Authorization: Bearer <token>` (from `POST /api/v1/auth/login`) |
| Session cookie | `metareel_session`, set by the login response (browsers) |

A header credential wins over the cookie, and a credential that is sent must be
valid (a wrong key is rejected even when a valid cookie is present).

### API keys

- Create, rotate, rename and revoke keys in the admin console under
  **Settings & API keys**. A key is shown once; only a PBKDF2-SHA256 digest of it is stored.
  Rotating replaces the secret immediately.
- Key enforcement can be switched off in the same page (e.g. local dev);
  anonymous requests are then rate limited per client IP. Credentials that are
  sent are still checked.

### Users

- `POST /api/v1/auth/login` with `{"username","password"}` returns
  `{token, expires_at, user}` and sets the `metareel_session` cookie (HttpOnly,
  SameSite=Strict, `Secure` when the request came over HTTPS, including via a
  proxy that sets `X-Forwarded-Proto: https`). `POST /api/v1/auth/logout` ends
  the session; `GET /api/v1/auth/me` says who a request is authenticated as.
- Sessions last `AUTH_SESSION_TTL` (default 30 days) and end after
  `AUTH_SESSION_IDLE_TIMEOUT` (default 7 days) without use. Only a digest of
  each token is stored; the maintenance job deletes expired sessions.
- Passwords (at least 10 characters) are stored as argon2id hashes. Logins are
  rate limited per client IP (10, then 1 every 6 s) and failed logins per
  username (5, then 1 every 30 s); answers are 429 with `Retry-After`.
- Roles are `admin` and `reader`. Both can use the public API; only admins
  can use the admin console when it requires sign-in (below).
- The first admin is created on startup from `AUTH_ADMIN_USERNAME` and
  `AUTH_ADMIN_PASSWORD` (or `AUTH_ADMIN_PASSWORD_FILE`, e.g. a Docker secret)
  when no users exist yet. Manage users with the CLI in the same binary:

```bash
docker exec -it metareel /app/metareel user create -role reader alice   # asks for the password
docker exec -it metareel /app/metareel user list
docker exec -it metareel /app/metareel user passwd alice                 # also signs alice out
docker exec -it metareel /app/metareel user disable alice                # also signs alice out
docker exec -it metareel /app/metareel user logout alice                 # ends every session
docker exec -it metareel /app/metareel user delete alice
```

Locally: `go run ./cmd/server user …`. Changes made with the CLI reach a running
server's session cache within 30 seconds. Admins can do the same in the console
under **Users & sessions**, which also lists active sessions and ends them.

### Admin console sign-in

Set `AUTH_REQUIRE_ADMIN_LOGIN=true` to require a signed-in admin for the
console and `/api/v1/admin`: without a session the API answers 401 and the
console shows a sign-in screen; readers get 403. API keys never open the admin
API. The setting is off by default (the admin API stays open, as before), so
create an admin first, then turn it on and restart. The server logs an error at
startup if it is on while no enabled admin exists; recover with
`metareel user create -role admin <name>`. Nobody can demote, disable or delete
their own account or the last enabled admin.

### Rate limits and exposure

- Requests are rate limited per key or per user (default 300/min, 0 = off,
  set in **Settings & API keys**); over the limit the API answers 429 with
  `Retry-After`.
- Without `AUTH_REQUIRE_ADMIN_LOGIN` the admin console and `/api/v1/admin`
  are open to anyone who can reach them. Either turn it on, or expose only the
  public API to the internet (e.g. block `/` and `/api/v1/admin` at the reverse
  proxy). Serve the console over HTTPS when it is reachable from outside your
  LAN, so the session cookie is marked `Secure`.

```bash
curl -H "X-API-Key: mr_..." http://localhost:8080/api/v1/top10/movies/US

token=$(curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"username":"alice","password":"..."}' http://localhost:8080/api/v1/auth/login | jq -r .token)
curl -H "Authorization: Bearer $token" http://localhost:8080/api/v1/top10/movies/US
```

## Tracking (watchlist and watch log)

`/api/v1/me/...` is one signed-in user's own data: what they want to watch,
what they have watched, and the tags on each watch. It is a separate
namespace from the shared data routes, and it behaves differently from them
in three ways worth knowing:

- **A user's session only.** An API key opens the rest of the public API but
  is refused here with 401: a key identifies an integration, not a person.
  Clients sign in with `POST /api/v1/auth/login` and send the session token
  as `Authorization: Bearer mrs_...`, or let a same-origin browser send the
  `metareel_session` cookie. Writes also require a same-origin request, so
  another site cannot drive them from a visitor's browser.
- **Nothing is cached.** These responses carry `Cache-Control: private,
  no-store` and `Vary: Authorization, Cookie`, and no `ETag`.
- **Anything on TMDB can be tracked.** Titles that never appeared on a chart
  are searched and fetched from TMDB on demand and their display metadata is
  cached in `tracked_titles`. That cache is deliberately separate from
  `tmdb_titles`, so user-tracked titles are never picked up by the mapping,
  metadata, credits or ratings jobs.

### The watch log

Every watch is its own row, so a rewatch is a second watch with its own
rating and tags rather than an edit of the first. `watch_number` counts them
and `is_rewatch` marks everything after the first.

A user's rating for a title is the rating on their **most recent rated
watch**; it is kept on the title's state row and recomputed whenever a watch
is logged, edited or deleted. Setting a rating directly on a title is
refused, so there is only ever one place a rating comes from.

`client_key` makes logging idempotent: sending one again returns the watch
stored the first time (with `200` rather than `201`), so a retry from a flaky
connection cannot create a duplicate. Keys are per user, and a genuine
rewatch uses a new one.

Deletes are permanent; there is no trash.

### Shows and episodes

A show is watched one episode at a time: a watch of `tmdb_kind: "tv"` carries
`season_number` and `episode_number`, and `POST /api/v1/me/watches` refuses a
show without them.

The episodes of a tracked show are fetched from TMDB on demand and cached in
`tracked_episodes`. That is a separate table from `tmdb_episodes`, which the
chart jobs fill with recent, upcoming and latest seasons only -- right for the
calendar, useless for marking a show watched, which needs every season.

`POST /api/v1/me/shows/{id}/watched` marks a run of episodes. `scope` is
`episode` (just this one), `season`, `up_to` (everything up to and including
it, across earlier seasons) or `all`. It marks only episodes that **have
aired** and that you have **not already seen**, so marking a season never
piles rewatches onto the episodes you have. One time applies to every episode
marked, which is why its precision defaults to a whole day; log episodes
individually when the exact times matter. `DELETE` on the same path takes
them back, deleting those watches for good.

Marking and unmarking also move a show between `watching` and `completed`, so
the up-next queue works without setting a status by hand. A status you chose
yourself -- `dropped`, `on_hold`, `not_interested` -- is never overwritten.

Progress is counted against the episodes that have **aired**, and that
denominator moves as a show airs: 100% today can be 96% next week. Specials
(season 0) are left out unless `include_specials` is set. `watched_episodes`
counts distinct episodes, so a rewatch is not extra progress.

`GET /api/v1/me/up-next` is the first aired, unwatched episode of every show
you have started and not finished, most recently aired first.

`GET /api/v1/me/shows` puts every show you track into one stage, in this
order: `watching` (part way through and watched within `stale_days`, 30 by
default), `stalled` (part way through, nothing watched for longer),
`not_started`, `on_hold`, `dropped` and `caught_up` (every aired episode
seen). A status you chose -- on hold, dropped -- decides the stage;
otherwise the episodes do, so a completed show whose new episode has aired
is back to `watching`. `GET /api/v1/me/upcoming` lists the episodes still to
air of every show that is not on hold, dropped or unwanted. Both refresh
episode lists older than a day from TMDB on the way, at most 12 shows a
request, and fall back to the cache if TMDB fails.

### Time

Clients send UTC instants, and the server stores UTC. What a client logs is
`ended_at`, when the watch finished; `started_at` is optional and stored only
when it is sent.

`tz` is the IANA zone the watch happened in, e.g. `America/New_York`. It
decides which **local day** the watch belongs to, and the UTC offset in force
at that instant is derived from it and stored next to it. Send `tz` rather
than `utc_offset_minutes`: an offset on its own cannot be recomputed after an
edit that crosses a daylight-saving change. Because clients send instants and
not local wall-clock times, the two ambiguous cases never arise -- the hour
that happens twice each autumn is two distinct instants, and the hour that is
skipped each spring is no instant at all.

`watched_on` is the local day a watch counts towards, taken from the local
day of its **start**: `started_at` when it was sent, otherwise `ended_at`
minus the title's runtime. It is computed once when the watch is written and
then left alone, so a later TMDB runtime revision cannot quietly move an
entry to another day and reshuffle a diary. `runtime_minutes_used` and
`runtime_source` record which runtime was used, so a day can be explained.

`time_precision` (`minute`, `day`, `month`, `year`) says how precisely the
time is known. Anything less precise than a minute is stored at the earliest
instant it could mean -- 1 January, the 1st, 00:00 local -- and no runtime is
subtracted from it, so "sometime in 2019" cannot land in 2018.

### Where and how a watch happened

Every field below is optional; a watch with none of them is still a watch.

**Premium formats.** One screening mixes independent things -- a brand
(IMAX, Dolby Cinema, Cinemark XD), a projection (70mm, laser, high frame
rate), a sound system (Atmos, DTS:X), a motion rig (4DX, D-BOX) and 2D or 3D
-- so a watch carries a list of format slugs rather than one label, and each
format records which axis it is on. That is what keeps "all my 70mm
screenings" and "IMAX hours this year" answerable later. Admins maintain the
list in the console under **Premium formats**; a format is **retired**, never
deleted, so a watch logged with one keeps its meaning.

**Venues and screens.** Cinemas are shared reference data, not per-user rows:
a cinema is a place, not an opinion, and one row per cinema is what keeps a
diary from filling with near-duplicates. Screen geometry (size, seats, aspect
ratio) belongs to the **screen**, so it is entered once and screens can be
compared; a watch that names a screen inherits it.

History is handled rather than overwritten:

- a watch snapshots `venue_name_at_watch` and `venue_city_at_watch`, so a
  later rename never rewrites what the diary says you went to;
- `venue_aliases` records what a cinema used to be called and when;
- `status` and `replaced_by_venue_id` record a closure or a rebuild, and a
  cinema watches point at cannot be deleted -- close it instead;
- a screen that is retrofitted gets `effective_from` / `effective_to` rather
  than having its old geometry overwritten.

A cinema's ids in outside catalogues live in `venue_refs`, keyed by source
(`google_place_id`, `osm_id`, `wikidata_id`, `fsq_id`, `overture_id`), the
same pattern the title mapping uses. Only Google's **place id** may be stored
indefinitely under their terms -- their other place data may not -- which is
why nothing else from Google is kept.

A venue can carry an IANA `timezone`, and a watch there takes it when the
client sends none. That is what makes the local day right when you are away
from home.

**Money** is in minor units (paise, cents) as integers, never floats, with an
ISO 4217 `currency` that is required whenever an amount is given. Ticket
price, booking fees and concessions are separate, and totals are reported per
currency rather than converted.

**Aspect ratios** are the viewer's own: TMDB has no aspect-ratio data at all.
Send `content_aspect_ratio` as written (`"1.43:1"`, `"16:9"`, `"1.9"`) and it
is stored both as text and as a number so it can be compared;
`content_aspect_variable` covers the films that change ratio part way
through.

### Stats

`/api/v1/me/stats/...` aggregates your own log. A few things are worth
knowing about how the numbers are arrived at.

**Time watched.** How long a watch was is not stored on every row:
`runtime_minutes_used` is only set when a start was derived from it. The
totals fall back to the episode's runtime, then the title's, so an imprecise
watch still counts. A title with no runtime anywhere contributes zero
minutes rather than a guess.

**Streaks** come from the days you watched, and a streak that ran to
*yesterday* still counts as current -- otherwise it would look broken every
morning until you watched something.

**Hours of the day** use the UTC offset stored with each watch, not UTC, so
an evening at home is an evening. Only watches logged to the minute can be
placed in an hour; imprecise ones are left out of that breakdown alone.

**Your rating vs the crowd.** A rating is 1-10 half-stars, which is already a
0-10 scale, so TMDB and IMDb compare directly and a Rotten Tomatoes
percentage is divided by ten. A title nobody has scored is **left out** of
the comparison rather than counted as a zero, which would drag every gap
upwards.

**Genres, countries and languages** come from `tmdb_titles`, which metareel
fills for titles it has charted. A title known only from the on-demand cache
contributes nothing to those three rather than being counted as "unknown".
**Decades** are broader: they use the release date, which the cache does
store, so every tracked title counts. The **people** breakdowns need credits,
which are fetched for charted titles, so they describe the charted part of a
diary rather than all of it.

**Money** is reported per currency and never converted: there is no rate
stored to do it honestly with.

### Watchlist insights

`/api/v1/me/watchlist/insights?country=IN` is the join that only works
because the charts and your watchlist live in one database: which of the
titles you mean to watch have **charted**, which you can **stream now**, and
which have **come off** a service.

"Leaving soon" is deliberately not there. metareel records availability
changes it has observed, not departures a service has scheduled, so the
honest version of that question is what has already gone.

### Ratings

Five stars, in half-star steps, stored as an integer 1-10: `1` is half a
star and `10` is five stars. There is no zero, as on Letterboxd; an unrated
watch has `rating: null`.

## Webhooks

Instead of polling `/api/v1/changes`, a client can register a URL (admin UI
→ Settings → Webhooks, or `POST /api/v1/admin/webhooks`
`{"name","url","events":["*"]}`) and get a signed POST per event:

| Event | When | `data` |
|---|---|---|
| `chart.updated` | A chart was stored or its content changed | country, provider, category, date, `entries` (as in `/top10`, with `origin` and `poster_path`) |
| `title.entered` | A title is on a chart that it was not on at the chart's previous date (`first_time`: never before) | `chart`, `entry` |
| `title.number_one` | A new #1 on a chart | `chart`, `entry` |
| `availability.added` / `availability.removed` | A title arrived on / left a service in a country (between two TMDB refreshes) | as in `/availability/changes` |
| `title.breakout` | A title became a breakout (rising in 2+ countries, as `/breakouts` with the defaults) on the current chart date or the two before; not repeated while it keeps rising | `date`, `window`, `min_countries`, `min_climb` and the breakout item |

The body is `{"id","type","created_at","data"}`. Verify
`X-Metareel-Signature: t=<unix>,v1=<hex>`, the HMAC-SHA256 of `<t>.<raw
body>` keyed with the webhook's secret (shown once when created or rotated),
reject old timestamps, and use `X-Metareel-Event-Id` to ignore repeats. Any
2xx answer counts as delivered; anything else (including redirects) is
retried after 1, 5, 30 minutes, 2 and 12 hours before the delivery fails.
The admin UI shows each webhook's delivery log, its bodies, and can send a
test ping or a delivery again. Events from before the server first ran with
webhooks are not sent. Compare signatures in constant time and reject
timestamps older than a few minutes.

Receivers may be on your LAN; deliveries never go to link-local addresses
(169.254.0.0/16 incl. cloud metadata, fe80::/10), multicast or unspecified
ones, nor to loopback unless `WEBHOOK_ALLOW_LOOPBACK=true` (for a receiver on
the same host outside Docker; inside a container loopback is metareel
itself). Proxies from the environment are not used. Signing secrets are
stored in the database (they are needed to sign), so copies and backups of
`data/metareel.db` carry them: rotate a webhook's secret if a copy leaks.

The admin API (and so webhook configuration) is open unless
`AUTH_REQUIRE_ADMIN_LOGIN` is on: keep it on your LAN otherwise. It sends no CORS headers (only the public API does), and it
refuses requests that change data when a browser marks them as coming from
another site (`Sec-Fetch-Site` / `Origin`), so a web page a LAN user opens
can't drive it; curl and scripts are unaffected.

## Web apps

Two Svelte 5 + Vite single-page apps ship with the server:

| App | Source | Served at | Talks to |
|---|---|---|---|
| Admin console | `web/` → `web/dist` (`UI_STATIC_DIR`) | `/admin` (Vite `base: '/admin/'`) | `/api/v1/admin` |
| User app | `app/` → `app/dist` (`APP_STATIC_DIR`) | `/` | the public API and `/api/v1/me`, with the session cookie |

The user app needs a signed-in user (accounts are created in the admin console)
and never sends an API key. Its design system is **Screening Room**: dark first
with a light mode, artwork-led, with the analytics on every screen. Tokens live
in `app/src/tokens.css`. Colour roles are fixed: white for the primary action,
green and red for movement, blue for new entries, orange for your own titles,
and tan for stale data. It has Home (breakout, KPIs with 14-day trends, the
Top 10, movement, your list, breakouts), Charts (Top 10 with trend, days,
IMDb, RT and origin), title pages (attention, timeline, In numbers, positions,
where to watch), Watchlist (your list joined with `/me/watchlist/insights`:
charting in a 30-day, 90-day or one-year window, streaming on a subscription
now, or recently gone from a service), Library (everything you track, with
status chips that filter, your average rating and its half-star spread,
favourites, a poster grid or a list, and a per-title menu that sets the
status or favourite through `PUT /me/titles/{kind}/{id}` or clears it) and
Diary (every watch grouped by its
local day, this year's totals, a year heatmap from `/me/stats/calendar`, and
edit and delete; deletes are permanent and ask first), Up next (shows by
stage from `/me/shows` -- continue watching, haven't watched in a while, not
started, on hold, dropped -- each with its next episode, progress and a
one-click "Watched", then an Upcoming timeline from `/me/upcoming` with
episodes per week; a Movies tab lists released watchlist movies and ones not
out yet), a show page (`#/show/:id`: a map of every episode, season tabs,
tick to mark, untick to delete after asking, "watched up to here") and Stats (one year at a
time from `/me/stats/year` and `/me/stats/ratings`: watches, hours, titles and
streaks; watches per month, weekday and hour; your rating spread and how far
you sit from TMDB, IMDb and RT, overall and per title; genres, decades,
countries and languages; most watched; where you watched; and people and spend
once there is data for them). Search (⌘K / Ctrl K
or `/`) keeps two result lists apart: titles with chart data (`/api/v1/search`,
with when they last charted) and all of TMDB (`/me/titles/search`, for logging
anything); a pasted IMDb ID or an IMDb, TMDB or Netflix link resolves through
`/titles/lookup`. ↵ opens, ⌘/Ctrl ↵ logs a watch, ⇧ ↵ toggles the watchlist.
Watchlist toggles and "Log a watch" write through `/api/v1/me`. The log
dialog takes the time to the minute, day, month or year; a rating, tags and
notes; and where you watched, which opens that place's fields: a cinema
(searched in `/me/venues` or added there), its screen, seat, screening type,
premium formats (`/me/formats`) and tickets with fees and snacks in one
currency; a streaming service and plan, with a price for rentals and
purchases; a disc's media and edition; a TV channel. Picture and sound
(3D, resolution, HDR, audio, languages, aspect ratios, screen size) are
optional and folded away. A watch at a cinema is entered and filed in the
cinema's time zone.

```bash
make dev-ui      # admin console at http://localhost:5173/admin/
make dev-app     # user app at http://localhost:5174
(cd web && npm run build) && (cd app && npm run build)   # production builds
```

The Docker image builds both. `UI_SERVE_STATIC=false` turns static serving
off (for example when a reverse proxy serves the files).

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
