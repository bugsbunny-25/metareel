---
name: admin-ui
description: Structure and conventions of the metareel Svelte 5 admin console in web/ (hash router with URL state, admin API client, shared components, design tokens). Use when adding or changing admin UI pages or components.
---

# Admin UI (web/)

Svelte 5 (runes) + Vite, no UI framework. Built to `web/dist` (`npm run build`) with
Vite `base: '/admin/'` and served by the Go server at /admin (the user app in `app/` owns /); the dev server proxies `/api` (`npm run dev`).

## Layout

- `src/App.svelte` — sidebar `NAV` grouped Overview · Charts · Data · Operations · System (an item's `paths` lists every route it covers), top bar (breadcrumb, running badge, Glossary, signed-in user + Sign out / Sign in, theme toggle), stats badges.
- `src/pages/*` — one file per page; drawers (`TitleDrawer`, `RunDrawer`) are opened via the `open` query param. `MappingHub` is the Mapping page: its tabs are the original routes `/titles` (`Titles`), `/netflix` (`NetflixTitles`) and `/mapping` (`Mapping`), so old links keep working; tab pages render a `.subhead` (hint + actions), not their own page header. `Insights` (leaderboard / movers / Netflix official / analytics / breakouts / discover / people tabs, `tab` query param; the last three are components with their own filters: `BreakoutsView`, `DiscoverView`, `PeopleView` + `PersonDrawer` (`person` query param), and the analytics tab adds `ReleaseAnalytics`), `Releases` (New & upcoming; its Episodes view is `EpisodeCalendar`), `DataHealth` (data-quality report + run-job buttons) and `WatchFormats` (premium formats: add, edit, retire, and logo upload with dark/light previews; uploads send the raw file through `uploadWatchFormatLogo`, since `req` only sends JSON). Tabbed pages must tag loaded data with its tab (`loaded = {tab, data}`) — rendering one tab's data in another tab's markup throws and freezes the page.
- `src/components/*` — `PageHeader` (title, one-line `desc`, `info` tip, `actions` snippet), `PageTabs`, `Kpi` (headline number + `info`, `foot` snippet, `spark` trend or `meter`, whole-tile `href`), `InfoTip`, `Glossary` (drawer, `glossary=1` query param); charts: `ColumnChart` (stacked, tooltip), `LineChart` (`percent` mode, crosshair), `Sparkline`, `HBars` (ranked bars), `SegmentBar` (parts of a whole with a clickable legend), `Legend`; plus `Pagination`, `SortTh`, `Drawer`, `Modal`, `ConfirmDialog`, `Toasts`, `Icon` (inline SVG set), `RunStatus` (incl. `partial`), `KindBadge`, `IdCell`, `RankChart`, title drawer parts shared by the FlixPatrol, Netflix and TMDB mapping drawers (`TitleIds` = the TMDB title's ID list with sources, `TitleLinks` = header links, `TitleDetailsTab`, `ChartHistoryTab`, `NetflixHistoryTab`, `RatingsTab`; all three drawers use the tabs Mapping · Details · Top 10 history · Netflix history · Ratings · Timeline (`TimelineTab`) · Credits (`CreditsTab`) · Releases (`ReleasesTab`, movies)), `ScheduleModal` (all job types; targets only for scrapes), `BackfillModal`. Charts are hand-written SVG sized with `bind:clientWidth`; no chart library.
- `src/lib/router.svelte.js` — hash router: `route.path`, `route.query`; `setQuery(patch, {replace})`, `navigate`, `href`, `intParam`.
- `src/lib/api.js` — **all data calls go to `/api/v1/admin`** (the UI never uses the key-protected public routes); sign-in calls go to `/api/v1/auth`. A 401 from the admin API calls the handler set by `setUnauthorizedHandler`, which `src/lib/auth.svelte.js` uses to show the sign-in screen (`auth.ready`, `auth.loginRequired`, `auth.user`; `App.svelte` renders `Login` instead of the shell when needed, and `#/login` opens it on purpose). `listRuns` returns `{items, total}` from `X-Total-Count`. `getStats().runs.daily` is a zero-filled 14-day run series for the charts.
- `src/lib/format.js` — dates (UTC-safe), durations, `shortError`, ID/URL parsers (`parseTmdbInput`, `parseImdbId`, `parseRtSlug`), `links`, `downloadCSV`.
- `src/lib/glossary.js` — plain-language term definitions (`GLOSSARY`, `define(term)`); reuse them for `info` tips instead of writing new explanations inline. `src/lib/charts.js` — shared series (`RUN_SERIES`, `COVERAGE_SERIES`) and `shortDay`.
- `src/lib/toast.svelte.js`, `src/lib/confirm.svelte.js` (`await confirm({...})`), `src/lib/constants.js` (countries, providers, page sizes).
- `src/app.css` — design tokens for light (default) and dark (`prefers-color-scheme` / `data-theme`): neutral surfaces (`--bg`, `--surface`, `--surface-2/3`), `--accent`, status colours, chart palette `--c1`…`--c6` and run outcomes `--c-ok/partial/fail/run`, `--grid`. Plus all component classes (`.card`, `.kpi`, `.table`, `.btn`, `.badge-*`, `.segmented`, `.page-tabs`, `.summary-strip`, `.hbar`, `.chart-tip`, `.drawer`, …). Reuse classes and tokens; don't add per-component colours.

## Conventions

- **Page state lives in the URL query** (filters, sort, page, size, open drawer) so reload/back/share work. Derive with `$derived(route.query.x ?? default)`; change with `setQuery({...})` (use `{replace: true}` for typing/debounced search; reset `page` when filters change).
- Fetch in an `$effect` that reads the derived params and guards against stale responses (`let cancelled = false; return () => { cancelled = true }`).
- Lists: `Pagination` (25/50/100/200, first/prev/next/last, page jump) and `SortTh` (`"field"` / `"-field"`). Server-paged lists map UI sort to server sort names (see `Titles.svelte`).
- Every action gives feedback: toast on success/error, `confirm()` for destructive/irreversible actions, disabled buttons while busy.
- Empty, loading (skeleton/spinner) and error (`.alert-error`) states for every list.
- Show timestamps relative with absolute local + UTC in `title`.
- Write for a first-time reader: every page gets a one-line `desc`; metrics and jargon get an `InfoTip` (prefer `define()` from the glossary, and add new terms there). Numbers use tabular figures (`.num`; right-aligned table cells get it automatically).
- Search boxes: `.search` > `span.icon` + `.input` (the wrapper is centred with a child selector; the SVG also has class `icon`).
- Accessibility: buttons for actions, `aria-label` on icon-only controls, Escape closes drawers/modals; the build must have no a11y warnings.

## Verify

`cd web && npm run build` (no warnings), then follow `local-verification` to click through the changed pages in both themes.
