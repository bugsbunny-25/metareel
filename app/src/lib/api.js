// API client for the user app. Same origin, so the HttpOnly session cookie set by
// /api/v1/auth/login authenticates every call: the public data routes accept a user
// session, and /api/v1/me/* requires one. The app never sends an API key.

const BASE = '/api/v1'

let onUnauthorized = () => {}
/** Called on a 401 from a data route (the session expired or was revoked). */
export function setUnauthorizedHandler(fn) {
  onUnauthorized = fn
}

export class ApiError extends Error {
  constructor(status, message) {
    super(message)
    this.status = status
  }
}

async function request(method, path, { body, signal } = {}) {
  const res = await fetch(BASE + path, {
    method,
    signal,
    credentials: 'same-origin',
    headers: body === undefined ? { Accept: 'application/json' } : { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (res.status === 401 && !path.startsWith('/auth/')) onUnauthorized()
  if (!res.ok) {
    let message = res.statusText || `HTTP ${res.status}`
    try {
      message = (await res.json()).error ?? message
    } catch {
      // non-JSON error body; keep the status text
    }
    if (res.status === 429) message = `Too many requests. Try again in ${res.headers.get('Retry-After') ?? 'a minute'} seconds.`
    throw new ApiError(res.status, message)
  }
  if (res.status === 204) return null
  return res.json()
}

function qs(params) {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== null && v !== '') p.set(k, String(v))
  const s = p.toString()
  return s ? `?${s}` : ''
}

/** Chart category in a URL: movies | tv-shows. */
const seg = (category) => (category === 'tv_shows' || category === 'tv-shows' ? 'tv-shows' : 'movies')

export const api = {
  // Sign-in
  session: () => request('GET', '/auth/session'),
  login: (username, password) => request('POST', '/auth/login', { body: { username, password } }),
  logout: () => request('POST', '/auth/logout'),

  // Reference lists
  countries: (signal) => request('GET', '/countries', { signal }),
  providers: (signal) => request('GET', '/providers', { signal }),

  // Charts and movement
  top10: (category, country, provider, signal) => request('GET', `/top10/${seg(category)}/${country}/${provider}`, { signal }),
  top10History: (category, country, provider, from, to, signal) =>
    request('GET', `/top10/${seg(category)}/${country}/${provider}/history${qs({ from, to })}`, { signal }),
  movers: (country, provider, date, signal) => request('GET', `/movers${qs({ country, provider, date })}`, { signal }),
  breakouts: (date, signal) => request('GET', `/breakouts${qs({ date })}`, { signal }),
  availabilityChanges: (country, signal) => request('GET', `/availability/changes${qs({ country, limit: 200 })}`, { signal }),

  // Discover, per country: highly rated titles not charting here, titles charting abroad you
  // can't stream here, and which subscriptions earn their keep (subscribed = TMDB provider ids).
  hiddenGems: (country, params, signal) => request('GET', `/discover/${country}/hidden-gems${qs(params)}`, { signal }),
  elsewhere: (country, params, signal) => request('GET', `/discover/${country}/elsewhere${qs(params)}`, { signal }),
  rotation: (country, params, signal) => request('GET', `/discover/${country}/rotation${qs(params)}`, { signal }),
  // What a service added (per day, live from JustWatch; at most 31 days) and has announced.
  newOnService: (country, service, params, signal) => request('GET', `/titles/new/${country}/${service}${qs(params)}`, { signal }),
  upcomingOnService: (country, service, params, signal) => request('GET', `/titles/upcoming/${country}/${service}${qs(params)}`, { signal }),
  // Episodes airing (from/to, at most 62 days) of shows streamable in the country.
  calendar: (country, params, signal) => request('GET', `/calendar/${country}${qs(params)}`, { signal }),
  // Titles ranked by Top 10 points: period day|week|month, country / provider omitted = all.
  leaderboard: (params, signal) => request('GET', `/leaderboards${qs(params)}`, { signal }),
  // Netflix's own weekly Top 10 (global or a country; week = any date in it) and all-time lists.
  netflixWeek: (country, week, signal) => request('GET', `/netflix/top10${country ? `/${country}` : ''}${qs({ week })}`, { signal }),
  netflixMostPopular: (signal) => request('GET', '/netflix/most-popular', { signal }),
  // Chart analytics: genres, decay, release-lag, origins (country, provider, from, to).
  analytics: (name, params, signal) => request('GET', `/analytics/${name}${qs(params)}`, { signal }),

  // Search: charted titles only (with last_charted), all of TMDB (needs a TMDB key), and ID lookup.
  searchCharted: (q, kind, signal) => request('GET', `/search${qs({ q, kind, limit: 8 })}`, { signal }),
  searchTMDB: (q, kind, signal) => request('GET', `/me/titles/search${qs({ q, kind, limit: 10 })}`, { signal }),
  lookup: (params, signal) => request('GET', `/titles/lookup${qs(params)}`, { signal }),

  // Titles
  ratingsBatch: (ids, signal) => request('GET', `/titles/tmdb/ratings${qs({ ids: ids.join(',') })}`, { signal }),
  overview: (kind, id, signal) => request('GET', `/titles/tmdb/${kind}/${id}`, { signal }),
  timeline: (kind, id, country, signal) => request('GET', `/titles/tmdb/${kind}/${id}/timeline${qs({ country })}`, { signal }),
  history: (kind, id, signal) => request('GET', `/titles/tmdb/${kind}/${id}/history`, { signal }),
  availabilityHistory: (kind, id, signal) => request('GET', `/titles/tmdb/${kind}/${id}/availability/history`, { signal }),
  credits: (kind, id, signal) => request('GET', `/titles/tmdb/${kind}/${id}/credits`, { signal }),

  // People: profile and filmography from TMDB, plus the chart record of their titles.
  person: (id, signal) => request('GET', `/people/${id}`, { signal }),
  searchPeople: (q, signal) => request('GET', `/me/people/search${qs({ q, limit: 5 })}`, { signal }),

  // Your account: who you are, where you're signed in, your password and preferences.
  account: (signal) => request('GET', '/me/account', { signal }),
  changePassword: (current, next) => request('POST', '/me/password', { body: { current_password: current, new_password: next } }),
  endSession: (id) => request('DELETE', `/me/sessions/${id}`),
  endOtherSessions: () => request('DELETE', '/me/sessions'),
  preferences: (signal) => request('GET', '/me/preferences', { signal }),
  setPreferences: (prefs) => request('PUT', '/me/preferences', { body: prefs }),

  // The signed-in user's library
  myTitles: (offset = 0, signal) => request('GET', `/me/titles${qs({ limit: 200, offset })}`, { signal }),
  myTitle: (kind, id, signal) => request('GET', `/me/titles/${kind}/${id}`, { signal }),
  // Status, favourite or watchlist note (a rating comes from the latest rated watch, not from here).
  setTitle: (kind, id, patch) => request('PUT', `/me/titles/${kind}/${id}`, { body: patch }),
  // Clears status, favourite and note; watches are kept.
  clearTitle: (kind, id) => request('DELETE', `/me/titles/${kind}/${id}`),
  addToWatchlist: (kind, id) => request('PUT', `/me/watchlist/${kind}/${id}`, { body: {} }),
  removeFromWatchlist: (kind, id) => request('DELETE', `/me/watchlist/${kind}/${id}`),
  watchlistInsights: (country, days, signal) => request('GET', `/me/watchlist/insights${qs({ country, days })}`, { signal }),

  // The diary: one row per watch, newest local day first, cursor-paged.
  diary: ({ kind, rated, from, to, cursor, limit = 60 } = {}, signal) =>
    request('GET', `/me/watches${qs({ tmdb_kind: kind, rated: rated ? 'true' : undefined, from, to, cursor, limit })}`, { signal }),
  watch: (id, signal) => request('GET', `/me/watches/${id}`, { signal }),
  logWatch: (input) => request('POST', '/me/watches', { body: input }),
  updateWatch: (id, input) => request('PUT', `/me/watches/${id}`, { body: input }),
  deleteWatch: (id) => request('DELETE', `/me/watches/${id}`),
  tags: (signal) => request('GET', '/me/tags', { signal }),

  // Cinema subscriptions (AMC Stubs A-List, ODEON Limitless, ...): yours and well-known plans,
  // and whether each paid off (covered tickets' face value against what the plan cost).
  subscriptions: (signal) => request('GET', '/me/subscriptions', { signal }),
  createSubscription: (body) => request('POST', '/me/subscriptions', { body }),
  updateSubscription: (id, body) => request('PUT', `/me/subscriptions/${id}`, { body }),
  deleteSubscription: (id) => request('DELETE', `/me/subscriptions/${id}`),
  subscriptionSavings: (signal) => request('GET', '/me/subscriptions/savings', { signal }),

  // Where and how a watch happened: premium formats, cinemas and their screens, streaming services.
  formats: (signal) => request('GET', '/me/formats', { signal }),
  // Closed cinemas too (they rank after open ones), so an old watch can be logged where it happened.
  venues: (q, country, signal) => request('GET', `/me/venues${qs({ q, country, include_closed: 'true', limit: 12 })}`, { signal }),
  venue: (id, signal) => request('GET', `/me/venues/${id}`, { signal }),
  createVenue: (body) => request('POST', '/me/venues', { body }),
  createScreen: (venueId, body) => request('POST', `/me/venues/${venueId}/screens`, { body }),
  services: (country, signal) => request('GET', `/services/${country}`, { signal }),

  // Shows: every show sorted into stages, episodes still to air, one show's episodes, and
  // marking runs of episodes watched (scope: episode | season | up_to | all) or unwatched.
  showQueue: (staleDays, signal) => request('GET', `/me/shows${qs({ stale_days: staleDays })}`, { signal }),
  upcoming: (days, signal) => request('GET', `/me/upcoming${qs({ days })}`, { signal }),
  show: (id, includeSpecials, signal) => request('GET', `/me/shows/${id}${qs({ include_specials: includeSpecials ? 'true' : undefined })}`, { signal }),
  markEpisodes: (id, body) => request('POST', `/me/shows/${id}/watched`, { body }),
  // Deletes the watches of those episodes for good. params: { scope, season, episode, include_specials }
  unmarkEpisodes: (id, params) => request('DELETE', `/me/shows/${id}/watched${qs(params)}`),

  // Stats over local days (YYYY-MM-DD).
  statsSummary: (from, to, signal) => request('GET', `/me/stats/summary${qs({ from, to })}`, { signal }),
  statsCalendar: (from, to, signal) => request('GET', `/me/stats/calendar${qs({ from, to })}`, { signal }),
  statsYear: (year, signal) => request('GET', `/me/stats/year${qs({ year })}`, { signal }),
  statsRatings: (from, to, signal) => request('GET', `/me/stats/ratings${qs({ from, to })}`, { signal }),
}
