<script>
  import Icon from './Icon.svelte'

  let { onclose, header, children, footer } = $props()

  function onkeydown(e) {
    if (e.key === 'Escape') onclose()
  }
</script>

<svelte:window {onkeydown} />

<div class="overlay" onclick={onclose} role="presentation"></div>
<div class="drawer" role="dialog" aria-modal="true" tabindex="-1">
  <div class="drawer-header">
    <div class="grow">{@render header()}</div>
    <button class="btn btn-ghost btn-icon" onclick={onclose} title="Close (Esc)"><Icon name="x" /></button>
  </div>
  <div class="drawer-body">{@render children()}</div>
  {#if footer}
    <div class="drawer-footer">{@render footer()}</div>
  {/if}
</div>
