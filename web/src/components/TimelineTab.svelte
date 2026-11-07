<script>
  // Everything that happened to a TMDB title, newest first (charts,
  // availability, ratings, pageviews, Netflix, release dates).
  import { getTimeline } from '../lib/api.js'
  import { dateOnly, number } from '../lib/format.js'
  import { COUNTRIES } from '../lib/constants.js'
  import Icon from './Icon.svelte'

  let { tmdbKind = '', tmdbId = '' } = $props()

  const GROUPS = {
    '': 'Everything',
    charts: 'Charts',
    availability: 'Availability',
    ratings: 'Ratings',
    pageviews: 'Wikipedia',
    netflix: 'Netflix',
    releases: 'Releases',
  }
  const GROUP_TYPES = {
    charts: 'chart_debut,chart_peak,chart_number_one,chart_reentry',
    availability: 'availability_added,availability_removed',
    ratings: 'rating_changed',
    pageviews: 'pageview_spike',
    netflix: 'netflix_top10_week,netflix_engagement,netflix_premiere',
    releases: 'release,season_premiere',
  }
  const BADGE = {
    chart_debut: ['Debut', 'badge-accent'], chart_peak: ['Peak', 'badge-info'], chart_number_one: ['#1', 'badge-success'],
    chart_reentry: ['Back', 'badge-info'], availability_added: ['Arrived', 'badge-success'], availability_removed: ['Left', 'badge-danger'],
    rating_changed: ['Rating', ''], pageview_spike: ['Wikipedia', 'badge-warning'], netflix_top10_week: ['Netflix Top 10', 'badge-danger'],
    netflix_engagement: ['Netflix views', ''], netflix_premiere: ['Netflix premiere', 'badge-danger'], season_premiere: ['Season', 'badge-accent'],
    release: ['Release', 'badge-accent'],
  }

  let group = $state('')
  let country = $state('')
  let data = $state(null)
  let error = $state('')
  let loading = $state(false)

  $effect(() => {
    const kind = tmdbKind
    const id = tmdbId
    const q = { types: GROUP_TYPES[group] ?? '', country, limit: 300 }
    data = null
    error = ''
    if (!id) return
    loading = true
    let cancelled = false
    getTimeline(kind, id, q)
      .then((d) => { if (!cancelled) data = d })
      .catch((e) => { if (!cancelled) error = e.message })
      .finally(() => { if (!cancelled) loading = false })
    return () => { cancelled = true }
  })
</script>

{#if !tmdbId}
  <div class="empty"><div class="empty-title">Not mapped to TMDB</div>The timeline is built per TMDB title. Map this title first.</div>
{:else}
  <div class="toolbar" style="margin-bottom:10px">
    <select class="select" bind:value={group} aria-label="Events">
      {#each Object.entries(GROUPS) as [v, l] (v)}<option value={v}>{l}{data?.counts && v === '' ? ` (${number(data.total)})` : ''}</option>{/each}
    </select>
    <select class="select" bind:value={country} aria-label="Country" title="Limits charts, availability and Netflix weeks to one country">
      <option value="">All countries</option>
      {#each COUNTRIES as c (c.code)}<option value={c.code}>{c.name}</option>{/each}
    </select>
    <span class="spacer"></span>
    {#if loading}<span class="spinner"></span>{/if}
  </div>
  {#if error}
    <div class="alert alert-error"><Icon name="alert" />{error}</div>
  {:else if !data}
    <div class="skeleton" style="height:160px"></div>
  {:else if !data.events.length}
    <div class="empty small">No events{group ? ' of this kind' : ''} yet.</div>
  {:else}
    {#if data.total > data.events.length}<p class="small muted">Newest {number(data.events.length)} of {number(data.total)} events.</p>{/if}
    <div class="timeline">
      {#each data.events as e, i (i)}
        {@const b = BADGE[e.type] ?? [e.type, '']}
        <div class="timeline-row">
          <div class="timeline-date num small">{dateOnly(e.date)}{#if e.end_date && e.end_date !== e.date}<div class="muted">to {dateOnly(e.end_date)}</div>{/if}</div>
          <span class="badge {b[1]} nowrap">{b[0]}</span>
          <div class="grow small">{e.summary}</div>
        </div>
      {/each}
    </div>
  {/if}
{/if}

<style>
  .timeline-row { display: flex; gap: 10px; align-items: baseline; padding: 6px 0; border-bottom: 1px solid var(--border); }
  .timeline-row:last-child { border-bottom: 0; }
  .timeline-date { min-width: 86px; }
</style>
