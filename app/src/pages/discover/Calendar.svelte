<!-- Episode calendar (#/discover?tab=calendar&days=&only=yours|library): episodes airing from
     today of the shows streamable in the country (/calendar/{country}, at most 62 days), one row
     per show, season and day, so a binge drop is one row. Filters run in the browser. -->
<script>
  import Poster from '../../components/Poster.svelte'
  import Icon from '../../components/Icon.svelte'
  import BarList from '../../components/BarList.svelte'
  import { api } from '../../lib/api.js'
  import { library } from '../../lib/library.svelte.js'
  import { prefs } from '../../lib/prefs.svelte.js'
  import { route, setQuery } from '../../lib/router.svelte.js'
  import { inCountry } from '../../lib/scope.svelte.js'
  import { servicesOf } from '../../lib/services.js'
  import { localDay, addDays, dayLabel, weekdayLabel } from '../../lib/dates.js'
  import { toast } from '../../lib/toast.svelte.js'

  let { country, families } = $props()

  const DAYS = [7, 14, 30, 60]
  const days = $derived(DAYS.includes(+route.query.days) ? +route.query.days : 14)
  const only = $derived(['yours', 'library'].includes(route.query.only) ? route.query.only : '')

  let data = $state(null)
  let error = $state('')
  $effect(() => {
    const cc = country, n = days
    const ctrl = new AbortController()
    data = null
    error = ''
    const from = localDay()
    api.calendar(cc, { from, to: addDays(from, n - 1) }, ctrl.signal)
      .then((d) => (data = d))
      .catch((err) => { if (err.name !== 'AbortError') error = err.message })
    return () => ctrl.abort()
  })

  const yours = $derived(new Set((prefs.values.services ?? []).map((id) => families.get(id)).filter(Boolean)))
  const tracked = (it) => {
    const t = library.get('tv', it.tmdb_id)
    return !!t && t.status !== 'dropped'
  }
  const rows = $derived((data?.items ?? []).map((it) => ({ ...it, services: servicesOf(it.offers ?? [], families) })))
  const list = $derived(rows.filter((it) => (only !== 'yours' || it.services.some((s) => yours.has(s))) && (only !== 'library' || tracked(it))))
  const groups = $derived.by(() => {
    const m = new Map()
    for (const it of list) {
      if (!m.has(it.air_date)) m.set(it.air_date, [])
      m.get(it.air_date).push(it)
    }
    return [...m]
  })

  const episodes = $derived(list.reduce((n, it) => n + (it.episodes?.length ?? 0), 0))
  const shows = $derived(new Set(list.map((it) => it.tmdb_id)).size)
  const premieres = $derived(list.filter((it) => it.season_premiere).length)
  const finales = $derived(list.filter((it) => it.season_finale).length)
  const mine = $derived(rows.filter(tracked).length)
  const byService = $derived.by(() => {
    const m = new Map()
    for (const it of list) for (const s of it.services) m.set(s, (m.get(s) ?? 0) + 1)
    return [...m].sort((a, b) => b[1] - a[1]).slice(0, 8).map(([label, value]) => ({ label, value }))
  })

  const today = localDay()
  const dayHead = (d) => (d === today ? 'Today' : d === addDays(today, 1) ? 'Tomorrow' : `${weekdayLabel(d)} ${dayLabel(d)}`)
  const eps = (it) => {
    const e = (it.episodes ?? []).map((x) => x.episode_number).sort((a, b) => a - b)
    if (!e.length) return `Season ${it.season_number}`
    const range = e.length === 1 ? `E${e[0]}` : `E${e[0]}–E${e.at(-1)}`
    return `S${it.season_number} · ${range}`
  }
  const epName = (it) => (it.episodes?.length === 1 && it.episodes[0].name && !/^Episode \d+$/.test(it.episodes[0].name) ? it.episodes[0].name : '')
  const status = (it) => {
    const t = library.get('tv', it.tmdb_id)
    return t?.status === 'watchlist' ? 'On your watchlist' : t?.status === 'watching' ? 'Watching' : t?.watch_count > 0 ? 'You’ve watched some' : ''
  }

  async function toggle(it) {
    try {
      const on = await library.toggleWatchlist('tv', it.tmdb_id)
      toast(on ? `Added ${it.title} to your watchlist.` : `Removed ${it.title} from your watchlist.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }
  const pick = (patch) => setQuery(patch, { replace: true })
</script>

<div class="d-filters">
  <div class="d-seg" role="group" aria-label="Days ahead">
    {#each DAYS as n}<button aria-pressed={days === n} onclick={() => pick({ days: n === 14 ? '' : n })}>{n === 7 ? 'This week' : `${n} days`}</button>{/each}
  </div>
  <div class="d-seg" role="group" aria-label="Which shows">
    <button aria-pressed={!only} onclick={() => pick({ only: '' })}>Everything</button>
    <button aria-pressed={only === 'yours'} disabled={!yours.size} title={yours.size ? '' : 'Pick your services under Which services to pay for'} onclick={() => pick({ only: 'yours' })}>On your services</button>
    <button aria-pressed={only === 'library'} onclick={() => pick({ only: 'library' })}>Shows you track</button>
  </div>
</div>

{#if error}
  <p class="alert" role="alert">Couldn’t load the calendar: {error}</p>
{:else if !data}
  <div class="skeleton" style="height:520px" aria-busy="true" aria-label="Loading"></div>
{:else}
  <section class="d-kpis" aria-label="The calendar at a glance">
    <div class="d-kpi"><span class="k-label">Episodes</span><b class="num">{episodes}</b><span class="k-sub">from {shows} {shows === 1 ? 'show' : 'shows'} in {days === 7 ? 'the next week' : `the next ${days} days`}</span></div>
    <div class="d-kpi"><span class="k-label">Season premieres</span><b class="num">{premieres}</b><span class="k-sub">new seasons starting</span></div>
    <div class="d-kpi"><span class="k-label">Finales</span><b class="num">{finales}</b><span class="k-sub">seasons ending</span></div>
    <div class="d-kpi yours"><span class="k-label">From shows you track</span><b class="num">{mine}</b><span class="k-sub">{mine ? 'rows below are marked' : 'add shows to see them here'}</span></div>
  </section>

  {#if !list.length}
    <section class="d-empty">
      <h2>{rows.length ? 'Nothing matches' : `No episodes coming up in ${inCountry(country)}`}</h2>
      <p>{rows.length ? 'Pick Everything to see every show.' : 'The calendar covers shows metareel knows that stream here; it fills in as episodes are refreshed.'}</p>
    </section>
  {:else}
    <div class="d-layout">
      <div class="d-main">
        {#each groups as [d, items] (d)}
          <section class="d-day" aria-label={dayHead(d)}>
            <h3>{dayHead(d)} <span class="hint num">{items.length} {items.length === 1 ? 'show' : 'shows'}</span></h3>
            <ul class="rows">
              {#each items as it (`${it.tmdb_id}:${it.season_number}`)}
                {@const st = status(it)}
                {@const t = library.get('tv', it.tmdb_id)}
                <li class:mine={!!st}>
                  <a class="pic" href="#/title/tv/{it.tmdb_id}" tabindex="-1" aria-hidden="true"><Poster path={it.poster_path} seed={it.tmdb_id} title={it.title} width={46} radius={6} size="w92" /></a>
                  <div class="txt">
                    <a class="name" href="#/title/tv/{it.tmdb_id}">{it.title}</a>
                    <span class="ep">
                      <b class="num">{eps(it)}</b>
                      {#if it.episodes?.length > 1}<span class="tag">{it.episodes.length} episodes at once</span>{/if}
                      {#if it.season_premiere}<span class="tag new">Premiere</span>{/if}
                      {#if it.season_finale}<span class="tag end">Finale</span>{/if}
                      {#if epName(it)}<span class="muted">“{epName(it)}”</span>{/if}
                    </span>
                    <span class="svc">
                      {#each it.services.slice(0, 3) as s}<span class="d-chip" class:yours={yours.has(s)}>{s}</span>{/each}
                      {#if st}<span class="st"><Icon name="check" size={12} />{st}</span>{/if}
                    </span>
                  </div>
                  <div class="acts">
                    {#if t}<a class="btn-secondary sm" href="#/show/{it.tmdb_id}">Episodes</a>{/if}
                    <button class="mark" aria-pressed={t?.status === 'watchlist'} aria-label="{t?.status === 'watchlist' ? 'Remove from' : 'Add to'} watchlist: {it.title}" onclick={() => toggle(it)}>
                      <Icon name={t?.status === 'watchlist' ? 'bookmark-fill' : 'bookmark'} size={15} />
                    </button>
                  </div>
                </li>
              {/each}
            </ul>
          </section>
        {/each}
      </div>
      <aside class="d-side">
        <section class="panel pad">
          <h2>By service</h2>
          <p class="hint">Rows per service, with the filters above.</p>
          {#if byService.length}<BarList items={byService} />{:else}<p class="hint">No offers listed.</p>{/if}
        </section>
        <p class="d-source">Air dates from TMDB (usually the original release, not per country); services are where each show streams in {inCountry(country)} now, data by JustWatch.</p>
      </aside>
    </div>
  {/if}
{/if}

<style>
  .rows { display: grid; margin: 0; padding: 0; list-style: none; }
  .rows li { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 14px; padding: 10px 6px; box-shadow: inset 0 -1px 0 var(--border-subtle); }
  .rows li.mine { background: color-mix(in srgb, var(--accent-yours) 6%, transparent); border-radius: 10px; }
  .pic { display: block; }
  .txt { display: grid; gap: 4px; min-width: 0; }
  .name { font: 700 16px/1.25 var(--font-display); color: inherit; text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .name:hover { text-decoration: underline; text-underline-offset: 3px; }
  .ep { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 8px; font-size: 13.5px; }
  .ep .muted { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 34ch; }
  .tag { padding: 2px 7px; border-radius: 8px; background: var(--bg-elevated); color: var(--text-muted); font-size: 11.5px; font-weight: 700; }
  .tag.new { color: var(--delta-debut); }
  .tag.end { color: var(--delta-down); }
  .svc { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 6px; }
  .st { display: inline-flex; align-items: center; gap: 4px; color: var(--accent-yours); font-size: 12px; font-weight: 600; }
  .acts { display: flex; align-items: center; gap: 8px; }
  .sm { height: 30px; padding: 0 12px; font-size: 13px; }
  .mark { display: grid; place-items: center; width: 30px; height: 30px; border: 0; border-radius: 50%; background: var(--bg-elevated); color: var(--text-muted); cursor: pointer; }
  .mark:hover { color: var(--text-primary); }
  .mark[aria-pressed='true'] { background: color-mix(in srgb, var(--accent-yours) 20%, transparent); color: var(--accent-yours); }
  @media (max-width: 640px) {
    .acts .sm { display: none; }
  }
</style>
