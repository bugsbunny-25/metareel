<script>
  // Stacked column chart with a hover tooltip.
  // data: [{ label, ...values }] in display order; series: [{ key, label, color }].
  // onselect(row) makes columns clickable.
  let { data, series, height = 180, formatLabel = (l) => l, labelEvery = 1, onselect = null, ariaLabel = 'Column chart' } = $props()

  let width = $state(0)
  let hover = $state(null) // index

  const pad = { l: 34, r: 8, t: 8, b: 22 }
  const total = (row) => series.reduce((s, x) => s + (row[x.key] ?? 0), 0)
  const yMax = $derived.by(() => {
    const m = Math.max(1, ...data.map(total))
    const step = m <= 5 ? 1 : m <= 10 ? 2 : m <= 25 ? 5 : m <= 50 ? 10 : Math.ceil(m / 5 / 10) * 10
    return Math.ceil(m / step) * step
  })
  const ticks = $derived([0, yMax / 2, yMax])
  const plotW = $derived(Math.max(0, width - pad.l - pad.r))
  const plotH = $derived(height - pad.t - pad.b)
  const band = $derived(data.length ? plotW / data.length : 0)
  const barW = $derived(Math.max(2, Math.min(28, band * 0.62)))
  const y = (v) => pad.t + plotH - (v / yMax) * plotH

  function stack(row) {
    let acc = 0
    return series.map((s) => {
      const v = row[s.key] ?? 0
      const seg = { key: s.key, color: s.color, y0: acc, y1: acc + v }
      acc += v
      return seg
    }).filter((s) => s.y1 > s.y0)
  }
</script>

<div style="position:relative" bind:clientWidth={width}>
  {#if width}
    <svg class="chart" {width} {height} role="img" aria-label={ariaLabel}>
      {#each ticks as t (t)}
        <line class="gridline" x1={pad.l} x2={width - pad.r} y1={y(t)} y2={y(t)} />
        <text x={pad.l - 6} y={y(t) + 3.5} text-anchor="end">{Math.round(t)}</text>
      {/each}
      {#each data as row, i (row.label)}
        {@const cx = pad.l + band * i + band / 2}
        {#each stack(row) as s (s.key)}
          <rect x={cx - barW / 2} y={y(s.y1)} width={barW} height={Math.max(0, y(s.y0) - y(s.y1))} fill={s.color} rx="2" opacity={hover === null || hover === i ? 1 : 0.45} />
        {/each}
        {#if i % labelEvery === 0 || i === data.length - 1}
          <text x={cx} y={height - 6} text-anchor="middle">{formatLabel(row.label)}</text>
        {/if}
        <rect
          class="hit"
          x={pad.l + band * i}
          y={pad.t}
          width={band}
          height={plotH}
          role="presentation"
          style:cursor={onselect ? 'pointer' : 'default'}
          onmouseenter={() => (hover = i)}
          onmouseleave={() => (hover = null)}
          onclick={() => onselect?.(row)}
        />
      {/each}
    </svg>
    {#if hover !== null}
      {@const row = data[hover]}
      <div class="chart-tip" style="left:{Math.min(Math.max(pad.l + band * hover + band / 2, 80), width - 80)}px;top:{Math.max(y(total(row)), 104)}px">
        <div class="tip-title">{row.title ?? formatLabel(row.label)}</div>
        {#each series as s (s.key)}
          {#if row[s.key]}<div class="tip-row"><span class="swatch" style="background:{s.color}"></span><span>{s.label}</span><span>{row[s.key]}</span></div>{/if}
        {/each}
        {#if !total(row)}<div class="muted">No runs</div>{/if}
      </div>
    {/if}
  {:else}
    <div style="height:{height}px"></div>
  {/if}
</div>
