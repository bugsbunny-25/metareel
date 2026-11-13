<!-- Up next. Shows by stage (continue watching → haven't watched in a while → not started →
     on hold → dropped) from /me/shows, then an Upcoming timeline of episodes still to air from
     /me/upcoming. Movies (?tab=movies) come from the library map: released ones still to watch,
     and ones not out yet. -->
<script>
  import { untrack } from 'svelte'
  import Poster from '../components/Poster.svelte'
  import Icon from '../components/Icon.svelte'
  import Columns from '../components/Columns.svelte'
  import TitleMenu from '../components/TitleMenu.svelte'
  import { api } from '../lib/api.js'
  import { library } from '../lib/library.svelte.js'
  import { route, setQuery } from '../lib/router.svelte.js'
  import { toast } from '../lib/toast.svelte.js'
  import { ago, sinceDay, daysUntil, dayLabel, weekdayLabel, localDay, addDays, hm } from '../lib/dates.js'
  import { markWatched, epCode, EPISODE_TYPE } from '../lib/shows.js'

  let { onlog } = $props()

  const tab = $derived(route.query.tab === 'movies' ? 'movies' : 'shows')
  const WINDOWS = [[30, '30 days'], [90, '90 days'], [365, 'A year']]
  const days = $derived(WINDOWS.some(([d]) => d === +route.query.days) ? +route.query.days : 90)

  // Stages in the order they're shown. Caught-up shows have no section: they surface in
  // Upcoming when a new episode is on its way, and back in Continue watching once it airs.
  const STAGES = [
    ['watching', 'Continue watching', 'Watched in the last 30 days'],
    ['stalled', 'Haven’t watched in a while', 'Nothing watched for more than 30 days'],
    ['not_started', 'Not started', 'On your watchlist'],
    ['on_hold', 'On hold', 'Paused; left out of Upcoming'],
    ['dropped', 'Dropped', 'Stopped watching; left out of Upcoming'],
  ]

  let queue = $state(null)
  let upcoming = $state(null)
  let error = $state('')
  let nonce = $state(0)
  let busy = $state(new Set())
  // The library status each show had when the queue was read. A status changed from a
  // title menu (or anywhere else) shows up as a difference, and the queue is re-read.
  let seen = new Map()

  $effect(() => {
    nonce
    const ctrl = new AbortController()
    api.showQueue(undefined, ctrl.signal)
      .then((q) => {
        seen = new Map(q.shows.map((s) => [s.tmdb_id, library.get('tv', s.tmdb_id)?.status ?? null]))
        queue = q
        error = ''
      })
      .catch((err) => { if (err.name !== 'AbortError') error = err.message })
    return () => ctrl.abort()
  })
  $effect(() => {
    nonce
    const d = days
    const ctrl = new AbortController()
    api.upcoming(d, ctrl.signal)
      .then((u) => (upcoming = u))
      .catch((err) => { if (err.name !== 'AbortError') error = err.message })
    return () => ctrl.abort()
  })
  $effect(() => {
    if (!queue || !library.loaded) return
    for (const [id, status] of seen) {
      if ((library.get('tv', id)?.status ?? null) !== status) {
        // Once per change: forget the snapshot until the new queue arrives, and bump nonce
        // without reading it here (that would make this effect re-run itself).
        seen = new Map()
        untrack(() => nonce++)
        return
      }
    }
  })
  const reload = () => nonce++

  const byStage = $derived.by(() => {
    const m = Object.fromEntries(STAGES.map(([k]) => [k, []]))
    for (const s of queue?.shows ?? []) m[s.stage]?.push(s)
    return m
  })
  const count = (k) => queue?.counts?.[k] ?? 0
  const inProgress = $derived([...byStage.watching, ...byStage.stalled])
  const toCatchUp = $derived(inProgress.reduce((n, s) => n + s.remaining_episodes, 0))
  const minutesLeft = $derived(inProgress.reduce((n, s) => n + s.minutes_left, 0))

  const today = localDay()
  const soon = $derived((upcoming?.episodes ?? []).filter((e) => daysUntil(e.air_date, today) <= 7))

  // Upcoming, grouped by day, with a binge drop of one show folded into one line.
  const timeline = $derived.by(() => {
    const days = []
    for (const e of upcoming?.episodes ?? []) {
      let day = days.at(-1)
      if (day?.date !== e.air_date) days.push((day = { date: e.air_date, items: [] }))
      const last = day.items.at(-1)
      if (last && last.tmdb_id === e.tmdb_id && last.season_number === e.season_number && last.to + 1 === e.episode_number) {
        last.to = e.episode_number
        last.count++
        last.finale ||= e.episode_type === 'finale'
      } else {
        day.items.push({ ...e, from: e.episode_number, to: e.episode_number, count: 1, finale: e.episode_type === 'finale' })
      }
    }
    return days
  })
  // Episodes per week across the window, for the strip above the timeline.
  const weeks = $derived.by(() => {
    const n = Math.ceil(days / 7)
    const cols = Array.from({ length: n }, (_, i) => {
      const start = addDays(today, i * 7 + 1)
      return { label: dayLabel(start), value: 0, title: '' }
    })
    for (const e of upcoming?.episodes ?? []) {
      const i = Math.floor((daysUntil(e.air_date, today) - 1) / 7)
      if (cols[i]) cols[i].value++
    }
    for (const c of cols) c.title = `Week of ${c.label}: ${c.value} episode${c.value === 1 ? '' : 's'}`
    return cols
  })
  const whenLabel = (date) => {
    const n = daysUntil(date, today)
    return n === 1 ? 'Tomorrow' : `${weekdayLabel(date)} ${dayLabel(date)}`
  }
  const inDays = (date) => {
    const n = daysUntil(date, today)
    return n <= 1 ? '' : n < 14 ? `in ${n} days` : ago(date)
  }

  async function watched(s) {
    const ep = s.next_episode
    busy = new Set(busy).add(s.tmdb_id)
    try {
      await markWatched(s.tmdb_id, { season: ep.season_number, episode: ep.episode_number, precision: 'minute' })
      toast(`${s.title}: ${epCode(ep.season_number, ep.episode_number)} watched.`)
      reload()
    } catch (err) {
      toast(err.message, 'error')
    } finally {
      busy.delete(s.tmdb_id)
      busy = new Set(busy)
    }
  }
  function logIt(s) {
    const ep = s.next_episode
    onlog({ kind: 'tv', id: s.tmdb_id, name: s.title, poster: s.poster_path, season: ep?.season_number ?? 1, episode: ep?.episode_number ?? 1, onsaved: reload })
  }
  async function resume(s) {
    try {
      await library.setState('tv', s.tmdb_id, { status: 'watching' })
      toast(`${s.title} is back in your queue.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }

  // Movies, straight from the library map.
  const movies = $derived.by(() => {
    const all = [...library.byKey.values()].filter((t) => t.tmdb_kind === 'movie')
    const out = (t) => !t.release_date || t.release_date <= today
    const toWatch = all
      .filter((t) => (t.status === 'watchlist' || t.status === 'watching') && !t.watch_count && out(t))
      .sort((a, b) => (a.watchlist_position ?? 1e9) - (b.watchlist_position ?? 1e9) || (b.created_at ?? '').localeCompare(a.created_at ?? ''))
    const comingSoon = all
      .filter((t) => !out(t) && t.status !== 'dropped' && t.status !== 'not_interested')
      .sort((a, b) => a.release_date.localeCompare(b.release_date))
    return { toWatch, comingSoon, minutes: toWatch.reduce((n, t) => n + (t.runtime || 0), 0) }
  })
  const year = (d) => d?.slice(0, 4) ?? ''
</script>

<div class="page">
  <header class="head">
    <h1>Up next</h1>
    <div class="seg" role="group" aria-label="Kind">
      <button aria-pressed={tab === 'shows'} onclick={() => setQuery({ tab: '' })}>Shows</button>
      <button aria-pressed={tab === 'movies'} onclick={() => setQuery({ tab: 'movies' })}>Movies</button>
    </div>
  </header>

  {#if error}<p class="alert" role="alert">Couldn’t load your queue: {error}</p>{/if}

  {#if tab === 'shows'}
    {#if !queue}
      <div class="skeleton" style="height:420px" aria-busy="true" aria-label="Loading"></div>
    {:else if !queue.shows.length}
      <section class="empty">
        <h2>No shows yet</h2>
        <p>Add a show to your watchlist or log an episode, and it lands here with its next episode. Press <kbd>/</kbd> to search.</p>
      </section>
    {:else}
      <section class="kpis" aria-label="Shows at a glance">
        <div class="kpi"><span class="k-label">To catch up</span><b class="num">{toCatchUp}</b><span class="k-sub">episode{toCatchUp === 1 ? '' : 's'} · about {hm(minutesLeft)}</span></div>
        <div class="kpi"><span class="k-label">In progress</span><b class="num">{inProgress.length}</b><span class="k-sub">{count('stalled')} not watched for 30+ days</span></div>
        <div class="kpi"><span class="k-label">Airing this week</span><b class="num">{upcoming ? soon.length : '–'}</b><span class="k-sub">{#if soon.length}next: {soon[0].show_title}, {whenLabel(soon[0].air_date)}{:else}nothing in the next 7 days{/if}</span></div>
        <div class="kpi"><span class="k-label">Caught up</span><b class="num">{count('caught_up')}</b><span class="k-sub">{(queue.shows ?? []).filter((s) => s.stage === 'caught_up' && s.next_air_date).length} with episodes on the way</span></div>
      </section>

      {#each STAGES as [key, name, hint]}
        {@const list = byStage[key]}
        {#if list.length}
          <section class="stage" aria-labelledby="st-{key}">
            <div class="sh"><h2 id="st-{key}">{name}</h2><span class="n num">{list.length}</span><span class="hint">{hint}</span></div>
            {#if key === 'watching' || key === 'stalled'}
              <div class="cards">
                {#each list as s (s.tmdb_id)}
                  {@const ep = s.next_episode}
                  {@const t = library.get('tv', s.tmdb_id)}
                  <article class="card" class:stalled={key === 'stalled'}>
                    <a class="art" href="#/show/{s.tmdb_id}" tabindex="-1" aria-hidden="true"><Poster path={s.poster_path} seed={s.tmdb_id} title={s.title} width={84} radius={8} size="w185" /></a>
                    <div class="body">
                      <div class="top">
                        <a class="name" href="#/show/{s.tmdb_id}">{s.title}</a>
                        {#if t}<TitleMenu {t} {onlog} />{/if}
                      </div>
                      <p class="ep"><span class="code num">{epCode(ep.season_number, ep.episode_number)}</span>{' '}{ep.name || 'Untitled episode'}</p>
                      <p class="meta">
                        {#if ep.runtime}{ep.runtime} min · {/if}aired {ep.air_date ? ago(ep.air_date) : 'on an unknown date'}
                        {#if EPISODE_TYPE[ep.episode_type]}{' '}<span class="badge">{EPISODE_TYPE[ep.episode_type]}</span>{/if}
                      </p>
                      <div class="prog" title="{s.watched_episodes} of {s.aired_episodes} aired episodes watched">
                        <span class="bar" aria-hidden="true"><i style:width="{s.percent}%"></i></span>
                        <span class="num">{s.watched_episodes}/{s.aired_episodes}</span>
                      </div>
                      <p class="foot">
                        {s.remaining_episodes} left{#if s.minutes_left} · {hm(s.minutes_left)}{/if}
                        {#if s.last_watched_at}{' · '}<span class:stale={key === 'stalled'}>last watched {sinceDay(s.last_watched_at)}</span>{/if}
                      </p>
                      <div class="actions">
                        <button class="btn-primary sm" disabled={busy.has(s.tmdb_id)} onclick={() => watched(s)} title="Mark {epCode(ep.season_number, ep.episode_number)} watched, just now">
                          <Icon name="check" size={15} />Watched
                        </button>
                        <button class="btn-secondary sm" onclick={() => logIt(s)}>Log…</button>
                        <a class="more-link" href="#/show/{s.tmdb_id}">Episodes</a>
                      </div>
                    </div>
                  </article>
                {/each}
              </div>
            {:else if key === 'not_started'}
              <div class="tiles">
                {#each list as s (s.tmdb_id)}
                  {@const t = library.get('tv', s.tmdb_id)}
                  <article class="tile">
                    <a class="art" href="#/show/{s.tmdb_id}"><Poster path={s.poster_path} seed={s.tmdb_id} title={s.title} width={150} radius={10} size="w342" /></a>
                    <div class="top">
                      <a class="name" href="#/show/{s.tmdb_id}">{s.title}</a>
                      {#if t}<TitleMenu {t} {onlog} />{/if}
                    </div>
                    <p class="meta">
                      {#if s.aired_episodes}
                        {s.aired_episodes} episode{s.aired_episodes === 1 ? '' : 's'} out{#if s.minutes_left} · {hm(s.minutes_left)}{/if}
                      {:else if s.next_air_date}
                        <span class="new">Premieres {dayLabel(s.next_air_date)}</span>
                      {:else}
                        No air dates yet
                      {/if}
                    </p>
                    {#if s.aired_episodes && s.next_air_date}<p class="meta"><span class="new">New episode {dayLabel(s.next_air_date)}</span></p>{/if}
                    {#if s.next_episode}
                      <button class="btn-secondary sm" disabled={busy.has(s.tmdb_id)} onclick={() => watched(s)}>
                        <Icon name="check" size={14} />Watched {epCode(s.next_episode.season_number, s.next_episode.episode_number)}
                      </button>
                    {/if}
                  </article>
                {/each}
              </div>
            {:else}
              <ul class="rows">
                {#each list as s (s.tmdb_id)}
                  {@const t = library.get('tv', s.tmdb_id)}
                  <li>
                    <a class="art" href="#/show/{s.tmdb_id}" tabindex="-1" aria-hidden="true"><Poster path={s.poster_path} seed={s.tmdb_id} title={s.title} width={44} radius={5} size="w92" /></a>
                    <div class="rbody">
                      <a class="name" href="#/show/{s.tmdb_id}">{s.title}</a>
                      <span class="hint">
                        {#if s.watched_episodes}{s.watched_episodes} of {s.aired_episodes} watched{#if s.next_episode}{' · '}next {epCode(s.next_episode.season_number, s.next_episode.episode_number)}{/if}{:else}Not started{/if}
                        {#if s.last_watched_at}{' · '}last watched {sinceDay(s.last_watched_at)}{/if}
                      </span>
                    </div>
                    <span class="bar slim" aria-hidden="true"><i style:width="{s.percent}%"></i></span>
                    <button class="btn-secondary sm" onclick={() => resume(s)}>Resume</button>
                    {#if t}<TitleMenu {t} {onlog} />{/if}
                  </li>
                {/each}
              </ul>
            {/if}
          </section>
        {/if}
      {/each}
    {/if}

    <section class="panel pad upcoming" aria-labelledby="up-h">
      <div class="uh">
        <h2 id="up-h">Upcoming</h2>
        <span class="hint">Episodes still to air of the shows you follow; on hold and dropped are left out</span>
        <div class="seg" role="group" aria-label="How far ahead">
          {#each WINDOWS as [d, label]}
            <button aria-pressed={days === d} onclick={() => setQuery({ days: d === 90 ? '' : d }, { replace: true })}>{label}</button>
          {/each}
        </div>
      </div>
      {#if !upcoming}
        <div class="skeleton" style="height:160px" aria-busy="true" aria-label="Loading"></div>
      {:else if !upcoming.episodes.length}
        <p class="hint">Nothing announced in the next {days === 365 ? 'year' : `${days} days`}. TMDB adds air dates as networks announce them.</p>
      {:else}
        <Columns items={weeks} height={70} every={days > 90 ? 8 : days > 30 ? 2 : 1} label="Episodes airing per week, next {days} days" />
        <ol class="timeline">
          {#each timeline as day (day.date)}
            <li class="day">
              <div class="date">
                <b>{whenLabel(day.date)}</b>
                <span class="hint">{inDays(day.date)}</span>
              </div>
              <ul class="eps">
                {#each day.items as e (e.tmdb_id + ':' + e.season_number + ':' + e.from)}
                  <li>
                    <a class="art" href="#/show/{e.tmdb_id}" tabindex="-1" aria-hidden="true"><Poster path={e.poster_path || null} seed={e.tmdb_id} title={e.show_title} width={36} radius={4} size="w92" /></a>
                    <div class="rbody">
                      <a class="name" href="#/show/{e.tmdb_id}">{e.show_title}</a>
                      <span class="hint">
                        <span class="code num">{e.count > 1 ? `S${e.season_number} E${e.from}–${e.to}` : epCode(e.season_number, e.from)}</span>
                        {' '}{e.count > 1 ? `${e.count} episodes` : e.name}
                      </span>
                    </div>
                    {#if e.from === 1}<span class="badge new">{e.season_number === 1 ? 'Series premiere' : 'Season premiere'}</span>{/if}
                    {#if e.finale}<span class="badge">Finale</span>{/if}
                  </li>
                {/each}
              </ul>
            </li>
          {/each}
        </ol>
        {#if upcoming.truncated}<p class="hint">Showing the first {upcoming.episodes.length} episodes; pick a shorter window to see them all.</p>{/if}
      {/if}
    </section>
  {:else}
    <section class="kpis two-k" aria-label="Movies at a glance">
      <div class="kpi"><span class="k-label">To watch</span><b class="num">{movies.toWatch.length}</b><span class="k-sub">{movies.minutes ? `about ${hm(movies.minutes)}` : 'out now, not watched yet'}</span></div>
      <div class="kpi"><span class="k-label">Coming soon</span><b class="num">{movies.comingSoon.length}</b><span class="k-sub">{#if movies.comingSoon.length}next: {movies.comingSoon[0].title}, {dayLabel(movies.comingSoon[0].release_date)}{:else}nothing announced{/if}</span></div>
    </section>

    <section class="stage" aria-labelledby="m-watch">
      <div class="sh"><h2 id="m-watch">To watch</h2><span class="n num">{movies.toWatch.length}</span><span class="hint">Out now, on your watchlist, not watched yet</span></div>
      {#if movies.toWatch.length}
        <div class="tiles">
          {#each movies.toWatch as t (t.tmdb_id)}
            <article class="tile">
              <a class="art" href="#/title/movie/{t.tmdb_id}"><Poster path={t.poster_path} seed={t.tmdb_id} title={t.title ?? ''} width={150} radius={10} size="w342" /></a>
              <div class="top">
                <a class="name" href="#/title/movie/{t.tmdb_id}">{t.title}</a>
                <TitleMenu {t} {onlog} />
              </div>
              <p class="meta">{year(t.release_date)}{#if t.runtime}{' · '}{hm(t.runtime)}{/if}</p>
              <button class="btn-secondary sm" onclick={() => onlog({ kind: 'movie', id: t.tmdb_id, name: t.title, poster: t.poster_path })}>
                <Icon name="plus" size={14} />Log a watch
              </button>
            </article>
          {/each}
        </div>
      {:else}
        <p class="hint">Nothing waiting. Movies you add to your watchlist show up here once they’re out.</p>
      {/if}
    </section>

    <section class="panel pad upcoming" aria-labelledby="m-soon">
      <div class="uh"><h2 id="m-soon">Coming soon</h2><span class="hint">Movies in your library that aren’t out yet, by first release date</span></div>
      {#if movies.comingSoon.length}
        <ol class="timeline">
          {#each movies.comingSoon as t (t.tmdb_id)}
            <li class="day">
              <div class="date"><b>{dayLabel(t.release_date)} {year(t.release_date)}</b><span class="hint">{inDays(t.release_date) || whenLabel(t.release_date)}</span></div>
              <ul class="eps">
                <li>
                  <a class="art" href="#/title/movie/{t.tmdb_id}" tabindex="-1" aria-hidden="true"><Poster path={t.poster_path} seed={t.tmdb_id} title={t.title ?? ''} width={36} radius={4} size="w92" /></a>
                  <div class="rbody"><a class="name" href="#/title/movie/{t.tmdb_id}">{t.title}</a></div>
                  <TitleMenu {t} {onlog} />
                </li>
              </ul>
            </li>
          {/each}
        </ol>
      {:else}
        <p class="hint">No unreleased movies in your library.</p>
      {/if}
    </section>
  {/if}
</div>

<style>
  .page { padding: 20px var(--gutter) 48px; display: grid; gap: 22px; }
  .head { display: flex; align-items: center; gap: 18px; }
  h1 { margin: 0; font: 700 32px/1.1 var(--font-display); letter-spacing: -0.02em; }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { height: 30px; padding: 0 14px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .seg button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }

  .kpis { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; }
  .kpis.two-k { grid-template-columns: repeat(2, minmax(0, 320px)); }
  .kpi { display: grid; gap: 4px; padding: 16px 18px; border-radius: 16px; background: var(--bg-surface); min-width: 0; }
  .k-label { color: var(--text-muted); font-size: 13px; font-weight: 600; }
  .kpi b { font: 700 32px/1.05 var(--font-sans); letter-spacing: -0.03em; }
  .k-sub { color: var(--text-muted); font-size: 12.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .stage { display: grid; gap: 12px; }
  .sh { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 10px; }
  .sh h2 { margin: 0; font: 700 22px/1.2 var(--font-display); }
  .n { color: var(--text-muted); font-weight: 700; }

  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(340px, 1fr)); gap: 12px; }
  .card { display: grid; grid-template-columns: 84px 1fr; gap: 16px; padding: 14px; border-radius: 16px; background: var(--bg-surface); }
  .body { display: grid; gap: 5px; align-content: start; min-width: 0; }
  .top { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; min-width: 0; }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 700 17px/1.25 var(--font-display); color: inherit; text-decoration: none; }
  .name:hover { text-decoration: underline; text-underline-offset: 3px; }
  .ep { margin: 0; font-size: 14.5px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .code { color: var(--accent-yours); font-weight: 700; }
  .meta, .foot { margin: 0; color: var(--text-muted); font-size: 13px; }
  .stale { color: var(--status-stale); font-weight: 600; }
  .new { color: var(--delta-debut); font-weight: 600; }
  .badge { display: inline-block; padding: 1px 7px; border-radius: 9px; background: var(--bg-elevated); color: var(--text-primary); font-size: 11.5px; font-weight: 700; }
  .badge.new { color: var(--delta-debut); }
  .prog { display: flex; align-items: center; gap: 10px; font-size: 12.5px; color: var(--text-muted); }
  .bar { flex: 1; height: 6px; border-radius: 3px; background: var(--bg-elevated); overflow: hidden; }
  .bar i { display: block; height: 100%; border-radius: 3px; background: var(--accent-yours); }
  .card.stalled .bar i { background: var(--status-stale); }
  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-top: 6px; }
  .sm { height: 32px; padding: 0 14px; font-size: 13.5px; gap: 6px; }
  .more-link { color: var(--text-muted); font-size: 13.5px; font-weight: 600; text-decoration: none; padding: 0 4px; }
  .more-link:hover { color: var(--text-primary); }

  .tiles { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 18px 14px; }
  .tile { display: grid; gap: 6px; align-content: start; min-width: 0; }
  .tile .art { display: block; transition: transform 0.15s ease; }
  .tile .art:hover { transform: translateY(-2px); }
  .tile .name { font-size: 15.5px; }
  .tile .btn-secondary { justify-self: start; }

  .rows { display: grid; gap: 2px; margin: 0; padding: 6px; list-style: none; border-radius: 16px; background: var(--bg-surface); }
  .rows li { display: grid; grid-template-columns: 44px minmax(0, 1fr) 120px auto auto; align-items: center; gap: 14px; padding: 8px; border-radius: 10px; }
  .rows li:hover { background: var(--bg-elevated); }
  .rbody { display: grid; gap: 2px; min-width: 0; }
  .rbody .name { font-size: 15.5px; }
  .rbody .hint { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .bar.slim { height: 4px; }

  .upcoming { gap: 14px; }
  .uh { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 14px; }
  .uh .seg { margin-left: auto; }
  .timeline { display: grid; margin: 0; padding: 0; list-style: none; }
  .day { display: grid; grid-template-columns: 150px 1fr; gap: 16px; padding: 12px 0; border-top: 1px solid var(--border-subtle); }
  .date { display: grid; align-content: start; gap: 2px; }
  .date b { font: 700 15px/1.2 var(--font-display); }
  .eps { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 10px 18px; margin: 0; padding: 0; list-style: none; }
  .eps li { display: flex; align-items: center; gap: 12px; min-width: 0; }
  .eps .rbody { flex: 1; }

  .empty { padding: 30px 0; }
  .empty h2 { margin: 0 0 6px; font: 700 24px/1.2 var(--font-display); }
  .empty p { color: var(--text-muted); max-width: 60ch; }
  kbd { padding: 1px 6px; border-radius: 5px; background: var(--bg-elevated); font: 600 12px/1.4 var(--font-sans); }

  @media (max-width: 960px) {
    .kpis { grid-template-columns: 1fr 1fr; }
    .kpis.two-k { grid-template-columns: 1fr 1fr; }
  }
  @media (max-width: 600px) {
    .cards { grid-template-columns: 1fr; }
    .card { grid-template-columns: 64px 1fr; gap: 12px; }
    .tiles { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .rows li { grid-template-columns: 44px minmax(0, 1fr) auto auto; }
    .rows .bar.slim { display: none; }
    .day { grid-template-columns: 1fr; gap: 8px; }
    .uh .seg { margin-left: 0; }
  }
</style>
