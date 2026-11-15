<!-- A chart rank numeral (Home's Top 10 rail, the Charts table): Barlow Condensed ExtraBold filled
     with the canvas, its outline a gradient lit from the top, a lone 1 standing on a foot
     (lib/numeral.js has the geometry). It sizes with the font-size set on it (class), so media
     and container queries can scale it; weight is the outline in px at any size.
     The outline is drawn at twice the weight under the fill, so only its outer half shows and
     the foot joins the 1's stem as one shape. -->
<script>
  import { numeralLayout, TRACKING } from '../lib/numeral.js'

  let { value, mine = false, weight = 2, label = null, class: cls = '' } = $props()

  const uid = $props.id()
  const L = $derived(numeralLayout(value))
</script>

<svg
  class="numeral {cls}"
  viewBox="0 0 {L.width} {L.height}"
  style:width="{L.width / 100}em"
  style:height="{L.height / 100}em"
  role={label ? 'img' : undefined}
  aria-label={label}
  aria-hidden={label ? undefined : 'true'}
>
  <defs>
    <linearGradient id="n{uid}" gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="0" y2={L.height}>
      <stop offset="0" style:stop-color={mine ? 'var(--numeral-yours-top)' : 'var(--numeral-top)'} />
      <stop offset="0.55" style:stop-color={mine ? 'var(--numeral-yours-mid)' : 'var(--numeral-mid)'} />
      <stop offset="1" style:stop-color={mine ? 'var(--numeral-yours-bottom)' : 'var(--numeral-bottom)'} />
    </linearGradient>
  </defs>
  {#each ['edge', 'body'] as layer}
    <g class={layer} stroke={layer === 'edge' ? `url(#n${uid})` : 'none'} stroke-width={weight * 2}>
      <text x={L.x} y={L.baseline} letter-spacing={TRACKING}>{value}</text>
      {#if L.foot}<rect x={L.foot.x} y={L.foot.y} width={L.foot.w} height={L.foot.h} />{/if}
    </g>
  {/each}
</svg>

<style>
  .numeral { display: block; flex: none; overflow: visible; }
  text { font: 800 100px 'Barlow Condensed', var(--font-sans); }
  text, rect { fill: var(--bg-canvas); vector-effect: non-scaling-stroke; stroke-linejoin: round; }
</style>
