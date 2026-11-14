# metareel mobile app: plan

Status: **in progress**; phase status notes are in §8. Written 2026-11-14;
decisions from the review with the user are in §0 and folded into each section.

A React Native app built with Expo (SDK 58) for phones and foldables on Android
and iOS, talking to the same metareel server as the web user app (`app/`).

Expo facts below were checked against the SDK 58 docs (`expo@58.0.22` and
docs.expo.dev) on 2026-11-14. Backend facts were checked in this repo.

---

## 0. Decisions (2026-11-14)

| # | Question | Decision |
|---|---|---|
| 1 | Apple Developer Program | **Already enrolled.** TestFlight, push, widgets and Live Activities are available from the first iOS build. |
| 2 | Android distribution | **Sideloaded APK**, published as **GitHub Release assets** of this repo (see §7). No Play Store. |
| 3 | How phones reach the server | **Public HTTPS domain** (already exposed safely), plus an **optional local HTTP URL** used when the phone is on the home network. No Tailscale. |
| 4 | Notifications | **Local first** (on-device new-episode reminders), **Expo push later** for watchlist arrivals. |
| 5 | Phone sessions | **"Remember this device" sessions of 180 days**, revocable in Settings → Sessions. |
| 6 | Ticket-stub photos | **Not wanted.** Dropped (no camera, no attachments). |
| 7 | Test devices | **No physical devices.** iOS simulator incl. the foldable iPhone Duo; Android resizable emulator and foldable emulators. |
| 8 | Top 10 on the phone | **Yes, a small Top 10 rail** on Tonight. Full Charts stay on the web. |
| 9 | "Now showing" | **iOS Live Activity and an Android equivalent** (ongoing progress notification). |
| 10 | Letterboxd links | **Server resolver + search fallback.** |
| 11 | Name and IDs | Home-screen name **metareel**; bundle ID / package **`io.github.bugsbunny25.metareel`** on both platforms. |
| 12 | Repo | **`mobile/` in this repo.** |

---

## 1. Scope and screens

The phone app is for the moments **away from the desk**: leaving a cinema,
on the sofa ticking off an episode, seeing a link in another app. Browsing,
analytics and admin stay on the web.

### v1 (phases 1–4)

| Screen / flow | What it does on the phone | API it uses (exists unless marked new) |
|---|---|---|
| **Sign in** | Server URL (+ optional home-network URL) + username + password, once. Token kept in the secure store. | `POST /api/v1/auth/login` (returns the `mrs_…` token in the body) |
| **Tonight** (home) | Up next (continue watching), what's airing today, a small **Top 10 rail** (your country, one service, switchable), the last few logs, a "Log a film" button. | `/me/up-next`, `/me/upcoming`, `/me/watches?limit=`, public Top 10 chart endpoint (session auth is accepted on public routes) |
| **Quick log** (the main flow) | Two taps after a film: pick the title (recent watchlist / now playing / search), "Just now" end time, optional stars. Works offline. | `POST /me/watches` with `client_key` |
| **Full log sheet** | Everything the web log form has: venue + screen, formats, ticket price, covered by a cinema subscription, disc/label, tags, rating. | `/me/watches`, `/me/venues`, `/me/formats`, `/me/subscriptions`, `/me/tags` |
| **Which cinema am I in?** | Inside the log sheet: nearest cinemas from GPS, with the one you used last at that spot first. | **new** `GET /me/venues/nearby` |
| **Up next + Show** | Stages as on the web; show page with season tabs and episode ticks. | `/me/up-next`, `/me/shows/{id}`, `/me/shows/{id}/watched` |
| **Watchlist** | List with where-to-watch chips for your services; add/remove. | `/me/watchlist`, `/me/watchlist/insights` |
| **Diary** | Reverse-chronological list grouped by day; edit/delete an entry. No heatmap in v1. | `/me/watches`, `/me/watches/{id}` |
| **Title** | Lean: poster hero (tinted), year/runtime, IMDb/RT/TMDB scores, where to watch, current chart positions, your watches, Log / Watchlist buttons. No attention chart or timeline. | `/me/titles/{kind}/{id}` (+ public title endpoints for ratings) |
| **Search** | One search field: catalogued + TMDB titles, and IDs/links (TMDB, IMDb `tt…`, Netflix). | `/me/titles/search`, the existing lookup used by the web ⌘K palette |
| **Subscriptions** | Your cinema plans and this period's break-even ("2 more films to break even"). | `/me/subscriptions`, `/me/subscriptions/savings` |
| **Settings** | Account, server URLs, theme, notification switches, sessions (with device names; revoke), app version + "update available", sign out. | `/me/account`, `/me/preferences`, `/me/sessions` |

### After v1 (phases 5–7)

- **Share sheet** in (Netflix / IMDb / TMDB / Letterboxd link → resolve → "Add to watchlist" or "Log it").
- **Notifications**: new episode of a show you follow (local); a watchlist title arriving on one of your services (push).
- **Widgets**: Up next, subscription break-even, iOS lock-screen accessory.
- **"Now showing"**: iOS Live Activity and an Android ongoing progress notification.

### Stays web-only

Stats and year in review, Discover (8 tabs), the full Charts page and movement
tables, Person pages (a phone Title links out to the web person page), Library
with its filters and histograms, venue editing and merging (admin), password
change, imports.

---

## 2. Foldables and large screens

### Approach: window size classes, not device detection

Everything keys off the **current window width** in dp (`useWindowDimensions`),
using Material's window size classes, so the same code handles a folded Z Fold
cover screen, the unfolded inner screen, a Flip, the iPhone Duo folded and
open, split-screen, freeform windows, tablets and iPad:

| Class | Width | Layout |
|---|---|---|
| Compact | < 600 dp | One column, bottom tab bar, sheets for logging |
| Medium | 600–839 dp | Navigation rail on the left, list + detail side by side where it helps (Diary, Up next → Show, Watchlist → Title) |
| Expanded | ≥ 840 dp | Rail + two panes with a wider detail pane; the log sheet opens as a side panel instead of a bottom sheet |

Rules that make fold/unfold and resizing seamless:

- **One navigator for all sizes.** Expo Router's headless tabs
  (`expo-router/ui`) with our own tab list that renders as a bottom bar or a rail
  depending on width. Swapping between two different navigators at the moment
  of unfolding would remount the tree and lose scroll and form state.
- **List–detail is a layout, not separate routes.** `diary/_layout.tsx` renders
  the list and, on medium/expanded, the selected entry beside it; on compact
  the same detail route pushes full screen. The URL stays the same either way,
  so unfolding mid-flow keeps you on the entry you were looking at.
- **Never read dimensions once at startup**, never lock orientation
  (`"orientation": "default"`). On Android 16 (API 36) apps can no longer
  restrict orientation or resizability on large screens anyway.
- Android: the Expo/React Native template declares the `configChanges` that
  keep the activity alive across fold/unfold and resize; we verify it.
  `resizeableActivity` defaults to true, so multi-window and freeform work.
- The in-progress log sheet's draft lives in a store persisted to SQLite, so
  even an activity recreation or process death does not lose a half-filled log.
- Found in phase 2: a screen-density change (the user changing Display size,
  or `adb shell wm density`) is not in the activity's `configChanges`, so
  Android recreates the activity and the app restarts on its first tab (dev
  builds also log "multiple navigation containers"). Folding, unfolding and
  window resizing don't change density and keep state (checked on the
  resizable emulator at 412, 673, 841 and 1280 dp). Phase 4 tried adding
  `density` to `configChanges`: the app then stays put, but React Native keeps
  text measured at the old scale (labels cut off, headings clipped) until
  things re-render. A restart on a rare settings change is the lesser evil, so
  density is left to Android's default (recreate).

### Hinge and posture

- **`expo-hinge`** gives the hinge **status** (closed, partially open, fully
  open) and **angle**: Android 11+ via Jetpack WindowManager `FoldingFeature`
  + the hinge angle sensor; iOS 27.1+ via `UIHingeInteraction` (the iPhone Duo).
  `useHinge()` re-renders on change.
- The docs don't show the hinge **bounds and orientation** (where the fold
  line is, horizontal or vertical) or **occlusion**. Tabletop posture needs
  them. **Custom:** a small local Expo module (`modules/fold-info`, ~100 lines
  of Kotlin) wrapping `WindowInfoTracker.windowLayoutInfo` → `FoldingFeature`
  `bounds`, `orientation`, `isSeparating`, `occlusionType`. On iOS, check during
  phase 4 whether `@expo/ui`'s `onHingeChange` / `ArrangementView` or UIKit
  gives the fold position on the iPhone Duo; if needed, the same module gets a
  small Swift side. Dropped if `expo-hinge` exposes bounds by then.
- Posture use is deliberately small: (1) in a two-pane layout, put the pane
  split on the hinge when it separates; (2) a tabletop layout for "Now showing"
  and the log sheet on a half-open device.

### iPad

Falls out of the same size classes with `ios.supportsTablet: true`. Expo
Router's native `SplitView` is iOS-only and alpha in SDK 58, so we don't use it.

### Test matrix (simulators and emulators only)

No physical devices, so everything is verified on:

- **Android emulator:** the **resizable** AVD (switches between phone, foldable,
  tablet and desktop sizes at runtime), the Pixel Fold and a Flip-style
  foldable AVD for hinge/posture (the emulator's virtual sensors set the hinge
  angle), split-screen and freeform windows.
- **iOS simulator:** an iPhone, the **iPhone Duo** (iOS 27.1, folded/unfolded
  and hinge), an iPad with Split View / Stage Manager.
- Simulators also cover push (iOS simulators receive remote pushes on Apple
  silicon), widgets, Live Activities, the share extension and simulated location.
- What simulators can't prove: real-device performance on long lists,
  real GPS accuracy inside a cinema, and OEM quirks (Samsung's One UI). If one of
  these bites, Samsung Remote Test Lab gives short sessions on real Galaxy
  foldables at no cost.

---

## 3. Architecture and stack

| Concern | Choice | Why / notes |
|---|---|---|
| Framework | **Expo SDK 58**, TypeScript, New Architecture | Every library below supports it. |
| Dev workflow | **Development builds from day one** (no Expo Go) | Widgets, hinge, share extension, push and our own modules need native code; Expo Go has no push since SDK 53. |
| Navigation | **Expo Router** with typed routes; headless tabs (`expo-router/ui`); native `formSheet` presentation for the log sheet on phones | Deep links for free (`metareel://title/movie/603`), needed by notifications, widgets and "Now showing". |
| Server data | **TanStack Query v5**, cache persisted to SQLite | Stale-while-revalidate, retry, focus/online refetch; the app opens with data in a cinema basement. |
| Offline writes | **Own outbox table in `expo-sqlite`** | Must survive app kills and be readable by background tasks; see below. |
| Local storage | `expo-sqlite` (outbox, query cache, nearby-venue cache, drafts); `expo-secure-store` (session token, server URLs) | |
| Server selection | Small "endpoint" module: public HTTPS URL always; home HTTP URL only after a verified probe (see §7) | |
| API client | `openapi-fetch` + types generated by `openapi-typescript` from `internal/openapi/spec/public.yaml` | The spec already documents the 41 `/me` paths. |
| Images | `expo-image` (disk cache, placeholder, transitions) | |
| Poster tint | **new**: server returns each title's average poster colour (`poster_color`) | The web samples a canvas; native has no cheap equivalent, and widgets need it too. |
| Styling | `StyleSheet` + `useTheme()` / `useWindowClass()`; tokens generated from `app/src/tokens.css` | No styling framework; web and phone never drift. |
| Fonts | Source Serif 4 + Source Sans 3 (OFL) from `@expo-google-fonts`, loaded with `useFonts` behind the splash screen | Same family names on both platforms (embedding via the config plugin uses PostScript names on iOS and file names on Android). `fontVariant: ['tabular-nums', 'lining-nums']`, matching `.num`. |
| Charts | Hand-drawn `react-native-svg` sparklines (Top 10 trend, break-even bar), ports of `Spark.svelte` | No chart library in v1. |
| Forms | Controlled components + a small draft store (Zustand, persisted) for the log sheet | |
| Dates | `Intl` + the server's rules (send UTC + IANA `tz`; the client sends the **end** time) | Port `app/src/lib/dates.js`. |
| Testing | Jest + `jest-expo` + React Native Testing Library; **Maestro** flows on the emulator/simulator; `tsc --noEmit`, `expo lint` | Flows run against a local metareel on a DB **copy**, on spare ports. |
| OTA updates | **EAS Update**, `runtimeVersion: { policy: "fingerprint" }`, channels `preview` and `production` | Free plan: 1,000 monthly active users. JS fixes ship without a new APK/TestFlight build. |
| Errors | No third-party crash reporting in v1; Settings → "Copy diagnostics" (versions, update ID, last API errors, tokens redacted) | |

### Theming ("Screening Room")

Dark first, light counterpart, follows the system or the server-side theme
preference the web already stores. Colour roles carry over exactly: white
primary action, green/red movement, blue new, orange `--accent-yours` for your
own titles, tan stale. Panels on two surface steps (18/20 radius), round icon
buttons, artwork-led heroes tinted from the poster. Native touches: system bars
follow the canvas, Android edge-to-edge, haptics on log/tick. The Top 10 rail
keeps the web's large numerals.

### Offline outbox

- Tapping **Log** writes a row (`id`, `client_key` = UUID from `expo-crypto`,
  `method`, `path`, `body`, `created_at`, `attempts`, `last_error`) and updates
  the cached Diary/Up next optimistically. The **end time is captured at tap**.
- Flushed in order on: enqueue, app foreground, network back (`expo-network`),
  and a background task (`expo-background-task`). Exponential backoff; a 4xx
  other than 408/429 stops the item and shows it in a "Couldn't sync" banner
  with Edit / Discard.
- Only **idempotent** operations are queued: `POST /me/watches` (the server
  returns the existing watch for a repeated `client_key`), episode marks
  (marking skips already-watched episodes), watchlist put/delete. Edits and
  deletes of existing entries need the network in v1.
- Offline "which cinema": GPS works without data, so the app keeps the last
  fetched nearby venues within ~30 km plus your recent venues and ranks them
  locally when offline.

---

## 4. Native capabilities (checked against Expo SDK 58 docs)

All need a **development build**. "Config plugin" means the capability is
turned on in `app.config.ts` and prebuild regenerates the native projects; we
never hand-edit `ios/` or `android/`.

| Capability | Library | Config plugin? | Notes |
|---|---|---|---|
| Local notifications | `expo-notifications` | Yes | Scheduled on the device; no server, no Firebase. |
| Push notifications (phase 6b) | `expo-notifications` | Yes | Android needs an FCM v1 project (`google-services.json`); iOS needs an APNs key (you're enrolled). |
| Receive shares | `expo-sharing` (`useIncomingShare`) | Yes: `ios.enabled` adds a Share Extension target + App Group; `android.enabled` adds the `ACTION_SEND` intent filter (`text/plain` for links) | |
| Widgets | `expo-widgets` (+ `@expo/ui`) | Yes: `widgets[]`, `groupIdentifier`; Android is opt-in with `enableAndroid: true` | Widget code runs in an isolated runtime: **no hooks, no app state, no async**, only `@expo/ui/swift-ui` / `@expo/ui/jetpack-compose` (a subset on Android). The app pushes data with `updateSnapshot` / `updateTimeline`. |
| Live Activity | `expo-widgets` (`createLiveActivity`, `.start(props, url)`) | Same plugin | iOS only. |
| Android "Now showing" | **Custom** local Expo module (`modules/now-showing`, Kotlin) | Module | An ongoing, promoted progress notification (Android 16 progress-style "live update" where available, a standard ongoing notification with a progress bar on older versions) with a "Log it" action. `expo-notifications` doesn't cover progress-style ongoing notifications. |
| Location | `expo-location` | Yes (permission strings) | **Foreground only.** No background location or geofencing. |
| Hinge | `expo-hinge` (+ `modules/fold-info`) | Native module | Android 11+, iOS 27.1+. |
| Background refresh | `expo-background-task` | Yes | Flush the outbox, refresh widgets, reschedule episode reminders. |
| Secure storage | `expo-secure-store` | Yes | Keychain / Keystore for the session token. |
| Home-network HTTP | Custom config plugin (`plugins/with-local-http`) | Yes | iOS: `NSAppTransportSecurity` → `NSAllowsLocalNetworking`, plus `NSLocalNetworkUsageDescription`. Android: a network security config allowing cleartext **only** for the configured home host(s), not app-wide. |

### Notifications

**Phase 6a, local:** new-episode alerts are predictable, so the app schedules
local notifications for the next two weeks of episodes of shows you follow
(from `/me/upcoming`), refreshed on every sync and by the background task.

**Phase 6b, push via Expo's push service** for watchlist arrivals: the Go job
posts to `exp.host/--/api/v2/push/send` (batches of 100, receipts after ~15
minutes), optionally with an Expo access token. The app registers with
`getExpoPushTokenAsync()`. Switching to direct FCM/APNs later only changes the
Go sender and which token the app registers.

### Share sheet

The share arrives as text/URL; the app routes to a `share` screen that resolves it:
- TMDB links, IMDb `tt…`, Netflix `title/<id>`: the existing lookup endpoint.
- **Letterboxd** (`letterboxd.com/film/<slug>/`, usually shared as `boxd.it/…`):
  new server resolver (B10). If it fails for any reason, the app searches with
  the slug turned into words.
- Other links (JustWatch, Prime Video, Disney+…): search with the shared text.

Result screen: poster + "Add to watchlist" / "Log it" / "Open".

### Widgets and "Now showing" (phase 7)

- **Up next** (small/medium): next episode of the top "continue watching" show,
  or next airing episode; tap → show page.
- **Break-even** (small): "A-List · 3 of 4 films this month", progress bar,
  saving so far; tap → subscriptions.
- Lock-screen accessory (iOS): next episode inline.
- Data is pushed from the app after each sync and by the background task, from
  one compact **new** `GET /me/glance` response.
- **"Now showing"**: started from the log sheet ("I'm watching this now", with
  an optional start time) or from a cinema pick. Shows title, venue, elapsed /
  remaining runtime; ends with "Log it", which opens the log pre-filled with the
  end time and venue. iOS: Live Activity (lock screen + Dynamic Island).
  Android: the `now-showing` module's ongoing notification. Both are local; no
  server push needed.

---

## 5. Backend changes in this repo (Go)

Follow `CLAUDE.md`, `project-conventions` and `database-changes`: queries in
`db/queries` + `make sqlc`, new migrations numbered after the latest
(`000035_cinema_subscriptions.sql` in this worktree; re-check `main` first),
OpenAPI + README + `docs/architecture.md` updated with each change, `/me`
routes with `RequireUser` + `NoStore` and never `ETag`.

### Verified already

- **Bearer auth works for native clients.** `POST /api/v1/auth/login` returns
  the token in the body "for non-browser clients" and also sets a cookie;
  `Authenticate` accepts `Authorization: Bearer mrs_…`.
- **The CSRF check does not block native clients.** `/api/v1/me` and
  `/api/v1/auth` use `SameOriginWrites`, which only refuses writes carrying a
  cross-site `Sec-Fetch-Site` or a foreign/`null` `Origin`. React Native's
  `fetch` (OkHttp / NSURLSession) sends neither. To pin it down: a router test
  "Bearer write with no `Origin` / `Sec-Fetch-Site` succeeds on `/me/watches`".
  The app sends `credentials: 'omit'` so the login cookie is never stored or
  replayed; the bearer token is the only credential.
- **Idempotent logging exists:** `LogWatch` returns the existing watch for a
  repeated `client_key`.
- **Public exposure:** login is rate-limited per client IP and per username.
  Phase 1 found that with no IP extractor configured, every request through
  a reverse proxy looked like the proxy, so all clients shared one per-IP
  sign-in bucket. Fixed with `SERVER_TRUSTED_PROXIES`: set it to the proxy's
  address so `X-Forwarded-For` (through those proxies only) gives the real
  client IP.

### Additions

| # | Change | Phase | Size |
|---|---|---|---|
| B1 | **Cinemas near me** (**done in phase 1**): `GET /api/v1/me/venues/nearby?lat=&lon=&radius_km=&limit=&include_closed=`. Migration adds an index on `venues(latitude, longitude)`; query `ListVenuesInBox` takes a lat/lon bounding box; Go computes haversine distance, sorts, cuts to `limit`, returns `distance_m` and `screen_count`, and boosts venues the user has logged at. Coordinates must **not** reach logs: strip `lat`/`lon` from logged query strings. | 1 | S |
| B2 | **Device label on sessions**: optional `device_name` in the login body ("iPhone Duo"), stored on `sessions`, shown in Settings → Sessions on web and phone. **Done in phase 1** (API; the web Settings page doesn't show it yet). | 1 | XS |
| B3 | **Remember this device**: `remember_device: true` at login → session with a 180-day absolute life (`AUTH_DEVICE_SESSION_TTL`, default `4320h`) that ends after 90 days unused (`AUTH_DEVICE_SESSION_IDLE_TIMEOUT`, default `2160h`), revocable like any session. Web logins unchanged. **Done in phase 1.** | 1 | S |
| B4 | **Local-URL verification**: login returns `session_id` and `probe_key` (= HMAC-SHA256(stored token digest, "metareel/probe/v1"), recomputed, never stored). `POST /api/v1/auth/probe {session_id, nonce}` (unauthenticated, rate-limited) answers `HMAC-SHA256(probe_key, nonce)`; the app keeps `probe_key` next to its token and only sends the token over the home HTTP URL once the proof matches (§7). Unknown or ended sessions get a non-matching proof, not an error. **Done in phase 1.** | 1 | S |
| B5 | **Poster colour**: average colour of the poster (small TMDB image, decoded in Go) computed when a title's display snapshot is cached; `poster_color` on title payloads. Moved to phase 3, where the first artwork-led screens (Title) arrive. | 3 | S |
| B6 | **Glance**: `GET /api/v1/me/glance`: top Up next items, next airing episode, current subscription period break-even, last watch. For widgets and background refresh. | 7 | S |
| B7 | **Devices**: migration `user_devices` (`id`, `user_id`, `session_id` → `sessions` ON DELETE CASCADE, `platform`, `push_provider` expo, `token` UNIQUE, `app_version`, `created_at`, `last_seen_at`, `disabled_at`). `PUT /me/devices` (upsert by token), `DELETE /me/devices/{id}`, `GET /me/devices`. Tokens kept out of logs. | 6b | S |
| B8 | **Notification preferences** in `user_preferences`: `notify_watchlist_arrivals`; reuse the existing `services` preference + country to filter. | 6b | XS |
| B9 | **Notifications job** `notifications.dispatch` (asynq, hourly): `watch_provider_events` with event `added` since a cursor in `app_settings` (webhooks pattern), joined to watchlisted titles per user, filtered by country/services/monetization (flatrate, free, ads), deduped by `notification_log (user_id, kind, ref) UNIQUE`, sent by a new `internal/client/expopush` (batches, receipts, `DeviceNotRegistered` → disable). Config `PUSH_ENABLED`, optional `EXPO_ACCESS_TOKEN` (never logged; `redactErr`). Shows on the admin schedules page. | 6b | M |
| B10 | **Provider coverage for watchlist titles** (verify first): `watch_provider_events` are written at metadata refresh. If never-charted watchlist titles aren't refreshed, add them to the refresh candidates. | 6b | S |
| B11 | **Letterboxd resolve**: extend the existing lookup: follow `boxd.it` redirects (host allow-list: `boxd.it`, `letterboxd.com`), read the TMDB ID from the film page; on any failure (incl. Cloudflare challenges) return "unresolved". No bot-protection evasion, short timeout, results cached. | 5 | S |
| B12 | **QR sign-in helper** (optional): web Settings shows a QR with the server URL(s), never a token. | 2 | XS |

Each lands with handler/router tests, OpenAPI, README and `architecture.md`
updates, and is verified on a DB copy per `local-verification`.

---

## 6. Where the app lives

**`mobile/` in this repo.**

- API changes and the app that uses them land in one PR; the OpenAPI spec is the contract.
- Not an npm workspace: `mobile/` has its own `package.json` and lockfile.
  Add `mobile/` to `.dockerignore` so the server image is unaffected.
- **Types:** `npm run gen:api` runs `openapi-typescript
  ../internal/openapi/spec/public.yaml -o src/api/schema.ts`; the generated file
  is committed and CI fails if regenerating changes it. `make mobile-types` wraps it.
- **Tokens:** `npm run gen:theme` reads `app/src/tokens.css` into `src/theme/tokens.ts`, same drift check.
- CI uses path filters, so Go-only PRs don't run mobile checks and vice versa.

---

## 7. Networking, builds, signing and distribution

### Reaching the server

- **Primary: the public HTTPS domain.** Valid certificate, so no ATS or
  cleartext exceptions are needed for it.
- **Optional home URL over HTTP** (e.g. `http://homelab.local:5080`), set in
  Settings. Rules:
  - The app probes it (short timeout) on launch, on network change and on
    foreground, and uses it only when it answers **and** passes the B4
    verification. Otherwise everything goes to the public URL.
  - Requests that arrive straight from the LAN (not through the proxy) carry
    the phone's LAN address, so rate limits and session IPs stay correct on
    both paths.
  - The bearer token is **never sent** to the home URL before verification, so
    an impostor answering `homelab.local` on someone else's Wi-Fi learns nothing.
  - Cleartext is allowed only for that host (Android network security config)
    and local networking (iOS `NSAllowsLocalNetworking`); iOS asks once for
    Local Network permission.
  - Accepted risk: on the home network the token travels unencrypted. Anyone
    able to sniff your home Wi-Fi could reuse it until it's revoked. The toggle
    is off by default.
- **No server URL is baked into builds**: the repo, the JS bundle and the
  released APK are public, so the domain is entered at sign-in (or scanned
  from the B12 QR code).

### Builds

| Need | How |
|---|---|
| Day-to-day development | Local dev builds on the Mac (`npx expo run:android`, `npx expo run:ios`) into the emulator/simulator, then Metro hot reload. |
| iOS releases | **EAS Build (cloud)** → `eas submit` → **TestFlight** (internal testers, no review). Credentials managed by EAS. |
| Android releases | **EAS Build (cloud)** produces a signed release **APK** with the EAS-managed keystore; a GitHub workflow attaches it to a GitHub Release (below). |
| CI checks | **GitHub Actions** on PRs touching `mobile/`: typecheck, lint, Jest, OpenAPI/token drift. |
| JS updates | GitHub Action on merge to `main` with `mobile/` changes → `eas update --channel preview`; promote to `production` by hand. |

EAS free tier: 15 Android + 15 iOS cloud builds a month, which is plenty;
`eas build --local` on the Mac is the unlimited fallback.

### Android: APK on GitHub Releases

- Tag `mobile-v1.2.0` → workflow `mobile-release.yml` runs
  `eas build -p android --profile release-apk --non-interactive --wait` (the
  build runs on EAS, so the **keystore never touches GitHub**), downloads the
  finished APK from EAS and attaches it with `gh release upload` to the
  `mobile-v1.2.0` release, with its SHA-256 in the notes. Only `EXPO_TOKEN` is
  needed as a GitHub secret.
- Release assets are the intended public channel; `actions/upload-artifact`
  is still never used for signed builds.
- **The APK is public**, so it must contain no secrets: no server URL, no
  tokens. `google-services.json` (for FCM in phase 6b) does end up in the APK;
  its Firebase API key is not a secret by design, but restrict it in Google
  Cloud to the package name and the signing certificate's SHA-1.
- **Updating:** JS changes arrive through EAS Update. For a new native build,
  the app checks the latest `mobile-v*` release via the GitHub API on launch
  (at most daily) and shows "Update available" in Settings with a link to the
  APK. Sideloading needs "Install unknown apps" allowed once for the browser.
- Back up the EAS-managed Android keystore to your password manager: losing it
  means existing installs can't be updated in place.

### iOS: TestFlight

Internal testing (up to 100 people on your App Store Connect team, no review)
for you and anyone you add. App Store Connect API key stored as an EAS secret
for `eas submit`. External testers would need beta review; not planned.

### Signing summary (public repo)

| Secret | Where |
|---|---|
| Android keystore, iOS certificates/profiles | EAS-managed credentials (+ your own Android keystore backup) |
| App Store Connect API key | EAS secret |
| `google-services.json` | EAS file secret (phase 6b) |
| `EXPO_TOKEN` | GitHub secret, used only on `push` to `main` and on `mobile-v*` tags, never on `pull_request` |

---

## 8. Phased roadmap

Estimates are focused working days for one developer with Claude doing most
of the typing. "S" ≈ ½–1 day, "M" ≈ 2–3 days.

| Phase | Contents | Effort | Exit check |
|---|---|---|---|
| **1. Backend prerequisites** | B1 nearby venues, B2 device label, B3 remember-device sessions, B4 local-URL probe, login rate-limit review for the public deployment, CSRF/bearer router test, OpenAPI. Expo account + EAS project; bundle ID registered. | 3–4 | Go tests + live check on a DB copy; probe verified from a script. |
| **2. App foundation** | Scaffold `mobile/`, config plugins (incl. `with-local-http`), fonts, theme generator, API types, sign-in + secure store + endpoint switching, TanStack Query + SQLite persistence, adaptive shell (headless tabs as bar/rail), B5 poster colour, first TestFlight build and first GitHub Release APK, EAS Update channels, CI. | 6–8 | Sign in on the iOS simulator (via TestFlight-equivalent build) and Android emulator; switches between home and public URL; shell resizes on fold/unfold without losing state. |
| **3. Core phone flows** | Tonight with Top 10 rail, quick log, full log sheet (venue nearby, formats, money, subscription covered), outbox + offline banner, Up next + Show ticks, Watchlist, Diary, Title (lean), Search, Subscriptions, Settings (sessions, update check). | 10–14 | Maestro: sign in → airplane mode → quick log → online → entry on the web Diary exactly once. |

| **4. Large screens & foldables** | List–detail on medium/expanded, rail, side-panel log sheet, `expo-hinge` + `fold-info`, tabletop layout, iPhone Duo pass, multi-window/freeform, iPad pass. | 4–6 | §2 test matrix passes; no state loss on fold/unfold mid-log. |
| **5. Share sheet** | `expo-sharing` receive on both platforms, resolve screen, B11 Letterboxd. | 2–4 | Shared Netflix, IMDb and Letterboxd links resolve or fall back to search. |
| **6. Notifications** | 6a: local episode reminders + background task. 6b: B7–B10, Firebase project, Expo push, permission UX, settings switches. | 2 + 4–5 | A watchlist title arriving on one of your services notifies once and deep-links to the title. |
| **7. Widgets & "Now showing"** | B6 glance, Up next + Break-even widgets (iOS, then Android), lock-screen accessory, iOS Live Activity, Android `now-showing` module. | 6–9 | Widgets update after a log and after a background refresh; "Now showing" ends with a pre-filled log on both platforms. |

**Phase 3 status (2026-11-14):** built and checked on the Android emulator:
Tonight (Top 10 rail, Up next with ticks, recent logs, Log a film), search
(TMDB titles and IMDb/TMDB/Netflix links; films and series interleaved),
the log sheet (when, stars, liked, cinema from GPS with an offline-ranked
cache, formats, price or subscription face value, streaming service, notes;
drafts survive restarts), the SQLite outbox (logs, episode ticks and watchlist
changes; verified offline → online with one server row per log), Diary
(grouped by the day a watch started, pending rows on top), a watch's page
(edit rating/liked/notes, delete), Title (poster-tinted hero via B5, scores,
where to watch with service logos, chart positions), Show (seasons, tick,
long-press to mark up to, unmark), Up next stages, Watchlist,
Subscriptions (break-even, savings), Settings (subscriptions, APK update
check), and a background task that flushes the outbox. Not done: Maestro
flows (the CLI isn't installed; the offline check was run by hand), the
side-panel log sheet on wide windows (phase 4), and logos for IMDb / RT /
TMDB scores (the web app is getting them; reuse when they land).

**Phase 4 status (2026-11-14):** Diary, Up next and Watchlist are
list–detail on medium and expanded windows (the detail views are shared with
the full screens); the log sheet is a side panel beside the dimmed content on
wide windows and a bottom sheet on phones; `modules/fold-info` (Android, ~90
lines of Kotlin) reports the fold's bounds and posture, used to put the
list–detail split on the hinge of a half-open book foldable (offset by the
rail) and to give the log sheet a tabletop layout (title and artwork above the
fold, form below). Checked on the Pixel 9 Pro Fold emulator: unfolded, half
open at 90°, tabletop in landscape. iOS: the iPhone Duo's fold position isn't
exposed (only `expo-hinge`'s status/angle), so the iOS split is the
size-class one. Density changes still recreate the activity (see section 2).

**Phase 5 status (2026-11-14):** metareel is a share target on both
platforms (`expo-sharing`: iOS share extension for one URL or text, Android
`SEND text/plain`). `+native-intent.tsx` routes the share to `/share`, which
pulls the first link out of the shared text (`src/lib/share.ts`: TMDB, IMDb,
Netflix and Letterboxd links, boxd.it short links) and resolves it through
`GET /titles/lookup`; anything else, or a link that doesn't resolve, offers
search with the shared words. B11 is `?letterboxd=` on the lookup: the server
fetches the film page (following boxd.it redirects by hand, Letterboxd hosts
only) and reads its TMDB id, caching hits for a day and misses for an hour.
Checked on the Android emulator against the test server: a Letterboxd link
(live fetch), a Netflix link with tracking params, and plain text (search).
The iOS share extension is built by the plugin but was not exercised on the
simulator.

v1 worth using day to day = phases 1–4, about **5–7 weeks of focused days**.
Phases 5–7 are independent and can be reordered.

### Risks

| Risk | Impact | Mitigation |
|---|---|---|
| `expo-widgets`, `expo-hinge`, `@expo/ui` are young APIs; Android widgets support only a Compose subset | Rework on SDK upgrades | Keep widgets small and isolated; check release status at phase 7. |
| No physical devices | Performance, GPS-indoors and OEM issues found late | Long-list profiling on the emulator; Samsung Remote Test Lab spot checks; quick fixes via EAS Update. |
| Home-network HTTP | Token visible on the home LAN | Off by default, verified before use, only for the configured host; revoke the device session if in doubt. |
| Public server + 180-day sessions | A stolen phone keeps access longer | Revocable device sessions with names; rate-limited login; sign-out removes push devices (cascade). |
| Public APK | Anyone can install it | Harmless without an account; no secrets or URLs inside; Firebase key restricted. |
| Watchlist arrival events only exist for refreshed titles | Missed notifications | B10 check before 6b. |
| Letterboxd behind Cloudflare | Share resolve fails | Always fall back to search; no evasion. |
| Android 16 progress notifications vary by OEM | "Now showing" looks different on Samsung | Fall back to a standard ongoing notification with a progress bar. |
| Three front-ends (admin, web app, mobile) | Drift | Generated API types and tokens with CI drift checks; phone scope kept small. |
| Background task timing is up to the OS | Widgets/reminders lag | Refresh on every app open; reminders scheduled two weeks ahead. |
| Location privacy | Coordinates in logs | Foreground only, never stored server-side, stripped from request logs. |

---

## 9. Still open (small, can be decided during the phases)

- **Notification timing** (6a): episode reminders at a fixed local hour (e.g.
  18:00) or at the episode's air time when TMDB has one?
- **Top 10 rail default**: which service and country by default (your
  `services` preference's first entry, or Netflix)?
- **"Now showing" start**: should picking a showtime later (e.g. from a cinema's
  listings) ever start it automatically? Today the plan only starts it by hand.
