<!-- Pick the cinema (and screen) a watch happened at, from the shared venue list, or add one.
     Venues are shared reference data, so a cinema added here is there for everyone; the diary
     snapshots its name and city, so a later rename never rewrites a past watch. -->
<script>
  import { untrack } from 'svelte'
  import { api } from '../lib/api.js'
  import { recall } from '../lib/money.js'

  /** venue: { id, name, city?, timezone? } | null; screenId: number | null; country: default for new venues. */
  let { venue = $bindable(null), screenId = $bindable(null), country = 'US' } = $props()

  let q = $state('')
  let results = $state([])
  let searching = $state(false)
  let error = $state('')
  let adding = $state(false)
  let draft = $state({ name: '', city: '', country: '', timezone: '' })
  let screens = $state([])
  let addingScreen = $state(false)
  let screenName = $state('')
  let busy = $state(false)
  const recent = recall('venues', [])

  // Search as you type, debounced; older requests are aborted.
  $effect(() => {
    const term = q.trim()
    if (venue || term.length < 2) { results = []; return }
    const ctrl = new AbortController()
    const t = setTimeout(() => {
      searching = true
      api.venues(term, undefined, ctrl.signal)
        .then((r) => { results = r.venues ?? []; error = '' })
        .catch((err) => { if (err.name !== 'AbortError') error = err.message })
        .finally(() => (searching = false))
    }, 250)
    return () => { clearTimeout(t); ctrl.abort() }
  })

  // A chosen venue's screens.
  $effect(() => {
    const id = venue?.id
    if (!id) { screens = []; return }
    const ctrl = new AbortController()
    api.venue(id, ctrl.signal)
      .then((r) => {
        screens = r.screens ?? []
        // Recent picks and past watches don't carry the zone; the venue record does.
        const tz = r.venue?.timezone ?? null
        if (tz && untrack(() => venue?.id === id && !venue.timezone)) venue = { ...untrack(() => venue), timezone: tz }
      })
      .catch(() => (screens = []))
    return () => ctrl.abort()
  })

  function choose(v) {
    venue = { id: v.id, name: v.name, chain: v.chain ?? null, city: v.city ?? null, timezone: v.timezone ?? null }
    screenId = null
    q = ''
    adding = false
  }
  function change() {
    venue = null
    screenId = null
  }
  function startAdding() {
    draft = { name: q.trim(), city: '', country, timezone: Intl.DateTimeFormat().resolvedOptions().timeZone }
    adding = true
  }
  async function create() {
    if (!draft.name.trim()) { error = 'A cinema needs a name.'; return }
    busy = true
    error = ''
    try {
      const r = await api.createVenue({
        name: draft.name.trim(), city: draft.city.trim() || null,
        country: draft.country.trim().toUpperCase() || null, timezone: draft.timezone.trim() || null,
      })
      choose(r.venue)
    } catch (err) {
      error = err.message
    } finally {
      busy = false
    }
  }
  async function addScreen() {
    if (!screenName.trim()) return
    busy = true
    error = ''
    try {
      const s = await api.createScreen(venue.id, { name: screenName.trim() })
      const screen = s.screen ?? s
      screens = [...screens, screen]
      screenId = screen.id
      screenName = ''
      addingScreen = false
    } catch (err) {
      error = err.message
    } finally {
      busy = false
    }
  }
  const place = (v) => [v.city, v.country].filter(Boolean).join(', ')
</script>

<div class="vp">
  {#if venue}
    <div class="chosen">
      <div><b>{venue.name}</b>{#if venue.city}<span class="hint">{' · '}{venue.city}</span>{/if}</div>
      <button type="button" class="link" onclick={change}>Change</button>
    </div>
    <label class="sub">Screen
      <select value={addingScreen ? '+' : (screenId ?? '')} onchange={(e) => {
        const v = e.currentTarget.value
        if (v === '+') { addingScreen = true; return }
        addingScreen = false
        screenId = v ? Number(v) : null
      }}>
        <option value="">Not sure</option>
        {#each screens as s (s.id)}<option value={s.id}>{s.name}{s.seats ? ` · ${s.seats} seats` : ''}</option>{/each}
        <option value="+">Add a screen…</option>
      </select>
    </label>
    {#if addingScreen}
      <div class="inline">
        <input bind:value={screenName} placeholder="e.g. Audi 3, IMAX" aria-label="New screen name" maxlength="80"
          onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addScreen() } }} />
        <button type="button" class="btn-secondary sm" disabled={busy || !screenName.trim()} onclick={addScreen}>Add</button>
      </div>
    {/if}
  {:else if adding}
    <fieldset class="new">
      <legend>New cinema</legend>
      <label>Name<input bind:value={draft.name} required maxlength="120" /></label>
      <div class="two">
        <label>City<input bind:value={draft.city} maxlength="80" /></label>
        <label>Country<input bind:value={draft.country} maxlength="2" placeholder="US" class="cc" /></label>
      </div>
      <label>Time zone<input bind:value={draft.timezone} placeholder="Europe/London" maxlength="64" /></label>
      <p class="hint">Cinemas are shared with everyone on this metareel. The time zone files a watch under the right day when you're travelling.</p>
      <div class="inline end">
        <button type="button" class="link" onclick={() => (adding = false)}>Cancel</button>
        <button type="button" class="btn-secondary sm" disabled={busy} onclick={create}>{busy ? 'Adding…' : 'Add cinema'}</button>
      </div>
    </fieldset>
  {:else}
    <input type="search" bind:value={q} placeholder="Search cinemas by name or city" aria-label="Cinema" autocomplete="off" />
    <ul class="results" aria-live="polite">
      {#if q.trim().length < 2}
        {#each recent as v (v.id)}
          <li><button type="button" onclick={() => choose(v)}><b>{v.name}</b><span class="hint">{v.city ?? ''} · last time</span></button></li>
        {/each}
      {:else}
        {#each results as v (v.id)}
          <li><button type="button" onclick={() => choose(v)}><b>{v.name}</b><span class="hint">{place(v)}{v.status === 'closed' ? ` · closed${v.closed_on ? ` ${v.closed_on.slice(0, 4)}` : ''}` : v.status !== 'open' ? ` · ${v.status}` : ''}</span></button></li>
        {/each}
        {#if !searching}
          <li><button type="button" class="add" onclick={startAdding}>Add “{q.trim()}” as a new cinema</button></li>
        {/if}
      {/if}
    </ul>
  {/if}
  {#if error}<p class="err" role="alert">{error}</p>{/if}
</div>

<style>
  .vp { display: grid; gap: 8px; }
  .chosen { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 10px 12px; border-radius: 10px; background: color-mix(in srgb, var(--accent-yours) 12%, var(--bg-canvas)); }
  .chosen b { color: var(--text-primary); font: 700 15px/1.2 var(--font-display); }
  .hint { color: var(--text-muted); font-weight: 400; font-size: 13px; }
  .link { border: 0; background: none; color: var(--text-muted); font: 600 13px/1 var(--font-sans); cursor: pointer; padding: 4px; }
  .link:hover { color: var(--text-primary); }
  .sub, label { display: grid; gap: 6px; font-size: 13px; font-weight: 600; color: var(--text-muted); }
  .inline { display: flex; gap: 8px; align-items: center; }
  .inline input { flex: 1; }
  .end { justify-content: flex-end; }
  .sm { height: 36px; padding: 0 14px; font-size: 13.5px; }
  .results { display: grid; gap: 2px; margin: 0; padding: 0; list-style: none; }
  .results button { display: flex; align-items: baseline; gap: 8px; width: 100%; padding: 8px 10px; border: 0; border-radius: 8px; background: none; color: var(--text-primary); text-align: left; font: 400 14px/1.3 var(--font-sans); cursor: pointer; }
  .results button:hover, .results button:focus-visible { background: var(--bg-elevated); outline: none; }
  .results .add { color: var(--accent-yours); font-weight: 600; }
  .new { display: grid; gap: 10px; margin: 0; padding: 12px; border: 1px solid var(--border-subtle); border-radius: 12px; }
  legend { padding: 0 4px; color: var(--text-primary); font: 700 14px/1 var(--font-display); }
  .two { display: grid; grid-template-columns: 1fr 90px; gap: 10px; }
  .cc { text-transform: uppercase; }
  input, select { min-width: 0; width: 100%; box-sizing: border-box; padding: 9px 12px; border: 1px solid var(--border-subtle); border-radius: 10px; background: var(--bg-canvas); color: var(--text-primary); font: 400 15px/1.3 var(--font-sans); }
  .err { margin: 0; color: var(--delta-down); font-size: 13px; }
</style>
