// Lazily loaded pages: each gets its own chunk, so the first load only fetches the
// shell and Home. Once signed in, the rest are fetched in the background
// (prefetchPages), so moving between pages doesn't wait on the network.
const loaders = {
  charts: () => import('../pages/Charts.svelte'),
  title: () => import('../pages/Title.svelte'),
  diary: () => import('../pages/Diary.svelte'),
  watchlist: () => import('../pages/Watchlist.svelte'),
  library: () => import('../pages/Library.svelte'),
  stats: () => import('../pages/Stats.svelte'),
  tv: () => import('../pages/TV.svelte'),
  movies: () => import('../pages/Movies.svelte'),
  you: () => import('../pages/You.svelte'),
  statsDetailed: () => import('../pages/StatsDetailed.svelte'),
  episode: () => import('../pages/Episode.svelte'),
  watch: () => import('../pages/Watch.svelte'),
  settings: () => import('../pages/Settings.svelte'),
  discover: () => import('../pages/Discover.svelte'),
  show: () => import('../pages/Show.svelte'),
  person: () => import('../pages/Person.svelte'),
  import: () => import('../pages/Import.svelte'),
  collection: () => import('../pages/Collection.svelte'),
  disc: () => import('../pages/Disc.svelte'),
}

const loaded = $state({})
const failed = $state({})
const pending = new Set()
const background = new Set()

// A background fetch that fails says nothing (opening the page tries again),
// unless the page was opened while it was still running.
function start(name, quiet = false) {
  if (!quiet) background.delete(name)
  if (loaded[name] || pending.has(name)) return
  pending.add(name)
  if (quiet) background.add(name)
  loaders[name]().then(
    (m) => (loaded[name] = m.default),
    (err) => {
      pending.delete(name)
      if (!background.delete(name)) failed[name] = err
    },
  )
}

/** The page's component once its chunk has loaded (starting the load if needed), else undefined. */
export function pageComponent(name) {
  if (!failed[name]) start(name)
  return loaded[name]
}

/** Why the page's chunk failed to load (e.g. a new version replaced it), if it did. */
export const pageError = (name) => failed[name]

/** Try a failed page again. */
export function retryPage(name) {
  delete failed[name]
  start(name)
}

/** Fetch every page's chunk when the browser is idle. */
export function prefetchPages() {
  const idle = window.requestIdleCallback ?? ((cb) => setTimeout(cb, 1500))
  idle(() => Object.keys(loaders).forEach((name) => start(name, true)))
}
