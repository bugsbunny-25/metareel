<script>
  import { listNetflixTitles, runJob } from '../lib/api.js'
  import { route, setQuery, intParam, href } from '../lib/router.svelte.js'
  import { dateOnly, dateTime, links, downloadCSV } from '../lib/format.js'
  import { toast } from '../lib/toast.svelte.js'
  import Icon from '../components/Icon.svelte'
  import Pagination from '../components/Pagination.svelte'
  import SortTh from '../components/SortTh.svelte'
  import KindBadge from '../components/KindBadge.svelte'
  import IdCell from '../components/IdCell.svelte'
  import NetflixTitleDrawer from './NetflixTitleDrawer.svelte'

  // UI sort ("field" / "-field") → server sort names.
  const SERVER_SORT = {
    name: 'name_asc', '-name': 'name_desc',
    weeks: 'weeks_asc', '-weeks': 'weeks_desc',
    best: 'best_rank_asc', '-best': 'best_rank_asc',
    last_week: 'last_week_asc', '-last_week': '',
    id: 'id_desc', '-id': 'id_desc',
  }
  const MATCH_BADGE = { matched: 'badge-success', manual: 'badge-accent', unmatched: 'badge-warning', pending: '' }

  const q = $derived(route.query.q ?? '')
  const kind = $derived(route.query.kind ?? '')
  const missing = $derived(route.query.missing ?? '')
  const flag = $derived(route.query.flag ?? '')
  const matchStatus = $derived(route.query.match ?? '')
  const sort = $derived(route.query.sort ?? '-last_week')
  const page = $derived(intParam(route.query.page, 1))
  const size = $derived(intParam(route.query.size, 50))
  const openId = $derived(route.query.open ? Number(route.query.open) : null)

  let data = $state({ items: [], total: 0 })
  let loading = $state(false)
  let error = $state('')
  let searchInput = $state()
  let running = $state(false)

  $effect(() => {
    const params = { q, kind, missing, flag, match_status: matchStatus, sort: SERVER_SORT[sort] ?? '', limit: size, offset: (page - 1) * size }
    loading = true
    let cancelled = false
    listNetflixTitles(params)
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
  const MISSING_LABEL = { tmdb: 'Missing TMDB ID', season: 'Season without a number' }
  const FLAG_LABEL = { cross_kind: 'TMDB kind differs from list', linked: 'Linked to FlixPatrol' }
  const activeFilters = $derived(
    [q && ['q', `Search: ${q}`], kind && ['kind', kind === 'movie' ? 'Films' : 'TV'], missing && ['missing', MISSING_LABEL[missing]], flag && ['flag', FLAG_LABEL[flag]], matchStatus && ['match', `Match: ${matchStatus}`]].filter(Boolean)
  )

  function onkeydown(e) {
    if (e.key === '/' && document.activeElement?.tagName !== 'INPUT') {
      e.preventDefault()
      searchInput?.focus()
    }
  }

  async function matchBacklog() {
    running = true
    try {
      await runJob('netflix.titles.match')
      toast.success('Netflix title matching queued — see Job runs')
    } catch (e) {
      toast.error(e.status === 409 ? 'Netflix title matching is already queued or running' : e.message)
    } finally {
      running = false
    }
  }

  function exportCSV() {
    downloadCSV('netflix-titles.csv', [
      { key: 'id', label: 'id' }, { key: 'show_title', label: 'show_title' }, { key: 'kind', label: 'list' },
      { key: 'tmdb_kind', label: 'tmdb_kind' }, { key: 'tmdb_id', label: 'tmdb_id' }, { key: 'imdb_id', label: 'imdb_id' }, { key: 'match_status', label: 'match_status' },
      { key: 'match_source', label: 'match_source' }, { key: 'seasons', label: 'season_titles' }, { key: 'seasons_unmapped', label: 'seasons_unmapped' },
      { key: 'weeks_in_global_top10', label: 'weeks_global' }, { key: 'best_global_rank', label: 'best_global_rank' }, { key: 'last_week', label: 'last_week' },
    ], data.items)
  }

  function onSaved(updated) {
    data.items = data.items.map((t) => (t.id === updated.id ? { ...t, ...updated } : t))
  }
</script>

<svelte:window {onkeydown} />

<div>
  <div class="subhead">
    <p class="tab-hint">Titles from Netflix's own weekly Top 10, the TMDB title each maps to and which season each season title means. Titles linked to a FlixPatrol title share its mapping. Click a row to review or fix it.</p>
    <div class="page-actions">
      <button class="btn" onclick={matchBacklog} disabled={running}><Icon name="play" />Match backlog now</button>
      <button class="btn" onclick={exportCSV} disabled={!data.items.length}><Icon name="download" />Export page</button>
    </div>
  </div>

  <div class="toolbar">
    <div class="search">
      <span class="icon"><Icon name="search" size={14} /></span>
      <input bind:this={searchInput} class="input" type="search" placeholder="Search show title or TMDB ID  ( / )" value={q} oninput={onSearch} />
    </div>
    <div class="segmented" role="group" aria-label="Kind">
      <button class:active={kind === ''} onclick={() => set({ kind: '' })}>All</button>
      <button class:active={kind === 'movie'} onclick={() => set({ kind: 'movie' })}>Films</button>
      <button class:active={kind === 'tv_show'} onclick={() => set({ kind: 'tv_show' })}>TV</button>
    </div>
    <select class="select" value={missing} onchange={(e) => set({ missing: e.currentTarget.value })} aria-label="Mapping filter">
      <option value="">Any mapping</option>
      <option value="tmdb">Missing TMDB ID</option>
      <option value="season">Season without a number</option>
    </select>
    <select class="select" value={flag} onchange={(e) => set({ flag: e.currentTarget.value })} aria-label="Flag filter">
      <option value="">Any link</option>
      <option value="linked">Linked to FlixPatrol</option>
      <option value="cross_kind">TMDB kind differs from list</option>
    </select>
    <select class="select" value={matchStatus} onchange={(e) => set({ match: e.currentTarget.value })} aria-label="Match status">
      <option value="">Any match status</option>
      <option value="unmatched">Unmatched (review queue)</option>
      <option value="pending">Pending (not tried yet)</option>
      <option value="matched">Matched automatically</option>
      <option value="manual">Fixed by hand</option>
    </select>
    <span class="spacer"></span>
    {#if loading}<span class="spinner"></span>{/if}
  </div>

  {#if activeFilters.length}
    <div class="filter-chips">
      {#each activeFilters as [key, label] (key)}
        <span class="chip">{label}<button onclick={() => set({ [key]: '' })} aria-label="Remove filter"><Icon name="x" size={11} /></button></span>
      {/each}
      <button class="btn btn-ghost btn-sm" onclick={() => set({ q: '', kind: '', missing: '', flag: '', match: '' })}>Clear all</button>
    </div>
  {/if}

  {#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

  <div class="card">
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <SortTh label="Title" field="name" {sort} onsort={(s) => set({ sort: s })} />
            <th title="Netflix's list (and the TMDB kind when it differs)">List</th>
            <th>Match</th>
            <th>TMDB</th>
            <th>IMDb</th>
            <th>Seasons</th>
            <SortTh label="Global weeks" field="weeks" firstDir="desc" align="right" title="Weeks in Netflix's global Top 10" {sort} onsort={(s) => set({ sort: s })} />
            <SortTh label="Best" field="best" align="right" title="Best global rank" {sort} onsort={(s) => set({ sort: s })} />
            <SortTh label="Last week" field="last_week" firstDir="desc" {sort} onsort={(s) => set({ sort: s })} />
            <th>FlixPatrol</th>
          </tr>
        </thead>
        <tbody>
          {#each data.items as t (t.id)}
            <tr class="clickable" class:selected={openId === t.id} onclick={() => setQuery({ open: t.id })}>
              <td>
                <div class="cell-title">{t.show_title}</div>
                {#if t.tmdb_title}<div class="cell-sub">TMDB: {t.tmdb_title}{t.tmdb_year ? ` (${t.tmdb_year})` : ''}</div>{/if}
              </td>
              <td class="nowrap">
                <KindBadge kind={t.kind} />
                {#if t.tmdb_kind && t.tmdb_kind !== (t.kind === 'tv_show' ? 'tv' : 'movie')}<span class="badge badge-info" title="Mapped to a TMDB {t.tmdb_kind}">→ TMDB {t.tmdb_kind === 'tv' ? 'TV' : 'movie'}</span>{/if}
              </td>
              <td class="nowrap" title={t.next_match_at ? `Next attempt ${dateTime(t.next_match_at)}` : (t.match_source ?? '')}>
                <span class="badge {MATCH_BADGE[t.match_status] ?? ''}">{t.match_status}</span>
                {#if t.match_status === 'unmatched'}<span class="muted small"> ×{t.match_attempts}</span>{/if}
              </td>
              <td><IdCell value={t.tmdb_id} url={links.tmdb(t.tmdb_kind ?? t.kind, t.tmdb_id)} /></td>
              <td><IdCell value={t.imdb_id} url={links.imdb(t.imdb_id)} /></td>
              <td class="nowrap">
                {#if t.kind !== 'tv_show' || !t.seasons}<span class="muted">—</span>
                {:else if t.seasons_unmapped}<span class="badge badge-warning" title="{t.seasons_unmapped} season title(s) without a season number">{t.seasons - t.seasons_unmapped}/{t.seasons} mapped</span>
                {:else}<span class="badge badge-success">{t.seasons} mapped</span>{/if}
              </td>
              <td class="right">{t.weeks_in_global_top10 || '—'}</td>
              <td class="right">{t.best_global_rank ? `#${t.best_global_rank}` : '—'}</td>
              <td class="nowrap">{t.last_week ? dateOnly(t.last_week) : '—'}</td>
              <td class="small">
                {#if t.flixpatrol}<a href={href('/titles', { q: t.flixpatrol.slug, open: t.flixpatrol.title_id })} onclick={(e) => e.stopPropagation()} title="Matched through this FlixPatrol title">{t.flixpatrol.slug}</a>{#if t.flixpatrol_links > 1} <span class="muted">+{t.flixpatrol_links - 1}</span>{/if}
                {:else if t.flixpatrol_links}<span class="badge" title="Linked FlixPatrol titles">{t.flixpatrol_links} linked</span>
                {:else}<span class="muted">—</span>{/if}
              </td>
            </tr>
          {:else}
            <tr><td colspan="10">
              <div class="empty">
                {#if loading}Loading…{:else}<div class="empty-title">No Netflix titles match</div>Try a different search or clear the filters. Titles appear after the "Netflix official Top 10" job has run.{/if}
              </div>
            </td></tr>
          {/each}
        </tbody>
      </table>
    </div>
    <Pagination {page} {size} total={data.total} onchange={(p, s) => setQuery({ page: p === 1 ? '' : p, size: s === 50 ? '' : s })} />
  </div>
  <p class="field-hint">Matching runs after each Netflix import and daily as "Netflix title matching": a linked FlixPatrol title's mapping first, then the title's Netflix ID (Tudum's Top 10 search → TMDB via Wikidata), then Wikidata by name (titles with a Netflix ID), then JustWatch's Netflix catalogue — as either TMDB kind, since Netflix lists specials as TV. Unmatched titles are retried after 1, 3, 7, then every 30 days. Titles fixed by hand are never changed automatically.</p>
</div>

{#if openId}
  <NetflixTitleDrawer id={openId} onclose={() => setQuery({ open: '' })} onsaved={onSaved} />
{/if}
