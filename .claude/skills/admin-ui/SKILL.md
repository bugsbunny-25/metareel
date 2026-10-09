---
name: admin-ui
description: Structure and conventions of the metareel Svelte 5 admin console in web/ (hash router with URL state, admin API client, shared components, design tokens). Use when adding or changing admin UI pages or components.
---

# Admin UI (web/)

Svelte 5 (runes) + Vite, no UI framework. Built to `web/dist` (`npm run build`) and
served by the Go server; the dev server proxies `/api` (`npm run dev`).

## Layout

- `src/App.svelte` — sidebar `NAV` (path → page component), stats badges, theme toggle.
- `src/pages/*` — one file per page; drawers (`TitleDrawer`, `RunDrawer`) are opened via the `open` query param. `Insights` (leaderboard / movers / Netflix official / analytics tabs, `tab` query param) and `DataHealth` (data-quality report + run-job buttons). Tabbed pages must tag loaded data with its tab (`loaded = {tab, data}`) — rendering one tab's data in another tab's markup throws and freezes the page.
- `src/components/*` — `Pagination`, `SortTh`, `Drawer`, `Modal`, `ConfirmDialog`, `Toasts`, `Icon` (inline SVG set), `RunStatus` (incl. `partial`), `KindBadge`, `IdCell`, `RankChart`, `ScheduleModal` (all job types; targets only for scrapes), `BackfillModal`.
- `src/lib/router.svelte.js` — hash router: `route.path`, `route.query`; `setQuery(patch, {replace})`, `navigate`, `href`, `intParam`.
- `src/lib/api.js` — **all calls go to `/api/v1/admin`** (the UI never uses the key-protected public routes). `listRuns` returns `{items, total}` from `X-Total-Count`.
- `src/lib/format.js` — dates (UTC-safe), durations, `shortError`, ID/URL parsers (`parseTmdbInput`, `parseImdbId`, `parseRtSlug`), `links`, `downloadCSV`.
- `src/lib/toast.svelte.js`, `src/lib/confirm.svelte.js` (`await confirm({...})`), `src/lib/constants.js` (countries, providers, page sizes).
- `src/app.css` — design tokens (`--bg`, `--surface`, `--accent`, …) for dark (default) and light (`prefers-color-scheme` / `data-theme`), plus all component classes (`.card`, `.table`, `.btn`, `.badge-*`, `.segmented`, `.drawer`, …). Reuse classes; don't add per-component colors.

## Conventions

- **Page state lives in the URL query** (filters, sort, page, size, open drawer) so reload/back/share work. Derive with `$derived(route.query.x ?? default)`; change with `setQuery({...})` (use `{replace: true}` for typing/debounced search; reset `page` when filters change).
- Fetch in an `$effect` that reads the derived params and guards against stale responses (`let cancelled = false; return () => { cancelled = true }`).
- Lists: `Pagination` (25/50/100/200, first/prev/next/last, page jump) and `SortTh` (`"field"` / `"-field"`). Server-paged lists map UI sort to server sort names (see `Titles.svelte`).
- Every action gives feedback: toast on success/error, `confirm()` for destructive/irreversible actions, disabled buttons while busy.
- Empty, loading (skeleton/spinner) and error (`.alert-error`) states for every list.
- Show timestamps relative with absolute local + UTC in `title`.
- Accessibility: buttons for actions, `aria-label` on icon-only controls, Escape closes drawers/modals; the build must have no a11y warnings.

## Verify

`cd web && npm run build` (no warnings), then follow `local-verification` to click through the changed pages in both themes.
