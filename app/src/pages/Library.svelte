<!-- Library. Everything you track: status, favourites, your rating (from your latest rated watch)
     and how often you've watched it. Read from the library map loaded at sign-in, so filters,
     counts and sorting are instant and every change elsewhere shows here at once.
     URL: ?status=watching|completed|watchlist|on_hold|dropped|not_interested|watched&kind=movie|tv
          &fav=1&sort=recent|rating|title|release|added&view=grid|list -->
<script>
  import Poster from '../components/Poster.svelte'
  import Icon from '../components/Icon.svelte'
  import TitleMenu from '../components/TitleMenu.svelte'
  import { library } from '../lib/library.svelte.js'
  import { route, setQuery } from '../lib/router.svelte.js'
  import { STATUSES, statusLabel } from '../lib/status.js'
  import { stars, starsLabel } from '../lib/rows.js'
  import { fromUTC, localDay, dayLabel } from '../lib/dates.js'

  let { onlog } = $props()

  const SORTS = [['recent', 'Recently watched'], ['rating', 'Your rating'], ['title', 'Title'], ['release', 'Newest release'], ['added', 'Recently added']]
  const status = $derived(STATUSES.some(([v]) => v === route.query.status) || route.query.status === 'watched' ? route.query.status : '')
  const kind = $derived(route.query.kind === 'movie' || route.query.kind === 'tv' ? route.query.kind : '')
  const fav = $derived(route.query.fav === '1')
  const sort = $derived(SORTS.some(([v]) => v === route.query.sort) ? route.query.sort : 'recent')
  const view = $derived(route.query.view === 'list' ? 'list' : 'grid')

  const all = $derived([...library.byKey.values()])
  // "Watched" = has watches but no status; every other chip is a status.
  const matches = (t, s) => (s === 'watched' ? !t.status && t.watch_count > 0 : t.status === s)
  const counts = $derived(Object.fromEntries([...STATUSES.map(([v]) => v), 'watched'].map((s) => [s, all.filter((t) => matches(t, s)).length])))
  const chips = $derived([
    ['', 'Everything', all.length],
    ...STATUSES.filter(([v]) => v !== 'not_interested' || counts[v]).map(([v, l]) => [v, l, counts[v]]),
    ...(counts.watched ? [['watched', 'Watched', counts.watched]] : []),
  ])

  const last = (t) => t.last_watched_at ?? ''
  const rows = $derived(
    all
      .filter((t) => !status || matches(t, status))
      .filter((t) => !kind || t.tmdb_kind === kind)
      .filter((t) => !fav || t.favorite)
      .sort((a, b) =>
        sort === 'rating' ? (b.rating ?? 0) - (a.rating ?? 0) || last(b).localeCompare(last(a))
        : sort === 'title' ? (a.title ?? '').localeCompare(b.title ?? '')
        : sort === 'release' ? (b.release_date ?? '').localeCompare(a.release_date ?? '')
        : sort === 'added' ? (b.created_at ?? '').localeCompare(a.created_at ?? '')
        : last(b).localeCompare(last(a)) || (b.updated_at ?? '').localeCompare(a.updated_at ?? ''))
  )

  // Your ratings, for the tile: average and a half-star histogram (1–10).
  const rated = $derived(all.filter((t) => t.rating))
  const avg = $derived(rated.length ? rated.reduce((s, t) => s + t.rating, 0) / rated.length / 2 : null)
  const hist = $derived(Array.from({ length: 10 }, (_, i) => rated.filter((t) => t.rating === i + 1).length))
  const histMax = $derived(Math.max(1, ...hist))
  const films = $derived(all.filter((t) => t.tmdb_kind === 'movie').length)
  const favourites = $derived(all.filter((t) => t.favorite).length)
  const rewatched = $derived(all.filter((t) => t.watch_count > 1).length)

  const year = (d) => (d ? d.slice(0, 4) : '')
  const lastLabel = (t) => {
    const d = fromUTC(t.last_watched_at)
    return d ? dayLabel(localDay(d)) : ''
  }
</script>

<div class="page">
  <header class="head">
    <h1>Library</h1>
    <span class="count num">{all.length} title{all.length === 1 ? '' : 's'}</span>
  </header>

  {#if !library.loaded}
    <div class="skeleton" style="height:420px" aria-busy="true" aria-label="Loading"></div>
  {:else if !all.length}
    <section class="empty">
      <h2>Your library is empty</h2>
      <p>Everything you add to your watchlist, log a watch of, or give a status lands here.</p>
      <a class="btn-primary" href="#/charts">Browse the charts</a>
    </section>
  {:else}
    <section class="kpis" aria-label="Your library at a glance">
      <div class="kpi"><span class="k-label">Titles</span><b class="num">{all.length}</b><span class="k-sub">{films} film{films === 1 ? '' : 's'} · {all.length - films} show{all.length - films === 1 ? '' : 's'}</span></div>
      <div class="kpi rating">
        <span class="k-label">Your ratings</span>
        <span class="k-row">
          <b class="num">{avg ? avg.toFixed(1) : '–'}<span class="of">{avg ? ' ★ average' : ''}</span></b>
          <span class="hist" role="img" aria-label="Rating spread: {hist.map((n, i) => `${(i + 1) / 2} stars: ${n}`).join(', ')}">
            {#each hist as n, i}<i style:height="{Math.max(2, (n / histMax) * 34)}px" class:half={i % 2 === 0} title="{(i + 1) / 2} ★: {n}"></i>{/each}
          </span>
        </span>
        <span class="k-sub">{rated.length} rated · from your latest watch of each</span>
      </div>
      <button class="kpi" aria-pressed={fav} onclick={() => setQuery({ fav: fav ? '' : '1' })}>
        <span class="k-label">Favourites</span><b class="num yours">{favourites}</b><span class="k-sub">{fav ? 'Showing favourites only' : 'Show only favourites'}</span>
      </button>
      <div class="kpi"><span class="k-label">Rewatched</span><b class="num">{rewatched}</b><span class="k-sub">Titles watched more than once</span></div>
    </section>

    <nav class="chips" aria-label="Status">
      {#each chips as [v, label, n]}
        <button aria-pressed={status === v} onclick={() => setQuery({ status: v })}>{label}<span class="n num">{n}</span></button>
      {/each}
    </nav>

    <div class="bar">
      <div class="seg" role="group" aria-label="Kind">
        <button aria-pressed={kind === ''} onclick={() => setQuery({ kind: '' })}>Everything</button>
        <button aria-pressed={kind === 'movie'} onclick={() => setQuery({ kind: 'movie' })}>Films</button>
        <button aria-pressed={kind === 'tv'} onclick={() => setQuery({ kind: 'tv' })}>Shows</button>
      </div>
      <label class="sort">Sort
        <select value={sort} onchange={(e) => setQuery({ sort: e.currentTarget.value === 'recent' ? '' : e.currentTarget.value })}>
          {#each SORTS as [v, l]}<option value={v}>{l}</option>{/each}
        </select>
      </label>
      <div class="seg views" role="group" aria-label="View">
        <button aria-pressed={view === 'grid'} aria-label="Posters" title="Posters" onclick={() => setQuery({ view: '' })}><Icon name="grid" size={15} /></button>
        <button aria-pressed={view === 'list'} aria-label="List" title="List" onclick={() => setQuery({ view: 'list' })}><Icon name="list" size={15} /></button>
      </div>
    </div>

    {#if !rows.length}
      <p class="none">Nothing here matches. <button class="link" onclick={() => setQuery({ status: '', kind: '', fav: '' })}>Show everything</button></p>
    {:else if view === 'grid'}
      <ul class="grid" aria-label="{rows.length} titles">
        {#each rows as t (t.tmdb_kind + t.tmdb_id)}
          <li class="card">
            <a class="art" href="#/title/{t.tmdb_kind}/{t.tmdb_id}" aria-label={t.title}>
              <Poster path={t.poster_path} seed={t.tmdb_id} title={t.title ?? ''} width={168} radius={10} size="w342" />
              {#if t.favorite}<span class="fav" aria-hidden="true"><Icon name="heart-fill" size={13} /></span>{/if}
            </a>
            <div class="info">
              <a class="name" href="#/title/{t.tmdb_kind}/{t.tmdb_id}" tabindex="-1">{t.title}</a>
              <span class="meta">{t.tmdb_kind === 'tv' ? 'Series' : 'Film'}{#if year(t.release_date)}{' · '}{year(t.release_date)}{/if}</span>
              <span class="line">
                <span class="st st-{t.status || 'none'}">{statusLabel(t)}</span>
                {#if t.rating}<span class="stars" aria-label={starsLabel(t.rating)}>{stars(t.rating)}</span>{/if}
              </span>
            </div>
            <div class="more"><TitleMenu {t} {onlog} /></div>
          </li>
        {/each}
      </ul>
    {:else}
      <div class="table-wrap">
        <table class="list">
          <caption class="sr">Your library, {rows.length} titles</caption>
          <thead><tr><th scope="col">Title</th><th scope="col">Status</th><th scope="col">Your rating</th><th scope="col" class="right">Watches</th><th scope="col">Last watched</th><th scope="col"><span class="sr">Options</span></th></tr></thead>
          <tbody>
            {#each rows as t (t.tmdb_kind + t.tmdb_id)}
              <tr>
                <td class="t">
                  <a class="cell" href="#/title/{t.tmdb_kind}/{t.tmdb_id}">
                    <Poster path={t.poster_path} seed={t.tmdb_id} title={t.title ?? ''} width={34} radius={4} size="w92" />
                    <span class="text"><span class="nm">{t.title}{#if t.favorite}{' '}<span class="heart" aria-label="Favourite"><Icon name="heart-fill" size={12} /></span>{/if}</span>
                      <span class="meta">{t.tmdb_kind === 'tv' ? 'Series' : 'Film'}{#if year(t.release_date)}{' · '}{year(t.release_date)}{/if}{#if t.tmdb_kind === 'tv' && t.episodes_watched}{' · '}{t.episodes_watched} episode{t.episodes_watched === 1 ? '' : 's'} seen{/if}</span></span>
                  </a>
                </td>
                <td><span class="st st-{t.status || 'none'}">{statusLabel(t)}</span></td>
                <td>{#if t.rating}<span class="stars" aria-label={starsLabel(t.rating)}>{stars(t.rating)}</span>{:else}<span class="muted">–</span>{/if}</td>
                <td class="right num">{t.watch_count || '–'}</td>
                <td class="muted">{lastLabel(t) || '–'}</td>
                <td class="me"><TitleMenu {t} {onlog} /></td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  {/if}
</div>

<style>
  .page { padding: 20px var(--gutter) 48px; }
  .head { display: flex; align-items: baseline; gap: 14px; margin-bottom: 16px; }
  h1 { margin: 0; font: 700 32px/1.1 var(--font-display); letter-spacing: -0.02em; }
  .count { color: var(--text-muted); font-size: 15px; }

  .kpis { display: grid; grid-template-columns: 1fr 1.5fr 1fr 1fr; gap: 12px; }
  .kpi { display: grid; gap: 4px; padding: 16px 18px; border: 0; border-radius: 16px; background: var(--bg-surface); color: var(--text-primary); text-align: left; font: inherit; }
  button.kpi { cursor: pointer; }
  button.kpi:hover { background: var(--bg-elevated); }
  button.kpi[aria-pressed='true'] { box-shadow: inset 0 0 0 2px var(--accent-yours); }
  .k-label { color: var(--text-muted); font-size: 13px; font-weight: 600; }
  .kpi b { font: 700 32px/1.05 var(--font-sans); letter-spacing: -0.03em; }
  .kpi b.yours { color: var(--accent-yours); }
  .of { color: var(--text-muted); font-size: 14px; font-weight: 600; letter-spacing: 0; }
  .k-sub { color: var(--text-muted); font-size: 12.5px; }
  .k-row { display: flex; align-items: flex-end; justify-content: space-between; gap: 12px; }
  .k-row b { white-space: nowrap; }
  .hist { display: flex; align-items: flex-end; gap: 3px; height: 36px; }
  .hist i { width: 9px; border-radius: 2px 2px 0 0; background: var(--accent-yours); }
  .hist i.half { opacity: 0.6; }

  .chips { display: flex; flex-wrap: wrap; gap: 8px; margin: 22px 0 0; }
  .chips button { display: inline-flex; align-items: center; gap: 8px; height: 34px; padding: 0 8px 0 14px; border: 0; border-radius: 17px; background: var(--bg-surface); color: var(--text-muted); font: 600 14px/1 var(--font-sans); cursor: pointer; }
  .chips button:hover { color: var(--text-primary); }
  .chips button[aria-pressed='true'] { background: var(--accent-primary); color: var(--accent-on); }
  .n { min-width: 22px; padding: 3px 6px; border-radius: 10px; background: var(--bg-elevated); color: var(--text-primary); font-size: 12px; text-align: center; }
  .chips button[aria-pressed='true'] .n { background: color-mix(in srgb, var(--accent-on) 18%, transparent); color: var(--accent-on); }

  .bar { display: flex; flex-wrap: wrap; align-items: center; gap: 12px 16px; margin: 16px 0 14px; }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { display: grid; place-items: center; height: 30px; padding: 0 14px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .views button { padding: 0 10px; }
  .seg button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }
  .sort { display: flex; align-items: center; gap: 8px; margin-left: auto; color: var(--text-muted); font-size: 13.5px; font-weight: 600; }
  .sort select { height: 32px; padding: 0 12px; border: 0; border-radius: 16px; background: var(--bg-elevated); color: var(--text-primary); font: 600 13.5px/1 var(--font-sans); }

  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(168px, 1fr)); gap: 26px 20px; margin: 0; padding: 0; list-style: none; }
  .card { position: relative; display: grid; gap: 10px; align-content: start; }
  .art { position: relative; display: block; border-radius: 10px; transition: transform 0.15s ease; }
  .art :global(.poster) { width: 100% !important; }
  .art:hover { transform: translateY(-3px); }
  .fav { position: absolute; top: 8px; left: 8px; display: grid; place-items: center; width: 26px; height: 26px; border-radius: 50%; background: var(--accent-yours); color: var(--bg-canvas); }
  .info { display: grid; gap: 2px; min-width: 0; padding-right: 36px; }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 600 16px/1.25 var(--font-display); text-decoration: none; }
  .name:hover { text-decoration: underline; text-underline-offset: 3px; }
  .meta { color: var(--text-muted); font-size: 12.5px; }
  .line { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; margin-top: 2px; }
  .more { position: absolute; right: 0; bottom: 6px; }

  .st { font-size: 12.5px; font-weight: 700; color: var(--text-muted); }
  .st-watching { color: var(--delta-up); }
  .st-completed { color: var(--text-primary); }
  .st-watchlist { color: var(--accent-yours); }
  .st-dropped, .st-not_interested { color: var(--delta-down); }
  .stars { color: var(--accent-yours); font-size: 13.5px; letter-spacing: 0.04em; white-space: nowrap; }

  .table-wrap { overflow-x: auto; }
  .list { width: 100%; border-collapse: separate; border-spacing: 0; }
  .list th { height: 34px; padding: 0 12px; color: var(--text-muted); font: 600 12.5px/1 var(--font-sans); text-align: left; white-space: nowrap; }
  .list td { height: 60px; padding: 4px 12px; vertical-align: middle; white-space: nowrap; }
  .list tbody tr:not(:first-child) td { box-shadow: inset 0 1px 0 var(--border-subtle); }
  .list tbody tr:hover td { background: var(--bg-elevated); }
  .list tbody tr:hover td:first-child { border-radius: 12px 0 0 12px; }
  .list tbody tr:hover td:last-child { border-radius: 0 12px 12px 0; }
  .t { width: 46%; }
  .cell { display: flex; align-items: center; gap: 12px; min-width: 0; color: inherit; text-decoration: none; }
  .text { display: grid; min-width: 0; }
  .nm { overflow: hidden; text-overflow: ellipsis; font: 600 16px/1.25 var(--font-display); }
  .cell:hover .nm { text-decoration: underline; text-underline-offset: 3px; }
  .heart { color: var(--accent-yours); display: inline-grid; vertical-align: -1px; }
  .me { width: 1%; }

  .empty { padding: 32px 0; display: grid; gap: 10px; justify-items: start; }
  .empty h2 { margin: 0; font: 700 24px/1.2 var(--font-display); }
  .empty p { margin: 0; color: var(--text-muted); max-width: 60ch; }
  .none { color: var(--text-muted); }
  .link { border: 0; background: none; color: var(--text-primary); text-decoration: underline; cursor: pointer; font: inherit; }

  @media (max-width: 960px) { .kpis { grid-template-columns: 1fr 1fr; } }
  @media (max-width: 560px) {
    /* Ratings get a full row on top; the three counts share the row below. */
    .kpis { grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; }
    .kpi { padding: 12px; }
    .kpi b { font-size: 26px; }
    .kpi.rating { grid-column: 1 / -1; order: -1; }
    .kpi:not(.rating) .k-sub { font-size: 11.5px; }
    .grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px 14px; }
    .sort { margin-left: 0; }
  }
  @media (prefers-reduced-motion: reduce) { .art { transition: none; } }
</style>
