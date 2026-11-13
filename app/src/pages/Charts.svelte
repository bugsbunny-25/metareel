<!-- Charts: the scope's Top 10 for shows or films, with 14-day trends, scores and origin. -->
<script>
  import Top10Row from '../components/Top10Row.svelte'
  import { loadCharts, offersFor, tmdbKind } from '../lib/charts.js'
  import { api } from '../lib/api.js'
  import { library } from '../lib/library.svelte.js'
  import { route, setQuery } from '../lib/router.svelte.js'
  import { scope, providerName, inCountry } from '../lib/scope.svelte.js'
  import { dayMonth } from '../lib/viz.js'
  import { toast } from '../lib/toast.svelte.js'

  let { onlog } = $props()

  const cat = $derived(route.query.cat === 'movies' ? 'movies' : 'tv')
  let loading = $state(true)
  let error = $state('')
  let data = $state(null)
  let events = $state([])

  $effect(() => {
    const { country, provider } = scope
    const ctrl = new AbortController()
    loading = true
    error = ''
    Promise.all([loadCharts(country, provider, ctrl.signal), api.availabilityChanges(country, ctrl.signal).catch(() => ({ events: [] }))])
      .then(([d, ch]) => { if (!ctrl.signal.aborted) { data = { ...d, country, provider }; events = ch.events ?? [] } })
      .catch((err) => { if (err.name !== 'AbortError') error = err.message })
      .finally(() => { if (!ctrl.signal.aborted) loading = false })
    return () => ctrl.abort()
  })

  const chart = $derived(data ? (cat === 'tv' ? data.tv : data.movies) : null)
  const rows = $derived(chart?.entries.map((e) => {
    const kind = tmdbKind(e.item.kind)
    const state = library.get(kind, e.item.tmdb_id)
    return { ...e, state, offers: state?.status === 'watchlist' ? offersFor(events, kind, e.item.tmdb_id) : [] }
  }) ?? [])

  async function onwatchlist(entry) {
    const kind = tmdbKind(entry.item.kind)
    try {
      const on = await library.toggleWatchlist(kind, entry.item.tmdb_id)
      toast(on ? `Added ${entry.item.name} to your watchlist.` : `Removed ${entry.item.name} from your watchlist.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }
  const log = (entry) => onlog({ kind: tmdbKind(entry.item.kind), id: entry.item.tmdb_id, name: entry.item.name, poster: entry.poster_path, season: entry.item.season_number })
</script>

<div class="page">
  <header class="head">
    <h1>Top 10</h1>
    <div class="seg" role="tablist" aria-label="Chart">
      <button role="tab" aria-selected={cat === 'tv'} onclick={() => setQuery({ cat: 'tv' })}>Shows</button>
      <button role="tab" aria-selected={cat === 'movies'} onclick={() => setQuery({ cat: 'movies' })}>Movies</button>
    </div>
    {#if chart}<span class="scope">{providerName(scope.provider)} · {inCountry(scope.country)} · {dayMonth(chart.date)}</span>{/if}
  </header>

  {#if error}
    <p class="alert" role="alert">Couldn’t load this chart: {error}</p>
  {:else if loading && !data}
    <div class="skeleton" style="height:640px" aria-busy="true" aria-label="Loading"></div>
  {:else if !chart}
    <p class="empty">No {cat === 'tv' ? 'shows' : 'films'} chart for {providerName(scope.provider)} in {inCountry(scope.country)}.</p>
  {:else}
    <div class="t10-wrap" aria-busy={loading}>
      <table class="t10">
        <caption class="sr">{cat === 'tv' ? 'Shows' : 'Films'}: {providerName(scope.provider)} Top 10 in {inCountry(scope.country)} on {chart.date}</caption>
        <thead>
          <tr>
            <th scope="col"><span class="sr">Rank</span></th>
            <th scope="col"><span class="sr">Change</span></th>
            <th scope="col">Title</th>
            <th scope="col" class="hn">14 days</th>
            <th scope="col" class="hn right">Days</th>
            <th scope="col" class="hn right">IMDb</th>
            <th scope="col" class="hn right">RT</th>
            <th scope="col" class="hn">Origin</th>
            <th scope="col"><span class="sr">Your list</span></th>
          </tr>
        </thead>
        <tbody>{#each rows as entry (entry.item.slug)}<Top10Row {entry} onlog={log} {onwatchlist} />{/each}</tbody>
      </table>
    </div>
    <p class="foot">Days counts dates on this chart. The 14-day line puts No. 1 at the top; gaps are days off the chart. Ratings come from IMDb and Rotten Tomatoes.</p>
  {/if}
</div>

<style>
  .page { padding: 20px var(--gutter) 40px; }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: 12px 16px; margin-bottom: 14px; }
  h1 { margin: 0; font: 700 32px/1.1 var(--font-display); letter-spacing: -0.02em; }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { height: 30px; padding: 0 14px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .seg button[aria-selected='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }
  .scope { margin-left: auto; color: var(--text-muted); font-size: 14px; }
  .t10-wrap { container: top10 / inline-size; overflow-x: auto; }
  .t10 { width: 100%; border-collapse: separate; border-spacing: 0; }
  .t10 th { height: 34px; padding: 0 10px; color: var(--text-muted); font: 600 12.5px/1 var(--font-sans); text-align: left; white-space: nowrap; }
  .foot { margin: 14px 4px 0; color: var(--text-muted); font-size: 13px; }
  .empty { color: var(--text-muted); }
  .alert { margin: 0; }
</style>
