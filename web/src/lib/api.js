// The admin UI talks only to /api/v1/admin, which serves the same data as the
// public API without needing an API key.
const BASE = '/api/v1/admin'

export class ApiError extends Error {
  constructor(message, status) {
    super(message)
    this.status = status
  }
}

async function req(path, { method = 'GET', body, query, withTotal = false } = {}) {
  let url = BASE + path
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
export const getRatings = (kind, tmdbId, refresh = false) =>
  req(`/titles/tmdb/${kind}/${tmdbId}/ratings`, { query: { refresh: refresh || '' } })

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

// ── Settings & API keys ─────────────────────────────────────────────
export const getSettings = () => req('/settings')
export const updateSettings = (body) => req('/settings', { method: 'PATCH', body })
export const listApiKeys = () => req('/api-keys')
export const createApiKey = (name) => req('/api-keys', { method: 'POST', body: { name } })
export const renameApiKey = (id, name) => req(`/api-keys/${id}`, { method: 'PATCH', body: { name } })
export const rotateApiKey = (id) => req(`/api-keys/${id}/rotate`, { method: 'POST' })
export const deleteApiKey = (id) => req(`/api-keys/${id}`, { method: 'DELETE' })

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
