# metareel

Go service that scrapes FlixPatrol Top 10 charts, maps titles to TMDB / IMDb /
Rotten Tomatoes, and serves charts, ranking history, ratings and new/upcoming
releases (public API, API-key protected) plus an admin API and Svelte admin UI.
Consumed by the user's other project, "tides".

## Skills (load the relevant one before working)

- `project-conventions` — layers, rules, endpoint checklist
- `database-changes` — migrations, sqlc, SQLite pitfalls
- `external-providers` — FlixPatrol/FlareSolverr, JustWatch, TMDB, RT, MDBList quirks
- `admin-ui` — Svelte admin console conventions
- `local-verification` — run the stack on a DB copy and check API + UI
- `debug-instance` — diagnose the deployed container

## Always

- Never edit `internal/repository/sqlc/*` or `.env` (hooks block it); change `db/queries` and run `make sqlc`.
- Keep API keys out of logs, run logs and errors (`redactErr` for keyed upstreams).
- Public data routes go in `registerReadRoutes`; admin-only routes on the admin group.
- Update OpenAPI specs, README and `docs/architecture.md` with API/config changes.
- Verify for real before saying done: `go build ./... && go vet ./... && go test -race -count=1 ./internal/...`, `cd web && npm run build`, then the `local-verification` flow for user-facing changes. Say what was verified live vs. only in tests.
- Never run tests or servers against `data/metareel.db`; use a copy.
- Don't add bot-protection evasion; FlixPatrol goes through the user's FlareSolverr, and live solves are run by the user.
