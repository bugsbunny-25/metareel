<!-- A modal yes/no for irreversible actions. request: null when closed, else
     { title, body, confirm: 'Delete watch', danger?: true, run: async () => {} }. -->
<script>
  let { request = $bindable(null) } = $props()

  let dialog
  let busy = $state(false)
  let error = $state('')

  $effect(() => {
    if (!dialog) return
    if (request && !dialog.open) {
      error = ''
      dialog.showModal()
    } else if (!request && dialog.open) {
      dialog.close()
    }
  })

  async function go() {
    busy = true
    error = ''
    try {
      await request.run()
      request = null
    } catch (err) {
      error = err.message
    } finally {
      busy = false
    }
  }
</script>

<dialog bind:this={dialog} onclose={() => (request = null)} aria-labelledby="confirm-h" aria-describedby="confirm-b">
  {#if request}
    <div class="box">
      <h2 id="confirm-h">{request.title}</h2>
      <p id="confirm-b">{request.body}</p>
      {#if error}<p class="err" role="alert">{error}</p>{/if}
      <footer>
        <button type="button" class="btn-secondary" onclick={() => (request = null)}>Cancel</button>
        <button type="button" class="btn-primary" class:danger={request.danger} disabled={busy} onclick={go}>{busy ? 'Working…' : request.confirm}</button>
      </footer>
    </div>
  {/if}
</dialog>

<style>
  dialog { width: min(420px, calc(100vw - 32px)); padding: 0; border: 0; border-radius: 20px; background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 30px 80px -20px rgb(0 0 0 / 0.6); }
  dialog::backdrop { background: rgb(0 0 0 / 0.55); }
  .box { display: grid; gap: 10px; padding: 22px; }
  h2 { margin: 0; font: 700 21px/1.2 var(--font-display); }
  p { margin: 0; color: var(--text-muted); line-height: 21px; }
  .err { color: var(--delta-down); }
  footer { display: flex; justify-content: flex-end; gap: 10px; margin-top: 8px; }
  .danger { background: var(--delta-down); color: var(--bg-canvas); }
</style>
