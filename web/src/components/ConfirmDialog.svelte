<script>
  import Modal from './Modal.svelte'
  import { pending, settle } from '../lib/confirm.svelte.js'
</script>

{#if pending.current}
  {@const p = pending.current}
  <Modal title={p.title} onclose={() => settle(false)}>
    <p class="muted">{p.message}</p>
    {#snippet footer()}
      <button class="btn" onclick={() => settle(false)}>Cancel</button>
      <!-- svelte-ignore a11y_autofocus -->
      <button class="btn" class:btn-primary={!p.danger} class:btn-danger={p.danger} autofocus onclick={() => settle(true)}>{p.confirmLabel}</button>
    {/snippet}
  </Modal>
{/if}
