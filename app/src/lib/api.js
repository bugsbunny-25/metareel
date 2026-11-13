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

  // The signed-in user's library
  myTitles: (offset = 0, signal) => request('GET', `/me/titles${qs({ limit: 200, offset })}`, { signal }),
  myTitle: (kind, id, signal) => request('GET', `/me/titles/${kind}/${id}`, { signal }),
  addToWatchlist: (kind, id) => request('PUT', `/me/watchlist/${kind}/${id}`, { body: {} }),
  removeFromWatchlist: (kind, id) => request('DELETE', `/me/watchlist/${kind}/${id}`),
  watchlistInsights: (country, days, signal) => request('GET', `/me/watchlist/insights${qs({ country, days })}`, { signal }),

  // The diary: one row per watch, newest local day first, cursor-paged.
  diary: ({ kind, rated, from, to, cursor, limit = 60 } = {}, signal) =>
    request('GET', `/me/watches${qs({ tmdb_kind: kind, rated: rated ? 'true' : undefined, from, to, cursor, limit })}`, { signal }),
  logWatch: (input) => request('POST', '/me/watches', { body: input }),
  updateWatch: (id, input) => request('PUT', `/me/watches/${id}`, { body: input }),
  deleteWatch: (id) => request('DELETE', `/me/watches/${id}`),
  tags: (signal) => request('GET', '/me/tags', { signal }),

  // Stats over local days (YYYY-MM-DD).
  statsSummary: (from, to, signal) => request('GET', `/me/stats/summary${qs({ from, to })}`, { signal }),
  statsCalendar: (from, to, signal) => request('GET', `/me/stats/calendar${qs({ from, to })}`, { signal }),
}
