<script>
  let { title, onclose, children, footer, large = false } = $props()

  function onkeydown(e) {
    if (e.key === 'Escape') onclose()
  }
</script>

<svelte:window {onkeydown} />

<div class="overlay" onclick={onclose} role="presentation"></div>
<div class="modal-wrap" onclick={(e) => e.target === e.currentTarget && onclose()} role="presentation">
  <div class="modal" class:modal-lg={large} role="dialog" aria-modal="true" aria-label={title}>
    <div class="modal-header"><div class="modal-title">{title}</div></div>
    <div class="modal-body">{@render children()}</div>
    {#if footer}<div class="modal-footer">{@render footer()}</div>{/if}
  </div>
</div>
