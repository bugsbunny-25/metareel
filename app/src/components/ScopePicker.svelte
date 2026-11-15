<!-- Country and service pickers for a view that reads the chart scope (Home's Top 10, Charts,
     Discover). The choice is remembered per browser, so every view opens on the last pair picked. -->
<script>
  import { scope, reference, setScope, providerName } from '../lib/scope.svelte.js'

  /** label: names the pickers for screen readers, e.g. "Top 10". */
  let { label = 'Chart' } = $props()

  const providers = $derived(reference.providers.filter((p) => p.countries.includes(scope.country)))
</script>

<div class="scope-pick" role="group" aria-label="{label}: country and service">
  <label>
    <span class="sr">Country</span>
    <select value={scope.country} onchange={(e) => setScope({ country: e.currentTarget.value })} disabled={!reference.loaded}>
      {#each reference.countries as c}<option value={c.code}>{c.name}</option>{/each}
      {#if !reference.loaded}<option value={scope.country}>{scope.country}</option>{/if}
    </select>
  </label>
  <label>
    <span class="sr">Service</span>
    <select value={scope.provider} onchange={(e) => setScope({ provider: e.currentTarget.value })} disabled={!reference.loaded}>
      {#each providers as p}<option value={p.slug}>{providerName(p.slug)}</option>{/each}
      {#if !reference.loaded}<option value={scope.provider}>{providerName(scope.provider)}</option>{/if}
    </select>
  </label>
</div>

<style>
  .scope-pick { display: inline-flex; flex-wrap: wrap; gap: 8px; min-width: 0; }
  label { min-width: 0; }
  select {
    max-width: 100%; height: 32px; padding: 0 28px 0 12px; border: 0; border-radius: 16px; appearance: none; cursor: pointer;
    background: var(--bg-elevated) url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6'%3E%3Cpath d='M1 1l4 4 4-4' fill='none' stroke='%23a1a1a8' stroke-width='1.5'/%3E%3C/svg%3E") no-repeat right 11px center;
    color: var(--text-primary); font: 600 13px/1 var(--font-sans); text-overflow: ellipsis;
  }
  select:disabled { opacity: 0.6; cursor: default; }
</style>
