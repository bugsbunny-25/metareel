<!-- Trends (#/discover?tab=trends&range=30|90&svc=1): what the charts in the country say, from
     metareel's analytics: genres' share of Top 10 points, how long debuts stay in the Top 10
     (/analytics/decay), how long titles take from release to the charts per service
     (/analytics/release-lag) and each service's originals versus licensed titles
     (/analytics/origins). svc=1 narrows genres and staying power to the service in the top bar.
     Each panel loads on its own and says so when it fails. -->
<script>
  import BarList from '../../components/BarList.svelte'
  import { api } from '../../lib/api.js'
  import { route, setQuery } from '../../lib/router.svelte.js'
  import { inCountry, providerName } from '../../lib/scope.svelte.js'
  import { seriesPath } from '../../lib/viz.js'
  import { localDay, addDays, dayLabel } from '../../lib/dates.js'

  let { country, provider } = $props()

  const range = $derived(+route.query.range === 90 ? 90 : 30)
  const svc = $derived(route.query.svc === '1')

  let genres = $state(null), decay = $state(null), lag = $state(null), origins = $state(null)
  $effect(() => {
    const cc = country, from = addDays(localDay(), -range), p = svc ? provider : undefined
    const ctrl = new AbortController()
    genres = decay = lag = origins = null
    const load = (name, params, set) => api.analytics(name, params, ctrl.signal).then(set).catch((err) => { if (err.name !== 'AbortError') set({ failed: err.message }) })
    load('genres', { country: cc, provider: p, from }, (d) => (genres = d))
    load('decay', { country: cc, provider: p, from }, (d) => (decay = d))
    load('release-lag', { country: cc, from }, (d) => (lag = d))
    load('origins', { country: cc, from }, (d) => (origins = d))
    return () => ctrl.abort()
  })

  const genreBars = $derived((genres?.items ?? []).slice(0, 10).map((g) => ({ label: g.genre, value: Math.round(g.share * 100), note: `${g.titles} titles` })))
  const curve = $derived(decay?.curve ?? [])
  const halfDay = $derived(curve.find((c) => c.retention < 0.5)?.day ?? null)
  const weekOn = $derived(curve.find((c) => c.day === 7)?.retention ?? null)
  const W = 560, H = 150
  const path = $derived(curve.length > 1 ? seriesPath(curve.map((c) => c.retention * 100), W, H, 4) : null)
  const lagBars = $derived(Object.entries(lag?.by_provider ?? {}).filter(([, v]) => v.titles >= 3)
    .sort((a, b) => a[1].median_days - b[1].median_days)
    .map(([p, v]) => ({ label: providerName(p), value: Math.round(v.median_days), note: `${v.titles} titles` })))
  const ORIGINS = [['original', 'Original'], ['licensed', 'Licensed'], ['in_house', 'In-house'], ['unknown', 'Unknown']]
  const originRows = $derived((origins?.providers ?? []).filter((p) => p.points > 0).sort((a, b) => b.points - a.points))
  const mine = $derived(originRows.find((p) => p.provider === provider) ?? null)
  const pct = (x) => `${Math.round((x ?? 0) * 100)}%`
  const pick = (patch) => setQuery(patch, { replace: true })
  const failed = (d) => d?.failed
</script>

<div class="d-filters">
  <div class="d-seg" role="group" aria-label="Range">
    <button aria-pressed={range === 30} onclick={() => pick({ range: '' })}>Last 30 days</button>
    <button aria-pressed={range === 90} onclick={() => pick({ range: 90 })}>Last 90 days</button>
  </div>
  <div class="d-seg" role="group" aria-label="Services">
    <button aria-pressed={!svc} onclick={() => pick({ svc: '' })}>Every service in {country}</button>
    <button aria-pressed={svc} onclick={() => pick({ svc: '1' })}>{providerName(provider)}</button>
  </div>
</div>

<section class="d-kpis" aria-label="Trends at a glance">
  <div class="d-kpi"><span class="k-label">Top genre</span><b class="small">{genres?.items?.[0]?.genre ?? '–'}</b><span class="k-sub">{genres?.items?.[0] ? `${pct(genres.items[0].share)} of Top 10 points` : genres ? 'no data' : 'loading…'}</span></div>
  <div class="d-kpi"><span class="k-label">Half gone by</span><b class="num">{halfDay != null ? `day ${halfDay}` : '–'}</b><span class="k-sub">{decay?.debuts ? `of ${decay.debuts} debuts` : decay ? 'no debuts' : 'loading…'}</span></div>
  <div class="d-kpi"><span class="k-label">Release to the charts</span><b class="num">{lag?.summary?.titles ? `${Math.round(lag.summary.median_days)} days` : '–'}</b><span class="k-sub">{lag?.summary?.titles ? `median over ${lag.summary.titles} titles` : lag ? 'no data' : 'loading…'}</span></div>
  <div class="d-kpi yours"><span class="k-label">{providerName(provider)} originals</span><b class="num">{mine ? pct(mine.origins?.original?.share) : '–'}</b><span class="k-sub">{mine ? 'of its Top 10 points' : origins ? `no ${providerName(provider)} chart here` : 'loading…'}</span></div>
</section>

<div class="grid">
  <section class="panel pad">
    <h2>Genres in the Top 10</h2>
    <p class="hint">Share of Top 10 points in {inCountry(country)}{svc ? ` on ${providerName(provider)}` : ''}{genres?.from ? `, ${dayLabel(genres.from)} – ${dayLabel(genres.to)}` : ''}. A title counts for each of its genres.</p>
    {#if failed(genres)}<p class="hint bad">Didn’t load: {genres.failed}</p>
    {:else if !genres}<div class="skeleton" style="height:220px"></div>
    {:else if genreBars.length}<BarList items={genreBars} unit="%" />
    {:else}<p class="hint">No chart points in this range.</p>{/if}
  </section>

  <section class="panel pad">
    <h2>How long hits last</h2>
    <p class="hint">Share of titles that debuted{svc ? ` on ${providerName(provider)}` : ''} still in a Top 10, by days since their debut.</p>
    {#if failed(decay)}<p class="hint bad">Didn’t load: {decay.failed}</p>
    {:else if !decay}<div class="skeleton" style="height:180px"></div>
    {:else if path}
      <svg class="chart" viewBox="0 0 {W} {H + 18}" role="img" aria-label="Retention curve: {weekOn != null ? `${pct(weekOn)} still charting after a week` : ''}{halfDay != null ? `, half gone by day ${halfDay}` : ''}">
        <line x1="4" x2={W - 4} y1={path.y(50)} y2={path.y(50)} stroke="var(--border-subtle)" stroke-dasharray="3 4" />
        <path d={path.area} fill="var(--accent-yours)" opacity="0.14" />
        <path d={path.line} fill="none" stroke="var(--accent-yours)" stroke-width="2" stroke-linejoin="round" />
        {#if halfDay != null}<circle cx={path.x(curve.findIndex((c) => c.day === halfDay))} cy={path.y(curve.find((c) => c.day === halfDay).retention * 100)} r="4" fill="var(--accent-yours)" />{/if}
        <text x="4" y={H + 14} class="ax">Debut</text>
        <text x={W - 4} y={H + 14} class="ax" text-anchor="end">Day {curve.at(-1).day}</text>
        <text x={W - 4} y={path.y(50) - 5} class="ax" text-anchor="end">50%</text>
      </svg>
      <p class="hint">{weekOn != null ? `${pct(weekOn)} are still charting a week after their debut` : ''}{halfDay != null ? `; half have dropped out by day ${halfDay}.` : '.'}</p>
    {:else}<p class="hint">No debuts in this range.</p>{/if}
  </section>

  <section class="panel pad">
    <h2>From release to the charts</h2>
    <p class="hint">Median days from a title’s release to its first day in a Top 10 in {inCountry(country)}, per service. Fewer is faster.</p>
    {#if failed(lag)}<p class="hint bad">Didn’t load: {lag.failed}</p>
    {:else if !lag}<div class="skeleton" style="height:200px"></div>
    {:else if lagBars.length}<BarList items={lagBars} unit=" d" />
    {:else}<p class="hint">Too few titles in this range.</p>{/if}
  </section>

  <section class="panel pad">
    <h2>Originals or licensed</h2>
    <p class="hint">What each service’s Top 10 points in {inCountry(country)} came from.</p>
    {#if failed(origins)}<p class="hint bad">Didn’t load: {origins.failed}</p>
    {:else if !origins}<div class="skeleton" style="height:200px"></div>
    {:else if originRows.length}
      <ul class="legend" aria-hidden="true">{#each ORIGINS as [k, l]}<li><i class="o-{k}"></i>{l}</li>{/each}</ul>
      <ol class="stack">
        {#each originRows as p (p.provider)}
          <li class:cur={p.provider === provider}>
            <span class="lab">{providerName(p.provider)}</span>
            <span class="bar" role="img" aria-label="{providerName(p.provider)}: {ORIGINS.map(([k, l]) => `${l} ${pct(p.origins?.[k]?.share)}`).join(', ')}">
              {#each ORIGINS as [k]}<i class="o-{k}" style:width={pct(p.origins?.[k]?.share)}></i>{/each}
            </span>
            <span class="val num">{pct(p.origins?.original?.share)}</span>
          </li>
        {/each}
      </ol>
      <p class="hint">The number is the share from the service’s own originals.</p>
    {:else}<p class="hint">No chart points in this range.</p>{/if}
  </section>
</div>
<p class="d-source">Computed by metareel from the FlixPatrol Top 10 charts it stores, TMDB metadata and release dates.</p>

<style>
  .grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; align-items: start; }
  .chart { display: block; width: 100%; height: auto; }
  .ax { fill: var(--text-muted); font: 400 12px var(--font-sans); }
  .bad { color: var(--delta-down); }
  .legend { display: flex; flex-wrap: wrap; gap: 6px 14px; margin: 2px 0 4px; padding: 0; list-style: none; color: var(--text-muted); font-size: 12.5px; }
  .legend li { display: inline-flex; align-items: center; gap: 6px; }
  .legend i { width: 10px; height: 10px; border-radius: 3px; }
  .stack { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }
  .stack li { display: grid; grid-template-columns: minmax(80px, 30%) 1fr 40px; align-items: center; gap: 12px; font-size: 14px; }
  .stack li.cur .lab { color: var(--accent-yours); font-weight: 700; }
  .lab { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .bar { display: flex; height: 12px; border-radius: 6px; overflow: hidden; background: var(--bg-elevated); }
  .bar i { display: block; height: 100%; }
  .val { text-align: right; font-weight: 700; }
  .o-original { background: var(--accent-yours); }
  .o-licensed { background: color-mix(in srgb, var(--text-primary) 55%, transparent); }
  .o-in_house { background: color-mix(in srgb, var(--accent-yours) 45%, var(--bg-elevated)); }
  .o-unknown { background: color-mix(in srgb, var(--text-primary) 18%, transparent); }
  @media (max-width: 960px) {
    .grid { grid-template-columns: 1fr; }
  }
</style>
