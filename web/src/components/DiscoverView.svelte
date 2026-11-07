<script>
  // Discovery per country (tides): hidden gems, trending elsewhere, and the
  // subscription-rotation advisor.
  import { getHiddenGems, getElsewhere, getRotation } from '../lib/api.js'
  import { route, setQuery, href } from '../lib/router.svelte.js'
  import { number, dateOnly } from '../lib/format.js'
  import { COUNTRIES, countryName, providerLabel } from '../lib/constants.js'
  import Icon from './Icon.svelte'
  import KindBadge from './KindBadge.svelte'
  import InfoTip from './InfoTip.svelte'

  const VIEWS = [
    ['gems', 'Hidden gems', 'Highly rated on IMDb, streamable here, and not in this country’s Top 10 in the last 90 days.'],
    ['elsewhere', 'Trending elsewhere', 'In other countries’ Top 10s this week, but not on any subscription, free or ads service here.'],
    ['rotation', 'Rotation', 'Which services are worth paying for this month: chart points of their titles, upcoming episodes, arrivals and top-rated titles.'],
  ]
  const view = $derived(route.query.view ?? 'gems')
  const country = $derived(route.query.country || 'US')
  const kind = $derived(route.query.kind ?? '')
  const slots = $derived(route.query.slots ?? '2')

  let loaded = $state(null) // { view, data }
  const data = $derived(loaded?.view === view ? loaded.data : null)
  let error = $state('')
  let loading = $state(false)

  $effect(() => {
    const v = view
    const c = country
    const k = kind
    const s = slots
    loading = true
    loaded = null
    let cancelled = false
    const p = v === 'gems' ? getHiddenGems(c, { kind: k, limit: 100 }) : v === 'elsewhere' ? getElsewhere(c, { kind: k, limit: 100 }) : getRotation(c, { slots: s })
    p.then((d) => { if (!cancelled) { loaded = { view: v, data: d }; error = '' } })
      .catch((e) => { if (!cancelled) error = e.message })
      .finally(() => { if (!cancelled) loading = false })
    return () => { cancelled = true }
  })

  const set = (patch) => setQuery(patch)
  const titleHref = (it) => href('/mapping', { open: `${it.kind}:${it.tmdb_id}` })
  const tvKind = (k) => (k === 'tv' ? 'tv_show' : 'movie')
  const REC = { subscribe: 'badge-success', add: 'badge-success', keep: 'badge-accent', pause: 'badge-warning', skip: '' }
</script>

<div class="toolbar">
  <div class="segmented" role="group" aria-label="View">
    {#each VIEWS as [v, l] (v)}<button class:active={view === v} onclick={() => set({ view: v === 'gems' ? '' : v })}>{l}</button>{/each}
  </div>
  <select class="select" value={country} onchange={(e) => set({ country: e.currentTarget.value })} aria-label="Country">
    {#each COUNTRIES as c (c.code)}<option value={c.code}>{c.name}</option>{/each}
  </select>
  {#if view !== 'rotation'}
    <div class="segmented" role="group" aria-label="Kind">
      {#each [['', 'All'], ['movie', 'Movies'], ['tv', 'TV']] as [v, l] (v)}<button class:active={kind === v} onclick={() => set({ kind: v })}>{l}</button>{/each}
    </div>
  {:else}
    <div class="segmented" role="group" aria-label="Services to pay for">
      {#each [['1', '1 service'], ['2', '2'], ['3', '3']] as [v, l] (v)}<button class:active={slots === v} onclick={() => set({ slots: v === '2' ? '' : v })}>{l}</button>{/each}
    </div>
  {/if}
  <span class="spacer"></span>
  {#if loading}<span class="spinner"></span>{/if}
</div>
<p class="small muted">{VIEWS.find((x) => x[0] === view)?.[2]}</p>

{#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

{#if !data && loading}
  <div class="skeleton" style="height:240px"></div>
{:else if data && view === 'gems'}
  <p class="small muted">{number(data.total)} of {number(data.considered)} rated titles streamable in {countryName(data.country)} weren't in its Top 10 from {dateOnly(data.from)} <InfoTip text="Score: the IMDb rating pulled toward 6.5 by 25,000 votes, so ratings from few votes rank lower." /></p>
  <div class="card">
    <div class="table-wrap">
      <table class="table">
        <thead><tr><th>Title</th><th>Kind</th><th class="right">IMDb</th><th class="right">Votes</th><th class="right">Score</th><th>Streaming here</th><th>Charted elsewhere</th></tr></thead>
        <tbody>
          {#each data.items as it (it.kind + it.tmdb_id)}
            <tr>
              <td><a class="cell-title" style="color:inherit" href={titleHref(it)}>{it.title}</a>{#if it.year}<div class="cell-sub">{it.year}{it.genres.length ? ` · ${it.genres.slice(0, 2).join(', ')}` : ''}</div>{/if}</td>
              <td><KindBadge kind={tvKind(it.kind)} /></td>
              <td class="right">{it.imdb_rating}</td>
              <td class="right">{number(it.imdb_votes)}</td>
              <td class="right">{it.score}</td>
              <td class="small">{[...new Set(it.streaming_here.map((o) => o.provider_name))].slice(0, 3).join(', ')}</td>
              <td class="small">{it.charted_in.length ? `${it.charted_in.join(', ')} · ${number(it.points_elsewhere)} pts` : '—'}</td>
            </tr>
          {:else}
            <tr><td colspan="7"><div class="empty">No hidden gems: every highly rated streamable title charted here, or none is rated yet.</div></td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
{:else if data && view === 'elsewhere'}
  <p class="small muted">{dateOnly(data.from)} – {dateOnly(data.to)} · other countries checked: {data.countries_checked.map(countryName).join(', ') || 'none'} · skipped: {data.skipped.available_here} streamable here, {data.skipped.charting_here} charting here, {data.skipped.availability_unknown} without offers yet</p>
  <div class="card">
    <div class="table-wrap">
      <table class="table">
        <thead><tr><th>Title</th><th>Kind</th><th class="right">Points</th><th>Where it charts</th><th>Here</th></tr></thead>
        <tbody>
          {#each data.items as it (it.kind + it.tmdb_id)}
            <tr>
              <td><a class="cell-title" style="color:inherit" href={titleHref(it)}>{it.title}</a>{#if it.year}<div class="cell-sub">{it.year}{it.imdb_rating ? ` · IMDb ${it.imdb_rating}` : ''}</div>{/if}</td>
              <td><KindBadge kind={tvKind(it.kind)} /></td>
              <td class="right">{number(it.points)}</td>
              <td class="small">{#each it.countries as c (c.country)}<div><span class="badge">{c.country}</span> best #{c.best_rank} · {c.providers.map(providerLabel).join(', ')}</div>{/each}</td>
              <td class="small">{it.rent_buy_here.length ? `rent / buy: ${[...new Set(it.rent_buy_here.map((o) => o.provider_name))].slice(0, 2).join(', ')}` : 'not available'}</td>
            </tr>
          {:else}
            <tr><td colspan="5"><div class="empty">Everything charting elsewhere can be streamed here.</div></td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
{:else if data && view === 'rotation'}
  <p class="small muted">Charts {dateOnly(data.past_from)} – {dateOnly(data.past_to)}{data.chart_basis === 'all_countries' ? ' (every country: none stored for this one)' : ''} · episodes until {dateOnly(data.upcoming_to)} <InfoTip text="Score = chart points of the service's titles + 10 per show with episodes coming + 3 per title that arrived + 1 per title rated 7.5+ on IMDb. A service groups its plans and channels." /></p>
  <div class="card">
    <div class="table-wrap">
      <table class="table">
        <thead><tr><th>Service</th><th></th><th class="right">Score</th><th class="right">Titles</th><th class="right">Chart pts</th><th class="right">Episodes soon</th><th class="right">Arrived</th><th class="right">Top rated</th><th>Highlights</th></tr></thead>
        <tbody>
          {#each data.services as s (s.provider_id)}
            <tr>
              <td class="cell-title">{s.provider_name}</td>
              <td><span class="badge {REC[s.recommendation]}">{s.recommendation}</span></td>
              <td class="right">{number(s.score)}</td>
              <td class="right">{number(s.titles)}</td>
              <td class="right">{number(s.chart_points)}</td>
              <td class="right">{s.upcoming_shows}</td>
              <td class="right">{s.arrivals}</td>
              <td class="right">{s.top_rated}</td>
              <td class="small">{s.highlights.slice(0, 3).map((h) => h.title).join(', ')}</td>
            </tr>
          {:else}
            <tr><td colspan="9"><div class="empty">No subscription services with enough titles here.</div></td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
{/if}
