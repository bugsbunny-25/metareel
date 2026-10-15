<script>
  // Tiny trend line for KPI tiles. values: number[] (oldest first).
  let { values, color = 'var(--c1)', height = 28 } = $props()

  const W = 100
  const pts = $derived.by(() => {
    const max = Math.max(...values)
    const min = Math.min(...values)
    const span = max - min || 1
    return values.map((v, i) => [(i / (values.length - 1)) * W, height - 2 - ((v - min) / span) * (height - 4)])
  })
  const line = $derived(pts.map((p) => p.join(',')).join(' '))
  const area = $derived(`0,${height} ${line} ${W},${height}`)
</script>

<svg viewBox="0 0 {W} {height}" preserveAspectRatio="none" width="100%" {height} aria-hidden="true" style="display:block">
  <polygon points={area} fill={color} opacity="0.1" />
  <polyline points={line} fill="none" stroke={color} stroke-width="1.6" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke" />
</svg>
