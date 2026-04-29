const BASE = '/api/v1'

async function req(path, options) {
  const res = await fetch(BASE + path, options)
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error ?? `HTTP ${res.status}`)
  }
  return res.json()
}

export function listTitles({ limit = 50, offset = 0, kind = '', q = '' } = {}) {
  const p = new URLSearchParams({ limit, offset })
  if (kind) p.set('kind', kind)
  if (q)    p.set('q', q)
  return req(`/titles?${p}`)
}

export function patchTitle(id, body) {
  return req(`/titles/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export function getTop10({ country, provider, category, date }) {
  const seg = category === 'tv_shows' ? 'tv-shows' : 'movies'
  const path = provider
    ? `/top10/${seg}/${country}/${provider}`
    : `/top10/${seg}/${country}`
  return req(`${path}?date=${date}`).catch(err => {
    if (err.message.includes('404') || err.message === 'HTTP 404') return { items: [] }
    throw err
  })
}

export function listSchedules() {
  return req('/admin/task-schedules', { cache: 'no-store' })
}

export function getSchedule(id) {
  return req(`/admin/task-schedules/${id}`, { cache: 'no-store' })
}

export function createSchedule(body) {
  return req('/admin/task-schedules', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export function updateSchedule(id, body) {
  return req(`/admin/task-schedules/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export function runScheduleNow(id) {
  return req(`/admin/task-schedules/${id}/run-now`, { method: 'POST' })
}

export const PROVIDERS = [
  { value: '',              label: 'All providers' },
  { value: 'netflix',       label: 'Netflix' },
  { value: 'hbo-max',       label: 'Max (HBO)' },
  { value: 'disney',        label: 'Disney+' },
  { value: 'amazon-prime',  label: 'Amazon Prime' },
  { value: 'paramount-plus',label: 'Paramount+' },
  { value: 'peacock',       label: 'Peacock' },
  { value: 'apple-tv',      label: 'Apple TV+' },
]

export const COUNTRIES = [
  { code: 'AR', name: 'Argentina' },
  { code: 'AU', name: 'Australia' },
  { code: 'AT', name: 'Austria' },
  { code: 'BE', name: 'Belgium' },
  { code: 'BR', name: 'Brazil' },
  { code: 'CA', name: 'Canada' },
  { code: 'CL', name: 'Chile' },
  { code: 'CO', name: 'Colombia' },
  { code: 'HR', name: 'Croatia' },
  { code: 'CZ', name: 'Czech Republic' },
  { code: 'DK', name: 'Denmark' },
  { code: 'EG', name: 'Egypt' },
  { code: 'FI', name: 'Finland' },
  { code: 'FR', name: 'France' },
  { code: 'DE', name: 'Germany' },
  { code: 'GR', name: 'Greece' },
  { code: 'HK', name: 'Hong Kong' },
  { code: 'HU', name: 'Hungary' },
  { code: 'IN', name: 'India' },
  { code: 'ID', name: 'Indonesia' },
  { code: 'IE', name: 'Ireland' },
  { code: 'IL', name: 'Israel' },
  { code: 'IT', name: 'Italy' },
  { code: 'JP', name: 'Japan' },
  { code: 'MX', name: 'Mexico' },
  { code: 'NL', name: 'Netherlands' },
  { code: 'NZ', name: 'New Zealand' },
  { code: 'NG', name: 'Nigeria' },
  { code: 'NO', name: 'Norway' },
  { code: 'PK', name: 'Pakistan' },
  { code: 'PH', name: 'Philippines' },
  { code: 'PL', name: 'Poland' },
  { code: 'PT', name: 'Portugal' },
  { code: 'RO', name: 'Romania' },
  { code: 'SA', name: 'Saudi Arabia' },
  { code: 'SG', name: 'Singapore' },
  { code: 'ZA', name: 'South Africa' },
  { code: 'KR', name: 'South Korea' },
  { code: 'ES', name: 'Spain' },
  { code: 'SE', name: 'Sweden' },
  { code: 'CH', name: 'Switzerland' },
  { code: 'TW', name: 'Taiwan' },
  { code: 'TH', name: 'Thailand' },
  { code: 'TR', name: 'Turkey' },
  { code: 'UA', name: 'Ukraine' },
  { code: 'AE', name: 'UAE' },
  { code: 'GB', name: 'United Kingdom' },
  { code: 'US', name: 'United States' },
  { code: 'VN', name: 'Vietnam' },
]
