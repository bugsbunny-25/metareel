<!-- Import (#/import, #/import?id=N): bring your history in from TV Time or Suite.
     An export zip is uploaded, matched to TMDB in the background, reviewed here
     (fix or skip what didn't match), then imported. Watches you already have are
     never written twice, so the same history from both apps only fills the gaps.
     A finished import can be undone as a whole. -->
<script>
  import { api } from '../lib/api.js'
  import { route, navigate, setQuery, href } from '../lib/router.svelte.js'
  import { toast } from '../lib/toast.svelte.js'
  import { library } from '../lib/library.svelte.js'
  import { fromUTC, browserZone } from '../lib/dates.js'
  import Poster from '../components/Poster.svelte'

  let { onconfirm } = $props()

  const SOURCES = { tvtime: 'TV Time', suite: 'Suite' }
  const STATUS = {
    matching: 'Matching titles', ready: 'Ready to review', importing: 'Importing',
    done: 'Imported', undone: 'Undone', failed: 'Stopped',
  }
  const busy = (s) => s === 'matching' || s === 'importing'
  const sentence = (m) => (m ? m[0].toUpperCase() + m.slice(1) + (/[.!?]$/.test(m) ? '' : '.') : m)
  const longDate = (ts) => fromUTC(ts)?.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' }) ?? ''
  const plural = (n, one, many = one + 's') => `${(n ?? 0).toLocaleString()} ${n === 1 ? one : many}`

  const id = $derived(route.query.id ? Number(route.query.id) : null)

  // ---- The list of imports, and uploading a new one --------------------------------
  let imports = $state(null)
  let listError = $state('')
  let listNonce = $state(0)
  $effect(() => {
    if (id) return
    listNonce
    const ctrl = new AbortController()
    api.imports(ctrl.signal)
      .then((r) => { imports = r.imports; listError = '' })
      .catch((err) => { if (err.name !== 'AbortError') listError = err.message })
    return () => ctrl.abort()
  })

  let uploading = $state(false)
  let uploadError = $state('')
  let dragging = $state(false)
  async function upload(file) {
    if (!file) return
    uploadError = ''
    if (!/\.zip$/i.test(file.name)) { uploadError = 'Choose the export’s .zip file, as you downloaded it.'; return }
    uploading = true
    try {
      const job = await api.uploadImport(file, browserZone())
      navigate('/import', { id: job.id })
    } catch (err) {
      uploadError = sentence(err.message)
    } finally {
      uploading = false
    }
  }
  function onDrop(e) {
    e.preventDefault()
    dragging = false
    upload(e.dataTransfer?.files?.[0])
  }

  // ---- One import: progress, review and the actions ------------------------------
  let job = $state(null)
  let jobError = $state('')
  let jobNonce = $state(0)
  let lastStatus = null
  $effect(() => {
    if (!id) { job = null; return }
    jobNonce
    const ctrl = new AbortController()
    let timer
    const load = () =>
      api.importJob(id, ctrl.signal)
        .then((j) => {
          if (lastStatus === 'importing' && j.status === 'done') {
            toast(`Imported ${plural(j.results?.watches_added, 'watch', 'watches')}.`)
            library.load().catch(() => {})
          }
          if (lastStatus && lastStatus !== j.status) itemsNonce++
          lastStatus = j.status
          job = j
          jobError = ''
          // Poll while the server is working on it.
          if (busy(j.status)) timer = setTimeout(load, 1500)
        })
        .catch((err) => { if (err.name !== 'AbortError') jobError = err.message })
    load()
    return () => { ctrl.abort(); clearTimeout(timer) }
  })

  const movies = $derived(job?.movies ?? {})
  const shows = $derived(job?.shows ?? {})
  const unmatched = $derived((movies.unmatched ?? 0) + (shows.unmatched ?? 0))
  const matched = $derived((movies.matched ?? 0) + (shows.matched ?? 0))
  const total = $derived((movies.total ?? 0) + (shows.total ?? 0))
  const progress = $derived.by(() => {
    if (!job) return 0
    if (job.status === 'matching') return total ? 1 - ((movies.pending ?? 0) + (shows.pending ?? 0)) / total : 0
    if (job.status === 'importing') return matched ? ((movies.imported ?? 0) + (shows.imported ?? 0)) / matched : 0
    return 1
  })

  // Review list: unmatched first while there are any.
  const FILTERS = [['unmatched', 'Needs a match'], ['matched', 'Matched'], ['skipped', 'Skipped']]
  const filter = $derived(route.query.show ?? (job && job.status === 'ready' && unmatched > 0 ? 'unmatched' : 'matched'))
  const kind = $derived(route.query.kind ?? '')
  const countFor = (f) => (movies[f] ?? 0) + (shows[f] ?? 0)

  let items = $state([])
  let itemsTotal = $state(0)
  let itemsBusy = $state(false)
  let itemsError = $state('')
  let itemsNonce = $state(0)
  $effect(() => {
    if (!id || !job || job.status === 'matching') return
    itemsNonce
    const f = filter, k = kind
    const ctrl = new AbortController()
    itemsBusy = true
    api.importItems(id, { kind: k, match: f, limit: 50 }, ctrl.signal)
      .then((r) => { items = r.items; itemsTotal = r.total; itemsError = '' })
      .catch((err) => { if (err.name !== 'AbortError') itemsError = err.message })
      .finally(() => (itemsBusy = false))
    return () => ctrl.abort()
  })
  async function more() {
    itemsBusy = true
    try {
      const r = await api.importItems(id, { kind, match: filter, limit: 50, offset: items.length })
      items = [...items, ...r.items]
      itemsTotal = r.total
    } catch (err) {
      toast(err.message, 'error')
    } finally {
      itemsBusy = false
    }
  }

  // Fixing one item: pick a candidate, search TMDB, skip or include it again.
  let openSearch = $state(null) // item id with the search open
  let query = $state('')
  let results = $state([])
  let searching = $state(false)
  let saving = $state(null) // item id being saved
  function startSearch(it) {
    openSearch = openSearch === it.id ? null : it.id
    query = it.title
    results = []
    if (openSearch) runSearch(it)
  }
  async function runSearch(it, e) {
    e?.preventDefault()
    if (!query.trim()) return
    searching = true
    try {
      results = (await api.searchTMDB(query.trim(), it.kind)).results ?? []
    } catch (err) {
      toast(err.message, 'error')
    } finally {
      searching = false
    }
  }
  async function setItem(it, body, message) {
    saving = it.id
    try {
      const updated = await api.setImportItem(id, it.id, body)
      // It leaves this filter when its status changes; the counts move with it.
      if (updated.match_status !== filter) items = items.filter((x) => x.id !== it.id)
      else items = items.map((x) => (x.id === it.id ? updated : x))
      openSearch = null
      toast(message ?? 'Saved.')
      jobNonce++
    } catch (err) {
      toast(sentence(err.message), 'error')
    } finally {
      saving = null
    }
  }
  const pick = (it, hit) => setItem(it, { tmdb_id: hit.tmdb_id }, `Matched to ${hit.title}.`)

  async function act(fn, message) {
    try {
      await fn()
      if (message) toast(message)
      jobNonce++
    } catch (err) {
      toast(sentence(err.message), 'error')
    }
  }
  const commit = () => act(() => api.commitImport(id))
  const retry = () => act(() => api.retryImport(id), 'Picking up where it stopped.')
  function undo() {
    onconfirm({
      title: 'Undo this import?',
      body: `This deletes the ${plural(job.results?.watches_added, 'watch', 'watches')} it added, for good, and puts each title’s status and watchlist place back as they were. Watches you logged yourself stay.`,
      confirm: 'Undo import',
      danger: true,
      run: async () => {
        const r = await api.undoImport(id)
        toast(`Removed ${plural(r.watches_removed, 'watch', 'watches')}.`)
        library.load().catch(() => {})
        jobNonce++
      },
    })
  }
  function forget() {
    onconfirm({
      title: 'Remove this import from the list?',
      body: job.status === 'done'
        ? 'The watches it added stay in your diary, but it can’t be undone any more.'
        : 'Nothing was imported from it, so nothing else changes.',
      confirm: 'Remove',
      danger: job.status === 'done',
      run: async () => {
        await api.deleteImport(id)
        navigate('/import')
      },
    })
  }

  function meta(it) {
    const bits = []
    if (it.kind === 'tv') {
      if (it.episodes_total) bits.push(it.match_status === 'matched' ? `${it.episodes_mapped}/${plural(it.episodes_total, 'episode')} found` : plural(it.episodes_total, 'episode'))
    } else if (it.watches) bits.push(it.watches === 1 ? 'Watched' : `Watched ${it.watches}×`)
    if (it.rewatches) bits.push(plural(it.rewatches, 'rewatch', 'rewatches'))
    if (it.watchlist) bits.push('Watchlist')
    if (it.stopped) bits.push('Stopped watching')
    if (it.paused) bits.push('For later')
    if (it.favorite) bits.push('Favourite')
    return bits.join(' · ')
  }
  function resultText(r) {
    const bits = []
    if (r.added) bits.push(`${plural(r.added, 'watch', 'watches')} added`)
    if (r.duplicates) bits.push(`${r.duplicates.toLocaleString()} already logged`)
    if (r.unmapped) bits.push(`${r.unmapped.toLocaleString()} not on TMDB`)
    return bits.join(' · ') || 'Nothing new'
  }
  const yearOf = (d) => (d ? d.slice(0, 4) : '')
</script>

<div class="page">
  {#if !id}
    <header class="head">
      <h1>Import</h1>
      <p class="lede">Bring your watch history in from TV Time or Suite. Titles are matched to TMDB first and you check the matches before anything is added. Anything you’ve already logged is skipped, so importing from both apps, or the same file twice, only fills the gaps.</p>
    </header>

    <section class="panel pad">
      <label
        class="drop"
        class:dragging
        ondragover={(e) => { e.preventDefault(); dragging = true }}
        ondragleave={() => (dragging = false)}
        ondrop={onDrop}
      >
        <input type="file" accept=".zip,application/zip" disabled={uploading} onchange={(e) => upload(e.currentTarget.files?.[0])} />
        <b>{uploading ? 'Reading the export…' : 'Choose an export zip, or drop it here'}</b>
        <span class="hint">TV Time data export or Suite export · up to 64 MB</span>
      </label>
      {#if uploadError}<p class="err" role="alert">{uploadError}</p>{/if}
      <details class="how">
        <summary>Where do I get an export?</summary>
        <dl>
          <dt>TV Time</dt>
          <dd>Ask TV Time for a copy of your data (Settings → Privacy, or their GDPR data request form). They email a zip of CSV files, such as <code>tracking-prod-records-v2.csv</code>. Upload that zip unchanged.</dd>
          <dt>Suite</dt>
          <dd>In Suite, export your library. You get a zip with <code>manifest.json</code>, <code>movies.ndjson</code> and <code>shows.ndjson</code>.</dd>
        </dl>
        <p class="hint">Times are read in your time zone here ({browserZone()}). Episodes TV Time or Suite marked in bulk keep just their day, as they do when you mark a season here.</p>
      </details>
    </section>

    <section class="panel pad" aria-labelledby="i-past">
      <h2 id="i-past">Your imports</h2>
      {#if listError}<p class="alert" role="alert">Couldn’t load your imports: {listError}</p>{/if}
      {#if !imports}
        <div class="skeleton" style="height:90px" aria-busy="true" aria-label="Loading"></div>
      {:else if !imports.length}
        <p class="hint">Nothing imported yet.</p>
      {:else}
        <ul class="jobs">
          {#each imports as j (j.id)}
            <li>
              <a href={href('/import', { id: j.id })}>
                <span class="src">{SOURCES[j.source] ?? j.source}</span>
                <span class="jbody">
                  <b>{j.file_name || 'Export'}</b>
                  <span class="hint">{longDate(j.created_at)} · {plural(j.movies.total, 'movie')} · {plural(j.shows.total, 'show')}{#if j.results && j.status === 'done'}{' · '}{plural(j.results.watches_added, 'watch', 'watches')} added{/if}</span>
                </span>
                <span class="pill st {j.status}">{STATUS[j.status] ?? j.status}</span>
              </a>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  {:else}
    <a class="back" href="#/import">← All imports</a>
    {#if jobError}<p class="alert" role="alert">Couldn’t load the import: {jobError}</p>{/if}
    {#if !job}
      <div class="skeleton" style="height:200px" aria-busy="true" aria-label="Loading"></div>
    {:else}
      <header class="head">
        <h1>From {SOURCES[job.source] ?? job.source}</h1>
        <p class="hint">{job.file_name || 'Export'} · uploaded {longDate(job.created_at)}{#if job.tz}{' · '}times in {job.tz}{/if}</p>
      </header>

      <section class="panel pad status" aria-live="polite">
        <div class="sline">
          <span class="pill st {job.status}">{STATUS[job.status] ?? job.status}</span>
          {#if job.status === 'matching'}
            <span class="hint">Matched {(total - (movies.pending ?? 0) - (shows.pending ?? 0)).toLocaleString()} of {total.toLocaleString()} titles to TMDB…</span>
          {:else if job.status === 'importing'}
            <span class="hint">Added {((movies.imported ?? 0) + (shows.imported ?? 0)).toLocaleString()} of {matched.toLocaleString()} titles…</span>
          {:else if job.status === 'ready'}
            <span class="hint">{unmatched ? `${plural(unmatched, 'title')} couldn’t be matched. Match them below, or import without them.` : 'Every title was matched.'}</span>
          {:else if job.status === 'failed'}
            <span class="hint err">{sentence(job.error)}</span>
          {:else if job.status === 'undone'}
            <span class="hint">Undone {longDate(job.undone_at)}: what it added was removed.</span>
          {/if}
        </div>
        {#if busy(job.status)}
          <div class="bar" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={Math.round(progress * 100)}><span style:width="{progress * 100}%"></span></div>
        {/if}

        <div class="tiles">
          <div class="tile">
            <span class="k">Movies</span>
            <b class="num">{(movies.matched ?? 0).toLocaleString()}<small>/{(movies.total ?? 0).toLocaleString()}</small></b>
            <span class="hint">matched{#if job.summary?.movie_watches}{' · '}{plural(job.summary.movie_watches, 'watch', 'watches')}{/if}</span>
          </div>
          <div class="tile">
            <span class="k">Shows</span>
            <b class="num">{(shows.matched ?? 0).toLocaleString()}<small>/{(shows.total ?? 0).toLocaleString()}</small></b>
            <span class="hint">matched</span>
          </div>
          <div class="tile">
            <span class="k">Episodes</span>
            <b class="num">{(shows.episodes_mapped ?? 0).toLocaleString()}<small>/{(shows.episodes_total ?? 0).toLocaleString()}</small></b>
            <span class="hint">found on TMDB</span>
          </div>
          {#if job.results}
            <div class="tile yours">
              <span class="k">Added</span>
              <b class="num">{job.results.watches_added.toLocaleString()}</b>
              <span class="hint">{job.results.duplicates_skipped.toLocaleString()} already logged{#if job.results.episodes_unmapped}{' · '}{job.results.episodes_unmapped.toLocaleString()} not on TMDB{/if}</span>
            </div>
          {/if}
        </div>

        <div class="actions">
          {#if job.status === 'ready'}
            <button type="button" class="btn-primary sm" disabled={!matched} onclick={commit}>Import {plural(matched, 'title')}</button>
            <span class="hint">{unmatched ? 'Titles without a match are left out. ' : ''}Watches you already have are skipped.</span>
          {:else if job.status === 'failed'}
            <button type="button" class="btn-primary sm" onclick={retry}>Retry</button>
          {:else if job.status === 'done'}
            <a class="btn-secondary sm" href="#/diary">Open your diary</a>
            <button type="button" class="btn-secondary sm" onclick={undo}>Undo import</button>
          {/if}
          {#if !busy(job.status)}<button type="button" class="quiet sm" onclick={forget}>Remove from list</button>{/if}
        </div>
      </section>

      {#if job.status !== 'matching'}
        <section class="panel pad" aria-labelledby="i-items">
          <div class="sh">
            <h2 id="i-items">Titles</h2>
            <div class="seg" role="group" aria-label="Show">
              {#each FILTERS as [f, label]}
                <button type="button" aria-pressed={filter === f} onclick={() => setQuery({ show: f })}>{label} <span class="num c">{countFor(f).toLocaleString()}</span></button>
              {/each}
            </div>
            <div class="seg" role="group" aria-label="Kind">
              {#each [['', 'All'], ['movie', 'Movies'], ['tv', 'Shows']] as [k, label]}
                <button type="button" aria-pressed={kind === k} onclick={() => setQuery({ kind: k })}>{label}</button>
              {/each}
            </div>
          </div>
          {#if itemsError}<p class="alert" role="alert">{itemsError}</p>{/if}
          {#if !items.length && !itemsBusy}
            <p class="hint empty">{filter === 'unmatched' ? 'Nothing left to match.' : filter === 'skipped' ? 'Nothing skipped.' : 'Nothing here.'}</p>
          {/if}
          <ul class="items" aria-busy={itemsBusy}>
            {#each items as it (it.id)}
              <li>
                <div class="row">
                  <Poster path={it.tmdb?.poster_path} title={it.tmdb?.title || it.title} width={44} />
                  <div class="ibody">
                    <div class="names">
                      <b>{it.title || 'Untitled'}{#if it.year}{' '}<span class="yr">({it.year})</span>{/if}</b>
                      {#if it.tmdb}
                        <span class="arrow" aria-hidden="true">→</span>
                        <a href={it.kind === 'tv' ? `#/show/${it.tmdb.tmdb_id}` : `#/title/movie/${it.tmdb.tmdb_id}`}>{it.tmdb.title || `TMDB ${it.tmdb.tmdb_id}`}{#if it.tmdb.release_date}{' '}<span class="yr">({yearOf(it.tmdb.release_date)})</span>{/if}</a>
                        {#if it.match_method === 'search'}<span class="pill tag" title="Matched by name and year">by name</span>{/if}
                        {#if it.match_method === 'manual'}<span class="pill tag">picked</span>{/if}
                        {#if it.match_method === 'history'}<span class="pill tag" title="You already logged this movie at the same moment, from another import">from your history</span>{/if}
                      {:else}
                        <span class="pill tag warn">No match</span>
                      {/if}
                    </div>
                    <span class="hint">{it.kind === 'tv' ? 'Show' : 'Movie'}{#if meta(it)}{' · '}{meta(it)}{/if}</span>
                    {#if it.result}<span class="hint res">{resultText(it.result)}</span>{/if}
                    {#if job.status === 'ready' && it.match_status === 'unmatched' && it.candidates?.length && openSearch !== it.id}
                      <div class="cands" role="group" aria-label="Possible matches">
                        {#each it.candidates as c (c.tmdb_id)}
                          <button type="button" class="cand" disabled={saving === it.id} onclick={() => pick(it, c)}>{c.title}{#if c.year}{' '}<span class="yr">{c.year}</span>{/if}</button>
                        {/each}
                      </div>
                    {/if}
                  </div>
                  {#if job.status === 'ready'}
                    <div class="iacts">
                      {#if it.match_status === 'skipped'}
                        <button type="button" class="btn-secondary sm" disabled={saving === it.id} onclick={() => setItem(it, { skip: false }, 'Back in the import.')}>Include</button>
                      {:else}
                        <button type="button" class="btn-secondary sm" aria-expanded={openSearch === it.id} onclick={() => startSearch(it)}>{it.tmdb ? 'Change' : 'Find'}</button>
                        <button type="button" class="quiet sm" disabled={saving === it.id} onclick={() => setItem(it, { skip: true }, 'Skipped.')}>Skip</button>
                      {/if}
                    </div>
                  {/if}
                </div>
                {#if openSearch === it.id}
                  <form class="search" onsubmit={(e) => runSearch(it, e)}>
                    <input type="search" bind:value={query} aria-label="Search TMDB" placeholder="Search TMDB" />
                    <button type="submit" class="btn-secondary sm" disabled={searching}>{searching ? 'Searching…' : 'Search'}</button>
                  </form>
                  {#if results.length}
                    <ul class="hits">
                      {#each results as hit (hit.tmdb_id)}
                        <li>
                          <button type="button" disabled={saving === it.id} onclick={() => pick(it, hit)}>
                            <Poster path={hit.poster_path} title={hit.title} width={32} />
                            <span><b>{hit.title}</b>{#if hit.year}{' '}<span class="yr">({hit.year})</span>{/if}{#if hit.original_title && hit.original_title !== hit.title}{' '}<span class="hint">· {hit.original_title}</span>{/if}</span>
                          </button>
                        </li>
                      {/each}
                    </ul>
                  {:else if !searching}
                    <p class="hint">Search by the title as TMDB knows it.</p>
                  {/if}
                {/if}
              </li>
            {/each}
          </ul>
          {#if items.length < itemsTotal}
            <button type="button" class="btn-secondary sm more" disabled={itemsBusy} onclick={more}>Show more ({(itemsTotal - items.length).toLocaleString()} left)</button>
          {/if}
        </section>
      {/if}
    {/if}
  {/if}
</div>

<style>
  .page { display: grid; gap: 16px; max-width: 920px; padding: 20px var(--gutter) 48px; }
  h1 { margin: 0; font: 700 32px/1.1 var(--font-display); letter-spacing: -0.02em; }
  .head { display: grid; gap: 6px; }
  .lede { max-width: 64ch; margin: 0; color: var(--text-muted); font-size: 15px; line-height: 1.5; }
  .hint { color: var(--text-muted); font-size: 13px; }
  .sm { height: 34px; padding: 0 14px; font-size: 13.5px; }
  .err { margin: 0; color: var(--delta-down); font-size: 13.5px; }
  .back { color: var(--text-muted); font-size: 14px; text-decoration: none; }
  .back:hover { color: var(--text-primary); }

  .drop { position: relative; display: grid; place-items: center; gap: 6px; padding: 36px 16px; border: 1.5px dashed var(--border-subtle); border-radius: 14px; text-align: center; cursor: pointer; transition: border-color 0.15s, background 0.15s; }
  .drop:hover, .drop.dragging { border-color: var(--accent-yours); background: var(--wash-yours); }
  .drop input { position: absolute; inset: 0; opacity: 0; cursor: pointer; }
  .drop b { font-size: 16px; }
  .how summary { cursor: pointer; color: var(--text-muted); font-size: 14px; }
  .how dl { display: grid; grid-template-columns: max-content 1fr; gap: 8px 16px; margin: 12px 0 8px; font-size: 14px; line-height: 1.5; }
  .how dt { font-weight: 700; }
  .how dd { margin: 0; color: var(--text-muted); }
  code { font-size: 12.5px; padding: 1px 5px; border-radius: 5px; background: var(--bg-elevated); }

  .jobs { display: grid; gap: 2px; margin: 0; padding: 0; list-style: none; }
  .jobs a { display: grid; grid-template-columns: 76px minmax(0, 1fr) auto; align-items: center; gap: 12px; padding: 10px 8px; border-radius: 10px; color: inherit; text-decoration: none; }
  .jobs a:hover { background: var(--bg-elevated); }
  .src { font: 700 13px/1 var(--font-sans); color: var(--accent-yours); }
  .jbody { display: grid; gap: 2px; min-width: 0; }
  .jbody b, .jbody .hint { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .st.ready { background: var(--wash-yours); color: var(--accent-yours); }
  .st.done { background: color-mix(in srgb, var(--delta-up) 16%, transparent); color: var(--delta-up); }
  .st.failed { background: color-mix(in srgb, var(--delta-down) 16%, transparent); color: var(--delta-down); }

  .status { gap: 14px; }
  .sline { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; }
  .bar { height: 6px; border-radius: 3px; background: var(--bg-elevated); overflow: hidden; }
  .bar span { display: block; height: 100%; border-radius: 3px; background: var(--accent-yours); transition: width 0.4s; }
  .tiles { display: grid; grid-template-columns: repeat(auto-fit, minmax(135px, 1fr)); gap: 10px; }
  .tile { display: grid; gap: 2px; padding: 12px 14px; border-radius: 12px; background: var(--bg-elevated); }
  .tile .k { color: var(--text-muted); font-size: 12px; font-weight: 700; letter-spacing: 0.04em; text-transform: uppercase; }
  .tile b { font: 700 26px/1.1 var(--font-display); }
  .tile small { color: var(--text-muted); font-size: 15px; font-weight: 600; }
  .tile.yours b { color: var(--accent-yours); }
  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; }
  .actions .quiet { margin-left: auto; }
  .quiet { border: 0; border-radius: 17px; background: none; color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .quiet:hover:not(:disabled) { background: var(--bg-elevated); color: var(--text-primary); }

  .sh { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 12px; }
  .sh h2 { margin-right: auto; }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { height: 30px; padding: 0 12px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13px/1 var(--font-sans); cursor: pointer; white-space: nowrap; }
  .seg button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }
  .seg .c { margin-left: 4px; opacity: 0.7; }
  .empty { padding: 18px 0; text-align: center; }

  .items { display: grid; gap: 2px; margin: 0; padding: 0; list-style: none; }
  .items[aria-busy='true'] { opacity: 0.6; }
  .items > li { padding: 10px 6px; border-top: 1px solid var(--border-subtle); }
  .items > li:first-child { border-top: 0; }
  .row { display: grid; grid-template-columns: 44px minmax(0, 1fr) auto; align-items: center; gap: 12px; }
  .ibody { display: grid; gap: 3px; min-width: 0; }
  .names { display: flex; flex-wrap: wrap; align-items: baseline; gap: 2px 8px; }
  .names b { font-size: 15px; }
  .names a { color: var(--text-primary); font-size: 15px; text-decoration: none; }
  .names a:hover { text-decoration: underline; }
  .yr { color: var(--text-muted); font-weight: 400; }
  .arrow { color: var(--text-muted); }
  .tag { padding: 2px 7px; font-size: 11.5px; }
  .tag.warn { background: color-mix(in srgb, var(--delta-down) 16%, transparent); color: var(--delta-down); }
  .res { color: var(--accent-yours); }
  .iacts { display: flex; gap: 6px; }
  .cands { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 4px; }
  .cand { padding: 5px 10px; border: 1px solid var(--border-subtle); border-radius: 14px; background: none; color: var(--text-primary); font: 600 12.5px/1.2 var(--font-sans); cursor: pointer; }
  .cand:hover:not(:disabled) { border-color: var(--accent-yours); background: var(--wash-yours); }
  .search { display: flex; gap: 8px; margin: 10px 0 6px 56px; }
  .search input { flex: 1; min-width: 0; padding: 8px 12px; border: 1px solid var(--border-subtle); border-radius: 10px; background: var(--bg-canvas); color: var(--text-primary); font: 400 14.5px/1.3 var(--font-sans); }
  .hits { display: grid; gap: 2px; margin: 0 0 0 56px; padding: 0; list-style: none; }
  .hits button { display: grid; grid-template-columns: 32px 1fr; align-items: center; gap: 10px; width: 100%; padding: 6px 8px; border: 0; border-radius: 9px; background: none; color: var(--text-primary); font: 400 14px/1.3 var(--font-sans); text-align: left; cursor: pointer; }
  .hits button:hover:not(:disabled) { background: var(--bg-elevated); }
  .more { justify-self: center; margin-top: 8px; }

  @media (max-width: 640px) {
    .row { grid-template-columns: 44px minmax(0, 1fr); }
    .iacts { grid-column: 2; }
    .search, .hits { margin-left: 0; }
    .jobs a { grid-template-columns: minmax(0, 1fr) auto; }
    .src { display: none; }
    .actions .quiet { margin-left: 0; }
  }
</style>
