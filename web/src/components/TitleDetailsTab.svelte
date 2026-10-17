<script>
  // A TMDB title's details: TMDB metadata, chart performance across every
  // FlixPatrol title mapped to it, and where to watch it.
  import { getTitleOverview } from '../lib/api.js'
  import { dateOnly, relativeTime, number } from '../lib/format.js'
  import { providerLabel, countryName } from '../lib/constants.js'
  import Icon from './Icon.svelte'

  let { tmdbKind = '', tmdbId = '' } = $props()

  let overview = $state(null)
  let error = $state('')
  let country = $state('US')

  $effect(() => {
    const kind = tmdbKind
    const id = tmdbId
    overview = null
    error = ''
    if (!id) return
    let cancelled = false
    getTitleOverview(kind, id, { include: 'metadata,stats,availability' })
      .then((o) => {
        if (cancelled) return
        overview = o
        const countries = Object.keys(o.availability?.countries ?? {})
        if (countries.length && !countries.includes(country)) country = countries.sort()[0]
      })
      .catch((e) => { if (!cancelled) error = e.message })
    return () => { cancelled = true }
  })
</script>

{#if !tmdbId}
  <div class="empty"><div class="empty-title">Not mapped to TMDB</div>Details come from the TMDB title. Map this title first.</div>
{:else if error}
  <div class="alert alert-error"><Icon name="alert" />{error}</div>
{:else if !overview}
  <div class="skeleton" style="height:160px"></div>
{:else}
  {@const m = overview.metadata}
  {#if m}
    <div class="row" style="align-items:flex-start;gap:14px">
      {#if m.poster_path}<img src="https://image.tmdb.org/t/p/w154{m.poster_path}" alt="Poster of {m.title}" width="92" style="border-radius:6px;flex:none" />{/if}
      <div class="grow">
        <div class="cell-title">{m.title}{m.original_title && m.original_title !== m.title ? ` (${m.original_title})` : ''}</div>
        <div class="small muted">
          {m.release_date ? dateOnly(m.release_date) : 'No release date'}{m.runtime ? ` · ${m.runtime} min` : ''}{m.number_of_seasons ? ` · ${m.number_of_seasons} season(s)` : ''}{m.status ? ` · ${m.status}` : ''}{m.original_language ? ` · ${m.original_language}` : ''}
        </div>
        <div class="row" style="flex-wrap:wrap;gap:4px;margin:6px 0">{#each m.genres as g (g)}<span class="chip">{g}</span>{/each}</div>
        {#if m.overview}<p class="small" style="margin:0">{m.overview}</p>{/if}
        <div class="small muted" style="margin-top:6px">
          {[m.networks?.length ? `Networks: ${m.networks.join(', ')}` : '', m.production_companies?.length ? `Studios: ${m.production_companies.slice(0, 3).join(', ')}` : '', m.origin_countries?.length ? `Origin: ${m.origin_countries.join(', ')}` : ''].filter(Boolean).join(' · ')}
        </div>
        {#if m.details_fetched_at}<div class="small muted" style="margin-top:6px">TMDB data {relativeTime(m.details_fetched_at)}</div>{/if}
      </div>
    </div>
  {:else}
    <div class="alert alert-warning small"><Icon name="alert" size={14} />No TMDB metadata yet — it is fetched by the "TMDB metadata + watch providers" job (needs TMDB_API_KEY).</div>
  {/if}

  {#if overview.stats}
    {@const st = overview.stats}
    <div class="section-title" style="margin-top:18px">Chart performance <span class="muted small">(every FlixPatrol title mapped to this TMDB title)</span></div>
    <div class="grid" style="grid-template-columns:repeat(4,1fr)">
      <div class="card stat"><div class="stat-label">Points</div><div class="stat-value">{number(st.total_points)}</div><div class="stat-foot">#1 = 10 … #10 = 1</div></div>
      <div class="card stat"><div class="stat-label">Days on charts</div><div class="stat-value">{st.days_on_charts}</div><div class="stat-foot">longest run {st.longest_streak_days}d</div></div>
      <div class="card stat"><div class="stat-label">Peak</div><div class="stat-value">#{st.peak_rank ?? '—'}</div><div class="stat-foot">{st.days_at_number_1} day(s) at #1</div></div>
      <div class="card stat"><div class="stat-label">Countries</div><div class="stat-value">{st.countries.length}</div><div class="stat-foot">{st.providers.map(providerLabel).join(', ')}</div></div>
    </div>
    {#if st.debut}<p class="small muted">Debuted {dateOnly(st.debut.date)} at #{st.debut.rank} on {providerLabel(st.debut.provider)} {countryName(st.debut.country)}{#if st.spread.length > 1}; reached {st.spread.slice(1).map((x) => `${x.country} +${x.days_after_debut}d`).join(', ')}{/if}.</p>{/if}
  {/if}

  <div class="section-title row">Where to watch
    {#if Object.keys(overview.availability?.countries ?? {}).length}
      <select class="select" style="height:26px" bind:value={country} aria-label="Country">
        {#each Object.keys(overview.availability.countries).sort() as c (c)}<option value={c}>{countryName(c)}</option>{/each}
      </select>
    {/if}
  </div>
  {#if overview.availability?.countries?.[country]}
    {@const offers = overview.availability.countries[country]}
    <dl class="kv">
      {#each [['flatrate', 'Stream'], ['free', 'Free'], ['ads', 'With ads'], ['rent', 'Rent'], ['buy', 'Buy']] as [k, label] (k)}
        {#if offers[k].length}<dt>{label}</dt><dd class="small">{#each offers[k] as p, i (p.provider_id)}{#if i}, {/if}{#if p.url}<a href={p.url} target="_blank" rel="noopener noreferrer">{p.provider_name}</a>{:else}{p.provider_name}{/if}{/each}</dd>{/if}
      {/each}
    </dl>
    <p class="field-hint">{overview.availability.attribution}{#if offers.link} · <a href={offers.link} target="_blank" rel="noopener">TMDB watch page</a>{/if}</p>
  {:else}
    <p class="muted small">No watch providers known{overview.availability ? ` in ${countryName(country)}` : ' yet'}.</p>
  {/if}
{/if}
