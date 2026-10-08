// Formatting and parsing helpers shared by the pages.

const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })

/** "3 hours ago" / "in 20 minutes". */
export function relativeTime(value) {
  if (!value) return '—'
  const d = toDate(value)
  if (!d) return '—'
  const diff = (d.getTime() - Date.now()) / 1000
  const abs = Math.abs(diff)
  const units = [
    ['year', 31536000], ['month', 2592000], ['week', 604800],
    ['day', 86400], ['hour', 3600], ['minute', 60],
  ]
  for (const [unit, secs] of units) {
    if (abs >= secs) return rtf.format(Math.round(diff / secs), unit)
  }
  return abs < 30 ? 'just now' : rtf.format(Math.round(diff), 'second')
}

/** Local date + time, e.g. "Oct 8, 2026, 4:05 PM". */
export function dateTime(value) {
  const d = toDate(value)
  return d ? d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' }) : '—'
}

/** Same instant in UTC, for tooltips. */
export function dateTimeUTC(value) {
  const d = toDate(value)
  return d ? `${d.toISOString().slice(0, 16).replace('T', ' ')} UTC` : ''
}

/** "Oct 8, 2026" for YYYY-MM-DD dates (no timezone shift). */
export function dateOnly(value) {
  if (!value) return '—'
  const [y, m, d] = value.slice(0, 10).split('-').map(Number)
  if (!y) return value
  return new Date(y, m - 1, d).toLocaleDateString(undefined, { dateStyle: 'medium' })
}

/** "1m 05s" between two timestamps (end defaults to now). */
export function duration(start, end) {
  const s = toDate(start)
  if (!s) return '—'
  const e = end ? toDate(end) : new Date()
  let secs = Math.max(0, Math.round((e - s) / 1000))
  const h = Math.floor(secs / 3600); secs -= h * 3600
  const m = Math.floor(secs / 60); secs -= m * 60
  if (h) return `${h}h ${String(m).padStart(2, '0')}m`
  if (m) return `${m}m ${String(secs).padStart(2, '0')}s`
  return `${secs}s`
}

/** "16:00" UTC → local "12:00 PM" for display next to UTC run times. */
export function utcTimeToLocal(hhmm) {
  const [h, m] = hhmm.split(':').map(Number)
  const d = new Date()
  d.setUTCHours(h, m, 0, 0)
  return d.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
}

export function todayUTC() {
  return new Date().toISOString().slice(0, 10)
}

export function addDays(yyyyMmDd, days) {
  const d = new Date(`${yyyyMmDd}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() + days)
  return d.toISOString().slice(0, 10)
}

export function number(n) {
  return n === null || n === undefined ? '—' : Number(n).toLocaleString()
}

function toDate(value) {
  if (!value) return null
  if (value instanceof Date) return value
  // SQLite "YYYY-MM-DD HH:MM:SS" timestamps are UTC.
  const iso = /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}/.test(value) ? `${value.replace(' ', 'T')}Z` : value
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? null : d
}

// ── External IDs: accept pasted URLs as well as bare IDs ────────────

/** "https://www.imdb.com/title/tt123/" or "tt123" → "tt123". */
export function parseImdbId(input) {
  const m = String(input ?? '').trim().match(/tt\d{5,}/)
  return m ? m[0] : String(input ?? '').trim()
}

/** "https://www.themoviedb.org/tv/1234-name" → { id: "1234", kind: "tv_show" }. */
export function parseTmdbInput(input) {
  const raw = String(input ?? '').trim()
  const m = raw.match(/themoviedb\.org\/(movie|tv)\/(\d+)/)
  if (m) return { id: m[2], kind: m[1] === 'tv' ? 'tv_show' : 'movie' }
  return { id: raw.replace(/\D/g, ''), kind: null }
}

/** RT URL or slug → "m/<vanity>" / "tv/<vanity>". */
export function parseRtSlug(input) {
  const raw = String(input ?? '').trim()
  if (!raw) return ''
  const path = raw.replace(/^https?:\/\/[^/]+/, '').replace(/^\/+|\/+$/g, '')
  const parts = path.split('/')
  const i = parts.findIndex((p) => p === 'm' || p === 'tv')
  return i >= 0 && parts[i + 1] ? `${parts[i]}/${parts[i + 1]}` : path
}

export const links = {
  tmdb: (kind, id) => (id ? `https://www.themoviedb.org/${kind === 'tv_show' || kind === 'tv' ? 'tv' : 'movie'}/${id}` : null),
  imdb: (id) => (id ? `https://www.imdb.com/title/${id}/` : null),
  rt: (slug) => (slug ? `https://www.rottentomatoes.com/${slug}` : null),
  flixpatrol: (slug) => (slug ? `https://flixpatrol.com/title/${slug}/` : null),
}

/** Download rows as a CSV file. columns: [{ key, label, value?(row) }]. */
export function downloadCSV(filename, columns, rows) {
  const esc = (v) => {
    const s = v === null || v === undefined ? '' : String(v)
    return /[",\n]/.test(s) ? `"${s.replaceAll('"', '""')}"` : s
  }
  const lines = [columns.map((c) => esc(c.label)).join(',')]
  for (const row of rows) lines.push(columns.map((c) => esc(c.value ? c.value(row) : row[c.key])).join(','))
  const blob = new Blob([lines.join('\n')], { type: 'text/csv' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = filename
  a.click()
  URL.revokeObjectURL(a.href)
}

/** A run error without the HTML body preview and repeated "scrape <url>:" prefixes. */
export function shortError(msg, max = 180) {
  if (!msg) return ''
  let s = msg.split(' body_preview=')[0]
  s = s.replace(/^(scrape https?:\/\/\S+: )+/, '')
  if (/Just a moment|status=403|Forbidden/.test(msg)) s += ' (blocked by Cloudflare)'
  return s.length > max ? `${s.slice(0, max)}…` : s
}
