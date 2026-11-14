<!-- Half-star rating, 1–10 (½ to 5 stars); null = unrated. Keyboard: arrows ±½,
     1–5 whole stars, Home/End ½ and 5, Backspace/Delete clears. Pointer picks the half under it. -->
<script>
  let { value = $bindable(null), label = 'Your rating' } = $props()

  const text = (v) => (v ? `${v / 2} of 5 stars` : 'Not rated')
  const set = (v) => (value = v == null ? null : Math.min(10, Math.max(1, v)))

  function onkeydown(e) {
    const k = e.key
    if (k === 'ArrowRight' || k === 'ArrowUp') set((value ?? 0) + 1)
    else if (k === 'ArrowLeft' || k === 'ArrowDown') set(value && value > 1 ? value - 1 : 1)
    else if (k === 'Home') set(1)
    else if (k === 'End') set(10)
    else if (k === 'Backspace' || k === 'Delete') set(null)
    else if (/^[1-5]$/.test(k)) set(Number(k) * 2)
    else return
    e.preventDefault()
  }

  function onpointer(e) {
    const r = e.currentTarget.getBoundingClientRect()
    set(Math.ceil(((e.clientX - r.left) / r.width) * 10))
  }
</script>

<div class="rating">
  <div
    class="stars"
    role="slider"
    tabindex="0"
    aria-label={label}
    aria-valuemin="1"
    aria-valuemax="10"
    aria-valuenow={value ?? undefined}
    aria-valuetext={text(value)}
    {onkeydown}
    onclick={onpointer}
  >
    {#each [1, 2, 3, 4, 5] as i}
      <span class="star" aria-hidden="true">
        <span class="fill" style:width={value >= i * 2 ? '100%' : value === i * 2 - 1 ? '50%' : '0%'}>★</span>★
      </span>
    {/each}
  </div>
  <span class="val">{value ? `${value / 2}` : 'Unrated'}</span>
  {#if value}<button type="button" class="clear" onclick={() => set(null)}>Clear</button>{/if}
</div>

<style>
  .rating { display: flex; align-items: center; gap: 12px; }
  .stars { display: inline-flex; gap: 2px; padding: 4px 6px; border-radius: 8px; cursor: pointer; font-size: 26px; line-height: 1; color: var(--border-strong); user-select: none; }
  .star { position: relative; display: inline-block; }
  .fill { position: absolute; inset: 0 auto 0 0; overflow: hidden; color: var(--accent-yours); }
  .val { min-width: 56px; color: var(--text-muted); font-variant-numeric: tabular-nums; }
  .clear { border: 0; background: none; color: var(--text-muted); text-decoration: underline; cursor: pointer; }
  .stars:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 2px; }
</style>
