// Which titles are in a Top 10 right now in your country, for the Top 10 badge on posters.
// One request per country (/top10/{country}: every service's latest movie and TV charts),
// shared by every poster on every page and refreshed at most every 30 minutes.
import { SvelteMap } from 'svelte/reactivity'
import { api } from './api.js'
import { scope, providerName, countryName } from './scope.svelte.js'

const TTL = 30 * 60 * 1000
const byCountry = new SvelteMap() // country -> Map("kind:id" -> best entry)
const loading = new Map() // country -> { at, promise }

function load(country) {
  const cur = loading.get(country)
  if (cur && Date.now() - cur.at < TTL) return
  const promise = api.top10Country(country).then((r) => {
    const best = new Map()
    for (const [category, list] of [['movies', r.movies ?? []], ['tv_shows', r.tv_shows ?? []]]) {
      for (const e of list) {
        if (!e.tmdb_id) continue
        const key = `${e.kind === 'tv_show' || e.kind === 'tv' ? 'tv' : 'movie'}:${e.tmdb_id}`
        const prev = best.get(key)
        if (!prev || e.rank < prev.rank) best.set(key, { rank: e.rank, provider: e.provider, category, date: e.date })
      }
    }
    byCountry.set(country, best)
  }).catch(() => loading.delete(country)) // try again next time a poster asks
  loading.set(country, { at: Date.now(), promise })
}

/** The title's best place in a Top 10 in your country today, or null. kind: movie | tv. */
export function top10Of(kind, id) {
  if (!kind || !id) return null
  const country = scope.country
  load(country)
  return byCountry.get(country)?.get(`${kind === 'tv_show' ? 'tv' : kind}:${id}`) ?? null
}

/** "No. 2 in the US on Netflix today" */
export function top10Text(t) {
  if (!t) return ''
  return `No. ${t.rank} in ${countryName(scope.country)} on ${providerName(t.provider)}${t.category === 'tv_shows' ? ' (shows)' : ' (films)'}`
}
