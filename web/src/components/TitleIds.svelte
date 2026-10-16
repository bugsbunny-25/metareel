<script>
  // Every ID of a TMDB title, with where each came from. The same list in
  // every drawer (FlixPatrol title, Netflix title, TMDB mapping): IDs belong
  // to the TMDB title, so all its titles show the same ones. ownNetflixId is
  // a Netflix title's own ID when it differs (e.g. a special mapped to its show).
  import { href } from '../lib/router.svelte.js'
  import { links, ID_SOURCES } from '../lib/format.js'
  import IdCell from './IdCell.svelte'

  let { ids = null, tmdbKind = '', tmdbId = '', ownNetflixId = '', showTitles = false, hint = true, mappingLink = true } = $props()

  const ROWS = [
    { key: 'imdb', field: 'imdb_id', label: 'IMDb', url: links.imdb },
    { key: 'wikidata', field: 'wikidata_id', label: 'Wikidata', url: links.wikidata },
    { key: 'rotten_tomatoes', field: 'rotten_tomatoes_id', label: 'Rotten Tomatoes', url: links.rt },
    { key: 'netflix', field: 'netflix_id', label: 'Netflix', url: links.netflix },
    { key: 'justwatch', field: 'justwatch_id', label: 'JustWatch' },
    { key: 'metacritic', field: 'metacritic_id', label: 'Metacritic', url: links.metacritic },
    { key: 'letterboxd', field: 'letterboxd_id', label: 'Letterboxd', url: links.letterboxd },
  ]
  const mappingHref = $derived(tmdbId && mappingLink ? href('/mapping', { open: `${tmdbKind}:${tmdbId}` }) : '')
</script>

{#if !tmdbId}
  <p class="muted small">Not mapped to TMDB, so no IDs yet: they all belong to the TMDB title.</p>
{:else}
  <dl class="kv">
    <dt>TMDB</dt>
    <dd><IdCell value={`${tmdbKind}/${tmdbId}`} url={links.tmdb(tmdbKind, tmdbId)} />
      {#if mappingHref}<a class="small" style="margin-left:6px" href={mappingHref}>TMDB mapping</a>{/if}</dd>
    {#each ROWS as r (r.key)}
      {@const value = ids?.[r.field]}
      {@const src = value ? ids.sources?.[r.key] : ''}
      <dt>{r.label}</dt>
      <dd>
        <IdCell {value} url={r.url?.(value)} label="Unknown" />
        {#if src}<span class="small muted id-source" title={ID_SOURCES[src]?.hint ?? ''}>{ID_SOURCES[src]?.label ?? src}</span>{/if}
        {#if r.key === 'netflix' && ownNetflixId && ownNetflixId !== value}
          <div class="small muted">This Netflix title's own ID: <a class="mono" href={links.netflix(ownNetflixId)} target="_blank" rel="noopener">{ownNetflixId}</a></div>
        {/if}
      </dd>
    {/each}
    {#if showTitles}
      <dt>FlixPatrol slugs</dt>
      <dd class="small">{#each ids?.flixpatrol_slugs ?? [] as s (s)}<div class="mono">{s}</div>{:else}<span class="muted">—</span>{/each}</dd>
      <dt>Netflix Top 10</dt>
      <dd class="small">{#each ids?.netflix_top10_titles ?? [] as n (n)}<div>{n}</div>{:else}<span class="muted">—</span>{/each}</dd>
    {/if}
  </dl>
  {#if hint}
    <p class="field-hint" style="margin-top:8px">These IDs belong to the TMDB title, so every title mapped to it shows the same ones. Each comes from the strongest source that has one: entered by hand, then TMDB, then Wikidata, then an automatic match, then a Netflix title mapped here.</p>
  {/if}
{/if}

<style>
  .id-source { margin-left: 6px; }
</style>
