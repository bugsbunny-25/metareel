<script>
  // Automatic mappings scored by how likely they are wrong, riskiest first.
  import { reviewMappings, getReviewSuggestions, patchTitle, patchNetflixTitle } from '../lib/api.js'
  import { toast } from '../lib/toast.svelte.js'
  import { providerLabel } from '../lib/constants.js'
  import { route, setQuery, href, intParam } from '../lib/router.svelte.js'
  import { dateOnly, links, number } from '../lib/format.js'
  import { define } from '../lib/glossary.js'
  import Icon from '../components/Icon.svelte'
  import InfoTip from '../components/InfoTip.svelte'
  import Pagination from '../components/Pagination.svelte'
  import KindBadge from '../components/KindBadge.svelte'

  const REASONS = {
    not_on_provider: 'Not on the service it charted on',
    released_later: 'Released after it charted',
    imdb_mismatch: 'IMDb ID disagrees',
    year_mismatch: 'Year differs from FlixPatrol',
    weak_source: 'Weak matching rule',
    name_differs: 'Name differs from TMDB',
    no_offers: 'No offers on TMDB',
    no_details: 'TMDB details pending',
  }
  const THRESHOLDS = [[25, 'Risk 25+'], [50, 'Risk 50+'], [0, 'Everything']]

  const subject = $derived(route.query.subject ?? '')
  const reason = $derived(route.query.reason ?? '')
  // 0 ("Everything") is a valid threshold, so not intParam (positive only).
  const minRisk = $derived.by(() => {
    const n = Number.parseInt(route.query.min_risk, 10)
    return Number.isFinite(n) && n >= 0 && n <= 100 ? n : 25
  })
  const page = $derived(intParam(route.query.page, 1))
  const size = $derived(intParam(route.query.size, 50))

  let data = $state({ items: [], total: 0, checked: 0, reasons: {} })
  let loading = $state(false)
  let error = $state('')
  let reload = $state(0)

  $effect(() => {
    const params = { subject, reason, min_risk: minRisk, limit: size, offset: (page - 1) * size }
    void reload
    loading = true
    let cancelled = false
    reviewMappings(params)
      .then((d) => { if (!cancelled) { data = d; error = '' } })
      .catch((e) => { if (!cancelled) error = e.message })
      .finally(() => { if (!cancelled) loading = false })
    return () => { cancelled = true }
  })

  const set = (patch) => setQuery({ ...patch, page: '' })
  const riskClass = (r) => (r >= 50 ? 'badge-danger' : r >= 25 ? 'badge-warning' : 'badge')
  // Opens the title's own drawer on its source tab.
  const titleHref = (it) => href(it.subject === 'netflix' ? '/netflix' : '/titles', { open: it.id })

  // One-click fixes: keep the mapping ("Looks right") or take a suggested
  // TMDB title. Both are manual fixes, so the title leaves the queue.
  const rowKey = (it) => `${it.subject}:${it.id}`
  const services = (it) => it.providers.map(providerLabel).join(' / ')
  let expanded = $state('') // row whose suggestions are shown
  let suggestions = $state({}) // rowKey -> { loading, data, error }
  let busy = $state('') // rowKey being saved

  async function toggleSuggestions(it) {
    const k = rowKey(it)
    if (expanded === k) {
      expanded = ''
      return
    }
    expanded = k
    if (suggestions[k]?.data) return
    suggestions[k] = { loading: true }
    try {
      suggestions[k] = { data: await getReviewSuggestions(it.subject, it.id) }
    } catch (e) {
      suggestions[k] = { error: e.message }
    }
  }

  async function setMapping(it, tmdbKind, tmdbId, message) {
    busy = rowKey(it)
    try {
      if (it.subject === 'netflix') await patchNetflixTitle(it.id, { tmdb_kind: tmdbKind, tmdb_id: tmdbId })
      else await patchTitle(it.id, { kind: tmdbKind === 'tv' ? 'tv_show' : 'movie', tmdb_id: tmdbId })
      toast.success(message)
      expanded = ''
      delete suggestions[rowKey(it)]
      reload++
    } catch (e) {
      toast.error(e.message)
    } finally {
      busy = ''
    }
  }

  const confirmMapping = (it) =>
    setMapping(it, it.tmdb_kind, it.tmdb_id, `Kept ${it.name} → ${it.tmdb_title || `TMDB ${it.tmdb_id}`}; it now counts as fixed by hand`)
  const useSuggestion = (it, s) =>
    setMapping(it, s.tmdb_kind, s.tmdb_id, `Mapped ${it.name} to ${s.title}${s.year ? ` (${s.year})` : ''}`)
</script>

<div>
  <div class="subhead">
    <p class="tab-hint">
      Every automatic mapping scored by how likely it is wrong <InfoTip text={define('Mapping risk')} />, riskiest first.
      Keep a right mapping with <b>Looks right</b>, or <b>Find fix</b> for other TMDB titles it could be (one click maps it); both count as fixed by hand, and those leave the queue.
    </p>
  </div>

  <div class="toolbar">
    <div class="segmented" role="group" aria-label="Source">
      <button class:active={subject === ''} onclick={() => set({ subject: '' })}>All</button>
      <button class:active={subject === 'flixpatrol'} onclick={() => set({ subject: 'flixpatrol' })}>FlixPatrol</button>
      <button class:active={subject === 'netflix'} onclick={() => set({ subject: 'netflix' })}>Netflix Top 10</button>
    </div>
    <div class="segmented" role="group" aria-label="Minimum risk">
      {#each THRESHOLDS as [value, label] (value)}
        <button class:active={minRisk === value} onclick={() => set({ min_risk: value === 25 ? '' : value })}>{label}</button>
      {/each}
    </div>
    <select class="select" value={reason} onchange={(e) => set({ reason: e.currentTarget.value })} aria-label="Reason">
      <option value="">Any reason</option>
      {#each Object.entries(REASONS) as [value, label] (value)}
        <option {value}>{label}{data.reasons?.[value] ? ` (${number(data.reasons[value])})` : ''}</option>
      {/each}
    </select>
    <span class="spacer"></span>
    {#if loading}<span class="spinner"></span>{/if}
    <span class="muted small">{number(data.total)} of {number(data.checked)} automatic mappings</span>
  </div>

  {#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

  <div class="card">
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>Risk</th>
            <th>Chart title</th>
            <th>Mapped to</th>
            <th>Why</th>
            <th>Matched by</th>
            <th>First charted</th>
            <th class="right">Chart weight</th>
            <th aria-label="Actions"></th>
          </tr>
        </thead>
        <tbody>
          {#each data.items as it (it.subject + it.id)}
            <tr>
              <td><span class="badge {riskClass(it.risk)} num">{it.risk}</span></td>
              <td>
                <div class="cell-title"><a href={titleHref(it)}>{it.name}</a></div>
                <div class="cell-sub">{it.subject === 'netflix' ? 'Netflix Top 10' : 'FlixPatrol'} · {it.providers.join(', ')}</div>
              </td>
              <td>
                <div class="cell-title">{it.tmdb_title || `TMDB ${it.tmdb_id}`}{it.tmdb_year ? ` (${it.tmdb_year})` : ''}</div>
                <div class="cell-sub"><KindBadge kind={it.tmdb_kind === 'tv' ? 'tv_show' : 'movie'} /> <a class="mono" href={links.tmdb(it.tmdb_kind, it.tmdb_id)} target="_blank" rel="noopener">TMDB {it.tmdb_id}</a></div>
              </td>
              <td class="small">
                {#each it.reasons as r (r.code)}<div title="+{r.points}">{r.text}</div>{:else}<span class="muted">—</span>{/each}
              </td>
              <td class="small mono">{it.match_source || '—'}</td>
              <td class="small nowrap">{it.first_charted ? dateOnly(it.first_charted) : '—'}</td>
              <td class="right" title={it.subject === 'netflix' ? 'Weeks in the global Top 10' : 'Chart points'}>{number(it.points)}</td>
              <td class="nowrap">
                <button class="btn btn-sm" onclick={() => confirmMapping(it)} disabled={busy === rowKey(it)} title="The mapping is right: keep it as a manual fix (it leaves the queue)"><Icon name="check" size={13} />Looks right</button>
                <button class="btn btn-sm" class:btn-primary={expanded === rowKey(it)} onclick={() => toggleSuggestions(it)} aria-expanded={expanded === rowKey(it)} title="Search JustWatch and TMDB for other titles it could be"><Icon name="search" size={13} />Find fix</button>
              </td>
            </tr>
            {#if expanded === rowKey(it)}
              {@const sg = suggestions[rowKey(it)] ?? {}}
              <tr class="subrow">
                <td colspan="8">
                  {#if sg.loading}
                    <div class="muted small"><span class="spinner"></span> Searching JustWatch and TMDB…</div>
                  {:else if sg.error}
                    <div class="alert alert-error"><Icon name="alert" />{sg.error}</div>
                  {:else if sg.data}
                    {#if sg.data.errors}<div class="muted small">Some searches failed: {Object.keys(sg.data.errors).join(', ')}.</div>{/if}
                    {#each sg.data.suggestions as s (s.tmdb_kind + s.tmdb_id)}
                      <div class="suggestion">
                        <span class="badge num" title="Suggestion score">{s.score}</span>
                        <div class="suggestion-main">
                          <div class="cell-title">{s.title}{s.year ? ` (${s.year})` : ''}</div>
                          <div class="cell-sub">
                            <KindBadge kind={s.tmdb_kind === 'tv' ? 'tv_show' : 'movie'} />
                            <a class="mono" href={links.tmdb(s.tmdb_kind, s.tmdb_id)} target="_blank" rel="noopener">TMDB {s.tmdb_id}</a>
                            {#if s.on_provider === true}<span class="badge badge-success">on {services(it)}</span>{:else if s.on_provider === false}<span class="badge">no {services(it)} offer</span>{/if}
                            {#if s.reasons.length}<span class="muted"> · {s.reasons.join(', ')}</span>{/if}
                          </div>
                        </div>
                        <button class="btn btn-sm btn-primary" onclick={() => useSuggestion(it, s)} disabled={busy === rowKey(it)}>Use this</button>
                      </div>
                    {:else}
                      <div class="muted small">No other TMDB title found under this name. <a href={titleHref(it)}>Open the title</a> to search by hand.</div>
                    {/each}
                  {/if}
                </td>
              </tr>
            {/if}
          {:else}
            <tr><td colspan="8">
              <div class="empty">
                {#if loading}Loading…{:else}<div class="empty-title">Nothing to review</div>No automatic mapping reaches this risk. Lower the threshold to see more.{/if}
              </div>
            </td></tr>
          {/each}
        </tbody>
      </table>
    </div>
    <Pagination {page} {size} total={data.total} onchange={(p, s) => setQuery({ page: p === 1 ? '' : p, size: s === 50 ? '' : s })} />
  </div>
  <p class="field-hint">Scores add up (capped at 100): a weak matching rule (up to 35), not on any offer of the service it charted on (35), a TMDB release after it first charted (40), an IMDb ID that disagrees (30), FlixPatrol's premiere year more than a year off (25), no offers at all (15), a different name (15). Offers come from TMDB watch providers (data by JustWatch) in any country.</p>
</div>

<style>
  .subrow td { background: var(--surface-2); }
  .suggestion { display: flex; align-items: center; gap: 10px; padding: 6px 0; }
  .suggestion + .suggestion { border-top: 1px solid var(--border); }
  .suggestion-main { flex: 1; min-width: 0; }
</style>
