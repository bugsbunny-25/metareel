<!-- A release as its case: the front art (your photo or the lookup's, else the film's TMDB
     poster) under the format's band, in the case's proportions. Steelbooks lose the band
     (they have none) and get a metal edge; a slipcover is a small tag. -->
<script>
  import { caseOf, coverOf, label } from '../lib/discs.js'
  import { posterSrc } from '../lib/rows.js'

  let { release, width = 168, eager = false } = $props()
  const look = $derived(caseOf(release.media_format))
  const cover = $derived(coverOf(release))
  const src = $derived(cover?.src ?? (cover?.poster ? posterSrc(cover.poster, 'w342') : null))
  const steel = $derived(release.packaging === 'steelbook')
  let failed = $state(null)
</script>

<span class="case" class:steel style:width="{width}px" style:--ratio={look.ratio} style:--band={look.band} style:--band-text={look.text}>
  {#if look.mark && !steel}<span class="band">{look.mark}</span>{/if}
  <span class="art">
    {#if src && src !== failed}
      <img {src} alt="" loading={eager ? 'eager' : 'lazy'} decoding="async" onerror={() => (failed = src)} class:poster={!cover?.src} />
    {:else}
      <span class="ph">{release.title}</span>
    {/if}
  </span>
  {#if release.has_slipcover}<span class="tag" title="Has a slipcover">Slip</span>{/if}
  {#if steel}<span class="tag steel-tag" title={label('packaging', 'steelbook')}>Steel</span>{/if}
</span>

<style>
  .case {
    position: relative; display: flex; flex-direction: column; flex: none; max-width: 100%;
    aspect-ratio: var(--ratio); overflow: hidden; border-radius: 6px 9px 9px 6px;
    background: var(--band); box-shadow: var(--shadow-art), inset 0 0 0 1px var(--poster-hairline);
  }
  /* The case's hinge: a darker strip down the left edge. */
  .case::after { content: ''; position: absolute; inset: 0 auto 0 0; width: 4%; background: linear-gradient(90deg, rgb(0 0 0 / 0.35), transparent); pointer-events: none; }
  .band { flex: none; padding: 5px 0 4px; background: var(--band); color: var(--band-text); font: 800 10px/1 var(--font-sans); letter-spacing: 0.14em; text-align: center; }
  .art { position: relative; flex: 1; min-height: 0; background: var(--bg-elevated); }
  img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
  .ph { position: absolute; inset: 0; display: grid; place-items: center; padding: 10px; color: var(--text-muted); font: 600 13px/1.25 var(--font-display); text-align: center; }
  .steel { background: linear-gradient(135deg, #9a9aa2, #e6e6ea 40%, #77777f 60%, #c8c8ce); padding: 3px; }
  .steel .art { border-radius: 4px 6px 6px 4px; overflow: hidden; }
  .tag { position: absolute; right: 6px; bottom: 6px; padding: 3px 6px; border-radius: 5px; background: rgb(0 0 0 / 0.72); color: #fff; font: 700 10px/1 var(--font-sans); letter-spacing: 0.06em; text-transform: uppercase; }
  .steel-tag { right: auto; left: 8px; }
</style>
