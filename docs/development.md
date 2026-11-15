# Development

Building, running and releasing metareel. For what it does, see the [README](../README.md); for how it works, [architecture.md](architecture.md).

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
├── cmd/server/          # entry point; also the `user` CLI
├── internal/
│   ├── buildinfo/       # version stamped at build time (GET /api/v1/version)
│   ├── client/          # upstream clients: TMDB, JustWatch, RT, Wikidata, FlareSolverr…
│   ├── config/          # env-driven config
│   ├── handler/         # HTTP handlers and auth middleware
│   ├── openapi/         # public and admin OpenAPI specs (served at /docs)
│   ├── repository/      # data access over the generated sqlc code
│   ├── router/          # route registration
│   ├── scraper/         # FlixPatrol parsing
│   ├── server/          # wiring and lifecycle
│   ├── service/         # business logic
│   └── tasks/, scheduler/  # background jobs and their schedules
├── db/migrations/, db/queries/  # goose migrations, sqlc queries
├── web/                 # admin console (Svelte), served at /admin
├── app/                 # web user app (Svelte), served at /
├── mobile/              # mobile app (Expo, React Native): mobile/README.md
├── tools/               # dev tools (own go.mod)
├── docs/                # these docs
├── Dockerfile, docker-compose.yml, Makefile
└── .env.example         # every setting
```

See [architecture.md](architecture.md) for how the pieces fit.

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

Each workflow's jobs run only when files they cover change: a first `changes` job
(`dorny/paths-filter`) lists the changed files and the other jobs skip themselves otherwise,
so a change to `docs/` or a README runs only those small jobs. Releases run every check
regardless, and if `changes` fails everything runs.

- `ci.yml` (Go: `cmd/`, `internal/`, `db/`, `tools/`, `go.mod`, `Makefile`, …)
  - `go-quality`: `go mod tidy` check, `make vet`, `make build`, race tests with coverage, coverage artifact
  - `integration`: Redis service container + DB env wiring + test suite
  - `govulncheck`: vulnerability scan with `govulncheck ./...`
- `codeql.yml` (Go sources)
  - CodeQL analysis for Go on PRs and `main` (a release is a commit `main` already analysed)
- `container.yml` (everything in the image: Go, `db/`, `web/`, `app/`, `Dockerfile`)
  - The image's frontend stages build both web UIs, so this is also their build check
  - PR: Docker build + Trivy scan + SBOM artifact
  - `main`: Docker build + push to GHCR as `main` and `sha-<commit>` + Trivy scan + SBOM artifact,
    then `cleanup-main-images` deletes every older main image (only once the whole build passed),
    so GHCR holds a single main image. Release images are never touched.
- `mobile.yml` (`mobile/`, the OpenAPI spec and design tokens it generates from)
  - Typecheck, lint, Jest, generated files (API types, theme tokens, license list) up to date;
    on `main`, publishes the JS to the EAS Update `preview` channel when `EXPO_TOKEN` is set
- `release.yml`
  - Releases everything under one version (below): runs `ci.yml` and the mobile checks
    (`mobile.yml`), builds and pushes the image via `container.yml`, builds the mobile apps on
    EAS via `mobile-release.yml` (iOS submitted to TestFlight, the Android APK downloaded from
    EAS), then creates the tag and release (generated notes, image digest, SBOM and APK
    attached). Nothing is tagged or sent to TestFlight if a check or the image fails. Without
    `EXPO_TOKEN` the release ships without mobile builds.
  - Start it from Actions → release → Run workflow (`version`, `prerelease`, `ref`), or push a
    `v*` tag / create a release in the UI. `prerelease` (off by default) only marks the GitHub
    release.
- `mobile-release.yml`
  - Called by `release.yml` only: the EAS builds for the release's commit.
- `staging-deploy.yml`
  - Manual (`workflow_dispatch`) deploy scaffold with smoke-test contract (`/healthz`, `/api/v1/test`)

### Versions

One version for the server (`GET /api/v1/version`), the image tag, the web UIs and the mobile
apps: `X.Y.Z`, released as the tag `vX.Y.Z`. No `-alpha` or other suffixes (the
`v0.x.0-alpha` tags before 0.5.0 were the last).

1. `make version V=0.6.0` writes it into `mobile/`, `app/` and `web/` (`package.json` and the
   lockfiles) plus `mobile/package.json`'s `versionCode`. Commit and push.
2. Release `v0.6.0`. `release.yml` refuses a tag that doesn't match the commit's version, so the
   source at a tag always says what it is (F-Droid builds from it).

The apps' build numbers come from the version (`mobile/version.ts`):
`versionCode = X*1,000,000 + Y*1,000 + Z` (0.5.0 is 5000). Every later version is higher, which
both stores require: Apple only accepts an upload whose build number (`CFBundleVersion`) is
higher than the last, and Android (the APK, F-Droid) only installs a higher `versionCode`.
Minor and patch go up to 999. Rebuilding the same version doesn't raise the build number, so
TestFlight refuses a second upload of it: release a new version instead.

For F-Droid, the metadata can read both from the tag's source:
`UpdateCheckMode: Tags ^v[0-9]+\.[0-9]+\.[0-9]+$` and
`UpdateCheckData: mobile/package.json|"versionCode":\s*(\d+)|.|"version":\s*"([^"]+)"`.

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

### Required checks (ruleset `main-pr`)

The workflows start on every PR, and a job skipped by its `if` counts as passed, so these
can be required without blocking PRs that don't touch their files. (Workflow-level `paths`
filters would leave them pending forever, and a ruleset can't scope a required check by path.)
The ruleset also requires a PR and an up-to-date branch. Edit it in Settings → Rules →
Rulesets → `main-pr` → "Require status checks to pass".

- `go-quality`
- `integration`
- `govulncheck`
- `analyze` (CodeQL)
- `build-scan-publish` (container + Trivy)
- `checks` (mobile), optional: it skips itself the same way

### Operational notes

- Container images are built from `Dockerfile` and use `linux/amd64`.
- Main images are tagged `main` + `sha-<commit>` and only the latest survives; release images are
  tagged with their version (`v*`) and kept. Cleanup uses the workflow's `GITHUB_TOKEN`, so the
  package's *Manage Actions access* must give this repository the Admin role.
- Tags and releases created by `release.yml` don't trigger other workflows (a `GITHUB_TOKEN`
  limit), which is why it calls `ci.yml`, `mobile.yml`, `container.yml` and
  `mobile-release.yml` itself.
- The mobile jobs share `.github/actions/setup-mobile` (Node, EAS CLI, `npm ci`).
- Trivy fails the workflow on `HIGH`/`CRITICAL` vulnerabilities.

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
