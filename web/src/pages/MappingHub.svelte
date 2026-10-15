<script>
  // One Mapping page with a tab per source. The tabs keep their original
  // URLs (/titles, /netflix, /mapping) so existing links still work.
  import { route, href } from '../lib/router.svelte.js'
  import { getStats } from '../lib/api.js'
  import { number } from '../lib/format.js'
  import PageHeader from '../components/PageHeader.svelte'
  import Titles from './Titles.svelte'
  import NetflixTitles from './NetflixTitles.svelte'
  import Mapping from './Mapping.svelte'

  const TABS = [
    { path: '/titles', label: 'FlixPatrol titles', page: Titles },
    { path: '/netflix', label: 'Netflix Top 10 titles', page: NetflixTitles },
    { path: '/mapping', label: 'TMDB mappings', page: Mapping },
  ]
  const tab = $derived(TABS.find((t) => t.path === route.path) ?? TABS[0])
  const Page = $derived(tab.page)

  let stats = $state(null)
  $effect(() => {
    getStats().then((s) => (stats = s)).catch(() => {})
  })
  const coverage = $derived(stats?.titles.total ? Math.round(((stats.titles.total - stats.titles.missing_tmdb) / stats.titles.total) * 100) : null)
</script>

<div class="page">
  <PageHeader
    title="Mapping"
    desc="Links every chart title to its TMDB movie or show, the key for ratings, genres, release dates and every other ID."
    info="FlixPatrol and Netflix name titles their own way. Matching jobs find the TMDB title automatically; anything they can't match with confidence waits here for review."
  >
    {#snippet actions()}
      {#if stats}
        <span class="muted small" title="FlixPatrol titles with a TMDB ID">{coverage}% of {number(stats.titles.total)} FlixPatrol titles mapped</span>
      {/if}
    {/snippet}
  </PageHeader>

  <nav class="page-tabs" aria-label="Mapping sources">
    {#each TABS as t (t.path)}
      <a href={href(t.path)} class:active={tab === t} aria-current={tab === t ? 'page' : undefined}>
        {t.label}
        {#if t.path === '/titles' && stats?.titles.missing_tmdb}<span class="badge badge-warning" title="Without a TMDB ID">{number(stats.titles.missing_tmdb)}</span>{/if}
      </a>
    {/each}
  </nav>

  <Page />
</div>
