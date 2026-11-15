<!-- A title's Top 10 run over the last 30 days in one country: rank (1 at the top) by day, one line per service, a
     gap where it left the chart. The country is the first in your country order (Settings) that
     the title has charted in; with no order set, the chart country you browse. Nothing renders
     when it charted in none of them.
     rankings: /titles/tmdb/{kind}/{id}/rankings → rankings [{ date, country, provider, rank }] -->
<script>
  import { prefs } from '../lib/prefs.svelte.js'
  import { scope, providerName, countryName } from '../lib/scope.svelte.js'
  import { dayMonth } from '../lib/viz.js'

  let { rankings = [] } = $props()

  const COLORS = ['var(--accent-yours)', '#8eb4f0', '#30d158', '#bf5af2', '#ffd60a', '#ff6482']
  const order = $derived.by(() => {
    const list = prefs.values.countries?.length ? prefs.values.countries : [scope.country, prefs.values.country].filter(Boolean)
    return [...new Set(list)]
  })
  // The first country in your order the title charted in (in the 30 days asked for), in that order.
  const country = $derived(order.find((c) => rankings.some((r) => r.country === c)) ?? null)
  const rows = $derived(country ? rankings.filter((r) => r.country === country) : [])

  // A fixed, compact height; the width is the panel's, measured (a viewBox would grow tall on wide screens).
  let W = $state(320)
  const H = 112
  const PAD = { l: 26, r: 10, t: 8, b: 22 }
  const graph = $derived.by(() => {
    if (!rows.length) return null
    const days = [...new Set(rows.map((r) => r.date))].sort()
    const t0 = Date.parse(days[0])
    const t1 = Date.parse(days.at(-1))
    const span = Math.max(1, (t1 - t0) / 86400000)
    const x = (d) => PAD.l + ((Date.parse(d) - t0) / 86400000 / span) * (W - PAD.l - PAD.r)
    const y = (rank) => PAD.t + ((rank - 1) / 9) * (H - PAD.t - PAD.b)
    const providers = [...new Set(rows.map((r) => r.provider))]
    const lines = providers.map((p, i) => {
      const pts = rows.filter((r) => r.provider === p).sort((a, b) => a.date.localeCompare(b.date))
      // Break the line where a day is missing (it left the Top 10).
      let d = ''
      let prev = null
      for (const r of pts) {
        const gap = prev && (Date.parse(r.date) - Date.parse(prev.date)) / 86400000 > 1.5
        d += `${!prev || gap ? 'M' : 'L'}${x(r.date).toFixed(1)},${y(r.rank).toFixed(1)}`
        prev = r
      }
      const best = Math.min(...pts.map((r) => r.rank))
      return { provider: p, color: COLORS[i % COLORS.length], d, pts, best, days: pts.length }
    })
    const ticks = span <= 1 ? [days[0]] : [days[0], days[Math.floor(days.length / 2)], days.at(-1)].filter((v, i, a) => a.indexOf(v) === i)
    return { lines, x, y, ticks, single: days.length === 1 }
  })
</script>

{#if graph}
  <section class="panel pad t10g" aria-labelledby="t10g-h">
    <div class="ph">
      <h2 id="t10g-h">Top 10 in {countryName(country)}</h2>
      <span class="hint">Last 30 days{order.length > 1 ? ' · first country in your order it charted in' : ''}</span>
    </div>
    <div class="plot" bind:clientWidth={W}>
    <svg width={W} height={H} viewBox="0 0 {W} {H}" role="img" aria-label="Daily Top 10 rank in {countryName(country)}: {graph.lines.map((l) => `${providerName(l.provider)} best No. ${l.best} over ${l.days} days`).join('; ')}">
      {#each [1, 5, 10] as r}
        <line x1={PAD.l} x2={W - PAD.r} y1={graph.y(r)} y2={graph.y(r)} class="grid" />
        <text x={PAD.l - 8} y={graph.y(r) + 4} class="axis" text-anchor="end">{r}</text>
      {/each}
      {#each graph.ticks as d}
        <text x={graph.x(d)} y={H - 6} class="axis" text-anchor={d === graph.ticks[0] ? 'start' : d === graph.ticks.at(-1) ? 'end' : 'middle'}>{dayMonth(d)}</text>
      {/each}
      {#each graph.lines as l (l.provider)}
        <path d={l.d} fill="none" stroke={l.color} stroke-width="2.5" stroke-linejoin="round" stroke-linecap="round" />
        {#each l.pts as p (p.date)}
          <circle cx={graph.x(p.date)} cy={graph.y(p.rank)} r={graph.single ? 4 : 2.2} fill={l.color}><title>{providerName(l.provider)}, {dayMonth(p.date)}: No. {p.rank}</title></circle>
        {/each}
      {/each}
    </svg>
    </div>
    <ul class="legend">
      {#each graph.lines as l (l.provider)}
        <li><i style:background={l.color}></i>{providerName(l.provider)} <span class="hint num">best No. {l.best} · {l.days} {l.days === 1 ? 'day' : 'days'}</span></li>
      {/each}
    </ul>
  </section>
{/if}

<style>
  .t10g { gap: 10px; }
  .ph { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 12px; }
  /* min-width 0: in the panel's grid the drawing must not set the column's width. */
  .plot { width: 100%; min-width: 0; overflow: hidden; }
  svg { display: block; }
  .grid { stroke: var(--border-subtle); stroke-width: 1; }
  .axis { fill: var(--text-muted); font: 600 11px var(--font-sans); }
  .legend { display: flex; flex-wrap: wrap; gap: 6px 18px; margin: 0; padding: 0; list-style: none; font-size: 13.5px; font-weight: 600; }
  .legend li { display: inline-flex; align-items: center; gap: 7px; }
  .legend i { width: 10px; height: 10px; border-radius: 50%; }
</style>
