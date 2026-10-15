<script>
  let { series } = $props()
</script>

<div class="legend">
  {#each series as s (s.key)}<span class="legend-item"><span class="swatch" style="background:{s.color}"></span>{s.label}</span>{/each}
</div>
