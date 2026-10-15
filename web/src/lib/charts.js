// Shared chart series and axis labels.

export const RUN_SERIES = [
  { key: 'succeeded', label: 'Succeeded', color: 'var(--c-ok)' },
  { key: 'partial', label: 'Partial', color: 'var(--c-partial)' },
  { key: 'failed', label: 'Failed', color: 'var(--c-fail)' },
  { key: 'started', label: 'Running', color: 'var(--c-run)' },
]

export const COVERAGE_SERIES = [
  { key: 'mapped', label: 'With TMDB ID', color: 'var(--c1)' },
  { key: 'rt', label: 'With Rotten Tomatoes', color: 'var(--c3)' },
]

/** "Oct 14" for a YYYY-MM-DD date (read as UTC). */
export const shortDay = (d) => new Date(`${d}T00:00:00Z`).toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' })

/** stats.runs.daily → ColumnChart rows. */
export const runRows = (daily) => (daily ?? []).map((d) => ({ ...d, label: d.date, title: `${shortDay(d.date)} (UTC)` }))
