<!-- TV (#/tv?seg=towatch|upcoming). To watch: shows by stage (continue watching → haven't
     watched in a while → not started → on hold → dropped) from /me/shows, each with its next
     episode. Upcoming: episodes still to air from /me/upcoming, by day. Both as a list or a poster
     grid; the segment and the view are remembered in this browser (as on mobile). Picking an
     episode opens the episode first; its page links to the whole series. -->
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
  import { remembered } from '../lib/remember.svelte.js'

  let { onlog } = $props()

  const savedSeg = remembered('tabs.tv.segment', 'towatch', ['towatch', 'upcoming'])
  const view = remembered('tabs.tv.view', 'list', ['list', 'grid'])
  const seg = $derived(route.query.seg === 'towatch' || route.query.seg === 'upcoming' ? route.query.seg : savedSeg.value)
  function pickSeg(v) {
    savedSeg.value = v
    setQuery({ seg: v }, { replace: true })
  }
  const epHref = (id, season, episode) => `#/episode/${id}/${season}/${episode}`
  const nextHref = (s) => (s.next_episode ? epHref(s.tmdb_id, s.next_episode.season_number, s.next_episode.episode_number) : `#/show/${s.tmdb_id}`)
  const countdown = (date) => {
    const n = daysUntil(date, today)
    return n <= 0 ? 'Today' : n === 1 ? 'Tomorrow' : `${n} days`
  }
  const WINDOWS = [[30, '30 days'], [90, '90 days'], [365, 'A year']]
  const days = $derived(WINDOWS.some(([d]) => d === +route.query.days) ? +route.query.days : 90)

  // Stages in the order they're shown. Caught-up shows have no section: they surface in
  // Upcoming when a new episode is on its way, and back in Continue watching once it airs.
  const STAGES = [
    ['watching', 'Continue watching', (n) => `Watched in the last ${n} days`],
    ['stalled', 'Haven’t watched in a while', (n) => `Nothing watched for more than ${n} days`],
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

  // The grid shows one poster per show: its next episode to air.
  const nextPerShow = $derived.by(() => {
    const seen = new Set()
    return (upcoming?.episodes ?? []).filter((e) => !seen.has(e.tmdb_id) && seen.add(e.tmdb_id))
  })
  const toWatchTiles = $derived(STAGES.flatMap(([k]) => byStage[k]).filter((s) => s.next_episode))
</script>

<div class="page">
  <header class="head">
    <h1>TV</h1>
    <div class="tabs" role="tablist" aria-label="To watch or upcoming">
      <button role="tab" aria-selected={seg === 'towatch'} onclick={() => pickSeg('towatch')}>To watch</button>
      <button role="tab" aria-selected={seg === 'upcoming'} onclick={() => pickSeg('upcoming')}>Upcoming</button>
    </div>
    <div class="seg view" role="group" aria-label="View">
      <button aria-pressed={view.value === 'list'} aria-label="List" title="List" onclick={() => (view.value = 'list')}><Icon name="list" size={16} /></button>
      <button aria-pressed={view.value === 'grid'} aria-label="Grid" title="Grid" onclick={() => (view.value = 'grid')}><Icon name="grid" size={16} /></button>
    </div>
  </header>

  {#if error}<p class="alert" role="alert">Couldn’t load your queue: {error}</p>{/if}

  {#if seg === 'towatch'}
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
        <div class="kpi"><span class="k-label">In progress</span><b class="num">{inProgress.length}</b><span class="k-sub">{count('stalled')} not watched for {queue.stale_days ?? 30}+ days</span></div>
        <div class="kpi"><span class="k-label">Airing this week</span><b class="num">{upcoming ? soon.length : '–'}</b><span class="k-sub">{#if soon.length}next: {soon[0].show_title}, {whenLabel(soon[0].air_date)}{:else}nothing in the next 7 days{/if}</span></div>
        <div class="kpi"><span class="k-label">Caught up</span><b class="num">{count('caught_up')}</b><span class="k-sub">{(queue.shows ?? []).filter((s) => s.stage === 'caught_up' && s.next_air_date).length} with episodes on the way</span></div>
      </section>

      {#if view.value === 'grid'}
        <div class="tiles">
          {#each toWatchTiles as s (s.tmdb_id)}
            {@const ep = s.next_episode}
            <a class="ptile" href={nextHref(s)} class:stalled={s.stage === 'stalled'}>
              <span class="pwrap">
                <Poster path={s.poster_path} seed={s.tmdb_id} title={s.title} width={150} radius={10} size="w342" />
                <span class="corner num">{epCode(ep.season_number, ep.episode_number)}</span>
              </span>
              <span class="bar" aria-hidden="true"><i style:width="{s.percent}%"></i></span>
              <span class="name">{s.title}</span>
              <span class="meta">{ep.name || 'Untitled episode'}</span>
            </a>
          {/each}
        </div>
      {:else}
      {#each STAGES as [key, name, hint]}
        {@const list = byStage[key]}
        {#if list.length}
          <section class="stage" aria-labelledby="st-{key}">
            <div class="sh"><h2 id="st-{key}">{name}</h2><span class="n num">{list.length}</span><span class="hint">{typeof hint === 'function' ? hint(queue.stale_days ?? 30) : hint}</span></div>
            {#if key === 'watching' || key === 'stalled'}
              <div class="cards">
                {#each list as s (s.tmdb_id)}
                  {@const ep = s.next_episode}
                  {@const t = library.get('tv', s.tmdb_id)}
                  <article class="card" class:stalled={key === 'stalled'}>
                    <a class="art" href={nextHref(s)} tabindex="-1" aria-hidden="true"><Poster path={s.poster_path} seed={s.tmdb_id} title={s.title} width={84} radius={8} size="w185" /></a>
                    <div class="body">
                      <div class="top">
                        <a class="name" href="#/show/{s.tmdb_id}">{s.title}</a>
                        {#if t}<TitleMenu {t} {onlog} />{/if}
                      </div>
                      <a class="ep" href={nextHref(s)}><span class="code num">{epCode(ep.season_number, ep.episode_number)}</span>{' '}{ep.name || 'Untitled episode'}</a>
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
                        <a class="more-link" href="#/show/{s.tmdb_id}">View whole series</a>
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
    {/if}
  {:else}
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
        {#if view.value === 'grid'}
          <div class="tiles">
            {#each nextPerShow as e (e.tmdb_id)}
              <a class="ptile" href={epHref(e.tmdb_id, e.season_number, e.episode_number)}>
                <span class="pwrap">
                  <Poster path={e.poster_path || null} seed={e.tmdb_id} title={e.show_title} width={150} radius={10} size="w342" />
                  <span class="corner right">{countdown(e.air_date)}</span>
                </span>
                <span class="name">{e.show_title}</span>
                <span class="meta"><span class="code num">{epCode(e.season_number, e.episode_number)}</span>{' '}{e.name || 'TBA'}</span>
              </a>
            {/each}
          </div>
        {:else}
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
                    <a class="art" href={epHref(e.tmdb_id, e.season_number, e.from)} tabindex="-1" aria-hidden="true"><Poster path={e.poster_path || null} seed={e.tmdb_id} title={e.show_title} width={36} radius={4} size="w92" /></a>
                    <div class="rbody">
                      <a class="name" href={epHref(e.tmdb_id, e.season_number, e.from)}>{e.show_title}</a>
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
        {/if}
        {#if upcoming.truncated}<p class="hint">Showing the first {upcoming.episodes.length} episodes; pick a shorter window to see them all.</p>{/if}
      {/if}
    </section>
  {/if}
</div>

<style>
  .page { padding: 20px var(--gutter) 48px; display: grid; gap: 22px; }
  .head { display: flex; align-items: center; gap: 18px; }
  h1 { margin: 0; font: 700 32px/1.1 var(--font-display); letter-spacing: -0.02em; }
  .tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--border-subtle); }
  .tabs button { height: 38px; padding: 0 18px; border: 0; border-bottom: 2px solid transparent; margin-bottom: -1px; background: none; color: var(--text-muted); font: 700 14px/1 var(--font-sans); letter-spacing: 0.05em; text-transform: uppercase; cursor: pointer; }
  .tabs button[aria-selected='true'] { color: var(--text-primary); border-bottom-color: var(--text-primary); }
  .seg.view { margin-left: auto; }
  .seg.view button { display: grid; place-items: center; width: 38px; padding: 0; }
  .ptile { display: grid; gap: 5px; align-content: start; min-width: 0; color: inherit; text-decoration: none; }
  .ptile:hover .name { text-decoration: underline; text-underline-offset: 3px; }
  .pwrap { position: relative; display: block; transition: transform 0.15s ease; }
  .ptile:hover .pwrap { transform: translateY(-2px); }
  .pwrap :global(.poster) { width: 100% !important; }
  .corner { position: absolute; left: 8px; top: 8px; padding: 3px 8px; border-radius: 8px; background: rgb(11 11 12 / 0.82); color: #f5f5f7; font-size: 12.5px; font-weight: 700; }
  .corner.right { left: auto; right: 8px; color: var(--accent-yours); }
  .ptile.stalled .bar i { background: var(--status-stale); }
  .ptile .name { font: 700 15.5px/1.25 var(--font-display); }
  .ptile .meta { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { height: 30px; padding: 0 14px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .seg button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }

  .kpis { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; }
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
  .ep { margin: 0; color: inherit; text-decoration: none; font-size: 14.5px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
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
  }
  @media (max-width: 600px) {
    .cards { grid-template-columns: 1fr; }
    .card { grid-template-columns: 64px 1fr; gap: 12px; }
    .tiles { grid-template-columns: repeat(auto-fill, minmax(124px, 1fr)); gap: 18px 12px; }
    .rows li { grid-template-columns: 44px minmax(0, 1fr) auto auto; }
    .rows .bar.slim { display: none; }
    .day { grid-template-columns: 1fr; gap: 8px; }
    .uh .seg { margin-left: 0; }
  }
</style>
