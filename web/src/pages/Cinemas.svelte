<script>
  import PageHeader from '../components/PageHeader.svelte'
  import Drawer from '../components/Drawer.svelte'
  import Icon from '../components/Icon.svelte'
  import {
    listVenues, getVenue, createVenue, updateVenue, deleteVenue,
    addVenueAlias, deleteVenueAlias, createScreen, updateScreen, deleteScreen,
  } from '../lib/api.js'
  import { route, setQuery } from '../lib/router.svelte.js'
  import { toast } from '../lib/toast.svelte.js'
  import { confirm } from '../lib/confirm.svelte.js'

  // Cinemas are shared reference data: users add them while logging a watch,
  // and every watch keeps a snapshot of the name and city it was logged with,
  // so editing here never rewrites what a diary says.
  const STATUSES = [
    ['open', 'Open'], ['closed', 'Closed'], ['renamed', 'Renamed'], ['rebuilt', 'Rebuilt'],
  ]
  const statusBadge = { open: 'badge-success', closed: 'badge-warning', renamed: 'badge-info', rebuilt: 'badge-info' }

  const q = $derived(route.query.q ?? '')
  const country = $derived(route.query.country ?? '')
  const showClosed = $derived(route.query.closed === '1')
  const open = $derived(route.query.open ?? '')

  let venues = $state([])
  let loading = $state(true)
  let error = $state('')
  let search = $state(route.query.q ?? '')

  async function load() {
    loading = true
    try {
      const data = await listVenues({ q, country, include_closed: showClosed ? 'true' : '' })
      venues = data.venues ?? []
      error = ''
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }
  $effect(() => {
    q; country; showClosed
    load()
  })
  // Debounced search into the URL.
  $effect(() => {
    const v = search
    const t = setTimeout(() => { if (v !== q) setQuery({ q: v }, { replace: true }) }, 250)
    return () => clearTimeout(t)
  })

  // ── Drawer: one cinema, or a new one ──────────────────────────────
  const blankVenue = () => ({
    name: '', chain: '', address: '', city: '', region: '', country: '', timezone: '',
    latitude: '', longitude: '', opened_on: '', closed_on: '', status: 'open', note: '',
  })
  let detail = $state(null) // { venue, screens, aliases, watch_count }
  let form = $state(null)
  let saving = $state(false)

  $effect(() => {
    const id = open
    if (!id) { detail = null; form = null; return }
    if (id === 'new') {
      detail = { venue: null, screens: [], aliases: [], watch_count: 0 }
      form = blankVenue()
      return
    }
    let cancelled = false
    getVenue(id)
      .then((d) => { if (!cancelled) { detail = d; form = toForm(d.venue) } })
      .catch((e) => { if (!cancelled) { toast.error(e.message); setQuery({ open: '' }) } })
    return () => { cancelled = true }
  })

  function toForm(v) {
    const f = blankVenue()
    for (const k of Object.keys(f)) f[k] = v[k] ?? f[k]
    return f
  }
  const text = (v) => (String(v ?? '').trim() || null)
  const num = (v) => (String(v ?? '').trim() === '' ? null : Number(v))

  async function saveVenue() {
    saving = true
    try {
      const body = {
        name: form.name.trim(), chain: text(form.chain), address: text(form.address), city: text(form.city),
        region: text(form.region), country: text(form.country), timezone: text(form.timezone),
        latitude: num(form.latitude), longitude: num(form.longitude),
        opened_on: text(form.opened_on), closed_on: text(form.closed_on), status: form.status, note: text(form.note),
      }
      if (detail.venue) {
        const d = await updateVenue(detail.venue.id, body)
        detail = d
        form = toForm(d.venue)
        toast.success(`Saved ${body.name}`)
      } else {
        const d = await createVenue(body)
        toast.success(`Added ${body.name}`)
        setQuery({ open: d.venue.id })
      }
      load()
    } catch (e) {
      toast.error(e.message)
    } finally {
      saving = false
    }
  }

  async function removeVenue() {
    const v = detail.venue
    const ok = await confirm({
      title: `Delete ${v.name}?`,
      message: 'Nobody has logged a watch here, so it can go for good. This can’t be undone.',
      confirmLabel: 'Delete cinema',
      danger: true,
    })
    if (!ok) return
    try {
      await deleteVenue(v.id)
      toast.success(`Deleted ${v.name}`)
      setQuery({ open: '' })
      load()
    } catch (e) {
      toast.error(e.message)
    }
  }

  // Screens
  const blankScreen = () => ({ id: null, name: '', number: '', seats: '', width: '', height: '', dim_unit: 'm', aspect_ratio: '', effective_from: '', effective_to: '', note: '' })
  let screen = $state(null)
  function editScreen(s) {
    screen = s
      ? { ...blankScreen(), ...Object.fromEntries(Object.entries(s).map(([k, v]) => [k, v ?? ''])), aspect_ratio: s.aspect_ratio ? String(s.aspect_ratio) : '', dim_unit: s.dim_unit ?? 'm' }
      : blankScreen()
  }
  async function saveScreen() {
    const body = {
      name: screen.name.trim(), number: num(screen.number), seats: num(screen.seats),
      width: num(screen.width), height: num(screen.height), dim_unit: screen.width || screen.height ? screen.dim_unit : null,
      aspect_ratio: text(screen.aspect_ratio), effective_from: text(screen.effective_from), effective_to: text(screen.effective_to), note: text(screen.note),
    }
    try {
      if (screen.id) await updateScreen(detail.venue.id, screen.id, body)
      else await createScreen(detail.venue.id, body)
      toast.success(screen.id ? `Saved ${body.name}` : `Added ${body.name}`)
      screen = null
      detail = await getVenue(detail.venue.id)
      load()
    } catch (e) {
      toast.error(e.message)
    }
  }
  async function removeScreen(s) {
    const ok = await confirm({
      title: `Delete ${s.name}?`,
      message: 'Watches logged in this screen keep their screen size and shape, but lose the link to it.',
      confirmLabel: 'Delete screen',
      danger: true,
    })
    if (!ok) return
    try {
      await deleteScreen(detail.venue.id, s.id)
      detail = await getVenue(detail.venue.id)
      load()
    } catch (e) {
      toast.error(e.message)
    }
  }

  // Former names
  let alias = $state({ name: '', from_date: '', to_date: '' })
  async function addAlias() {
    try {
      await addVenueAlias(detail.venue.id, { name: alias.name.trim(), from_date: text(alias.from_date), to_date: text(alias.to_date) })
      alias = { name: '', from_date: '', to_date: '' }
      detail = await getVenue(detail.venue.id)
    } catch (e) {
      toast.error(e.message)
    }
  }
  async function removeAlias(a) {
    try {
      await deleteVenueAlias(detail.venue.id, a.id)
      detail = await getVenue(detail.venue.id)
    } catch (e) {
      toast.error(e.message)
    }
  }

  const size = (s) => (s.width && s.height ? `${s.width} × ${s.height} ${s.dim_unit ?? ''}` : '')
  const zones = typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf('timeZone') : []
</script>

<PageHeader
  title="Cinemas"
  desc="The cinemas and screens watches are logged at. Users add cinemas while logging; correct, complete or close them here."
  info="Shared by everyone. Each watch keeps the name and city it was logged with, so renaming a cinema never rewrites a diary. A cinema with watches can't be deleted: set its status to closed instead."
>
  {#snippet actions()}
    <button class="btn btn-primary" onclick={() => setQuery({ open: 'new' })}><Icon name="plus" /> Add cinema</button>
  {/snippet}
</PageHeader>

<div class="filters">
  <div class="search">
    <span class="icon"><Icon name="search" /></span>
    <input class="input" placeholder="Search by name or city" bind:value={search} aria-label="Search cinemas" />
  </div>
  <input class="input cc" placeholder="Country" maxlength="2" value={country} aria-label="Country code"
    onchange={(e) => setQuery({ country: e.currentTarget.value.trim().toUpperCase() }, { replace: true })} />
  <label class="check"><input type="checkbox" checked={showClosed} onchange={(e) => setQuery({ closed: e.currentTarget.checked ? '1' : '' }, { replace: true })} /> Include closed</label>
</div>

{#if error}<div class="alert-error">{error}</div>{/if}

<div class="card">
  {#if loading && !venues.length}
    <p class="muted">Loading cinemas…</p>
  {:else if !venues.length}
    <p class="muted">{q || country ? 'No cinemas match.' : 'No cinemas yet. Users add them while logging, or add one here.'}</p>
  {:else}
    <table class="table">
      <thead><tr><th>Name</th><th>City</th><th>Country</th><th>Time zone</th><th class="num">Screens</th><th>Status</th></tr></thead>
      <tbody>
        {#each venues as v (v.id)}
          <tr class="clickable" class:dim={v.status === 'closed'} onclick={() => setQuery({ open: v.id })}>
            <td><a href="#/cinemas?open={v.id}" onclick={(e) => { e.preventDefault(); setQuery({ open: v.id }) }}>{v.name}</a>{#if v.chain}<div class="muted">{v.chain}</div>{/if}</td>
            <td>{v.city ?? '—'}</td>
            <td>{v.country ?? '—'}</td>
            <td class="muted">{v.timezone ?? '—'}</td>
            <td class="num">{v.screen_count}</td>
            <td><span class="badge {statusBadge[v.status] ?? ''}">{v.status}</span></td>
          </tr>
        {/each}
      </tbody>
    </table>
    {#if venues.length === 200}<p class="muted">Showing the first 200; search to narrow it down.</p>{/if}
  {/if}
</div>

{#if form && detail}
  <Drawer onclose={() => { screen = null; setQuery({ open: '' }) }}>
    {#snippet header()}
      <div class="dh">{detail.venue ? detail.venue.name : 'Add a cinema'}</div>
      {#if detail.venue}<div class="muted">{detail.watch_count} {detail.watch_count === 1 ? 'watch' : 'watches'} logged here · id {detail.venue.id}</div>{/if}
    {/snippet}

    <section>
      <div class="grid2">
        <label class="wide">Name<input class="input" bind:value={form.name} required /></label>
        <label>Chain<input class="input" bind:value={form.chain} placeholder="e.g. PVR, Odeon" /></label>
        <label>Status
          <select class="input" bind:value={form.status}>{#each STATUSES as [v, l]}<option value={v}>{l}</option>{/each}</select>
        </label>
        <label class="wide">Address<input class="input" bind:value={form.address} /></label>
        <label>City<input class="input" bind:value={form.city} /></label>
        <label>Region<input class="input" bind:value={form.region} /></label>
        <label>Country<input class="input cc" bind:value={form.country} maxlength="2" placeholder="GB" /></label>
        <label>Time zone
          <input class="input" bind:value={form.timezone} list="tz-list" placeholder="Europe/London" />
          <datalist id="tz-list">{#each zones as z}<option value={z}></option>{/each}</datalist>
        </label>
        <label>Latitude<input class="input num" bind:value={form.latitude} inputmode="decimal" /></label>
        <label>Longitude<input class="input num" bind:value={form.longitude} inputmode="decimal" /></label>
        <label>Opened<input class="input" type="date" bind:value={form.opened_on} /></label>
        <label>Closed<input class="input" type="date" bind:value={form.closed_on} /></label>
        <label class="wide">Note<input class="input" bind:value={form.note} /></label>
      </div>
      <p class="muted small">The time zone files a watch at this cinema under the right local day; latitude and longitude go together.</p>
    </section>

    {#if detail.venue}
      <section>
        <h3>Screens <span class="muted">({detail.screens.length})</span></h3>
        {#if detail.screens.length}
          <table class="table compact">
            <thead><tr><th>Screen</th><th class="num">Seats</th><th>Size</th><th class="num">Ratio</th><th></th></tr></thead>
            <tbody>
              {#each detail.screens as s (s.id)}
                <tr>
                  <td>{s.name}{#if s.number}<span class="muted"> · no. {s.number}</span>{/if}{#if s.note}<div class="muted">{s.note}</div>{/if}</td>
                  <td class="num">{s.seats ?? '—'}</td>
                  <td>{size(s) || '—'}</td>
                  <td class="num">{s.aspect_ratio ? `${s.aspect_ratio.toFixed(2)}:1` : '—'}</td>
                  <td class="right">
                    <button class="btn btn-sm" onclick={() => editScreen(s)}>Edit</button>
                    <button class="btn btn-sm" onclick={() => removeScreen(s)} title="Delete screen"><Icon name="trash" /></button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
        {#if screen}
          <div class="sub-form">
            <div class="grid3">
              <label class="wide">Name<input class="input" bind:value={screen.name} placeholder="Screen 1, IMAX, Audi 3" /></label>
              <label>Number<input class="input num" bind:value={screen.number} inputmode="numeric" /></label>
              <label>Seats<input class="input num" bind:value={screen.seats} inputmode="numeric" /></label>
              <label>Width<input class="input num" bind:value={screen.width} inputmode="decimal" /></label>
              <label>Height<input class="input num" bind:value={screen.height} inputmode="decimal" /></label>
              <label>Unit<select class="input" bind:value={screen.dim_unit}><option value="m">metres</option><option value="ft">feet</option></select></label>
              <label>Aspect ratio<input class="input" bind:value={screen.aspect_ratio} placeholder="1.90:1" /></label>
              <label>From<input class="input" type="date" bind:value={screen.effective_from} /></label>
              <label>To<input class="input" type="date" bind:value={screen.effective_to} /></label>
              <label class="wide">Note<input class="input" bind:value={screen.note} /></label>
            </div>
            <div class="row-actions">
              <button class="btn" onclick={() => (screen = null)}>Cancel</button>
              <button class="btn btn-primary" disabled={!screen.name.trim()} onclick={saveScreen}>{screen.id ? 'Save screen' : 'Add screen'}</button>
            </div>
          </div>
        {:else}
          <button class="btn btn-sm" onclick={() => editScreen(null)}><Icon name="plus" /> Add screen</button>
        {/if}
      </section>

      <section>
        <h3>Former names</h3>
        {#if detail.aliases?.length}
          <ul class="aliases">
            {#each detail.aliases as a (a.id)}
              <li>
                <span>{a.name}</span>
                <span class="muted">{a.from_date ?? '…'} – {a.to_date ?? '…'}</span>
                <button class="btn btn-sm btn-ghost" onclick={() => removeAlias(a)} title="Remove"><Icon name="x" /></button>
              </li>
            {/each}
          </ul>
        {/if}
        <div class="alias-form">
          <input class="input" bind:value={alias.name} placeholder="Former name" aria-label="Former name" />
          <input class="input" type="date" bind:value={alias.from_date} aria-label="Used from" />
          <input class="input" type="date" bind:value={alias.to_date} aria-label="Used until" />
          <button class="btn btn-sm" disabled={!alias.name.trim()} onclick={addAlias}>Add</button>
        </div>
      </section>
    {/if}

    {#snippet footer()}
      {#if detail.venue && detail.watch_count === 0}
        <button class="btn btn-ghost danger" onclick={removeVenue}><Icon name="trash" /> Delete</button>
      {:else if detail.venue}
        <span class="muted small">Has watches: set the status to closed rather than deleting.</span>
      {/if}
      <span class="grow"></span>
      <button class="btn" onclick={() => setQuery({ open: '' })}>Close</button>
      <button class="btn btn-primary" disabled={saving || !form.name.trim()} onclick={saveVenue}>{saving ? 'Saving…' : detail.venue ? 'Save' : 'Add cinema'}</button>
    {/snippet}
  </Drawer>
{/if}

<style>
  .filters { display: flex; flex-wrap: wrap; align-items: center; gap: 0.75rem; margin-bottom: 1rem; }
  .filters .search { flex: 1; min-width: 220px; max-width: 420px; }
  .cc { width: 90px; text-transform: uppercase; }
  .check { display: inline-flex; align-items: center; gap: 0.4rem; font-size: 0.9rem; }
  .muted { color: var(--muted); font-size: 0.85rem; }
  .small { font-size: 0.8rem; }
  .dim td { opacity: 0.6; }
  .clickable { cursor: pointer; }
  .right { text-align: right; white-space: nowrap; }
  .dh { font-size: 1.05rem; font-weight: 600; }
  section { margin-bottom: 1.4rem; }
  h3 { margin: 0 0 0.6rem; font-size: 0.95rem; }
  .grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: 0.7rem 0.9rem; }
  .grid3 { display: grid; grid-template-columns: repeat(3, 1fr); gap: 0.6rem 0.8rem; }
  .grid2 label, .grid3 label { display: grid; gap: 0.25rem; font-size: 0.8rem; color: var(--muted); }
  .wide { grid-column: 1 / -1; }
  .sub-form { padding: 0.8rem; border: 1px solid var(--border, #ddd); border-radius: 8px; margin-top: 0.6rem; }
  .row-actions { display: flex; justify-content: flex-end; gap: 0.5rem; margin-top: 0.7rem; }
  .aliases { list-style: none; margin: 0 0 0.6rem; padding: 0; display: grid; gap: 0.3rem; }
  .aliases li { display: flex; align-items: center; gap: 0.7rem; }
  .alias-form { display: grid; grid-template-columns: 1fr 140px 140px auto; gap: 0.5rem; }
  .grow { flex: 1; }
  .danger { color: var(--danger, #c4281c); }
  .compact td, .compact th { padding-top: 0.35rem; padding-bottom: 0.35rem; }
</style>
