// The admin UI talks only to /api/v1/admin, which serves the same data as the
// public API without needing an API key (and, when the server requires it,
// with the admin's session cookie), plus /api/v1/auth to sign in and out.
const BASE = '/api/v1/admin'
const AUTH = '/api/v1/auth'

// Called when the admin API answers 401 (the session ended or expired).
let onUnauthorized = () => {}
export function setUnauthorizedHandler(fn) {
  onUnauthorized = fn
}

export class ApiError extends Error {
  constructor(message, status) {
    super(message)
    this.status = status
  }
}

async function req(path, { method = 'GET', body, query, withTotal = false, base = BASE } = {}) {
  let url = base + path
  if (query) {
    const p = new URLSearchParams()
    for (const [k, v] of Object.entries(query)) {
      if (v !== '' && v !== null && v !== undefined) p.set(k, String(v))
    }
    const qs = p.toString()
    if (qs) url += `?${qs}`
  }
  const res = await fetch(url, {
    method,
    cache: 'no-store',
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    if (res.status === 401 && base === BASE) onUnauthorized()
    const data = await res.json().catch(() => ({}))
    throw new ApiError(data.error ?? `Request failed (HTTP ${res.status})`, res.status)
  }
  if (res.status === 204) return null
  const data = await res.json()
  if (withTotal) return { items: data, total: Number(res.headers.get('X-Total-Count') ?? data.length) }
  return data
}

// ── Dashboard ───────────────────────────────────────────────────────
export const getStats = () => req('/stats')

// ── Titles ──────────────────────────────────────────────────────────
export const listTitles = (q) => req('/titles', { query: q })
export const getTitle = (id) => req(`/titles/${id}`)
export const patchTitle = (id, body) => req(`/titles/${id}`, { method: 'PATCH', body })
export const getTitleRankings = (id, q) => req(`/titles/${id}/rankings`, { query: q })
export const getTitleCandidates = (id, q) => req(`/titles/${id}/candidates`, { query: q })
export const getTmdbRankings = (kind, tmdbId, q) => req(`/titles/tmdb/${kind}/${tmdbId}/rankings`, { query: q })
export const getTmdbNetflix = (kind, tmdbId) => req(`/titles/tmdb/${kind}/${tmdbId}/netflix`)
export const getRatings = (kind, tmdbId, refresh = false) =>
  req(`/titles/tmdb/${kind}/${tmdbId}/ratings`, { query: { refresh: refresh || '' } })

// ── Netflix official Top 10 mapping ─────────────────────────────────
export const listNetflixTitles = (q) => req('/netflix/titles', { query: q })
export const getNetflixTitle = (id) => req(`/netflix/titles/${id}`)
export const patchNetflixTitle = (id, body) => req(`/netflix/titles/${id}`, { method: 'PATCH', body })
export const rematchNetflixTitle = (id, clear = false) => req(`/netflix/titles/${id}/rematch`, { method: 'POST', body: { clear } })
export const getNetflixTitleCandidates = (id, q) => req(`/netflix/titles/${id}/candidates`, { query: q })
export const patchNetflixSeason = (id, body) => req(`/netflix/seasons/${id}`, { method: 'PATCH', body })

// ── TMDB mapping (every source on one TMDB title) ───────────────────
export const listMappings = (q) => req('/mappings', { query: q })
export const getMapping = (kind, tmdbId) => req(`/mappings/${kind}/${tmdbId}`)
export const remapMapping = (kind, tmdbId, body) => req(`/mappings/${kind}/${tmdbId}/remap`, { method: 'POST', body })
export const syncMappings = () => req('/mappings/sync', { method: 'POST' })
export const listMappingEvents = (q) => req('/mapping-events', { query: q })
export const listMappingRefs = (q) => req('/mapping-refs', { query: q })
export const reviewMappings = (q) => req('/mappings/review', { query: q })

// ── Roadmap views: breakouts, discovery, people, calendar, timeline ───
export const getBreakouts = (q) => req('/breakouts', { query: q })
export const getHiddenGems = (country, q) => req(`/discover/${country}/hidden-gems`, { query: q })
export const getElsewhere = (country, q) => req(`/discover/${country}/elsewhere`, { query: q })
export const getRotation = (country, q) => req(`/discover/${country}/rotation`, { query: q })
export const getPerson = (id) => req(`/people/${id}`)
export const getCalendar = (country, q) => req(`/calendar/${country}`, { query: q })
export const getTimeline = (kind, tmdbId, q) => req(`/titles/tmdb/${kind}/${tmdbId}/timeline`, { query: q })
export const getCredits = (kind, tmdbId) => req(`/titles/tmdb/${kind}/${tmdbId}/credits`)
export const getTitleReleases = (kind, tmdbId, q) => req(`/titles/tmdb/${kind}/${tmdbId}/releases`, { query: q })
export const getReviewSuggestions = (subject, id) => req(`/mappings/review/${subject}/${id}/suggestions`)

// ── Top 10 ──────────────────────────────────────────────────────────
export function getTop10({ country, provider, category, date }) {
  const seg = category === 'tv_shows' ? 'tv-shows' : 'movies'
  const path = provider ? `/top10/${seg}/${country}/${provider}` : `/top10/${seg}/${country}`
  return req(path, { query: { date } }).catch((err) => {
    if (err.status === 404) return { items: [], date: date || null }
    throw err
  })
}

// ── Releases ────────────────────────────────────────────────────────
export const listServices = (country) => req(`/services/${country}`)
export const getReleases = (type, country, service, q) =>
  req(`/titles/${type}/${country}/${service}`, { query: q })

// ── Schedules & runs ────────────────────────────────────────────────
export const listSchedules = () => req('/task-schedules')
export const getSchedule = (id) => req(`/task-schedules/${id}`)
export const createSchedule = (body) => req('/task-schedules', { method: 'POST', body })
export const updateSchedule = (id, body) => req(`/task-schedules/${id}`, { method: 'PATCH', body })
export const addScheduleTargets = (id, targets) =>
  req(`/task-schedules/${id}/flixpatrol-targets`, { method: 'POST', body: { targets } })
export const runScheduleNow = (id) => req(`/task-schedules/${id}/run-now`, { method: 'POST' })
export const listRuns = (q) => req('/task-runs', { query: q, withTotal: true })
export const getRun = (id) => req(`/task-runs/${id}`)
export const getRunLogs = (id) => req(`/task-runs/${id}/logs`)

// ── Sign-in, users and sessions ─────────────────────────────────────
export const getAuthSession = () => req('/session', { base: AUTH })
export const login = (username, password) => req('/login', { method: 'POST', body: { username, password }, base: AUTH })
export const logout = () => req('/logout', { method: 'POST', base: AUTH })
export const listUsers = () => req('/users')
export const createUser = (body) => req('/users', { method: 'POST', body })
export const updateUser = (username, body) => req(`/users/${encodeURIComponent(username)}`, { method: 'PATCH', body })
export const deleteUser = (username) => req(`/users/${encodeURIComponent(username)}`, { method: 'DELETE' })
export const logoutUser = (username) => req(`/users/${encodeURIComponent(username)}/logout`, { method: 'POST' })
export const listSessions = () => req('/sessions')
export const revokeSession = (id) => req(`/sessions/${id}`, { method: 'DELETE' })

// ── Settings & API keys ─────────────────────────────────────────────
// Premium formats a watch can carry (IMAX, Dolby Cinema, 4DX, 70mm, ...).
// The admin list includes retired ones; the user-facing list does not.
export const listWatchFormats = () => req('/watch-formats')
export const createWatchFormat = (body) => req('/watch-formats', { method: 'POST', body })
export const updateWatchFormat = (id, body) => req(`/watch-formats/${id}`, { method: 'PATCH', body })
export const deleteWatchFormatLogo = (id) => req(`/watch-formats/${id}/logo`, { method: 'DELETE' })
// The logo goes up as the raw image (req only sends JSON); the server works out its type.
export async function uploadWatchFormatLogo(id, file) {
  const res = await fetch(`${BASE}/watch-formats/${id}/logo`, {
    method: 'PUT', cache: 'no-store', headers: { 'Content-Type': file.type || 'application/octet-stream' }, body: file,
  })
  if (!res.ok) {
    if (res.status === 401) onUnauthorized()
    const data = await res.json().catch(() => ({}))
    throw new ApiError(data.error ?? `Upload failed (HTTP ${res.status})`, res.status)
  }
  return res.json()
}

export const getSettings = () => req('/settings')
export const updateSettings = (body) => req('/settings', { method: 'PATCH', body })
export const listApiKeys = () => req('/api-keys')
export const createApiKey = (name) => req('/api-keys', { method: 'POST', body: { name } })
export const renameApiKey = (id, name) => req(`/api-keys/${id}`, { method: 'PATCH', body: { name } })
export const rotateApiKey = (id) => req(`/api-keys/${id}/rotate`, { method: 'POST' })
export const deleteApiKey = (id) => req(`/api-keys/${id}`, { method: 'DELETE' })

// ── Webhooks ────────────────────────────────────────────────────────
export const listWebhooks = () => req('/webhooks')
export const createWebhook = (body) => req('/webhooks', { method: 'POST', body })
export const updateWebhook = (id, body) => req(`/webhooks/${id}`, { method: 'PATCH', body })
export const deleteWebhook = (id) => req(`/webhooks/${id}`, { method: 'DELETE' })
export const rotateWebhookSecret = (id) => req(`/webhooks/${id}/rotate-secret`, { method: 'POST' })
export const testWebhook = (id) => req(`/webhooks/${id}/test`, { method: 'POST' })
export const listWebhookDeliveries = (id, q) => req(`/webhooks/${id}/deliveries`, { query: q })
export const getWebhookDelivery = (id) => req(`/webhook-deliveries/${id}`)
export const retryWebhookDelivery = (id) => req(`/webhook-deliveries/${id}/retry`, { method: 'POST' })

// ── Jobs ────────────────────────────────────────────────────────────
export const listTaskTypes = () => req('/task-types')
export const runJob = (type, body = {}) => req(`/jobs/${type}/run`, { method: 'POST', body })
export const backfill = (body) => req('/backfill', { method: 'POST', body })
export const rematchTitle = (id, clear = false) => req(`/titles/${id}/rematch`, { method: 'POST', body: { clear } })
export const getDataQuality = () => req('/data-quality')

// ── Insights ────────────────────────────────────────────────────────
export const getLeaderboard = (q) => req('/leaderboards', { query: q })
export const getMovers = (q) => req('/movers', { query: q })
export const getCharts = (q) => req('/charts', { query: q })
export const getNetflixTop10 = (country, q) => req(country ? `/netflix/top10/${country}` : '/netflix/top10', { query: q })
export const getTitleOverview = (kind, tmdbId, q) => req(`/titles/tmdb/${kind}/${tmdbId}`, { query: q })
export const getAnalytics = (name, q) => req(`/analytics/${name}`, { query: q })
