<script>
  import { listTitles } from '../lib/api.js'
  import Pagination from './Pagination.svelte'
  import EditModal from './EditModal.svelte'

  const LIMIT = 50

  let items   = $state([])
  let total   = $state(0)
  let offset  = $state(0)
  let kind    = $state('')
  let search  = $state('')
  let loading = $state(false)
  let error   = $state('')
  let editing = $state(null)

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

  let debounceTimer

  function onSearchInput(e) {
    clearTimeout(debounceTimer)
    debounceTimer = setTimeout(() => {
      search = e.target.value.trim()
      offset = 0
    }, 300)
  }

  function setKind(k) {
    kind = k
    offset = 0
  }

  $effect(() => {
    const s = search, k = kind, o = offset
    loading = true
    error = ''
    listTitles({ limit: LIMIT, offset: o, kind: k, q: s })
      .then(data => {
        items = data.items ?? []
        total = data.total ?? 0
      })
      .catch(e => { error = e.message })
      .finally(() => { loading = false })
  })

  function openEdit(title) {
    editing = title
  }

  function onSaved(updatedTitle) {
    if (updatedTitle?.id) {
      items = items.map((item) => item.id === updatedTitle.id ? { ...item, ...updatedTitle } : item)
    }

    // Reload current page
    const s = search, k = kind, o = offset
    listTitles({ limit: LIMIT, offset: o, kind: k, q: s })
      .then(data => { items = data.items ?? []; total = data.total ?? 0 })
    editing = null
  }
</script>

<div class="toolbar">
  <input
    class="search-input"
    type="search"
    placeholder="Search by name…"
    oninput={onSearchInput}
  />
  <div class="filter-group">
    <button class="filter-btn" class:active={kind === ''}       onclick={() => setKind('')}>All</button>
    <button class="filter-btn" class:active={kind === 'movie'}   onclick={() => setKind('movie')}>Movies</button>
    <button class="filter-btn" class:active={kind === 'tv_show'} onclick={() => setKind('tv_show')}>TV Shows</button>
  </div>
</div>

{#if error}
  <div class="state-msg error-msg">{error}</div>
{:else if loading && items.length === 0}
  <div class="state-msg">Loading…</div>
{:else if items.length === 0}
  <div class="state-msg">No titles found.</div>
{:else}
  <table class="data-table">
    <thead>
      <tr>
        <th>ID</th>
        <th>Slug</th>
        <th>Name</th>
        <th>Kind</th>
        <th>TMDB ID</th>
        <th>IMDb ID</th>
        <th>RT URL</th>
        <th></th>
      </tr>
    </thead>
    <tbody>
      {#each items as title (title.id)}
        <tr>
          <td class="cell-mono">{title.id}</td>
          <td class="cell-mono">{title.slug}</td>
          <td>{title.name}</td>
          <td>
            {#if title.kind === 'movie'}
              <span class="badge badge-movie">movie</span>
            {:else}
              <span class="badge badge-tv">tv</span>
            {/if}
          </td>
          <td class="cell-mono">
            {#if title.tmdb_id}
              {title.tmdb_id}
              <a href={tmdbUrl(title.kind, title.tmdb_id)} target="_blank" rel="noopener" aria-label="Open TMDB in new tab" title="Open in new tab" style="margin-left:8px;color:var(--accent)">↗</a>
            {:else}
              —
            {/if}
          </td>
          <td class="cell-mono">
            {#if title.imdb_id}
              {title.imdb_id}
              <a href={imdbUrl(title.imdb_id)} target="_blank" rel="noopener" aria-label="Open IMDb in new tab" title="Open in new tab" style="margin-left:8px;color:var(--accent)">↗</a>
            {:else}
              —
            {/if}
          </td>
          <td class="cell-mono">
            {#if title.rt_url}
              {title.rt_url}
              <a href={rtUrl(title.rt_url)} target="_blank" rel="noopener" aria-label="Open Rotten Tomatoes in new tab" title="Open in new tab" style="color:var(--accent)">↗</a>
            {:else}
              —
            {/if}
          </td>
          <td>
            <button class="btn btn-sm" onclick={() => openEdit(title)}>Edit</button>
          </td>
        </tr>
      {/each}
    </tbody>
  </table>

  <Pagination
    {offset}
    limit={LIMIT}
    {total}
    onchange={(o) => { offset = o }}
  />
{/if}

{#if editing}
  <EditModal
    title={editing}
    onclose={() => { editing = null }}
    onsave={onSaved}
  />
{/if}
