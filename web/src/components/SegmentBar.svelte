<script>
  // One bar split into parts of a whole, with a legend of counts and shares.
  // segments: [{ key, label, value, color, href? }]
  import { number } from '../lib/format.js'

  let { segments, label = 'Breakdown' } = $props()
  const total = $derived(segments.reduce((s, x) => s + x.value, 0))
  const pct = (v) => (total ? (v / total) * 100 : 0)
</script>

<div class="segbar" role="img" aria-label={`${label}: ${segments.map((s) => `${s.label} ${s.value}`).join(', ')}`}>
  {#each segments as s (s.key)}
    {#if s.value}<span style="width:{pct(s.value)}%;background:{s.color}" title="{s.label}: {number(s.value)} ({Math.round(pct(s.value))}%)"></span>{/if}
  {/each}
</div>
<div class="seglegend">
  {#each segments as s (s.key)}
    <svelte:element this={s.href && s.value ? 'a' : 'div'} class="segitem" href={s.href && s.value ? s.href : undefined}>
      <span class="swatch" style="background:{s.color}"></span>
      <span class="grow">{s.label}</span>
      <span class="num" style="font-weight:600">{number(s.value)}</span>
      <span class="num muted" style="min-width:38px;text-align:right">{Math.round(pct(s.value))}%</span>
    </svelte:element>
  {/each}
</div>

<style>
  .segbar { display: flex; height: 10px; border-radius: 99px; overflow: hidden; background: var(--surface-3); gap: 2px; }
  .segbar span { display: block; height: 100%; }
  .seglegend { display: grid; grid-template-columns: repeat(auto-fit, minmax(170px, 1fr)); gap: 2px 16px; margin-top: 12px; }
  .segitem { display: flex; align-items: center; gap: 8px; font-size: 12.5px; padding: 3px 0; color: var(--text-2); }
  a.segitem:hover { color: var(--accent); text-decoration: none; }
</style>
