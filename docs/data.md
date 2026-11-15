# Data

Where metareel's data comes from, the jobs that collect it, and how titles are matched across sources.

## Jobs

Every job is a schedule (admin UI → Schedules; run times in UTC) and can be run
on demand from Data health or `POST /api/v1/admin/jobs/:type/run`.

| Job | Default | What it does |
|---|---|---|
| `flixpatrol.top10.scrape` | your schedules | Scrapes each target's current chart, fills gaps from the last `backfill_days`, re-checks hourly for `recheck_hours` when FlixPatrol has not published the day's chart yet. A failed chart page does not stop the run (status `partial`); retries only redo failed pages. New titles are queued for matching right away. |
| `titles.enrich` | 03:00 | Matches titles to TMDB / IMDb (FlixPatrol title page → JustWatch → TMDB), retrying unmatched ones after 1, 3, 7, then every 30 days; finds RT slugs via Wikidata SPARQL, then RT search. Titles fixed in the admin UI (`manual`) are never re-matched, but their TMDB titles get missing RT slugs. |
| `titles.metadata` | 04:00 | TMDB details + watch providers with each service's link from JustWatch (charting and watchlisted titles every 3 days, others monthly), Wikidata IDs (RT, Metacritic, Letterboxd) of TMDB titles films' Wikidata distributors (for origins) and English Wikipedia articles (for pageviews), monthly. Needs `TMDB_API_KEY`. |
| `ratings.prewarm` | 15:30 | Refreshes expired ratings of titles charting in the last 3 days. |
| `netflix.top10.import` | 21:00 | Netflix's official weekly Top 10 (global, per country, most popular); skipped when unchanged. Matches up to 400 new titles (see [TMDB mapping](#tmdb-mapping)) and maps season titles to season numbers. |
| `netflix.titles.match` | 09:00 | Works through the backlog of unmatched Netflix titles (retrying after 1, 3, 7, then every 30 days; `manual` titles are never touched), looks up Netflix IDs of up to 200 more titles (fixing earlier name matches), then remaps seasons. Also queued by "Match again" in the admin UI. |
| `imdb.ratings.import` | 06:00 | IMDb's daily ratings dataset for the IMDb IDs metareel knows (also kept as history: a row whenever a title's rating or votes changed). |
| `tmdb.credits.refresh` | 05:30 | TMDB cast (top 25) and key crew of the titles metareel knows: never-fetched titles first, each again after 60 days, at most 1,000 a run (one request per title). Feeds `/titles/tmdb/:kind/:id/credits` (which asks TMDB live for titles it hasn't stored), `/people/:id` and `/analytics/people`. |
| `venues.import` | — (by hand) | Every cinema OpenStreetMap has in `VENUES_IMPORT_COUNTRIES` (default US, GB, IN; or `{"countries": ["IN"]}`), via the Overpass API, as venues to pick when logging a watch: name, chain (`brand`), address (or, when OSM has none on the cinema, the address of the building it is in or next to), coordinates, time zone from the coordinates, and the city (or the nearest city or town node within 20 km). Then the country's closed and demolished cinemas from Wikidata (largely Cinema Treasures; ~13,000 in the US, ~2,800 in GB), imported closed with their dates. Matches venues entered before by OSM ID, Wikidata ID or the same name nearby; re-runs only fill empty fields and never delete. Also on the console's Cinemas page. Data © OpenStreetMap contributors (ODbL). |
| `tmdb.episodes.refresh` | 05:00 | TMDB episodes (air dates, names, finales) of the TV shows metareel knows: seasons from the last three years, undated ones and the latest, one request per show (20 seasons each). Airing shows daily, returning shows weekly, ended shows monthly; at most 1,500 shows a run. Feeds `/calendar/:country` and `/analytics/release-patterns`. |
| `wikipedia.pageviews.import` | 10:00 | Daily English Wikipedia user pageviews of every title with an article (the metadata job finds it on Wikidata), since the last stored day (90 days for a new title), for `/history`. Paced for Wikimedia's 200 requests/minute limit (about 15 minutes for 2,500 titles). |
| `netflix.engagement.import` | 07:00 | Netflix's "What We Watched" reports (hours viewed, views, runtime and the premiere date of originals for every title watched 50k+ hours in six months) from the list in `app_settings` `netflix.engagement.urls` (seeded with H2 2025 and H1 2026; skipped when unchanged). Run it with `{"url": "https://…xlsx"}` to add a new report. Rows are linked to Netflix Top 10 titles by name and shown in `/titles/tmdb/:kind/:id/netflix` (`engagement`); their premiere dates mark Netflix originals for the chart `origin` and `/analytics/origins`. |
| `tmdb.popularity.import` | 09:30 | TMDB's daily ID exports (no API key): popularity of every TMDB title metareel knows, one row per title per day, for `/history`. Skipped when unchanged; yesterday's export is used until today's is published (~08:00 UTC). |
| `mapping.refresh` | — (queued) | Follows up every mapping change (matches, fixes, remaps, shared mappings): TMDB details and Wikidata IDs of the TMDB titles now mapped to, season numbers. Run by hand, it first recomputes every linked title's mapping. |
| `notifications.dispatch` | hourly at :15 | Mobile push notifications: a title on someone's watchlist arriving on one of their streaming services in their country (they opt in on the phone), once per title, through Expo's push service. Does nothing unless `PUSH_ENABLED=true` (optional `EXPO_ACCESS_TOKEN`); the first run only starts following provider changes. |
| `maintenance` | 02:00 | Run retention (`RUN_RETENTION_DAYS`), stale-chart alerts, deletes TMDB people left without credits, SQLite optimize. |

### TMDB mapping

One mapping keyed by TMDB: TMDB titles are the hub, and FlixPatrol titles and
Netflix official Top 10 titles both point at one (`kind` / `tmdb_kind` is the
TMDB namespace, so a stand-up special in Netflix's TV list can be a TMDB
movie). Each TMDB title keeps every ID — IMDb, Wikidata, Rotten Tomatoes,
Metacritic, Letterboxd, Netflix (Wikidata P1874 or Tudum's Top 10 search),
JustWatch — and the FlixPatrol slugs and Netflix Top 10 names mapped to it
(admin UI → Mapping → TMDB mappings; public `ids` on `/titles/tmdb/:kind/:id`
and `/titles/:id`; `ids` on the admin Netflix title too). These IDs live only
on the TMDB title, so every title mapped to it shows the same ones — the
FlixPatrol, Netflix and TMDB mapping drawers show the same ID list and the
same Details / Top 10 history / Netflix history / Ratings tabs — and moving a
title to another TMDB title shows that title's. Each ID records where it came
from (`ids.sources`) and is only replaced by the same or a stronger source:
entered by hand > TMDB > Wikidata > an automatic match > a Netflix title
mapped to it (its Tudum Netflix ID fills the TMDB title's while no other
source has one). The whole mapping, one row per chart title with every ID,
is the database view `mapping_refs` (`GET /api/v1/admin/mapping-refs`, or
"Export reference" on the TMDB mappings tab). An IMDb, Wikidata or RT ID you enter on a title is stored as its TMDB
title's, set by hand (clear it to hand it back). Data health lists titles
whose deciding match or fix came with another IMDb ID than their TMDB
title's, and Netflix titles whose own Netflix ID (Tudum) differs from their
TMDB title's — usually a match to another title of the same name ("check
mapping"), unless it is a special or season of a show whose own Netflix title
has that ID ("same show").

A title's mapping is computed, never copied: matching, Netflix IDs and fixes
by hand store **evidence** (what each source says the title is), and the
mapping follows from the title's own evidence and its linked titles' (see
Links). Taking a match or fix back therefore also takes back what linked
titles had from it. "Match again" lifts a fix by hand: the title's own
earlier match applies again (or the fix stays as its match if it has none)
until matching finds another; "Clear and match again" forgets it.

Every mapping change is written by one writer and recorded with who made it
(a job, link sharing or the admin UI): see the History section of the title
and mapping drawers, or `GET /api/v1/admin/mapping-events`. The follow-up
(TMDB details, IDs, season numbers) runs in a `mapping.refresh` job, so admin
fixes return quickly.

- **Links.** A Netflix title and a FlixPatrol title with the same normalized
  name (with or without ": Season N") in the same Netflix list (Films / TV)
  that were on Netflix's charts in the same or an adjacent week are linked.
  Linked titles share one mapping; each title weighs itself and the titles
  directly linked to it (only those linked to nothing else): a title fixed by hand wins, then a Netflix title
  mapped by its Netflix ID, then FlixPatrol's match (made with the title
  page's year), then Netflix's. Linked titles that disagree at the deciding
  level (two hand fixes, or two same-name FlixPatrol titles such as a show and
  its revival) are a conflict left for review (Mapping → TMDB mappings → Conflicts filter). Links are refreshed by
  `titles.enrich`, `netflix.top10.import` and `netflix.titles.match` (or
  "Sync links now").
- **Netflix titles** are matched by: a linked or same-name FlixPatrol title;
  their **Netflix ID** — Tudum's Top 10 search (`netflix.com/tudum/top10`)
  gives each charted title's ID (a season's for TV, whose public title page
  redirects to the show), accepted when the name (a season title for TV),
  list and a charted week agree, then mapped to TMDB through
  `tmdb_titles.netflix_id` or Wikidata; Wikidata items with a Netflix ID whose
  English label is the name (the other TMDB kind only if released in the year
  it first charted or the one before; never an item with a Netflix ID other
  than the title's own — such earlier matches are redone by the next Netflix
  matching run); JustWatch's US Netflix catalogue as
  either kind; a single exact name in all of JustWatch US.
- **Fixing.** Fix a single title in its drawer (TMDB kind + ID, or an IMDb /
  Wikidata ID resolved through TMDB's `/find`, then Wikidata), or move
  everything on a wrong TMDB title at once from Mapping → TMDB mappings → Move all.

### Seasons

Netflix lists TV shows per season (`season_title`, e.g. "Wednesday: Season 2").
Each season title is mapped to a TMDB season number: from the title itself
("Season 2", "S3", "Limited Series"), a TMDB season of that name, "Part /
Volume / Chapter N", the TMDB season that aired just before it first charted
(or before the date in a dated title such as "Raw: September 28, 2026"), or
the only season of a single-season show. Unresolved ones show up under
Netflix titles → "Season without a number" for fixing by hand.

FlixPatrol charts name only the show, so TV rankings get a season too
(`season_source`): `chart` when the chart name says so, `netflix` on Netflix
charts from the best-ranked season of the show in Netflix's official Top 10
that week (the country's list, else global), otherwise `tmdb_air_date` (the
latest season aired by that date). They are refreshed after scrapes, matching,
TMDB metadata and Netflix jobs.

Only one FlixPatrol page is fetched at a time across all jobs
(`SCRAPER_MIN_INTERVAL` apart). A schedule cannot be queued twice (409).
Set `ALERT_WEBHOOK_URL` to get failed runs and stale charts as JSON, Slack,
Discord or ntfy messages.

### Data sources and terms

- **FlixPatrol** (charts, title pages) through your FlareSolverr.
- **Netflix Top 10** (`netflix.com/tudum/top10` TSV downloads), and the Top
  10 page's own search for Netflix IDs (an undocumented persisted GraphQL
  query, sent anonymously; `NETFLIX_TOP10_SEARCH=false` turns it off).
- **Netflix "What We Watched" engagement reports** (`.xlsx` downloads linked
  from about.netflix.com), and **Wikimedia pageviews** (REST API, no key;
  identified by User-Agent, 200 requests a minute).
- **IMDb non-commercial datasets** (`title.ratings.tsv.gz`) — personal and
  non-commercial use only; see https://developer.imdb.com/non-commercial-datasets/.
- **TMDB** (search, details, watch providers — watch provider data is by
  JustWatch and must be attributed; daily ID exports for popularity), **Wikidata** (SPARQL, CC0; IDs and film distributors),
  **JustWatch** GraphQL, **Rotten Tomatoes** search index, **MDBList** (optional).
