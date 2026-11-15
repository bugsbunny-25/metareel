<!-- Movies (#/movies?seg=towatch|upcoming): films from your library. To watch: out already, on
     your watchlist (or started), not watched yet, in watchlist order. Upcoming: not out yet, by
     month of first release. As a list or a poster grid; the segment and the view are remembered
     in this browser (as on mobile). The Watchlist page has where each one streams. -->
<script>
  import Poster from '../components/Poster.svelte'
  import Icon from '../components/Icon.svelte'
  import TitleMenu from '../components/TitleMenu.svelte'
  import { library } from '../lib/library.svelte.js'
  import { route, setQuery } from '../lib/router.svelte.js'
  import { remembered } from '../lib/remember.svelte.js'
  import { localDay, daysUntil, dayLabel, hm } from '../lib/dates.js'

  let { onlog } = $props()

  const savedSeg = remembered('tabs.movies.segment', 'towatch', ['towatch', 'upcoming'])
  const view = remembered('tabs.movies.view', 'list', ['list', 'grid'])
  const seg = $derived(route.query.seg === 'towatch' || route.query.seg === 'upcoming' ? route.query.seg : savedSeg.value)
  function pickSeg(v) {
    savedSeg.value = v
    setQuery({ seg: v }, { replace: true })
  }

  const today = localDay()
  const movies = $derived.by(() => {
    const all = [...library.byKey.values()].filter((t) => t.tmdb_kind === 'movie')
    const out = (t) => !t.release_date || t.release_date <= today
    const toWatch = all
      .filter((t) => (t.status === 'watchlist' || t.status === 'watching') && !t.watch_count && out(t))
      .sort((a, b) => (a.watchlist_position ?? 1e9) - (b.watchlist_position ?? 1e9) || (b.created_at ?? '').localeCompare(a.created_at ?? ''))
    const upcoming = all
      .filter((t) => !out(t) && t.status !== 'dropped' && t.status !== 'not_interested')
      .sort((a, b) => a.release_date.localeCompare(b.release_date))
    return { toWatch, upcoming, minutes: toWatch.reduce((n, t) => n + (t.runtime || 0), 0) }
  })
  const MONTH = new Intl.DateTimeFormat(undefined, { month: 'long', year: 'numeric' })
  const months = $derived.by(() => {
    const groups = []
    for (const t of movies.upcoming) {
      const key = t.release_date.slice(0, 7)
      if (groups.at(-1)?.key !== key) groups.push({ key, title: MONTH.format(new Date(`${key}-15T12:00:00`)), items: [] })
      groups.at(-1).items.push(t)
    }
    return groups
  })
  const countdown = (date) => {
    const n = daysUntil(date, today)
    return n <= 0 ? 'Today' : n === 1 ? 'Tomorrow' : n < 60 ? `${n} days` : dayLabel(date)
  }
  const year = (d) => d?.slice(0, 4) ?? ''
  const meta = (t) => [year(t.release_date), t.runtime ? hm(t.runtime) : ''].filter(Boolean).join(' · ')
  const log = (t) => onlog({ kind: 'movie', id: t.tmdb_id, name: t.title, poster: t.poster_path })
  const summary = $derived(
    seg === 'towatch'
      ? movies.toWatch.length ? `${movies.toWatch.length} film${movies.toWatch.length === 1 ? '' : 's'}${movies.minutes ? ` · about ${hm(movies.minutes)}` : ''} to watch` : ''
      : movies.upcoming.length ? `${movies.upcoming.length} coming · next: ${movies.upcoming[0].title}, ${dayLabel(movies.upcoming[0].release_date)}` : '',
  )
</script>

<div class="page">
  <header class="head">
    <h1>Movies</h1>
    <div class="tabs" role="tablist" aria-label="To watch or upcoming">
      <button role="tab" aria-selected={seg === 'towatch'} onclick={() => pickSeg('towatch')}>To watch</button>
      <button role="tab" aria-selected={seg === 'upcoming'} onclick={() => pickSeg('upcoming')}>Upcoming</button>
    </div>
    <a class="more" href="#/watchlist">Watchlist and where to stream</a>
    <div class="seg view" role="group" aria-label="View">
      <button aria-pressed={view.value === 'list'} aria-label="List" title="List" onclick={() => (view.value = 'list')}><Icon name="list" size={16} /></button>
      <button aria-pressed={view.value === 'grid'} aria-label="Grid" title="Grid" onclick={() => (view.value = 'grid')}><Icon name="grid" size={16} /></button>
    </div>
  </header>

  {#if summary}<p class="hint sum num">{summary}</p>{/if}

  {#if !library.loaded}
    <div class="skeleton" style="height:320px" aria-busy="true" aria-label="Loading"></div>
  {:else if seg === 'towatch'}
    {#if !movies.toWatch.length}
      <p class="hint">Nothing waiting. Films you add to your watchlist show up here once they’re out. Press <kbd>/</kbd> to search.</p>
    {:else if view.value === 'grid'}
      <div class="tiles">
        {#each movies.toWatch as t (t.tmdb_id)}
          <a class="ptile" href="#/title/movie/{t.tmdb_id}">
            <span class="pwrap"><Poster path={t.poster_path} seed={t.tmdb_id} title={t.title ?? ''} width={150} radius={10} size="w342" /></span>
            <span class="name">{t.title}</span>
            <span class="meta">{meta(t)}</span>
          </a>
        {/each}
      </div>
    {:else}
      <ul class="rows">
        {#each movies.toWatch as t (t.tmdb_id)}
          <li>
            <a class="art" href="#/title/movie/{t.tmdb_id}" tabindex="-1" aria-hidden="true"><Poster path={t.poster_path} seed={t.tmdb_id} title={t.title ?? ''} width={56} radius={6} size="w185" /></a>
            <div class="rbody">
              <a class="name" href="#/title/movie/{t.tmdb_id}">{t.title}</a>
              <span class="hint">{meta(t)}{#if t.watchlist_note}{' · '}<i>{t.watchlist_note}</i>{/if}</span>
            </div>
            <button class="btn-secondary sm" onclick={() => log(t)}><Icon name="check" size={14} />Log</button>
            <TitleMenu {t} {onlog} />
          </li>
        {/each}
      </ul>
    {/if}
  {:else if !movies.upcoming.length}
    <p class="hint">No unreleased films in your library. Add one to your watchlist and it shows here until it’s out.</p>
  {:else}
    {#each months as m (m.key)}
      <section class="month" aria-labelledby="mo-{m.key}">
        <h2 id="mo-{m.key}" class="pillhead">{m.title}</h2>
        {#if view.value === 'grid'}
          <div class="tiles">
            {#each m.items as t (t.tmdb_id)}
              <a class="ptile" href="#/title/movie/{t.tmdb_id}">
                <span class="pwrap">
                  <Poster path={t.poster_path} seed={t.tmdb_id} title={t.title ?? ''} width={150} radius={10} size="w342" />
                  <span class="corner">{countdown(t.release_date)}</span>
                </span>
                <span class="name">{t.title}</span>
                <span class="meta">{dayLabel(t.release_date)}</span>
              </a>
            {/each}
          </div>
        {:else}
          <ul class="rows">
            {#each m.items as t (t.tmdb_id)}
              <li>
                <a class="art" href="#/title/movie/{t.tmdb_id}" tabindex="-1" aria-hidden="true"><Poster path={t.poster_path} seed={t.tmdb_id} title={t.title ?? ''} width={56} radius={6} size="w185" /></a>
                <div class="rbody">
                  <a class="name" href="#/title/movie/{t.tmdb_id}">{t.title}</a>
                  <span class="hint">{dayLabel(t.release_date)} {year(t.release_date)}</span>
                </div>
                <span class="count">{countdown(t.release_date)}</span>
                <TitleMenu {t} {onlog} />
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    {/each}
  {/if}
</div>

<style>
  .page { padding: 20px var(--gutter) 48px; display: grid; gap: 16px; }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 18px; }
  h1 { margin: 0; font: 700 32px/1.1 var(--font-display); letter-spacing: -0.02em; }
  .tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--border-subtle); }
  .tabs button { height: 38px; padding: 0 18px; border: 0; border-bottom: 2px solid transparent; margin-bottom: -1px; background: none; color: var(--text-muted); font: 700 14px/1 var(--font-sans); letter-spacing: 0.05em; text-transform: uppercase; cursor: pointer; }
  .tabs button[aria-selected='true'] { color: var(--text-primary); border-bottom-color: var(--text-primary); }
  .more { margin-left: auto; color: var(--text-muted); font-size: 13.5px; font-weight: 600; }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { display: grid; place-items: center; width: 38px; height: 30px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); cursor: pointer; }
  .seg button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }
  .sum { margin: 0; font-size: 14px; }

  .tiles { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 18px 14px; }
  .ptile { display: grid; gap: 5px; align-content: start; min-width: 0; color: inherit; text-decoration: none; }
  .ptile:hover .name { text-decoration: underline; text-underline-offset: 3px; }
  .pwrap { position: relative; display: block; transition: transform 0.15s ease; }
  .ptile:hover .pwrap { transform: translateY(-2px); }
  .pwrap :global(.poster) { width: 100% !important; }
  .corner { position: absolute; right: 8px; top: 8px; padding: 3px 8px; border-radius: 8px; background: rgb(11 11 12 / 0.82); color: var(--accent-yours); font-size: 12.5px; font-weight: 700; }
  .ptile .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 700 15.5px/1.25 var(--font-display); }
  .meta { color: var(--text-muted); font-size: 13px; }

  .rows { display: grid; gap: 2px; margin: 0; padding: 6px; list-style: none; border-radius: 16px; background: var(--bg-surface); max-width: 980px; }
  .rows li { display: grid; grid-template-columns: 56px minmax(0, 1fr) auto auto; align-items: center; gap: 14px; padding: 8px; border-radius: 10px; }
  .rows li:hover { background: var(--bg-elevated); }
  .rbody { display: grid; gap: 3px; min-width: 0; }
  .rbody .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: inherit; font: 700 17px/1.25 var(--font-display); text-decoration: none; }
  .rbody .name:hover { text-decoration: underline; text-underline-offset: 3px; }
  .sm { height: 32px; padding: 0 14px; font-size: 13.5px; gap: 6px; }
  .count { color: var(--accent-yours); font-weight: 700; font-size: 14px; white-space: nowrap; }

  .month { display: grid; gap: 12px; }
  .pillhead { justify-self: start; margin: 8px 0 0; padding: 6px 14px; border-radius: 16px; background: var(--bg-elevated); color: var(--text-muted); font: 700 12.5px/1.2 var(--font-sans); letter-spacing: 0.06em; text-transform: uppercase; }
  kbd { padding: 1px 6px; border-radius: 5px; background: var(--bg-elevated); font: 600 12px/1.4 var(--font-sans); }

  @media (max-width: 600px) {
    .tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .more { margin-left: 0; order: 5; width: 100%; }
    .rows li { grid-template-columns: 48px minmax(0, 1fr) auto; }
    .rows li :global(details) { display: none; }
  }
</style>
