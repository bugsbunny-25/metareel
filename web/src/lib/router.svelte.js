// Minimal hash router. Routes look like "#/titles?q=foo&page=2"; the query
// string holds page state (filters, sort, paging, open drawer) so it survives
// reloads, works with back/forward and can be shared as a link.

function parse() {
  const raw = window.location.hash.replace(/^#/, '') || '/'
  const [path, qs = ''] = raw.split('?')
  return { path: path || '/', query: Object.fromEntries(new URLSearchParams(qs)) }
}

export const route = $state(parse())

function sync() {
  const next = parse()
  route.path = next.path
  route.query = next.query
}

window.addEventListener('hashchange', sync)

function build(path, query) {
  const params = new URLSearchParams()
  for (const [k, v] of Object.entries(query ?? {})) {
    if (v !== '' && v !== null && v !== undefined && v !== false) params.set(k, String(v))
  }
  const qs = params.toString()
  return `#${path}${qs ? `?${qs}` : ''}`
}

/** Go to a page (adds a history entry). */
export function navigate(path, query = {}) {
  const hash = build(path, query)
  if (hash === window.location.hash) return
  window.location.hash = hash
}

/**
 * Merge changes into the current page's query. Empty values are removed.
 * replace=true updates the URL without a new history entry (use for typing).
 */
export function setQuery(patch, { replace = false } = {}) {
  const query = { ...route.query, ...patch }
  const hash = build(route.path, query)
  if (hash === window.location.hash) return
  if (replace) {
    history.replaceState(null, '', hash)
    sync()
  } else {
    window.location.hash = hash
  }
}

export function href(path, query = {}) {
  return build(path, query)
}

/** Read a positive integer from the query, with a default. */
export function intParam(value, fallback) {
  const n = Number.parseInt(value, 10)
  return Number.isFinite(n) && n > 0 ? n : fallback
}
