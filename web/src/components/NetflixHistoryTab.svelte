<script>
  // Netflix's official weekly Top 10: of one Netflix title (netflixTitle,
  // from its drawer) or of every Netflix title mapped to a TMDB title.
  import { getTmdbNetflix } from '../lib/api.js'
  import { dateOnly, number, seasonLabel, SEASON_SOURCES } from '../lib/format.js'
  import { countryName } from '../lib/constants.js'
  import Icon from './Icon.svelte'

  let { netflixTitle = null, tmdbKind = '', tmdbId = '' } = $props()

  let loaded = $state(null) // {key, data} for the TMDB title
  let error = $state('')

  $effect(() => {
    const k = `${tmdbKind}:${tmdbId}`
    if (netflixTitle || !tmdbId) return
    loaded = null
    error = ''
    let cancelled = false
    getTmdbNetflix(tmdbKind, tmdbId)
      .then((d) => { if (!cancelled) loaded = { key: k, data: d } })
      .catch((e) => {
        if (cancelled) return
        if (e.status === 404) loaded = { key: k, data: null }
        else error = e.message
      })
    return () => { cancelled = true }
  })

  // Both sources in one shape: summary, global weeks, per-country summary, seasons.
  const view = $derived.by(() => {
    if (netflixTitle) {
      const t = netflixTitle
      return {
        weeks: t.weeks_in_global_top10, best: t.best_global_rank,
        views: t.global_weeks.reduce((s, w) => s + (w.views ?? 0), 0),
        hours: t.global_weeks.reduce((s, w) => s + (w.hours_viewed ?? 0), 0),
        global: t.global_weeks, countries: t.countries, seasons: null, showTitles: false,
        tv: t.kind === 'tv_show' && t.tmdb_kind !== 'movie',
      }
    }
    const d = loaded?.data
    if (!d) return null
    const byCountry = new Map()
    for (const w of d.country_weeks ?? []) {
      const c = byCountry.get(w.country) ?? { country: w.country, weeks: new Set(), best_rank: w.rank, first_week: w.week, last_week: w.week }
      c.weeks.add(w.week)
      c.best_rank = Math.min(c.best_rank, w.rank)
      if (w.week < c.first_week) c.first_week = w.week
      if (w.week > c.last_week) c.last_week = w.week
      byCountry.set(w.country, c)
    }
    return {
      weeks: d.weeks_in_global_top10, best: d.best_global_rank, views: d.total_views, hours: d.total_hours_viewed,
      global: [...(d.global ?? [])].sort((a, b) => (a.week < b.week ? 1 : a.week > b.week ? -1 : a.rank - b.rank)),
      countries: [...byCountry.values()].map((c) => ({ ...c, weeks: c.weeks.size })).sort((a, b) => b.weeks - a.weeks),
      seasons: d.seasons?.length ? d.seasons : null,
      showTitles: new Set((d.global ?? []).map((w) => w.show_title)).size > 1,
      tv: tmdbKind === 'tv',
    }
  })
</script>

{#if !netflixTitle && !tmdbId}
  <div class="empty"><div class="empty-title">Not mapped to TMDB</div>Netflix's Top 10 titles are found through the TMDB title. Map this title first.</div>
{:else if error}
  <div class="alert alert-error"><Icon name="alert" />{error}</div>
{:else if !netflixTitle && loaded?.key !== `${tmdbKind}:${tmdbId}`}
  <div class="skeleton" style="height:140px"></div>
{:else if !view}
  <div class="empty"><div class="empty-title">Not in Netflix's Top 10</div>No Netflix Top 10 title is mapped to this TMDB title.</div>
{:else}
  <div class="summary-strip">
    <div><div class="summary-label">Weeks in global Top 10</div><div class="summary-value">{view.weeks}</div></div>
    <div><div class="summary-label">Best global rank</div><div class="summary-value">{view.best ? `#${view.best}` : '—'}</div></div>
    <div><div class="summary-label">Views</div><div class="summary-value">{number(view.views)}</div></div>
    <div><div class="summary-label">Hours viewed</div><div class="summary-value">{number(view.hours)}</div></div>
  </div>

  {#if view.seasons}
    <div class="section-title">Seasons</div>
    <div class="card table-wrap">
      <table class="table">
        <thead><tr><th>Season</th><th class="right">Weeks</th><th class="right">Best</th><th class="right">Views</th><th>Charted</th></tr></thead>
        <tbody>
          {#each view.seasons as s (s.season_title)}
            <tr>
              <td>{#if s.season_number != null}<span class="badge" title={SEASON_SOURCES[s.season_source]?.hint}>{seasonLabel(s.season_number)}</span> {:else}<span class="badge badge-warning">?</span> {/if}<span class="small">{s.season_title}</span></td>
              <td class="right">{s.weeks_in_global_top10 || '—'}</td>
              <td class="right">{s.best_global_rank ? `#${s.best_global_rank}` : '—'}</td>
              <td class="right">{number(s.total_views)}</td>
              <td class="small nowrap">{dateOnly(s.first_week)}{s.last_week !== s.first_week ? ` – ${dateOnly(s.last_week)}` : ''}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  <div class="section-title">Global weekly Top 10</div>
  <div class="card table-wrap">
    <table class="table">
      <thead><tr><th>Week ending</th>{#if view.showTitles}<th>Title</th>{/if}<th>List</th><th class="right">#</th>{#if view.tv}<th>Season</th>{/if}<th class="right">Views</th><th class="right">Hours</th><th class="right">Weeks in</th></tr></thead>
      <tbody>
        {#each view.global as w (w.week + w.category + w.rank)}
          <tr>
            <td class="nowrap">{dateOnly(w.week)}</td>
            {#if view.showTitles}<td class="small">{w.show_title}</td>{/if}
            <td class="small nowrap">{w.category}</td>
            <td class="right mono">{w.rank}</td>
            {#if view.tv}<td class="small">{#if w.season_number != null}<span class="badge">{seasonLabel(w.season_number)}</span> {/if}<span class="muted">{w.season_title ?? ''}</span></td>{/if}
            <td class="right">{number(w.views)}</td>
            <td class="right">{number(w.hours_viewed)}</td>
            <td class="right">{w.cumulative_weeks ?? ''}</td>
          </tr>
        {:else}
          <tr><td colspan="8"><div class="empty small">Not in the global Top 10 (only in country lists or the most-popular list).</div></td></tr>
        {/each}
      </tbody>
    </table>
  </div>

  <div class="section-title">Countries <span class="muted small">(imported countries only)</span></div>
  <div class="card table-wrap">
    <table class="table">
      <thead><tr><th>Country</th><th class="right">Weeks</th><th class="right">Best</th><th>First</th><th>Last</th></tr></thead>
      <tbody>
        {#each view.countries as c (c.country)}
          <tr><td>{countryName(c.country)}</td><td class="right">{c.weeks}</td><td class="right">#{c.best_rank}</td><td class="nowrap">{dateOnly(c.first_week)}</td><td class="nowrap">{dateOnly(c.last_week)}</td></tr>
        {:else}
          <tr><td colspan="5"><div class="empty small">No country weeks imported.</div></td></tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}
