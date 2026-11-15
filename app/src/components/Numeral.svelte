<!-- A chart rank numeral (Home's Top 10 rail, the Charts table): a heavy digit filled with the
     canvas, its outline a gradient lit from the top, clipped to its 0.78em line. It sizes with
     the font-size set on it (class), so media and container queries can shrink it.
     weight: the outline in hundredths of an em. -->
<script>
  let { value, mine = false, weight = 2, label = null, class: cls = '' } = $props()

  const uid = $props.id()
  const digits = $derived(String(value).length)
  // In a 100-unit em: the glyphs' advance (tightened by the letter-spacing), and the 0.78em line.
  const w = $derived(Math.ceil(100 * (0.52 * digits - 0.06 * (digits - 1))) + 4)
  const h = 80
</script>

<svg
  class="numeral {cls}"
  viewBox="0 0 {w} {h}"
  style:width="{w / 100}em"
  style:height="{h / 100}em"
  role={label ? 'img' : undefined}
  aria-label={label}
  aria-hidden={label ? undefined : 'true'}
>
  <defs>
    <linearGradient id="n{uid}" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" style:stop-color={mine ? 'var(--numeral-yours-top)' : 'var(--numeral-top)'} />
      <stop offset="0.55" style:stop-color={mine ? 'var(--numeral-yours-mid)' : 'var(--numeral-mid)'} />
      <stop offset="1" style:stop-color={mine ? 'var(--numeral-yours-bottom)' : 'var(--numeral-bottom)'} />
    </linearGradient>
  </defs>
  <text x={w - 2} y={h - 3} text-anchor="end" stroke="url(#n{uid})" stroke-width={weight}>{value}</text>
</svg>

<style>
  .numeral { display: block; flex: none; overflow: hidden; }
  text { fill: var(--bg-canvas); font: 800 100px var(--font-sans); letter-spacing: -6px; paint-order: normal; }
</style>
