# metareel

[![CI](https://github.com/bugsbunny-25/metareel/actions/workflows/ci.yml/badge.svg)](https://github.com/bugsbunny-25/metareel/actions/workflows/ci.yml)
[![CodeQL](https://github.com/bugsbunny-25/metareel/actions/workflows/codeql.yml/badge.svg)](https://github.com/bugsbunny-25/metareel/actions/workflows/codeql.yml)
[![Container](https://github.com/bugsbunny-25/metareel/actions/workflows/container.yml/badge.svg)](https://github.com/bugsbunny-25/metareel/actions/workflows/container.yml)

metareel follows what the world is streaming and keeps track of what you
watch. It collects the daily Top 10 charts of the streaming services in every
country, links each title to TMDB, IMDb and Rotten Tomatoes, and puts your own
watchlist and diary next to them. It runs as one self-hosted server with a web
app, an admin console, iOS and Android apps, and an API.

## Features

**Charts and trends**
- Daily Top 10s per service and country (from FlixPatrol), Netflix's official
  weekly Top 10 and its viewing reports, with history back to when you started.
- Movement, debuts, number ones, days in the Top 10, and breakouts: titles
  rising in several countries at once.
- Each title's run: rank history, a timeline of what happened to it, Wikipedia
  attention, TMDB popularity, and where it charts abroad.
- Trending today and what's in cinemas near you.

**Ratings and where to watch**
- IMDb, Rotten Tomatoes (critics and Popcornmeter), TMDB and more on every title.
- Where it streams, rents or sells in each of your countries, with links, and
  when it arrived on or left a service.
- Who made it: cast, crew and person pages with their filmographies.

**Your tracking**
- A watchlist that tells you which titles are charting, streaming now or gone.
- A diary of every watch: when (to the minute, or just the year), where (cinema
  and screen, streaming service, disc, TV), how (IMAX, Dolby, 70mm, 3D…),
  rating, tags and notes. Rewatches are their own entries.
- Shows episode by episode: new series and seasons, up next, continue
  watching, upcoming episodes and reminders.
- Statistics and a year in review: time watched, streaks, genres, people,
  decades, how your ratings compare with the critics', and what you spent.
- Cinema subscriptions (AMC A-List, ODEON Limitless…): whether each plan paid
  for itself this month.
- Imports from TV Time and Suite: titles are matched to TMDB, you review the
  matches, and anything you have already logged is skipped. An import can be
  undone.
- A collection of the discs you own (DVD, Blu-ray, 4K UHD, Steelbooks, box
  sets): scan the barcode to look the release up, link it to its films, keep
  photos of the front, back and slipcover, every disc's picture and sound and
  special features, and what you paid where. The phone app shows it as a
  shelf of spines.

**Discover**
- New and upcoming releases per service, an episode calendar, leaderboards,
  hidden gems, and which services are worth paying for this month.

**Apps**
- **Web app** at `/`: everything above, in a dark, artwork-led design.
- **Mobile app** for iOS and Android: the same, plus home-screen widgets, a
  share sheet, notifications, offline logging and layouts for foldables and
  tablets. Android builds are attached to each
  [release](https://github.com/bugsbunny-25/metareel/releases); iOS ships
  through TestFlight.
- **Admin console** at `/admin`: data health, title mapping fixes, schedules,
  users, API keys, cinemas and premium formats.

**For your own tools**
- A public REST API (API key or user session) with OpenAPI docs at `/docs`,
  and signed webhooks for chart changes, new number ones, availability changes
  and breakouts.

## Get started

You need Docker, a [TMDB API key](https://www.themoviedb.org/settings/api) and
a [FlareSolverr](https://github.com/FlareSolverr/FlareSolverr) instance (the
charts are fetched through it).

```bash
cp .env.example .env    # set TMDB_API_KEY, FLARESOLVERR_URL, AUTH_ADMIN_USERNAME and AUTH_ADMIN_PASSWORD
make up                 # builds the image and starts metareel and Redis
```

Then open `http://localhost:8080` for the web app and `/admin` for the console,
signed in as that admin. Add chart schedules for the countries and services you
want in **Schedules**, people in **Users & sessions**, and keys for other tools
in **Settings & API keys**. Every setting is in [`.env.example`](.env.example).

## Releases

Every release has one version, `X.Y.Z`, shared by the server, the container
image (`ghcr.io/bugsbunny-25/metareel`), the web apps and the mobile apps. See
[releases](https://github.com/bugsbunny-25/metareel/releases).

## Documentation

| | |
|---|---|
| [docs/apps.md](docs/apps.md) | The web app, admin console and mobile app in detail |
| [docs/api.md](docs/api.md) | Endpoints, access (keys, users, sessions), tracking API, webhooks |
| [docs/data.md](docs/data.md) | Jobs, title matching across sources, data sources and their terms |
| [docs/development.md](docs/development.md) | Running locally, CI, versions and releasing, migrations |
| [docs/architecture.md](docs/architecture.md) | How it works inside |
| [mobile/README.md](mobile/README.md) | Building and releasing the mobile app |

## Data and credits

This product uses the TMDB API but is not endorsed or certified by TMDB.
Streaming availability is from JustWatch, via TMDB. Charts come from
FlixPatrol and Netflix's Top 10; ratings from IMDb's non-commercial datasets
(personal, non-commercial use only), Rotten Tomatoes and MDBList; IDs from
Wikidata; cinemas from OpenStreetMap (© OpenStreetMap contributors, ODbL).
Details and each source's terms are in [docs/data.md](docs/data.md).
