<script>
  // Ranked horizontal bars. items: [{ key, label, value, display?, sub?, href?, color? }]
  let { items, max = null, color = 'var(--c1)' } = $props()

  const top = $derived(max ?? Math.max(1, ...items.map((i) => i.value)))
</script>

<div class="hbars">
  {#each items as it (it.key ?? it.label)}
    <svelte:element this={it.href ? 'a' : 'div'} class="hbar" href={it.href || undefined} title={it.title ?? undefined}>
      <span class="hbar-label">{it.label}</span>
      <span class="hbar-track"><span class="hbar-fill" style="width:{(it.value / top) * 100}%;background:{it.color ?? color}"></span></span>
      <span class="hbar-value">{it.display ?? it.value}{#if it.sub}<small>{it.sub}</small>{/if}</span>
    </svelte:element>
  {/each}
</div>
