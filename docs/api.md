# API

The public API, the per-user tracking API and webhooks. The OpenAPI specs are the complete reference: Swagger UI at `/docs` (public) and `/docs/admin` (admin) on a running server, sources in `internal/openapi/spec/`.

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
| POST   | `/api/v1/auth/probe`                        | Unauthenticated: proves to the mobile app that a server on its home-network URL is the one it signed in to ([users](#users)) |
| GET    | `/api/v1/auth/me`                           | Who the request is authenticated as (user or API key) |
| GET    | `/api/v1/auth/session`                      | For the console: whether admin sign-in is required and who is signed in |
| GET    | `/api/v1/top10/{movies\|tv-shows}/:country[/:provider]` | Top 10 charts (`date` optional = latest; entries carry `previous_rank`, `days_in_top10`, the TMDB `poster_path` once the title has metadata, for TV `season_number` + `season_source` when known, and the [title facts](#title-facts-on-chart-rows)) |
| GET    | `/api/v1/top10/:country`                    | Every provider's movie + TV chart of a country in one call |
| GET    | `/api/v1/top10/{movies\|tv-shows}/:country/:provider/history` | One chart over a range (`from`, `to`; ≤ 92 days) |
| GET    | `/api/v1/top10/global/:provider`            | A provider's titles across all countries by points (`date`, `category`, `kind`) |
| GET    | `/api/v1/charts`, `/api/v1/charts/:country/:provider/dates` | Stored charts with date ranges and freshness; stored dates |
| GET    | `/api/v1/changes?since=`                    | Charts stored / re-scraped since a time (poll this) |
| GET    | `/api/v1/movers`                            | Climbers, fallers, debuts, re-entries, exits (`country`, `provider`, `date`). Like every chart-derived list (history, breakouts, leaderboards, analytics items), each title carries `poster_path` (TMDB path, null until it has metadata); history, movers, breakouts, leaderboards and the Netflix Top 10 also carry the [title facts](#title-facts-on-chart-rows) |
| GET    | `/api/v1/discover/:country/hidden-gems`     | Highly rated titles (IMDb ≥ `min_rating` 7.5 from ≥ `min_votes` 5,000) streamable in the country that weren't in its Top 10 over `days` (90), ranked by a vote-weighted rating (`kind`, `provider_id`, `limit`) |
| GET    | `/api/v1/discover/:country/elsewhere`       | Titles in other countries' Top 10s over `days` (7) with no subscription / free / ads offer in the country, with where they chart and rent / buy offers here (`kind`, `limit`) |
| GET    | `/api/v1/discover/:country/rotation`        | Which subscription services to pay for (plans and channels grouped per service; `include_other=true` adds TV bundles and niche services): each scored on its titles' chart points, upcoming episodes, arrivals and top-rated titles over `days` (30), with `subscribe` / `skip` (or `keep` / `pause` / `add` given `subscribed=8,9`) for the best `slots` (2); each service carries `logo_path` and `chart_provider` |
| GET    | `/api/v1/breakouts`                         | Titles rising in several countries at once: entered a chart or climbed `min_climb`+ places (default 3) over `window` days (1–14, default 3) in at least `min_countries` countries (default 2); `date`, `category`, `provider`, `kind`, `limit` |
| GET    | `/api/v1/leaderboards`                      | Titles by chart points (`period=day\|week\|month\|custom`, filters) |
| GET    | `/api/v1/export/rankings`                   | Chart entries as CSV / NDJSON (≤ 366 days) |
| GET    | `/api/v1/countries`, `/api/v1/providers`    | Supported countries; providers with stored charts, each with `tmdb_provider_id` (main), `tmdb_provider_ids` (every tier that counts as it) and TMDB's `logo_path` |
| GET    | `/api/v1/titles`, `/api/v1/titles/:id`      | Scraped titles (paged; `match_status` filter); a single mapped title carries its TMDB title's `ids` |
| GET    | `/api/v1/search?q=&kind=movie\|tv&limit=` | Find titles by any name (TMDB title / original title, FlixPatrol and Netflix names); accent- and punctuation-insensitive, word prefixes ("strang thin"), best match then most recently charted first |
| GET    | `/api/v1/titles/lookup?imdb=\|tmdb=\|slug=\|netflix=\|letterboxd=` | Find a title by any ID (`netflix` = the number in netflix.com/title/<id>; `letterboxd` = a boxd.it or letterboxd.com film link, resolved through the film page's TMDB id) |
| GET    | `/api/v1/version` | The running server's version, git revision and `features` (optional API capabilities, e.g. `chart_title_facts`, so clients need not guess from missing fields) |
| GET    | `/api/v1/titles/trending?kind=all\|movie\|tv` | What is trending on TMDB today (live, cached 30 minutes); items carry `overview`, and titles metareel knows `season_count` (TV) and `logo_path` |
| GET    | `/api/v1/titles/in-cinemas/:country`        | Films in cinemas in a country, from TMDB's now playing (live, cached 30 minutes), with `overview` and, for films metareel knows, `logo_path` and the country's `certification` |
| GET    | `/api/v1/titles/tmdb/:kind/:id`             | Everything about a title: TMDB metadata, chart stats, current positions, ratings, where to watch, Netflix numbers (`include=`; `rankings`, `timeline` and `releases` only when asked for, per `countries=IN,US`, at most 5), and `ids` — every known ID (IMDb, Wikidata, Rotten Tomatoes, Netflix, JustWatch, FlixPatrol slugs, Netflix Top 10 names) |
| GET    | `/api/v1/titles/tmdb/:kind/:id/stats`       | Chart performance: points, peak, days at #1, debut, streak, spread across countries |
| GET    | `/api/v1/titles/tmdb/:kind/:id/availability` | Watch providers per country (TMDB, data by JustWatch), each with `url` to the title on that service and, for charted services, `chart_provider` |
| GET    | `/api/v1/titles/tmdb/:kind/:id/availability/history` | When the title arrived on / left each service per country (`added` / `removed` between two TMDB refreshes; `country` optional) |
| GET    | `/api/v1/titles/tmdb/:kind/:id/history`     | Ratings, TMDB popularity and English Wikipedia pageviews over time (`from`, `to`; default last 90 days, ≤ 366): IMDb dataset, each provider's rating sources (a point when a value changed) and daily TMDB popularity |
| GET    | `/api/v1/titles/tmdb/:kind/:id/credits`     | Top 25 cast and key crew (directors, writers, creators, composers, cinematographers) from TMDB; TV with episodes credited |
| GET    | `/api/v1/people/:id`                        | A TMDB person: profile and full filmography live from TMDB (cached 12 h; `details_status` says why they're missing), and their titles metareel knows with roles and each title's chart record (points, days, best rank, countries); `seasons=true` adds `filmography_seasons` (every TV credit once per season: season number, name, air date, the person's episode count, season poster, roles) and `seasons_status` |
| GET    | `/api/v1/titles/tmdb/movie/:id/releases`    | A movie's premiere, theatrical (limited / wide), digital (PVOD), physical and TV dates per country from TMDB, the observed streaming arrival (first offer seen arriving, or first chart day) and the windows between them in days (`country`) |
| GET    | `/api/v1/titles/tmdb/:kind/:id/timeline`    | Everything that happened to a title, newest first: chart debuts, peaks, #1 runs and re-entries, availability added/removed, rating changes, Wikipedia pageview spikes, Netflix Top 10 weeks and engagement periods, season premieres, release (`types`, `country`, `from`, `to`, `limit`) |
| GET    | `/api/v1/calendar/:country`                 | Upcoming episodes of shows streamable in the country (subscription / free / ads), one entry per show, season and day with the services (`from`, `to`: default the next 14 days, ≤ 62; `provider_id`) |
| GET    | `/api/v1/availability/changes`              | Titles that arrived on / left a service across titles (`since` RFC 3339, default 7 days; `country`, `event=added\|removed`, `monetization`, `provider_id`, `limit`); each event carries the provider's `logo_path` and, for charted services, `chart_provider` (the `/providers` slug) |
| GET    | `/api/v1/titles/tmdb/:kind/:id/netflix`     | Weeks in Netflix's official Top 10, with a per-season summary (`seasons`) for TV |
| GET    | `/api/v1/titles/tmdb/:kind/:id/rankings`    | Ranking history by TMDB ID (`kind` = movie or tv; `country`, `from`, `to` optional) |
| GET    | `/api/v1/titles/tmdb/rankings?ids=movie:425,tv:1` | Batch ranking history (max 50 ids)    |
| GET    | `/api/v1/titles/tmdb/:kind/:id/ratings`     | IMDb + Rotten Tomatoes ratings by TMDB ID, cached for `RATINGS_TTL` (`?refresh=true` to force); IMDb's own dataset wins for IMDb |
| GET    | `/api/v1/titles/tmdb/ratings?ids=`          | Batch ratings (max 50; refreshes up to 10 uncached) |
| GET    | `/api/v1/netflix/top10[/:country]`, `/api/v1/netflix/most-popular` | Netflix's official weekly Top 10 and most-popular lists (rows carry `season_number` / `season_source`, and the mapped TMDB title's `poster_path`) |
| GET    | `/api/v1/analytics/{decay,country-similarity,release-lag,ratings-vs-popularity,genres,netflix-calibration}` | Analytics over a range (`from`, `to`, `country`, `provider`, `category`, `kind`) |
| GET    | `/api/v1/analytics/release-patterns`        | How long TV seasons stay in the Top 10 by release pattern (`binge` / `batch` / `weekly` / `daily` / `irregular`, from TMDB episode air dates), over `window` days (30–180, default 60) after each premiere; `from`, `to` = premiere range (default last 365 days), `country`, `provider` |
| GET    | `/api/v1/analytics/spread-lag`              | How many days after a title first charts in one country it charts in the others: overall and per-country lags, origin → destination routes, titles never reached within `window` days (7–90, default 30). Debuts between `from` and `to` (default last 90 days, ≤ 180); `country` = debuted there; `provider`, `category`, `kind` |
| GET    | `/api/v1/analytics/people`                  | People ranked by the Top 10 points of their titles in a range (`role` = actor (top-10 billed) / director / writer / creator / composer / cinematographer; `from`, `to`, `country`, `provider`, `category`, `kind`, `limit`) |
| GET    | `/api/v1/analytics/release-windows`         | Theatrical → digital (PVOD), theatrical → streaming and digital → streaming windows of movies released in theaters in `country` (default US) between `from` and `to` (default last 365 days): medians, quartiles, how many were ≤ 17 / 31 / 45 / 90 days, per streaming service, and each movie (`limit`, ≤ 500) |
| GET    | `/api/v1/analytics/origins`                 | Each service's chart points by origin — `original` / `in_house` (parent company's studios and channels) / `licensed` / `unknown` — per week or month (`interval`), with the owners of licensed titles and top titles. Chart entries carry the same `origin` |
| GET    | `/api/v1/titles/new/:country/:service`      | Titles added to a service per day, live from JustWatch (`from`, `to` default last 7 days; `type`) |
| GET    | `/api/v1/titles/upcoming/:country/:service` | Titles announced for a service, live from JustWatch (`from`, `to`, `type`) |
| GET    | `/api/v1/services/:country`                 | JustWatch service codes in a country (for `:service`), each with its `tmdb_provider_id`, TMDB `logo_path` and, for charted services, `chart_provider` |
| GET/POST/PATCH/DELETE | `/api/v1/admin/...`           | Admin API (admin sign-in with `AUTH_REQUIRE_ADMIN_LOGIN`): schedules, runs, stats, API keys, users (`/users`, `/users/:username`, `/users/:username/logout`) and sessions (`/sessions`, `DELETE /sessions/:id`), settings, title corrections (`PATCH /api/v1/admin/titles/:id`, `POST .../rematch`), `POST /jobs/:type/run`, `POST /backfill`, `GET /data-quality`, `GET /task-types`, and admin copies of the read routes above |
| GET/PATCH/POST | `/api/v1/admin/netflix/titles[/:id[/rematch\|/candidates]]` | Netflix official Top 10 titles: list (`q`, `kind`, `match_status`, `missing=tmdb\|season`, `flag=cross_kind\|linked`, `sort`), detail with season titles, weeks and linked FlixPatrol titles, fix by hand (`tmdb_kind` + `tmdb_id`, or an IMDb / Wikidata ID resolved to TMDB), rematch, candidate search |
| GET/POST | `/api/v1/admin/mappings[/sync\|/:kind/:id[/remap]]` | The TMDB-keyed mapping: every TMDB title with all its IDs and the FlixPatrol / Netflix titles mapped to it (`q`, `kind`, `flag=conflict\|cross_kind\|both\|netflix_only\|flixpatrol_only\|manual`); `sync` recomputes links now; `remap` moves everything on a TMDB title to another (by TMDB, IMDb or Wikidata ID) |
| GET    | `/api/v1/admin/mappings/review/:subject/:id/suggestions` | Other TMDB titles a queued mapping could be, ranked (name, kind, release year against the first chart, an offer on the charted service), for the queue's one-click fix |
| GET    | `/api/v1/admin/mappings/review?subject=flixpatrol\|netflix&reason=&min_risk=25` | Mapping review queue: every automatic mapping scored 0–100 by how likely it is wrong (not on the service it charted on, released after it charted, IMDb / year / name disagreement, weak matching rule), riskiest first, with the reasons |
| GET    | `/api/v1/admin/mapping-refs?tmdb_kind=movie\|tv&mapped=true` | The mapping reference (view `mapping_refs`): one row per chart title with its TMDB title and every ID (IMDb, Wikidata, RT, Metacritic, Letterboxd, JustWatch, Netflix), FlixPatrol slug / kind and Netflix title / kind |
| GET    | `/api/v1/admin/mapping-events?subject=flixpatrol:12\|netflix:34&tmdb=movie:550&limit=` | History of mapping changes, newest first: action, TMDB title before / after, source and who made it (job, `mapping.sync`, `admin`) |
| PATCH  | `/api/v1/admin/netflix/seasons/:id`         | Set a Netflix season title's season number by hand (`{"season_number": 2}`) or hand it back (`{"auto": true}`) |
| GET/POST/PATCH | `/api/v1/admin/watch-formats[/:id]`  | The premium-format taxonomy (console: Premium formats): cinema axes (`brand`, `projection`, `sound`, `motion`, `dimension`) plus home media (`disc`: DVD, Blu-ray, 4K UHD Blu-ray; `label`: Criterion, Arrow…). A format is retired with `active: false`, never deleted. `logo_display` is `original` or `mono` (clients draw a dark logo white). Logos are uploaded in the console (one at a time, or in bulk as `<slug>.<ext>` files: PNG, SVG, WebP, JPEG or GIF, at most 512 KB) and stored in the database; they are trademarks, so they are not kept in the repo |
| POST   | `/api/v1/admin/venues/:id/merge`            | Merge a duplicate cinema into another (`{"into": id}`): watches, screens, former names and outside ids move, its name becomes a former name, it is deleted |
| GET/POST/PUT/DELETE | `/api/v1/admin/venues[/:id[/screens[/:screen_id]\|/aliases[/:alias_id]]]` | Cinemas for the console's Cinemas page: search, add, correct, close (status `closed`), delete one nobody watched at; screens and former names. The same handlers as `/api/v1/me/venues` |
| PUT/DELETE | `/api/v1/admin/watch-formats/:id/logo`   | Upload a format's logo (raw image body: PNG, JPEG, WebP or GIF ≤ 4096 px a side, or an SVG of drawing elements only; ≤ 512 KB; type taken from the bytes) or remove it |
| GET/POST/PATCH/DELETE | `/api/v1/admin/webhooks[/:id[/test\|/rotate-secret\|/deliveries]]`, `/api/v1/admin/webhook-deliveries/:id[/retry]` | Outbound webhooks (see [Webhooks](#webhooks)) |
| GET    | `/api/v1/me/titles/search?q=&kind=`         | Search TMDB for anything to track (not only titles metareel has charted); needs a signed-in user |
| GET    | `/api/v1/me/people/search?q=`               | Search TMDB for actors and crew by name (opens `/people/:id`); needs a signed-in user |
| GET    | `/api/v1/me/titles`                         | The user's tracked titles (`status`, `kind`, `favorite`, `sort`, `limit`, `offset`) |
| GET/PUT/DELETE | `/api/v1/me/titles/{movie\|tv}/:id`  | One title: its TMDB metadata plus the user's status, favourite flag, watchlist note and watch roll-up; GET adds `poster_color` (#rrggbb, the poster's average colour, cached) for artwork-tinted screens |
| GET    | `/api/v1/me/watchlist`                      | The watchlist, in list order |
| PUT/DELETE | `/api/v1/me/watchlist/{movie\|tv}/:id` | Put a title on the watchlist or take it off (watches and favourites are kept) |
| GET/POST | `/api/v1/me/watches`                      | The watch log (diary), newest local day first (cursor paged); log a watch |
| GET/PUT/DELETE | `/api/v1/me/watches/:id`            | One watch: read, replace, or delete it for good |
| GET    | `/api/v1/me/tags`                           | The user's tags with how many watches carry each |
| DELETE | `/api/v1/me/tags/:id`                       | Delete a tag and detach it from every watch |
| GET    | `/api/v1/me/account`                        | Your user and where you're signed in (sessions, `current` marks this one) |
| POST   | `/api/v1/me/password`                       | Change your password (`current_password`, `new_password`); ends your other sessions |
| DELETE | `/api/v1/me/sessions[/:id]`                 | Sign one of your other sessions out, or (no id) every session but this one |
| GET/PUT | `/api/v1/me/preferences`                   | Preferences that follow you: default `country` + `provider`, `currency`, `screen_unit` (m/ft), `stale_days` for `/me/shows`, `services` you pay for (TMDB provider IDs, as `tmdb_provider_id` / `provider_id` elsewhere), `countries` in your order (at most 20), `notify_watchlist_arrivals`; PUT replaces them. Clients that sync these (the apps, Tides) sign in as the user |
| GET    | `/api/v1/me/glance`                         | Widget summary: next episodes of up to 3 shows in progress, the next one airing, the cinema plan's month, the last watch |
| GET/PUT | `/api/v1/me/devices`                       | Phones registered for push notifications (mobile app); PUT registers this device's Expo push token for this session |
| DELETE | `/api/v1/me/devices/:id`                    | Unregister one of your devices |
| GET    | `/api/v1/me/shows`                          | Every show you track, sorted into stages (`new_episodes` for a series or season out within `stale_days`, `watching`, `stalled` after `stale_days` (30) without a watch, `not_started`, `on_hold`, `dropped`, `caught_up`) with progress, the next episode and the next air date |
| GET    | `/api/v1/me/upcoming`                       | Episodes still to air of your shows, soonest first (`days`: 90, ≤ 365); on hold, dropped and unwanted shows left out |
| GET    | `/api/v1/me/shows/:id`                      | A show with the user's progress: seasons, episodes, watch counts (`include_specials`) |
| POST   | `/api/v1/me/shows/:id/watched`              | Mark episodes watched: `scope` = `episode` / `season` / `up_to` / `all` |
| DELETE | `/api/v1/me/shows/:id/watched`              | Unmark episodes (`scope`, `season`, `episode`, `include_specials`); deletes their watches |
| GET    | `/api/v1/me/up-next`                        | The next episode to watch of each show in progress |
| GET/POST | `/api/v1/me/imports`                      | Your imports; POST uploads a TV Time or Suite export zip as the body (`tz`, `file_name`; 64 MB max) and answers 202 while titles are matched to TMDB in the background |
| GET/DELETE | `/api/v1/me/imports/:id`                | One import with its progress (`matching` -> `ready` -> `importing` -> `done`); DELETE forgets it, keeping what it wrote |
| GET    | `/api/v1/me/imports/:id/items`              | Its titles for review (`kind`, `match` = unmatched / matched / skipped / pending, `limit`, `offset`), with TMDB candidates for unmatched ones |
| PUT    | `/api/v1/me/imports/:id/items/:item_id`     | `{"tmdb_id"}` matches a title by hand, `{"skip": true|false}` leaves it out or puts it back (while `ready`) |
| POST   | `/api/v1/me/imports/:id/commit`             | Write the matched titles (202). Watches you already have are skipped, so the same history from two apps only fills gaps |
| POST   | `/api/v1/me/imports/:id/retry`              | Resume a failed import |
| POST   | `/api/v1/me/imports/:id/undo`               | Delete what the import wrote and put each title's state back |
| GET    | `/api/v1/me/discs`                          | Your physical media: each copy with its full release (parts, TMDB titles, discs, pictures) and totals (copies, discs, titles, box sets, by format, spend per currency, retailers). `format`, `q`, `tmdb_kind` + `tmdb_id` (box-set parts included), `sort` = added / title / purchased / price / format |
| GET    | `/api/v1/me/discs/lookup`                   | A scanned `barcode` (EAN-13, UPC-A, EAN-8, GTIN-14): `known` with the release metareel has and your copies of it, else `found` with a draft from UPCitemdb (format, edition, Steelbook, box set, seasons, retailer pictures) and TMDB title / collection candidates, or `not_found` / `rate_limited` / `unavailable`. Answers are cached per barcode |
| POST   | `/api/v1/me/discs`                          | Add a copy: `release_id` (a known release) or `release` (a new one; `parts` make a box set), the copy's `price_minor` + `currency`, `retailer`, `purchased_on`, `condition` (sealed / new / used), `notes`, and `import_images` (`{url, kind, release_id?}` from the lookup, or TMDB images). 409 when the barcode belongs to a release already |
| GET/PUT/DELETE | `/api/v1/me/discs/:id`              | One of your copies; PUT changes the copy's own fields; DELETE takes it off your shelf (the release stays) |
| GET    | `/api/v1/me/disc-collections`               | TMDB collections matching `q`, each with its films, to fill a box set |
| GET/PUT | `/api/v1/me/disc-releases/:id`             | A release with your copies of it; PUT replaces it (titles, discs, specs, special features, parts by id). Releases are shared: only someone with a copy may edit one (403 otherwise) |
| POST   | `/api/v1/me/disc-releases/:id/images`       | Upload a picture (`kind` = front, back, slip_front, slip_back, spine, disc, booklet, inside, other) as the body: JPEG / PNG / WebP / GIF, 25 MB and 50 MP max. Turned upright, scaled to 2000 px (plus a 480 px thumbnail) and re-encoded, which drops EXIF (a phone photo's location) |
| POST   | `/api/v1/me/disc-releases/:id/images/import` | Fetch `{url, kind}`: only pictures the release's cached barcode lookup offered, or TMDB images; never a local or private address |
| PUT/DELETE | `/api/v1/me/disc-releases/:id/images/:image_id` | Change a picture's `kind` / `position`, or delete it (its file goes when nothing else uses it) |
| GET    | `/api/v1/disc-images/:name`                 | A stored picture (`<sha256>[_thumb].jpg|png`, the URLs in `images[]`). Signed-in users only; cacheable for good (`private, immutable`) |
| GET    | `/api/v1/me/formats`                        | The premium formats a watch can carry (IMAX, Dolby Cinema, 4DX, 70mm, Atmos…), each with a versioned `logo_url` once it has a logo |
| GET    | `/api/v1/formats/:slug/logo`                | A format's logo image (use the format's `logo_url`; cached for a year at its current `?v=`, served with a sandboxing CSP) |
| GET/POST | `/api/v1/me/venues`                       | Search cinemas (`q`: every word in the name, chain, city, address or postcode, e.g. `pvr mumbai` or `amc 10036`; `country`, `include_closed`); add one |
| GET    | `/api/v1/me/venues/nearby`                  | Cinemas near `lat`/`lon` (`radius_km` default 10, max 100; `limit`; `include_closed`), nearest first with `distance_m`; ones you watched at win ties within 250 m. The position is never stored and is redacted in the request log |
| GET/PUT/DELETE | `/api/v1/me/venues/:id`             | One cinema with its screens, former names and outside ids; delete is refused while watches point at it |
| POST/DELETE | `/api/v1/me/venues/:id/aliases[/:alias_id]` | Record or remove a name a cinema used to have |
| POST   | `/api/v1/me/venues/:id/screens`             | Add an auditorium (size, seats, aspect ratio) |
| PUT/DELETE | `/api/v1/me/venues/:id/screens/:screen_id` | Edit or remove an auditorium |
| GET/POST | `/api/v1/me/subscriptions`                | Your cinema subscriptions (plan, chain, price per month or year with its `prices` over time, `periods` it ran: cancel and resubscribe leaves an unbilled gap) and well-known plans; add one |
| PUT/DELETE | `/api/v1/me/subscriptions/:id`          | Replace (e.g. end, resubscribe, change the price) or delete a plan; deleting keeps its watches, uncovered |
| GET    | `/api/v1/me/subscriptions/savings`          | Per plan: bills paid (at the price then, none in gaps) against covered tickets' face value (or your usual ticket price), extras, savings, break-even, month by month; and what a plan would have to cost at chains you paid at |
| GET    | `/api/v1/me/stats/summary`                  | Totals, where you watched, formats, spend (`from`, `to`) |
| GET    | `/api/v1/me/stats/calendar`                 | Day-by-day heatmap and streaks |
| GET    | `/api/v1/me/stats/year`                     | Year in review (`year`, default this year) |
| GET    | `/api/v1/me/stats/ratings`                  | Your rating distribution and how it compares with TMDB, IMDb and RT |
| GET    | `/api/v1/me/stats/spend`                    | Money per currency, per cinema, per format |
| GET    | `/api/v1/me/stats/people`                   | Top cast or crew (`job=Director`, `limit`) |
| GET    | `/api/v1/me/stats/detailed`                 | Everything about your shows or films (`kind=tv\|movie`, empty for both): time, trends, top titles, streak, genres, people, binges; for shows also stages, time left, upcoming and a catch-up date |
| GET    | `/api/v1/me/watchlist/insights`             | What your watchlist is doing on the charts and on streaming (`country`, `days`) |
| GET    | `/admin` (and `/admin/...`)                 | The admin console SPA from `web/dist/` (`UI_STATIC_DIR`) |
| GET    | `/` (and any unmatched path)                | The user app SPA from `app/dist/` (`APP_STATIC_DIR`); redirects to `/admin/` when the app isn't built |

### Title facts on chart rows

Chart rows (Top 10s, chart history, movers, breakouts, leaderboards, the global
Top 10 and Netflix's weekly Top 10) carry the title's stored TMDB facts, so a
client needs no TMDB lookup per row: `tmdb_kind` (`movie` / `tv`, the spelling
TMDB-keyed URLs use), `backdrop_path`, `overview`, `release_date`, `year`,
`runtime` (TV: episode runtime), `season_count` (TV), `genres`, `logo_path`
(the title's logo, English else the original language) and `certification`
(the age rating in the chart's country; omitted on breakouts, the global Top 10
and leaderboards without `country`). Each is omitted when unknown; everything but `tmdb_kind` is omitted until the title has
TMDB details. `include=ratings` adds `ratings` (stored IMDb and Rotten Tomatoes
headline values with `refreshed_at` and `stale`); they are never fetched for
the request (`ratings.prewarm` keeps charting titles fresh). Any other
`include` value is a 400.

GET responses under `/api/v1` carry an `ETag`; send it back as
`If-None-Match` to get `304 Not Modified` when nothing changed. The one
exception is `/api/v1/me/...`: those responses are per-user, so they carry
`Cache-Control: private, no-store` and no `ETag` instead.

## Public API access

Every `/api/v1` route outside `/api/v1/admin` and `/api/v1/auth` needs
credentials: an **API key** or a **signed-in user's session**. Both work on
every route. Health checks and `/docs` stay open.

| Credential | How to send it |
| --- | --- |
| API key (`mr_…`) | `X-API-Key: <key>` or `Authorization: Bearer <key>` |
| Session token (`mrs_…`) | `Authorization: Bearer <token>` (from `POST /api/v1/auth/login`) |
| Session cookie | `metareel_session`, set by the login response (browsers); the admin console uses its own `metareel_admin_session` |

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
  `{token, expires_at, session_id, probe_key, remember_device, user}` and sets the `metareel_session` cookie (HttpOnly,
  SameSite=Strict, `Secure` when the request came over HTTPS, including via a
  proxy that sets `X-Forwarded-Proto: https`). `POST /api/v1/auth/logout` ends
  the session; `GET /api/v1/auth/me` says who a request is authenticated as.
  The admin console signs in separately (`?client=admin` on login, logout and
  `/auth/session`) with its own `metareel_admin_session` cookie, the only
  cookie the admin API reads: being signed in to the app at `/` doesn't sign
  you in to `/admin`, and signing out of one keeps the other.
- Sessions last `AUTH_SESSION_TTL` (default 30 days) and end after
  `AUTH_SESSION_IDLE_TIMEOUT` (default 7 days) without use. Only a digest of
  each token is stored; the maintenance job deletes expired sessions.
- Native clients (the planned mobile app, `docs/mobile-app-plan.md`) can also
  send `device_name` (shown in session lists) and `remember_device: true` for
  a session that lasts `AUTH_DEVICE_SESSION_TTL` (default 180 days) and ends
  after `AUTH_DEVICE_SESSION_IDLE_TIMEOUT` (default 90 days) unused. They are
  revoked like any other session.
- `POST /api/v1/auth/probe` `{"session_id","nonce"}` lets the app check that
  a server answering on a plain-HTTP home address is this one before it sends
  its token there: the answer is `hex(HMAC-SHA256(probe_key, nonce))` with the
  `probe_key` from sign-in. Unknown or ended sessions get a proof that never
  matches. Rate limited per client IP.
- Behind a reverse proxy, set `SERVER_TRUSTED_PROXIES` (IPs or CIDR ranges)
  so rate limits see the real client from `X-Forwarded-For`; otherwise every
  client looks like the proxy and shares one sign-in limit.
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

`GET /api/v1/me/shows/{id}/episodes/{season}/{episode}` is one episode for a
details screen: TMDB's synopsis, still, score, directors, writers and up to 12
guest stars, fetched live (not stored), plus your watches of that episode,
newest first. When TMDB can't be reached it answers with the cached name, air
date and runtime and `live: false`; an episode TMDB doesn't list is a 404.

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
order: `new_episodes` (a series or season that premiered within
`stale_days`, with nothing watched since), `watching` (part way through and
watched within `stale_days`, 30 by default), `stalled` (part way through, nothing watched for longer),
`not_started`, `on_hold`, `dropped` and `caught_up` (every aired episode
seen). A status you chose -- on hold, dropped -- decides the stage;
otherwise the episodes do, so a completed show whose new episode has aired
is back to `watching`. A new series or season left unwatched for `stale_days`
after its premiere moves on to `not_started` or `stalled`. `GET /api/v1/me/upcoming` lists the episodes still to
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
