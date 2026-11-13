// Loads a scope's two Top 10 charts with 14-day rank histories and ratings, in the
// entry shape the chart row reads: { item, category, poster_path, ratings, offers, hist }.
// Library state is joined at render time so watchlist changes show without a reload.
import { api, ApiError } from './api.js'

const DAY = 864e5
const iso = (t) => new Date(t).toISOString().slice(0, 10)

/** 404 means "no chart for this category here", which is an empty chart, not an error. */
async function orEmpty(p) {
  try {
    return await p
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) return null
    throw err
  }
}

function historyBySlug(h) {
  const dates = (h?.days ?? []).map((d) => d.date)
  const bySlug = {}
  ;(h?.days ?? []).forEach((d, i) => {
    for (const it of d.items) (bySlug[it.slug] ??= Array(dates.length).fill(null))[i] = it.rank
  })
  return { dates, bySlug }
}

export const tmdbKind = (k) => (k === 'tv_show' || k === 'tv' ? 'tv' : 'movie')

export async function loadCharts(country, provider, signal) {
  const [tv, movies] = await Promise.all([
    orEmpty(api.top10('tv_shows', country, provider, signal)),
    orEmpty(api.top10('movies', country, provider, signal)),
  ])
  const date = [tv?.date, movies?.date].filter(Boolean).sort().at(-1) ?? null
  if (!date) return { date: null, tv: null, movies: null }

  const from = iso(Date.parse(date) - 13 * DAY)
  const ids = [...(tv?.items ?? []), ...(movies?.items ?? [])].filter((i) => i.tmdb_id).map((i) => `${tmdbKind(i.kind)}:${i.tmdb_id}`)
  const [htv, hmv, ratings] = await Promise.all([
    tv ? orEmpty(api.top10History('tv_shows', country, provider, from, date, signal)) : null,
    movies ? orEmpty(api.top10History('movies', country, provider, from, date, signal)) : null,
    // A ratings outage shouldn't blank the chart; rows show "–" instead.
    ids.length ? api.ratingsBatch([...new Set(ids)].slice(0, 50), signal).catch(() => ({ items: [] })) : { items: [] },
  ])
  const byId = Object.fromEntries((ratings.items ?? []).map((r) => [`${r.kind}:${r.tmdb_id}`, r]))

  const build = (chart, hist, category) => {
    if (!chart) return null
    const h = historyBySlug(hist)
    return {
      date: chart.date,
      histDates: h.dates,
      entries: chart.items.map((item) => ({
        item,
        category,
        poster_path: item.poster_path ?? null,
        ratings: item.tmdb_id ? byId[`${tmdbKind(item.kind)}:${item.tmdb_id}`] ?? null : null,
        offers: [],
        hist: h.bySlug[item.slug] ?? null,
      })),
    }
  }
  return { date, tv: build(tv, htv, 'tv_shows'), movies: build(movies, hmv, 'movies') }
}

/** OfferEvents for one title in a changes feed, newest first, subscription offers only. */
export function offersFor(events, kind, id) {
  return events.filter((e) => e.tmdb_kind === kind && e.tmdb_id === id && e.monetization === 'flatrate')
}
