<!-- A ratings source's logo in place of its name, from the artwork in src/assets/ratings (IMDb and
     TMDB as SVG, Rotten Tomatoes as PNGs at 2x the sizes used): IMDb's wordmark in gold on dark and black on light; the
     Tomatometer's fresh tomato, rotten splat or Certified Fresh badge; the audience score's full or
     tipped popcorn; TMDB's stacked wordmark. The name is the accessible label and tooltip. -->
<script>
  import imdbGold from '../assets/ratings/imdb-gold.svg'
  import imdbBlack from '../assets/ratings/imdb-black.svg'
  import rtFresh from '../assets/ratings/rt-fresh.png'
  import rtRotten from '../assets/ratings/rt-rotten.png'
  import rtCertified from '../assets/ratings/rt-certified-fresh.png'
  import popcornHot from '../assets/ratings/rt-popcorn-hot.png'
  import popcornStale from '../assets/ratings/rt-popcorn-stale.png'
  import tmdbLogo from '../assets/ratings/tmdb.svg'

  /**
   * source: imdb | rt (Tomatometer) | rt-audience (Popcornmeter) | tmdb.
   * rotten: score under 60 (rt: splat; rt-audience: tipped popcorn). certified: Certified Fresh.
   * dark: sits on a dark plate in both themes (poster badges), so IMDb stays gold.
   */
  let { source, rotten = false, certified = false, dark = false, height = 16, title = '' } = $props()

  const NAMES = $derived({
    imdb: 'IMDb',
    rt: certified ? 'Rotten Tomatoes, Certified Fresh' : rotten ? 'Rotten Tomatoes, rotten' : 'Rotten Tomatoes',
    'rt-audience': 'Rotten Tomatoes audience score',
    tmdb: 'TMDB',
  })
  const label = $derived(title || (NAMES[source] ?? source))
  const rtSrc = $derived(source === 'rt' ? (certified ? rtCertified : rotten ? rtRotten : rtFresh) : rotten ? popcornStale : popcornHot)
</script>

{#if source === 'imdb'}
  <span class="rating-logo imdb" class:dark role="img" aria-label={label} title={label} style:height="{height}px">
    <img class="on-dark" src={imdbGold} alt="" height={height} width={Math.round(height * 2.41)} />
    <img class="on-light" src={imdbBlack} alt="" height={height} width={Math.round(height * 2.41)} />
  </span>
{:else if source === 'rt' || source === 'rt-audience'}
  <img class="rating-logo" src={rtSrc} alt={label} title={label} height={height} width={height} />
{:else if source === 'tmdb'}
  <img class="rating-logo" src={tmdbLogo} alt={label} title={label} height={height} width={Math.round(height * 2.33)} />
{/if}

<style>
  .rating-logo { display: inline-block; flex: none; vertical-align: -0.2em; object-fit: contain; }
  .imdb { line-height: 0; }
  .imdb img { display: block; height: 100%; width: auto; }
  .on-light { display: none !important; }
  :global(:root[data-theme='light']) .imdb:not(.dark) .on-light { display: block !important; }
  :global(:root[data-theme='light']) .imdb:not(.dark) .on-dark { display: none !important; }
</style>
