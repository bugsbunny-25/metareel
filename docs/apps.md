# Apps

The admin console, the web user app and the mobile app in detail. The mobile app has its own [README](../mobile/README.md).

## Web apps

Two Svelte 5 + Vite single-page apps ship with the server:

| App | Source | Served at | Talks to |
|---|---|---|---|
| Admin console | `web/` → `web/dist` (`UI_STATIC_DIR`) | `/admin` (Vite `base: '/admin/'`) | `/api/v1/admin` |
| User app | `app/` → `app/dist` (`APP_STATIC_DIR`) | `/` | the public API and `/api/v1/me`, with the session cookie |
| Mobile app | `mobile/` (Expo, React Native) → EAS builds with every release (`v*` tags): APK on the GitHub Release, iOS via TestFlight | native | the public API and `/api/v1/me`, with a Bearer session token ([mobile/README.md](../mobile/README.md)) |

The user app needs a signed-in user (accounts are created in the admin console)
and never sends an API key. Its design system is **Screening Room**: dark first
with a light mode, artwork-led, with the analytics on every screen. Tokens live
in `app/src/tokens.css`. Colour roles are fixed: white for the primary action,
green and red for movement, blue for new entries, orange for your own titles,
and tan for stale data. It has Home (breakout, KPIs with 14-day trends, the
Top 10, movement, your list, breakouts), Charts (Top 10 with trend, days,
IMDb, RT and origin), title pages (attention, timeline, In numbers, positions,
where to watch), Watchlist (your list joined with `/me/watchlist/insights`:
charting in a 30-day, 90-day or one-year window, streaming on a subscription
now, or recently gone from a service), Library (everything you track, with
status chips that filter, your average rating and its half-star spread,
favourites, a poster grid or a list, and a per-title menu that sets the
status or favourite through `PUT /me/titles/{kind}/{id}` or clears it) and
Diary (your last 4 films and 4 shows, your ratings, every watch grouped by its
local day, this year's totals, a year heatmap from `/me/stats/calendar`, and
edit and delete; deletes are permanent and ask first; each watch has a page,
`#/watch/:id`, with every detail and its formats and discs as logos), TV
(`#/tv`, as on mobile: To watch -- shows by stage from `/me/shows`, continue
watching, haven't watched in a while, not started, on hold, dropped, each with
its next episode, progress and a one-click "Watched" -- and Upcoming, from
`/me/upcoming` by day with episodes per week; as cards or a poster grid, the
choice and the segment remembered per browser; an episode opens its page
first, `#/episode/:show/:season/:episode` from
`/me/shows/{id}/episodes/{season}/{episode}`, with "View whole series"),
Movies (`#/movies`: released watchlist films To watch and Upcoming ones by
month, list or grid), You (`#/you`: your statistics at a glance, this year,
whether each cinema plan has broken even this period and what it saved, your
genres, your shows and films as posters, and links to Settings, the library,
the watchlist and the year in review; Detailed stats at `#/you/stats` from
`/me/stats/detailed`, Shows or Movies: time spent, episodes, rewatches,
most-watched with hours, shows by status, time left and the catch-up date,
upcoming watch time, trends, weekdays, ratings, genres, people and binges),
an Up next row on Home with one-click Watched, Trending and In cinemas rows on
Home (from `/titles/trending` and `/titles/in-cinemas/{country}`), a Top 10
badge on every poster of a title in a Top 10 in your country today (one
`/top10/{country}` per country, matched locally; not on the charts), a Top 10
graph on title pages for the first country in your country order (Settings,
`/me/preferences` `countries`) the title charted in, Movies' To watch sorted as
added or by release date, a show page (`#/show/:id`: a map of every episode, season tabs,
tick to mark, untick to delete after asking, "watched up to here"), a person page (`#/person/:id`
from `/people/:id`: photo, born/died, biography, IMDb link; credits, their charted titles and
what you've seen and bookmarked of their work; the titles of theirs that charted; and the
filmography by department with a Coming up list, film/series filter, newest/popular/top-rated
sort and appearances as themselves hidden by default; title pages link their cast and crew
here, and so do Stats' people) and Year in review (`#/stats`, one year at a
time from `/me/stats/year` and `/me/stats/ratings`: watches, hours, titles and
streaks; watches per month, weekday and hour; your rating spread and how far
you sit from TMDB, IMDb and RT, overall and per title; genres, decades,
countries and languages; most watched; where you watched; and people and spend
once there is data for them). Search (⌘K / Ctrl K
or `/`) keeps three result lists apart: titles with chart data (`/api/v1/search`,
with when they last charted), all of TMDB (`/me/titles/search`, for logging
anything) and people (`/me/people/search`, opening their person page); a pasted IMDb ID or an IMDb, TMDB or Netflix link resolves through
`/titles/lookup`. ↵ opens, ⌘/Ctrl ↵ logs a watch, ⇧ ↵ toggles the watchlist.
Watchlist toggles and "Log a watch" write through `/api/v1/me`. The log
dialog takes the time to the minute, day, month or year; a rating, tags and
notes; and where you watched, which opens that place's fields: a cinema
(searched in `/me/venues`, closed and demolished ones included after open
ones, or added there), its screen, seat, screening type,
premium formats (`/me/formats`, with their logos) and tickets with fees and snacks in one
currency, or "covered by" one of your cinema subscriptions (picked for you at
the plan's chain) with the ticket's face value; a streaming service and plan, with a price for rentals and
purchases; a physical-media watch's disc (4K UHD Blu-ray, Blu-ray or DVD, with
their logos; or VHS, a file or a film print), its label (Criterion, Arrow Video,
Kino Lorber… with logos) and edition; a TV channel. Picture and sound
(3D, resolution, HDR, audio, languages, aspect ratios, screen size) are
optional and folded away. A watch at a cinema is entered and filed in the
cinema's time zone. Discover (`#/discover`, for the top bar's country) has
eight tabs: new & upcoming on the top bar's service (`/titles/new` and
`/titles/upcoming`, by day), the episode calendar (`/calendar/:country`,
your shows marked), leaderboards (`/leaderboards` for the service, the
country or the world), the Netflix Top 10 (global and country weeks with
views, and the all-time most popular), trends (`/analytics/genres`,
`decay`, `release-lag`, `origins`), hidden gems (the best 200 from `/discover/:country/hidden-gems`,
filtered in the browser by kind, IMDb rating and service, hiding what you've
seen, with per-service and genre tallies), popular elsewhere (where each
title charts abroad and any rent or buy offers here) and which services to
pay for (`/rotation` with the services you pay for, a preference, so the
advice is keep, pause or add; each score broken into chart points, shows with
new episodes, arrivals and top-rated titles). Offers are grouped into the
service you'd pay for using the provider ids the rotation lists per service.
Settings (`#/settings`, from the account menu) holds the
account (sign out), preferences saved to the account (default charts for a new
device, currency, screen unit, when a show counts as not watched in a while),
the theme for this device (dark, light or match the system), a password
change, every signed-in session with sign out per device or everywhere
else, and your cinema subscriptions (`/me/subscriptions`: AMC Stubs A-List,
Regal Unlimited, ODEON Limitless… with price per month or year, chain, the
runs you had it (cancel and resubscribe; the gaps aren't billed) and price
changes over time). Stats weighs each plan's covered tickets at face value against what it
cost, month by month with the break-even, and says what a plan would have had
to cost to pay off at the chains you buy tickets from
(`/me/subscriptions/savings`).

```bash
make dev-ui      # admin console at http://localhost:5173/admin/
make dev-app     # user app at http://localhost:5174
(cd web && npm run build) && (cd app && npm run build)   # production builds
```

The Docker image builds both. `UI_SERVE_STATIC=false` turns static serving
off (for example when a reverse proxy serves the files).
