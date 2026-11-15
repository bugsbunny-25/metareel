<!-- You (#/you): who you are, your statistics at a glance (streak, shows, episodes, films) with a
     way into the detailed statistics, this year, whether your cinema plans pay off, your genres,
     and your shows and films as posters. Settings, your year in review, the library and the
     watchlist are a click away from here. -->
<script>
  import Icon from '../components/Icon.svelte'
  import BarList from '../components/BarList.svelte'
  import PosterShelf from '../components/PosterShelf.svelte'
  import { api } from '../lib/api.js'
  import { session } from '../lib/session.svelte.js'
  import { href } from '../lib/router.svelte.js'
  import { stars } from '../lib/rows.js'
  import { shortDuration, plural } from '../lib/duration.js'
  import { formatMoney } from '../lib/money.js'

  let account = $state(null)
  let stats = $state(null)
  let savings = $state(null)
  let queue = $state(null)
  let films = $state([])
  let error = $state('')

  $effect(() => {
    const ctrl = new AbortController()
    const s = ctrl.signal
    // The rest of the page still works without these.
    const soft = (p) => p.catch(() => null)
    api.statsDetailed('', s).then((d) => (stats = d)).catch((err) => { if (err.name !== 'AbortError') error = err.message })
    soft(api.account(s)).then((a) => (account = a))
    soft(api.subscriptionSavings(s)).then((d) => (savings = d))
    soft(api.showQueue(undefined, s)).then((q) => (queue = q))
    soft(api.diary({ kind: 'movie', limit: 60 }, s)).then((r) => (films = r?.watches ?? []))
    return () => ctrl.abort()
  })

  const name = $derived(account?.user?.username ?? session.user.username)
  const since = $derived(account?.user?.created_at ? new Date(account.user.created_at).toLocaleDateString(undefined, { month: 'long', year: 'numeric' }) : '')

  const myShows = $derived(
    [...(queue?.shows ?? [])]
      .filter((x) => x.watched_episodes > 0)
      .sort((a, b) => (b.last_watched_at ?? '').localeCompare(a.last_watched_at ?? ''))
      .slice(0, 16)
      .map((x) => ({ key: x.tmdb_id, href: `#/show/${x.tmdb_id}`, poster: x.poster_path, seed: x.tmdb_id, title: x.title ?? '', percent: x.percent ?? 0, caption: `${x.watched_episodes}/${x.aired_episodes}` })),
  )
  // The latest watch of each film; a rewatch doesn't repeat a poster.
  const myFilms = $derived.by(() => {
    const seen = new Set()
    const out = []
    for (const w of films) {
      if (seen.has(w.tmdb_id)) continue
      seen.add(w.tmdb_id)
      out.push({ key: w.tmdb_id, href: `#/title/movie/${w.tmdb_id}`, poster: w.poster_path, seed: w.tmdb_id, title: w.title ?? '', caption: stars(w.rating) || null })
      if (out.length === 16) break
    }
    return out
  })
  const plans = $derived((savings?.subscriptions ?? []).filter((p) => p.active))
  const s = $derived(stats)
  const statsHref = (kind) => href('/you/stats', { kind })
</script>

<div class="page">
  <header class="profile">
    <span class="avatar" aria-hidden="true">{name.slice(0, 1).toUpperCase()}</span>
    <div class="who">
      <h1>{name}</h1>
      <p class="hint">{since ? `Watching since ${since}` : 'Your watching, in numbers'}</p>
    </div>
    <nav class="links" aria-label="Your pages">
      <a class="btn-secondary" href="#/library">Library</a>
      <a class="btn-secondary" href="#/watchlist">Watchlist</a>
      <a class="btn-secondary" href="#/stats">Year in review</a>
      <a class="btn-secondary gear" href="#/settings"><Icon name="gear" size={16} />Settings</a>
    </nav>
  </header>

  {#if error}<p class="alert" role="alert">Couldn’t load your statistics: {error}</p>{/if}

  <section aria-labelledby="st-h">
    <a class="sh" href={statsHref('')}><h2 id="st-h">Statistics</h2><Icon name="chevron" size={20} /></a>
    {#if !s && !error}
      <div class="skeleton" style="height:104px" aria-busy="true" aria-label="Loading"></div>
    {:else if s}
      <div class="tiles">
        <a class="tile" href={statsHref('')}><b class="num"><span class="flame"><Icon name="flame" size={24} /></span>{s.streak?.current ?? 0}</b><span>Day streak · best {s.streak?.longest ?? 0}</span></a>
        <a class="tile" href={statsHref('tv')}><b class="num">{(s.shows ?? 0).toLocaleString()}</b><span>Shows</span></a>
        <a class="tile" href={statsHref('tv')}><b class="num">{(s.episodes ?? 0).toLocaleString()}</b><span>Watched episodes</span></a>
        <a class="tile" href={statsHref('movie')}><b class="num">{(s.films ?? 0).toLocaleString()}</b><span>Movies</span></a>
        <a class="tile" href={statsHref('')}><b class="num">{shortDuration(s.minutes ?? 0)}</b><span>Time watching</span></a>
      </div>
    {/if}
  </section>

  {#if s}
    <section aria-labelledby="yr-h">
      <a class="sh" href="#/stats"><h2 id="yr-h">This year</h2><Icon name="chevron" size={20} /></a>
      <div class="tiles three">
        <div class="tile"><b class="num">{(s.this_year_episodes ?? 0).toLocaleString()}</b><span>Episodes</span></div>
        <div class="tile"><b class="num">{(s.this_year_movies ?? 0).toLocaleString()}</b><span>Movies</span></div>
        <div class="tile"><b class="num">{shortDuration(s.this_year?.minutes ?? 0)}</b><span>Watch time</span></div>
      </div>
    </section>
  {/if}

  <div class="two">
    {#if plans.length}
      <section aria-labelledby="pl-h">
        <a class="sh" href={statsHref('movie')}><h2 id="pl-h">Cinema plans</h2><Icon name="chevron" size={20} /></a>
        <div class="plans">
          {#each plans as p (p.id)}
            {@const month = p.months?.at(-1)}
            {@const need = p.break_even_watches ?? 0}
            {@const done = month?.watches ?? 0}
            {@const saved = p.saved_minor ?? 0}
            <article class="panel pad plan">
              <div class="ph"><h3>{p.plan}</h3><span class="hint">{formatMoney(p.price_minor, p.currency)} / {p.period}</span></div>
              {#if need}
                <p class="be"><b class="num">{done} of {need}</b> films to break even this {p.period}{#if done >= need}<span class="paid"> · paid off</span>{/if}</p>
                <span class="bar" role="progressbar" aria-valuemin="0" aria-valuemax={need} aria-valuenow={Math.min(done, need)} aria-label="Films this {p.period}"><i style:width="{Math.min(100, (done / need) * 100)}%"></i></span>
              {/if}
              <p class="hint">{saved >= 0 ? 'Saved' : 'Down'} <b class="num" class:up={saved >= 0} class:down={saved < 0}>{formatMoney(Math.abs(saved), p.currency)}</b> over {plural(p.watches ?? 0, 'covered film')}</p>
            </article>
          {/each}
        </div>
      </section>
    {/if}

    {#if s?.genres?.length}
      <section aria-labelledby="ge-h">
        <div class="sh"><h2 id="ge-h">Your genres</h2></div>
        <div class="panel pad"><BarList items={s.genres.slice(0, 8).map((g) => ({ label: g.label, value: g.count }))} /></div>
      </section>
    {/if}
  </div>

  {#if myShows.length}
    <section aria-labelledby="ms-h">
      <a class="sh" href="#/tv"><h2 id="ms-h">My shows</h2><Icon name="chevron" size={20} /></a>
      <PosterShelf items={myShows} label="Shows you’ve watched, latest first" />
    </section>
  {/if}

  {#if myFilms.length}
    <section aria-labelledby="mf-h">
      <a class="sh" href="#/diary?kind=movie"><h2 id="mf-h">My movies</h2><Icon name="chevron" size={20} /></a>
      <PosterShelf items={myFilms} label="Films you’ve watched, latest first" />
    </section>
  {/if}
</div>

<style>
  .page { padding: 20px var(--gutter) 48px; display: grid; gap: 26px; }
  .profile { display: flex; flex-wrap: wrap; align-items: center; gap: 14px 20px; }
  .avatar { display: grid; place-items: center; width: 76px; height: 76px; border-radius: 50%; border: 2px solid var(--accent-yours); background: var(--bg-elevated); font: 700 34px/1 var(--font-display); }
  .who { flex: 1; min-width: 180px; }
  h1 { margin: 0 0 2px; font: 700 32px/1.1 var(--font-display); letter-spacing: -0.02em; }
  .who .hint { margin: 0; font-size: 14px; }
  .links { display: flex; flex-wrap: wrap; gap: 8px; }
  .links a { text-decoration: none; }
  .gear { gap: 6px; }

  section { display: grid; gap: 12px; min-width: 0; align-content: start; }
  .sh { display: inline-flex; align-items: center; gap: 4px; justify-self: start; color: inherit; text-decoration: none; }
  .sh h2 { margin: 0; font: 700 22px/1.2 var(--font-display); }
  a.sh { color: var(--text-primary); }
  a.sh :global(svg) { color: var(--accent-yours); }
  a.sh:hover h2 { text-decoration: underline; text-underline-offset: 4px; }

  .tiles { display: grid; grid-template-columns: repeat(auto-fill, minmax(170px, 1fr)); gap: 12px; }
  .tiles.three { grid-template-columns: repeat(3, minmax(0, 220px)); }
  .tile { display: grid; gap: 4px; justify-items: center; padding: 16px; border-radius: 16px; background: var(--bg-surface); color: inherit; text-decoration: none; text-align: center; }
  a.tile:hover { background: var(--bg-elevated); }
  .tile b { display: inline-flex; align-items: center; gap: 4px; font: 700 30px/1.1 var(--font-sans); letter-spacing: -0.03em; }
  .tile span { color: var(--text-muted); font-size: 13.5px; }
  .flame { color: var(--accent-yours); display: inline-flex; }

  .two { display: grid; grid-template-columns: repeat(auto-fit, minmax(360px, 1fr)); gap: 26px; }
  .plans { display: grid; gap: 12px; }
  .plan { gap: 10px; }
  .ph { display: flex; align-items: baseline; gap: 10px; }
  .ph h3 { flex: 1; margin: 0; font: 700 17px/1.25 var(--font-display); }
  .plan p { margin: 0; }
  .be b { font-weight: 700; }
  .paid, .up { color: var(--delta-up); }
  .down { color: var(--delta-down); }
  .bar { display: block; height: 8px; border-radius: 4px; background: var(--bg-elevated); overflow: hidden; }
  .bar i { display: block; height: 100%; border-radius: 4px; background: var(--accent-yours); }

  @media (max-width: 600px) {
    .tiles { grid-template-columns: 1fr 1fr; }
    .tiles.three { grid-template-columns: repeat(3, minmax(0, 1fr)); }
    .tile b { font-size: 24px; }
    .two { grid-template-columns: 1fr; }
  }
</style>
