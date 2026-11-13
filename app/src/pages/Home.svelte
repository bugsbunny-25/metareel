<!-- Home. A breakout hero tinted from its poster, KPI tiles with 14-day trends, the Top 10 rail,
     movement and your-list tables, and breakouts by country. Everything is for the chosen scope. -->
<script>
  import Poster from '../components/Poster.svelte'
  import Icon from '../components/Icon.svelte'
  import { api } from '../lib/api.js'
  import { loadCharts, offersFor, tmdbKind } from '../lib/charts.js'
  import { library, libKey } from '../lib/library.svelte.js'
  import { scope, providerName, countryName, inCountry } from '../lib/scope.svelte.js'
  import { sampleColor, seriesPath, debutsPerDay, dayMonth } from '../lib/viz.js'
  import { shortDate } from '../lib/rows.js'
  import { toast } from '../lib/toast.svelte.js'

  let { onlog } = $props()

  let loading = $state(true)
  let error = $state('')
  let data = $state(null)
  let tab = $state('tv')
  let tint = $state(null)

  $effect(() => {
    const { country, provider } = scope
    const ctrl = new AbortController()
    loading = true
    error = ''
    load(country, provider, ctrl.signal)
      .then((d) => { if (!ctrl.signal.aborted) data = d })
      .catch((err) => { if (err.name !== 'AbortError') error = err.message })
      .finally(() => { if (!ctrl.signal.aborted) loading = false })
    return () => ctrl.abort()
  })

  async function load(country, provider, signal) {
    // Movement and breakouts are asked for the chart date on screen, so a scrape that is a
    // day behind still shows matching data instead of a 404 for "today".
    const [charts, changes] = await Promise.all([
      loadCharts(country, provider, signal),
      api.availabilityChanges(country, signal).catch(() => ({ events: [] })),
    ])
    const [movers, breakouts] = await Promise.all([
      charts.date ? api.movers(country, provider, charts.date, signal).catch(() => null) : null,
      charts.date ? api.breakouts(charts.date, signal).catch(() => ({ items: [] })) : { items: [] },
    ])
    const items = breakouts?.items ?? []
    // Feature the strongest breakout charting here; fall back to today's No. 1 show.
    const b = items.find((x) => x.countries?.some((c) => c.country === country)) ?? null
    const top = [...(charts.tv?.entries ?? []), ...(charts.movies?.entries ?? [])].find((e) => e.item.tmdb_id) ?? null
    const pick = b ? { kind: tmdbKind(b.kind), id: b.tmdb_id, name: b.name, breakout: b } : top?.item.tmdb_id ? { kind: tmdbKind(top.item.kind), id: top.item.tmdb_id, name: top.item.name, entry: top } : null
    let hero = null
    if (pick?.id) {
      const ov = await api.overview(pick.kind, pick.id, signal).catch(() => null)
      const m = ov?.metadata ?? {}
      hero = { ...pick, poster: m.poster_path ?? null, genres: m.genres ?? [], year: (m.release_date ?? '').slice(0, 4), seasons: m.number_of_seasons ?? 0 }
    }
    return { country, provider, charts, movers, breakouts: items, events: changes?.events ?? [], hero }
  }

  // Library state joins in here, so watchlist changes show immediately.
  const withState = (chart) => chart?.entries.map((e) => {
    const kind = tmdbKind(e.item.kind)
    const state = library.get(kind, e.item.tmdb_id)
    const offers = state?.status === 'watchlist' ? offersFor(data.events, kind, e.item.tmdb_id) : []
    return { ...e, state, offers }
  }) ?? []
  const tvRows = $derived(data ? withState(data.charts.tv) : [])
  const movieRows = $derived(data ? withState(data.charts.movies) : [])
  const rail = $derived(tab === 'tv' ? tvRows : movieRows)

  $effect(() => {
    tint = null
    if (data?.hero?.poster) sampleColor(`https://image.tmdb.org/t/p/w92${data.hero.poster}`).then((c) => (tint = c))
  })

  // KPI: new entries per day, both charts, from the 14-day histories.
  const debuts = $derived.by(() => {
    if (!data?.charts.date) return []
    const n = Math.max(data.charts.tv?.histDates.length ?? 0, data.charts.movies?.histDates.length ?? 0)
    if (n < 2) return []
    const tv = debutsPerDay(tvRows.map((e) => e.hist), n)
    const mv = debutsPerDay(movieRows.map((e) => e.hist), n)
    return tv.map((v, i) => v + (mv[i] ?? 0))
  })
  const spark = $derived(debuts.length > 1 ? seriesPath(debuts, 132, 36, 3) : null)
  const today = $derived(debuts.at(-1) ?? 0)
  const avg = $derived(debuts.length > 1 ? (debuts.slice(0, -1).reduce((a, b) => a + b, 0) / (debuts.length - 1)).toFixed(1) : '–')

  // Your list: watchlist and watching titles, joined with today's charts and the changes feed.
  const myList = $derived.by(() => {
    if (!data) return []
    const pos = new Map()
    for (const e of [...tvRows, ...movieRows]) if (e.item.tmdb_id) pos.set(libKey(tmdbKind(e.item.kind), e.item.tmdb_id), e)
    const ORDER = { left: 0, charting: 1, arrived: 2, quiet: 3 }
    return library.active.map((t) => {
      const k = libKey(t.tmdb_kind, t.tmdb_id)
      const offers = offersFor(data.events, t.tmdb_kind, t.tmdb_id)
      const left = offers.find((o) => o.event === 'removed')
      const arrived = offers.find((o) => o.event === 'added')
      const e = pos.get(k)
      const note = left ? { type: 'left', text: `Left ${left.provider_name} ${shortDate(left.observed_at)}` }
        : e ? { type: 'charting', text: `No. ${e.item.rank} today · ${e.category === 'tv_shows' ? 'shows' : 'films'}` }
        : arrived ? { type: 'arrived', text: `New on ${arrived.provider_name} ${shortDate(arrived.observed_at)}` }
        : { type: 'quiet', text: 'Not in today’s Top 10' }
      return { ...t, note }
    }).sort((a, b) => ORDER[a.note.type] - ORDER[b.note.type]).slice(0, 8)
  })
  const charting = $derived(myList.filter((m) => m.note.type === 'charting').length)
  const leftCount = $derived(myList.filter((m) => m.note.type === 'left').length)

  const ORDER = { up: 0, debut: 1, down: 2 }
  const size = (m) => (m.previous_rank == null ? 0 : Math.abs(m.previous_rank - m.rank))
  const moves = $derived.by(() => {
    const rows = []
    for (const c of data?.movers?.charts ?? []) {
      for (const m of c.climbers ?? []) rows.push({ ...m, type: 'up' })
      for (const m of c.debuts ?? []) rows.push({ ...m, type: 'debut' })
      for (const m of c.fallers ?? []) rows.push({ ...m, type: 'down' })
    }
    return rows.sort((a, b) => ORDER[a.type] - ORDER[b.type] || size(b) - size(a) || a.rank - b.rank).slice(0, 8)
  })
  const prevDate = $derived(data?.movers?.charts?.[0]?.previous_date)
  const localBreakouts = $derived((data?.breakouts ?? []).filter((b) => b.countries?.some((c) => c.country === data.country)))
  const heroBreak = $derived(data?.hero?.breakout ?? null)
  const heroEntry = $derived(data?.hero ? [...tvRows, ...movieRows].find((e) => e.item.tmdb_id === data.hero.id) : null)
  const heroState = $derived(data?.hero ? library.get(data.hero.kind, data.hero.id) : null)
  const titleHref = (kind, id) => (id ? `#/title/${tmdbKind(kind)}/${id}` : undefined)

  async function toggleHero() {
    const h = data.hero
    try {
      const on = await library.toggleWatchlist(h.kind, h.id)
      toast(on ? `Added ${h.name} to your watchlist.` : `Removed ${h.name} from your watchlist.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }
</script>

{#if error}
  <p class="alert" role="alert">Couldn’t load today’s charts: {error}</p>
{:else if loading && !data}
  <div class="loading" aria-busy="true" aria-label="Loading">
    <div class="skeleton" style="height:210px"></div>
    <div class="skeleton" style="height:96px"></div>
    <div class="skeleton" style="height:180px"></div>
  </div>
{:else if data && !data.charts.date}
  <section class="empty">
    <h1>No chart yet</h1>
    <p>metareel has no {providerName(data.provider)} Top 10 for {inCountry(data.country)}. Pick another service or country at the top.</p>
  </section>
{:else if data}
  <div class="home" aria-busy={loading}>
    {#if data.hero}
      <section class="hero" style:--tint={tint ? `rgb(${tint.join(' ')} / 0.42)` : 'transparent'}>
        <a class="hero-art" href={titleHref(data.hero.kind, data.hero.id)} aria-hidden="true" tabindex="-1"><Poster path={data.hero.poster} seed={data.hero.id} title={data.hero.name} width={128} radius={10} /></a>
        <div class="hero-copy">
          <p class="eyebrow">{heroBreak ? 'Breaking out' : `No. ${heroEntry?.item.rank ?? 1} in ${inCountry(data.country)} today`}</p>
          <h1><a href={titleHref(data.hero.kind, data.hero.id)}>{data.hero.name}</a></h1>
          <p class="hero-meta">
            {data.hero.kind === 'tv' ? (data.hero.seasons ? `Series · ${data.hero.seasons} season${data.hero.seasons === 1 ? '' : 's'}` : 'Series') : 'Film'}{#if data.hero.genres.length}{' · '}{data.hero.genres.slice(0, 2).join(', ')}{/if}{#if data.hero.year}{' · '}{data.hero.year}{/if}
          </p>
          <div class="hero-actions">
            <button class="btn-primary" type="button" aria-pressed={heroState?.status === 'watchlist'} onclick={toggleHero}>
              <Icon name={heroState?.status === 'watchlist' ? 'check' : 'plus'} size={17} />{heroState?.status === 'watchlist' ? 'On your Watchlist' : 'Add to Watchlist'}
            </button>
            <button class="btn-secondary" type="button" onclick={() => onlog({ kind: data.hero.kind, id: data.hero.id, name: data.hero.name, poster: data.hero.poster, season: heroEntry?.item.season_number })}>
              <Icon name="plus" size={17} />Log a watch
            </button>
          </div>
        </div>
        <dl class="hero-stats">
          {#if heroBreak}
            {#each heroBreak.countries.slice(0, 2) as c}
              <div><dt>{countryName(c.country)}</dt><dd class="num">#{c.rank}</dd><span>{c.movement === 'entered' ? 'entered in last 3 days' : `up ${c.gain} in 3 days`}</span></div>
            {/each}
            <div><dt>Breakout score</dt><dd class="num">{heroBreak.score}</dd><span>{heroBreak.rising_countries} countries</span></div>
          {/if}
          {#if heroEntry}<div><dt>In this Top 10</dt><dd class="num">{heroEntry.item.days_in_top10}</dd><span>{heroEntry.item.days_in_top10 === 1 ? 'day' : 'days'}</span></div>{/if}
        </dl>
      </section>
    {/if}

    <section class="kpis" aria-label="Today at a glance">
      <div class="kpi">
        <span class="k-label">New entries today</span>
        <span class="k-row"><b class="num">{today}</b>
          {#if spark}
            <svg width="132" height="36" viewBox="0 0 132 36" role="img" aria-label="New entries per day over {debuts.length} days">
              <path d={spark.area} fill="var(--text-primary)" opacity="0.08" />
              <path d={spark.line} fill="none" stroke="var(--text-primary)" stroke-width="1.5" stroke-linejoin="round" />
              <circle cx={spark.x(debuts.length - 1)} cy={spark.y(today)} r="3" fill="var(--delta-debut)" />
            </svg>
          {/if}
        </span>
        <span class="k-sub">Shows and films · {avg} a day on average</span>
      </div>
      <div class="kpi">
        <span class="k-label">Breaking out here</span>
        <span class="k-row"><b class="num">{localBreakouts.length}</b></span>
        <span class="k-sub">Rising in 2+ countries, including this one</span>
      </div>
      <div class="kpi">
        <span class="k-label">Your list on a chart</span>
        <span class="k-row"><b class="num">{charting}<span class="of">{' '}of {library.active.length}</span></b></span>
        <span class="k-sub">Watchlist and watching</span>
      </div>
      <div class="kpi">
        <span class="k-label">Left a service</span>
        <span class="k-row"><b class="num" class:bad={leftCount > 0}>{leftCount}</b></span>
        <span class="k-sub">From your list, recently</span>
      </div>
    </section>

    <section class="block">
      <div class="block-head">
        <h2>Top 10 in {inCountry(data.country)} today</h2>
        <div class="seg" role="tablist" aria-label="Chart">
          <button role="tab" aria-selected={tab === 'tv'} onclick={() => (tab = 'tv')} disabled={!tvRows.length}>Shows</button>
          <button role="tab" aria-selected={tab === 'movies'} onclick={() => (tab = 'movies')} disabled={!movieRows.length}>Movies</button>
        </div>
        <span class="hint">{providerName(data.provider)} · {dayMonth(data.charts.date)}</span>
        <a class="more" href="#/charts?cat={tab}">Full chart and trends</a>
      </div>
      <ol class="top-rail">
        {#each rail as e (e.item.slug)}
          <li>
            <span class="big num" aria-hidden="true">{e.item.rank}</span>
            <a class="tile" href={titleHref(e.item.kind, e.item.tmdb_id)}>
              <Poster path={e.poster_path} seed={e.item.tmdb_id ?? e.item.slug} title={e.item.name} width={92} radius={8} size="w185" />
              {#if e.state?.status === 'watchlist'}<span class="mark" aria-hidden="true"><Icon name="bookmark-fill" size={12} /></span>
              {:else if e.state?.watch_count > 0}<span class="mark" aria-hidden="true"><Icon name="check" size={12} /></span>{/if}
              <span class="sr">No. {e.item.rank}: {e.item.name}{e.state?.status === 'watchlist' ? ', on your watchlist' : e.state?.watch_count > 0 ? ', watched' : ''}</span>
            </a>
          </li>
        {/each}
      </ol>
    </section>

    <div class="two">
      <section class="panel">
        <div class="panel-h"><h2>Movement</h2><span class="hint">Shows and films{prevDate ? ` · vs ${dayMonth(prevDate)}` : ''}</span></div>
        {#if moves.length}
          <table class="tbl">
            <thead><tr><th scope="col">Title</th><th scope="col" class="right">Now</th><th scope="col" class="right">Was</th><th scope="col" class="right">Change</th></tr></thead>
            <tbody>
              {#each moves as m}
                <tr>
                  <td><a class="cell" href={titleHref(m.kind, m.tmdb_id)}><Poster path={m.poster_path} seed={m.tmdb_id ?? m.slug} title={m.name} width={26} radius={4} size="w92" /><span class="ell">{m.name}</span></a></td>
                  <td class="right num">{m.rank}</td>
                  <td class="right num muted">{m.previous_rank ?? '–'}</td>
                  <td class="right"><span class="chg {m.type}">{m.type === 'debut' ? 'New' : `${m.type === 'up' ? 'Up' : 'Down'} ${size(m)}`}</span></td>
                </tr>
              {/each}
            </tbody>
          </table>
        {:else}
          <p class="empty-note">No movement to show: the chart hasn’t changed since the last scrape.</p>
        {/if}
      </section>

      <section class="panel">
        <div class="panel-h"><h2>Your list today</h2><span class="hint">Joined with today’s charts and availability</span></div>
        {#if myList.length}
          <table class="tbl">
            <thead><tr><th scope="col">Title</th><th scope="col">Status</th><th scope="col">Today</th></tr></thead>
            <tbody>
              {#each myList as t}
                <tr>
                  <td><a class="cell" href={titleHref(t.tmdb_kind, t.tmdb_id)}><Poster path={t.poster_path} seed={t.tmdb_id} title={t.title ?? ''} width={26} radius={4} size="w92" /><span class="ell">{t.title}</span></a></td>
                  <td><span class="pill">{t.status === 'watchlist' ? 'Watchlist' : 'Watching'}</span></td>
                  <td class:bad={t.note.type === 'left'} class:muted={t.note.type === 'quiet'}>{t.note.text}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        {:else}
          <p class="empty-note">Nothing on your list yet. Use the bookmark on any title to add it; it will show here when it charts or changes service.</p>
        {/if}
      </section>
    </div>

    {#if data.breakouts.length}
      <section class="panel wide">
        <div class="panel-h"><h2>Rising in several countries</h2><span class="hint">Entered or climbed 3+ places in 2+ countries over 3 days</span></div>
        <table class="tbl">
          <thead><tr><th scope="col">Title</th><th scope="col">Service</th><th scope="col" class="right">Countries</th><th scope="col" class="right">Score</th><th scope="col">Where</th></tr></thead>
          <tbody>
            {#each data.breakouts.slice(0, 8) as b}
              <tr>
                <td><a class="cell" href={titleHref(b.kind, b.tmdb_id)}><Poster path={b.poster_path} seed={b.tmdb_id ?? b.slug} title={b.name} width={26} radius={4} size="w92" /><span class="ell">{b.name}</span></a></td>
                <td class="muted">{providerName(b.countries[0]?.provider ?? '')}</td>
                <td class="right num">{b.rising_countries}</td>
                <td class="right num">{b.score}</td>
                <td><span class="codes">{#each b.countries as c}<span class="code"><b>{c.country}</b> <span class="num">#{c.rank}</span>{#if c.movement === 'entered'}{' '}<span class="new">New</span>{/if}</span>{/each}</span></td>
              </tr>
            {/each}
          </tbody>
        </table>
      </section>
    {/if}
  </div>
{/if}

<style>
  .home { padding-bottom: 40px; }
  .loading { display: grid; gap: 16px; padding: 8px var(--gutter); }
  .empty { padding: 60px var(--gutter); }
  .empty h1 { margin: 0 0 8px; font: 700 32px/1.1 var(--font-display); }

  .hero {
    display: grid; grid-template-columns: auto 1fr auto; align-items: center; gap: 28px; margin: 6px var(--gutter) 0; padding: 24px 28px;
    border-radius: var(--radius-hero); background: linear-gradient(100deg, var(--tint) 0%, transparent 70%), var(--bg-surface);
  }
  .hero-art { display: block; box-shadow: var(--shadow-art); border-radius: 10px; }
  .eyebrow { margin: 0 0 8px; color: var(--delta-up); font: 700 12px/1 var(--font-sans); letter-spacing: 0.06em; text-transform: uppercase; }
  h1 { margin: 0; font: 700 40px/1.05 var(--font-display); letter-spacing: -0.015em; }
  h1 a { text-decoration: none; }
  h1 a:hover { text-decoration: underline; text-decoration-thickness: 2px; text-underline-offset: 5px; }
  .hero-meta { margin: 8px 0 0; color: var(--text-muted); font-size: 14.5px; }
  .hero-actions { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 18px; }
  .hero-stats { display: grid; grid-template-columns: repeat(2, minmax(118px, auto)); gap: 10px; margin: 0; }
  .hero-stats div { display: grid; padding: 10px 14px; border-radius: 14px; background: color-mix(in srgb, var(--bg-canvas) 45%, transparent); }
  .hero-stats dt { color: var(--text-muted); font-size: 12.5px; }
  .hero-stats dd { margin: 2px 0 0; font: 700 24px/1.1 var(--font-sans); letter-spacing: -0.02em; }
  .hero-stats span { color: var(--text-muted); font-size: 12.5px; }

  .kpis { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin: 16px var(--gutter) 0; }
  .kpi { display: grid; gap: 6px; padding: 16px 18px; border-radius: 16px; background: var(--bg-surface); }
  .k-label { color: var(--text-muted); font-size: 13px; font-weight: 600; }
  .k-row { display: flex; align-items: flex-end; justify-content: space-between; gap: 8px; height: 38px; }
  .k-row b { font: 700 32px/1 var(--font-sans); letter-spacing: -0.03em; }
  .of { color: var(--text-muted); font-size: 15px; font-weight: 600; letter-spacing: 0; }
  .k-sub { color: var(--text-muted); font-size: 12.5px; }

  .block { margin: 32px var(--gutter) 0; }
  .block-head { display: flex; flex-wrap: wrap; align-items: center; gap: 12px 16px; margin-bottom: 12px; }
  h2 { margin: 0; font: 700 21px/1.2 var(--font-display); letter-spacing: -0.01em; }
  .more { margin-left: auto; color: var(--text-muted); font-size: 13.5px; text-decoration: none; }
  .more:hover { color: var(--text-primary); text-decoration: underline; }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { height: 28px; padding: 0 14px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13px/1 var(--font-sans); cursor: pointer; }
  .seg button[aria-selected='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }
  .seg button:disabled { opacity: 0.4; cursor: default; }
  .top-rail { display: flex; gap: 4px; margin: 0; padding: 0 0 6px; list-style: none; overflow-x: auto; scrollbar-width: thin; }
  .top-rail li { position: relative; display: flex; align-items: flex-end; flex: none; }
  .big { margin-right: -14px; color: var(--bg-canvas); -webkit-text-stroke: 2px var(--border-strong); font: 800 100px/0.78 var(--font-sans); letter-spacing: -0.06em; }
  .tile { position: relative; z-index: 1; display: block; border-radius: 8px; transition: transform 0.15s ease; }
  .tile:hover { transform: translateY(-3px); }
  .mark { position: absolute; top: 6px; right: 6px; display: grid; place-items: center; width: 22px; height: 22px; border-radius: 50%; background: var(--accent-yours); color: var(--bg-canvas); }

  .two { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin: 32px var(--gutter) 0; }
  .panel.wide { margin: 16px var(--gutter) 0; }
  .cell { display: flex; align-items: center; gap: 10px; min-width: 0; max-width: 300px; color: inherit; text-decoration: none; }
  .cell:hover .ell { text-decoration: underline; text-underline-offset: 3px; }
  .ell { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 600 15px/1.2 var(--font-display); }
  .tbl td { white-space: nowrap; }
  .codes { display: flex; gap: 14px; color: var(--text-muted); }
  .code b { color: var(--text-primary); font-weight: 700; }
  .new { color: var(--delta-debut); font-weight: 700; font-size: 12px; }
  .empty-note { margin: 4px 12px 10px; color: var(--text-muted); }

  @media (max-width: 1100px) {
    .hero { grid-template-columns: auto 1fr; }
    .hero-stats { grid-column: 1 / -1; grid-template-columns: repeat(auto-fit, minmax(130px, 1fr)); }
    .kpis { grid-template-columns: repeat(2, 1fr); }
    .two { grid-template-columns: 1fr; }
  }
  @media (max-width: 640px) {
    .hero { grid-template-columns: 1fr; padding: 20px; }
    .hero-art { display: none; }
    h1 { font-size: 32px; }
    .kpis { grid-template-columns: 1fr 1fr; gap: 8px; }
    .k-row b { font-size: 26px; }
    .k-row svg { display: none; }
    .big { font-size: 76px; }
    .panel, .panel.wide { overflow-x: auto; }
  }
</style>
