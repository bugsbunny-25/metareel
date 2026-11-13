# metareel user web app — data inventory

A brief for designing the user-facing web app (discovery + tracking) served at
`/`. It lists **every piece of data the UI can show**, where it comes from, and
what is missing. The admin console (`/admin`) is a separate app and is not
covered here.

Everything marked **now** exists and is served today. Everything marked
**Phase N** is planned and not yet built — design for it, but do not assume a
field exists.

---

## 1. What the app is

One app with two halves over one session:

- **Discovery** — streaming Top 10 charts across countries and services,
  movement, breakouts, ratings from three sources, availability, release
  calendars, credits, and analytics. All of this already exists in the public
  API; the app needs no new backend for it.
- **Tracking** — the signed-in user's watchlist, statuses, watch log
  (rewatches included), ratings and tags.

The interesting screens are the ones that combine both (see §7).

---

## 2. Reading this document

- Field names are exactly as the JSON uses them.
- `?` after a type means nullable or absent.
- "Chart points" always means the FlixPatrol scoring: rank 1 = 10 points,
  rank 2 = 9 … rank 10 = 1 point.

---

## 3. Global conventions

### Images
TMDB returns **paths**, not URLs. The UI must build the URL itself:

```
https://image.tmdb.org/t/p/{size}{path}
```

| Field | Appears on | Suggested size |
|---|---|---|
| `poster_path` | titles, chart rows (`Top10Result`, `ChartTitle`), diary entries, search results | `w342`, `w500` |
| `backdrop_path` | title metadata | `w1280`, `original` |
| `profile_path` | cast and crew | `w185` |
| `logo_path` | watch providers | `w92` |

`poster_url` on release items (`/titles/new`, `/titles/upcoming`) is the
exception — it is already a full URL, from JustWatch.

### The two `kind` vocabularies — important
- TMDB-keyed routes (`/titles/tmdb/{kind}/{id}`, tracking, discover, search):
  `movie | tv`
- FlixPatrol chart routes and `titles.kind`: `movie | tv_show`
- `rankings.category` (what chart a row came from): `movies | tv_shows`

A title's `kind` is the TMDB namespace of its `tmdb_id` and **can differ from
the chart it appears on** — a stand-up special in a TV chart is a TMDB movie.
The UI should label by `kind`, not by chart category.

### Identifiers
- `tmdb_id` is a **string** everywhere, even though it is numeric.
- A title is addressed by the pair `(kind, tmdb_id)`.
- `title_id` / `id` on chart routes is metareel's internal FlixPatrol title id —
  useful for admin, not for linking in this app. Prefer `(kind, tmdb_id)`.
- Several FlixPatrol slugs and Netflix Top 10 names can map to one TMDB title;
  `flixpatrol_slugs` / `netflix_top10_titles` expose that.

### Dates and times
- Dates: `YYYY-MM-DD`.
- Discovery timestamps: RFC 3339 / `date-time`.
- Tracking timestamps (`ended_at`, `created_at`, `first_watched_at`, …):
  `YYYY-MM-DD HH:MM:SS` in **UTC**.
- Writing a watch: the client sends RFC 3339 instants (`2026-11-12T23:00:00Z`).
- `watched_on` is the **local** day a watch belongs to, computed server-side and
  frozen. Group the diary by this, never by `ended_at`.

### Rating scale
Tracking ratings are **1–10 half-stars** (Letterboxd style): 1 = ½ star,
10 = 5 stars. There is no zero. Divide by 2 to display.
External ratings use their own scales: IMDb 0–10, Rotten Tomatoes 0–100, TMDB
`vote_average` 0–10.

### Auth
- `POST /api/v1/auth/login` → `{ token, expires_at, user: { id, username, role } }`
  and sets an `HttpOnly; SameSite=Strict` cookie. Same-origin app → use the
  cookie; ignore the token.
- `GET /api/v1/auth/me` → `{ kind: "user", id, name, role }`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/session` → whether sign-in is required, and the current user.
- Tracking routes (`/api/v1/me/*`) require a **user session**. An API key is
  refused with 401. Discovery routes accept either.

### Caching
Tracking responses carry `Cache-Control: private, no-store` and
`Vary: Authorization, Cookie`, and have no ETag. Discovery responses are
ETagged and safe to cache.

### Errors
Every error is `{ "error": "message" }`.
400 validation, 401 unauthenticated, 404 not found, 409 conflict,
429 rate limited (with `Retry-After`), 502 upstream failure, 500 otherwise.

### Reference vocabularies (populate filters from these)
- `GET /api/v1/countries` → `[{ code, slug, name }]`
- `GET /api/v1/providers` → `[{ slug, countries[] }]`
- `GET /api/v1/services/{country}` → `{ country, services: [{ requested, justwatch_package, name }] }`
- `GET /api/v1/charts` → which country × provider × category charts exist, their
  date coverage and staleness.

---

## 4. Tracking data (per user)

### 4.1 Available now — Phase 1

All under `/api/v1/me`.

#### Title state — `user_titles`
`GET /me/titles`, `GET /me/titles/{kind}/{id}`, `PUT /me/titles/{kind}/{id}`,
`DELETE /me/titles/{kind}/{id}`

| Field | Type | Notes |
|---|---|---|
| `tmdb_kind` | `movie \| tv` | |
| `tmdb_id` | string | |
| `status` | enum | `""`, `watchlist`, `watching`, `completed`, `dropped`, `on_hold`, `not_interested` |
| `rating` | int? 1–10 | **derived**: the most recent watch's rating. Not settable here. |
| `favorite` | bool | |
| `watchlist_position` | int? | set only while `status = watchlist` |
| `watchlist_note` | string? | |
| `first_watched_at` | ts? | derived |
| `last_watched_at` | ts? | derived |
| `watch_count` | int | derived |
| `created_at`, `updated_at` | ts | |
| `title`, `release_date`, `poster_path`, `runtime` | display | list responses only |

List response: `{ titles: [...], total, limit, offset }`.
Filters: `status`, `kind`, `favorite`, `sort`, `limit` (max 200), `offset`.
`sort`: `position_asc`, `rating_desc`, `watched_desc`, `title_asc`.

Single response: `{ title: {...}, state: {...} | null }` — `state` is null when
the user has never touched the title. The `title` object is always present:

| Field | Type |
|---|---|
| `tmdb_kind`, `tmdb_id`, `title`, `release_date`, `poster_path` | |
| `runtime`, `number_of_seasons`, `number_of_episodes` | int (0 = unknown) |
| `charted` | bool — metareel has chart data for it, so the discovery screens have more to show |

#### Watchlist
`GET /me/watchlist` (same shape as `/me/titles` with `status=watchlist`,
default sort `position_asc`), `PUT /me/watchlist/{kind}/{id}`,
`DELETE /me/watchlist/{kind}/{id}`.

#### Watch log — `watch_entries`
`GET /me/watches`, `POST /me/watches`, `GET|PUT|DELETE /me/watches/{id}`

One row **per watch**: a rewatch is another row with its own rating and tags.

| Field | Type | Notes |
|---|---|---|
| `id` | int | |
| `client_key` | string? | idempotency key; replaying returns the first watch with 200 instead of 201 |
| `subject_kind` | `movie` | `episode` from Phase 2 |
| `tmdb_kind`, `tmdb_id` | | |
| `season_number`, `episode_number`, `tmdb_episode_id` | ? | always null in Phase 1 |
| `ended_at` | ts | UTC; **what the user logs** |
| `started_at` | ts? | only when explicitly given |
| `runtime_minutes_used` | int? | runtime behind a derived start |
| `runtime_source` | `tmdb_title \| tmdb_episode \| user \| unknown` | provenance |
| `tz` | string? | IANA name, e.g. `America/New_York` |
| `utc_offset_minutes` | int? | derived from `tz` at that instant |
| `watched_on` | date | local day; **group the diary by this** |
| `time_precision` | `minute \| day \| month \| year` | imprecise times are stored at the earliest instant they could mean |
| `rating` | int? 1–10 | half-stars |
| `liked` | bool | separate from the rating |
| `is_rewatch` | bool | derived |
| `watch_number` | int | 1 = first time |
| `notes` | string? | private; there are no reviews |
| `venue_kind` | `theater \| streaming \| physical \| broadcast \| other`? | detail fields come in Phase 3 |
| `source` | `manual \| api \| import:*` | |
| `tags` | string[] | per watch, not per title |
| `created_at`, `updated_at` | ts | `created_at` ≠ when it was watched |
| `title`, `release_date`, `poster_path` | display | diary responses only |

Diary response: `{ watches: [...], limit, next_cursor? }` — newest local day
first, cursor-paginated (`cursor` = `"YYYY-MM-DD:id"`), max limit 200.
Filters: `tmdb_kind`, `tmdb_id`, `subject_kind`, `from`, `to`, `rated`.

Deletes are **permanent** — there is no trash, so the UI needs a confirm step.

#### Tags
`GET /me/tags` → `{ tags: [{ id, name, watch_count, created_at }] }`
`DELETE /me/tags/{id}`
Tags are created implicitly by naming them on a watch. Max 20 per watch,
50 characters each, de-duplicated case-insensitively.

#### TMDB search (for "log anything")
`GET /me/titles/search?q=&kind=&limit=` →
`{ results: [{ tmdb_kind, tmdb_id, title, original_title, year, release_date, poster_path }] }`

This searches **all of TMDB**, not just titles metareel knows — which is how a
user logs an obscure film. `GET /api/v1/search` is the other search: it covers
only titles with chart data, but returns `last_charted` and a match `score`.
The UI probably wants both, labelled differently.

### 4.2 Planned tracking data

**Phase 2 — TV**
- Per-episode watches (`subject_kind: episode`, `season_number`,
  `episode_number`, `tmdb_episode_id`)
- `user_episode_progress`: watched/unwatched per episode, watch counts
- Progress: episodes watched / total, with an explicit denominator and an
  `include_specials` flag
- Bulk actions: mark episode, mark season, **mark everything up to here**
- `GET /me/up-next`: the next unwatched episode per in-progress show

**Phase 3 — venues, formats, money, presentation**
Added to each watch: `venue_id`, `screen_id`, `venue_name_at_watch`,
`venue_city_at_watch`, `seat`, `screening_type`
(`regular|preview|premiere|festival|press|rerelease|marathon`), `event_name`,
`with_qa`, `price_minor` + `currency` + `fees_minor` + `concessions_minor` +
`tickets_count`, `provider_id` + `monetization`, `media`
(`bluray_4k|bluray|dvd|vhs|digital_file|35mm|70mm`), `edition`, `device`,
`dimension` (`2d|3d|3d_hfr`), `resolution`, `hdr`, `audio_format`,
`frame_rate`, `audio_language`, `subtitle_language`,
`content_aspect_ratio_text` + `content_aspect_ratio` + `content_aspect_variable`,
`screen_aspect_ratio`, `screen_width`/`screen_height` + `screen_dim_unit`.

Plus **formats** as a many-to-many admin-managed taxonomy:
`{ slug, label, category: brand|projection|sound|motion|dimension, active }` —
IMAX, IMAX 3D, IMAX 70mm, IMAX Laser, Dolby Cinema, 4DX, ScreenX, Cinemark XD,
RealD 3D, D-BOX, PLF, HFR, Atmos, 35mm, 70mm.

Plus **venues**: `{ name, chain, address, city, state, country, lat, lng,
timezone, opened_on, closed_on, status, replaced_by_venue_id }`, with
`venue_aliases` for renames and `venue_screens`
`{ name, number, width, height, unit, aspect_ratio, seats, effective_from, effective_to }`.

**Phase 4 — stats** (endpoints to be designed; the raw material is all there)
Hours watched, films vs episodes, by month/weekday/hour, streaks, rating
distribution, your rating vs IMDb/RT/TMDB, top director/actor/genre/country/
decade (from `/credits` and metadata), spend by venue/format/currency,
theatre-vs-home ratio, premium-format hours, completion stats, milestones.

**Not planned for v1**: reviews, companions, social/follows/feeds, public
profiles, notifications, scrobbling, data export, imports (moved after v1),
ticket-stub attachments, TMDB episode groups.

### 4.3 Planned endpoints

**None of these exist yet. The paths are provisional** — they are here so UI
flows can be designed against a concrete shape, not as a contract. The field
names inside them come from §4.2 and are more settled than the routes.

#### Phase 2 — TV

| Endpoint | Purpose / shape |
|---|---|
| `GET /me/shows/{id}` | A show's tracking state with per-season and per-episode progress: `{ title, state, progress: { watched, total, percent, include_specials }, seasons: [{ season_number, name, air_date, episode_count, watched_count, episodes: [{ episode_number, name, air_date, runtime, episode_type, watched_count, last_watched_at? }] }] }` |
| `POST /me/watches` | Extended, not new: `subject_kind: "episode"` plus `season_number`, `episode_number` |
| `POST /me/shows/{id}/episodes/watched` | Bulk mark, one transaction. Body selects a scope — `{ season_number }`, `{ up_to: { season_number, episode_number } }` ("everything up to here"), or `{ episodes: [{ season_number, episode_number }] }` — plus `ended_at`, `tz`, optional `rating`, `tags`, `include_specials`. → `{ created, skipped, entries[] }` |
| `DELETE /me/shows/{id}/episodes/watched` | Bulk unmark, same scope selectors |
| `GET /me/up-next?limit=` | `{ items: [{ title, state, progress, next_episode: { season_number, episode_number, name, air_date, runtime } }] }` — the next unwatched episode per in-progress show |
| `GET /me/calendar?from=&to=&country=` | The personalised episode calendar: `/api/v1/calendar/{country}` filtered to tracked shows, with each item's watched state |

#### Phase 3 — venues, formats, money

| Endpoint | Purpose / shape |
|---|---|
| `GET /api/v1/watch-formats` | Public read of the admin-managed taxonomy, so the log form can render options: `{ formats: [{ id, slug, label, category, sort_order, active }] }` |
| `GET\|POST\|PATCH /api/v1/admin/watch-formats` | Admin CRUD (admin console, not this app) |
| `GET /me/venues?q=&country=` | The user's known venues, searchable |
| `POST /me/venues` | Create from a picked candidate or by hand |
| `GET\|PUT\|DELETE /me/venues/{id}` | One venue, with its aliases and screens |
| `GET /me/venues/search?q=&country=` | Candidate lookup against the seeded Overture/Foursquare data (and optionally Google Places autocomplete), returning rows to create a venue from: `{ candidates: [{ source, ref, name, address, city, country, lat, lng }] }` |
| `GET /me/venues/{id}/screens`, `POST /me/venues/{id}/screens` | Screens of a venue |
| `PUT\|DELETE /me/screens/{id}` | One screen |
| `POST /me/watches`, `PUT /me/watches/{id}` | Extended with every Phase 3 field from §4.2, plus `formats: [slug]` |

#### Phase 4 — stats

| Endpoint | Purpose / shape |
|---|---|
| `GET /me/stats?from=&to=` | Headline numbers: watches, titles, rewatches, minutes, films vs episodes, distinct days, longest streak |
| `GET /me/stats/year/{year}` | Year in review: the above plus by month, by weekday, by hour, first/last watch, milestones |
| `GET /me/stats/calendar?from=&to=` | Heatmap series: `[{ day, watches, minutes }]` |
| `GET /me/stats/ratings` | Your rating distribution, and your rating vs IMDb / RT / TMDB per title |
| `GET /me/stats/people?role=` | Most-watched actors, directors, writers (joined from `/credits`) |
| `GET /me/stats/genres`, `/countries`, `/decades`, `/languages` | Taste breakdowns from TMDB metadata |
| `GET /me/stats/formats` | Premium-format hours, theatre vs home, 2D vs 3D (needs Phase 3) |
| `GET /me/stats/spend?currency=` | Ticket spend by venue, format and month, per currency, no FX conversion (needs Phase 3) |

A stats screen built before these exist has to aggregate `/me/watches` on the
client. That works for a few thousand entries and stops working beyond that, so
treat any client-side stats as a stopgap.

#### Known gap in Phase 1

`watchlist_position` is stored and `PUT /me/watchlist/{kind}/{id}` puts a title
at the end, but **there is no reorder endpoint yet** — something like
`PUT /me/watchlist/order` with `{ items: [{ tmdb_kind, tmdb_id }] }`. Drag-to-
reorder cannot be wired up until that exists; design the watchlist so ordering
is a later addition rather than a core interaction.

---

## 5. Discovery data (all available now)

### 5.1 Top 10 and charts

`GET /top10/movies/{country}`, `/top10/tv-shows/{country}` (all providers),
`/top10/movies/{country}/{provider}`, `/top10/tv-shows/{country}/{provider}`,
`/top10/{country}` (combined), `/top10/global/{provider}`

**`Top10Result`** — the core chart row:

| Field | Type | Notes |
|---|---|---|
| `date` | date | chart date of this entry |
| `rank` | int | 1–10 |
| `previous_rank` | int? | on this chart's previous **scraped** date; null if absent then |
| `days_in_top10` | int | dates on this chart up to and including `date` |
| `provider` | string | |
| `title_id`, `slug`, `name` | | FlixPatrol identity |
| `kind` | `movie \| tv_show` | |
| `tmdb_id`, `imdb_id`, `rotten_tomatoes_url` | ? | |
| `poster_path` | string? | TMDB path from stored metadata; omitted until the title has it |
| `season_number` | int? | TV, when known |
| `season_source` | `chart \| netflix \| tmdb_air_date` | how the season was determined |
| `origin` | `{ origin: original\|in_house\|licensed\|unknown, owner, reason }` | who made it; `owner` is the media group, `reason` the deciding brand |

`Top10Response` wraps them: `{ date, country, category, provider, items[] }`.
`Top10Combined` returns `{ country, movies[], tv_shows[], dates: {provider: date} }`.

**Chart history** `/top10/{movies|tv-shows}/{country}/{provider}/history` →
`{ country, provider, category, from, to, days: [{ date, items: [ChartTitle + rank, season_number, season_source] }] }`

**Chart catalog** `/charts` → per country × provider × category:
`{ first_date, latest_date, dates, last_scraped_at, stale }` plus
`current_chart_date`. Use this to drive country/provider pickers and to warn
when a chart is stale.

**Available dates** `/charts/{country}/{provider}/dates`.

**Chart changes feed** `/changes?since=` →
`{ since, next, items: [{ id, date, country, provider, category, entries, scraped_at, changed_at }] }`

### 5.2 Movement

**Movers** `/movers` → `{ country, provider, charts: [{ category, date, previous_date, climbers[], fallers[], debuts[], re_entries[], exits[] }] }`
Each `Mover` = `ChartTitle` + `{ category, rank?, previous_rank?, change }`
(`rank` null for exits; `previous_rank` null for debuts and re-entries).

**Breakouts** `/breakouts` — titles rising across many countries at once:
`{ date, window, min_countries, min_climb, category?, provider?, kind?,
countries_checked[], total, items: [Breakout] }`
`Breakout` = `ChartTitle` + `{ rising_countries, charting_countries[], score,
countries: [{ country, provider, category, date, base_date, rank, base_rank?,
movement: entered|climbed, gain }] }`

**`ChartTitle`** (shared base): `{ title_id, slug, name, kind, tmdb_id?, imdb_id?, poster_path? }`.
Every chart-derived list (history, movers, breakouts, leaderboards, `/top10/global`, analytics items) carries `poster_path`, so a grid of chart titles needs no per-title request for posters.

### 5.3 Leaderboards

`/leaderboards?period=&country=&provider=&category=&kind=` →
`{ period, from, to, country?, provider?, category?, kind?, items: [ChartTitle + { position, points, days_on_charts, appearances, best_rank, countries[], providers[], slugs[] }] }`

### 5.4 Title detail

`GET /titles/tmdb/{kind}/{id}` returns **`TitleOverview`**, the one-shot payload
for a title page. It bundles everything below, plus `errors: {section: message}`
for sections that failed to load — so the UI must render partial data.

```
TitleOverview = {
  kind, tmdb_id,
  flixpatrol_titles[],   // RankedTitle: { slug, name, imdb_id?, rotten_tomatoes_url? }
  metadata,              // TmdbMetadata
  stats,                 // TitleStats
  current_positions[],   // ChartMoment: { date, country, provider, category, rank }
  ratings,               // RatingsResponse
  availability,          // AvailabilityResponse
  netflix: { weeks_in_global_top10, best_global_rank?, total_views,
             total_hours_viewed, countries[], seasons[] },
  ids,                   // TitleExternalIDs
  errors
}
```

**`TmdbMetadata`** — `tmdb_kind`, `tmdb_id`, `title`, `original_title`,
`original_language`, `overview`, `status`, `release_date`, `last_air_date`,
`runtime`, `number_of_seasons`, `number_of_episodes`, `genres[]`,
`origin_countries[]`, `production_companies[]`, `networks[]`, `poster_path`,
`backdrop_path`, `popularity`, `vote_average`, `vote_count`,
`digital_release_dates {country: date}`, `imdb_id`, `wikidata_id`,
`rotten_tomatoes_id`, `metacritic_id`, `letterboxd_id`, `netflix_id`,
`justwatch_id`, `details_fetched_at`, and for TV
`seasons: [{ season_number, name, air_date, episode_count }]` (season 0 = specials).

**`TitleStats`** (`/titles/tmdb/{kind}/{id}/stats`) — `titles[]`, `total_points`,
`days_on_charts`, `appearances`, `peak_rank?`, `days_at_number_1`,
`debut` / `last_seen` (`ChartMoment`), `longest_streak_days`, `countries[]`,
`providers[]`, `spread: [{ country, first_date, days_after_debut }]`,
`charts: [ChartSummary]`.

**`ChartSummary`** — `{ country, provider, category, days_in_top10, best_rank, first_date, last_date, last_rank }`

**Rankings** `/titles/tmdb/{kind}/{id}/rankings` → `{ kind, tmdb_id, titles[], charts[], rankings: [{ date, country, provider, category, rank, slug, season_number?, season_source }] }`
Batch variant: `/titles/tmdb/rankings?ids=` → `{ items[], not_found[] }`

**Ratings** `/titles/tmdb/{kind}/{id}/ratings` → **`RatingsResponse`**:

| Field | Notes |
|---|---|
| `kind`, `tmdb_id`, `imdb_id?`, `title?`, `year?` | |
| `imdb` | `{ rating 0–10, votes?, url?, source: imdb_dataset\|justwatch\|mdblist }?` |
| `rotten_tomatoes` | `{ url?, critics_score 0–100?, critics_rating: Certified Fresh\|Fresh\|Rotten?, certified_fresh?, critics_source?, audience_score?, audience_rating: Upright\|Spilled?, audience_source? }?` |
| `sources[]` | every raw value: `{ provider: justwatch\|mdblist\|rottentomatoes\|imdb_dataset, source, value, votes?, url?, fetched_at }` — `source` includes `imdb`, `tomatoes`, `popcorn`, `tmdb`, `metacritic`, `letterboxd`, `trakt`, `mdblist_average` |
| `refreshed_at`, `cached`, `stale` | `stale` = a refresh was due but failed; show it |

Batch variant: `/titles/tmdb/ratings?ids=`

**Availability** `/titles/tmdb/{kind}/{id}/availability[?country=]` →
`{ kind, tmdb_id, country?, fetched_at, attribution, countries: { CC: { link?, flatrate[], free[], ads[], rent[], buy[] } } }`
Each **`WatchProvider`** = `{ country, monetization, provider_id, provider_name,
logo_path?, display_priority?, url?, link?, fetched_at }`.
`url` is the deep link to the title on that service (may carry JustWatch's
affiliate tag); `attribution` must be displayed (JustWatch data).

**Availability history** `/titles/tmdb/{kind}/{id}/availability/history` →
`OfferEvent[]`: `{ id, tmdb_kind, tmdb_id, country, monetization, provider_id,
provider_name, event: added|removed, previous_check_at, observed_at }`

**Value history** `/titles/tmdb/{kind}/{id}/history` →
`{ kind, tmdb_id, from, to,
   imdb: { imdb_id, points: [{date, value, votes?}] }?,
   ratings: [{ provider, source, points[] }],
   popularity: [{ date, popularity }],
   wikipedia: { article, url, points: [{ date, views }] }? }`
Four time series for sparklines and charts.

**Timeline** `/titles/tmdb/{kind}/{id}/timeline` — the single richest thing for a
title page: one merged, dated event stream.
`{ kind, tmdb_id, title?, country?, from?, to?, total, counts: {type: n}, events: [TimelineEvent] }`

**`TimelineEvent`** — `{ type, date, end_date?, summary }` plus one typed detail
object. `type` is one of `chart_debut`, `chart_peak`, `chart_number_one`,
`chart_reentry`, `availability_added`, `availability_removed`, `rating_changed`,
`pageview_spike`, `netflix_top10_week`, `netflix_engagement`, `season_premiere`,
`release`, `netflix_premiere`. Detail objects:

- `chart`: `{ country, provider, category, rank, season_number, days }`
- `availability`: `{ provider_id, provider_name, monetization, countries[] }`
- `rating`: `{ provider, source, value, previous_value, votes }`
- `pageviews`: `{ peak_day, peak_views, baseline_views, ratio, days }`
- `netflix`: `{ week, category, global_rank?, season_title, season_number, hours_viewed, views, cumulative_weeks, countries, country_rank }`
- `engagement`: `{ period_start, period_end, title, hours_viewed?, views?, runtime_minutes?, season_number, available_globally? }`
- `season`: `{ season_number, name, episode_count }`

`summary` is a ready-made human sentence (e.g. "Debuted at #3 on netflix TV
shows (US)"), so a timeline can be rendered without per-type copy.

**Releases (per movie)** `/titles/tmdb/movie/{id}/releases` →
`{ kind, tmdb_id, title?, release_date?, countries: [CountryReleases] }`
**`CountryReleases`** = `{ country, premiere?, theatrical_limited?, theatrical?,
digital?, physical?, tv?, notes {kind: note},
streaming: { date, basis: offer_added|first_chart, provider, provider_id, monetization }?,
streaming_now: [{ provider_id, provider_name, monetization }],
windows: { theatrical_basis: wide|limited?, theatrical_to_digital?,
theatrical_to_streaming?, digital_to_streaming? } }`

**Credits** `/titles/tmdb/{kind}/{id}/credits` →
`{ kind, tmdb_id, title?,
   cast: [{ person_id, name, profile_path?, character, order, episodes? }],
   crew: [{ person_id, name, profile_path?, job, department, episodes? }] }`
Crew jobs stored: Director, Screenplay, Writer, Story, Novel, Teleplay,
Characters, Creator, Original Music Composer, Director of Photography.

**Netflix (per title)** `/titles/tmdb/{kind}/{id}/netflix` →
`{ kind, tmdb_id, weeks_in_global_top10, best_global_rank?, total_views,
total_hours_viewed, countries[], seasons: [NetflixSeasonSummary],
global: [NetflixWeekRow], country_weeks: [NetflixWeekRow], engagement[] }`
`engagement` is the title's rows in Netflix's "What We Watched" reports (titles
watched 50k+ hours in a six-month period).

**`NetflixSeasonSummary`** = `{ season_number?, season_source, season_title,
weeks_in_global_top10, best_global_rank?, total_views, total_hours_viewed,
first_week, last_week, countries }`

### 5.5 People

`GET /people/{id}` → `{ person_id, name, profile_path?, known_for?, points,
charting, titles: [{ kind, tmdb_id, title, year?, poster_path?, roles[], points,
days, best_rank?, countries, first_charted?, last_charted? }] }`

### 5.6 Releases, calendar and availability changes

**New / upcoming on a service** `/titles/new/{country}/{service}`,
`/titles/upcoming/{country}/{service}` → `{ type, country, service, from?, to?,
count, truncated, items: [ReleaseItem] }`

**`ReleaseItem`** — `{ date?, kind, title, season_number?, season_title?, year?,
original_release_date?, tmdb_id?, imdb_id?, justwatch_id, justwatch_url?,
description?, runtime_minutes?, genres[], poster_url?, imdb_rating?, imdb_votes?,
tomatometer?, certified_fresh?, offer: { url?, monetization_type, presentation_type }?,
release_type? }`
These are fetched **live** from JustWatch per request, not stored — expect
latency and handle `truncated`.

**Episode calendar** `/calendar/{country}` → `{ country, from, to, provider_id?,
items: [{ air_date, tmdb_id, title, poster_path?, season_number,
season_premiere, season_finale,
episodes: [{ episode_number, name, episode_type: standard|finale|mid_season, runtime }],
offers: [{ provider_id, provider_name, monetization, url }] }] }`
Only covers shows metareel knows (charting titles), and TMDB air dates are the
original release, not per country.

**Availability changes feed** `/availability/changes` → `OfferEvent[]` with
`title`, `release_date`, `poster_path` added. This is the "arriving / leaving"
feed and the basis for "leaving soon, in your watchlist".

### 5.7 Discovery

**Hidden gems** `/discover/{country}/hidden-gems` → highly rated, streamable
in the country, not charting much here:
`{ country, from, to, kind?, provider_id?, min_rating, min_votes, considered,
total, items: [DiscoverTitle + { genres[], score, streaming_here: [DiscoverOffer],
points_elsewhere, charted_in[] }] }`

**Elsewhere** `/discover/{country}/elsewhere` → popular abroad, not available
here: `{ ..., countries_checked[], skipped: { available_here, charting_here,
availability_unknown }, total, items: [DiscoverTitle + { points,
countries: [{ country, points, best_rank, providers[] }], rent_buy_here: [DiscoverOffer] }] }`

**`DiscoverTitle`** = `{ kind, tmdb_id, title, year?, poster_path?, imdb_rating?, imdb_votes? }`
**`DiscoverOffer`** = `{ provider_id, provider_name, monetization }`

**Service rotation** `/discover/{country}/rotation` → which subscriptions are
worth keeping: `{ country, past_from, past_to, upcoming_to, chart_basis, slots,
subscribed[], services: [{ provider_id, provider_name, provider_ids[], titles,
chart_points, charting, arrivals, departures, upcoming_shows, top_rated, score,
recommendation: subscribe|skip|keep|pause|add,
highlights: [{ kind, tmdb_id, title, detail }] }] }`

### 5.8 Netflix official data

`/netflix/top10`, `/netflix/top10/{country}` → `{ week, country?, category?, items: [NetflixWeekRow] }`

**`NetflixWeekRow`** — `{ week, country, category, rank, show_title,
season_title?, season_number?, season_source, hours_viewed?, runtime_hours?,
views?, cumulative_weeks?, kind, tmdb_id?, imdb_id?, netflix_title_id }`

`/netflix/most-popular` → Netflix's all-time most popular lists.

### 5.9 Analytics

All take `from`, `to`, and most take `country`, `provider`, `category`, `kind`;
all echo the resolved filter back as `{ from, to, country?, provider?, category?, kind? }`.

| Endpoint | Returns |
|---|---|
| `/analytics/decay` | `debuts`, `curve: [{ day, titles, charting, retention, avg_rank?, avg_points }]` — 30 days from debut |
| `/analytics/country-similarity` | `countries {CC: n}`, `pairs: [{ a, b, jaccard, shared }]` |
| `/analytics/release-lag` | `summary`, `by_provider {provider: LagSummary}`, `items_total`, `items: [ChartTitle + { country, provider, release_date, release_basis, first_chart_date, lag_days, peak_rank }]` |
| `/analytics/ratings-vs-popularity` | `titles`, `with_imdb`, `correlation_points_imdb`, `correlation_points_rt_critics`, `acclaimed_underwatched[]`, `popular_poorly_rated[]`, `items: [ChartTitle + { points, best_rank, imdb_rating?, imdb_votes?, rt_critics?, rt_audience?, imdb_source }]` |
| `/analytics/genres` | `points_with_genres`, `points_without_genres`, `items: [{ genre, points, titles, share }]` |
| `/analytics/origins` | per provider: `points`, `origins {original\|in_house\|licensed\|unknown: {points, share, titles}}`, `series: [{ period, points, shares }]`, `suppliers: [{ owner, name, points, share, titles }]`, `top {origin: [{ name, kind, tmdb_id, points, owner, reason }]}` |
| `/analytics/release-patterns` | `patterns: [{ pattern: binge\|batch\|weekly\|daily\|irregular, seasons, chart_runs, median/mean_days_in_top10, median_last_day, share_still_charting_day_28, median_best_rank, median_points }]`, `seasons[]`, `excluded { unaired, single, in_progress, uncharted }` |
| `/analytics/spread-lag` | `titles`, `multi_country`, `simultaneous`, `summary`, `countries: [{ country, origin, arrived, arrival_lag, not_reached }]`, `pairs: [{ from, to, reached, not_reached, lag }]`, `items[]` with per-country `arrivals` |
| `/analytics/release-windows` | `movies`, `theatrical_to_digital`, `theatrical_to_streaming`, `digital_to_streaming` (each `LagSummary` + `within_days {17,31,45,90}` + `missing`), `streaming_by_provider[]`, `items: [CountryReleases + { tmdb_id, title, poster_path? }]` |
| `/analytics/people` | `role`, `total`, `people: [{ person_id, name, profile_path?, points, titles, best_rank, top_titles: [{ kind, tmdb_id, title, points }] }]` |
| `/analytics/netflix-calibration` | `rows: [{ week, show_title, kind, tmdb_id, netflix_global_rank, netflix_views?, netflix_hours_viewed?, flixpatrol_points, flixpatrol_days, views_per_point? }]`, `correlation_points_views`, `median_views_per_point`, `weeks_with_flixpatrol_charts` |

**`LagSummary`** = `{ titles, median_days?, p25_days?, p75_days? }`

### 5.10 Search and lookup

- `GET /api/v1/search?q=&kind=&limit=` → titles **with chart data**:
  `{ query, items: [{ kind, tmdb_id, title, original_title, year, poster_path,
  matched, sources: [flixpatrol|netflix], last_charted, score }] }`
  Matches any name a title goes by, accent/punctuation-insensitive, word
  prefixes allowed.
- `GET /me/titles/search` → **all of TMDB** (see §4.1).
- `GET /api/v1/titles/lookup?imdb=|tmdb=|slug=|netflix=` → resolve an external
  ID to `(kind, tmdb_id)`.

### 5.11 Export

`GET /api/v1/export/rankings?from=&to=&country=&provider=&category=&format=csv|ndjson`
— chart entries over a range (default 30 days, max 366). Columns: `date`,
`country`, `provider`, `category`, `rank`, `season_number`, `title_id`, `slug`,
`name`, `kind`, `tmdb_id`, `imdb_id`, `season_source`.

---

## 6. Screen inventory

A starting map of screens to the data above. The UI agent can restructure
this; what matters is that every field has a home.

| Screen | Data |
|---|---|
| **Home / today** | `/top10/{country}` combined, `/movers`, `/breakouts`, `/availability/changes`, plus the user's `/me/watchlist` and `/me/watches` (recent) |
| **Charts browser** | `/charts` (catalog + staleness), `/top10/*`, `/top10/*/history`, country & provider pickers from `/countries`, `/providers` |
| **Movement** | `/movers` (climbers, fallers, debuts, re-entries, exits), `/breakouts` with its per-country table |
| **Leaderboards** | `/leaderboards` with period/country/provider/category/kind filters |
| **Title page** | `/titles/tmdb/{kind}/{id}` (overview bundle) + `/timeline` + `/history` (4 series) + `/credits` + `/releases` + `/availability` + `/rankings`; and the user's `/me/titles/{kind}/{id}` state with a log-a-watch action |
| **Person page** | `/people/{id}` |
| **Search** | `/api/v1/search` for charted titles, `/me/titles/search` for all of TMDB, `/titles/lookup` for IDs |
| **Discover** | `/discover/{country}/hidden-gems`, `/elsewhere`, `/rotation` |
| **Calendar** | `/calendar/{country}` (episodes + offers); Phase 2 filters it to tracked shows |
| **New & upcoming** | `/titles/new/{country}/{service}`, `/titles/upcoming/{country}/{service}` (live, slow, `truncated`) |
| **Netflix** | `/netflix/top10`, `/netflix/most-popular`, `/analytics/netflix-calibration` |
| **Analytics / insights** | the 11 analytics endpoints in §5.9 |
| **My watchlist** | `/me/watchlist` (ordered), with availability joined per item |
| **My library** | `/me/titles` filtered by status, favourites, sorted |
| **My diary** | `/me/watches` grouped by `watched_on`, with tags and ratings |
| **Log a watch** | `/me/titles/search` → `POST /me/watches`; Phase 3 adds the venue/format/price form |
| **My stats** | Phase 4 |
| **Up next** | Phase 2 |
| **Settings** | `/auth/me`, logout, default country, units, currency |

---

## 7. Cross-cutting combinations

These are the features only this app can have, because it holds both halves.
All are **client-side joins** in Phase 1 — fetch both sides and intersect. Do
not expect a server endpoint.

| Feature | Join |
|---|---|
| Watchlist items now charting | `/me/watchlist` × `/top10/{country}` or `/titles/tmdb/rankings` (batch) |
| Watchlist items leaving a service soon | `/me/watchlist` × `/availability/changes` (`event: removed`) |
| Watchlist items newly streaming | `/me/watchlist` × `/availability/changes` (`event: added`) |
| Where to watch what I want | `/me/watchlist` × `/titles/tmdb/{kind}/{id}/availability` per item |
| Hidden gems I haven't seen | `/discover/{country}/hidden-gems` minus titles with `watch_count > 0` |
| Breakouts I haven't seen | `/breakouts` minus watched |
| My rating vs the crowd | `/me/titles` `rating` (÷2 → 5-star) × `/titles/tmdb/ratings` batch (IMDb, RT, TMDB) |
| I watched it before it charted | watch `watched_on` × `stats.debut.date` |
| My taste profile | watched titles × `metadata.genres`, `origin_countries`, `/credits` |
| Is this subscription worth it | `/discover/{country}/rotation` × my watch history per provider |

---

## 8. Gaps the UI must design around

1. **Partial data is normal.** `TitleOverview.errors` names sections that
   failed. Ratings carry `stale`. Charts carry `stale`. Many metadata fields are
   null for titles metareel has not enriched. Design empty and degraded states
   for every panel.
2. **Titles outside metareel's catalogue have almost no data.** Anything the
   user logs that never charted has only TMDB basics (title, year, poster,
   runtime, season counts) and `charted: false` — no stats, no ratings history,
   no timeline. A title page must work in both modes.
3. **No episode tracking yet** (Phase 2). `subject_kind` is always `movie`.
4. **No reviews, no companions, no social.** `notes` is private free text.
5. **No venue, format, price or presentation data yet** (Phase 3).
6. **No stats endpoints yet** (Phase 4) — any stats screen must either compute
   client-side from `/me/watches` or wait.
7. **No images of our own.** Everything is a TMDB path or a JustWatch URL;
   there is no upload or asset store.
8. **Episode and credit coverage is limited to titles metareel knows**, and
   episode data covers only recent, upcoming and latest seasons.
9. **`/titles/new` and `/titles/upcoming` are live upstream calls** — slow,
   may be `truncated`, and can fail with 502.
10. **Attribution is required** where `attribution` is returned (JustWatch).
11. **Rate limiting exists** (429 with `Retry-After`); batch endpoints
    (`/titles/tmdb/rankings`, `/titles/tmdb/ratings`) exist specifically so a
    grid of titles is one request, not fifty.
12. **Deletes are permanent** in tracking — no undo, so confirm.
13. **The watchlist cannot be reordered yet** — positions exist, but no reorder
    endpoint does (§4.3). Don't make drag-to-reorder central to the design.
14. **Planned endpoint paths in §4.3 are provisional.** The field names are
    fairly settled; the routes are not. Keep the data layer thin enough that
    renaming a route is not a redesign.
