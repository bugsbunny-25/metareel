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
