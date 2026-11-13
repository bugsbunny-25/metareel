<!-- Vertical columns for a short series (months, weekdays, hours). Heights share one scale from
     zero; the tallest is labelled and every column has a tooltip and a screen-reader summary.
     items: [{ label, value, title? }] -->
<script>
  let { items = [], height = 120, label = '', unit = '', every = 1, highlight = -1 } = $props()

  const max = $derived(Math.max(1, ...items.map((d) => d.value)))
  const peak = $derived(items.reduce((b, d, i) => (d.value > (items[b]?.value ?? -1) ? i : b), 0))
</script>

<figure class="cols" aria-label={label}>
  <div class="plot" style:height="{height}px" role="img" aria-label="{label}: {items.map((d) => `${d.label} ${d.value}${unit}`).join(', ')}">
    {#each items as d, i}
      <div class="col" title={d.title ?? `${d.label}: ${d.value}${unit}`}>
        {#if i === peak && d.value > 0}<span class="peak num">{d.value}</span>{/if}
        <i class:hl={i === highlight} class:zero={!d.value} style:height="{d.value ? Math.max(3, (d.value / max) * (height - 18)) : 2}px"></i>
      </div>
    {/each}
  </div>
  <div class="axis" aria-hidden="true">
    {#each items as d, i}<span>{i % every === 0 ? d.label : ''}</span>{/each}
  </div>
</figure>

<style>
  .cols { margin: 0; }
  .plot { display: flex; align-items: flex-end; gap: 4px; box-shadow: inset 0 -1px 0 var(--border-subtle); }
  .col { position: relative; flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: flex-end; min-width: 0; height: 100%; }
  i { display: block; width: 100%; max-width: 34px; border-radius: 4px 4px 0 0; background: color-mix(in srgb, var(--accent-yours) 70%, var(--bg-elevated)); }
  i.hl { background: var(--accent-yours); }
  i.zero { background: var(--bg-elevated); }
  .peak { margin-bottom: 3px; color: var(--text-primary); font-size: 12px; font-weight: 700; }
  .axis { display: flex; gap: 4px; margin-top: 6px; }
  .axis span { flex: 1; min-width: 0; overflow: hidden; color: var(--text-muted); font-size: 11px; text-align: center; white-space: nowrap; }
</style>
