<script>
  // Titles rising in several countries at once (GET /breakouts).
  import { getBreakouts } from '../lib/api.js'
  import { route, setQuery, href } from '../lib/router.svelte.js'
  import { dateOnly, number } from '../lib/format.js'
  import { providerLabel, countryName } from '../lib/constants.js'
  import Icon from './Icon.svelte'
  import KindBadge from './KindBadge.svelte'
  import InfoTip from './InfoTip.svelte'

  const date = $derived(route.query.date ?? '')
  const windowDays = $derived(route.query.window ?? '3')
  const minCountries = $derived(route.query.min_countries ?? '2')
  const category = $derived(route.query.category ?? '')

  let data = $state(null)
  let error = $state('')
  let loading = $state(false)

  $effect(() => {
    const q = { date, window: windowDays, min_countries: minCountries, category, limit: 100 }
    loading = true
    let cancelled = false
    getBreakouts(q)
      .then((d) => { if (!cancelled) { data = d; error = '' } })
      .catch((e) => { if (!cancelled) { data = null; error = e.message } })
      .finally(() => { if (!cancelled) loading = false })
    return () => { cancelled = true }
  })

  const set = (patch) => setQuery(patch)
  const titleHref = (b) => (b.tmdb_id ? href('/mapping', { open: `${b.kind === 'tv_show' ? 'tv' : 'movie'}:${b.tmdb_id}` }) : href('/titles', { open: b.title_id }))
  const move = (c) => (c.movement === 'entered' ? `entered at #${c.rank}` : `#${c.base_rank} → #${c.rank}`)
</script>

<div class="toolbar">
  <div class="segmented" role="group" aria-label="Window">
    {#each [['1', '1 day'], ['3', '3 days'], ['7', '7 days'], ['14', '14 days']] as [v, l] (v)}<button class:active={windowDays === v} onclick={() => set({ window: v === '3' ? '' : v })}>{l}</button>{/each}
  </div>
  <div class="segmented" role="group" aria-label="Countries">
    {#each [['2', '2+ countries'], ['3', '3+'], ['1', 'Any']] as [v, l] (v)}<button class:active={minCountries === v} onclick={() => set({ min_countries: v === '2' ? '' : v })}>{l}</button>{/each}
  </div>
  <div class="segmented" role="group" aria-label="Category">
    {#each [['', 'All'], ['movies', 'Movies'], ['tv_shows', 'TV']] as [v, l] (v)}<button class:active={category === v} onclick={() => set({ category: v })}>{l}</button>{/each}
  </div>
  <input class="input" type="date" value={date} onchange={(e) => set({ date: e.currentTarget.value })} aria-label="Date" title="Date (default: the current chart date)" />
  <span class="spacer"></span>
  {#if loading}<span class="spinner"></span>{/if}
</div>

{#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

{#if !data && loading}
  <div class="skeleton" style="height:240px"></div>
{:else if data}
  <p class="small muted">
    {dateOnly(data.date)} against {data.window} day{data.window === 1 ? '' : 's'} earlier · rising = entered a chart or climbed {data.min_climb}+ places
    <InfoTip text="Each chart's latest date is compared with its chart about `window` days before. A title rises on a chart when it entered it or climbed min_climb places; per country its best chart counts. Webhooks can subscribe to title.breakout." />
    · countries compared: {data.countries_checked.map(countryName).join(', ') || 'none'}
  </p>
  <div class="card">
    <div class="table-wrap">
      <table class="table">
        <thead><tr><th>Title</th><th>Kind</th><th class="right">Rising in</th><th class="right">Score</th><th>How it rose</th><th>Charting in</th></tr></thead>
        <tbody>
          {#each data.items as b (b.kind + (b.tmdb_id ?? b.title_id))}
            <tr>
              <td><a class="cell-title" style="color:inherit" href={titleHref(b)}>{b.name}</a></td>
              <td><KindBadge kind={b.kind} /></td>
              <td class="right">{b.rising_countries}</td>
              <td class="right" title="Chart points gained (rank 1 = 10 … rank 10 = 1)">{number(b.score)}</td>
              <td class="small">
                {#each b.countries as c (c.country)}<div><span class="badge">{c.country}</span> {move(c)} <span class="muted">· {providerLabel(c.provider)}</span></div>{/each}
              </td>
              <td class="small">{b.charting_countries.join(', ')}</td>
            </tr>
          {:else}
            <tr><td colspan="6"><div class="empty">No title is rising in {data.min_countries}+ countries at once.</div></td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
{/if}
