<script>
  import { getReleases, listServices } from '../lib/api.js'
  import { route, setQuery, intParam } from '../lib/router.svelte.js'
  import { dateOnly, number, links, downloadCSV } from '../lib/format.js'
  import { COUNTRIES, PROVIDERS } from '../lib/constants.js'
  import Icon from '../components/Icon.svelte'
  import Pagination from '../components/Pagination.svelte'
  import SortTh from '../components/SortTh.svelte'
  import KindBadge from '../components/KindBadge.svelte'

  const type = $derived(route.query.type ?? 'new')
  const country = $derived(route.query.country ?? 'US')
  const service = $derived(route.query.service ?? 'netflix')
  const kind = $derived(route.query.kind ?? '')
  const from = $derived(route.query.from ?? '')
  const to = $derived(route.query.to ?? '')
  const q = $derived(route.query.q ?? '')
  const sort = $derived(route.query.sort ?? (type === 'new' ? '-date' : 'date'))
  const page = $derived(intParam(route.query.page, 1))
  const size = $derived(intParam(route.query.size, 50))

  let services = $state([])
  let data = $state(null)
  let loading = $state(false)
  let error = $state('')

  $effect(() => {
    const c = country
    listServices(c).then((r) => (services = r.services)).catch(() => (services = []))
  })

  $effect(() => {
    const args = [type, country, service, { type: kind, from, to }]
    loading = true
    let cancelled = false
    getReleases(...args)
      .then((d) => { if (!cancelled) { data = d; error = '' } })
      .catch((e) => { if (!cancelled) { data = null; error = e.message } })
      .finally(() => { if (!cancelled) loading = false })
    return () => { cancelled = true }
  })

  // Our providers first, then everything JustWatch lists for the country.
  const otherServices = $derived(
    services.filter((s) => !PROVIDERS.some((p) => p.label === s.name)).sort((a, b) => a.name.localeCompare(b.name))
  )

  const SORTS = {
    date: (it) => it.date ?? '9999',
    title: (it) => it.title.toLowerCase(),
    imdb: (it) => it.imdb_rating ?? -1,
    year: (it) => it.year ?? 0,
  }
  const filtered = $derived.by(() => {
    let items = data?.items ?? []
    if (q) {
      const needle = q.toLowerCase()
      items = items.filter((it) => it.title.toLowerCase().includes(needle) || it.genres.some((g) => g.toLowerCase().includes(needle)))
    }
    const field = sort.replace(/^-/, '')
    const get = SORTS[field] ?? SORTS.date
    const dir = sort.startsWith('-') ? -1 : 1
    return [...items].sort((a, b) => (get(a) > get(b) ? dir : get(a) < get(b) ? -dir : 0))
  })
  const pageItems = $derived(filtered.slice((page - 1) * size, page * size))

  const set = (patch) => setQuery({ ...patch, page: '' })
  let debounce
  function onSearch(e) {
    const v = e.currentTarget.value
    clearTimeout(debounce)
    debounce = setTimeout(() => setQuery({ q: v.trim(), page: '' }, { replace: true }), 250)
  }

  function exportCSV() {
    downloadCSV(`${type}-${country}-${service}.csv`, [
      { key: 'date', label: 'date' }, { key: 'kind', label: 'kind' }, { key: 'title', label: 'title' },
      { key: 'season_number', label: 'season' }, { key: 'year', label: 'year' }, { key: 'tmdb_id', label: 'tmdb_id' },
      { key: 'imdb_id', label: 'imdb_id' }, { key: 'imdb_rating', label: 'imdb_rating' }, { key: 'tomatometer', label: 'tomatometer' },
      { label: 'genres', value: (r) => r.genres.join('; ') }, { key: 'justwatch_url', label: 'justwatch_url' },
    ], filtered)
  }
</script>

<div class="page">
  <div class="page-header">
    <div>
      <h1 class="page-title">New & upcoming</h1>
      <p class="page-desc">Titles added to, or announced for, a streaming service — live from JustWatch, nothing is stored.</p>
    </div>
    <div class="page-actions">
      <button class="btn" onclick={exportCSV} disabled={!filtered.length}><Icon name="download" />Export CSV</button>
    </div>
  </div>

  <div class="toolbar">
    <div class="segmented" role="group" aria-label="Listing">
      <button class:active={type === 'new'} onclick={() => setQuery({ type: 'new', from: '', to: '', sort: '', page: '' })}>New</button>
      <button class:active={type === 'upcoming'} onclick={() => setQuery({ type: 'upcoming', from: '', to: '', sort: '', page: '' })}>Upcoming</button>
    </div>
    <select class="select" value={country} onchange={(e) => set({ country: e.currentTarget.value })} aria-label="Country">
      {#each COUNTRIES as c (c.code)}<option value={c.code}>{c.name}</option>{/each}
    </select>
    <select class="select" style="max-width:220px" value={service} onchange={(e) => set({ service: e.currentTarget.value })} aria-label="Service">
      <optgroup label="Charted providers">
        {#each PROVIDERS as p (p.value)}<option value={p.value}>{p.label}</option>{/each}
      </optgroup>
      {#if otherServices.length}
        <optgroup label="All services in {country}">
          {#each otherServices as s (s.justwatch_package)}<option value={s.justwatch_package}>{s.name}</option>{/each}
        </optgroup>
      {/if}
    </select>
    <div class="segmented" role="group" aria-label="Kind">
      <button class:active={kind === ''} onclick={() => set({ kind: '' })}>All</button>
      <button class:active={kind === 'movie'} onclick={() => set({ kind: 'movie' })}>Movies</button>
      <button class:active={kind === 'tv'} onclick={() => set({ kind: 'tv' })}>TV</button>
    </div>
    <label class="row small muted">From <input class="input" type="date" value={from} onchange={(e) => set({ from: e.currentTarget.value })} /></label>
    <label class="row small muted">To <input class="input" type="date" value={to} onchange={(e) => set({ to: e.currentTarget.value })} /></label>
    <div class="search">
      <span class="icon"><Icon name="search" size={14} /></span>
      <input class="input" style="width:200px" type="search" placeholder="Filter title or genre" value={q} oninput={onSearch} />
    </div>
    {#if loading}<span class="spinner"></span>{/if}
  </div>

  {#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

  {#if data}
    <div class="row small muted" style="margin-bottom:10px;gap:14px">
      <span><strong style="color:var(--text)">{data.service.name}</strong> ({data.service.justwatch_package})</span>
      {#if data.from}<span>{dateOnly(data.from)} – {dateOnly(data.to)}</span>{/if}
      <span>{filtered.length} of {data.count} titles</span>
      {#if data.truncated}<span class="badge badge-warning">Truncated at 1000 per day</span>{/if}
    </div>
  {/if}

  <div class="card">
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th style="width:46px"></th>
            <SortTh label="Title" field="title" {sort} onsort={(s) => set({ sort: s })} />
            <th>Kind</th>
            <SortTh label={type === 'new' ? 'Added' : 'Release'} field="date" firstDir={type === 'new' ? 'desc' : 'asc'} {sort} onsort={(s) => set({ sort: s })} />
            <SortTh label="Year" field="year" firstDir="desc" {sort} onsort={(s) => set({ sort: s })} />
            <th>Genres</th>
            <SortTh label="IMDb" field="imdb" firstDir="desc" align="right" {sort} onsort={(s) => set({ sort: s })} />
            <th class="right">RT</th>
            <th>Links</th>
          </tr>
        </thead>
        <tbody>
          {#each pageItems as it (it.justwatch_id + (it.date ?? ''))}
            <tr>
              <td>{#if it.poster_url}<img class="poster" src={it.poster_url} alt="" loading="lazy" />{:else}<div class="poster"></div>{/if}</td>
              <td style="max-width:360px">
                <div class="cell-title">{it.title}{#if it.season_number} <span class="muted">· {it.season_title ?? `Season ${it.season_number}`}</span>{/if}</div>
                {#if it.description}<div class="cell-sub truncate" title={it.description}>{it.description}</div>{/if}
              </td>
              <td><KindBadge kind={it.kind} /></td>
              <td class="nowrap">{it.date ? dateOnly(it.date) : 'TBA'}{#if it.release_type && it.release_type !== 'DIGITAL'}<div class="cell-sub">{it.release_type.toLowerCase()}</div>{/if}</td>
              <td>{it.year ?? '—'}</td>
              <td class="small muted">{it.genres.slice(0, 3).join(', ')}</td>
              <td class="right nowrap">{it.imdb_rating ? it.imdb_rating.toFixed(1) : '—'}{#if it.imdb_votes}<div class="cell-sub">{number(it.imdb_votes)}</div>{/if}</td>
              <td class="right">{it.tomatometer != null ? `${it.tomatometer}%` : '—'}</td>
              <td class="nowrap small">
                {#if it.justwatch_url}<a href={it.justwatch_url} target="_blank" rel="noopener">JustWatch</a>{/if}
                {#if it.offer?.url} · <a href={it.offer.url} target="_blank" rel="noopener">Watch</a>{/if}
                {#if it.tmdb_id} · <a href={links.tmdb(it.kind, it.tmdb_id)} target="_blank" rel="noopener">TMDB</a>{/if}
              </td>
            </tr>
          {:else}
            <tr><td colspan="9"><div class="empty">{#if loading}Loading from JustWatch…{:else}<div class="empty-title">Nothing found</div>Try another service, country or date range.{/if}</div></td></tr>
          {/each}
        </tbody>
      </table>
    </div>
    <Pagination {page} {size} total={filtered.length} onchange={(p, s) => setQuery({ page: p === 1 ? '' : p, size: s === 50 ? '' : s })} />
  </div>
</div>
