<script>
  // 14-day rank trend: rank 1 at the top, gaps where the title was off the chart.
  import { rankPath } from '../lib/viz.js';

  let { ranks, width = 64, height = 20, label = '' } = $props();
  const p = $derived(ranks ? rankPath(ranks, width, height) : null);
  const last = $derived(ranks ? ranks.length - 1 : -1);
</script>

{#if p && p.d}
  <svg class="spark" {width} {height} viewBox="0 0 {width} {height}" role="img" aria-label={label}>
    <path d={p.d} fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round" />
    {#if ranks[last] != null}<circle cx={p.x(last)} cy={p.y(ranks[last])} r="2" fill="currentColor" />{/if}
  </svg>
{:else}
  <span class="none" aria-label="No trend yet">—</span>
{/if}

<style>
  .spark { display: block; overflow: visible; }
  .none { color: var(--text-muted); }
</style>
