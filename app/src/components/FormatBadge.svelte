<!-- A premium format as a badge: its logo on a dark plate (logos are often white, and the
     plate keeps them legible in the light theme too), or its label when it has no logo. -->
<script>
  /** format: { slug, label, logo_url? } */
  let { format, height = 14 } = $props()
  let broken = $state(false)
</script>

{#if format.logo_url && !broken}
  <span class="fb" title={format.label}>
    <img class:mono={format.logo_display === 'mono'} src={format.logo_url} alt={format.label} style:height="{height}px" loading="lazy" onerror={() => (broken = true)} />
  </span>
{:else}
  <span class="fb text">{format.label}</span>
{/if}

<style>
  .fb { display: inline-flex; align-items: center; padding: 3px 6px; border-radius: 5px; background: #161618; vertical-align: middle; }
  .fb img { display: block; max-width: 96px; object-fit: contain; }
  /* logo_display 'mono': a dark logo drawn white so it shows on the plate */
  .fb img.mono { filter: brightness(0) invert(1); }
  .fb.text { padding: 2px 7px; background: var(--bg-elevated); color: var(--text-primary); font-size: 11.5px; font-weight: 600; white-space: nowrap; }
</style>
