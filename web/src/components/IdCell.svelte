<script>
  import Icon from './Icon.svelte'

  // An external ID with a link, or a "missing" marker.
  let { value, url, label = 'Missing' } = $props()
</script>

{#if value}
  <span class="id-chip">
    {value}
    {#if url}
      <a href={url} target="_blank" rel="noopener noreferrer" title="Open in new tab" onclick={(e) => e.stopPropagation()}><Icon name="external" size={12} /></a>
    {/if}
  </span>
{:else}
  <span class="id-missing"><Icon name="alert" size={12} />{label}</span>
{/if}
