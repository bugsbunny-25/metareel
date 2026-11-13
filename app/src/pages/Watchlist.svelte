<!-- Watchlist. Your titles in the order you added them, joined with what metareel knows about
     each in your country: charting in the last 30 days, streaming on a subscription now, or
     recently gone from a service (observed, never forecast). Rows come from the library, so a
     change anywhere in the app shows here at once. Filters live in the URL:
     ?show=streaming|charting|left&kind=movie|tv&sort=added|title|release&days=30|90|365. Order is the add
     order; there's no manual reordering yet. -->
<script>
  import Poster from '../components/Poster.svelte'
  import Icon from '../components/Icon.svelte'
  import { api } from '../lib/api.js'
  import { library, libKey } from '../lib/library.svelte.js'
  import { route, setQuery } from '../lib/router.svelte.js'
  import { scope, providerName, inCountry, countryName } from '../lib/scope.svelte.js'
  import { dayLabel } from '../lib/dates.js'
  import { toast } from '../lib/toast.svelte.js'

  let { onlog } = $props()

  const show = $derived(['streaming', 'charting', 'left'].includes(route.query.show) ? route.query.show : 'all')
  const kind = $derived(route.query.kind === 'movie' || route.query.kind === 'tv' ? route.query.kind : '')
  const sort = $derived(['title', 'release'].includes(route.query.sort) ? route.query.sort : 'added')
  const WINDOWS = [[30, '30 days'], [90, '90 days'], [365, 'year']]
  const days = $derived(WINDOWS.some(([d]) => String(d) === route.query.days) ? Number(route.query.days) : 30)
  const windowLabel = $derived(WINDOWS.find(([d]) => d === days)[1])

  let insights = $state(null)
  let insightsError = $state('')
  $effect(() => {
    const country = scope.country
    const win = days
    const ctrl = new AbortController()
    insightsError = ''
    api.watchlistInsights(country, win, ctrl.signal)
      .then((r) => (insights = r))
      .catch((err) => { if (err.name !== 'AbortError') { insights = null; insightsError = err.message } })
    return () => ctrl.abort()
  })

  // One record per title from the three insight lists.
  const joined = $derived.by(() => {
    const out = new Map()
    const at = (k) => out.get(k) ?? (out.set(k, { charting: null, streaming: [], left: null }), out.get(k))
    for (const c of insights?.charting ?? []) {
      const r = at(libKey(c.tmdb_kind, c.tmdb_id))
      if (!r.charting || c.best_rank < r.charting.best_rank || (c.best_rank === r.charting.best_rank && c.ranked_on > r.charting.ranked_on)) r.charting = c
    }
    for (const s of insights?.streaming_now ?? []) {
      const r = at(libKey(s.tmdb_kind, s.tmdb_id))
      if (!r.streaming.some((x) => x.provider_name === s.provider_name)) r.streaming.push(s)
    }
    // Plan tiers ("Netflix Standard with Ads", "… Amazon Channel") are the same service to a
    // viewer: keep the base name when it's there.
    for (const r of out.values()) {
      r.streaming = r.streaming.filter((s) => !r.streaming.some((o) => o !== s && s.provider_name.startsWith(o.provider_name + ' ')))
    }
    for (const l of insights?.recently_left ?? []) {
      const r = at(libKey(l.tmdb_kind, l.tmdb_id))
      if (!r.left || l.observed_at > r.left.observed_at) r.left = l
    }
    return out
  })

  const all = $derived(
    [...library.byKey.values()].filter((t) => t.status === 'watchlist').map((t) => ({ ...t, info: joined.get(libKey(t.tmdb_kind, t.tmdb_id)) ?? null }))
  )
  const counts = $derived({
    all: all.length,
    streaming: all.filter((t) => t.info?.streaming.length).length,
    charting: all.filter((t) => t.info?.charting).length,
    left: all.filter((t) => t.info?.left && !t.info?.streaming.length).length,
  })
  const rows = $derived(
    all
      .filter((t) => !kind || t.tmdb_kind === kind)
      .filter((t) => show === 'all' || (show === 'streaming' ? t.info?.streaming.length : show === 'charting' ? t.info?.charting : t.info?.left && !t.info?.streaming.length))
      .sort((a, b) =>
        sort === 'title' ? (a.title ?? '').localeCompare(b.title ?? '')
        : sort === 'release' ? (b.release_date ?? '').localeCompare(a.release_date ?? '')
        : (a.watchlist_position ?? 1e9) - (b.watchlist_position ?? 1e9))
  )

  const year = (d) => (d ? d.slice(0, 4) : '')
  const length = (t) => (t.tmdb_kind === 'movie' && t.runtime ? `${Math.floor(t.runtime / 60)} h ${t.runtime % 60} min` : '')

  async function remove(t) {
    try {
      await library.toggleWatchlist(t.tmdb_kind, t.tmdb_id)
      toast(`Removed ${t.title} from your watchlist.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }
  const log = (t) => onlog({ kind: t.tmdb_kind, id: t.tmdb_id, name: t.title, poster: t.poster_path, season: 1 })
</script>

<div class="page">
  <header class="head">
    <h1>Watchlist</h1>
    <span class="count num">{all.length} title{all.length === 1 ? '' : 's'}</span>
  </header>

  <section class="kpis" aria-label="Your watchlist {inCountry(scope.country)}">
    <button class="kpi" aria-pressed={show === 'all'} onclick={() => setQuery({ show: '' })}>
      <span class="k-label">On your list</span><b class="num">{counts.all}</b><span class="k-sub">In the order you added them</span>
    </button>
    <button class="kpi" aria-pressed={show === 'streaming'} onclick={() => setQuery({ show: 'streaming' })} disabled={!insights}>
      <span class="k-label">Streaming now</span><b class="num up">{insights ? counts.streaming : '–'}</b><span class="k-sub">On a subscription in {countryName(scope.country)}</span>
    </button>
    <button class="kpi" aria-pressed={show === 'charting'} onclick={() => setQuery({ show: 'charting' })} disabled={!insights}>
      <span class="k-label">Charted lately</span><b class="num yours">{insights ? counts.charting : '–'}</b><span class="k-sub">In a Top 10 in the last {windowLabel}</span>
    </button>
    <button class="kpi" aria-pressed={show === 'left'} onclick={() => setQuery({ show: 'left' })} disabled={!insights}>
      <span class="k-label">Left a service</span><b class="num" class:bad={counts.left > 0}>{insights ? counts.left : '–'}</b><span class="k-sub">And not streaming elsewhere</span>
    </button>
  </section>
  {#if insightsError}<p class="note">Chart and streaming details didn’t load ({insightsError}); your list is still here.</p>{/if}

  <div class="bar">
    <div class="seg" role="group" aria-label="Kind">
      <button aria-pressed={kind === ''} onclick={() => setQuery({ kind: '' })}>Everything</button>
      <button aria-pressed={kind === 'movie'} onclick={() => setQuery({ kind: 'movie' })}>Films</button>
      <button aria-pressed={kind === 'tv'} onclick={() => setQuery({ kind: 'tv' })}>Shows</button>
    </div>
    <label class="sort">Window
      <select value={days} onchange={(e) => setQuery({ days: e.currentTarget.value === '30' ? '' : e.currentTarget.value })}>
        {#each WINDOWS as [d, l]}<option value={d}>Last {l}</option>{/each}
      </select>
    </label>
    <label class="sort">Sort
      <select value={sort} onchange={(e) => setQuery({ sort: e.currentTarget.value === 'added' ? '' : e.currentTarget.value })}>
        <option value="added">Order added</option>
        <option value="title">Title</option>
        <option value="release">Newest release</option>
      </select>
    </label>
  </div>

  {#if !library.loaded}
    <div class="skeleton" style="height:320px" aria-busy="true" aria-label="Loading"></div>
  {:else if !all.length}
    <section class="empty">
      <h2>Nothing on your watchlist</h2>
      <p>Use the bookmark on any chart row or title page. Titles you add show here with where they’re streaming and whether they’re charting.</p>
      <a class="btn-primary" href="#/charts">Browse the charts</a>
    </section>
  {:else if !rows.length}
    <p class="empty-note">No titles on your list match. <button class="link" onclick={() => setQuery({ show: '', kind: '' })}>Show everything</button></p>
  {:else}
    <div class="table-wrap">
      <table class="list">
        <caption class="sr">Your watchlist, {rows.length} titles</caption>
        <thead>
          <tr><th scope="col">Title</th><th scope="col">Charts · last {windowLabel}</th><th scope="col">Where to watch · {scope.country}</th><th scope="col"><span class="sr">Actions</span></th></tr>
        </thead>
        <tbody>
          {#each rows as t (t.tmdb_kind + t.tmdb_id)}
            <tr>
              <td class="title">
                <a class="cell" href="#/title/{t.tmdb_kind}/{t.tmdb_id}">
                  <Poster path={t.poster_path} seed={t.tmdb_id} title={t.title ?? ''} width={44} radius={5} size="w185" />
                  <span class="text">
                    <span class="name">{t.title}</span>
                    <span class="meta">{t.tmdb_kind === 'tv' ? 'Series' : 'Film'}{#if year(t.release_date)}{' · '}{year(t.release_date)}{/if}{#if length(t)}{' · '}{length(t)}{/if}{#if t.watch_count}{' · '}watched {t.watch_count}×{/if}</span>
                    {#if t.watchlist_note}<span class="wnote">{t.watchlist_note}</span>{/if}
                  </span>
                </a>
              </td>
              <td class="charts" class:none={!t.info?.charting}>
                {#if t.info?.charting}
                  <span class="rank num">No.&nbsp;{t.info.charting.best_rank}</span>
                  <span class="sub">{providerName(t.info.charting.provider)} · {t.info.charting.category === 'tv_shows' ? 'shows' : 'films'} · {dayLabel(t.info.charting.ranked_on.slice(0, 10))}</span>
                {:else}
                  <span class="muted">–</span>
                {/if}
              </td>
              <td class="where">
                {#if t.info?.streaming.length}
                  <span class="providers">
                    {#each t.info.streaming.slice(0, 3) as s}
                      {#if s.link}<a class="pill strong" href={s.link} target="_blank" rel="noopener noreferrer">{s.provider_name}</a>{:else}<span class="pill strong">{s.provider_name}</span>{/if}
                    {/each}
                    {#if t.info.streaming.length > 3}<span class="pill">+{t.info.streaming.length - 3}</span>{/if}
                  </span>
                {:else if t.info?.left}
                  <span class="left">Left {t.info.left.provider_name} · {dayLabel(t.info.left.observed_at.slice(0, 10))}</span>
                {:else}
                  <span class="muted">{insights ? 'Not on a subscription here' : '–'}</span>
                {/if}
              </td>
              <td class="me">
                <span class="actions">
                  <button class="round" title="Log a watch" aria-label="Log a watch of {t.title}" onclick={() => log(t)}><Icon name="plus" size={15} /></button>
                  <button class="round on" title="Remove from watchlist" aria-label="Remove {t.title} from watchlist" onclick={() => remove(t)}><Icon name="bookmark-fill" size={15} /></button>
                </span>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <p class="foot">Streaming and departures are what metareel has observed in {countryName(scope.country)}; services don’t publish when a title will leave. Watch provider data by JustWatch, via TMDB.</p>
  {/if}
</div>

<style>
  .page { padding: 20px var(--gutter) 48px; }
  .head { display: flex; align-items: baseline; gap: 14px; margin-bottom: 16px; }
  h1 { margin: 0; font: 700 32px/1.1 var(--font-display); letter-spacing: -0.02em; }
  .count { color: var(--text-muted); font-size: 15px; }

  .kpis { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; }
  .kpi { display: grid; gap: 4px; padding: 16px 18px; border: 0; border-radius: 16px; background: var(--bg-surface); color: var(--text-primary); text-align: left; cursor: pointer; font: inherit; box-shadow: inset 0 0 0 1px transparent; }
  .kpi:hover:not(:disabled) { background: var(--bg-elevated); }
  .kpi[aria-pressed='true'] { box-shadow: inset 0 0 0 2px var(--text-primary); }
  .kpi:disabled { cursor: default; }
  .k-label { color: var(--text-muted); font-size: 13px; font-weight: 600; }
  .kpi b { font: 700 32px/1.05 var(--font-sans); letter-spacing: -0.03em; }
  .kpi b.up { color: var(--delta-up); }
  .kpi b.yours { color: var(--accent-yours); }
  .k-sub { color: var(--text-muted); font-size: 12.5px; }
  .note { margin: 10px 0 0; color: var(--text-muted); }

  .bar { display: flex; flex-wrap: wrap; align-items: center; gap: 12px 16px; margin: 24px 0 10px; }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { height: 30px; padding: 0 14px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .seg button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }
  .sort:first-of-type { margin-left: auto; }
  .sort { display: flex; align-items: center; gap: 8px; color: var(--text-muted); font-size: 13.5px; font-weight: 600; }
  .sort select { height: 32px; padding: 0 12px; border: 0; border-radius: 16px; background: var(--bg-elevated); color: var(--text-primary); font: 600 13.5px/1 var(--font-sans); }

  .table-wrap { overflow-x: auto; }
  .list { width: 100%; border-collapse: separate; border-spacing: 0; }
  .list th { height: 34px; padding: 0 12px; color: var(--text-muted); font: 600 12.5px/1 var(--font-sans); text-align: left; white-space: nowrap; }
  .list td { height: 76px; padding: 8px 12px; vertical-align: middle; }
  .list tbody tr:not(:first-child) td { box-shadow: inset 0 1px 0 var(--border-subtle); }
  .list tbody tr:hover td { background: var(--bg-elevated); }
  .list tbody tr:hover td:first-child { border-radius: 12px 0 0 12px; }
  .list tbody tr:hover td:last-child { border-radius: 0 12px 12px 0; }
  .title { width: 44%; }
  .cell { display: flex; align-items: center; gap: 14px; min-width: 0; color: inherit; text-decoration: none; }
  .text { display: grid; gap: 2px; min-width: 0; }
  .name { font: 600 17px/1.2 var(--font-display); }
  .cell:hover .name { text-decoration: underline; text-underline-offset: 3px; }
  .meta { color: var(--text-muted); font-size: 13px; }
  .wnote { color: var(--text-muted); font: italic 400 14px/20px var(--font-display); }
  .charts { white-space: nowrap; }
  .rank { display: block; color: var(--accent-yours); font: 700 18px/1.1 var(--font-display); }
  .sub { color: var(--text-muted); font-size: 12.5px; }
  .providers { display: flex; flex-wrap: wrap; gap: 6px; }
  .pill.strong { color: var(--text-primary); text-decoration: none; }
  a.pill.strong:hover { background: var(--bg-surface); text-decoration: underline; }
  .left { color: var(--delta-down); font-weight: 600; white-space: nowrap; }
  .me { width: 1%; }
  .actions { display: flex; gap: 6px; }
  .round { display: grid; place-items: center; width: 34px; height: 34px; border: 0; border-radius: 50%; background: var(--bg-elevated); color: var(--text-primary); cursor: pointer; }
  tr:hover .round { background: var(--bg-surface); }
  .round.on { background: var(--accent-yours); color: var(--bg-canvas); }
  .foot { margin: 14px 4px 0; color: var(--text-muted); font-size: 13px; }
  .empty { padding: 32px 0; display: grid; gap: 10px; justify-items: start; }
  .empty h2 { margin: 0; font: 700 24px/1.2 var(--font-display); }
  .empty p { margin: 0; color: var(--text-muted); max-width: 60ch; }
  .empty-note { color: var(--text-muted); }
  .link { border: 0; background: none; color: var(--text-primary); text-decoration: underline; cursor: pointer; font: inherit; }

  @media (max-width: 900px) { .kpis { grid-template-columns: 1fr 1fr; } }
  @media (max-width: 640px) {
    .list thead { display: none; }
    .list, .list tbody, .list tr, .list td { display: block; width: 100%; }
    .list tr { display: grid; grid-template-columns: 1fr auto; padding: 10px 0; box-shadow: inset 0 -1px 0 var(--border-subtle); }
    .list td { height: auto; padding: 4px 0; box-shadow: none !important; }
    .list .title { grid-column: 1 / -1; }
    .list .charts, .list .where { grid-column: 1; padding-left: 58px; }
    .list .me { grid-row: 2; grid-column: 2; }
    .list .charts.none { display: none; }
    .sort { margin-left: 0; }
  }
</style>
