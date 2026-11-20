<!-- A release's spine, for the shelf view: your spine photo when there is one, otherwise a
     spine made from the cover (blurred, so it reads as the case's colour) with the format's
     band at the top and the title running up it. Thickness follows the number of discs; a
     box set is a wide block. -->
<script>
  import { caseOf, coverOf, imageOf, isBoxSet } from '../lib/discs.js'
  import { posterSrc } from '../lib/rows.js'

  let { release, height = 220 } = $props()
  const look = $derived(caseOf(release.media_format))
  const spine = $derived(imageOf(release, 'spine'))
  const cover = $derived(coverOf(release))
  const art = $derived(cover?.src ?? (cover?.poster ? posterSrc(cover.poster, 'w185') : null))
  const box = $derived(isBoxSet(release))
  // A keepcase is ~14 mm deep against 170 mm tall; multi-disc cases grow, box sets more.
  const width = $derived(Math.round(height * (box ? 0.2 + Math.min(release.parts.length || release.disc_count, 8) * 0.025 : 0.085 + Math.min(Math.max(release.disc_count - 1, 0), 3) * 0.02)))
</script>

<span class="spine" class:box style:height="{height * (look.ratio < 0.75 ? 1.1 : 1)}px" style:width="{width}px" style:--band={look.band} style:--band-text={look.text}>
  {#if spine}
    <img class="photo" src={spine.thumb_url} alt="" loading="lazy" />
  {:else}
    {#if art}<span class="tint" style:background-image="url({art})"></span>{/if}
    <span class="band" aria-hidden="true">{look.mark ? look.mark.replace('ULTRA HD', 'UHD').replace('BLU-RAY', 'BD') : ''}</span>
    <span class="title">{release.title}</span>
  {/if}
</span>

<style>
  .spine {
    position: relative; display: flex; flex-direction: column; align-items: center; overflow: hidden; flex: none;
    border-radius: 3px; background: var(--band); color: #fff;
    box-shadow: inset 2px 0 0 rgb(255 255 255 / 0.08), inset -2px 0 0 rgb(0 0 0 / 0.35), 0 1px 0 rgb(0 0 0 / 0.4);
  }
  .photo { width: 100%; height: 100%; object-fit: cover; }
  .tint { position: absolute; inset: -12px; background-size: cover; background-position: center; filter: blur(10px) saturate(1.3) brightness(0.55); transform: scale(1.2); }
  .band { position: relative; z-index: 1; flex: none; width: 100%; padding: 4px 0; background: var(--band); color: var(--band-text); font: 800 8px/1 var(--font-sans); letter-spacing: 0.08em; text-align: center; white-space: nowrap; overflow: hidden; }
  .title {
    position: relative; z-index: 1; flex: 1; min-height: 0; overflow: hidden; padding: 8px 0;
    writing-mode: vertical-rl; text-overflow: ellipsis; white-space: nowrap;
    font: 700 13px/1 var(--font-display); text-shadow: 0 1px 2px rgb(0 0 0 / 0.6);
  }
  .box .title { font-size: 15px; }
</style>
