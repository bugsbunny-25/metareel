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

## Wikidata (SPARQL, `client/wikidata_sparql.go`)

- POST `https://query.wikidata.org/sparql` (form `query=`, `Accept: application/sparql-results+json`, a descriptive User-Agent per Wikimedia policy). Batch 100 TMDB IDs per query: `VALUES ?tmdb {...} ?item wdt:P4947 ?tmdb` (movies; TV = `P4983`), optional `P345` IMDb, `P1258` RT (`m/...`, `tv/...`), `P1712` Metacritic, `P6127` Letterboxd. Only numeric IDs go into the query.

## TMDB details (`client/tmdb_details.go`)

- `/{movie|tv}/{id}?append_to_response=external_ids,watch/providers[,release_dates]` — one call for metadata, IMDb/Wikidata IDs, watch providers for every country (`results.{CC}.{flatrate,free,ads,rent,buy}`, plus a per-country `link`) and release dates (type 4 = digital). Watch provider data is JustWatch's and must be attributed.

## Netflix Top 10 (`service/netflix_import.go`)

- TSVs at `https://www.netflix.com/tudum/top10/data/{all-weeks-global,all-weeks-countries,most-popular}.tsv`. HEAD returns 403 and `If-Modified-Since` / `Range` are ignored, so `client.Downloader` GETs and closes the body when `Last-Modified` is unchanged. countries is ~32 MB / 500k rows (filtered by `NETFLIX_TOP10_COUNTRIES`). `week` is the Sunday ending a Monday–Sunday week; `N/A` means empty. No IDs or years: titles are matched via a same-name FlixPatrol title that charted on Netflix, else JustWatch's US Netflix catalogue, else a single exact-name hit in all of JustWatch US.

## IMDb datasets

- `https://datasets.imdbws.com/title.ratings.tsv.gz` (~9 MB, daily, ETag). Personal / non-commercial use only. Imported for known IMDb IDs (`IMDB_DATASET_SCOPE`); `imdb_dataset` is the first-choice IMDb rating in ratings responses.

## How mapping works (`titles.enrich`: `title_enrich.go` + `title_matcher.go`)

Scrapes only record titles (`match_status = pending`) and queue `titles.enrich` for them.

1. FlixPatrol title page (read once, stored as `titles.fp_*`) → name, premiere date, kind (fallback: Top 10 name, its `: Season N`-less form, slug year).
2. For the page's kind, then the chart's kind if different: JustWatch path lookup → JustWatch search → TMDB search. Accept only title match (normalized) **and** year within ±1 (`pickCandidate`).
3. TMDB external IDs → IMDb + Wikidata → RT slug; if still missing, RT search (`FindRTSlug`).
4. Unmatched titles retry after 1, 3, 7, then every 30 days (`next_match_at`); RT lookups back off the same way (`next_rt_at`), Wikidata batch first. `manual` titles (saved in the admin UI) are never touched; "Match again" / `POST /admin/titles/:id/rematch` resets the backoff.

Ratings (`/titles/tmdb/{kind}/{id}/ratings`) store every provider's values in `title_rating_sources`; RT's own numbers win for RT, then the preferred provider.
