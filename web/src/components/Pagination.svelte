<script>
  import Icon from './Icon.svelte'
  import { PAGE_SIZES } from '../lib/constants.js'

  // page is 1-based. onchange(page, size) is called for any change.
  let { page, size, total, onchange } = $props()

  const pages = $derived(Math.max(1, Math.ceil(total / size)))
  const first = $derived(total === 0 ? 0 : (page - 1) * size + 1)
  const last = $derived(Math.min(total, page * size))

  function go(p) {
    const next = Math.min(Math.max(1, p), pages)
    if (next !== page) onchange(next, size)
  }

  function onJump(e) {
    const n = Number.parseInt(e.currentTarget.value, 10)
    if (Number.isFinite(n)) go(n)
    else e.currentTarget.value = page
  }
</script>

<div class="pagination">
  <div>
    {#if total === 0}
      No results
    {:else}
      Showing <strong>{first.toLocaleString()}–{last.toLocaleString()}</strong> of <strong>{total.toLocaleString()}</strong>
    {/if}
  </div>
  <div class="controls">
    <label class="row small">
      Rows
      <select class="select" value={size} onchange={(e) => onchange(1, Number(e.currentTarget.value))}>
        {#each PAGE_SIZES as s (s)}<option value={s}>{s}</option>{/each}
      </select>
    </label>
    <button class="btn btn-sm btn-icon" title="First page" disabled={page <= 1} onclick={() => go(1)}><Icon name="chevronsLeft" size={14} /></button>
    <button class="btn btn-sm btn-icon" title="Previous page" disabled={page <= 1} onclick={() => go(page - 1)}><Icon name="chevronLeft" size={14} /></button>
    <span class="row small">
      Page
      <input class="input" aria-label="Page number" value={page} onchange={onJump} onkeydown={(e) => e.key === 'Enter' && onJump(e)} />
      of {pages.toLocaleString()}
    </span>
    <button class="btn btn-sm btn-icon" title="Next page" disabled={page >= pages} onclick={() => go(page + 1)}><Icon name="chevronRight" size={14} /></button>
    <button class="btn btn-sm btn-icon" title="Last page" disabled={page >= pages} onclick={() => go(pages)}><Icon name="chevronsRight" size={14} /></button>
  </div>
</div>
