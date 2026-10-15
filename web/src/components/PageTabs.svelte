<script>
  // Page-level tabs. tabs: [{ key, label, count? }]; the active tab is kept
  // in the URL by the page (onselect).
  let { tabs, active, onselect, label = 'Sections' } = $props()
</script>

<div class="page-tabs" role="tablist" aria-label={label}>
  {#each tabs as t (t.key)}
    <button role="tab" aria-selected={active === t.key} class:active={active === t.key} onclick={() => onselect(t.key)}>
      {t.label}{#if t.count}<span class="badge badge-warning">{t.count}</span>{/if}
    </button>
  {/each}
</div>
