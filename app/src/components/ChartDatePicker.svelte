<!-- Which day's Top 10 to show: previous/next chart day, a date field limited to the days this
     scope has charts for, and "Latest" to go back to today's. value is '' for the latest chart;
     shown is the date of the chart on screen. A day with no chart snaps to the nearest earlier one. -->
<script>
  import { api } from '../lib/api.js'
  import { dayMonth } from '../lib/viz.js'

  let { country, provider, value = '', shown = null, onchange } = $props()

  let days = $state([]) // ascending YYYY-MM-DD
  $effect(() => {
    const ctrl = new AbortController()
    days = []
    api.chartDates(country, provider, ctrl.signal)
      .then((r) => (days = [...new Set((r?.items ?? []).map((x) => x.date))].sort()))
      .catch(() => {})
    return () => ctrl.abort()
  })

  const current = $derived(value || shown || days.at(-1) || '')
  const idx = $derived(days.indexOf(current))
  const prev = $derived(idx > 0 ? days[idx - 1] : idx === -1 ? days.findLast((d) => d < current) ?? null : null)
  const next = $derived(idx >= 0 && idx < days.length - 1 ? days[idx + 1] : idx === -1 ? days.find((d) => d > current) ?? null : null)
  const latest = $derived(days.at(-1) ?? null)

  // The latest day is stored as '' so links and reloads follow new scrapes.
  const go = (d) => onchange(!d || d === latest ? '' : d)
  function pick(e) {
    const d = e.currentTarget.value
    if (!d) return go('')
    go(days.includes(d) ? d : days.findLast((x) => x <= d) ?? days[0] ?? d)
  }
</script>

<div class="date-pick" role="group" aria-label="Chart date">
  <button type="button" class="step" onclick={() => go(prev)} disabled={!prev} aria-label={prev ? `Previous chart, ${dayMonth(prev)}` : 'No earlier chart'} title="Previous day">‹</button>
  <label>
    <span class="sr">Chart date</span>
    <input type="date" value={current} min={days[0]} max={latest} onchange={pick} disabled={!days.length} />
  </label>
  <button type="button" class="step" onclick={() => go(next)} disabled={!next} aria-label={next ? `Next chart, ${dayMonth(next)}` : 'No later chart'} title="Next day">›</button>
  {#if value && value !== latest}<button type="button" class="latest" onclick={() => go('')}>Latest</button>{/if}
</div>

<style>
  .date-pick { display: inline-flex; align-items: center; gap: 4px; }
  input {
    height: 32px; padding: 0 10px; border: 0; border-radius: 16px; background: var(--bg-elevated);
    color: var(--text-primary); font: 600 13px/1 var(--font-sans); font-variant-numeric: tabular-nums; color-scheme: inherit;
  }
  input:disabled { opacity: 0.6; }
  .step {
    display: grid; place-items: center; width: 32px; height: 32px; border: 0; border-radius: 50%;
    background: var(--bg-elevated); color: var(--text-primary); font: 600 18px/1 var(--font-sans); cursor: pointer;
  }
  .step:disabled { opacity: 0.35; cursor: default; }
  .latest { height: 32px; padding: 0 12px; border: 0; border-radius: 16px; background: var(--accent-primary); color: var(--accent-on); font: 600 13px/1 var(--font-sans); cursor: pointer; }
</style>
