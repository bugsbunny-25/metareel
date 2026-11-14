<!-- New & upcoming on the service in the top bar (#/discover?tab=new&when=new|soon&kind=&days=):
     what it added per day (/titles/new, live from JustWatch, up to 30 days) and what it has
     announced (/titles/upcoming, soonest first, undated last). One request per view; kind and
     "hide what I've seen" filter in the browser. -->
<script>
  import TitleTile from '../../components/TitleTile.svelte'
  import BarList from '../../components/BarList.svelte'
  import { api } from '../../lib/api.js'
  import { library } from '../../lib/library.svelte.js'
  import { route, setQuery } from '../../lib/router.svelte.js'
  import { inCountry, providerName } from '../../lib/scope.svelte.js'
  import { localDay, addDays, dayLabel, weekdayLabel } from '../../lib/dates.js'
  import { votes } from '../../lib/services.js'

  let { country, provider } = $props()

  const when = $derived(route.query.when === 'soon' ? 'soon' : 'new')
  const kind = $derived(['movie', 'tv'].includes(route.query.kind) ? route.query.kind : '')
  const DAYS = [7, 14, 30]
  const days = $derived(DAYS.includes(+route.query.days) ? +route.query.days : 7)
  const showSeen = $derived(route.query.seen === '1')

  let data = $state(null)
  let error = $state('')
  $effect(() => {
    const cc = country, svc = provider, w = when, n = days
    const ctrl = new AbortController()
    data = null
    error = ''
    const today = localDay()
    const req = w === 'new'
      ? api.newOnService(cc, svc, { from: addDays(today, -(n - 1)), to: today }, ctrl.signal)
      : api.upcomingOnService(cc, svc, {}, ctrl.signal)
    req.then((d) => (data = d)).catch((err) => { if (err.name !== 'AbortError') error = err.message })
    return () => ctrl.abort()
  })

  const seen = (it) => {
    const t = it.tmdb_id ? library.get(it.kind, it.tmdb_id) : null
    return !!t && (t.watch_count > 0 || t.status === 'completed')
  }
  const all = $derived(data?.items ?? [])
  const list = $derived(all.filter((it) => (!kind || it.kind === kind) && (showSeen || !seen(it))))
  const seenCount = $derived(all.filter(seen).length)

  // Grouped by day; announced titles without a date come last.
  const groups = $derived.by(() => {
    const m = new Map()
    for (const it of list) {
      const k = it.date ?? ''
      if (!m.has(k)) m.set(k, [])
      m.get(k).push(it)
    }
    return [...m]
  })
  let shownDays = $state(6)
  $effect(() => { when; kind; days; showSeen; provider; country; shownDays = 6 })

  const today = localDay()
  const dayHead = (d) => {
    if (!d) return 'Date not announced'
    if (d === today) return 'Today'
    if (d === addDays(today, -1)) return 'Yesterday'
    if (d === addDays(today, 1)) return 'Tomorrow'
    return `${weekdayLabel(d)} ${dayLabel(d)}`
  }
  const films = $derived(list.filter((it) => it.kind === 'movie').length)
  const onList = $derived(list.filter((it) => it.tmdb_id && library.get(it.kind, it.tmdb_id)?.status === 'watchlist').length)
  const best = $derived(list.filter((it) => it.imdb_rating && (it.imdb_votes ?? 0) >= 1000).sort((a, b) => b.imdb_rating - a.imdb_rating)[0] ?? null)
  const week = $derived(list.filter((it) => it.date && it.date <= addDays(today, 7)).length)
  const dated = $derived(list.filter((it) => it.date).length)
  const genres = $derived.by(() => {
    const m = new Map()
    for (const it of list) for (const g of it.genres ?? []) m.set(g, (m.get(g) ?? 0) + 1)
    return [...m].sort((a, b) => b[1] - a[1]).slice(0, 8).map(([label, value]) => ({ label, value }))
  })
  const perDay = $derived(when === 'new' && groups.length ? Math.round((list.length / days) * 10) / 10 : 0)

  const label = (it) => it.kind === 'tv' && it.season_title && it.season_title !== it.title ? `${it.title}: ${it.season_title}` : it.title
  const meta = (it) => [it.kind === 'tv' ? 'Series' : 'Film', it.year, (it.genres ?? []).slice(0, 2).join(', ')].filter(Boolean).join(' · ')
  const badge = (it) => (it.imdb_rating ? `★ ${it.imdb_rating.toFixed(1)}` : it.tomatometer != null ? `${it.tomatometer}%` : '')
  const pick = (patch) => setQuery(patch, { replace: true })
  const svc = $derived(data?.service?.name ?? providerName(provider))
</script>

<div class="d-filters">
  <div class="d-seg" role="group" aria-label="New or upcoming">
    <button aria-pressed={when === 'new'} onclick={() => pick({ when: '' })}>Just added</button>
    <button aria-pressed={when === 'soon'} onclick={() => pick({ when: 'soon', days: '' })}>Coming soon</button>
  </div>
  {#if when === 'new'}
    <div class="d-seg" role="group" aria-label="Days">
      {#each DAYS as n}<button aria-pressed={days === n} onclick={() => pick({ days: n === 7 ? '' : n })}>{n} days</button>{/each}
    </div>
  {/if}
  <div class="d-seg" role="group" aria-label="Kind">
    {#each [['', 'All'], ['movie', 'Films'], ['tv', 'Shows']] as [v, l]}<button aria-pressed={kind === v} onclick={() => pick({ kind: v })}>{l}</button>{/each}
  </div>
  <label class="d-check"><input type="checkbox" checked={showSeen} onchange={(e) => pick({ seen: e.currentTarget.checked ? '1' : '' })} /> Show what I’ve seen{#if seenCount}{' '}<span class="hint num">{seenCount}</span>{/if}</label>
</div>

{#if error}
  <p class="alert" role="alert">Couldn’t load {when === 'new' ? 'what’s new' : 'what’s coming'} on {providerName(provider)}: {error}</p>
{:else if !data}
  <div class="skeleton" style="height:520px" aria-busy="true" aria-label="Loading"></div>
{:else}
  <section class="d-kpis" aria-label="{when === 'new' ? 'New' : 'Coming'} on {svc} at a glance">
    {#if when === 'new'}
      <div class="d-kpi"><span class="k-label">Added in {days} days</span><b class="num">{list.length}</b><span class="k-sub">{perDay} a day on {svc} in {inCountry(country)}</span></div>
      <div class="d-kpi"><span class="k-label">Films · Shows</span><b class="num">{films} · {list.length - films}</b><span class="k-sub">seasons count under their show</span></div>
    {:else}
      <div class="d-kpi"><span class="k-label">Announced</span><b class="num">{list.length}</b><span class="k-sub">{dated} with a date on {svc}</span></div>
      <div class="d-kpi"><span class="k-label">In the next 7 days</span><b class="num">{week}</b><span class="k-sub">{films} films, {list.length - films} shows in all</span></div>
    {/if}
    <div class="d-kpi"><span class="k-label">Best rated</span><b class="small" title={best ? label(best) : ''}>{best ? label(best) : '–'}</b><span class="k-sub">{best ? `IMDb ${best.imdb_rating.toFixed(1)} from ${votes(best.imdb_votes)} votes` : 'no IMDb rating yet'}</span></div>
    <div class="d-kpi yours"><span class="k-label">On your watchlist</span><b class="num">{onList}</b><span class="k-sub">{onList ? 'of these titles' : 'bookmark one below'}</span></div>
  </section>

  {#if data.truncated}<p class="d-note">JustWatch returned its maximum; the oldest of these may be missing.</p>{/if}

  {#if !list.length}
    <section class="d-empty">
      <h2>{all.length ? 'Nothing left after the filters' : when === 'new' ? `Nothing new on ${svc} lately` : `Nothing announced on ${svc}`}</h2>
      <p>{all.length ? 'Show what you’ve seen, or pick All.' : `JustWatch lists nothing for ${svc} in ${inCountry(country)} here. Try another service in the top bar.`}</p>
    </section>
  {:else}
    <div class="d-layout">
      <div class="d-main">
        {#each groups.slice(0, shownDays) as [d, items] (d)}
          <section class="d-day" aria-label={dayHead(d)}>
            <h3>{dayHead(d)} <span class="hint num">{items.length} {items.length === 1 ? 'title' : 'titles'}</span></h3>
            <ul class="d-grid">
              {#each items as it (`${it.justwatch_id}:${it.season_number ?? ''}:${it.date ?? ''}`)}
                <TitleTile kind={it.kind} tmdbId={it.tmdb_id} title={label(it)} poster={it.poster_url} meta={meta(it)} badge={badge(it)} badgeTone="gold" />
              {/each}
            </ul>
          </section>
        {/each}
        {#if groups.length > shownDays}
          <button class="btn-secondary d-more" onclick={() => (shownDays += 6)}>Show more days <span class="hint">{groups.length - shownDays} left</span></button>
        {/if}
      </div>
      <aside class="d-side">
        <section class="panel pad">
          <h2>Genres</h2>
          <p class="hint">{when === 'new' ? 'Of what was added' : 'Of what’s announced'}, with the filters above.</p>
          {#if genres.length}<BarList items={genres} />{:else}<p class="hint">No genres listed.</p>{/if}
        </section>
        <p class="d-source">Live from JustWatch for {svc} in {inCountry(country)}. Dates are the release on the service.</p>
      </aside>
    </div>
  {/if}
{/if}
