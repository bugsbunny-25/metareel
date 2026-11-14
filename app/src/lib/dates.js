// Local-day helpers. Tracking stores instants in UTC ("YYYY-MM-DD HH:MM:SS") and files each
// watch under a local day (watched_on) computed by the server; the UI shows local time.

const pad = (n) => String(n).padStart(2, '0')

/** A Date as the browser's local YYYY-MM-DD. */
export const localDay = (d = new Date()) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`

/** A Date as a <input type="datetime-local"> value. */
export const localInput = (d = new Date()) => `${localDay(d)}T${pad(d.getHours())}:${pad(d.getMinutes())}`

/** Server UTC timestamp ("YYYY-MM-DD HH:MM:SS" or RFC 3339) → Date. */
export function fromUTC(ts) {
  if (!ts) return null
  const s = ts.includes('T') ? ts : ts.replace(' ', 'T') + 'Z'
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? null : d
}

/** Shift a YYYY-MM-DD by n days (calendar arithmetic, no time zone involved). */
export function addDays(day, n) {
  const d = new Date(day + 'T12:00:00Z')
  d.setUTCDate(d.getUTCDate() + n)
  return d.toISOString().slice(0, 10)
}

const fmt = (opts) => new Intl.DateTimeFormat(undefined, { ...opts, timeZone: 'UTC' })
const monthYear = fmt({ month: 'long', year: 'numeric' })
const weekday = fmt({ weekday: 'short' })
const shortDay = fmt({ day: 'numeric', month: 'short' })

/** "November 2026" for a YYYY-MM-DD. */
export const monthLabel = (day) => monthYear.format(new Date(day + 'T12:00:00Z'))
/** "Fri" for a YYYY-MM-DD. */
export const weekdayLabel = (day) => weekday.format(new Date(day + 'T12:00:00Z'))
/** "13 Nov" for a YYYY-MM-DD. */
export const dayLabel = (day) => shortDay.format(new Date(day + 'T12:00:00Z'))
/** Local clock time of a UTC timestamp, e.g. "21:40". */
export const clock = (ts) => {
  const d = fromUTC(ts)
  return d ? d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' }) : ''
}

const rel = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })
/** "3 days ago", "last week", "in 2 months" for a UTC timestamp or a YYYY-MM-DD. */
export function ago(ts, now = new Date()) {
  const d = /^\d{4}-\d{2}-\d{2}$/.test(ts ?? '') ? new Date(ts + 'T12:00:00Z') : fromUTC(ts)
  if (!d) return ''
  const days = (d - now) / 86400000
  const abs = Math.abs(days)
  if (abs < 1) return Math.abs((d - now) / 3600000) < 1 ? 'just now' : rel.format(Math.round((d - now) / 3600000), 'hour')
  if (abs < 14) return rel.format(Math.round(days), 'day')
  if (abs < 60) return rel.format(Math.round(days / 7), 'week')
  if (abs < 730) return rel.format(Math.round(days / 30.44), 'month')
  return rel.format(Math.round(days / 365.25), 'year')
}

/** Whole days from today (local) to a YYYY-MM-DD: 0 today, 1 tomorrow, -1 yesterday. */
export const daysUntil = (day, today = localDay()) => Math.round((new Date(day + 'T12:00:00Z') - new Date(today + 'T12:00:00Z')) / 86400000)

/** Minutes as "9 h 40 m", "45 m" or "3 h". */
export function hm(min) {
  const m = Math.round(min ?? 0)
  if (m < 60) return `${m} m`
  return m % 60 ? `${Math.floor(m / 60)} h ${m % 60} m` : `${m / 60} h`
}

/**
 * How long ago, by local day: "today", "yesterday", "5 days ago", then weeks and months.
 * For watches, whose time may only be known to the day (stored as local midnight).
 */
export function sinceDay(ts, now = new Date()) {
  const d = fromUTC(ts)
  if (!d) return ''
  const n = daysUntil(localDay(d), localDay(now))
  return n > -14 ? rel.format(n, 'day') : ago(localDay(d), now)
}

/** The browser's IANA time zone. */
export const browserZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone

/** A Date as "YYYY-MM-DDTHH:MM" wall time in an IANA zone (for datetime-local inputs). */
export function zonedInput(date, zone) {
  if (!zone) return localInput(date)
  const p = Object.fromEntries(new Intl.DateTimeFormat('en-CA', {
    timeZone: zone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
  }).formatToParts(date).map((x) => [x.type, x.value]))
  return `${p.year}-${p.month}-${p.day}T${p.hour}:${p.minute}`
}

/**
 * Wall time "YYYY-MM-DDTHH:MM" in an IANA zone → the instant (Date). Two passes settle the
 * offset across a DST change; a skipped spring-forward time lands an hour later.
 */
export function zonedToDate(wall, zone) {
  if (!zone) return new Date(wall)
  const [d, t = '00:00'] = wall.split('T')
  const [y, mo, da] = d.split('-').map(Number)
  const [h, mi] = t.split(':').map(Number)
  const asUTC = Date.UTC(y, mo - 1, da, h, mi)
  let guess = asUTC
  for (let i = 0; i < 2; i++) {
    const shown = zonedInput(new Date(guess), zone)
    const [sd, st] = shown.split('T')
    const [sy, smo, sda] = sd.split('-').map(Number)
    const [sh, smi] = st.split(':').map(Number)
    guess += asUTC - Date.UTC(sy, smo - 1, sda, sh, smi)
  }
  return new Date(guess)
}
