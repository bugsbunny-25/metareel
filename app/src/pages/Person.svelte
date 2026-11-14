<!-- Person page (#/person/:id?dept=&kind=&sort=&self=1). A hero with their photo and profile from
     TMDB, In numbers (credits, chart record, what you've seen of their work), the titles of theirs
     metareel charted, and the whole filmography by department. Without TMDB (no key, or it failed)
     the page keeps what metareel knows: the titles it has credits for. -->
<script>
  import Poster from '../components/Poster.svelte'
  import Icon from '../components/Icon.svelte'
  import { api } from '../lib/api.js'
  import { library } from '../lib/library.svelte.js'
  import { route, setQuery } from '../lib/router.svelte.js'
  import { toast } from '../lib/toast.svelte.js'
  import { compact } from '../lib/viz.js'
  import { stars } from '../lib/rows.js'

  let { id } = $props()

  const fmt = new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' })
  const fmtShort = new Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' })
  const longDate = (d) => fmt.format(new Date(d + 'T12:00:00Z'))
  const shortDate = (d) => fmtShort.format(new Date(d + 'T12:00:00Z'))

  let p = $state(null)
  let error = $state('')
  $effect(() => {
    const pid = id
    const ctrl = new AbortController()
    p = null
    error = ''
    api.person(pid, ctrl.signal)
      .then((x) => (p = x))
      .catch((err) => { if (err.name !== 'AbortError') error = err.status === 404 ? 'Neither metareel nor TMDB knows this person.' : err.message })
    return () => ctrl.abort()
  })
  $effect(() => { if (p?.name) document.title = `${p.name} · metareel` })

  const det = $derived(p?.details ?? null)
  // Without TMDB, the titles metareel has credits for stand in for the filmography.
  const films = $derived(
    !p ? [] : p.filmography?.length ? p.filmography
      : p.titles.filter((t) => t.title).map((t) => ({ ...t, date: null, departments: t.roles.some((r) => r === 'Actor' || r.startsWith('as ')) ? ['Acting'] : ['Crew'], popularity: 0, vote_average: 0, vote_count: 0 })),
  )
  const charted = $derived((p?.titles ?? []).filter((t) => t.days > 0))
  const chartOf = $derived(new Map((p?.titles ?? []).map((t) => [`${t.kind}:${t.tmdb_id}`, t])))

  // Talk shows, award ceremonies and documentaries list people "as Self"; they swamp an actor's
  // credits, so they are hidden unless asked for.
  const SELF = /^as (self|himself|herself|themselves|themself)\b/i
  const selfOnly = (f) => f.roles.length > 0 && f.roles.every((r) => SELF.test(r))
  const showSelf = $derived(route.query.self === '1')
  const selfCount = $derived(films.filter(selfOnly).length)

  const today = new Date().toISOString().slice(0, 10)
  const upcoming = (f) => !f.date || f.date > today
  const seenIt = (f) => {
    const t = library.get(f.kind, f.tmdb_id)
    return !!t && (t.watch_count > 0 || t.status === 'completed')
  }

  // Departments, biggest first, with the person's own department leading.
  const base = $derived(films.filter((f) => showSelf || !selfOnly(f)))
  const depts = $derived.by(() => {
    const n = new Map()
    for (const f of base) for (const d of f.departments) n.set(d, (n.get(d) ?? 0) + 1)
    const known = det?.known_for ?? p?.known_for
    return [...n].sort((a, b) => (b[0] === known) - (a[0] === known) || b[1] - a[1])
  })
  const dept = $derived(depts.some(([d]) => d === route.query.dept) ? route.query.dept : depts[0]?.[0] ?? '')
  const kind = $derived(['movie', 'tv'].includes(route.query.kind) ? route.query.kind : '')
  const SORTS = [['new', 'Newest'], ['popular', 'Popular'], ['rating', 'Top rated']]
  const sort = $derived(SORTS.some(([s]) => s === route.query.sort) ? route.query.sort : 'new')
  const rated = (f) => (f.vote_count >= 50 ? f.vote_average : 0)
  const list = $derived.by(() => {
    const out = base.filter((f) => (!dept || f.departments.includes(dept)) && (!kind || f.kind === kind))
    if (sort === 'popular') return out.slice().sort((a, b) => b.popularity - a.popularity)
    if (sort === 'rating') return out.slice().sort((a, b) => rated(b) - rated(a) || b.vote_count - a.vote_count)
    return out
  })
  const kinds = $derived({ movie: base.filter((f) => (!dept || f.departments.includes(dept)) && f.kind === 'movie').length, tv: base.filter((f) => (!dept || f.departments.includes(dept)) && f.kind === 'tv').length })
  // Newest first puts what's still to come in its own list above the grid, soonest first.
  const soon = $derived(sort === 'new' ? list.filter(upcoming).sort((a, b) => (a.date ?? '9999') < (b.date ?? '9999') ? -1 : 1) : [])
  const grid = $derived(sort === 'new' ? list.filter((f) => !upcoming(f)) : list)
  let shown = $state(30)
  $effect(() => { dept; kind; sort; showSelf; shown = 30 })

  // Your history with their work, over everything they're credited on.
  const mine = $derived(films.map((f) => library.get(f.kind, f.tmdb_id)).filter(Boolean))
  const watched = $derived(mine.filter((t) => t.watch_count > 0 || t.status === 'completed'))
  const onList = $derived(mine.filter((t) => t.status === 'watchlist').length)
  const ratings = $derived(watched.map((t) => t.rating).filter(Boolean))
  const avg = $derived(ratings.length ? ratings.reduce((a, b) => a + b, 0) / ratings.length : 0)
  const released = $derived(films.filter((f) => !upcoming(f)).length)
  const next = $derived(films.filter((f) => f.date && f.date > today).at(-1) ?? null)

  const age = (from, to) => {
    const a = new Date(from + 'T12:00:00Z'), b = to ? new Date(to + 'T12:00:00Z') : new Date()
    return b.getUTCFullYear() - a.getUTCFullYear() - (b.getUTCMonth() < a.getUTCMonth() || (b.getUTCMonth() === a.getUTCMonth() && b.getUTCDate() < a.getUTCDate()) ? 1 : 0)
  }
  let bioOpen = $state(false)
  // Collapsed: the first paragraph, clamped to five lines.
  const bioFirst = $derived(det?.biography?.split(/\n\s*\n/)[0] ?? '')
  const longBio = $derived(!!det?.biography && (det.biography.length > bioFirst.length || bioFirst.length > 520))
  const DEPT_LABEL = { Acting: 'Acting', Directing: 'Directing', Writing: 'Writing', Production: 'Production', Sound: 'Music & sound', Camera: 'Camera', Editing: 'Editing', Art: 'Art', 'Costume & Make-Up': 'Costume & make-up', 'Visual Effects': 'Visual effects', Crew: 'Crew', Lighting: 'Lighting', Creator: 'Created' }
  const deptLabel = (d) => DEPT_LABEL[d] ?? d
  const roleLine = (f) => {
    const own = f.roles.filter((r) => (dept === 'Acting') === (r === 'Actor' || r.startsWith('as ')))
    const shownRoles = (own.length ? own : f.roles).slice(0, 2).join(' · ')
    return shownRoles + (f.episodes && f.kind === 'tv' ? ` · ${f.episodes} ep${f.episodes === 1 ? '' : 's'}` : '')
  }

  const pick = (patch) => setQuery(patch, { replace: true })
  async function toggle(f) {
    try {
      const on = await library.toggleWatchlist(f.kind, f.tmdb_id)
      toast(on ? `Added ${f.title} to your watchlist.` : `Removed ${f.title} from your watchlist.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }
</script>

{#if error}
  <p class="alert" role="alert">{error}</p>
{:else if !p}
  <div class="page" aria-busy="true" aria-label="Loading"><div class="skeleton" style="height:300px"></div></div>
{:else}
  <article class="page">
    <header class="head">
      <div class="face">
        {#if p.profile_path}
          <img src="https://image.tmdb.org/t/p/w342{p.profile_path}" alt="" />
        {:else}
          <Poster title={p.name} seed={p.person_id} width={168} radius={14} />
        {/if}
      </div>
      <div class="copy">
        {#if det?.known_for ?? p.known_for}<p class="kicker">Known for {deptLabel(det?.known_for ?? p.known_for).toLowerCase()}</p>{/if}
        <h1>{p.name}</h1>
        {#if det}
          <p class="meta">
            {#if det.birthday}
              {#if det.deathday}{longDate(det.birthday)} – {longDate(det.deathday)} (aged {age(det.birthday, det.deathday)}){:else}Born {longDate(det.birthday)} (age {age(det.birthday)}){/if}
            {/if}
            {#if det.place_of_birth}{det.birthday ? ' · ' : 'Born in '}{det.place_of_birth}{/if}
          </p>
          {#if det.biography}
            <p class="bio" class:clamped={longBio && !bioOpen}>{longBio && !bioOpen ? bioFirst : det.biography}</p>
            {#if longBio}<button class="link" onclick={() => (bioOpen = !bioOpen)} aria-expanded={bioOpen}>{bioOpen ? 'Show less' : 'Read more'}</button>{/if}
          {/if}
          <p class="links">
            {#if det.imdb_id}<a href="https://www.imdb.com/name/{det.imdb_id}/" target="_blank" rel="noopener noreferrer">IMDb</a>{/if}
            <a href="https://www.themoviedb.org/person/{p.person_id}" target="_blank" rel="noopener noreferrer">TMDB</a>
            {#if det.homepage && /^https?:\/\//.test(det.homepage)}<a href={det.homepage} target="_blank" rel="noopener noreferrer">Website</a>{/if}
          </p>
        {:else}
          <p class="meta">{p.details_status === 'unavailable' ? 'TMDB didn’t answer, so the profile and full filmography are missing. Try again later.' : 'Showing the titles metareel has credits for; a TMDB key adds the profile and full filmography.'}</p>
        {/if}
      </div>
    </header>

    <section class="kpis" aria-label="In numbers">
      <div class="kpi">
        <span class="k-label">Credits</span>
        <b class="num">{released}</b>
        <span class="k-sub">{#if next}Next: {next.title}, {next.year ?? 'TBA'}{:else if films.length > released}+{films.length - released} not out yet{:else}{depts.length} {depts.length === 1 ? 'department' : 'departments'}{/if}</span>
      </div>
      <div class="kpi">
        <span class="k-label">On the charts</span>
        <b class="num">{charted.length}</b>
        <span class="k-sub">{charted.length ? `${compact(p.points)} Top 10 points` : 'No title of theirs has charted'}</span>
      </div>
      <div class="kpi yours">
        <span class="k-label">You’ve seen</span>
        <b class="num">{watched.length}</b>
        <span class="k-sub">{#if avg}Your average {stars(Math.round(avg))}{:else}{released ? `of ${released} released` : 'Nothing out yet'}{/if}</span>
      </div>
      <div class="kpi yours">
        <span class="k-label">On your watchlist</span>
        <b class="num">{onList}</b>
        <span class="k-sub">{onList ? 'Waiting for you' : 'Bookmark one below'}</span>
      </div>
    </section>

    {#if charted.length}
      <section class="panel">
        <div class="panel-h"><h2>On the charts</h2><span class="hint">Their titles in streaming Top 10s · a No. 1 day scores 10 points, a No. 10 day 1</span></div>
        <table class="tbl">
          <thead><tr><th scope="col">Title</th><th scope="col" class="hide-s">Role</th><th scope="col" class="right">Points</th><th scope="col" class="right hide-s">Days</th><th scope="col" class="right">Best</th><th scope="col" class="right hide-s">Countries</th></tr></thead>
          <tbody>
            {#each charted.slice(0, 8) as t (t.kind + t.tmdb_id)}
              <tr>
                <td><a class="trow" href="#/title/{t.kind}/{t.tmdb_id}"><Poster path={t.poster_path} seed={t.tmdb_id} title={t.title} width={28} radius={4} size="w92" /><span><b>{t.title}</b>{#if t.year}<span class="muted"> · {t.year}</span>{/if}</span></a></td>
                <td class="muted hide-s role">{t.roles.slice(0, 2).join(' · ')}</td>
                <td class="right num"><b>{compact(t.points)}</b></td>
                <td class="right num hide-s">{t.days}</td>
                <td class="right num"><span class="rk" class:one={t.best_rank === 1}>No.&nbsp;{t.best_rank}</span></td>
                <td class="right num hide-s">{t.countries}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if charted.length > 8}<p class="hint more-c">and {charted.length - 8} more charted titles, marked in the filmography below.</p>{/if}
      </section>
    {/if}

    <section class="films" aria-labelledby="films-h">
      <div class="films-h">
        <h2 id="films-h">Filmography</h2>
        <span class="hint">{list.length} {list.length === 1 ? 'title' : 'titles'}{#if !p.filmography?.length}{' '}metareel has credits for{/if}</span>
      </div>
      {#if depts.length > 1}
        <div class="tabs" role="tablist" aria-label="Department">
          {#each depts as [d, n] (d)}
            <button role="tab" aria-selected={dept === d} onclick={() => pick({ dept: d === depts[0][0] ? '' : d })}>{deptLabel(d)} <span class="num">{n}</span></button>
          {/each}
        </div>
      {/if}
      <div class="filters">
        <div class="seg" role="group" aria-label="Kind">
          <button aria-pressed={!kind} onclick={() => pick({ kind: '' })}>All</button>
          <button aria-pressed={kind === 'movie'} disabled={!kinds.movie} onclick={() => pick({ kind: 'movie' })}>Films <span class="num">{kinds.movie}</span></button>
          <button aria-pressed={kind === 'tv'} disabled={!kinds.tv} onclick={() => pick({ kind: 'tv' })}>Series <span class="num">{kinds.tv}</span></button>
        </div>
        <div class="seg" role="group" aria-label="Sort">
          {#each SORTS as [s, l]}<button aria-pressed={sort === s} onclick={() => pick({ sort: s === 'new' ? '' : s })}>{l}</button>{/each}
        </div>
        {#if selfCount}
          <label class="check"><input type="checkbox" checked={showSelf} onchange={(e) => pick({ self: e.currentTarget.checked ? '1' : '' })} /> Appearances as themselves <span class="hint num">{selfCount}</span></label>
        {/if}
      </div>

      {#if soon.length}
        <div class="soon">
          <h3>Coming up <span class="hint num">{soon.length}</span></h3>
          <ol>
            {#each soon.slice(0, 8) as f (f.kind + f.tmdb_id)}
              {@const t = library.get(f.kind, f.tmdb_id)}
              <li>
                <span class="when num">{f.date ? shortDate(f.date) : 'Date TBA'}</span>
                <a class="name" href="#/title/{f.kind}/{f.tmdb_id}">{f.title}</a>
                <span class="muted s-role">{f.kind === 'tv' ? 'Series' : 'Film'}{#if roleLine(f)}{' · '}{roleLine(f)}{/if}</span>
                <button class="mark" aria-pressed={t?.status === 'watchlist'} aria-label="{t?.status === 'watchlist' ? 'Remove from' : 'Add to'} watchlist: {f.title}" title={t?.status === 'watchlist' ? 'On your watchlist' : 'Add to watchlist'} onclick={() => toggle(f)}>
                  <Icon name={t?.status === 'watchlist' ? 'bookmark-fill' : 'bookmark'} size={15} />
                </button>
              </li>
            {/each}
          </ol>
          {#if soon.length > 8}<p class="hint">and {soon.length - 8} more announced.</p>{/if}
        </div>
      {/if}
      {#if grid.length}
        <ul class="grid">
          {#each grid.slice(0, shown) as f (f.kind + f.tmdb_id)}
            {@const t = library.get(f.kind, f.tmdb_id)}
            {@const c = chartOf.get(`${f.kind}:${f.tmdb_id}`)}
            <li class="tile">
              <a class="art" href="#/title/{f.kind}/{f.tmdb_id}">
                <Poster path={f.poster_path ?? null} seed={f.tmdb_id} title={f.title} width={160} radius={10} size="w342" />
                {#if upcoming(f)}<span class="badge soon">{f.date ? shortDate(f.date) : 'Announced'}</span>
                {:else if c?.days}<span class="badge chart" title="{c.days} days in Top 10s, best No. {c.best_rank}">Top 10 · No. {c.best_rank}</span>{/if}
              </a>
              <div class="tt">
                <a class="name" href="#/title/{f.kind}/{f.tmdb_id}">{f.title}</a>
                <button class="mark" aria-pressed={t?.status === 'watchlist'} aria-label="{t?.status === 'watchlist' ? 'Remove from' : 'Add to'} watchlist: {f.title}" title={t?.status === 'watchlist' ? 'On your watchlist' : 'Add to watchlist'} onclick={() => toggle(f)}>
                  <Icon name={t?.status === 'watchlist' ? 'bookmark-fill' : 'bookmark'} size={15} />
                </button>
              </div>
              <p class="meta">{f.year ?? 'TBA'} · {f.kind === 'tv' ? 'Series' : 'Film'}{#if rated(f)}{' · '}<span title="TMDB {f.vote_average.toFixed(1)} from {f.vote_count.toLocaleString()} votes">★ {f.vote_average.toFixed(1)}</span>{/if}</p>
              {#if roleLine(f)}<p class="role">{roleLine(f)}</p>{/if}
              {#if seenIt(f)}<p class="seen"><Icon name="check" size={12} />Seen{#if t?.rating}{' · '}{stars(t.rating)}{/if}</p>{/if}
            </li>
          {/each}
        </ul>
        {#if grid.length > shown}
          <button class="btn-secondary more-btn" onclick={() => (shown += 30)}>Show more <span class="hint">{grid.length - shown} left</span></button>
        {/if}
      {:else if !soon.length}
        <p class="hint">No credits here{#if selfCount && !showSelf}; try including appearances as themselves{/if}.</p>
      {/if}
      {#if p.filmography?.length}<p class="source">Profile and credits: TMDB. Chart record: metareel.</p>{/if}
    </section>
  </article>
{/if}

<style>
  .page { display: grid; gap: 18px; padding: 20px var(--gutter) 40px; }
  .head { display: flex; gap: 32px; align-items: flex-start; padding: 28px; border-radius: var(--radius-hero); background: linear-gradient(100deg, color-mix(in srgb, var(--accent-yours) 16%, transparent) 0%, transparent 70%), var(--bg-surface); }
  .face { flex: none; width: 168px; border-radius: 14px; overflow: hidden; box-shadow: var(--shadow-art); }
  .face img { display: block; width: 100%; aspect-ratio: 2 / 3; object-fit: cover; }
  .face :global(.poster) { width: 100% !important; }
  .copy { min-width: 0; display: grid; gap: 8px; align-content: start; }
  .kicker { margin: 0; color: var(--accent-yours); font-size: 13px; font-weight: 700; letter-spacing: 0.02em; }
  h1 { margin: 0; font: 700 44px/1.05 var(--font-display); letter-spacing: -0.02em; }
  .meta { margin: 0; color: var(--text-muted); font-size: 14.5px; }
  .bio { margin: 4px 0 0; max-width: 72ch; font: 400 16.5px/25px var(--font-display); white-space: pre-line; }
  .bio.clamped { display: -webkit-box; -webkit-line-clamp: 5; line-clamp: 5; -webkit-box-orient: vertical; overflow: hidden; }
  .link { justify-self: start; padding: 0; border: 0; background: none; color: var(--accent-yours); font: 600 14px/1 var(--font-sans); cursor: pointer; }
  .links { display: flex; gap: 14px; margin: 6px 0 0; font-size: 14px; font-weight: 600; }
  .links a { color: var(--text-muted); text-underline-offset: 3px; }
  .links a:hover { color: var(--text-primary); }

  .kpis { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
  .kpi { display: grid; gap: 4px; padding: 16px 18px; border-radius: 16px; background: var(--bg-surface); min-width: 0; }
  .kpi.yours b { color: var(--accent-yours); }
  .k-label { color: var(--text-muted); font-size: 13px; font-weight: 600; }
  .kpi b { font: 700 30px/1.05 var(--font-sans); letter-spacing: -0.03em; }
  .k-sub { color: var(--text-muted); font-size: 12.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .trow { display: flex; align-items: center; gap: 10px; padding: 4px 0; color: inherit; text-decoration: none; }
  .trow:hover b { text-decoration: underline; text-underline-offset: 3px; }
  .role { max-width: 260px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .rk { font-weight: 700; }
  .rk.one { color: var(--delta-up); }
  .more-c { margin: 6px 12px 2px; }

  .films { display: grid; gap: 14px; min-width: 0; }
  .films-h { display: flex; align-items: baseline; gap: 12px; }
  .films-h h2 { margin: 0; font: 700 26px/1.2 var(--font-display); }
  .tabs { display: flex; gap: 4px; overflow-x: auto; scrollbar-width: none; border-bottom: 1px solid var(--border-subtle); }
  .tabs button { flex: none; padding: 9px 12px; border: 0; border-bottom: 2px solid transparent; background: none; color: var(--text-muted); font: 600 14.5px/1 var(--font-sans); cursor: pointer; }
  .tabs button span { margin-left: 4px; font-weight: 400; }
  .tabs button[aria-selected='true'] { color: var(--text-primary); border-bottom-color: var(--accent-yours); }
  .filters { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 14px; }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { height: 30px; padding: 0 12px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .seg button span { font-weight: 400; }
  .seg button:disabled { opacity: 0.45; cursor: default; }
  .seg button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }
  .check { display: inline-flex; align-items: center; gap: 8px; font-size: 14px; cursor: pointer; }
  .check input { width: 16px; height: 16px; accent-color: var(--accent-yours); }

  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 22px 16px; margin: 0; padding: 0; list-style: none; }
  .tile { display: grid; gap: 4px; align-content: start; min-width: 0; }
  .art { position: relative; display: block; transition: transform 0.15s ease; }
  .art:hover { transform: translateY(-2px); }
  .art :global(.poster) { width: 100% !important; }
  .badge { position: absolute; left: 8px; bottom: 8px; padding: 3px 7px; border-radius: 8px; background: rgb(0 0 0 / 0.74); color: #fff; font-size: 12px; font-weight: 700; }
  .badge.chart { color: #f5c518; }
  .tt { display: flex; align-items: flex-start; justify-content: space-between; gap: 6px; margin-top: 4px; }
  .name { font: 700 15.5px/1.25 var(--font-display); color: inherit; text-decoration: none; }
  .name:hover { text-decoration: underline; text-underline-offset: 3px; }
  .mark { flex: none; display: grid; place-items: center; width: 28px; height: 28px; border: 0; border-radius: 50%; background: var(--bg-elevated); color: var(--text-muted); cursor: pointer; }
  .mark:hover { color: var(--text-primary); }
  .mark[aria-pressed='true'] { background: color-mix(in srgb, var(--accent-yours) 20%, transparent); color: var(--accent-yours); }
  .tile .meta { font-size: 12.5px; }
  .tile .role { margin: 0; max-width: none; font-size: 12.5px; line-height: 1.35; white-space: normal; display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; }
  .seen { display: inline-flex; align-items: center; gap: 4px; margin: 0; color: var(--accent-yours); font-size: 12px; font-weight: 600; }
  .more-btn { justify-self: center; }
  .soon { display: grid; gap: 6px; padding: 14px 18px; border-radius: 16px; background: var(--bg-surface); }
  .soon h3 { margin: 0 0 2px; font: 700 16px/1.25 var(--font-display); }
  .soon ol { display: grid; margin: 0; padding: 0; list-style: none; }
  .soon li { display: grid; grid-template-columns: 96px minmax(0, auto) minmax(0, 1fr) auto; align-items: center; gap: 12px; min-height: 40px; box-shadow: inset 0 1px 0 var(--border-subtle); }
  .soon li:first-child { box-shadow: none; }
  .when { color: var(--accent-yours); font-size: 13px; font-weight: 700; }
  .s-role { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; }
  .source { margin: 0; color: var(--text-muted); font-size: 12px; }

  @media (max-width: 900px) {
    .kpis { grid-template-columns: 1fr 1fr; }
    .hide-s { display: none; }
  }
  @media (max-width: 640px) {
    .head { flex-direction: column; padding: 20px; gap: 18px; }
    .face { width: 128px; }
    h1 { font-size: 34px; }
    .grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .soon li { grid-template-columns: 78px minmax(0, 1fr) auto; padding: 6px 0; }
    .s-role { display: none; }
  }
</style>
