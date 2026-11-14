<!-- A year of watching, one square per local day (weeks as columns, Monday first). Intensity
     is the day's watch count in four steps of the "yours" orange. Days arrive sparse from
     /me/stats/calendar; the gaps are filled here. -->
<script>
  import { addDays, dayLabel, monthLabel } from '../lib/dates.js'

  let { days = [], to, onpick } = $props()

  // On narrow screens the year scrolls; start at the recent end, not last December.
  let wrap
  $effect(() => { if (wrap && days) wrap.scrollLeft = wrap.scrollWidth })

  const CELL = 12, GAP = 3, LEFT = 24, TOP = 16
  const byDay = $derived(new Map(days.map((d) => [d.day, d])))

  // 53 columns ending on the week of `to`; rows Monday..Sunday.
  const grid = $derived.by(() => {
    const end = new Date(to + 'T12:00:00Z')
    const endDow = (end.getUTCDay() + 6) % 7 // Monday = 0
    const start = addDays(to, -(52 * 7 + endDow))
    const cells = []
    for (let i = 0; i < 53 * 7; i++) {
      const day = addDays(start, i)
      if (day > to) break
      const s = byDay.get(day)
      cells.push({ day, col: Math.floor(i / 7), row: i % 7, n: s?.watches ?? 0, minutes: s?.minutes ?? 0 })
    }
    return cells
  })
  const months = $derived(grid.filter((c) => c.row === 0 && c.day.slice(8) <= '07').map((c) => ({ col: c.col, label: monthLabel(c.day).slice(0, 3) })))
  const level = (n) => (n === 0 ? 0 : n === 1 ? 1 : n <= 3 ? 2 : n <= 5 ? 3 : 4)
  const W = LEFT + 53 * (CELL + GAP)
  const H = TOP + 7 * (CELL + GAP)
</script>

<div class="wrap" bind:this={wrap}>
  <!-- role=group (not img) so the day buttons inside stay reachable by assistive tech. -->
  <svg viewBox="0 0 {W} {H}" width={W} height={H} role="group" aria-label="Watches per day over the last year">
    {#each months as m}<text x={LEFT + m.col * (CELL + GAP)} y="10" class="lab">{m.label}</text>{/each}
    {#each [['Mon', 0], ['Wed', 2], ['Fri', 4]] as [l, r]}<text x="0" y={TOP + r * (CELL + GAP) + 10} class="lab">{l}</text>{/each}
    {#each grid as c (c.day)}
      {@const label = `${dayLabel(c.day)}: ${c.n === 0 ? 'nothing logged' : `${c.n} watch${c.n === 1 ? '' : 'es'}${c.minutes ? `, ${Math.round(c.minutes / 6) / 10} h` : ''}`}`}
      {#if onpick && c.n > 0}
        <!-- Days with watches are buttons: they filter the diary to that day. -->
        <rect
          x={LEFT + c.col * (CELL + GAP)} y={TOP + c.row * (CELL + GAP)} width={CELL} height={CELL} rx="3"
          class="l{level(c.n)} pick" role="button" tabindex="0" aria-label="{label}. Show this day"
          onclick={() => onpick(c.day)}
          onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onpick(c.day) } }}
        ><title>{label}</title></rect>
      {:else}
        <rect x={LEFT + c.col * (CELL + GAP)} y={TOP + c.row * (CELL + GAP)} width={CELL} height={CELL} rx="3" class="l{level(c.n)}"><title>{label}</title></rect>
      {/if}
    {/each}
  </svg>
  <p class="legend" aria-hidden="true">Less <i class="l0"></i><i class="l1"></i><i class="l2"></i><i class="l3"></i><i class="l4"></i> More</p>
</div>

<style>
  .wrap { overflow-x: auto; padding-bottom: 4px; }
  svg { display: block; }
  .lab { fill: var(--text-muted); font: 500 10px var(--font-sans); }
  rect.l0, i.l0 { fill: var(--bg-elevated); background: var(--bg-elevated); }
  rect.l1, i.l1 { fill: color-mix(in srgb, var(--accent-yours) 35%, var(--bg-elevated)); background: color-mix(in srgb, var(--accent-yours) 35%, var(--bg-elevated)); }
  rect.l2, i.l2 { fill: color-mix(in srgb, var(--accent-yours) 60%, var(--bg-elevated)); background: color-mix(in srgb, var(--accent-yours) 60%, var(--bg-elevated)); }
  rect.l3, i.l3 { fill: color-mix(in srgb, var(--accent-yours) 82%, var(--bg-elevated)); background: color-mix(in srgb, var(--accent-yours) 82%, var(--bg-elevated)); }
  rect.l4, i.l4 { fill: var(--accent-yours); background: var(--accent-yours); }
  rect.pick { cursor: pointer; }
  rect:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 1px; }
  .legend { display: flex; align-items: center; justify-content: flex-end; gap: 3px; margin: 6px 0 0; color: var(--text-muted); font-size: 11.5px; }
  .legend i { display: inline-block; width: 11px; height: 11px; border-radius: 3px; }
</style>
