<!-- Hidden gems: highly rated (IMDb 7.5+ from 5,000+ votes), streamable in the country on a
     subscription, free or with ads, and not in its Top 10 over the last 90 days. One request
     (up to 200); kind, service, rating and "hide what I've seen" filter in the browser. -->
<script>
  import Poster from '../../components/Poster.svelte'
  import Icon from '../../components/Icon.svelte'
  import BarList from '../../components/BarList.svelte'
  import { api } from '../../lib/api.js'
  import { library } from '../../lib/library.svelte.js'
  import { prefs } from '../../lib/prefs.svelte.js'
  import { route, setQuery } from '../../lib/router.svelte.js'
  import { inCountry } from '../../lib/scope.svelte.js'
  import { toast } from '../../lib/toast.svelte.js'
  import { servicesOf, votes, regionName } from '../../lib/services.js'
  import { dayLabel } from '../../lib/dates.js'

  let { country, families, onlog } = $props()

  let data = $state(null)
  let error = $state('')
  $effect(() => {
    const cc = country
    const ctrl = new AbortController()
    data = null
    error = ''
    api.hiddenGems(cc, { limit: 200 }, ctrl.signal)
      .then((d) => (data = d))
      .catch((err) => { if (err.name !== 'AbortError') error = err.message })
    return () => ctrl.abort()
  })

  // Filters live in the URL.
  const kind = $derived(['movie', 'tv'].includes(route.query.kind) ? route.query.kind : '')
  const service = $derived(route.query.service ?? '')
  const RATINGS = [[7.5, '7.5+'], [8, '8+'], [8.5, '8.5+']]
  const minRating = $derived(RATINGS.some(([r]) => r === +route.query.rating) ? +route.query.rating : 7.5)
  const showSeen = $derived(route.query.seen === '1')
  let shown = $state(24)
  $effect(() => { kind; service; minRating; showSeen; shown = 24 })

  const yours = $derived(new Set((prefs.values.services ?? []).map((id) => families.get(id)).filter(Boolean)))
  const seen = (it) => {
    const t = library.get(it.kind, it.tmdb_id)
    return !!t && (t.watch_count > 0 || t.status === 'completed')
  }
  const items = $derived((data?.items ?? []).map((it) => ({ ...it, services: servicesOf(it.streaming_here, families) })))
  // Everything but the service filter, so the service tally shows what each one would leave.
  const base = $derived(items.filter((it) => (!kind || it.kind === kind) && (it.imdb_rating ?? 0) >= minRating && (showSeen || !seen(it))))
  const list = $derived(base.filter((it) => !service || (service === '__yours' ? it.services.some((s) => yours.has(s)) : it.services.includes(service))))
  const seenCount = $derived(items.filter(seen).length)

  const tally = (pick) => {
    const m = new Map()
    for (const it of pick) for (const k of it) m.set(k, (m.get(k) ?? 0) + 1)
    return [...m].sort((a, b) => b[1] - a[1])
  }
  const byService = $derived(tally(base.map((it) => it.services)).slice(0, 8))
  const byGenre = $derived(tally(list.map((it) => it.genres ?? [])).slice(0, 8).map(([label, value]) => ({ label, value })))
  const onYours = $derived(base.filter((it) => it.services.some((s) => yours.has(s))).length)

  async function toggle(it) {
    const was = library.get(it.kind, it.tmdb_id)?.status === 'watchlist'
    try {
      await library.toggleWatchlist(it.kind, it.tmdb_id)
      toast(was ? `Removed ${it.title} from your watchlist.` : `Added ${it.title} to your watchlist.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }
  const pick = (patch) => setQuery(patch, { replace: true })
</script>

{#if error}
  <p class="alert" role="alert">Couldn’t load hidden gems: {error}</p>
{:else if !data}
  <div class="skeleton" style="height:520px" aria-busy="true" aria-label="Loading"></div>
{:else if !data.total}
  <section class="empty">
    <h2>No hidden gems found in {inCountry(country)}</h2>
    <p>Gems need IMDb ratings and streaming availability in {inCountry(country)}; metareel gathers both as it maps the charts. Try another country in the top bar.</p>
  </section>
{:else}
  <section class="kpis" aria-label="Hidden gems at a glance">
    <div class="kpi"><span class="k-label">Gems</span><b class="num">{list.length}</b><span class="k-sub" title="The {items.length} best rated are loaded; filters apply to those">{items.length < data.total ? `the best ${items.length} of ${data.total}` : `of ${data.total}`} rated {data.min_rating}+ by {votes(data.min_votes)}+ people</span></div>
    <div class="kpi"><span class="k-label">On your services</span><b class="num">{yours.size ? onYours : '–'}</b><span class="k-sub">{yours.size ? [...yours].join(', ') : 'Pick the services you pay for under Which services to pay for'}</span></div>
    <div class="kpi"><span class="k-label">Seen already</span><b class="num">{seenCount}</b><span class="k-sub">{showSeen ? 'shown' : 'hidden from the list'}</span></div>
    <div class="kpi"><span class="k-label">Out of the charts</span><b class="num">90 days</b><span class="k-sub">not in the Top 10 in {inCountry(country)} since {dayLabel(data.from)}</span></div>
  </section>

  <div class="filters">
    <div class="seg" role="group" aria-label="Kind">
      {#each [['', 'All'], ['movie', 'Films'], ['tv', 'Shows']] as [v, l]}<button aria-pressed={kind === v} onclick={() => pick({ kind: v })}>{l}</button>{/each}
    </div>
    <div class="seg" role="group" aria-label="IMDb rating">
      {#each RATINGS as [r, l]}<button aria-pressed={minRating === r} onclick={() => pick({ rating: r === 7.5 ? '' : r })}>IMDb {l}</button>{/each}
    </div>
    <label class="check"><input type="checkbox" checked={showSeen} onchange={(e) => pick({ seen: e.currentTarget.checked ? '1' : '' })} /> Show what I’ve seen</label>
  </div>

  <div class="layout">
    <div class="main">
      {#if service}
        <p class="filtering">On <b>{service === '__yours' ? 'your services' : service}</b> · {list.length} {list.length === 1 ? 'title' : 'titles'} <button class="link" onclick={() => pick({ service: '' })}>Show all</button></p>
      {/if}
      {#if list.length}
        <ul class="grid">
          {#each list.slice(0, shown) as it (it.kind + it.tmdb_id)}
            {@const t = library.get(it.kind, it.tmdb_id)}
            <li class="tile" class:mine={!!t}>
              <a class="art" href="#/title/{it.kind}/{it.tmdb_id}">
                <Poster path={it.poster_path ?? null} seed={it.tmdb_id} title={it.title} width={170} radius={10} size="w342" />
                <span class="score num" title="IMDb {it.imdb_rating} from {it.imdb_votes?.toLocaleString()} votes">★ {it.imdb_rating?.toFixed(1)}</span>
              </a>
              <div class="tt">
                <a class="name" href="#/title/{it.kind}/{it.tmdb_id}">{it.title}</a>
                <button class="mark" aria-pressed={t?.status === 'watchlist'} aria-label="{t?.status === 'watchlist' ? 'Remove from' : 'Add to'} watchlist: {it.title}" title={t?.status === 'watchlist' ? 'On your watchlist' : 'Add to watchlist'} onclick={() => toggle(it)}>
                  <Icon name={t?.status === 'watchlist' ? 'bookmark-fill' : 'bookmark'} size={15} />
                </button>
              </div>
              <p class="meta">{it.year ?? ''}{it.year ? ' · ' : ''}{it.kind === 'tv' ? 'Series' : 'Film'}{#if it.genres?.length}{' · '}{it.genres.slice(0, 2).join(', ')}{/if}</p>
              <p class="svc">
                {#each it.services.slice(0, 2) as s}<span class="chip" class:yours={yours.has(s)}>{s}</span>{/each}
                {#if it.services.length > 2}<span class="more">+{it.services.length - 2}</span>{/if}
              </p>
              {#if seen(it)}<p class="note seen"><Icon name="check" size={12} />Seen</p>
              {:else if it.charted_in?.length}<p class="note">Charting in {it.charted_in.slice(0, 2).map(regionName).join(', ')}</p>{/if}
            </li>
          {/each}
        </ul>
        {#if list.length > shown}
          <button class="btn-secondary more-btn" onclick={() => (shown += 24)}>Show more <span class="hint">{list.length - shown} left</span></button>
        {/if}
      {:else}
        <p class="hint">Nothing matches. Loosen the rating, or show what you’ve seen.</p>
      {/if}
    </div>

    <aside class="side">
      <section class="panel pad">
        <h2>Where they stream</h2>
        <p class="hint">Gems per service, with the filters above. Pick one to narrow the list.</p>
        <ol class="svc-list">
          {#if yours.size}
            <li><button class:on={service === '__yours'} onclick={() => pick({ service: service === '__yours' ? '' : '__yours' })}><span>Your services</span><b class="num">{onYours}</b></button></li>
          {/if}
          {#each byService as [name, n] (name)}
            <li>
              <button class:on={service === name} class:yours={yours.has(name)} onclick={() => pick({ service: service === name ? '' : name })}>
                <span>{name}</span><i style:width="{(n / (byService[0]?.[1] || 1)) * 100}%" aria-hidden="true"></i><b class="num">{n}</b>
              </button>
            </li>
          {/each}
        </ol>
      </section>
      {#if byGenre.length}
        <section class="panel pad">
          <h2>Genres</h2>
          <BarList items={byGenre} />
        </section>
      {/if}
    </aside>
  </div>
{/if}

<style>
  .kpis { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
  .kpi { display: grid; gap: 4px; padding: 16px 18px; border-radius: 16px; background: var(--bg-surface); min-width: 0; }
  .k-label { color: var(--text-muted); font-size: 13px; font-weight: 600; }
  .kpi b { font: 700 30px/1.05 var(--font-sans); letter-spacing: -0.03em; }
  .k-sub { color: var(--text-muted); font-size: 12.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .filters { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 14px; }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { height: 30px; padding: 0 12px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .seg button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }
  .check { display: inline-flex; align-items: center; gap: 8px; font-size: 14px; cursor: pointer; }
  .check input { width: 16px; height: 16px; accent-color: var(--accent-yours); }

  .layout { display: grid; grid-template-columns: minmax(0, 1fr) 300px; gap: 20px; align-items: start; }
  .main { display: grid; gap: 14px; min-width: 0; }
  .filtering { margin: 0; font-size: 14px; }
  .link { border: 0; background: none; color: var(--text-muted); font: 600 13px/1 var(--font-sans); cursor: pointer; text-decoration: underline; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 22px 16px; margin: 0; padding: 0; list-style: none; }
  .tile { display: grid; gap: 5px; align-content: start; min-width: 0; }
  .art { position: relative; display: block; transition: transform 0.15s ease; }
  .art:hover { transform: translateY(-2px); }
  .art :global(.poster) { width: 100% !important; }
  .score { position: absolute; left: 8px; bottom: 8px; padding: 3px 7px; border-radius: 8px; background: rgb(0 0 0 / 0.72); color: #f5c518; font-size: 12.5px; font-weight: 700; }
  .tt { display: flex; align-items: flex-start; justify-content: space-between; gap: 6px; margin-top: 4px; }
  .name { font: 700 15.5px/1.25 var(--font-display); color: inherit; text-decoration: none; }
  .name:hover { text-decoration: underline; text-underline-offset: 3px; }
  .mark { flex: none; display: grid; place-items: center; width: 28px; height: 28px; border: 0; border-radius: 50%; background: var(--bg-elevated); color: var(--text-muted); cursor: pointer; }
  .mark:hover { color: var(--text-primary); }
  .mark[aria-pressed='true'] { background: color-mix(in srgb, var(--accent-yours) 20%, transparent); color: var(--accent-yours); }
  .meta { margin: 0; color: var(--text-muted); font-size: 12.5px; }
  .svc { display: flex; flex-wrap: wrap; gap: 4px; margin: 2px 0 0; }
  .chip { padding: 2px 7px; border-radius: 8px; background: var(--bg-elevated); font-size: 11.5px; font-weight: 600; white-space: nowrap; }
  .chip.yours { background: color-mix(in srgb, var(--accent-yours) 18%, transparent); color: var(--accent-yours); }
  .more { color: var(--text-muted); font-size: 11.5px; font-weight: 600; align-self: center; }
  .note { display: inline-flex; align-items: center; gap: 4px; margin: 0; color: var(--delta-debut); font-size: 12px; font-weight: 600; }
  .note.seen { color: var(--text-muted); }
  .more-btn { justify-self: center; }

  .side { display: grid; gap: 16px; position: sticky; top: 84px; }
  .side h2 { margin: 0; font: 700 18px/1.25 var(--font-display); }
  .svc-list { display: grid; gap: 2px; margin: 4px 0 0; padding: 0; list-style: none; }
  .svc-list button { position: relative; display: flex; align-items: center; justify-content: space-between; gap: 10px; width: 100%; padding: 8px 10px; border: 0; border-radius: 8px; background: none; color: var(--text-primary); font: 500 14px/1.2 var(--font-sans); text-align: left; cursor: pointer; overflow: hidden; }
  .svc-list button span, .svc-list button b { position: relative; z-index: 1; }
  .svc-list button i { position: absolute; left: 0; top: 0; bottom: 0; background: var(--bg-elevated); border-radius: 8px; }
  .svc-list button.yours span { color: var(--accent-yours); font-weight: 700; }
  .svc-list button:hover i, .svc-list button:hover { background: color-mix(in srgb, var(--text-primary) 6%, transparent); }
  .svc-list button.on { box-shadow: inset 0 0 0 1.5px var(--accent-primary); }

  .empty { padding: 30px 0; }
  .empty h2 { margin: 0 0 6px; font: 700 24px/1.2 var(--font-display); }
  .empty p { color: var(--text-muted); max-width: 60ch; }

  @media (max-width: 1100px) {
    .layout { grid-template-columns: 1fr; }
    .side { position: static; grid-template-columns: 1fr 1fr; }
  }
  @media (max-width: 760px) {
    .kpis { grid-template-columns: 1fr 1fr; }
    .side { grid-template-columns: 1fr; }
    .grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  }
</style>
