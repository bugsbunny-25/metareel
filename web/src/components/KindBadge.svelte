<script>
  let { kind } = $props()
</script>

{#if kind === 'movie'}
  <span class="badge badge-accent">Movie</span>
{:else}
  <span class="badge badge-info">TV</span>
{/if}
