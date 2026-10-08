<script>
  import { getTitle, patchTitle, getTitleCandidates, getTitleRankings, getRatings } from '../lib/api.js'
  import { links, parseImdbId, parseTmdbInput, parseRtSlug, dateOnly, relativeTime, dateTime, number } from '../lib/format.js'
  import { providerLabel, countryName } from '../lib/constants.js'
  import { toast } from '../lib/toast.svelte.js'
  import Drawer from '../components/Drawer.svelte'
  import Icon from '../components/Icon.svelte'
  import KindBadge from '../components/KindBadge.svelte'
  import RankChart from '../components/RankChart.svelte'

  let { id, onclose, onsaved } = $props()

  let title = $state(null)
  let loadError = $state('')
  let tab = $state('mapping')

  // Mapping form
  let form = $state({ kind: 'movie', tmdb_id: '', imdb_id: '', rt_url: '' })
  let saving = $state(false)
  const dirty = $derived(
    title && (form.kind !== title.kind || form.tmdb_id !== (title.tmdb_id ?? '') || form.imdb_id !== (title.imdb_id ?? '') || form.rt_url !== (title.rt_url ?? ''))
  )

  // Candidate search
  let search = $state({ name: '', year: '', kind: '' })
  let candidates = $state(null)
  let searching = $state(false)

  // Rankings & ratings tabs (loaded on first open)
  let rankings = $state(null)
  let chartKey = $state('')
  let ratings = $state(null)
  let ratingsError = $state('')
  let ratingsLoading = $state(false)

  $effect(() => {
    const titleId = id
    title = null
    rankings = null
    ratings = null
    candidates = null
    loadError = ''
    getTitle(titleId)
      .then((t) => {
        title = t
        resetForm()
        const slugYear = t.slug.match(/-(\d{4})$/)?.[1] ?? ''
        search = { name: t.name, year: slugYear, kind: t.kind }
        if (!t.tmdb_id || !t.rt_url) findCandidates()
      })
      .catch((e) => (loadError = e.message))
  })

  function resetForm() {
    form = { kind: title.kind, tmdb_id: title.tmdb_id ?? '', imdb_id: title.imdb_id ?? '', rt_url: title.rt_url ?? '' }
  }

  // Accept pasted URLs: normalize to IDs when a field loses focus.
  function normalizeTmdb() {
    const { id: tmdbId, kind } = parseTmdbInput(form.tmdb_id)
    form.tmdb_id = tmdbId
    if (kind) form.kind = kind
  }

  async function save() {
    saving = true
    try {
      const updated = await patchTitle(title.id, {
        kind: form.kind,
        tmdb_id: form.tmdb_id.trim(),
        imdb_id: parseImdbId(form.imdb_id),
        rt_url: parseRtSlug(form.rt_url),
      })
      title = { ...title, ...updated }
      resetForm()
      ratings = null
      onsaved?.(updated)
      toast.success(`Saved mapping for "${title.name}"`)
    } catch (e) {
      toast.error(e.message)
    } finally {
      saving = false
    }
  }

  async function findCandidates() {
    searching = true
    try {
      candidates = await getTitleCandidates(id, { name: search.name, year: search.year, kind: search.kind })
    } catch (e) {
      toast.error(e.message)
    } finally {
      searching = false
    }
  }

  function useCandidate(c) {
    if (c.tmdb_id) form.tmdb_id = c.tmdb_id
    if (c.imdb_id) form.imdb_id = c.imdb_id
    if (c.kind) form.kind = c.kind
    toast.info(`Filled in from "${c.title}" — review and save`)
  }

  function openTab(name) {
    tab = name
    if (name === 'rankings' && !rankings) loadRankings()
    if (name === 'ratings' && !ratings && title?.tmdb_id) loadRatings(false)
  }

  async function loadRankings() {
    try {
      rankings = await getTitleRankings(id)
      chartKey = rankings.charts[0] ? key(rankings.charts[0]) : ''
    } catch (e) {
      toast.error(e.message)
      rankings = { charts: [], rankings: [] }
    }
  }

  async function loadRatings(refresh) {
    ratingsLoading = true
    ratingsError = ''
    try {
      ratings = await getRatings(title.kind === 'tv_show' ? 'tv' : 'movie', title.tmdb_id, refresh)
      if (refresh) toast.success('Ratings refreshed')
    } catch (e) {
      ratingsError = e.message
    } finally {
      ratingsLoading = false
    }
  }

  const key = (c) => `${c.country}/${c.provider}/${c.category}`
  const chartPoints = $derived(
    rankings ? rankings.rankings.filter((r) => key(r) === chartKey).map((r) => ({ date: r.date, rank: r.rank })) : []
  )
</script>

<Drawer {onclose}>
  {#snippet header()}
    {#if title}
      <div class="row" style="gap:10px">
        <div class="drawer-title">{title.name}</div>
        <KindBadge kind={title.kind} />
      </div>
      <div class="row small muted" style="margin-top:4px;flex-wrap:wrap">
        <span class="mono">{title.slug}</span>·<span>#{title.id}</span>·
        <a href={links.flixpatrol(title.slug)} target="_blank" rel="noopener">FlixPatrol <Icon name="external" size={11} /></a>
        {#if title.tmdb_id}<a href={links.tmdb(title.kind, title.tmdb_id)} target="_blank" rel="noopener">TMDB <Icon name="external" size={11} /></a>{/if}
        {#if title.imdb_id}<a href={links.imdb(title.imdb_id)} target="_blank" rel="noopener">IMDb <Icon name="external" size={11} /></a>{/if}
        {#if title.rt_url}<a href={links.rt(title.rt_url)} target="_blank" rel="noopener">Rotten Tomatoes <Icon name="external" size={11} /></a>{/if}
      </div>
    {:else}
      <div class="drawer-title">{loadError ? 'Title not found' : 'Loading…'}</div>
    {/if}
  {/snippet}

  {#if loadError}
    <div class="alert alert-error"><Icon name="alert" />{loadError}</div>
  {:else if title}
    <div class="tabs">
      <button class="tab" class:active={tab === 'mapping'} onclick={() => openTab('mapping')}>Mapping</button>
      <button class="tab" class:active={tab === 'rankings'} onclick={() => openTab('rankings')}>Chart history</button>
      <button class="tab" class:active={tab === 'ratings'} onclick={() => openTab('ratings')}>Ratings</button>
    </div>

    {#if tab === 'mapping'}
      <div class="section-title">External IDs</div>
      <div class="field-row">
        <label class="field">
          <span class="field-label">Kind</span>
          <select class="select" bind:value={form.kind}>
            <option value="movie">Movie</option>
            <option value="tv_show">TV show</option>
          </select>
          <span class="field-hint">Which TMDB namespace the ID belongs to.</span>
        </label>
        <label class="field">
          <span class="field-label">TMDB ID</span>
          <input class="input mono" bind:value={form.tmdb_id} onblur={normalizeTmdb} placeholder="e.g. 701387 or TMDB URL" />
        </label>
      </div>
      <div class="field-row">
        <label class="field">
          <span class="field-label">IMDb ID</span>
          <input class="input mono" bind:value={form.imdb_id} onblur={() => (form.imdb_id = parseImdbId(form.imdb_id))} placeholder="tt1234567 or IMDb URL" />
        </label>
        <label class="field">
          <span class="field-label">Rotten Tomatoes</span>
          <input class="input mono" bind:value={form.rt_url} onblur={() => (form.rt_url = parseRtSlug(form.rt_url))} placeholder="m/slug, tv/slug or RT URL" />
        </label>
      </div>
      <p class="field-hint" style="margin-top:-6px">Paste IDs or full URLs; they are normalized automatically. Leave a field empty to clear it.</p>
      <div class="row" style="margin-top:12px">
        <button class="btn btn-primary" onclick={save} disabled={!dirty || saving}>{saving ? 'Saving…' : 'Save mapping'}</button>
        <button class="btn" onclick={resetForm} disabled={!dirty}>Discard changes</button>
        {#if dirty}<span class="badge badge-warning">Unsaved changes</span>{/if}
      </div>

      <div class="section-title" style="margin-top:24px">Find matches</div>
      <div class="row" style="flex-wrap:wrap">
        <input class="input grow" bind:value={search.name} placeholder="Title" onkeydown={(e) => e.key === 'Enter' && findCandidates()} />
        <input class="input" style="width:80px" bind:value={search.year} placeholder="Year" inputmode="numeric" />
        <select class="select" bind:value={search.kind}>
          <option value="movie">Movie</option>
          <option value="tv_show">TV show</option>
        </select>
        <button class="btn" onclick={findCandidates} disabled={searching || !search.name}>
          {#if searching}<span class="spinner"></span>{:else}<Icon name="search" size={14} />{/if}Search
        </button>
      </div>

      {#if candidates}
        {#if candidates.errors}
          {#each Object.entries(candidates.errors) as [provider, msg] (provider)}
            <div class="alert alert-warning small" style="margin-top:10px"><Icon name="alert" size={14} />{provider}: {msg}</div>
          {/each}
        {/if}

        <div class="section-title">JustWatch <span class="muted">(TMDB + IMDb)</span></div>
        {#each candidates.justwatch as c, i (i)}
          {@render candidateRow(c, form.tmdb_id === c.tmdb_id)}
        {:else}<p class="muted small">No JustWatch results.</p>{/each}

        <div class="section-title">TMDB</div>
        {#each candidates.tmdb as c, i (i)}
          {@render candidateRow(c, form.tmdb_id === c.tmdb_id)}
        {:else}<p class="muted small">No TMDB results (TMDB search needs TMDB_API_KEY).</p>{/each}

        <div class="section-title">Rotten Tomatoes</div>
        {#each candidates.rotten_tomatoes as c (c.slug)}
          <div class="candidate">
            <div class="meta">
              <div class="row"><strong class="truncate">{c.title}</strong><span class="muted">{c.year || ''}</span>
                {#if form.rt_url === c.slug}<span class="badge badge-success">Selected</span>{/if}</div>
              <div class="small muted row">
                <span class="mono">{c.slug}</span>
                {#if c.critics_score !== null}<span>🍅 {c.critics_score}%</span>{/if}
                {#if c.audience_score !== null}<span>🍿 {c.audience_score}%</span>{/if}
                <a href={c.url} target="_blank" rel="noopener"><Icon name="external" size={11} /></a>
              </div>
            </div>
            <button class="btn btn-sm" onclick={() => { form.rt_url = c.slug; toast.info(`RT set to ${c.slug} — review and save`) }}>Use</button>
          </div>
        {:else}<p class="muted small">No Rotten Tomatoes results.</p>{/each}
      {/if}
    {:else if tab === 'rankings'}
      {#if !rankings}
        <div class="skeleton" style="height:140px"></div>
      {:else if rankings.rankings.length === 0}
        <div class="empty"><div class="empty-title">No chart history</div>This title has no stored rankings.</div>
      {:else}
        <div class="table-wrap card">
          <table class="table">
            <thead><tr><th>Chart</th><th class="right">Days</th><th class="right">Best</th><th>First</th><th>Last</th><th class="right">Last rank</th></tr></thead>
            <tbody>
              {#each rankings.charts as c (key(c))}
                <tr class="clickable" class:selected={chartKey === key(c)} onclick={() => (chartKey = key(c))}>
                  <td>{providerLabel(c.provider)} · {countryName(c.country)} <span class="muted small">({c.category === 'movies' ? 'Movies' : 'TV'})</span></td>
                  <td class="right">{c.days_in_top10}</td>
                  <td class="right">#{c.best_rank}</td>
                  <td class="nowrap">{dateOnly(c.first_date)}</td>
                  <td class="nowrap">{dateOnly(c.last_date)}</td>
                  <td class="right">#{c.last_rank}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
        <div class="section-title">Rank over time · {chartKey}</div>
        <div class="card card-body"><RankChart points={chartPoints} /></div>
      {/if}
    {:else}
      {#if !title.tmdb_id}
        <div class="empty"><div class="empty-title">Not mapped to TMDB</div>Ratings are looked up by TMDB ID. Map this title first.</div>
      {:else if ratingsError}
        <div class="alert alert-error"><Icon name="alert" />{ratingsError}</div>
        <button class="btn" onclick={() => loadRatings(false)}>Retry</button>
      {:else if !ratings}
        <div class="skeleton" style="height:90px"></div>
      {:else}
        <div class="row" style="justify-content:space-between;margin-bottom:12px">
          <span class="muted small">
            {ratings.cached ? 'From cache' : 'Fetched'} · refreshed <span title={dateTime(ratings.refreshed_at)}>{relativeTime(ratings.refreshed_at)}</span>
            {#if ratings.stale}<span class="badge badge-warning">stale</span>{/if}
          </span>
          <button class="btn btn-sm" onclick={() => loadRatings(true)} disabled={ratingsLoading}><Icon name="refresh" size={12} />Refresh now</button>
        </div>
        <div class="grid" style="grid-template-columns:repeat(3,1fr)">
          <div class="card stat">
            <div class="stat-label">IMDb</div>
            <div class="score">{ratings.imdb ? ratings.imdb.rating.toFixed(1) : '—'}</div>
            <div class="stat-foot">{ratings.imdb ? `${number(ratings.imdb.votes)} votes · via ${ratings.imdb.source}` : 'No rating'}</div>
          </div>
          <div class="card stat">
            <div class="stat-label">Tomatometer</div>
            <div class="score">{ratings.rotten_tomatoes?.critics_score ?? '—'}{ratings.rotten_tomatoes?.critics_score !== null && ratings.rotten_tomatoes ? '%' : ''}</div>
            <div class="stat-foot">{ratings.rotten_tomatoes?.critics_rating ?? 'No score'}{ratings.rotten_tomatoes?.critics_source ? ` · via ${ratings.rotten_tomatoes.critics_source}` : ''}</div>
          </div>
          <div class="card stat">
            <div class="stat-label">Popcornmeter</div>
            <div class="score">{ratings.rotten_tomatoes?.audience_score ?? '—'}{ratings.rotten_tomatoes?.audience_score !== null && ratings.rotten_tomatoes ? '%' : ''}</div>
            <div class="stat-foot">{ratings.rotten_tomatoes?.audience_rating ?? 'No score'}{ratings.rotten_tomatoes?.audience_source ? ` · via ${ratings.rotten_tomatoes.audience_source}` : ''}</div>
          </div>
        </div>
        <div class="section-title">All sources</div>
        <div class="card table-wrap">
          <table class="table">
            <thead><tr><th>Provider</th><th>Source</th><th class="right">Value</th><th class="right">Votes</th><th>Fetched</th></tr></thead>
            <tbody>
              {#each ratings.sources as s (s.provider + s.source)}
                <tr>
                  <td>{s.provider}</td>
                  <td>{#if s.url}<a href={s.url} target="_blank" rel="noopener">{s.source}</a>{:else}{s.source}{/if}</td>
                  <td class="right mono">{s.value}</td>
                  <td class="right">{number(s.votes)}</td>
                  <td class="muted small">{relativeTime(s.fetched_at)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    {/if}
  {/if}
</Drawer>

{#snippet candidateRow(c, selected)}
  <div class="candidate">
    <div class="meta">
      <div class="row"><strong class="truncate">{c.title}</strong><span class="muted">{c.year ?? ''}</span>
        {#if selected}<span class="badge badge-success">Selected</span>{/if}</div>
      <div class="small muted row">
        {#if c.tmdb_id}<span class="mono">TMDB {c.tmdb_id}</span>{/if}
        {#if c.imdb_id}<span class="mono">{c.imdb_id}</span>{/if}
        {#if c.url}<a href={c.url} target="_blank" rel="noopener"><Icon name="external" size={11} /></a>{/if}
      </div>
    </div>
    <button class="btn btn-sm" onclick={() => useCandidate(c)} disabled={!c.tmdb_id && !c.imdb_id}>Use</button>
  </div>
{/snippet}
