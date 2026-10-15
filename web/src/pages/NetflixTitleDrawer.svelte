<script>
  import { getNetflixTitle, patchNetflixTitle, rematchNetflixTitle, getNetflixTitleCandidates, patchNetflixSeason } from '../lib/api.js'
  import { href } from '../lib/router.svelte.js'
  import { links, parseImdbId, parseTmdbInput, parseWikidataId, dateOnly, relativeTime, dateTime, number, SEASON_SOURCES, seasonLabel } from '../lib/format.js'
  import { countryName } from '../lib/constants.js'
  import { toast } from '../lib/toast.svelte.js'
  import { confirm } from '../lib/confirm.svelte.js'
  import Drawer from '../components/Drawer.svelte'
  import Icon from '../components/Icon.svelte'
  import KindBadge from '../components/KindBadge.svelte'

  let { id, onclose, onsaved } = $props()

  const MATCH_BADGE = { matched: 'badge-success', manual: 'badge-accent', unmatched: 'badge-warning', pending: '' }

  let title = $state(null)
  let loadError = $state('')
  let tab = $state('mapping')

  // Mapping form. tmdb_kind is the TMDB namespace, which can differ from
  // Netflix's list (a stand-up special in the TV list is a TMDB movie).
  let form = $state({ tmdb_kind: 'movie', tmdb_id: '', imdb_id: '', wikidata_id: '' })
  let saving = $state(false)
  const listTmdbKind = $derived(title?.kind === 'tv_show' ? 'tv' : 'movie')
  const dirty = $derived(title && (form.tmdb_id !== (title.tmdb_id ?? '') || form.imdb_id !== (title.imdb_id ?? '') || form.wikidata_id !== '' ||
    (form.tmdb_id && form.tmdb_kind !== (title.tmdb_kind ?? listTmdbKind))))
  const showSeasons = $derived(title?.kind === 'tv_show' && title?.tmdb_kind !== 'movie')

  // Candidate search (Netflix gives no year; a year narrows Rotten Tomatoes only).
  let search = $state({ name: '', year: '', kind: 'movie' })
  let candidates = $state(null)
  let searching = $state(false)

  // Seasons: pending edits by season id ('' = unchanged).
  let seasonEdits = $state({})
  let savingSeason = $state(null)

  $effect(() => {
    const titleId = id
    title = null
    candidates = null
    seasonEdits = {}
    loadError = ''
    getNetflixTitle(titleId)
      .then((t) => {
        if (titleId !== id) return
        apply(t)
        search = { name: t.show_title, year: '', kind: t.kind }
        tab = t.kind === 'tv_show' && t.tmdb_kind !== 'movie' && t.tmdb_id && t.seasons_unmapped > 0 ? 'seasons' : 'mapping'
        if (!t.tmdb_id) findCandidates()
      })
      .catch((e) => (loadError = e.message))
  })

  function apply(t) {
    title = t
    resetForm()
    onsaved?.(t)
  }

  function resetForm() {
    form = { tmdb_kind: title.tmdb_kind ?? (title.kind === 'tv_show' ? 'tv' : 'movie'), tmdb_id: title.tmdb_id ?? '', imdb_id: title.imdb_id ?? '', wikidata_id: '' }
  }

  function normalizeTmdb() {
    const parsed = parseTmdbInput(form.tmdb_id)
    form.tmdb_id = parsed.id
    if (parsed.kind) form.tmdb_kind = parsed.kind === 'tv_show' ? 'tv' : parsed.kind
  }

  async function save() {
    saving = true
    try {
      const body = { tmdb_kind: form.tmdb_kind, tmdb_id: form.tmdb_id.trim(), imdb_id: parseImdbId(form.imdb_id) }
      if (form.wikidata_id) body.wikidata_id = parseWikidataId(form.wikidata_id)
      apply(await patchNetflixTitle(title.id, body))
      seasonEdits = {}
      toast.success(`Saved mapping for "${title.show_title}"`)
    } catch (e) {
      toast.error(e.message)
    } finally {
      saving = false
    }
  }

  let rematching = $state(false)
  async function rematch(clear) {
    if (clear && !(await confirm({ title: 'Clear and match again?', message: `Forget the TMDB and IMDb IDs of "${title.show_title}" and match it from scratch.`, confirmLabel: 'Clear and match', danger: true }))) return
    rematching = true
    try {
      const res = await rematchNetflixTitle(title.id, clear)
      apply(res.title)
      toast.success(res.job ? 'Matching queued — the result appears shortly (see Job runs)' : 'Matching is already queued')
      setTimeout(() => getNetflixTitle(title.id).then((t) => { if (t.id === id) apply(t) }).catch(() => {}), 8000)
    } catch (e) {
      toast.error(e.message)
    } finally {
      rematching = false
    }
  }

  async function findCandidates() {
    searching = true
    try {
      candidates = await getNetflixTitleCandidates(id, { name: search.name, year: search.year, kind: search.kind })
    } catch (e) {
      toast.error(e.message)
    } finally {
      searching = false
    }
  }

  function useCandidate(c) {
    if (c.tmdb_id) {
      form.tmdb_id = c.tmdb_id
      form.tmdb_kind = c.kind === 'tv_show' ? 'tv' : 'movie'
    }
    if (c.imdb_id) form.imdb_id = c.imdb_id
    tab = 'mapping'
    toast.info(`Filled in from "${c.title}" — review and save`)
  }

  async function saveSeason(s, body) {
    savingSeason = s.id
    try {
      apply(await patchNetflixSeason(s.id, body))
      seasonEdits = { ...seasonEdits, [s.id]: '' }
      toast.success(body.auto ? `"${s.season_title}" is mapped automatically again` : `"${s.season_title}" set to season ${body.season_number}`)
    } catch (e) {
      toast.error(e.message)
    } finally {
      savingSeason = null
    }
  }

  function seasonValue(s) {
    const edit = seasonEdits[s.id]
    return edit !== undefined && edit !== '' ? edit : (s.season_number ?? '')
  }
  function seasonDirty(s) {
    const edit = seasonEdits[s.id]
    return edit !== undefined && edit !== '' && Number(edit) !== s.season_number
  }
  const tmdbSeasonName = (n) => title?.tmdb_seasons.find((x) => x.season_number === n)?.name
</script>

<Drawer {onclose}>
  {#snippet header()}
    {#if title}
      <div class="row" style="gap:10px">
        <div class="drawer-title">{title.show_title}</div>
        <KindBadge kind={title.kind} />
        <span class="badge {MATCH_BADGE[title.match_status] ?? ''}">{title.match_status}</span>
      </div>
      <div class="row small muted" style="margin-top:4px;flex-wrap:wrap">
        <span>Netflix title #{title.id}</span>
        {#if title.last_week}·<span>last charted week ending {dateOnly(title.last_week)}</span>{/if}
        {#if title.tmdb_id}·<a href={links.tmdb(title.tmdb_kind ?? title.kind, title.tmdb_id)} target="_blank" rel="noopener">TMDB <Icon name="external" size={11} /></a>
          <a href={href('/mapping', { open: `${title.tmdb_kind}:${title.tmdb_id}` })}>All IDs</a>{/if}
        {#if title.imdb_id}<a href={links.imdb(title.imdb_id)} target="_blank" rel="noopener">IMDb <Icon name="external" size={11} /></a>{/if}
        {#if title.netflix_id}<a href={links.netflix(title.netflix_id)} target="_blank" rel="noopener" title="Netflix ID {title.netflix_id}">Netflix <Icon name="external" size={11} /></a>{/if}
      </div>
    {:else}
      <div class="drawer-title">{loadError ? 'Netflix title not found' : 'Loading…'}</div>
    {/if}
  {/snippet}

  {#if loadError}
    <div class="alert alert-error"><Icon name="alert" />{loadError}</div>
  {:else if title}
    <div class="tabs">
      <button class="tab" class:active={tab === 'mapping'} onclick={() => (tab = 'mapping')}>Mapping</button>
      {#if showSeasons}
        <button class="tab" class:active={tab === 'seasons'} onclick={() => (tab = 'seasons')}>
          Seasons {#if title.seasons_unmapped}<span class="badge badge-warning">{title.seasons_unmapped}</span>{/if}
        </button>
      {/if}
      <button class="tab" class:active={tab === 'weeks'} onclick={() => (tab = 'weeks')}>Top 10 history</button>
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
        {#if title.tmdb_title}<dt>TMDB title</dt><dd>{title.tmdb_title}{title.tmdb_year ? ` (${title.tmdb_year})` : ''}
          {#if title.tmdb_kind && title.tmdb_kind !== listTmdbKind}<span class="badge badge-info" title="Netflix lists it as {title.kind === 'tv_show' ? 'TV' : 'a film'}">TMDB {title.tmdb_kind === 'tv' ? 'TV show' : 'movie'}</span>{/if}</dd>{/if}
        <dt>FlixPatrol</dt>
        <dd class="small">
          {#each title.linked_titles as l (l.title_id)}
            <div><a href={href('/titles', { q: l.slug, open: l.title_id })}>{l.name}</a> <span class="muted mono">{l.slug}</span>
              <span class="badge {MATCH_BADGE[l.match_status] ?? ''}">{l.match_status}</span>
              {#if l.tmdb_id && (l.tmdb_id !== title.tmdb_id || (l.kind === 'tv_show' ? 'tv' : 'movie') !== title.tmdb_kind)}<span class="badge badge-warning" title="Mapped to TMDB {l.kind === 'tv_show' ? 'tv' : 'movie'} {l.tmdb_id}">differs</span>{/if}
            </div>
          {:else}<span class="muted">No linked FlixPatrol title (none with this name on Netflix's charts in the same weeks)</span>{/each}
          {#if title.linked_titles.length}<div class="field-hint">Linked titles share one mapping: a title fixed by hand wins, then a Netflix ID match, then FlixPatrol's match, then Netflix's.</div>{/if}
        </dd>
        <dt>Netflix ID</dt>
        <dd class="small">{#if title.netflix_id}<span class="mono">{title.netflix_id}</span>{#if title.match_source === 'netflix_id'} <span class="muted">— mapped by it (exact)</span>{/if}{:else}<span class="muted">not found on Tudum's Top 10 search yet</span>{/if}</dd>
        <dt>Global Top 10</dt>
        <dd class="small">{title.weeks_in_global_top10} week(s){title.best_global_rank ? `, best #${title.best_global_rank}` : ''}{title.countries.length ? ` · ${title.countries.length} countr${title.countries.length === 1 ? 'y' : 'ies'}` : ''}</dd>
      </dl>
      <div class="row" style="margin:10px 0 4px;flex-wrap:wrap">
        <button class="btn btn-sm" onclick={() => rematch(false)} disabled={rematching}><Icon name="refresh" size={12} />Match again</button>
        <button class="btn btn-sm" onclick={() => rematch(true)} disabled={rematching || !title.tmdb_id}><Icon name="x" size={12} />Clear and rematch</button>
        <span class="field-hint">Saving IDs below marks the title as fixed by hand.</span>
      </div>

      <div class="section-title">External IDs</div>
      <div class="field-row">
        <div class="field">
          <span class="field-label">TMDB kind</span>
          <div class="segmented" role="group" aria-label="TMDB kind">
            <button class:active={form.tmdb_kind === 'movie'} onclick={() => (form.tmdb_kind = 'movie')}>Movie</button>
            <button class:active={form.tmdb_kind === 'tv'} onclick={() => (form.tmdb_kind = 'tv')}>TV show</button>
          </div>
        </div>
        <label class="field">
          <span class="field-label">TMDB ID</span>
          <input class="input mono" bind:value={form.tmdb_id} onblur={normalizeTmdb} placeholder="e.g. 119051 or TMDB URL" />
        </label>
      </div>
      <div class="field-row">
        <label class="field">
          <span class="field-label">IMDb ID</span>
          <input class="input mono" bind:value={form.imdb_id} onblur={() => (form.imdb_id = parseImdbId(form.imdb_id))} placeholder="tt1234567 or IMDb URL" />
        </label>
        <label class="field">
          <span class="field-label">Wikidata ID</span>
          <input class="input mono" bind:value={form.wikidata_id} onblur={() => (form.wikidata_id = parseWikidataId(form.wikidata_id))} placeholder="Q141461750 or Wikidata URL" />
        </label>
      </div>
      <p class="field-hint" style="margin-top:-6px">Paste IDs or URLs. Netflix lists this under {title.kind === 'tv_show' ? 'TV' : 'Films'}; pick the TMDB kind it really is. Leave the TMDB ID empty to find it from the IMDb or Wikidata ID. Saving also updates linked FlixPatrol titles, fetches TMDB's season list and remaps seasons.</p>
      <div class="row" style="margin-top:12px">
        <button class="btn btn-primary" onclick={save} disabled={!dirty || saving}>{saving ? 'Saving…' : 'Save mapping'}</button>
        <button class="btn" onclick={resetForm} disabled={!dirty}>Discard changes</button>
        {#if dirty}<span class="badge badge-warning">Unsaved changes</span>{/if}
      </div>

      <div class="section-title" style="margin-top:24px">Find matches</div>
      <div class="row" style="flex-wrap:wrap">
        <div class="segmented" role="group" aria-label="Search kind">
          <button class:active={search.kind === 'movie'} onclick={() => (search.kind = 'movie')}>Movies</button>
          <button class:active={search.kind === 'tv_show'} onclick={() => (search.kind = 'tv_show')}>TV</button>
        </div>
        <input class="input grow" bind:value={search.name} placeholder="Title" aria-label="Search title" onkeydown={(e) => e.key === 'Enter' && findCandidates()} />
        <input class="input" style="width:80px" bind:value={search.year} placeholder="Year" aria-label="Year" inputmode="numeric" title="Optional; needed for the Rotten Tomatoes search" />
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
      {/if}
    {:else if tab === 'seasons'}
      <p class="small muted" style="margin-top:0">Netflix charts each season separately. Each season title is mapped to a TMDB season number; FlixPatrol's Netflix rankings of the show use these to tell which season was charting.</p>
      {#if !title.tmdb_id}
        <div class="alert alert-warning small"><Icon name="alert" size={14} />Not mapped to TMDB yet — only seasons named in the title ("Season 2") can be mapped. Map the show first for TMDB season names and air dates.</div>
      {:else if !title.tmdb_seasons.length}
        <div class="alert alert-info small"><Icon name="alert" size={14} />TMDB's season list for this show is not fetched yet (the "TMDB metadata" job does it; saving the mapping fetches it now).</div>
      {/if}
      <div class="card table-wrap">
        <table class="table">
          <thead><tr><th>Season title</th><th>Season</th><th>Source</th><th class="right">Weeks</th><th class="right">Best</th><th>Charted</th><th></th></tr></thead>
          <tbody>
            {#each title.season_titles as s (s.id)}
              <tr>
                <td>
                  <div class="cell-title">{s.season_title}</div>
                  {#if s.season_number !== null && tmdbSeasonName(s.season_number)}<div class="cell-sub">TMDB: {tmdbSeasonName(s.season_number)}</div>{/if}
                  {#if s.netflix_id}<div class="cell-sub mono"><a href={links.netflix(s.netflix_id)} target="_blank" rel="noopener">Netflix {s.netflix_id}</a></div>{/if}
                </td>
                <td class="nowrap">
                  {#if title.tmdb_seasons.length}
                    <select class="select" style="height:28px;max-width:180px" value={String(seasonValue(s))} aria-label="Season number of {s.season_title}"
                      onchange={(e) => (seasonEdits = { ...seasonEdits, [s.id]: e.currentTarget.value })}>
                      {#if s.season_number === null}<option value="">— not mapped —</option>{/if}
                      {#each title.tmdb_seasons as ts (ts.season_number)}
                        <option value={String(ts.season_number)}>{seasonLabel(ts.season_number)} · {ts.name}{ts.air_date ? ` (${ts.air_date.slice(0, 4)})` : ''}</option>
                      {/each}
                      {#if s.season_number !== null && !title.tmdb_seasons.some((ts) => ts.season_number === s.season_number)}
                        <option value={String(s.season_number)}>{seasonLabel(s.season_number)} (not on TMDB)</option>
                      {/if}
                    </select>
                  {:else}
                    <input class="input mono" style="width:70px;height:28px" type="number" min="0" max="999" value={seasonValue(s)} aria-label="Season number of {s.season_title}"
                      oninput={(e) => (seasonEdits = { ...seasonEdits, [s.id]: e.currentTarget.value })} />
                  {/if}
                </td>
                <td class="nowrap">
                  {#if s.season_source}
                    <span class="badge {s.season_source === 'manual' ? 'badge-accent' : ['tmdb_air_date', 'number'].includes(s.season_source) ? 'badge-info' : ''}" title={SEASON_SOURCES[s.season_source]?.hint}>{SEASON_SOURCES[s.season_source]?.label ?? s.season_source}</span>
                  {:else}<span class="badge badge-warning">unmapped</span>{/if}
                </td>
                <td class="right">{s.weeks_in_global_top10 || '—'}</td>
                <td class="right">{s.best_global_rank ? `#${s.best_global_rank}` : '—'}</td>
                <td class="nowrap small">{s.first_week ? dateOnly(s.first_week) : '—'}{s.last_week && s.last_week !== s.first_week ? ` – ${dateOnly(s.last_week)}` : ''}</td>
                <td class="nowrap right">
                  {#if seasonDirty(s)}
                    <button class="btn btn-primary btn-sm" disabled={savingSeason === s.id} onclick={() => saveSeason(s, { season_number: Number(seasonEdits[s.id]) })}>Save</button>
                  {:else if s.season_source === 'manual'}
                    <button class="btn btn-sm" disabled={savingSeason === s.id} onclick={() => saveSeason(s, { auto: true })} title="Hand back to automatic mapping">Auto</button>
                  {/if}
                </td>
              </tr>
            {:else}
              <tr><td colspan="7"><div class="empty small">No season titles — Netflix listed this show without one.</div></td></tr>
            {/each}
          </tbody>
        </table>
      </div>
      <p class="field-hint">"from title", "TMDB name" and "only season" are reliable; "numbered" and "air date" are inferred — check them. A season saved here is "fixed by hand" and never changed automatically; "Auto" hands it back.</p>
    {:else}
      <div class="section-title" style="margin-top:0">Global weekly Top 10</div>
      <div class="card table-wrap">
        <table class="table">
          <thead><tr><th>Week ending</th><th>List</th><th class="right">#</th>{#if showSeasons}<th>Season</th>{/if}<th class="right">Views</th><th class="right">Hours</th><th class="right">Weeks in</th></tr></thead>
          <tbody>
            {#each title.global_weeks as w (w.week + w.category + w.rank)}
              <tr>
                <td class="nowrap">{dateOnly(w.week)}</td>
                <td class="small nowrap">{w.category}</td>
                <td class="right mono">{w.rank}</td>
                {#if showSeasons}<td class="small">{#if w.season_number !== null}<span class="badge">{seasonLabel(w.season_number)}</span> {/if}<span class="muted">{w.season_title ?? ''}</span></td>{/if}
                <td class="right">{number(w.views)}</td>
                <td class="right">{number(w.hours_viewed)}</td>
                <td class="right">{w.cumulative_weeks ?? ''}</td>
              </tr>
            {:else}
              <tr><td colspan="7"><div class="empty small">Not in the global Top 10 (only in country lists or the most-popular list).</div></td></tr>
            {/each}
          </tbody>
        </table>
      </div>
      <div class="section-title">Countries <span class="muted small">(imported countries only)</span></div>
      <div class="card table-wrap">
        <table class="table">
          <thead><tr><th>Country</th><th class="right">Weeks</th><th class="right">Best</th><th>First</th><th>Last</th></tr></thead>
          <tbody>
            {#each title.countries as c (c.country)}
              <tr><td>{countryName(c.country)}</td><td class="right">{c.weeks}</td><td class="right">#{c.best_rank}</td><td class="nowrap">{dateOnly(c.first_week)}</td><td class="nowrap">{dateOnly(c.last_week)}</td></tr>
            {:else}
              <tr><td colspan="5"><div class="empty small">No country weeks imported.</div></td></tr>
            {/each}
          </tbody>
        </table>
      </div>
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
