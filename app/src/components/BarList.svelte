<!-- Ranked horizontal bars: label, a bar on one shared scale from zero, the value. A real list,
     so screen readers get every row. items: [{ label, value, note? }] -->
<script>
  let { items = [], unit = '' } = $props()
  const max = $derived(Math.max(1, ...items.map((d) => d.value)))
</script>

<ol class="bars">
  {#each items as d}
    <li>
      <span class="lab" title={d.label}>{d.label}</span>
      <span class="track" aria-hidden="true"><i style:width="{(d.value / max) * 100}%"></i></span>
      <span class="val num">{d.value}{unit}{#if d.note}<span class="note">{' '}{d.note}</span>{/if}</span>
    </li>
  {/each}
</ol>

<style>
  .bars { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }
  li { display: grid; grid-template-columns: minmax(80px, 34%) 1fr auto; align-items: center; gap: 12px; }
  .lab { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 14px; }
  .track { height: 10px; border-radius: 5px; background: var(--bg-elevated); overflow: hidden; }
  .track i { display: block; height: 100%; border-radius: 5px; background: var(--accent-yours); }
  .val { min-width: 28px; text-align: right; font-weight: 700; font-size: 14px; }
  .note { color: var(--text-muted); font-weight: 400; font-size: 12.5px; }
</style>
