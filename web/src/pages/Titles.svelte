<script>
  import { listTitles } from '../lib/api.js'
  import { route, setQuery, intParam } from '../lib/router.svelte.js'
  import { relativeTime, dateOnly, dateTime, links, downloadCSV } from '../lib/format.js'
  import Icon from '../components/Icon.svelte'
  import Pagination from '../components/Pagination.svelte'
  import SortTh from '../components/SortTh.svelte'
  import KindBadge from '../components/KindBadge.svelte'
  import IdCell from '../components/IdCell.svelte'
  import TitleDrawer from './TitleDrawer.svelte'

  // UI sort ("field" / "-field") → server sort names.
  const SERVER_SORT = {
    name: 'name_asc', '-name': 'name_desc',
    updated: 'updated_asc', '-updated': 'updated_desc',
    last_ranked: 'last_ranked_asc', '-last_ranked': 'last_ranked_desc',
    rankings: 'rankings_asc', '-rankings': 'rankings_desc',
    id: 'id_asc', '-id': '',
  }

  const q = $derived(route.query.q ?? '')
  const kind = $derived(route.query.kind ?? '')
  const missing = $derived(route.query.missing ?? '')
  const sort = $derived(route.query.sort ?? '-id')
  const page = $derived(intParam(route.query.page, 1))
  const size = $derived(intParam(route.query.size, 50))
  const openId = $derived(route.query.open ? Number(route.query.open) : null)

  let data = $state({ items: [], total: 0 })
  let loading = $state(false)
  let error = $state('')
  let reloadTick = $state(0)
  let searchInput = $state()

  $effect(() => {
    const params = { q, kind, missing, sort: SERVER_SORT[sort] ?? '', limit: size, offset: (page - 1) * size }
    reloadTick
    loading = true
    let cancelled = false
    listTitles(params)
      .then((d) => { if (!cancelled) { data = d; error = '' } })
      .catch((e) => { if (!cancelled) error = e.message })
      .finally(() => { if (!cancelled) loading = false })
    return () => { cancelled = true }
  })

  let debounce
  function onSearch(e) {
    const value = e.currentTarget.value
    clearTimeout(debounce)
    debounce = setTimeout(() => setQuery({ q: value.trim(), page: '' }, { replace: true }), 300)
  }

  const set = (patch) => setQuery({ ...patch, page: '' })
  const activeFilters = $derived(
    [q && ['q', `Search: ${q}`], kind && ['kind', kind === 'movie' ? 'Movies' : 'TV'], missing && ['missing', `Missing ${missing.toUpperCase()}`]].filter(Boolean)
  )

  function onkeydown(e) {
    if (e.key === '/' && document.activeElement?.tagName !== 'INPUT') {
      e.preventDefault()
      searchInput?.focus()
    }
  }

  function exportCSV() {
    downloadCSV('titles.csv', [
      { key: 'id', label: 'id' }, { key: 'slug', label: 'slug' }, { key: 'name', label: 'name' }, { key: 'kind', label: 'kind' },
      { key: 'tmdb_id', label: 'tmdb_id' }, { key: 'imdb_id', label: 'imdb_id' }, { key: 'rt_url', label: 'rt_slug' },
      { key: 'rankings_count', label: 'rankings' }, { key: 'last_ranked_on', label: 'last_ranked_on' }, { key: 'updated_at', label: 'updated_at' },
    ], data.items)
  }

  function onSaved(updated) {
    data.items = data.items.map((t) => (t.id === updated.id ? { ...t, ...updated } : t))
  }
</script>

<svelte:window {onkeydown} />

<div class="page">
  <div class="page-header">
    <div>
      <h1 class="page-title">Titles & mapping</h1>
      <p class="page-desc">Every title seen in a FlixPatrol chart and its TMDB, IMDb and Rotten Tomatoes IDs. Click a row to review or fix its mapping.</p>
    </div>
    <div class="page-actions">
      <button class="btn" onclick={exportCSV} disabled={!data.items.length}><Icon name="download" />Export page</button>
    </div>
  </div>

  <div class="toolbar">
    <div class="search">
      <span class="icon"><Icon name="search" size={14} /></span>
      <input bind:this={searchInput} class="input" type="search" placeholder="Search name or slug  ( / )" value={q} oninput={onSearch} />
    </div>
    <div class="segmented" role="group" aria-label="Kind">
      <button class:active={kind === ''} onclick={() => set({ kind: '' })}>All</button>
      <button class:active={kind === 'movie'} onclick={() => set({ kind: 'movie' })}>Movies</button>
      <button class:active={kind === 'tv_show'} onclick={() => set({ kind: 'tv_show' })}>TV</button>
    </div>
    <select class="select" value={missing} onchange={(e) => set({ missing: e.currentTarget.value })} aria-label="Mapping filter">
      <option value="">Any mapping</option>
      <option value="tmdb">Missing TMDB ID</option>
      <option value="imdb">Missing IMDb ID</option>
      <option value="rt">Missing Rotten Tomatoes</option>
    </select>
    <span class="spacer"></span>
    {#if loading}<span class="spinner"></span>{/if}
  </div>

  {#if activeFilters.length}
    <div class="filter-chips">
      {#each activeFilters as [key, label] (key)}
        <span class="chip">{label}<button onclick={() => set({ [key]: '' })} aria-label="Remove filter"><Icon name="x" size={11} /></button></span>
      {/each}
      <button class="btn btn-ghost btn-sm" onclick={() => set({ q: '', kind: '', missing: '' })}>Clear all</button>
    </div>
  {/if}

  {#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

  <div class="card">
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <SortTh label="Title" field="name" {sort} onsort={(s) => set({ sort: s })} />
            <th>Kind</th>
            <th>TMDB</th>
            <th>IMDb</th>
            <th>Rotten Tomatoes</th>
            <SortTh label="Charted" field="rankings" firstDir="desc" align="right" title="Number of chart positions" {sort} onsort={(s) => set({ sort: s })} />
            <SortTh label="Last charted" field="last_ranked" firstDir="desc" {sort} onsort={(s) => set({ sort: s })} />
            <SortTh label="Updated" field="updated" firstDir="desc" {sort} onsort={(s) => set({ sort: s })} />
          </tr>
        </thead>
        <tbody>
          {#each data.items as t (t.id)}
            <tr class="clickable" class:selected={openId === t.id} onclick={() => setQuery({ open: t.id })}>
              <td>
                <div class="cell-title">{t.name}</div>
                <div class="cell-sub mono">{t.slug}</div>
              </td>
              <td><KindBadge kind={t.kind} /></td>
              <td><IdCell value={t.tmdb_id} url={links.tmdb(t.kind, t.tmdb_id)} /></td>
              <td><IdCell value={t.imdb_id} url={links.imdb(t.imdb_id)} /></td>
              <td><IdCell value={t.rt_url} url={links.rt(t.rt_url)} /></td>
              <td class="right">{t.rankings_count ?? 0}</td>
              <td class="nowrap">{t.last_ranked_on ? dateOnly(t.last_ranked_on) : '—'}</td>
              <td class="nowrap muted" title={dateTime(t.updated_at)}>{relativeTime(t.updated_at)}</td>
            </tr>
          {:else}
            <tr><td colspan="8">
              <div class="empty">
                {#if loading}Loading…{:else}<div class="empty-title">No titles match</div>Try a different search or clear the filters.{/if}
              </div>
            </td></tr>
          {/each}
        </tbody>
      </table>
    </div>
    <Pagination {page} {size} total={data.total} onchange={(p, s) => setQuery({ page: p === 1 ? '' : p, size: s === 50 ? '' : s })} />
  </div>
</div>

{#if openId}
  <TitleDrawer id={openId} onclose={() => setQuery({ open: '' })} onsaved={onSaved} />
{/if}
