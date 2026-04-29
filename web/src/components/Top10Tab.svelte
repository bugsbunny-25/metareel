<script>
  import { getTop10, PROVIDERS, COUNTRIES } from '../lib/api.js'
  import EditModal from './EditModal.svelte'

  function currentDateUTC() {
    return new Date().toISOString().slice(0, 10)
  }

  function yesterdayUTC() {
    const d = new Date()
    d.setDate(d.getDate() - 1)
    return d.toISOString().slice(0, 10)
  }

  function defaultDateUTC() {
    const now = new Date()
    return now.getUTCHours() < 12 ? yesterdayUTC() : currentDateUTC()
  }

  let country  = $state('US')
  let provider = $state('')
  let category = $state('movies')
  let date     = $state(defaultDateUTC())
  let items    = $state([])
  let loading  = $state(false)
  let error    = $state('')
  let editing  = $state(null)

  function tmdbUrl(kind, tmdbId) {
    if (!tmdbId) return ''
    const mediaType = kind === 'movie' ? 'movie' : 'tv'
    return `https://www.themoviedb.org/${mediaType}/${tmdbId}`
  }

  function imdbUrl(imdbId) {
    if (!imdbId) return ''
    return `https://www.imdb.com/title/${imdbId}/`
  }

  function rtUrl(rtSlug) {
    if (!rtSlug) return ''
    return `https://www.rottentomatoes.com/${rtSlug}`
  }

  function titleKey(item) {
    return item.id ?? item.slug
  }

  function onTitleSaved(updatedTitle) {
    if (!updatedTitle) {
      editing = null
      return
    }

    const key = titleKey(updatedTitle)
    items = items.map((item) =>
      titleKey(item) === key
        ? { ...item, ...updatedTitle, rotten_tomatoes_url: updatedTitle.rt_url ?? item.rotten_tomatoes_url }
        : item
    )
    editing = null
  }

  function stepDate(delta) {
    const d = new Date(date)
    d.setDate(d.getDate() + delta)
    date = d.toISOString().slice(0, 10)
  }

  $effect(() => {
    const c = country, p = provider, cat = category, d = date
    if (!c || !d) return
    loading = true
    error = ''
    getTop10({ country: c, provider: p, category: cat, date: d })
      .then(data => { items = data.items ?? [] })
      .catch(e => { error = e.message; items = [] })
      .finally(() => { loading = false })
  })
</script>

<div class="top10-filters">
  <select bind:value={country}>
    {#each COUNTRIES as c}
      <option value={c.code}>{c.name} ({c.code})</option>
    {/each}
  </select>

  <select bind:value={provider}>
    {#each PROVIDERS as p}
      <option value={p.value}>{p.label}</option>
    {/each}
  </select>

  <div class="filter-group">
    <button class="filter-btn" class:active={category === 'movies'}   onclick={() => (category = 'movies')}>Movies</button>
    <button class="filter-btn" class:active={category === 'tv_shows'} onclick={() => (category = 'tv_shows')}>TV Shows</button>
  </div>

  <div class="date-nav">
    <button class="btn btn-sm" onclick={() => stepDate(-1)}>←</button>
    <input type="date" bind:value={date} max={currentDateUTC()} />
    <button class="btn btn-sm" onclick={() => stepDate(1)}>→</button>
  </div>
</div>

{#if error}
  <div class="state-msg error-msg">{error}</div>
{:else if loading}
  <div class="state-msg">Loading…</div>
{:else if items.length === 0}
  <div class="state-msg">No data for this selection.</div>
{:else}
  <table class="data-table">
    <thead>
      <tr>
        <th>#</th>
        <th>Name</th>
        <th>Kind</th>
        <th>Provider</th>
        <th>TMDB ID</th>
        <th>IMDb ID</th>
        <th>RT URL</th>
        <th></th>
      </tr>
    </thead>
    <tbody>
      {#each items as item (item.rank + item.provider + item.slug)}
        <tr>
          <td class="rank-cell">{item.rank}</td>
          <td>{item.name}</td>
          <td>
            {#if item.kind === 'movie'}
              <span class="badge badge-movie">movie</span>
            {:else}
              <span class="badge badge-tv">tv</span>
            {/if}
          </td>
          <td class="cell-mono">{item.provider}</td>
          <td class="cell-mono">
            {#if item.tmdb_id}
              {item.tmdb_id}
              <a href={tmdbUrl(item.kind, item.tmdb_id)} target="_blank" rel="noopener" aria-label="Open TMDB in new tab" title="Open in new tab" style="margin-left:8px;color:var(--accent)">↗</a>
            {:else}
              —
            {/if}
          </td>
          <td class="cell-mono">
            {#if item.imdb_id}
              {item.imdb_id}
              <a href={imdbUrl(item.imdb_id)} target="_blank" rel="noopener" aria-label="Open IMDb in new tab" title="Open in new tab" style="margin-left:8px;color:var(--accent)">↗</a>
            {:else}
              —
            {/if}
          </td>
          <td class="cell-mono">
            {#if item.rotten_tomatoes_url}
              {item.rotten_tomatoes_url}
              <a href={rtUrl(item.rotten_tomatoes_url)} target="_blank" rel="noopener" aria-label="Open Rotten Tomatoes in new tab" title="Open in new tab" style="color:var(--accent)">↗</a>
            {:else}
              —
            {/if}
          </td>
          <td>
            {#if item.slug}
              <button class="btn btn-sm" onclick={() => (editing = { ...item, rt_url: item.rotten_tomatoes_url ?? item.rt_url })}>Edit</button>
            {/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}

{#if editing}
  <EditModal
    title={editing}
    onclose={() => (editing = null)}
    onsave={onTitleSaved}
  />
{/if}
