<script>
  import { listMappings, syncMappings } from '../lib/api.js'
  import { route, setQuery, intParam } from '../lib/router.svelte.js'
  import { dateOnly, links, downloadCSV, ID_SOURCES } from '../lib/format.js'
  import { toast } from '../lib/toast.svelte.js'
  import Icon from '../components/Icon.svelte'
  import Pagination from '../components/Pagination.svelte'
  import KindBadge from '../components/KindBadge.svelte'
  import MappingDrawer from './MappingDrawer.svelte'

  const FLAGS = {
    conflict: 'Conflicts (linked titles disagree)',
    cross_kind: 'Listed as the other kind by a source',
    both: 'On FlixPatrol and Netflix',
    netflix_only: 'Netflix Top 10 only',
    flixpatrol_only: 'FlixPatrol only',
    manual: 'Fixed by hand',
  }

  const q = $derived(route.query.q ?? '')
  const kind = $derived(route.query.kind ?? '')
  const flag = $derived(route.query.flag ?? '')
  const page = $derived(intParam(route.query.page, 1))
  const size = $derived(intParam(route.query.size, 50))
  const open = $derived(route.query.open ?? '')

  let data = $state({ items: [], total: 0 })
  let loading = $state(false)
  let error = $state('')
  let reload = $state(0)
  let syncing = $state(false)

  $effect(() => {
    const params = { q, kind, flag, limit: size, offset: (page - 1) * size }
    void reload
    loading = true
    let cancelled = false
    listMappings(params)
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
    [q && ['q', `Search: ${q}`], kind && ['kind', kind === 'movie' ? 'Movies' : 'TV shows'], flag && ['flag', FLAGS[flag]]].filter(Boolean)
  )

  async function sync() {
    syncing = true
    try {
      const r = await syncMappings()
      toast.success(`${r.links} links (${r.links_added} new, ${r.links_removed} removed); ${r.netflix_titles_updated} Netflix and ${r.flixpatrol_titles_updated} FlixPatrol titles took a linked mapping${r.conflicts ? `; ${r.conflicts} conflict(s)` : ''}`)
      reload++
    } catch (e) {
      toast.error(e.message)
    } finally {
      syncing = false
    }
  }

  const entryName = (e) => e.title ?? e.flixpatrol_titles[0]?.name ?? e.netflix_titles[0]?.show_title ?? `TMDB ${e.tmdb_id}`

  function exportCSV() {
    downloadCSV('tmdb-mapping.csv', [
      { key: 'tmdb_kind', label: 'tmdb_kind' }, { key: 'tmdb_id', label: 'tmdb_id' }, { key: 'title', label: 'title' }, { key: 'year', label: 'year' },
      { key: 'imdb_id', label: 'imdb_id' }, { key: 'wikidata_id', label: 'wikidata_id' }, { key: 'rotten_tomatoes_id', label: 'rotten_tomatoes_id' },
      { key: 'netflix_id', label: 'netflix_id' }, { key: 'justwatch_id', label: 'justwatch_id' },
      { key: 'flixpatrol_slugs', label: 'flixpatrol_slugs' }, { key: 'netflix_top10_titles', label: 'netflix_top10_titles' },
    ], data.items.map((e) => ({ ...e, ...e.ids, flixpatrol_slugs: e.ids.flixpatrol_slugs.join('|'), netflix_top10_titles: e.ids.netflix_top10_titles.join('|') })))
  }

  // An ID badge's tooltip: the ID and where it came from.
  function idTitle(e, key, value) {
    const src = e.ids.sources?.[key]
    return src ? `${value} (${ID_SOURCES[src]?.label ?? src})` : value
  }
</script>

<div>
  <div class="subhead">
    <p class="tab-hint">One row per TMDB title with every ID and every chart title mapped to it. FlixPatrol and Netflix titles that are the same show are linked and share one mapping. Click a row to see or move everything mapped to it.</p>
    <div class="page-actions">
      <button class="btn" onclick={sync} disabled={syncing} title="Recompute links between Netflix and FlixPatrol titles and share mappings over them"><Icon name="refresh" />{syncing ? 'Syncing…' : 'Sync links now'}</button>
      <button class="btn" onclick={exportCSV} disabled={!data.items.length}><Icon name="download" />Export page</button>
    </div>
  </div>

  <div class="toolbar">
    <div class="search">
      <span class="icon"><Icon name="search" size={14} /></span>
      <input class="input" type="search" placeholder="Title, slug, Netflix name, TMDB or IMDb ID" value={q} oninput={onSearch} aria-label="Search" />
    </div>
    <div class="segmented" role="group" aria-label="TMDB kind">
      <button class:active={kind === ''} onclick={() => set({ kind: '' })}>All</button>
      <button class:active={kind === 'movie'} onclick={() => set({ kind: 'movie' })}>Movies</button>
      <button class:active={kind === 'tv'} onclick={() => set({ kind: 'tv' })}>TV</button>
    </div>
    <select class="select" value={flag} onchange={(e) => set({ flag: e.currentTarget.value })} aria-label="Filter">
      <option value="">Everything</option>
      {#each Object.entries(FLAGS) as [value, label] (value)}<option {value}>{label}</option>{/each}
    </select>
    <span class="spacer"></span>
    {#if loading}<span class="spinner"></span>{/if}
  </div>

  {#if activeFilters.length}
    <div class="filter-chips">
      {#each activeFilters as [key, label] (key)}
        <span class="chip">{label}<button onclick={() => set({ [key]: '' })} aria-label="Remove filter"><Icon name="x" size={11} /></button></span>
      {/each}
      <button class="btn btn-ghost btn-sm" onclick={() => set({ q: '', kind: '', flag: '' })}>Clear all</button>
    </div>
  {/if}

  {#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

  <div class="card">
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>TMDB title</th>
            <th>Kind</th>
            <th>IDs</th>
            <th>FlixPatrol</th>
            <th>Netflix Top 10</th>
            <th>Flags</th>
            <th>Last seen</th>
          </tr>
        </thead>
        <tbody>
          {#each data.items as e (e.tmdb_kind + e.tmdb_id)}
            {@const key = `${e.tmdb_kind}:${e.tmdb_id}`}
            <tr class="clickable" class:selected={open === key} onclick={() => setQuery({ open: key })}>
              <td>
                <div class="cell-title">{entryName(e)}{e.year ? ` (${e.year})` : ''}</div>
                <div class="cell-sub mono"><a href={links.tmdb(e.tmdb_kind, e.tmdb_id)} target="_blank" rel="noopener" onclick={(ev) => ev.stopPropagation()}>TMDB {e.tmdb_id}</a>{#if !e.title}<span class="muted" title="The TMDB metadata job fills it in"> · TMDB details pending</span>{/if}</div>
              </td>
              <td><KindBadge kind={e.tmdb_kind === 'tv' ? 'tv_show' : 'movie'} /></td>
              <td class="small nowrap">
                {#if e.ids.imdb_id}<span class="badge" title={idTitle(e, 'imdb', e.ids.imdb_id)}>IMDb</span>{/if}
                {#if e.ids.wikidata_id}<span class="badge" title={idTitle(e, 'wikidata', e.ids.wikidata_id)}>Wikidata</span>{/if}
                {#if e.ids.rotten_tomatoes_id}<span class="badge" title={idTitle(e, 'rotten_tomatoes', e.ids.rotten_tomatoes_id)}>RT</span>{/if}
                {#if e.ids.netflix_id}<span class="badge" title={idTitle(e, 'netflix', `Netflix ${e.ids.netflix_id}`)}>Netflix</span>{/if}
                {#if e.ids.justwatch_id}<span class="badge" title={idTitle(e, 'justwatch', e.ids.justwatch_id)}>JustWatch</span>{/if}
              </td>
              <td class="small">
                {#each e.flixpatrol_titles.slice(0, 2) as t (t.id)}<div class="mono truncate" style="max-width:220px">{t.slug}</div>{/each}
                {#if e.flixpatrol_titles.length > 2}<span class="muted">+{e.flixpatrol_titles.length - 2} more</span>{/if}
                {#if !e.flixpatrol_titles.length}<span class="muted">—</span>{/if}
              </td>
              <td class="small">
                {#each e.netflix_titles.slice(0, 2) as n (n.id)}
                  <div class="truncate" style="max-width:220px">{n.show_title} <span class="muted">({n.list === 'tv_show' ? 'TV' : 'Films'})</span></div>
                {/each}
                {#if e.netflix_titles.length > 2}<span class="muted">+{e.netflix_titles.length - 2} more</span>{/if}
                {#if !e.netflix_titles.length}<span class="muted">—</span>{/if}
              </td>
              <td class="nowrap">
                {#if e.conflicts}<span class="badge badge-danger" title="Linked titles are mapped to another TMDB title">conflict</span>{/if}
                {#if e.cross_kind}<span class="badge badge-info" title="A source lists it as the other kind">cross-kind</span>{/if}
                {#if e.manual}<span class="badge badge-accent" title="{e.manual} title(s) fixed by hand">manual</span>{/if}
              </td>
              <td class="nowrap small">{e.last_seen ? dateOnly(e.last_seen) : '—'}</td>
            </tr>
          {:else}
            <tr><td colspan="7">
              <div class="empty">
                {#if loading}Loading…{:else}<div class="empty-title">Nothing mapped matches</div>Try a different search or clear the filters.{/if}
              </div>
            </td></tr>
          {/each}
        </tbody>
      </table>
    </div>
    <Pagination {page} {size} total={data.total} onchange={(p, s) => setQuery({ page: p === 1 ? '' : p, size: s === 50 ? '' : s })} />
  </div>
  <p class="field-hint">Links and shared mappings are refreshed by the title matching, Netflix import and Netflix matching jobs. Precedence inside a linked group: a title fixed by hand, then a Netflix title mapped by its Netflix ID, then FlixPatrol's match (made with the title page's year), then Netflix's. Netflix IDs come from Tudum's Top 10 search and Wikidata (P1874).</p>
</div>

{#if open}
  <MappingDrawer refKey={open} onclose={() => setQuery({ open: '' })} onchanged={(key) => { reload++; if (key) setQuery({ open: key }) }} />
{/if}
