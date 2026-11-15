// Watch time as months (30 days), days and hours, for the You page and the detailed stats.

export function splitDuration(minutes) {
  const hours = Math.floor((minutes ?? 0) / 60)
  const days = Math.floor(hours / 24)
  return { months: Math.floor(days / 30), days: days % 30, hours: hours % 24 }
}

/** "3d 4h", or "45m" under an hour. */
export function shortDuration(minutes) {
  if (minutes > 0 && minutes < 60) return `${minutes}m`
  const { months, days, hours } = splitDuration(minutes)
  return [months ? `${months}mo` : '', days ? `${days}d` : '', hours || (!months && !days) ? `${hours}h` : ''].filter(Boolean).join(' ')
}

export const hoursText = (minutes) => `${Math.round((minutes ?? 0) / 60).toLocaleString()} h`

export const plural = (n, one, many = `${one}s`) => `${(n ?? 0).toLocaleString()} ${n === 1 ? one : many}`
