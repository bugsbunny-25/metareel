<script>
  import { getLeaderboard, getMovers, getNetflixTop10, getAnalytics } from '../lib/api.js'
  import { route, setQuery, href } from '../lib/router.svelte.js'
  import { number, dateOnly } from '../lib/format.js'
  import { COUNTRIES, PROVIDERS, providerLabel, countryName } from '../lib/constants.js'
  import Icon from '../components/Icon.svelte'
  import KindBadge from '../components/KindBadge.svelte'

  const TABS = [
    ['leaderboard', 'Leaderboard'],
    ['movers', 'Movers'],
    ['netflix', 'Netflix official'],
    ['analytics', 'Analytics'],
  ]
  const tab = $derived(route.query.tab ?? 'leaderboard')
  const country = $derived(route.query.country ?? (tab === 'movers' ? 'US' : ''))
  const provider = $derived(route.query.provider ?? (tab === 'movers' ? 'netflix' : ''))
  const period = $derived(route.query.period ?? 'week')
  const kind = $derived(route.query.kind ?? '')
  const date = $derived(route.query.date ?? '')
  const nfCategory = $derived(route.query.category ?? '')
  const range = $derived(route.query.range ?? '30')

  // Loaded data is tagged with its tab so a tab switch never renders the
  // previous tab's data in the new tab's layout.
  let loaded = $state(null) // { tab, data }
  const data = $derived(loaded?.tab === tab ? loaded.data : null)
  let loading = $state(false)
  let error = $state('')

  function rangeQuery(days) {
    const to = new Date()
    const from = new Date(Date.now() - (Number(days) - 1) * 864e5)
    return { from: from.toISOString().slice(0, 10), to: to.toISOString().slice(0, 10) }
  }

  $effect(() => {
    const t = tab
    const q = { country, provider, period, kind, date, category: nfCategory, range }
    loading = true
    loaded = null
    let cancelled = false
    let p
    if (t === 'leaderboard') p = getLeaderboard({ period: q.period, country: q.country, provider: q.provider, kind: q.kind, date: q.date, limit: 50 })
    else if (t === 'movers') p = getMovers({ country: q.country || 'US', provider: q.provider || 'netflix', date: q.date })
    else if (t === 'netflix') p = getNetflixTop10(q.country, { week: q.date, category: q.category })
    else {
      const r = { ...rangeQuery(q.range), country: q.country, provider: q.provider, kind: q.kind }
      p = Promise.all([
        getAnalytics('genres', r), getAnalytics('release-lag', r), getAnalytics('decay', r),
        getAnalytics('country-similarity', { ...r, country: '' }), getAnalytics('ratings-vs-popularity', r),
      ]).then(([genres, lag, decay, similarity, rvp]) => ({ genres, lag, decay, similarity, rvp }))
    }
    p.then((d) => { if (!cancelled) { loaded = { tab: t, data: d }; error = '' } })
      .catch((e) => { if (!cancelled) { error = e.status === 404 ? '' : e.message; loaded = e.status === 404 ? { tab: t, data: { empty: true } } : null } })
      .finally(() => { if (!cancelled) loading = false })
    return () => { cancelled = true }
  })

  const set = (patch) => setQuery(patch)
  const titleHref = (it) => href('/titles', { q: it.slug, open: it.title_id })
</script>

<div class="page">
  <div class="page-header">
    <div>
      <h1 class="page-title">Insights</h1>
      <p class="page-desc">Points leaderboards (#1 = 10 points … #10 = 1 per chart per day), daily movers, Netflix's official numbers and cross-chart analytics. The same data is on the public API.</p>
    </div>
  </div>

  <div class="tabs" style="margin:0 0 16px;padding:0">
    {#each TABS as [k, label] (k)}
      <button class="tab" class:active={tab === k} onclick={() => setQuery({ tab: k === 'leaderboard' ? '' : k, country: '', provider: '', date: '', category: '' })}>{label}</button>
    {/each}
  </div>

  <div class="toolbar">
    {#if tab === 'leaderboard'}
      <div class="segmented" role="group" aria-label="Period">
        {#each [['day', 'Day'], ['week', 'Week'], ['month', '30 days']] as [v, l] (v)}<button class:active={period === v} onclick={() => set({ period: v === 'week' ? '' : v })}>{l}</button>{/each}
      </div>
    {/if}
    {#if tab === 'analytics'}
      <div class="segmented" role="group" aria-label="Range">
        {#each [['30', '30 days'], ['90', '90 days'], ['365', '1 year']] as [v, l] (v)}<button class:active={range === v} onclick={() => set({ range: v === '30' ? '' : v })}>{l}</button>{/each}
      </div>
    {/if}
    <select class="select" value={country} onchange={(e) => set({ country: e.currentTarget.value })} aria-label="Country">
      {#if tab !== 'movers'}<option value="">{tab === 'netflix' ? 'Global' : 'All countries'}</option>{/if}
      {#each COUNTRIES as c (c.code)}<option value={c.code}>{c.name}</option>{/each}
    </select>
    {#if tab !== 'netflix'}
      <select class="select" value={provider} onchange={(e) => set({ provider: e.currentTarget.value })} aria-label="Provider">
        {#if tab !== 'movers'}<option value="">All providers</option>{/if}
        {#each PROVIDERS as p (p.value)}<option value={p.value}>{p.label}</option>{/each}
      </select>
    {/if}
    {#if tab === 'leaderboard' || tab === 'analytics'}
      <div class="segmented" role="group" aria-label="Kind">
        {#each [['', 'All'], ['movie', 'Movies'], ['tv_show', 'TV']] as [v, l] (v)}<button class:active={kind === v} onclick={() => set({ kind: v })}>{l}</button>{/each}
      </div>
    {/if}
    {#if tab === 'netflix' && !country}
      <select class="select" value={nfCategory} onchange={(e) => set({ category: e.currentTarget.value })} aria-label="Category">
        <option value="">All lists</option>
        {#each ['Films (English)', 'Films (Non-English)', 'TV (English)', 'TV (Non-English)'] as c (c)}<option value={c}>{c}</option>{/each}
      </select>
    {/if}
    {#if tab !== 'analytics'}
      <input class="input" type="date" value={date} onchange={(e) => set({ date: e.currentTarget.value })} aria-label={tab === 'netflix' ? 'Week' : 'Date'} title={tab === 'netflix' ? 'Any day of the week' : 'End date (default: latest)'} />
    {/if}
    <span class="spacer"></span>
    {#if loading}<span class="spinner"></span>{/if}
  </div>

  {#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

  {#if !data && loading}
    <div class="skeleton" style="height:240px"></div>
  {:else if data?.empty}
    <div class="card"><div class="empty"><div class="empty-title">No data</div>Nothing stored for these filters yet.</div></div>
  {:else if data && tab === 'leaderboard'}
    <p class="small muted">{dateOnly(data.from)} – {dateOnly(data.to)}</p>
    <div class="card">
      <div class="table-wrap">
        <table class="table">
          <thead><tr><th class="right">#</th><th>Title</th><th>Kind</th><th class="right">Points</th><th class="right">Days</th><th class="right">Best</th><th>Countries</th><th>Providers</th></tr></thead>
          <tbody>
            {#each data.items as it (it.position)}
              <tr>
                <td class="right mono">{it.position}</td>
                <td><a href={titleHref(it)} class="cell-title" style="color:inherit">{it.name}</a>{#if it.slugs.length > 1}<div class="cell-sub">{it.slugs.length} FlixPatrol titles</div>{/if}</td>
                <td><KindBadge kind={it.kind} /></td>
                <td class="right mono">{number(it.points)}</td>
                <td class="right">{it.days_on_charts}</td>
                <td class="right">#{it.best_rank}</td>
                <td class="small">{it.countries.join(', ')}</td>
                <td class="small">{it.providers.map(providerLabel).join(', ')}</td>
              </tr>
            {:else}
              <tr><td colspan="8"><div class="empty">No chart entries in this period.</div></td></tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {:else if data && tab === 'movers'}
    <div class="grid grid-2">
      {#each data.charts as c (c.category)}
        <div class="card">
          <div class="card-header">
            <div class="card-title">{c.category === 'movies' ? 'Movies' : 'TV shows'} · {dateOnly(c.date)}</div>
            <span class="muted small">{c.previous_date ? `vs ${dateOnly(c.previous_date)}` : 'no earlier chart'}</span>
          </div>
          <div class="card-body">
            {#each [['climbers', 'Climbers', 'badge-success'], ['fallers', 'Fallers', 'badge-danger'], ['debuts', 'New', 'badge-accent'], ['re_entries', 'Back', 'badge-accent'], ['exits', 'Dropped out', '']] as [k, label, cls] (k)}
              {#if c[k].length}
                <div class="section-title" style="margin-top:6px">{label}</div>
                {#each c[k] as m (m.title_id)}
                  <div class="row small" style="padding:3px 0">
                    <span class="mono" style="width:56px">{m.rank ? `#${m.rank}` : '—'}</span>
                    <a href={titleHref(m)} class="grow truncate" style="color:inherit">{m.name}</a>
                    {#if m.change}<span class="badge {cls}">{m.change > 0 ? '▲' : '▼'} {Math.abs(m.change)}</span>{:else if m.previous_rank}<span class="muted">was #{m.previous_rank}</span>{/if}
                  </div>
                {/each}
              {/if}
            {/each}
          </div>
        </div>
      {/each}
    </div>
  {:else if data && tab === 'netflix'}
    <p class="small muted">Week ending {dateOnly(data.week)} (Monday–Sunday) · {data.country ? countryName(data.country) : 'Global'} · from netflix.com/tudum/top10</p>
    <div class="card">
      <div class="table-wrap">
        <table class="table">
          <thead><tr>{#if !data.country}<th>List</th>{:else}<th>List</th>{/if}<th class="right">#</th><th>Title</th>{#if !data.country}<th class="right">Views</th><th class="right">Hours</th>{/if}<th class="right">Weeks</th><th>TMDB</th></tr></thead>
          <tbody>
            {#each data.items as it, i (i)}
              <tr>
                <td class="small nowrap">{it.category}</td>
                <td class="right mono">{it.rank}</td>
                <td><div class="cell-title">{it.show_title}</div>{#if it.season_title}<div class="cell-sub">{it.season_title}</div>{/if}</td>
                {#if !data.country}<td class="right">{number(it.views)}</td><td class="right">{number(it.hours_viewed)}</td>{/if}
                <td class="right">{it.cumulative_weeks ?? ''}</td>
                <td class="mono small">{it.tmdb_id ?? '—'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {:else if data && tab === 'analytics'}
    {@const g = data.genres}
    {@const lag = data.lag}
    {@const decay = data.decay}
    {@const rvp = data.rvp}
    <div class="grid grid-2">
      <div class="card">
        <div class="card-header"><div class="card-title">Genres by points</div><span class="muted small">{number(g.points_without_genres)} points without TMDB genres</span></div>
        <div class="card-body">
          {#each g.items.slice(0, 12) as it (it.genre)}
            <div class="row small" style="margin:4px 0">
              <span style="width:130px" class="truncate">{it.genre}</span>
              <div class="grow" style="background:var(--surface-3);border-radius:3px;height:8px"><div style="width:{Math.round(it.share * 100)}%;background:var(--accent);height:8px;border-radius:3px"></div></div>
              <span class="mono" style="width:44px;text-align:right">{Math.round(it.share * 100)}%</span>
            </div>
          {:else}<div class="empty small">No titles with TMDB genres yet.</div>{/each}
        </div>
      </div>

      <div class="card">
        <div class="card-header"><div class="card-title">Release → first chart</div><span class="muted small">{lag.summary.titles} titles</span></div>
        <div class="card-body">
          {#if lag.summary.median_days != null}
            <p class="small">Median <strong>{lag.summary.median_days}</strong> days (middle half {lag.summary.p25_days}–{lag.summary.p75_days}).</p>
            <dl class="kv small">
              {#each Object.entries(lag.by_provider).sort((a, b) => a[1].median_days - b[1].median_days) as [p, s] (p)}<dt>{providerLabel(p)}</dt><dd>{s.median_days} days median · {s.titles} titles</dd>{/each}
            </dl>
          {:else}<div class="empty small">Needs TMDB release dates (metadata job).</div>{/if}
        </div>
      </div>

      <div class="card">
        <div class="card-header"><div class="card-title">After a debut</div><span class="muted small">{decay.debuts} debuts followed for 30 days</span></div>
        <div class="table-wrap">
          <table class="table">
            <thead><tr><th class="right">Day</th><th class="right">Still charting</th><th class="right">Avg rank</th><th class="right">Titles tracked</th></tr></thead>
            <tbody>
              {#each decay.curve.filter((p) => [0, 1, 2, 3, 5, 7, 10, 14, 21, 28].includes(p.day)) as p (p.day)}
                <tr><td class="right">{p.day}</td><td class="right">{Math.round(p.retention * 100)}%</td><td class="right">{p.avg_rank ?? '—'}</td><td class="right muted">{p.titles}</td></tr>
              {:else}<tr><td colspan="4"><div class="empty small">No debuts in this range.</div></td></tr>{/each}
            </tbody>
          </table>
        </div>
      </div>

      <div class="card">
        <div class="card-header"><div class="card-title">Ratings vs popularity</div>
          <span class="muted small">r = {rvp.correlation_points_imdb ?? '—'} (IMDb) · {rvp.with_imdb}/{rvp.titles} rated</span></div>
        <div class="card-body small">
          <div class="section-title" style="margin-top:0">Acclaimed, under-watched</div>
          {#each rvp.acclaimed_underwatched.slice(0, 6) as it (it.title_id)}<div class="row"><a href={titleHref(it)} class="grow truncate" style="color:inherit">{it.name}</a><span class="mono">IMDb {it.imdb_rating}</span><span class="muted mono">{it.points} pts</span></div>{:else}<p class="muted">None.</p>{/each}
          <div class="section-title">Popular, poorly rated</div>
          {#each rvp.popular_poorly_rated.slice(0, 6) as it (it.title_id)}<div class="row"><a href={titleHref(it)} class="grow truncate" style="color:inherit">{it.name}</a><span class="mono">IMDb {it.imdb_rating}</span><span class="muted mono">{it.points} pts</span></div>{:else}<p class="muted">None.</p>{/each}
        </div>
      </div>

      <div class="card">
        <div class="card-header"><div class="card-title">Countries that watch alike</div><span class="muted small">overlap of charting titles</span></div>
        <div class="table-wrap">
          <table class="table">
            <tbody>
              {#each data.similarity.pairs.slice(0, 10) as p (p.a + p.b)}
                <tr><td>{countryName(p.a)} · {countryName(p.b)}</td><td class="right mono">{Math.round(p.jaccard * 100)}%</td><td class="right muted small">{p.shared} shared</td></tr>
              {:else}<tr><td><div class="empty small">Needs charts from two or more countries.</div></td></tr>{/each}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  {/if}
</div>
