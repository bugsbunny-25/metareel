<script>
  // Multi-series line chart with a hover crosshair and tooltip.
  // data: [{ label, ...values }] oldest first; series: [{ key, label, color }].
  // percent: values are 0..1 and the axis shows 0–100%.
  let { data, series, height = 200, percent = false, formatLabel = (l) => l, formatValue = null, labelEvery = 1, yMin = null, ariaLabel = 'Line chart' } = $props()

  let width = $state(0)
  let hover = $state(null)

  const pad = { l: 40, r: 12, t: 10, b: 22 }
  const fmt = $derived(formatValue ?? ((v) => (percent ? `${Math.round(v * 100)}%` : Math.round(v).toLocaleString())))
  const domain = $derived.by(() => {
    if (percent) return [yMin ?? 0, 1]
    const vals = data.flatMap((r) => series.map((s) => r[s.key]).filter((v) => v != null))
    const max = Math.max(1, ...vals)
    return [yMin ?? 0, max * 1.08]
  })
  const plotW = $derived(Math.max(0, width - pad.l - pad.r))
  const plotH = $derived(height - pad.t - pad.b)
  const x = (i) => pad.l + (data.length > 1 ? (i / (data.length - 1)) * plotW : plotW / 2)
  const y = (v) => pad.t + plotH - ((v - domain[0]) / (domain[1] - domain[0] || 1)) * plotH
  const ticks = $derived([domain[0], (domain[0] + domain[1]) / 2, domain[1]])
  const path = (key) => {
    let d = ''
    let pen = false
    data.forEach((r, i) => {
      const v = r[key]
      if (v == null) { pen = false; return }
      d += `${pen ? 'L' : 'M'}${x(i)},${y(v)}`
      pen = true
    })
    return d
  }
  const step = $derived(data.length > 1 ? plotW / (data.length - 1) : plotW)
</script>

<div style="position:relative" bind:clientWidth={width}>
  {#if width}
    <svg class="chart" {width} {height} role="img" aria-label={ariaLabel}>
      {#each ticks as t, i (i)}
        <line class="gridline" x1={pad.l} x2={width - pad.r} y1={y(t)} y2={y(t)} />
        <text x={pad.l - 6} y={y(t) + 3.5} text-anchor="end">{fmt(t)}</text>
      {/each}
      {#each data as row, i (row.label)}
        {#if i % labelEvery === 0 || i === data.length - 1}
          <text x={x(i)} y={height - 6} text-anchor={i === 0 ? 'start' : i === data.length - 1 ? 'end' : 'middle'}>{formatLabel(row.label)}</text>
        {/if}
      {/each}
      {#if hover !== null}<line class="axis" x1={x(hover)} x2={x(hover)} y1={pad.t} y2={pad.t + plotH} stroke-dasharray="3 3" />{/if}
      {#each series as s (s.key)}
        <path d={path(s.key)} fill="none" stroke={s.color} stroke-width="2" stroke-linejoin="round" stroke-linecap="round" />
        {#if hover !== null && data[hover][s.key] != null}
          <circle cx={x(hover)} cy={y(data[hover][s.key])} r="3.5" fill="var(--surface)" stroke={s.color} stroke-width="2" />
        {/if}
      {/each}
      {#each data as row, i (row.label)}
        <rect class="hit" x={x(i) - step / 2} y={pad.t} width={step} height={plotH} role="presentation" onmouseenter={() => (hover = i)} onmouseleave={() => (hover = null)} />
      {/each}
    </svg>
    {#if hover !== null}
      {@const row = data[hover]}
      <div class="chart-tip" style="left:{Math.min(Math.max(x(hover), 80), width - 80)}px;top:{pad.t + 4}px;transform:translate(-50%, 0)">
        <div class="tip-title">{row.title ?? formatLabel(row.label)}</div>
        {#each series as s (s.key)}
          {#if row[s.key] != null}<div class="tip-row"><span class="swatch" style="background:{s.color}"></span><span>{s.label}</span><span>{fmt(row[s.key])}</span></div>{/if}
        {/each}
      </div>
    {/if}
  {:else}
    <div style="height:{height}px"></div>
  {/if}
</div>
