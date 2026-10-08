---
name: external-providers
description: Behaviour, quirks and debugging recipes for metareel's upstreams — FlixPatrol (Cloudflare/FlareSolverr), JustWatch GraphQL, TMDB, Rotten Tomatoes Algolia, MDBList, Wikidata — and how title matching uses them. Use when changing clients, matching logic, ratings or releases, or when an upstream call fails.
---

# External providers

Probe an upstream live (curl or a throwaway `go run`) before writing code against
it, and again after — several fields and enums below were only discoverable that way.

## FlixPatrol (charts + title pages)

- Behind Cloudflare: direct requests (and even a normal browser) get `403 "Just a moment..."`. The service fetches through **FlareSolverr** when `FLARESOLVERR_URL` is set (`FlareSolverrFetcher`; session reuse via `FLARESOLVERR_SESSION`, first solve ~10s, then ~1s).
- Do **not** add our own Cloudflare evasion (TLS spoofing, solvers, headless tricks) and do not drive a browser through the challenge yourself; the user runs FlareSolverr and runs live tests themselves (`go run ./cmd/fpcheck -flaresolverr http://<host>:8191`).
- For test fixtures use Wayback Machine snapshots (`https://web.archive.org/web/<ts>id_/<url>`; responses may be gzip/zstd-compressed).
- Title page header (`/title/<slug>/`): `<h1>` = full title; next sibling row: first item `Movie`/`TV Show`, item `title="Premiere"` holds `MM/DD/YYYY`, item with `.fflag` holds the country. Parsed by `parseTitleDetails`.
- Slugs with a `-YYYY` suffix disambiguate same-name titles (`YearFromSlug`).
- Chart pages: tables after headings "TOP 10 Movies" / "TOP 10 TV Shows". The effective chart date flips at 12:00 UTC.

## JustWatch GraphQL (`https://apis.justwatch.com/graphql`, no key)

- **Introspection is disabled.** Invalid enum values / fields return **HTTP 422 with no detail** through the Go client. To see the real error: `graphql.ConstructQuery(&q, vars)`, then POST the query text with curl/python and read the body (it names the bad field, e.g. `standardWebURL` vs `standardWebUrl`).
- Lookups: `urlV2(fullPath: "/us/movie/<slug>")`, `node(id: "tm123"|"ts123")`, `popularTitles(filter: {searchQuery, objectTypes, packages})`, `newTitles(pageType: NEW|UPCOMING, date, filter)`, `packages(country, platform: WEB)`.
- `content.scoring`: `imdbScore, imdbVotes, tmdbScore, tmdbPopularity, jwRating, tomatoMeter, certifiedFresh` — no RT audience score.
- Seasons: `externalIds.tmdbId` is `"<showId>:<season>"`; use `show.content.externalIds`. Season descriptions are often empty → fall back to the show.
- `newTitles`: NEW is per day (one call per date); UPCOMING ignores `date`. The `objectTypes` filter is ignored here — filter movie/TV in Go. The service-specific release date is in `upcomingReleases[]` (prefer `DIGITAL` for the requested package; ignore other packages like `tmd`/theatrical).
- **Package codes vary by country**: Prime Video `amp` (US/GB/DE/JP) vs `prv` (IN/BR); Paramount+ `pmp` vs `ppp` (US Premium) / `ppe`; HBO Max `mxx` vs `mxu` (JP); Apple TV+ is `atp` (not `apt`). Map FlixPatrol providers through `justWatchPackages` (ordered candidates) and resolve against `/api/v1/services/{country}`.

## TMDB (`TMDB_API_KEY`, v3, key in query string)

- Movie search results use `title`/`release_date`; **TV uses `name`/`first_air_date`**.
- `/{movie|tv}/{id}/external_ids` gives `imdb_id`, `wikidata_id`. `/{movie|tv}/{id}` gives title + date.
- The key is in the URL → wrap transport errors with `redactErr`.

## Rotten Tomatoes (Algolia index used by rottentomatoes.com, as Seerr does)

- POST `https://79frdp12pn-dsn.algolia.net/1/indexes/*/queries`, index `content_rt`, filter `isEmsSearchable=1 AND type:"movie"|"tv"`; public app id/key in `client/rottentomatoes.go`.
- Searching the **slug's vanity** (`apex_2026`) returns that exact hit first → `BySlug`. Otherwise `Search(name, year)` ports Seerr's scoring (Jaro title similarity, alternate titles ×0.8, year penalty 0.4/yr, min score 0.175). Without a year, don't search (ambiguous).
- Slugs are stored as `m/<vanity>` / `tv/<vanity>` (same as Wikidata P1258). Hits may lack `rottenTomatoes` scores or individual fields.

## MDBList (`MDBLIST_API_KEY`, 1000 req/day free)

- `GET https://api.mdblist.com/tmdb/{movie|show}/{id}?apikey=` → `ratings: [{source, value, votes, url}]`; sources include `imdb`, `tomatoes`, `popcorn`/`tomatoesaudience`, `metacritic`, `letterboxd`, `trakt`, … plus top-level `score`, `score_average`. Shape inferred from Kometa's client; verify once a key is available. 429 has `Retry-After`.

## Wikidata

- `GET /w/rest.php/wikibase/v1/entities/items/{Q}/statements?property=P1258` → RT id (`m/...` or `tv/...`).

## How mapping works (scrape time, `flixpatrol_job.go`)

1. FlixPatrol title page → name, premiere year, kind (fallback: Top 10 name + slug year).
2. For the page's kind, then the chart's kind if different: JustWatch path lookup → JustWatch search → TMDB search. Accept only title match (normalized) **and** year within ±1 (`pickCandidate`).
3. TMDB external IDs → IMDb + Wikidata → RT slug; if still missing, RT search (`FindRTSlug`).
4. Titles with TMDB+IMDb+RT are not re-matched; fix wrong ones in the admin UI (title drawer → Find matches).

Ratings (`/titles/tmdb/{kind}/{id}/ratings`) store every provider's values in `title_rating_sources`; RT's own numbers win for RT, then the preferred provider.
