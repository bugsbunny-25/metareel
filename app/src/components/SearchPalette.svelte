<!-- Search (⌘K / Ctrl K, or "/"). Two result lists, kept apart because they answer different
     questions: "In the charts" (/api/v1/search: titles metareel has chart data for, with when
     they last charted) and "All of TMDB" (/me/titles/search: anything, for logging an obscure
     film). A pasted IMDb ID or an IMDb, TMDB or Netflix link resolves directly. Empty, it shows
     titles recently touched in your library.
     Keys: ↑↓ move, Tab next list, ↵ open, ⌘/Ctrl ↵ log a watch, ⇧↵ watchlist, Esc close. -->
<script>
  import Poster from './Poster.svelte'
  import Icon from './Icon.svelte'
  import { api, ApiError } from '../lib/api.js'
  import { library } from '../lib/library.svelte.js'
  import { navigate } from '../lib/router.svelte.js'
  import { dayLabel } from '../lib/dates.js'
  import { toast } from '../lib/toast.svelte.js'

  let { open = $bindable(false), onlog } = $props()

  let dialog
  let input = $state(null)
  let q = $state('')
  let kind = $state('')
  let active = $state(0)
  let charted = $state({ status: 'idle', items: [], error: '' })
  let tmdb = $state({ status: 'idle', items: [], error: '' })
  let byId = $state({ status: 'idle', items: [], error: '', via: '' })

  const mac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform)
  const MOD = mac ? '⌘' : 'Ctrl'

  $effect(() => {
    if (!dialog) return
    if (open && !dialog.open) {
      dialog.showModal()
      active = 0
      queueMicrotask(() => input?.select())
    } else if (!open && dialog.open) {
      dialog.close()
    }
  })

  /** An ID or link the lookup endpoint can resolve, or null. */
  function parseId(text) {
    const t = text.trim()
    let m = t.match(/themoviedb\.org\/(movie|tv)\/(\d+)/i)
    if (m) return { direct: { kind: m[1].toLowerCase(), id: m[2] }, via: 'TMDB link' }
    m = t.match(/\b(tt\d{6,10})\b/i)
    if (m) return { params: { imdb: m[1].toLowerCase() }, via: 'IMDb ID' }
    m = t.match(/netflix\.com\/(?:[a-z]{2}(?:-[a-z]{2})?\/)?title\/(\d+)/i)
    if (m) return { params: { netflix: m[1] }, via: 'Netflix link' }
    return null
  }

  const year = (d) => (d ? Number(String(d).slice(0, 4)) || null : null)

  // Search as you type: debounced, and an older request never overwrites a newer one.
  $effect(() => {
    const text = q.trim()
    const k = kind
    if (!open || text.length < 2) {
      charted = { status: 'idle', items: [], error: '' }
      tmdb = { status: 'idle', items: [], error: '' }
      byId = { status: 'idle', items: [], error: '', via: '' }
      return
    }
    const ctrl = new AbortController()
    const timer = setTimeout(() => run(text, k, ctrl.signal), 220)
    return () => { clearTimeout(timer); ctrl.abort() }
  })

  async function run(text, k, signal) {
    active = 0
    const id = parseId(text)
    if (id) {
      charted = { status: 'idle', items: [], error: '' }
      tmdb = { status: 'idle', items: [], error: '' }
      byId = { status: 'loading', items: [], error: '', via: id.via }
      try {
        const refs = id.direct ? [id.direct] : ((await api.lookup(id.params, signal)).items ?? []).map((x) => ({ kind: x.kind, id: x.tmdb_id }))
        // The lookup returns IDs only; /me/titles fills in the name and poster.
        const items = await Promise.all(refs.slice(0, 3).map(async (r) => {
          const t = (await api.myTitle(r.kind, r.id, signal).catch(() => null))?.title
          return { kind: r.kind, id: r.id, title: t?.title ?? `${r.kind === 'tv' ? 'TV show' : 'Film'} ${r.id}`, year: year(t?.release_date), poster: t?.poster_path ?? null, sub: `Found by ${id.via}` }
        }))
        if (!signal.aborted) byId = { status: 'ok', items, error: '', via: id.via }
      } catch (err) {
        if (err.name === 'AbortError') return
        byId = { status: 'error', items: [], error: err instanceof ApiError && err.status === 404 ? `metareel doesn’t know a title with that ${id.via}.` : err.message, via: id.via }
      }
      return
    }
    byId = { status: 'idle', items: [], error: '', via: '' }
    charted = { ...charted, status: 'loading', error: '' }
    tmdb = { ...tmdb, status: 'loading', error: '' }
    api.searchCharted(text, k || undefined, signal)
      .then((r) => { charted = { status: 'ok', error: '', items: (r.items ?? []).map((x) => ({
        kind: x.kind, id: x.tmdb_id, title: x.title, year: x.year || null, poster: x.poster_path || null,
        sub: [x.matched && x.matched !== x.title ? `Also known as ${x.matched}` : '', x.last_charted ? `Last charted ${dayLabel(x.last_charted)}` : '', x.sources?.includes('netflix') ? 'Netflix Top 10' : ''].filter(Boolean).join(' · '),
      })) } })
      .catch((err) => { if (err.name !== 'AbortError') charted = { status: 'error', items: [], error: err.message } })
    api.searchTMDB(text, k || undefined, signal)
      .then((r) => { tmdb = { status: 'ok', error: '', items: (r.results ?? []).map((x) => ({
        kind: x.tmdb_kind, id: x.tmdb_id, title: x.title, year: x.year || year(x.release_date), poster: x.poster_path || null,
        sub: x.original_title && x.original_title !== x.title ? x.original_title : '',
      })) } })
      .catch((err) => {
        if (err.name === 'AbortError') return
        tmdb = { status: 'error', items: [], error: err instanceof ApiError && err.status === 502 ? 'TMDB search isn’t available on this server right now.' : err.message }
      })
  }

  const key = (it) => `${it.kind}:${it.id}`
  const recent = $derived(
    [...library.byKey.values()].sort((a, b) => (b.updated_at ?? '').localeCompare(a.updated_at ?? '')).slice(0, 6)
      .map((t) => ({ kind: t.tmdb_kind, id: t.tmdb_id, title: t.title, year: year(t.release_date), poster: t.poster_path, sub: t.status === 'watchlist' ? 'On your watchlist' : t.watch_count ? `Watched ${t.watch_count}×` : '' }))
  )
  const lanes = $derived.by(() => {
    if (q.trim().length < 2) return recent.length ? [{ key: 'recent', label: 'Recently in your library', state: { status: 'ok', items: recent } }] : []
    if (byId.status !== 'idle') return [{ key: 'id', label: `By ${byId.via}`, state: byId }]
    const seen = new Set(charted.items.map(key))
    return [
      { key: 'charted', label: 'In the charts', state: charted },
      { key: 'tmdb', label: 'All of TMDB', state: { ...tmdb, items: tmdb.items.filter((x) => !seen.has(key(x))) } },
    ]
  })
  const flat = $derived(lanes.flatMap((l) => l.state.items.map((it) => ({ ...it, lane: l.key }))))
  const laneStart = $derived(lanes.map((l, i) => lanes.slice(0, i).reduce((n, x) => n + x.state.items.length, 0)))
  const busy = $derived(lanes.some((l) => l.state.status === 'loading'))
  $effect(() => { if (active >= flat.length) active = Math.max(0, flat.length - 1) })

  function close() { open = false }
  function go(it) {
    close()
    navigate(`/title/${it.kind}/${it.id}`)
  }
  function log(it) {
    close()
    onlog({ kind: it.kind, id: it.id, name: it.title, poster: it.poster })
  }
  async function watchlist(it) {
    try {
      const on = await library.toggleWatchlist(it.kind, it.id)
      toast(on ? `Added ${it.title} to your watchlist.` : `Removed ${it.title} from your watchlist.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }

  function onkeydown(e) {
    const n = flat.length
    if (e.key === 'Escape') { e.preventDefault(); close() } // <dialog> also cancels on Escape; this keeps it explicit
    else if (e.key === 'ArrowDown' && n) { active = (active + 1) % n; e.preventDefault() }
    else if (e.key === 'ArrowUp' && n) { active = (active - 1 + n) % n; e.preventDefault() }
    else if (e.key === 'Tab' && lanes.length > 1 && n) {
      // Jump to the first result of the next (or previous) list that has any.
      const cur = lanes.findIndex((l, i) => active >= laneStart[i] && active < laneStart[i] + l.state.items.length)
      for (let step = 1; step < lanes.length + 1; step++) {
        const j = (cur + (e.shiftKey ? -step : step) + lanes.length * 2) % lanes.length
        if (lanes[j].state.items.length) { active = laneStart[j]; break }
      }
      e.preventDefault()
    } else if (e.key === 'Enter' && flat[active]) {
      e.preventDefault()
      if (e.metaKey || e.ctrlKey) log(flat[active])
      else if (e.shiftKey) watchlist(flat[active])
      else go(flat[active])
    }
  }

  const mark = (it) => {
    const s = library.get(it.kind, it.id)
    return s?.status === 'watchlist' ? 'watchlist' : s?.watch_count > 0 ? 'watched' : ''
  }
</script>

<dialog bind:this={dialog} onclose={close} aria-label="Search" class="palette">
  {#if open}
    <div class="box">
      <div class="field">
        <Icon name="search" size={18} />
        <input
          bind:this={input}
          bind:value={q}
          {onkeydown}
          type="search"
          role="combobox"
          aria-expanded={flat.length > 0}
          aria-controls="search-results"
          aria-autocomplete="list"
          aria-activedescendant={flat[active] ? `sr-${active}` : undefined}
          aria-label="Search titles"
          placeholder="Search titles, or paste an IMDb, TMDB or Netflix link"
          autocomplete="off"
          spellcheck="false"
        />
        <div class="kinds" role="group" aria-label="Kind">
          <button type="button" aria-pressed={kind === ''} onclick={() => (kind = '')}>All</button>
          <button type="button" aria-pressed={kind === 'movie'} onclick={() => (kind = 'movie')}>Films</button>
          <button type="button" aria-pressed={kind === 'tv'} onclick={() => (kind = 'tv')}>Shows</button>
        </div>
      </div>

      <div class="results" id="search-results">
        {#each lanes as lane, li (lane.key)}
          <section class="lane" aria-labelledby="lane-{lane.key}">
            <h2 id="lane-{lane.key}">{lane.label}{#if lane.state.status === 'ok' && lane.key !== 'recent'}<span class="n num">{lane.state.items.length}</span>{/if}</h2>
            {#if lane.state.status === 'loading' && !lane.state.items.length}
              <p class="msg">Searching…</p>
            {:else if lane.state.status === 'error'}
              <p class="msg bad">{lane.state.error}</p>
            {:else if lane.state.status === 'ok' && !lane.state.items.length}
              <p class="msg">{lane.key === 'charted' ? 'Nothing by that name has charted.' : 'No matches.'}</p>
            {:else}
              <ul role="listbox" aria-labelledby="lane-{lane.key}">
                {#each lane.state.items as it, i (key(it))}
                  {@const idx = laneStart[li] + i}
                  {@const m = mark(it)}
                  <!-- Pointer users click; keyboard users stay in the input (aria-activedescendant). -->
                  <!-- svelte-ignore a11y_click_events_have_key_events -->
                  <li id="sr-{idx}" role="option" aria-selected={idx === active} class:active={idx === active}
                    onmouseenter={() => (active = idx)} onclick={() => go(it)}>
                    <Poster path={it.poster} seed={it.id} title={it.title ?? ''} width={34} radius={4} size="w92" />
                    <span class="text">
                      <span class="name">{it.title}{#if it.year}{' '}<span class="yr num">{it.year}</span>{/if}</span>
                      <span class="sub">{it.kind === 'tv' ? 'Series' : 'Film'}{#if it.sub}{' · '}{it.sub}{/if}</span>
                    </span>
                    {#if m}<span class="mine" title={m === 'watchlist' ? 'On your watchlist' : 'Watched'}><Icon name={m === 'watchlist' ? 'bookmark-fill' : 'check'} size={13} /></span>{/if}
                    <span class="acts">
                      <button type="button" tabindex="-1" aria-label="Log a watch of {it.title}" title="Log a watch ({MOD} ↵)" onclick={(e) => { e.stopPropagation(); log(it) }}><Icon name="plus" size={14} /></button>
                      <button type="button" tabindex="-1" aria-label="{m === 'watchlist' ? 'Remove' : 'Add'} {it.title} {m === 'watchlist' ? 'from' : 'to'} watchlist" title="Watchlist (⇧ ↵)" onclick={(e) => { e.stopPropagation(); watchlist(it) }}><Icon name={m === 'watchlist' ? 'bookmark-fill' : 'bookmark'} size={14} /></button>
                    </span>
                  </li>
                {/each}
              </ul>
            {/if}
          </section>
        {/each}
        {#if !lanes.length}
          <p class="msg pad">Type a title. Charted titles come first, then everything else on TMDB.</p>
        {/if}
      </div>

      <footer>
        <span><kbd>↑</kbd><kbd>↓</kbd> move</span><span><kbd>Tab</kbd> next list</span><span><kbd>↵</kbd> open</span>
        <span><kbd>{MOD}</kbd><kbd>↵</kbd> log</span><span><kbd>⇧</kbd><kbd>↵</kbd> watchlist</span><span><kbd>Esc</kbd> close</span>
      </footer>
      <p class="sr" aria-live="polite">{busy ? 'Searching' : q.trim().length >= 2 ? `${flat.length} results` : ''}</p>
    </div>
  {/if}
</dialog>

<style>
  .palette { width: min(720px, calc(100vw - 24px)); max-height: min(640px, calc(100vh - 80px)); margin: 10vh auto auto; padding: 0; border: 0; border-radius: 20px; background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 40px 100px -20px rgb(0 0 0 / 0.65); }
  .palette::backdrop { background: rgb(0 0 0 / 0.55); }
  .box { display: grid; grid-template-rows: auto 1fr auto; max-height: min(640px, calc(100vh - 80px)); }
  .field { display: flex; align-items: center; gap: 10px; padding: 14px 16px; box-shadow: inset 0 -1px 0 var(--border-subtle); color: var(--text-muted); }
  input { flex: 1; min-width: 0; border: 0; background: none; color: var(--text-primary); font: 400 19px/1.3 var(--font-display); outline: none; }
  input::placeholder { color: var(--text-muted); font-size: 16px; }
  input::-webkit-search-cancel-button { display: none; }
  .field:focus-within { box-shadow: inset 0 -2px 0 var(--accent-yours); }
  .kinds { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .kinds button { height: 26px; padding: 0 10px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 12.5px/1 var(--font-sans); cursor: pointer; }
  .kinds button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); }

  .results { overflow-y: auto; padding: 6px 8px 8px; }
  .lane + .lane { margin-top: 6px; }
  h2 { display: flex; align-items: center; gap: 8px; margin: 10px 10px 4px; color: var(--text-muted); font: 700 12px/1 var(--font-sans); letter-spacing: 0.06em; text-transform: uppercase; }
  .n { padding: 2px 6px; border-radius: 8px; background: var(--bg-elevated); font-size: 11px; letter-spacing: 0; }
  ul { margin: 0; padding: 0; list-style: none; }
  li { display: flex; align-items: center; gap: 12px; padding: 7px 10px; border-radius: 12px; cursor: pointer; }
  li.active { background: var(--bg-elevated); }
  .text { display: grid; flex: 1; min-width: 0; }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 600 16px/1.25 var(--font-display); }
  .yr { color: var(--text-muted); font: 400 14px/1 var(--font-sans); }
  .sub { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-muted); font-size: 12.5px; }
  .mine { display: grid; place-items: center; width: 22px; height: 22px; border-radius: 50%; background: var(--accent-yours); color: var(--bg-canvas); flex: none; }
  .acts { display: flex; gap: 4px; opacity: 0; }
  li.active .acts { opacity: 1; }
  .acts button { display: grid; place-items: center; width: 30px; height: 30px; border: 0; border-radius: 50%; background: var(--bg-surface); color: var(--text-primary); cursor: pointer; }
  .acts button:hover { background: var(--accent-primary); color: var(--accent-on); }
  .msg { margin: 6px 10px 10px; color: var(--text-muted); font: italic 400 14.5px/1.4 var(--font-display); }
  .msg.pad { padding: 18px 0; text-align: center; }
  .msg.bad { color: var(--delta-down); font-style: normal; font-family: var(--font-sans); }
  footer { display: flex; flex-wrap: wrap; gap: 6px 16px; padding: 10px 16px; box-shadow: inset 0 1px 0 var(--border-subtle); color: var(--text-muted); font-size: 12px; }
  kbd { display: inline-grid; place-items: center; min-width: 20px; height: 20px; margin-right: 3px; padding: 0 5px; border-radius: 5px; background: var(--bg-elevated); color: var(--text-primary); font: 600 11px/1 var(--font-sans); }
  @media (hover: none) { .acts { opacity: 1; } }
  @media (max-width: 560px) {
    .palette { margin-top: 12px; }
    .field { flex-wrap: wrap; }
    .kinds { order: 3; }
    footer { display: none; }
  }
</style>
