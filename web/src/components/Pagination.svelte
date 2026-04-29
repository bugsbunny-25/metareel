<script>
  let { offset, limit, total, onchange } = $props()

  const page = $derived(Math.floor(offset / limit) + 1)
  const totalPages = $derived(Math.max(1, Math.ceil(total / limit)))
</script>

<div class="pagination">
  <button
    class="btn btn-sm"
    disabled={offset === 0}
    onclick={() => onchange(Math.max(0, offset - limit))}
  >← Prev</button>
  <span>Page {page} of {totalPages} &nbsp;·&nbsp; {total} total</span>
  <button
    class="btn btn-sm"
    disabled={page >= totalPages}
    onclick={() => onchange(offset + limit)}
  >Next →</button>
</div>
