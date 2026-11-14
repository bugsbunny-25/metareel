<!-- Popular elsewhere: titles in other countries' Top 10s that have no subscription, free or
     ads offer here, ranked by the chart points they earned abroad, with where they chart and
     any rent or buy offers here. -->
<script>
  import Poster from '../../components/Poster.svelte'
  import Icon from '../../components/Icon.svelte'
  import { api } from '../../lib/api.js'
  import { library } from '../../lib/library.svelte.js'
  import { route, setQuery } from '../../lib/router.svelte.js'
  import { inCountry, providerName } from '../../lib/scope.svelte.js'
  import { toast } from '../../lib/toast.svelte.js'
  import { servicesOf, regionName } from '../../lib/services.js'

  let { country, families } = $props()

  const WINDOWS = [[7, 'Last week'], [14, 'Two weeks'], [30, 'A month']]
  const days = $derived(WINDOWS.some(([d]) => d === +route.query.window) ? +route.query.window : 7)
  const kind = $derived(['movie', 'tv'].includes(route.query.kind) ? route.query.kind : '')

  let data = $state(null)
  let error = $state('')
  $effect(() => {
    const params = { days, kind: kind || undefined, limit: 100 }
    const cc = country
    const ctrl = new AbortController()
    data = null
    error = ''
    api.elsewhere(cc, params, ctrl.signal)
      .then((d) => (data = d))
      .catch((err) => { if (err.name !== 'AbortError') error = err.message })
    return () => ctrl.abort()
  })

  const items = $derived(data?.items ?? [])
  const max = $derived(Math.max(1, ...items.map((it) => it.points)))
  const rentable = $derived(items.filter((it) => it.rent_buy_here?.length).length)
  const byCountry = $derived.by(() => {
    const m = new Map()
    for (const it of items) for (const c of it.countries) m.set(c.country, (m.get(c.country) ?? 0) + 1)
    return [...m].sort((a, b) => b[1] - a[1])
  })
  const here = (it) => servicesOf(it.rent_buy_here, families)
  const modes = (it) => [...new Set((it.rent_buy_here ?? []).map((o) => (o.monetization === 'buy' ? 'buy' : 'rent')))]

  async function toggle(it) {
    const was = library.get(it.kind, it.tmdb_id)?.status === 'watchlist'
    try {
      await library.toggleWatchlist(it.kind, it.tmdb_id)
      toast(was ? `Removed ${it.title} from your watchlist.` : `Added ${it.title} to your watchlist.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }
  const pick = (patch) => setQuery(patch, { replace: true })
</script>

<div class="filters">
  <div class="seg" role="group" aria-label="Charting over">
    {#each WINDOWS as [d, l]}<button aria-pressed={days === d} onclick={() => pick({ window: d === 7 ? '' : d })}>{l}</button>{/each}
  </div>
  <div class="seg" role="group" aria-label="Kind">
    {#each [['', 'All'], ['movie', 'Films'], ['tv', 'Shows']] as [v, l]}<button aria-pressed={kind === v} onclick={() => pick({ kind: v })}>{l}</button>{/each}
  </div>
</div>

{#if error}
  <p class="alert" role="alert">Couldn’t load what’s popular elsewhere: {error}</p>
{:else if !data}
  <div class="skeleton" style="height:420px" aria-busy="true" aria-label="Loading"></div>
{:else}
  <section class="kpis" aria-label="Popular elsewhere at a glance">
    <div class="kpi"><span class="k-label">Not streaming here</span><b class="num">{data.total}</b><span class="k-sub">charting abroad, no subscription in {inCountry(country)}</span></div>
    <div class="kpi"><span class="k-label">Rent or buy here</span><b class="num">{rentable}</b><span class="k-sub">of these {items.length}</span></div>
    <div class="kpi"><span class="k-label">Countries compared</span><b class="num">{data.countries_checked.length}</b><span class="k-sub">{data.countries_checked.map(regionName).join(', ') || 'none with charts'}</span></div>
    <div class="kpi"><span class="k-label">Left out</span><b class="num">{(data.skipped?.available_here ?? 0) + (data.skipped?.charting_here ?? 0)}</b><span class="k-sub">{data.skipped?.available_here ?? 0} stream here · {data.skipped?.charting_here ?? 0} chart here</span></div>
  </section>

  {#if !items.length}
    <section class="empty">
      <h2>Nothing popular elsewhere that you can’t watch in {inCountry(country)}</h2>
      <p>{data.countries_checked.length ? 'Everything charting in the other countries is streaming here too. Try a longer window.' : 'metareel has no other countries’ charts for this window yet.'}</p>
    </section>
  {:else}
    {#if byCountry.length > 1}
      <p class="hint">Charting in {byCountry.map(([c, n]) => `${regionName(c)} (${n})`).join(' · ')}</p>
    {/if}
    <ol class="rows">
      {#each items as it, i (it.kind + it.tmdb_id)}
        {@const t = library.get(it.kind, it.tmdb_id)}
        <li>
          <span class="rk num">{i + 1}</span>
          <a class="art" href="#/title/{it.kind}/{it.tmdb_id}" tabindex="-1" aria-hidden="true"><Poster path={it.poster_path ?? null} seed={it.tmdb_id} title={it.title} width={52} radius={6} size="w185" /></a>
          <div class="body">
            <a class="name" href="#/title/{it.kind}/{it.tmdb_id}">{it.title}</a>
            <span class="hint">{it.year ?? ''}{it.year ? ' · ' : ''}{it.kind === 'tv' ? 'Series' : 'Film'}{#if it.imdb_rating}{' · '}IMDb {it.imdb_rating.toFixed(1)}{/if}</span>
            <div class="where">
              {#each it.countries as c (c.country)}
                <span class="cc" title="{c.points} chart points in {regionName(c.country)}">
                  <b class="num">#{c.best_rank}</b> {regionName(c.country)}{#if c.providers?.length}<span class="hint">{' · '}{c.providers.map(providerName).join(', ')}</span>{/if}
                </span>
              {/each}
            </div>
          </div>
          <div class="pts" title="{it.points} chart points abroad">
            <span class="bar" aria-hidden="true"><i style:width="{(it.points / max) * 100}%"></i></span>
            <span class="num">{it.points}</span>
          </div>
          <div class="here">
            {#if here(it).length}
              <span class="lbl">{modes(it).includes('rent') ? (modes(it).includes('buy') ? 'Rent or buy' : 'Rent') : 'Buy'} here</span>
              <span class="hint">{here(it).slice(0, 3).join(', ')}{here(it).length > 3 ? ` +${here(it).length - 3}` : ''}</span>
            {:else}
              <span class="hint">Not available in {inCountry(country)}</span>
            {/if}
          </div>
          <button class="mark" aria-pressed={t?.status === 'watchlist'} aria-label="{t?.status === 'watchlist' ? 'Remove from' : 'Add to'} watchlist: {it.title}" title={t?.status === 'watchlist' ? 'On your watchlist' : 'Add to watchlist'} onclick={() => toggle(it)}>
            <Icon name={t?.status === 'watchlist' ? 'bookmark-fill' : 'bookmark'} size={15} />
          </button>
        </li>
      {/each}
    </ol>
  {/if}
{/if}

<style>
  .filters { display: flex; flex-wrap: wrap; gap: 10px 14px; }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { height: 30px; padding: 0 12px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .seg button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }
  .kpis { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
  .kpi { display: grid; gap: 4px; padding: 16px 18px; border-radius: 16px; background: var(--bg-surface); min-width: 0; }
  .k-label { color: var(--text-muted); font-size: 13px; font-weight: 600; }
  .kpi b { font: 700 30px/1.05 var(--font-sans); letter-spacing: -0.03em; }
  .k-sub { color: var(--text-muted); font-size: 12.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .hint { color: var(--text-muted); font-size: 13px; }

  .rows { display: grid; gap: 2px; margin: 0; padding: 6px; list-style: none; border-radius: 16px; background: var(--bg-surface); }
  .rows li { display: grid; grid-template-columns: 26px 52px minmax(0, 1fr) 150px 190px 30px; align-items: center; gap: 14px; padding: 10px 8px; border-radius: 10px; }
  .rows li:hover { background: var(--bg-elevated); }
  .rk { color: var(--text-muted); font: 700 18px/1 var(--font-display); text-align: right; }
  .body { display: grid; gap: 3px; min-width: 0; }
  .name { font: 700 16px/1.25 var(--font-display); color: inherit; text-decoration: none; }
  .name:hover { text-decoration: underline; text-underline-offset: 3px; }
  .where { display: flex; flex-wrap: wrap; gap: 4px 12px; margin-top: 2px; font-size: 13px; }
  .cc b { color: var(--accent-yours); }
  .pts { display: flex; align-items: center; gap: 8px; font-size: 13px; font-weight: 700; }
  .bar { flex: 1; height: 6px; border-radius: 3px; background: var(--bg-elevated); overflow: hidden; }
  .bar i { display: block; height: 100%; border-radius: 3px; background: var(--text-muted); }
  .here { display: grid; gap: 2px; min-width: 0; }
  .here .lbl { font-size: 13.5px; font-weight: 600; }
  .here .hint { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .mark { display: grid; place-items: center; width: 30px; height: 30px; border: 0; border-radius: 50%; background: var(--bg-elevated); color: var(--text-muted); cursor: pointer; }
  .mark[aria-pressed='true'] { background: color-mix(in srgb, var(--accent-yours) 20%, transparent); color: var(--accent-yours); }
  .empty { padding: 30px 0; }
  .empty h2 { margin: 0 0 6px; font: 700 22px/1.2 var(--font-display); }
  .empty p { color: var(--text-muted); max-width: 60ch; }

  @media (max-width: 1000px) {
    .rows li { grid-template-columns: 22px 52px minmax(0, 1fr) 30px; }
    .pts { grid-column: 3; }
    .here { grid-column: 3; }
  }
  @media (max-width: 760px) {
    .kpis { grid-template-columns: 1fr 1fr; }
    .rows li { grid-template-columns: 44px minmax(0, 1fr) 30px; gap: 10px; }
    .rk { display: none; }
    .pts, .here { grid-column: 2; }
  }
</style>
