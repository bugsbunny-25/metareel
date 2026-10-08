---
name: project-conventions
description: Architecture, layering rules and change checklists for the metareel Go service (scraper, public/admin API, ratings, releases, admin UI). Use before changing Go code, adding endpoints, config or background jobs.
user-invocable: false
---

# metareel conventions

Go 1.26 service that scrapes FlixPatrol Top 10 charts on a schedule, maps titles to
TMDB / IMDb / Rotten Tomatoes, and serves charts, ranking history, ratings and
new/upcoming releases over a public API plus an admin API + Svelte admin UI.

## Layers (top → bottom; never skip a layer)

| Dir | Role |
|---|---|
| `cmd/server` | Entry point. `cmd/fpcheck` = one-off scrape + mapping check (no DB). |
| `internal/router` | Route registration. `registerReadRoutes` is mounted twice: on the public group (API-key middleware) and under `/api/v1/admin` (no key, used by the UI). Admin-only routes go on `admin` only. |
| `internal/handler` | Thin: parse/validate input → call service → map errors to status. `admin.go` holds admin/key/settings handlers + `RequireAPIKey` middleware. |
| `internal/service` | Business logic. One service per concern: `top10_read`, `title_rankings`, `title_admin`, `title_candidates`, `ratings`, `releases`, `api_keys`, `admin_stats`, `task_schedule_admin`, `flixpatrol_job` (+ `title_match`). |
| `internal/repository` | Wrappers over sqlc code; convert `sql.Null*` ↔ pointers. Never put SQL here. |
| `internal/repository/sqlc` | **Generated. Never edit** (a hook blocks it). See the `database-changes` skill. |
| `internal/client` | Typed clients: JustWatch GraphQL, TMDB, Rotten Tomatoes (Algolia), MDBList, Wikidata, FlareSolverr. See `external-providers`. |
| `internal/scraper/flixpatrol` | FlixPatrol parsing behind the `Fetcher` interface (`CollyFetcher` or `FlareSolverrFetcher`, wrapped in `LoggingFetcher`). |
| `internal/tasks`, `internal/scheduler` | Asynq task payloads/handlers; DB-driven periodic schedules; slog adapter for asynq logs. |
| `internal/config` | Env config (caarlos0/env). Every new var → `config.go`, `.env.example`, `docs/architecture.md` table. |
| `web/` | Svelte 5 admin UI, built to `web/dist`, served by the Go server. See `admin-ui`. |

## Rules

- **Errors → HTTP status**: `*service.ValidationError` → 400, `sql.ErrNoRows` / `ErrTitleNotFound` → 404, upstream failures (`ErrUpstream`, `ErrRatingsUnavailable`) → 502, else 500 with a generic message. Never return internal error text for 500s.
- **Secrets**: API keys never appear in logs, run logs, errors or task payloads. Wrap HTTP client errors from keyed APIs with `redactErr` (`internal/client/redact.go`). Public API keys are stored only as PBKDF2-SHA256 digests (`hashAPIKey`); never a bare fast hash (CodeQL flags it).
- **Logging**: `log/slog` JSON. Log failures at warn/error with context (ids, url, provider), successes at info/debug. Background job progress also goes to the run log via `FlixPatrolRunOptions.RunLog` (shown in the admin UI) — keep those messages human-readable.
- **External calls**: always a timeout and a `ctx`; long jobs check `ctx.Done()`; be polite (request delays for FlixPatrol).
- **Kinds**: `titles.kind` is `movie | tv_show` and is the TMDB namespace of `tmdb_id`; it can differ from the chart category (a special in the TV chart can be a movie). TMDB-keyed URLs use `movie | tv`. `rankings.category` is `movies | tv_shows`.
- **Times**: schedules and charts are UTC. FlixPatrol's "today" switches at 12:00 UTC.
- Don't store data the user asked to be live (releases are fetched from JustWatch per request; only the per-country service list is cached in memory).

## Adding an endpoint (checklist)

1. Service method + `ValidationError`s for bad input; table-driven test (use `newTestDB(t)` for real SQLite with migrations).
2. Handler using existing error mappers (`writeTitleError`, `writeReleasesError`, …).
3. Route: public data → `registerReadRoutes` (gets both public + admin copies); admin-only → `admin` group.
4. OpenAPI: `internal/openapi/spec/public.yaml` or `admin.yaml` (validate YAML; quote descriptions containing `: `).
5. README endpoint table; `docs/architecture.md` if a flow changed.
6. Verify live (see `local-verification`): auth (401 without key on public routes), happy path, error codes.

## Commands

- `go build ./... && go vet ./... && go test -race -count=1 ./internal/...`
- `make sqlc` (needs `make tools` once: sqlc v1.29.0, goose v3.25.0)
- UI: `cd web && npm run build`
- A PostToolUse hook gofmts edited Go files and vets their package; fix what it reports.
