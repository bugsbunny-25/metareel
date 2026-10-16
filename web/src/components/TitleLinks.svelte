<script>
  // External links of a title for a drawer header: its source page, then
  // its TMDB title's pages (the same set in every drawer).
  import { links } from '../lib/format.js'
  import Icon from './Icon.svelte'

  let { ids = null, tmdbKind = '', tmdbId = '', flixpatrolSlug = '', netflixId = '' } = $props()

  const items = $derived([
    ['FlixPatrol', links.flixpatrol(flixpatrolSlug)],
    ['TMDB', tmdbId ? links.tmdb(tmdbKind, tmdbId) : null],
    ['IMDb', links.imdb(ids?.imdb_id)],
    ['Rotten Tomatoes', links.rt(ids?.rotten_tomatoes_id)],
    ['Netflix', links.netflix(netflixId || ids?.netflix_id)],
    ['Wikidata', links.wikidata(ids?.wikidata_id)],
  ].filter(([, url]) => url))
</script>

{#each items as [label, url] (label)}
  <a href={url} target="_blank" rel="noopener">{label} <Icon name="external" size={11} /></a>
{/each}
