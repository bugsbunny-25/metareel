<!-- Leaderboards (#/discover?tab=leaderboard&period=day|week|month&where=service|country|world
     &cat=movies|tv_shows): titles ranked by Top 10 points (No. 1 = 10 … No. 10 = 1 per chart per
     day) from /leaderboards, for the service in the top bar, every service in the country, or
     every chart metareel stores. -->
<script>
  import Poster from '../../components/Poster.svelte'
  import Icon from '../../components/Icon.svelte'
  import BarList from '../../components/BarList.svelte'
  import { api } from '../../lib/api.js'
  import { library } from '../../lib/library.svelte.js'
  import { route, setQuery } from '../../lib/router.svelte.js'
  import { inCountry, providerName } from '../../lib/scope.svelte.js'
  import { dayLabel } from '../../lib/dates.js'
  import { tmdbKind } from '../../lib/rows.js'
  import { regionName } from '../../lib/services.js'
  import { toast } from '../../lib/toast.svelte.js'

  let { country, provider } = $props()

  const PERIODS = [['day', 'Today'], ['week', 'This week'], ['month', 'This month']]
  const period = $derived(PERIODS.some(([p]) => p === route.query.period) ? route.query.period : 'week')
  const where = $derived(['country', 'world'].includes(route.query.where) ? route.query.where : 'service')
  const cat = $derived(['movies', 'tv_shows'].includes(route.query.cat) ? route.query.cat : '')

  let data = $state(null)
  let error = $state('')
  $effect(() => {
    const params = { period, category: cat || undefined, limit: 50,
      country: where === 'world' ? undefined : country, provider: where === 'service' ? provider : undefined }
    const ctrl = new AbortController()
    data = null
    error = ''
    api.leaderboard(params, ctrl.signal)
      .then((d) => (data = d))
      .catch((err) => { if (err.name !== 'AbortError') error = err.message })
    return () => ctrl.abort()
  })

  let shown = $state(20)
  $effect(() => { period; where; cat; country; provider; shown = 20 })
  const items = $derived(data?.items ?? [])
  const top = $derived(items[0]?.points || 1)
  const total = $derived(items.reduce((n, it) => n + it.points, 0))
  const films = $derived(items.filter((it) => it.kind === 'movie'))
  const filmShare = $derived(total ? Math.round((films.reduce((n, it) => n + it.points, 0) / total) * 100) : 0)
  const mine = $derived(items.filter((it) => it.tmdb_id && library.get(tmdbKind(it.kind), it.tmdb_id)).length)
  const longest = $derived(items.length ? items.reduce((a, b) => (b.days_on_charts > a.days_on_charts ? b : a)) : null)
  const spread = $derived.by(() => {
    const m = new Map()
    const key = where === 'world' ? 'countries' : 'providers'
    for (const it of items) for (const k of it[key] ?? []) m.set(k, (m.get(k) ?? 0) + 1)
    return [...m].sort((a, b) => b[1] - a[1]).slice(0, 8)
      .map(([k, value]) => ({ label: where === 'world' ? regionName(k) : providerName(k), value }))
  })
  const scopeLabel = $derived(where === 'service' ? `${providerName(provider)} in ${inCountry(country)}` : where === 'country' ? `every service in ${inCountry(country)}` : 'every chart metareel tracks')

  async function toggle(it) {
    const k = tmdbKind(it.kind)
    try {
      const on = await library.toggleWatchlist(k, it.tmdb_id)
      toast(on ? `Added ${it.name} to your watchlist.` : `Removed ${it.name} from your watchlist.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }
  const pick = (patch) => setQuery(patch, { replace: true })
</script>

<div class="d-filters">
  <div class="d-seg" role="group" aria-label="Period">
    {#each PERIODS as [p, l]}<button aria-pressed={period === p} onclick={() => pick({ period: p === 'week' ? '' : p })}>{l}</button>{/each}
  </div>
  <div class="d-seg" role="group" aria-label="Charts">
    <button aria-pressed={where === 'service'} onclick={() => pick({ where: '' })}>{providerName(provider)}</button>
    <button aria-pressed={where === 'country'} onclick={() => pick({ where: 'country' })}>All of {country}</button>
    <button aria-pressed={where === 'world'} onclick={() => pick({ where: 'world' })}>Every country</button>
  </div>
  <div class="d-seg" role="group" aria-label="Category">
    {#each [['', 'All'], ['movies', 'Films'], ['tv_shows', 'Shows']] as [v, l]}<button aria-pressed={cat === v} onclick={() => pick({ cat: v })}>{l}</button>{/each}
  </div>
</div>

{#if error}
  <p class="alert" role="alert">Couldn’t load the leaderboard: {error}</p>
{:else if !data}
  <div class="skeleton" style="height:520px" aria-busy="true" aria-label="Loading"></div>
{:else if !items.length}
  <section class="d-empty">
    <h2>No chart points for {scopeLabel}</h2>
    <p>metareel has no stored Top 10 for this period here. Try a longer period or another service.</p>
  </section>
{:else}
  <section class="d-kpis" aria-label="The leaderboard at a glance">
    <div class="d-kpi"><span class="k-label">Leader</span><b class="small" title={items[0].name}>{items[0].name}</b><span class="k-sub">{items[0].points} points · best No. {items[0].best_rank}</span></div>
    <div class="d-kpi"><span class="k-label">Films’ share</span><b class="num">{filmShare}%</b><span class="k-sub">of the top {items.length}’s points; shows {100 - filmShare}%</span></div>
    <div class="d-kpi"><span class="k-label">Longest run</span><b class="small" title={longest?.name}>{longest?.name ?? '–'}</b><span class="k-sub">{longest ? `${longest.days_on_charts} ${longest.days_on_charts === 1 ? 'day' : 'days'} in a Top 10` : ''}</span></div>
    <div class="d-kpi yours"><span class="k-label">In your library</span><b class="num">{mine}</b><span class="k-sub">of these {items.length}</span></div>
  </section>

  <div class="d-layout">
    <div class="d-main">
      <p class="d-note">{scopeLabel} · {data.from === data.to ? dayLabel(data.from) : `${dayLabel(data.from)} – ${dayLabel(data.to)}`} · a No. 1 day scores 10 points, a No. 10 day 1</p>
      <ol class="board">
        {#each items.slice(0, shown) as it (it.title_id)}
          {@const k = tmdbKind(it.kind)}
          {@const t = it.tmdb_id ? library.get(k, it.tmdb_id) : null}
          {@const href = it.tmdb_id ? `#/title/${k}/${it.tmdb_id}` : null}
          <li>
            <span class="pos num" class:one={it.position === 1}>{it.position}</span>
            <svelte:element this={href ? 'a' : 'span'} class="pic" {href} tabindex="-1" aria-hidden="true"><Poster path={it.poster_path ?? null} seed={it.tmdb_id ?? it.slug} title={it.name} width={42} radius={5} size="w92" /></svelte:element>
            <div class="txt">
              {#if href}<a class="name" {href}>{it.name}</a>{:else}<span class="name">{it.name}</span>{/if}
              <span class="sub">{k === 'tv' ? 'Series' : 'Film'} · best No. {it.best_rank} · {it.days_on_charts} {it.days_on_charts === 1 ? 'day' : 'days'}{#if where === 'world'}{' · '}{it.countries.length} {it.countries.length === 1 ? 'country' : 'countries'}{:else if where === 'country'}{' · '}{it.providers.map(providerName).join(', ')}{/if}{#if t}{' · '}<span class="you">{t.status === 'watchlist' ? 'On your watchlist' : t.watch_count > 0 ? 'Seen' : 'In your library'}</span>{/if}</span>
              <span class="bar" aria-hidden="true"><i style:width="{(it.points / top) * 100}%"></i></span>
            </div>
            <b class="pts num">{it.points}<span class="hint"> pts</span></b>
            {#if it.tmdb_id}
              <button class="mark" aria-pressed={t?.status === 'watchlist'} aria-label="{t?.status === 'watchlist' ? 'Remove from' : 'Add to'} watchlist: {it.name}" onclick={() => toggle(it)}>
                <Icon name={t?.status === 'watchlist' ? 'bookmark-fill' : 'bookmark'} size={15} />
              </button>
            {:else}<span></span>{/if}
          </li>
        {/each}
      </ol>
      {#if items.length > shown}
        <button class="btn-secondary d-more" onclick={() => (shown += 20)}>Show more <span class="hint">{items.length - shown} left</span></button>
      {/if}
    </div>
    <aside class="d-side">
      {#if where !== 'service' && spread.length}
        <section class="panel pad">
          <h2>{where === 'world' ? 'Where they charted' : 'Which services'}</h2>
          <p class="hint">Titles of the top {items.length} per {where === 'world' ? 'country' : 'service'}.</p>
          <BarList items={spread} />
        </section>
      {/if}
      <section class="panel pad">
        <h2>Films and shows</h2>
        <BarList items={[{ label: 'Films', value: films.length }, { label: 'Shows', value: items.length - films.length }]} />
      </section>
      <p class="d-source">FlixPatrol Top 10 charts stored by metareel.</p>
    </aside>
  </div>
{/if}

<style>
  .board { display: grid; margin: 0; padding: 0; list-style: none; }
  .board li { display: grid; grid-template-columns: 34px auto minmax(0, 1fr) auto auto; align-items: center; gap: 12px; padding: 9px 4px; box-shadow: inset 0 -1px 0 var(--border-subtle); }
  .pos { font: 700 20px/1 var(--font-display); text-align: right; color: var(--text-muted); }
  .pos.one { color: var(--delta-up); }
  .pic { display: block; }
  .txt { display: grid; gap: 4px; min-width: 0; }
  .name { font: 700 16px/1.25 var(--font-display); color: inherit; text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  a.name:hover { text-decoration: underline; text-underline-offset: 3px; }
  .sub { color: var(--text-muted); font-size: 12.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .you { color: var(--accent-yours); font-weight: 600; }
  .bar { height: 5px; border-radius: 3px; background: var(--bg-elevated); overflow: hidden; }
  .bar i { display: block; height: 100%; background: var(--accent-yours); border-radius: 3px; }
  .pts { font-size: 17px; white-space: nowrap; }
  .pts .hint { font-weight: 400; }
  .mark { display: grid; place-items: center; width: 30px; height: 30px; border: 0; border-radius: 50%; background: var(--bg-elevated); color: var(--text-muted); cursor: pointer; }
  .mark:hover { color: var(--text-primary); }
  .mark[aria-pressed='true'] { background: color-mix(in srgb, var(--accent-yours) 20%, transparent); color: var(--accent-yours); }
  @media (max-width: 560px) {
    .board li { grid-template-columns: 26px auto minmax(0, 1fr) auto; }
    .board li > :last-child { display: none; }
  }
</style>
