<script>
  import Icon from './Icon.svelte'
  import { toasts, dismiss } from '../lib/toast.svelte.js'
</script>

<div class="toasts" aria-live="polite">
  {#each toasts as t (t.id)}
    <div class="toast toast-{t.kind}" role={t.kind === 'error' ? 'alert' : 'status'}>
      <span style="color: var(--{t.kind === 'error' ? 'danger' : t.kind === 'success' ? 'success' : 'accent'})">
        <Icon name={t.kind === 'error' ? 'alert' : t.kind === 'success' ? 'check' : 'sparkles'} />
      </span>
      <div class="msg">{t.message}</div>
      <button class="btn btn-ghost btn-sm btn-icon" onclick={() => dismiss(t.id)} title="Dismiss"><Icon name="x" size={12} /></button>
    </div>
  {/each}
</div>
