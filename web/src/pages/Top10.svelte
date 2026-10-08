<script>
  import { getTop10 } from '../lib/api.js'
  import { route, setQuery } from '../lib/router.svelte.js'
  import { dateOnly, addDays, todayUTC, links, downloadCSV } from '../lib/format.js'
  import { COUNTRIES, PROVIDERS, providerLabel } from '../lib/constants.js'
  import Icon from '../components/Icon.svelte'
  import SortTh from '../components/SortTh.svelte'
  import KindBadge from '../components/KindBadge.svelte'
  import IdCell from '../components/IdCell.svelte'
  import TitleDrawer from './TitleDrawer.svelte'

  const country = $derived(route.query.country ?? 'US')
  const provider = $derived(route.query.provider ?? '')
  const category = $derived(route.query.category ?? 'movies')
  const date = $derived(route.query.date ?? '') // '' = latest
  const sort = $derived(route.query.sort ?? 'rank')
  const openId = $derived(route.query.open ? Number(route.query.open) : null)

  let data = $state({ items: [], date: null })
  let loading = $state(false)
  let error = $state('')

  $effect(() => {
    const params = { country, provider, category, date }
    loading = true
    let cancelled = false
    getTop10(params)
      .then((d) => { if (!cancelled) { data = d; error = '' } })
      .catch((e) => { if (!cancelled) { error = e.message; data = { items: [], date: null } } })
      .finally(() => { if (!cancelled) loading = false })
    return () => { cancelled = true }
  })

  const shownDate = $derived(date || data.date || '')

  function step(days) {
    const base = shownDate || todayUTC()
    setQuery({ date: addDays(base, days) })
  }

  // movement: positive = climbed, null = new entry
  const movement = (it) => (it.previous_rank ? it.previous_rank - it.rank : null)

  const sortValue = {
    rank: (it) => it.provider + String(it.rank).padStart(2, '0'),
    name: (it) => it.name.toLowerCase(),
    days: (it) => it.days_in_top10,
    move: (it) => movement(it) ?? 100,
  }
  const items = $derived.by(() => {
    const field = sort.replace(/^-/, '')
    const get = sortValue[field] ?? sortValue.rank
    const dir = sort.startsWith('-') ? -1 : 1
    return [...data.items].sort((a, b) => (get(a) > get(b) ? dir : get(a) < get(b) ? -dir : 0))
  })

  const newEntries = $derived(data.items.filter((it) => !it.previous_rank).length)
  const unmapped = $derived(data.items.filter((it) => !it.tmdb_id).length)

  function exportCSV() {
    downloadCSV(`top10-${country}-${provider || 'all'}-${category}-${shownDate}.csv`, [
      { key: 'date', label: 'date' }, { key: 'provider', label: 'provider' }, { key: 'rank', label: 'rank' },
      { key: 'previous_rank', label: 'previous_rank' }, { key: 'days_in_top10', label: 'days_in_top10' },
      { key: 'name', label: 'name' }, { key: 'slug', label: 'slug' }, { key: 'kind', label: 'kind' },
      { key: 'tmdb_id', label: 'tmdb_id' }, { key: 'imdb_id', label: 'imdb_id' }, { key: 'rotten_tomatoes_url', label: 'rt_slug' },
    ], items)
  }
</script>

<div class="page">
  <div class="page-header">
    <div>
      <h1 class="page-title">Top 10 charts</h1>
      <p class="page-desc">Daily FlixPatrol charts with rank movement and days on chart. Click a title to check or fix its mapping.</p>
    </div>
    <div class="page-actions">
      <button class="btn" onclick={exportCSV} disabled={!items.length}><Icon name="download" />Export CSV</button>
    </div>
  </div>

  <div class="toolbar">
    <select class="select" value={country} onchange={(e) => setQuery({ country: e.currentTarget.value })} aria-label="Country">
      {#each COUNTRIES as c (c.code)}<option value={c.code}>{c.name}</option>{/each}
    </select>
    <select class="select" value={provider} onchange={(e) => setQuery({ provider: e.currentTarget.value })} aria-label="Provider">
      <option value="">All providers</option>
      {#each PROVIDERS as p (p.value)}<option value={p.value}>{p.label}</option>{/each}
    </select>
    <div class="segmented" role="group" aria-label="Category">
      <button class:active={category === 'movies'} onclick={() => setQuery({ category: 'movies' })}>Movies</button>
      <button class:active={category === 'tv_shows'} onclick={() => setQuery({ category: 'tv_shows' })}>TV shows</button>
    </div>
    <div class="row" style="gap:4px">
      <button class="btn btn-icon" onclick={() => step(-1)} title="Previous day"><Icon name="chevronLeft" /></button>
      <input class="input" type="date" value={shownDate} max={todayUTC()} onchange={(e) => setQuery({ date: e.currentTarget.value })} aria-label="Chart date" />
      <button class="btn btn-icon" onclick={() => step(1)} title="Next day" disabled={!date}><Icon name="chevronRight" /></button>
      <button class="btn" class:btn-primary={!date} onclick={() => setQuery({ date: '' })}>Latest</button>
    </div>
    <span class="spacer"></span>
    {#if loading}<span class="spinner"></span>{/if}
  </div>

  {#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

  {#if items.length}
    <div class="row small muted" style="margin-bottom:10px;gap:14px">
      <span>Chart date <strong style="color:var(--text)">{dateOnly(shownDate)}</strong>{date ? '' : ' (latest)'}</span>
      <span>{newEntries} new {newEntries === 1 ? 'entry' : 'entries'}</span>
      {#if unmapped}<a href="#/titles?missing=tmdb"><span class="badge badge-warning">{unmapped} not mapped to TMDB</span></a>{/if}
    </div>
  {/if}

  <div class="card">
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <SortTh label="#" field="rank" {sort} onsort={(s) => setQuery({ sort: s })} />
            <SortTh label="Move" field="move" firstDir="desc" {sort} onsort={(s) => setQuery({ sort: s })} title="Change since the previous scraped chart" />
            <SortTh label="Title" field="name" {sort} onsort={(s) => setQuery({ sort: s })} />
            <th>Kind</th>
            {#if !provider}<th>Provider</th>{/if}
            <SortTh label="Days" field="days" firstDir="desc" align="right" {sort} onsort={(s) => setQuery({ sort: s })} title="Days on this chart so far" />
            <th>TMDB</th>
            <th>IMDb</th>
            <th>Rotten Tomatoes</th>
          </tr>
        </thead>
        <tbody>
          {#each items as it (it.provider + it.rank)}
            {@const m = movement(it)}
            <tr class="clickable" class:selected={openId === it.title_id} onclick={() => setQuery({ open: it.title_id })}>
              <td class="rank">{it.rank}</td>
              <td class="nowrap" title={it.previous_rank ? `Was #${it.previous_rank}` : 'Not on the previous chart'}>
                {#if m === null}<span class="move-new">NEW</span>
                {:else if m > 0}<span class="move-up">▲ {m}</span>
                {:else if m < 0}<span class="move-down">▼ {-m}</span>
                {:else}<span class="muted">—</span>{/if}
              </td>
              <td>
                <div class="cell-title">{it.name}</div>
                <div class="cell-sub mono">{it.slug}</div>
              </td>
              <td><KindBadge kind={it.kind} /></td>
              {#if !provider}<td class="nowrap">{providerLabel(it.provider)}{#if it.date !== shownDate}<div class="cell-sub">{dateOnly(it.date)}</div>{/if}</td>{/if}
              <td class="right">{it.days_in_top10}</td>
              <td><IdCell value={it.tmdb_id} url={links.tmdb(it.kind, it.tmdb_id)} /></td>
              <td><IdCell value={it.imdb_id} url={links.imdb(it.imdb_id)} /></td>
              <td><IdCell value={it.rotten_tomatoes_url} url={links.rt(it.rotten_tomatoes_url)} /></td>
            </tr>
          {:else}
            <tr><td colspan="9">
              <div class="empty">
                {#if loading}Loading…{:else}<div class="empty-title">No chart for this selection</div>Try another date, or check that a schedule scrapes this country and provider.{/if}
              </div>
            </td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
</div>

{#if openId}
  <TitleDrawer id={openId} onclose={() => setQuery({ open: '' })} onsaved={(t) => (data.items = data.items.map((it) => (it.title_id === t.id ? { ...it, kind: t.kind, tmdb_id: t.tmdb_id, imdb_id: t.imdb_id, rotten_tomatoes_url: t.rt_url } : it)))} />
{/if}
