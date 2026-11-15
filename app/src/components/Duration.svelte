<!-- Watch time as months, days and hours, each a big numeral with its unit. -->
<script>
  import { splitDuration } from '../lib/duration.js'

  let { minutes = 0, size = 34 } = $props()
  const d = $derived(splitDuration(minutes))
  const parts = $derived([
    d.months ? [d.months, d.months === 1 ? 'month' : 'months'] : null,
    d.months || d.days ? [d.days, d.days === 1 ? 'day' : 'days'] : null,
    [d.hours, d.hours === 1 ? 'hour' : 'hours'],
  ].filter(Boolean))
</script>

<p class="dur" style:--size="{size}px" aria-label={parts.map(([n, u]) => `${n} ${u}`).join(', ')}>
  {#each parts as [n, unit]}<span class="part" aria-hidden="true"><b class="num">{n}</b><small>{unit}</small></span>{/each}
</p>

<style>
  .dur { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 16px; margin: 0; }
  .part { display: inline-flex; align-items: baseline; gap: 5px; }
  b { font: 700 var(--size)/1.05 var(--font-sans); letter-spacing: -0.03em; }
  small { color: var(--text-muted); font-size: 12px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.06em; }
</style>
