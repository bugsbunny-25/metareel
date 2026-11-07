<script>
  // Upcoming episodes of shows streamable in a country (GET /calendar), by
  // day, filtered by service on the page.
  import { getCalendar } from '../lib/api.js'
  import { href } from '../lib/router.svelte.js'
  import { dateOnly } from '../lib/format.js'
  import { countryName } from '../lib/constants.js'
  import Icon from './Icon.svelte'

  let { country = 'US' } = $props()

  let days = $state(14)
  let service = $state('') // provider name
  let data = $state(null)
  let error = $state('')
  let loading = $state(false)

  $effect(() => {
    const c = country
    const to = new Date(Date.now() + (days - 1) * 864e5).toISOString().slice(0, 10)
    loading = true
    let cancelled = false
    getCalendar(c, { to })
      .then((d) => { if (!cancelled) { data = d; error = '' } })
      .catch((e) => { if (!cancelled) { data = null; error = e.message } })
      .finally(() => { if (!cancelled) loading = false })
    return () => { cancelled = true }
  })

  const services = $derived([...new Set((data?.items ?? []).flatMap((it) => it.offers.map((o) => o.provider_name)))].sort())
  const byDay = $derived.by(() => {
    const groups = new Map()
    for (const it of data?.items ?? []) {
      if (service && !it.offers.some((o) => o.provider_name === service)) continue
      if (!groups.has(it.air_date)) groups.set(it.air_date, [])
      groups.get(it.air_date).push(it)
    }
    return [...groups]
  })
  const epLabel = (it) => {
    const n = it.episodes.map((e) => e.episode_number)
    return n.length > 1 ? `S${it.season_number} E${n[0]}–E${n[n.length - 1]}` : `S${it.season_number} E${n[0]}`
  }
</script>

<div class="toolbar">
  <div class="segmented" role="group" aria-label="Days ahead">
    {#each [[14, '2 weeks'], [31, '1 month'], [62, '2 months']] as [v, l] (v)}<button class:active={days === v} onclick={() => (days = v)}>{l}</button>{/each}
  </div>
  <select class="select" bind:value={service} aria-label="Service">
    <option value="">Every service</option>
    {#each services as s (s)}<option value={s}>{s}</option>{/each}
  </select>
  <span class="spacer"></span>
  {#if loading}<span class="spinner"></span>{/if}
</div>

{#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

{#if !data && loading}
  <div class="skeleton" style="height:240px"></div>
{:else if data}
  <p class="small muted">Episodes of the shows metareel knows that stream in {countryName(data.country)} (subscription, free or ads), {dateOnly(data.from)} – {dateOnly(data.to)}. Air dates are TMDB's (the original release, not per country).</p>
  <div class="card">
    <div class="table-wrap">
      <table class="table">
        <thead><tr><th>Day</th><th>Show</th><th>Episodes</th><th>Where</th></tr></thead>
        <tbody>
          {#each byDay as [day, items] (day)}
            {#each items as it, i (it.tmdb_id + it.season_number)}
              <tr>
                <td class="nowrap small">{i === 0 ? dateOnly(day) : ''}</td>
                <td>
                  <a class="cell-title" style="color:inherit" href={href('/mapping', { open: `tv:${it.tmdb_id}` })}>{it.title}</a>
                  {#if it.season_premiere}<span class="badge badge-accent">premiere</span>{/if}
                  {#if it.season_finale}<span class="badge badge-warning">finale</span>{/if}
                </td>
                <td class="small">{epLabel(it)}{#if it.episodes.length === 1 && it.episodes[0].name} <span class="muted">· {it.episodes[0].name}</span>{:else if it.episodes.length > 1} <span class="muted">· {it.episodes.length} episodes</span>{/if}</td>
                <td class="small">{[...new Set(it.offers.map((o) => o.provider_name))].slice(0, 3).join(', ')}</td>
              </tr>
            {/each}
          {:else}
            <tr><td colspan="4"><div class="empty">No episodes coming{service ? ` on ${service}` : ''} in this period. Episodes come from the TMDB episodes job.</div></td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
{/if}
