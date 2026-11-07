<script>
  // A movie's release dates per country (TMDB) and the windows between
  // theatrical, digital and streaming.
  import { getTitleReleases } from '../lib/api.js'
  import { dateOnly } from '../lib/format.js'
  import { countryName, providerLabel } from '../lib/constants.js'
  import Icon from './Icon.svelte'

  let { tmdbKind = '', tmdbId = '' } = $props()

  let data = $state(null)
  let error = $state('')
  let all = $state(false)
  const MAIN = ['US', 'GB', 'IN', 'CA', 'AU', 'DE', 'FR']

  $effect(() => {
    const kind = tmdbKind
    const id = tmdbId
    data = null
    error = ''
    if (!id || kind !== 'movie') return
    let cancelled = false
    getTitleReleases(kind, id)
      .then((d) => { if (!cancelled) data = d })
      .catch((e) => { if (!cancelled) error = e.message })
    return () => { cancelled = true }
  })

  // Countries with a theatrical / digital date or a streaming arrival;
  // the main ones first unless every country is asked for.
  const rows = $derived.by(() => {
    const list = (data?.countries ?? []).filter((c) => c.theatrical || c.theatrical_limited || c.digital || c.streaming || c.tv)
    const shown = all ? list : list.filter((c) => MAIN.includes(c.country))
    return shown.sort((a, b) => (MAIN.indexOf(a.country) + 1 || 99) - (MAIN.indexOf(b.country) + 1 || 99) || a.country.localeCompare(b.country))
  })
  const days = (n) => (n == null ? '—' : `${n} d`)
</script>

{#if !tmdbId}
  <div class="empty"><div class="empty-title">Not mapped to TMDB</div>Release dates come from TMDB. Map this title first.</div>
{:else if tmdbKind !== 'movie'}
  <div class="empty small">TMDB has release dates for movies only.</div>
{:else if error}
  <div class="alert alert-error"><Icon name="alert" />{error}</div>
{:else if !data}
  <div class="skeleton" style="height:160px"></div>
{:else if !rows.length && !all}
  <div class="empty small">No release dates for the main countries. <button class="btn btn-ghost btn-sm" onclick={() => (all = true)}>Show every country</button></div>
{:else}
  <div class="table-wrap">
    <table class="table">
      <thead><tr><th>Country</th><th>Theatrical</th><th>Digital</th><th>Streaming</th><th class="right">To digital</th><th class="right">To streaming</th></tr></thead>
      <tbody>
        {#each rows as c (c.country)}
          <tr>
            <td class="small">{countryName(c.country)}</td>
            <td class="small nowrap">{c.theatrical ? dateOnly(c.theatrical) : c.theatrical_limited ? `${dateOnly(c.theatrical_limited)} (limited)` : '—'}</td>
            <td class="small nowrap">{c.digital ? dateOnly(c.digital) : '—'}</td>
            <td class="small">
              {#if c.streaming}<span title={c.streaming.basis === 'first_chart' ? 'First chart day (it was streaming by then)' : 'First seen as a streaming offer'}>{dateOnly(c.streaming.date)} · {c.streaming.basis === 'first_chart' ? providerLabel(c.streaming.provider) : c.streaming.provider}</span>
              {:else if c.streaming_now.length}<span class="muted">now on {c.streaming_now[0].provider_name}{c.streaming_now.length > 1 ? ` +${c.streaming_now.length - 1}` : ''}</span>
              {:else}—{/if}
            </td>
            <td class="right">{days(c.windows.theatrical_to_digital)}</td>
            <td class="right">{days(c.windows.theatrical_to_streaming)}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  {#if !all}<button class="btn btn-ghost btn-sm" onclick={() => (all = true)}>Show every country</button>{/if}
  <p class="field-hint">Release dates from TMDB. Streaming is observed (the first offer seen arriving, or the first chart day), so it can be a little late.</p>
{/if}
