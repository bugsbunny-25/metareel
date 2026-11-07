<script>
  // People ranked by the Top 10 points of their titles (GET /analytics/people).
  import { getAnalytics } from '../lib/api.js'
  import { route, setQuery, href } from '../lib/router.svelte.js'
  import { number, dateOnly } from '../lib/format.js'
  import { COUNTRIES, PROVIDERS } from '../lib/constants.js'
  import Icon from './Icon.svelte'
  import InfoTip from './InfoTip.svelte'
  import PersonDrawer from './PersonDrawer.svelte'

  const ROLES = [['actor', 'Actors'], ['director', 'Directors'], ['writer', 'Writers'], ['creator', 'Creators'], ['composer', 'Composers'], ['cinematographer', 'Cinematographers']]
  const role = $derived(route.query.role ?? 'actor')
  const range = $derived(route.query.range ?? '90')
  const country = $derived(route.query.country ?? '')
  const provider = $derived(route.query.provider ?? '')
  const kind = $derived(route.query.kind ?? '')
  const person = $derived(route.query.person ?? '')

  let data = $state(null)
  let error = $state('')
  let loading = $state(false)

  $effect(() => {
    const to = new Date()
    const from = new Date(Date.now() - (Number(range) - 1) * 864e5)
    const q = { role, country, provider, kind, from: from.toISOString().slice(0, 10), to: to.toISOString().slice(0, 10), limit: 100 }
    loading = true
    let cancelled = false
    getAnalytics('people', q)
      .then((d) => { if (!cancelled) { data = d; error = '' } })
      .catch((e) => { if (!cancelled) { data = null; error = e.message } })
      .finally(() => { if (!cancelled) loading = false })
    return () => { cancelled = true }
  })

  const set = (patch) => setQuery(patch)
  const maxPoints = $derived(data?.people?.[0]?.points ?? 1)
</script>

<div class="toolbar">
  <select class="select" value={role} onchange={(e) => set({ role: e.currentTarget.value })} aria-label="Role">
    {#each ROLES as [v, l] (v)}<option value={v}>{l}</option>{/each}
  </select>
  <div class="segmented" role="group" aria-label="Range">
    {#each [['30', '30 days'], ['90', '90 days'], ['365', '1 year']] as [v, l] (v)}<button class:active={range === v} onclick={() => set({ range: v === '90' ? '' : v })}>{l}</button>{/each}
  </div>
  <select class="select" value={country} onchange={(e) => set({ country: e.currentTarget.value })} aria-label="Country">
    <option value="">All countries</option>
    {#each COUNTRIES as c (c.code)}<option value={c.code}>{c.name}</option>{/each}
  </select>
  <select class="select" value={provider} onchange={(e) => set({ provider: e.currentTarget.value })} aria-label="Provider">
    <option value="">All providers</option>
    {#each PROVIDERS as p (p.value)}<option value={p.value}>{p.label}</option>{/each}
  </select>
  <div class="segmented" role="group" aria-label="Kind">
    {#each [['', 'All'], ['movie', 'Movies'], ['tv_show', 'TV']] as [v, l] (v)}<button class:active={kind === v} onclick={() => set({ kind: v })}>{l}</button>{/each}
  </div>
  <span class="spacer"></span>
  {#if loading}<span class="spinner"></span>{/if}
</div>

{#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

{#if !data && loading}
  <div class="skeleton" style="height:240px"></div>
{:else if data}
  <p class="small muted">{dateOnly(data.from)} – {dateOnly(data.to)} · {number(data.total)} people <InfoTip text="Each title's chart points go in full to everyone credited in the role (actors: billed in its top 10), once per person. Credits from TMDB (TMDB credits job)." /></p>
  <div class="card">
    <div class="table-wrap">
      <table class="table">
        <thead><tr><th class="right">#</th><th>Person</th><th class="right">Points</th><th class="right">Titles</th><th class="right">Best</th><th>Top titles</th></tr></thead>
        <tbody>
          {#each data.people as p, i (p.person_id)}
            <tr class="clickable" onclick={() => set({ person: p.person_id })}>
              <td class="right mono">{i + 1}</td>
              <td><a class="cell-title" style="color:inherit" href={href('/insights', { ...route.query, person: p.person_id })} onclick={(e) => e.stopPropagation()}>{p.name}</a></td>
              <td class="right"><div class="cell-bar"><span class="hbar-track"><span class="hbar-fill" style="width:{(p.points / maxPoints) * 100}%"></span></span><span class="num" style="min-width:44px">{number(p.points)}</span></div></td>
              <td class="right">{p.titles}</td>
              <td class="right">#{p.best_rank}</td>
              <td class="small">{p.top_titles.map((t) => t.title).join(', ')}</td>
            </tr>
          {:else}
            <tr><td colspan="6"><div class="empty">No credits for charting titles yet. The TMDB credits job (Data health) fetches them.</div></td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
{/if}

{#if person}
  <PersonDrawer personId={person} onclose={() => set({ person: '' })} />
{/if}
