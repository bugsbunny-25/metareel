<script>
  import { href } from '../lib/router.svelte.js'
  import { getTitle, patchTitle, getTitleCandidates, rematchTitle } from '../lib/api.js'
  import { parseImdbId, parseTmdbInput, parseRtSlug, dateOnly, relativeTime, dateTime } from '../lib/format.js'
  import { toast } from '../lib/toast.svelte.js'
  import { confirm } from '../lib/confirm.svelte.js'
  import Drawer from '../components/Drawer.svelte'
  import Icon from '../components/Icon.svelte'
  import KindBadge from '../components/KindBadge.svelte'
  import MappingHistory from '../components/MappingHistory.svelte'
  import TitleIds from '../components/TitleIds.svelte'
  import TitleLinks from '../components/TitleLinks.svelte'
  import TitleDetailsTab from '../components/TitleDetailsTab.svelte'
  import ChartHistoryTab from '../components/ChartHistoryTab.svelte'
  import NetflixHistoryTab from '../components/NetflixHistoryTab.svelte'
  import RatingsTab from '../components/RatingsTab.svelte'

  let { id, onclose, onsaved } = $props()

  let title = $state(null)
  let loadError = $state('')
  let tab = $state('mapping')
  const TABS = [['mapping', 'Mapping'], ['details', 'Details'], ['top10', 'Top 10 history'], ['netflix', 'Netflix history'], ['ratings', 'Ratings']]
  const MATCH_BADGE = { matched: 'badge-success', manual: 'badge-accent', unmatched: 'badge-warning', pending: '' }
  // The TMDB title (movie | tv) whose details, ratings and IDs every tab shows.
  const tmdbKind = $derived(title?.kind === 'tv_show' ? 'tv' : 'movie')
  const tmdbId = $derived(title?.tmdb_id ?? '')

  // Mapping form
  let form = $state({ kind: 'movie', tmdb_id: '', imdb_id: '', rt_url: '' })
  let saving = $state(false)
  const dirty = $derived(
    title && (form.kind !== title.kind || form.tmdb_id !== (title.tmdb_id ?? '') || form.imdb_id !== (title.imdb_id ?? '') || form.rt_url !== (title.rt_url ?? ''))
  )

  // Candidate search
  let search = $state({ name: '', year: '', kind: '' })
  let candidates = $state(null)
  let searching = $state(false)

  // Matching
  let rematching = $state(false)
  let historyVersion = $state(0)
  async function rematch(clear) {
    if (clear && !(await confirm({ title: 'Clear and match again?', message: `Forget what "${title.name}" is mapped to (and Netflix matches made through it) and match it from scratch. Linked titles that took this mapping fall back to their own.`, confirmLabel: 'Clear and match', danger: true }))) return
    rematching = true
    try {
      const res = await rematchTitle(title.id, clear)
      title = { ...title, ...res.title }
      resetForm()
      historyVersion++
      onsaved?.(res.title)
      toast.success(res.job ? 'Matching queued — the result appears in a few seconds (see Job runs)' : 'Matching is already queued')
      setTimeout(() => getTitle(title.id).then((t) => { if (t.id === id) { title = t; resetForm(); historyVersion++; onsaved?.(t) } }).catch(() => {}), 8000)
    } catch (e) {
      toast.error(e.message)
    } finally {
      rematching = false
    }
  }

  $effect(() => {
    const titleId = id
    title = null
    candidates = null
    loadError = ''
    getTitle(titleId)
      .then((t) => {
        title = t
        resetForm()
        const slugYear = t.slug.match(/-(\d{4})$/)?.[1] ?? ''
        search = { name: t.name, year: slugYear, kind: t.kind }
        if (!t.tmdb_id || !t.rt_url) findCandidates()
      })
      .catch((e) => (loadError = e.message))
  })

  function resetForm() {
    form = { kind: title.kind, tmdb_id: title.tmdb_id ?? '', imdb_id: title.imdb_id ?? '', rt_url: title.rt_url ?? '' }
  }

  // Accept pasted URLs: normalize to IDs when a field loses focus.
  function normalizeTmdb() {
    const { id: tmdbId, kind } = parseTmdbInput(form.tmdb_id)
    form.tmdb_id = tmdbId
    if (kind) form.kind = kind
  }

  async function save() {
    saving = true
    try {
      // Only what changed: IMDb / RT IDs belong to the TMDB title, and one
      // sent is stored as its, set by hand.
      const body = {}
      if (form.kind !== title.kind) body.kind = form.kind
      if (form.tmdb_id.trim() !== (title.tmdb_id ?? '')) body.tmdb_id = form.tmdb_id.trim()
      if (parseImdbId(form.imdb_id) !== (title.imdb_id ?? '')) body.imdb_id = parseImdbId(form.imdb_id)
      if (parseRtSlug(form.rt_url) !== (title.rt_url ?? '')) body.rt_url = parseRtSlug(form.rt_url)
      const updated = await patchTitle(title.id, body)
      historyVersion++
      title = { ...title, ...updated }
      resetForm()
      onsaved?.(updated)
      toast.success(`Saved mapping for "${title.name}"`)
    } catch (e) {
      toast.error(e.message)
    } finally {
      saving = false
    }
  }

  async function findCandidates() {
    searching = true
    try {
      candidates = await getTitleCandidates(id, { name: search.name, year: search.year, kind: search.kind })
    } catch (e) {
      toast.error(e.message)
    } finally {
      searching = false
    }
  }

  function useCandidate(c) {
    if (c.tmdb_id) form.tmdb_id = c.tmdb_id
    if (c.imdb_id) form.imdb_id = c.imdb_id
    if (c.kind) form.kind = c.kind
    toast.info(`Filled in from "${c.title}" — review and save`)
  }

</script>

<Drawer {onclose}>
  {#snippet header()}
    {#if title}
      <div class="row" style="gap:10px">
        <div class="drawer-title">{title.name}</div>
        <KindBadge kind={title.kind} />
        <span class="badge {MATCH_BADGE[title.match_status] ?? ''}">{title.match_status}</span>
      </div>
      <div class="row small muted" style="margin-top:4px;flex-wrap:wrap">
        <span>FlixPatrol title #{title.id}</span>·<span class="mono">{title.slug}</span>·
        <TitleLinks ids={title.ids} {tmdbKind} {tmdbId} flixpatrolSlug={title.slug} />
      </div>
    {:else}
      <div class="drawer-title">{loadError ? 'Title not found' : 'Loading…'}</div>
    {/if}
  {/snippet}

  {#if loadError}
    <div class="alert alert-error"><Icon name="alert" />{loadError}</div>
  {:else if title}
    <div class="tabs" role="tablist">
      {#each TABS as [key, label] (key)}
        <button class="tab" role="tab" aria-selected={tab === key} class:active={tab === key} onclick={() => (tab = key)}>{label}</button>
      {/each}
    </div>

    {#if tab === 'mapping'}
      <div class="section-title">Matching</div>
      <dl class="kv">
        <dt>Status</dt>
        <dd>
          <span class="badge {MATCH_BADGE[title.match_status] ?? ''}">{title.match_status}</span>
          <span class="muted small">
            {#if title.match_status === 'manual'}fixed by hand — automatic matching leaves it alone
            {:else if title.match_status === 'unmatched'}{title.match_attempts} attempt(s){#if title.next_match_at}; next <span title={dateTime(title.next_match_at)}>{relativeTime(title.next_match_at)}</span>{/if}
            {:else if title.match_status === 'pending'}waiting for the matching job
            {:else if title.match_source}via {title.match_source}{/if}
          </span>
        </dd>
        {#if title.matched_name}<dt>Matched to</dt><dd>{title.matched_name} {title.matched_year ? `(${title.matched_year})` : ''}</dd>{/if}
        {#if title.flixpatrol}
          <dt>FlixPatrol page</dt>
          <dd class="small">{title.flixpatrol.name ?? '—'}{title.flixpatrol.kind ? ` · ${title.flixpatrol.kind}` : ''}{title.flixpatrol.premiere_date ? ` · premiered ${dateOnly(title.flixpatrol.premiere_date)}` : ''}{title.flixpatrol.country ? ` · ${title.flixpatrol.country}` : ''}</dd>
        {/if}
        {#if title.netflix_links?.length}
          <dt>Netflix Top 10</dt>
          <dd class="small">
            {#each title.netflix_links as l (l.netflix_title_id)}
              <div><a href={href('/netflix', { open: l.netflix_title_id })}>{l.show_title}</a> <span class="muted">({l.list === 'tv_show' ? 'TV' : 'Films'} list)</span>
                <span class="badge">{l.match_status}</span>
                {#if l.tmdb_id && (l.tmdb_id !== title.tmdb_id || l.tmdb_kind !== (title.kind === 'tv_show' ? 'tv' : 'movie'))}<span class="badge badge-warning" title="Mapped to TMDB {l.tmdb_kind} {l.tmdb_id}">differs</span>{/if}
              </div>
            {/each}
            <div class="field-hint">Linked titles share one mapping: a title fixed by hand wins, then a Netflix ID match, then FlixPatrol's match, then Netflix's.</div>
          </dd>
        {/if}
      </dl>
      <div class="row" style="margin:10px 0 4px">
        <button class="btn btn-sm" onclick={() => rematch(false)} disabled={rematching}><Icon name="refresh" size={12} />Match again</button>
        <button class="btn btn-sm" onclick={() => rematch(true)} disabled={rematching || !title.tmdb_id}><Icon name="x" size={12} />Clear and rematch</button>
        <span class="field-hint">Saving IDs below marks the title as fixed by hand.</span>
      </div>

      <div class="section-title">IDs</div>
      <TitleIds ids={title.ids} {tmdbKind} {tmdbId} />

      <div class="section-title">Fix the mapping</div>
      <div class="field-row">
        <label class="field">
          <span class="field-label">Kind</span>
          <select class="select" bind:value={form.kind}>
            <option value="movie">Movie</option>
            <option value="tv_show">TV show</option>
          </select>
          <span class="field-hint">Which TMDB namespace the ID belongs to.</span>
        </label>
        <label class="field">
          <span class="field-label">TMDB ID</span>
          <input class="input mono" bind:value={form.tmdb_id} onblur={normalizeTmdb} placeholder="e.g. 701387 or TMDB URL" />
        </label>
      </div>
      <div class="field-row">
        <label class="field">
          <span class="field-label">IMDb ID</span>
          <input class="input mono" bind:value={form.imdb_id} onblur={() => (form.imdb_id = parseImdbId(form.imdb_id))} placeholder="tt1234567 or IMDb URL" />
        </label>
        <label class="field">
          <span class="field-label">Rotten Tomatoes</span>
          <input class="input mono" bind:value={form.rt_url} onblur={() => (form.rt_url = parseRtSlug(form.rt_url))} placeholder="m/slug, tv/slug or RT URL" />
        </label>
      </div>
      <p class="field-hint" style="margin-top:-6px">Paste IDs or full URLs; they are normalized automatically. Leave a field empty to clear it. Only changed fields are saved. IMDb and RT IDs belong to the TMDB title: one you enter becomes its, set by hand, and changing the TMDB ID shows the new title's.</p>
      <div class="row" style="margin-top:12px">
        <button class="btn btn-primary" onclick={save} disabled={!dirty || saving}>{saving ? 'Saving…' : 'Save mapping'}</button>
        <button class="btn" onclick={resetForm} disabled={!dirty}>Discard changes</button>
        {#if dirty}<span class="badge badge-warning">Unsaved changes</span>{/if}
      </div>

      <div class="section-title" style="margin-top:24px">History</div>
      <MappingHistory subject={`flixpatrol:${title.id}`} version={historyVersion} />

      <div class="section-title" style="margin-top:24px">Find matches</div>
      <div class="row" style="flex-wrap:wrap">
        <input class="input grow" bind:value={search.name} placeholder="Title" onkeydown={(e) => e.key === 'Enter' && findCandidates()} />
        <input class="input" style="width:80px" bind:value={search.year} placeholder="Year" inputmode="numeric" />
        <select class="select" bind:value={search.kind}>
          <option value="movie">Movie</option>
          <option value="tv_show">TV show</option>
        </select>
        <button class="btn" onclick={findCandidates} disabled={searching || !search.name}>
          {#if searching}<span class="spinner"></span>{:else}<Icon name="search" size={14} />{/if}Search
        </button>
      </div>

      {#if candidates}
        {#if candidates.errors}
          {#each Object.entries(candidates.errors) as [provider, msg] (provider)}
            <div class="alert alert-warning small" style="margin-top:10px"><Icon name="alert" size={14} />{provider}: {msg}</div>
          {/each}
        {/if}

        <div class="section-title">JustWatch <span class="muted">(TMDB + IMDb)</span></div>
        {#each candidates.justwatch as c, i (i)}
          {@render candidateRow(c, form.tmdb_id === c.tmdb_id)}
        {:else}<p class="muted small">No JustWatch results.</p>{/each}

        <div class="section-title">TMDB</div>
        {#each candidates.tmdb as c, i (i)}
          {@render candidateRow(c, form.tmdb_id === c.tmdb_id)}
        {:else}<p class="muted small">No TMDB results (TMDB search needs TMDB_API_KEY).</p>{/each}

        <div class="section-title">Rotten Tomatoes</div>
        {#each candidates.rotten_tomatoes as c (c.slug)}
          <div class="candidate">
            <div class="meta">
              <div class="row"><strong class="truncate">{c.title}</strong><span class="muted">{c.year || ''}</span>
                {#if form.rt_url === c.slug}<span class="badge badge-success">Selected</span>{/if}</div>
              <div class="small muted row">
                <span class="mono">{c.slug}</span>
                {#if c.critics_score !== null}<span>🍅 {c.critics_score}%</span>{/if}
                {#if c.audience_score !== null}<span>🍿 {c.audience_score}%</span>{/if}
                <a href={c.url} target="_blank" rel="noopener"><Icon name="external" size={11} /></a>
              </div>
            </div>
            <button class="btn btn-sm" onclick={() => { form.rt_url = c.slug; toast.info(`RT set to ${c.slug} — review and save`) }}>Use</button>
          </div>
        {:else}<p class="muted small">No Rotten Tomatoes results.</p>{/each}
      {/if}
    {:else if tab === 'details'}
      <TitleDetailsTab {tmdbKind} {tmdbId} />
    {:else if tab === 'top10'}
      <ChartHistoryTab titleId={title.id} />
    {:else if tab === 'netflix'}
      <NetflixHistoryTab {tmdbKind} {tmdbId} />
    {:else}
      <RatingsTab {tmdbKind} {tmdbId} />
    {/if}
  {/if}
</Drawer>

{#snippet candidateRow(c, selected)}
  <div class="candidate">
    <div class="meta">
      <div class="row"><strong class="truncate">{c.title}</strong><span class="muted">{c.year ?? ''}</span>
        {#if selected}<span class="badge badge-success">Selected</span>{/if}</div>
      <div class="small muted row">
        {#if c.tmdb_id}<span class="mono">TMDB {c.tmdb_id}</span>{/if}
        {#if c.imdb_id}<span class="mono">{c.imdb_id}</span>{/if}
        {#if c.url}<a href={c.url} target="_blank" rel="noopener"><Icon name="external" size={11} /></a>{/if}
      </div>
    </div>
    <button class="btn btn-sm" onclick={() => useCandidate(c)} disabled={!c.tmdb_id && !c.imdb_id}>Use</button>
  </div>
{/snippet}
