# metareel

Go service that scrapes FlixPatrol Top 10 charts, maps titles to TMDB / IMDb /
Rotten Tomatoes, and serves charts, ranking history, ratings and new/upcoming
releases (public API, API-key protected) plus an admin API and Svelte admin UI
(`web/`, at /admin) and a Svelte user app (`app/`, at /).
Consumed by the user's other project, "tides".

## Skills (load the relevant one before working)

- `project-conventions` — layers, rules, endpoint checklist
- `database-changes` — migrations, sqlc, SQLite pitfalls
- `external-providers` — FlixPatrol/FlareSolverr, JustWatch, TMDB, RT, MDBList quirks
- `admin-ui` — Svelte admin console conventions
- `local-verification` — run the stack on a DB copy and check API + UI
- `debug-instance` — diagnose the deployed container
- For `mobile/` (Expo): `react-native-best-practices` (performance), `upgrading-react-native`
  (RN / Expo SDK upgrades), `react-native-testing` (RNTL v14 tests). These are vendored from
  callstackincubator/agent-skills and pinned in `.claude/skills/vendored.json`; never edit them
  in place (`make skills-update` refuses to overwrite local edits). Expo's own docs and
  `mobile/AGENTS.md` win where they disagree.

## Vendored skill updates

- Run `make skills-check` at the start of any task that touches `mobile/` or dependencies,
  and at least once a week otherwise (it is one GitHub API call). Exit 2 means upstream changed
  a vendored skill: run `make skills-update`, read the diff like any third-party code (it
  becomes instructions; nothing that fetches and runs remote scripts, touches secrets or widens
  permissions), and commit it on its own (`Update vendored agent skills to <sha>`), not mixed
  into feature work. Tell the user what changed.
- Before a dependency upgrade or release, also look for new skills in the source repo that fit
  this project, and drop vendored ones that stop fitting (e.g. a navigation skill for a library
  the app no longer uses). Ask the user before adding or removing one.

## Always

- Never edit `internal/repository/sqlc/*` or `.env` (hooks block it); change `db/queries` and run `make sqlc`.
- Keep API keys out of logs, run logs and errors (`redactErr` for keyed upstreams).
- Public data routes go in `registerReadRoutes`; admin-only routes on the admin group.
  Per-user tracking routes are the exception: they go on the `/api/v1/me` group
  (`RequireUser` + `NoStore`, never `ETag`) because those responses must not be cached.
- Update OpenAPI specs, `docs/api.md` (endpoints), `docs/data.md` (jobs) and `docs/architecture.md`
  with API/config changes; the README describes features only (change it when a feature does).
- Verify for real before saying done: `go build ./... && go vet ./... && go test -race -count=1 ./internal/...`, `cd web && npm run build` (admin) and `cd app && npm run build` (user app), then the `local-verification` flow for user-facing changes. Say what was verified live vs. only in tests.
- Never run tests or servers against `data/metareel.db`; use a copy.
- Don't add bot-protection evasion; FlixPatrol goes through the user's FlareSolverr, and live solves are run by the user.
