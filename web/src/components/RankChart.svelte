<script>
  // Rank over time for one chart: rank 1 at the top, 10 at the bottom.
  // points: [{ date: 'YYYY-MM-DD', rank }], sorted by date.
  let { points, width = 640, height = 150 } = $props()

  const pad = { l: 28, r: 10, t: 10, b: 22 }
  const days = $derived.by(() => {
    if (points.length === 0) return { start: 0, span: 1 }
    const t0 = Date.parse(points[0].date)
    const t1 = Date.parse(points[points.length - 1].date)
    return { start: t0, span: Math.max(1, t1 - t0) }
  })
  const x = (date) => pad.l + ((Date.parse(date) - days.start) / days.span) * (width - pad.l - pad.r)
  const y = (rank) => pad.t + ((rank - 1) / 9) * (height - pad.t - pad.b)

  // Break the line where a day is missing (the title dropped out).
  const segments = $derived.by(() => {
    const segs = []
    let cur = []
    for (let i = 0; i < points.length; i++) {
      const p = points[i]
      if (i > 0 && Date.parse(p.date) - Date.parse(points[i - 1].date) > 86400000) {
        segs.push(cur)
        cur = []
      }
      cur.push(p)
    }
    if (cur.length) segs.push(cur)
    return segs
  })
</script>

<svg viewBox="0 0 {width} {height}" width="100%" role="img" aria-label="Rank history">
  {#each [1, 5, 10] as r (r)}
    <line x1={pad.l} x2={width - pad.r} y1={y(r)} y2={y(r)} stroke="var(--border)" stroke-dasharray="3 3" />
    <text x={pad.l - 6} y={y(r) + 4} text-anchor="end" font-size="10" fill="var(--muted)">#{r}</text>
  {/each}
  {#if points.length}
    <text x={pad.l} y={height - 6} font-size="10" fill="var(--muted)">{points[0].date}</text>
    <text x={width - pad.r} y={height - 6} font-size="10" fill="var(--muted)" text-anchor="end">{points[points.length - 1].date}</text>
  {/if}
  {#each segments as seg, i (i)}
    <polyline
      fill="none"
      stroke="var(--accent)"
      stroke-width="2"
      stroke-linejoin="round"
      points={seg.map((p) => `${x(p.date)},${y(p.rank)}`).join(' ')}
    />
  {/each}
  {#each points as p (p.date)}
    <circle cx={x(p.date)} cy={y(p.rank)} r="3" fill="var(--accent)"><title>{p.date}: #{p.rank}</title></circle>
  {/each}
</svg>
