<script>
  // Automatic mappings scored by how likely they are wrong, riskiest first.
  import { reviewMappings } from '../lib/api.js'
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

  $effect(() => {
    const params = { subject, reason, min_risk: minRisk, limit: size, offset: (page - 1) * size }
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
</script>

<div>
  <div class="subhead">
    <p class="tab-hint">
      Every automatic mapping scored by how likely it is wrong <InfoTip text={define('Mapping risk')} />, riskiest first.
      Open a title to check it and fix it by hand; titles fixed by hand leave the queue.
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
            </tr>
          {:else}
            <tr><td colspan="7">
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
