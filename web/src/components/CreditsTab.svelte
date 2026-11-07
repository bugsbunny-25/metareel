<script>
  // A TMDB title's top-billed cast and key crew (tmdb.credits.refresh).
  import { getCredits } from '../lib/api.js'
  import { href } from '../lib/router.svelte.js'
  import Icon from './Icon.svelte'

  let { tmdbKind = '', tmdbId = '' } = $props()

  let data = $state(null)
  let error = $state('')

  $effect(() => {
    const kind = tmdbKind
    const id = tmdbId
    data = null
    error = ''
    if (!id) return
    let cancelled = false
    getCredits(kind, id)
      .then((d) => { if (!cancelled) data = d })
      .catch((e) => { if (!cancelled) error = e.message })
    return () => { cancelled = true }
  })

  // Crew grouped by job, in a fixed order.
  const JOB_ORDER = ['Creator', 'Director', 'Screenplay', 'Writer', 'Teleplay', 'Story', 'Novel', 'Characters', 'Original Music Composer', 'Director of Photography']
  const crewByJob = $derived.by(() => {
    const groups = new Map()
    for (const c of data?.crew ?? []) {
      if (!groups.has(c.job)) groups.set(c.job, [])
      groups.get(c.job).push(c)
    }
    return [...groups].sort((a, b) => JOB_ORDER.indexOf(a[0]) - JOB_ORDER.indexOf(b[0]))
  })
  const personHref = (id) => href('/insights', { tab: 'people', person: id })
</script>

{#if !tmdbId}
  <div class="empty"><div class="empty-title">Not mapped to TMDB</div>Credits come from TMDB. Map this title first.</div>
{:else if error}
  <div class="alert alert-error"><Icon name="alert" />{error}</div>
{:else if !data}
  <div class="skeleton" style="height:160px"></div>
{:else if !data.cast.length && !data.crew.length}
  <div class="empty small">No credits stored yet. The TMDB credits job (Data health) fetches them.</div>
{:else}
  {#if crewByJob.length}
    <div class="section-title" style="margin-top:0">Crew</div>
    {#each crewByJob as [job, people] (job)}
      <div class="row small credit-row">
        <span class="muted credit-job">{job}</span>
        <span class="grow">{#each people as p, i (p.person_id)}{i ? ', ' : ''}<a href={personHref(p.person_id)}>{p.name}</a>{#if p.episodes}<span class="muted"> ({p.episodes} ep.)</span>{/if}{/each}</span>
      </div>
    {/each}
  {/if}
  {#if data.cast.length}
    <div class="section-title">Cast</div>
    {#each data.cast as p (p.person_id)}
      <div class="row small credit-row">
        <a class="credit-job" href={personHref(p.person_id)}>{p.name}</a>
        <span class="grow muted">{p.character || '—'}</span>
        {#if p.episodes}<span class="muted num">{p.episodes} ep.</span>{/if}
      </div>
    {/each}
  {/if}
  <p class="field-hint">From TMDB: the top 25 cast and the directors, writers, creators, composers and cinematographers.</p>
{/if}

<style>
  .credit-row { padding: 4px 0; border-bottom: 1px solid var(--border); gap: 10px; }
  .credit-job { min-width: 170px; }
</style>
