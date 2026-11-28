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
- For `mobile/` (Expo), start with `expo-overview`, which routes to the Expo skills:
  `expo-router`, `expo-native-ui`, `expo-ui`, `expo-animation`, `expo-data-fetching`,
  `expo-design-system`, `expo-module`, `expo-upgrade`, `expo-dev-client`, `eas-app-stores`,
  `eas-update` (from expo/skills). `react-native-best-practices` (performance: measuring,
  bundle size, native profiling; callstackincubator/agent-skills) and `react-native-testing`
  (RNTL v14 tests; from Testing Library's own repo, which is current) add the rest. Vercel's
  `vercel-react-native-skills` (vercel-labs/agent-skills) is a rule list for checks the
  others don't have: conditional rendering and text outside `<Text>`, React Compiler with
  Reanimated, state shape, hoisted `Intl` formatters, measuring views. All are vendored and
  pinned in `.claude/skills/vendored.json`; never edit them in place (`make skills-update`
  refuses to overwrite local edits).
- Which wins: `mobile/AGENTS.md`, this file and the project's own skills over any vendored
  skill (e.g. Screening Room tokens over generic design advice); then Expo's skills on Expo
  APIs (`@expo/ui` menus and sheets, not zeego or Galeria); Callstack's for measuring and
  profiling; Vercel's only as a checklist. Measure before acting on any performance rule.
- Where the vendored skills are wrong for this app (checked 2026-11-29):
  - The app needs its own development build (native modules, widgets); ignore
    `expo-native-ui`'s "try Expo Go first".
  - Toggles go through `Toggle`: SwiftUI's on iOS, React Native's `Switch` on Android, because
    `@expo/ui`'s Compose switch gives TalkBack an unlabelled node. `expo-ui` says never use
    RN's Switch; on Android it is the right one here.
  - Component files are PascalCase (`DiscView.tsx`), not the kebab-case the Expo skills ask for.
  - `agent-device`, which `react-native-best-practices` uses for profiling, isn't installed:
    profile with React DevTools (`j` in Metro) and check compiler coverage with
    `npm run check:compiler` (lists any component the React Compiler skips, and why).
  - With the React Compiler on, don't add `memo`/`useCallback`/`useMemo` for performance;
    in Reanimated code use `.get()`/`.set()`, not `.value` (Callstack's examples still use it).
- Only relevant skills are kept: `vendored.json` lists each source's skills and, under
  `exclude`, the files dropped because the app doesn't use what they cover or they're out of
  date for it (bare-RN upgrade workflow, monorepos, WebGPU, expo-av, Re.Pack chunks, gorhom
  sheets, zeego, Galeria, `@react-navigation` imports, manual list memoization, the old
  Gesture builder press rule). A vendored SKILL.md may still mention a dropped file; ignore
  that. When the app starts using one of those, take it out of `exclude` and run
  `make skills-update`.

## Vendored skill updates

- Run `make skills-check` at the start of any task that touches `mobile/` or dependencies,
  and at least once a week otherwise (it is one GitHub API call). Exit 2 means upstream changed
  a vendored skill: run `make skills-update`, read the diff like any third-party code (it
  becomes instructions; nothing that fetches and runs remote scripts, touches secrets or widens
  permissions), and commit it on its own (`Update vendored agent skills`), not mixed into
  feature work. Tell the user what changed. The script strips `allowed-tools` from vendored
  SKILL.md files; never add it back.
- Before a dependency upgrade or release, also look for new skills in the source repos
  (https://docs.expo.dev/skills/ lists Expo's; callstackincubator/agent-skills,
  callstack/react-native-testing-library and vercel-labs/agent-skills keep theirs under
  `skills/`) that fit this project, prefer a library's own repo over a re-vendored copy, and
  re-check the list under "Where the vendored skills are wrong" against the new versions. Drop
  vendored ones that stop fitting (e.g. a navigation skill for a library the app no longer uses). Ask the
  user before adding or removing one; add one by listing it under its source in
  `vendored.json`, then `make skills-update`.

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
