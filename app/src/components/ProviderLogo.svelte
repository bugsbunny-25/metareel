<!-- A streaming service's logo (TMDB's provider artwork, from JustWatch) in place of its name. The
     name is the alt text and tooltip; without a logo, or if it fails to load, the name shows in a
     pill instead. With a link, the logo opens the title on that service. -->
<script>
  /** name: provider name. logo: TMDB logo_path (or a full URL). link: where it opens, if anywhere. */
  let { name, logo = null, link = null, size = 36 } = $props()

  let broken = $state(false)
  const src = $derived(!logo ? null : /^https?:/.test(logo) ? logo : `https://image.tmdb.org/t/p/w92${logo}`)
</script>

<svelte:element this={link ? 'a' : 'span'} class="provider" class:text={!src || broken} href={link} target={link ? '_blank' : undefined}
  rel={link ? 'noopener noreferrer' : undefined} title={name} style:--size="{size}px">
  {#if src && !broken}
    <img {src} alt={name} width={size} height={size} loading="lazy" onerror={() => (broken = true)} />
  {:else}
    {name}
  {/if}
</svelte:element>

<style>
  .provider { display: inline-grid; place-items: center; flex: none; border-radius: calc(var(--size) * 0.24); text-decoration: none; }
  img { display: block; width: var(--size); height: var(--size); border-radius: inherit; box-shadow: 0 0 0 1px var(--poster-hairline); }
  .text {
    height: 28px; padding: 0 12px; border-radius: 14px; background: var(--bg-elevated);
    color: var(--text-primary); font: 600 13px/1 var(--font-sans); white-space: nowrap;
  }
  a.provider:hover img { transform: translateY(-1px); }
  a.provider:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 2px; }
</style>
