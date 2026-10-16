<script>
  // FlixPatrol daily Top 10 history: of one FlixPatrol title (titleId, also
  // while unmapped) or of every FlixPatrol title mapped to a TMDB title.
  import { getTitleRankings, getTmdbRankings } from '../lib/api.js'
  import { dateOnly, seasonLabel } from '../lib/format.js'
  import { providerLabel, countryName } from '../lib/constants.js'
  import RankChart from './RankChart.svelte'

  let { titleId = 0, tmdbKind = '', tmdbId = '' } = $props()

  let data = $state(null)
  let chartKey = $state('')

  const key = (c) => `${c.country}/${c.provider}/${c.category}`

  $effect(() => {
    const load = titleId ? getTitleRankings(titleId) : tmdbId ? getTmdbRankings(tmdbKind, tmdbId) : null
    data = null
    if (!load) return
    let cancelled = false
    load
      .then((d) => { if (!cancelled) { data = d; chartKey = d.charts[0] ? key(d.charts[0]) : '' } })
      .catch((e) => { if (!cancelled) data = { charts: [], rankings: [], error: e.status === 404 ? '' : e.message } })
    return () => { cancelled = true }
  })

  const tv = $derived(data?.rankings.some((r) => r.category === 'tv_shows'))
  const slugs = $derived(data ? [...new Set(data.rankings.map((r) => r.slug).filter(Boolean))] : [])
  // Distinct seasons on a chart, in order of first appearance.
  const chartSeasons = (c) => [...new Set(data.rankings.filter((r) => key(r) === key(c) && r.season_number != null).map((r) => r.season_number))]
  const points = $derived(data ? data.rankings.filter((r) => key(r) === chartKey).map((r) => ({ date: r.date, rank: r.rank })) : [])
</script>

{#if !titleId && !tmdbId}
  <div class="empty"><div class="empty-title">Not mapped to TMDB</div>FlixPatrol's charts are found through the TMDB title. Map this title first.</div>
{:else if !data}
  <div class="skeleton" style="height:140px"></div>
{:else if data.error}
  <div class="alert alert-error">{data.error}</div>
{:else if data.rankings.length === 0}
  <div class="empty"><div class="empty-title">No Top 10 history</div>No FlixPatrol chart has ranked {titleId ? 'this title' : 'a title mapped here'}.</div>
{:else}
  {#if slugs.length > 1}<p class="small muted" style="margin-top:0">From {slugs.length} FlixPatrol titles: <span class="mono">{slugs.join(', ')}</span></p>{/if}
  <div class="table-wrap card">
    <table class="table">
      <thead><tr><th>Chart</th><th class="right">Days</th><th class="right">Best</th><th>First</th><th>Last</th><th class="right">Last rank</th>{#if tv}<th title="Seasons charting on this chart (latest last)">Seasons</th>{/if}</tr></thead>
      <tbody>
        {#each data.charts as c (key(c))}
          <tr class="clickable" class:selected={chartKey === key(c)} onclick={() => (chartKey = key(c))}>
            <td>{providerLabel(c.provider)} · {countryName(c.country)} <span class="muted small">({c.category === 'movies' ? 'Movies' : 'TV'})</span></td>
            <td class="right">{c.days_in_top10}</td>
            <td class="right">#{c.best_rank}</td>
            <td class="nowrap">{dateOnly(c.first_date)}</td>
            <td class="nowrap">{dateOnly(c.last_date)}</td>
            <td class="right">#{c.last_rank}</td>
            {#if tv}<td class="small nowrap">{chartSeasons(c).map(seasonLabel).join(', ') || '—'}</td>{/if}
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  <div class="section-title">Rank over time · {chartKey}</div>
  <div class="card card-body"><RankChart points={points} /></div>
{/if}
