<!-- A show's episodes and your progress through them (#/show/:id?season=&specials=1), from
     /me/shows/:id. Tick an episode to mark it watched today; untick to delete its watches
     (asks first, they're gone for good). Each row's menu marks everything up to it or opens
     the log dialog for a dated, rated watch. -->
<script>
  import Poster from '../components/Poster.svelte'
  import Icon from '../components/Icon.svelte'
  import TitleMenu from '../components/TitleMenu.svelte'
  import { api } from '../lib/api.js'
  import { library } from '../lib/library.svelte.js'
  import { route, setQuery } from '../lib/router.svelte.js'
  import { toast } from '../lib/toast.svelte.js'
  import { statusLabel } from '../lib/status.js'
  import { sinceDay, dayLabel, localDay, hm } from '../lib/dates.js'
  import { markWatched, unmarkWatched, epCode, EPISODE_TYPE } from '../lib/shows.js'

  let { id, onlog, onconfirm } = $props()

  const specials = $derived(route.query.specials === '1')
  let data = $state(null)
  let error = $state('')
  let busy = $state(false)
  let nonce = $state(0)

  $effect(() => {
    nonce
    const sp = specials
    const ctrl = new AbortController()
    api.show(id, sp, ctrl.signal)
      .then((d) => { data = d; error = '' })
      .catch((err) => { if (err.name !== 'AbortError') error = err.status === 404 ? 'TMDB doesn’t know this show.' : err.message })
    return () => ctrl.abort()
  })
  const reload = () => nonce++

  const today = localDay()
  const aired = (e) => !!e.air_date && e.air_date <= today
  const title = $derived(data?.title ?? {})
  const seasons = $derived(data?.seasons ?? [])
  const regular = $derived(seasons.filter((s) => s.season_number > 0))
  const next = $derived.by(() => {
    for (const s of regular) for (const e of s.episodes) if (aired(e) && !e.watch_count) return e
    return null
  })
  const upcoming = $derived.by(() => {
    for (const s of regular) for (const e of s.episodes) if (!aired(e) && e.air_date) return e
    return null
  })
  const minutesLeft = $derived(regular.reduce((n, s) => n + s.episodes.reduce((m, e) => m + (aired(e) && !e.watch_count ? e.runtime ?? 0 : 0), 0), 0))
  const minutesWatched = $derived(regular.reduce((n, s) => n + s.episodes.reduce((m, e) => m + (e.watch_count ? (e.runtime ?? 0) * e.watch_count : 0), 0), 0))
  const p = $derived(data?.progress ?? { watched_episodes: 0, aired_episodes: 0, percent: 0 })

  const seasonNo = $derived.by(() => {
    const want = Number(route.query.season)
    if (route.query.season !== undefined && seasons.some((s) => s.season_number === want)) return want
    // Where you are: the next episode to watch, else the next one to air, else the last
    // season you watched something in, else the first.
    const lastSeen = regular.findLast((s) => s.watched_episodes > 0)
    return next?.season_number ?? upcoming?.season_number ?? lastSeen?.season_number ?? regular[0]?.season_number ?? seasons[0]?.season_number ?? 1
  })
  const season = $derived(seasons.find((s) => s.season_number === seasonNo) ?? null)
  const seasonAired = $derived(season?.episodes.filter(aired) ?? [])
  const seasonLeft = $derived(seasonAired.filter((e) => !e.watch_count).length)
  const seasonName = (n) => (n === 0 ? 'Specials' : `Season ${n}`)
  const range = (s) => {
    const dates = s.episodes.map((e) => e.air_date).filter(Boolean).sort()
    if (!dates.length) return 'No air dates yet'
    const a = dates[0], b = dates.at(-1)
    return a === b ? `${dayLabel(a)} ${a.slice(0, 4)}` : `${dayLabel(a)} ${a.slice(0, 4)} – ${dayLabel(b)} ${b.slice(0, 4)}`
  }

  async function run(action, done) {
    // Keep the season in view: without this, marking the last unwatched episode would move
    // the default season and the list would jump.
    if (route.query.season === undefined) setQuery({ season: seasonNo }, { replace: true })
    busy = true
    try {
      const res = await action()
      if (res?.title) data = res
      else reload()
      if (done) toast(done(res))
    } catch (err) {
      toast(err.message, 'error')
    } finally {
      busy = false
    }
  }
  const plural = (n, w) => `${n} ${w}${n === 1 ? '' : 's'}`
  const markOne = (e) => run(() => markWatched(id, { season: e.season_number, episode: e.episode_number, includeSpecials: specials }),
    () => `${epCode(e.season_number, e.episode_number)} watched.`)
  const markUpTo = (e) => run(() => markWatched(id, { scope: 'up_to', season: e.season_number, episode: e.episode_number, includeSpecials: specials }),
    (r) => (r.marked ? `Marked ${plural(r.marked, 'episode')} watched, up to ${epCode(e.season_number, e.episode_number)}.` : 'Those were already watched.'))
  const markSeason = (n) => run(() => markWatched(id, { scope: 'season', season: n, includeSpecials: specials }),
    (r) => `${seasonName(n)}: marked ${plural(r.marked, 'episode')} watched.`)
  const markAll = () => run(() => markWatched(id, { scope: 'all', includeSpecials: specials }),
    (r) => `Marked ${plural(r.marked, 'episode')} watched. Caught up.`)

  function unmark(e) {
    onconfirm({
      title: `Remove ${epCode(e.season_number, e.episode_number)} from your diary?`,
      body: `This deletes ${e.watch_count === 1 ? 'its watch' : `all ${e.watch_count} watches`} of “${e.name || 'this episode'}”, ratings and notes included. It can’t be undone.`,
      confirm: e.watch_count === 1 ? 'Delete watch' : 'Delete watches',
      danger: true,
      run: () => run(() => unmarkWatched(id, { season: e.season_number, episode: e.episode_number, includeSpecials: specials }),
        () => `${epCode(e.season_number, e.episode_number)} marked unwatched.`),
    })
  }
  function unmarkSeason(s) {
    const n = s.episodes.reduce((m, e) => m + e.watch_count, 0)
    onconfirm({
      title: `Mark ${seasonName(s.season_number).toLowerCase()} unwatched?`,
      body: `This deletes ${plural(n, 'watch')} across ${plural(s.watched_episodes, 'episode')}, ratings and notes included. It can’t be undone.`,
      confirm: 'Delete watches',
      danger: true,
      run: () => run(() => unmarkWatched(id, { scope: 'season', season: s.season_number, includeSpecials: specials }),
        (r) => `${seasonName(s.season_number)}: removed ${plural(r.removed, 'watch')}.`),
    })
  }
  const logEp = (e) => onlog({ kind: 'tv', id, name: title.title, poster: title.poster_path, season: e.season_number, episode: e.episode_number, onsaved: reload })
  const shut = (ev) => ev.currentTarget.closest('details')?.removeAttribute('open')

  const lib = $derived(library.get('tv', id))
</script>

<div class="page">
  {#if error}
    <p class="alert" role="alert">Couldn’t load this show: {error}</p>
  {:else if !data}
    <div class="skeleton" style="height:520px" aria-busy="true" aria-label="Loading"></div>
  {:else}
    <header class="hero">
      <Poster path={title.poster_path} seed={id} title={title.title} width={150} radius={12} size="w342" />
      <div class="hbody">
        <p class="crumb"><a href="#/up-next">Up next</a> / Episodes</p>
        <h1>{title.title}</h1>
        <p class="sub">
          {#if title.release_date}{title.release_date.slice(0, 4)}{' · '}{/if}{plural(regular.length, 'season')} · {plural(regular.reduce((n, s) => n + s.total_episodes, 0), 'episode')}
          {#if lib}{' · '}<span class="pill">{statusLabel(lib)}</span>{/if}
        </p>
        <div class="hactions">
          {#if next}
            <button class="btn-primary" disabled={busy} onclick={() => markOne(next)}><Icon name="check" size={16} />Watched {epCode(next.season_number, next.episode_number)}</button>
          {/if}
          <a class="btn-secondary" href="#/title/tv/{id}">Title page</a>
          {#if lib}<TitleMenu t={lib} {onlog} align="left" />{/if}
        </div>
      </div>
      <dl class="numbers" aria-label="In numbers">
        <div><dt>Watched</dt><dd class="num">{p.watched_episodes}<small>/{p.aired_episodes}</small></dd></div>
        <div><dt>Complete</dt><dd class="num">{p.percent}%</dd></div>
        <div><dt>Time left</dt><dd class="num">{minutesLeft ? hm(minutesLeft) : '–'}</dd></div>
        <div><dt>Time watched</dt><dd class="num">{minutesWatched ? hm(minutesWatched) : '–'}</dd></div>
        <div class="wide">
          <dt>{next ? 'Up next' : upcoming ? 'Next episode airs' : 'Status'}</dt>
          <dd>
            {#if next}{epCode(next.season_number, next.episode_number)} · {next.name}
            {:else if upcoming}<span class="new">{dayLabel(upcoming.air_date)} · {epCode(upcoming.season_number, upcoming.episode_number)}</span>
            {:else if p.aired_episodes}Caught up{:else}Nothing aired yet{/if}
          </dd>
        </div>
      </dl>
    </header>

    <section class="panel pad map" aria-label="Every episode">
      <div class="mh">
        <h2>Progress</h2>
        <span class="legend"><i class="cell watched"></i>Watched <i class="cell aired"></i>Not watched <i class="cell future"></i>Not aired</span>
        {#if p.watched_episodes < p.aired_episodes}
          <button class="btn-secondary sm" disabled={busy} onclick={markAll}>Mark all aired watched</button>
        {/if}
      </div>
      {#each seasons as s (s.season_number)}
        <div class="mrow">
          <button class="mlabel" class:on={s.season_number === seasonNo} onclick={() => setQuery({ season: s.season_number }, { replace: true })}>{s.season_number === 0 ? 'Sp' : `S${s.season_number}`}</button>
          <span class="cells" role="img" aria-label="{seasonName(s.season_number)}: {s.watched_episodes} of {s.total_episodes} watched">
            {#each s.episodes as e (e.episode_number)}
              <i class="cell" class:watched={e.watch_count > 0} class:aired={aired(e) && !e.watch_count} class:future={!aired(e)} class:next={e === next} title="{epCode(e.season_number, e.episode_number)} · {e.name}{e.watch_count ? ' · watched' : aired(e) ? '' : e.air_date ? ` · airs ${dayLabel(e.air_date)}` : ' · no air date'}"></i>
            {/each}
          </span>
          <span class="mcount num">{s.watched_episodes}/{s.total_episodes}</span>
        </div>
      {/each}
    </section>

    <section class="seasons">
      <div class="tabs" role="group" aria-label="Season">
        {#each seasons as s (s.season_number)}
          <button aria-pressed={s.season_number === seasonNo} onclick={() => setQuery({ season: s.season_number }, { replace: true })}>
            {seasonName(s.season_number)}
            <span class="tc num">{#if s.watched_episodes === s.total_episodes && s.total_episodes}<Icon name="check" size={12} />{:else}{s.watched_episodes}/{s.total_episodes}{/if}</span>
          </button>
        {/each}
        <label class="sp"><input type="checkbox" checked={specials} onchange={(e) => setQuery({ specials: e.currentTarget.checked ? '1' : '', season: '' }, { replace: true })} /> Include specials</label>
      </div>

      {#if season}
        <div class="panel season">
          <div class="seh">
            <div>
              <h2>{seasonName(season.season_number)}</h2>
              <span class="hint">{range(season)} · {season.watched_episodes} of {season.total_episodes} watched</span>
            </div>
            <div class="seh-actions">
              {#if seasonLeft}<button class="btn-secondary sm" disabled={busy} onclick={() => markSeason(season.season_number)}><Icon name="check" size={14} />Mark season watched</button>{/if}
              {#if season.watched_episodes}<button class="quiet sm" disabled={busy} onclick={() => unmarkSeason(season)}>Mark unwatched</button>{/if}
            </div>
          </div>
          <ol class="eps">
            {#each season.episodes as e (e.episode_number)}
              {@const isAired = aired(e)}
              <li class:watched={e.watch_count > 0} class:future={!isAired} class:next={e === next}>
                <button class="tick" aria-pressed={e.watch_count > 0} disabled={busy || (!isAired && !e.watch_count)}
                  aria-label="{epCode(e.season_number, e.episode_number)} {e.watch_count ? 'watched; mark unwatched' : isAired ? 'mark watched' : 'not aired yet'}"
                  title={e.watch_count ? 'Watched. Click to remove.' : isAired ? 'Mark watched today' : 'Not aired yet'}
                  onclick={() => (e.watch_count ? unmark(e) : markOne(e))}>
                  {#if e.watch_count}<Icon name="check" size={14} />{/if}
                </button>
                <span class="no num">{e.episode_number}</span>
                <div class="ebody">
                  <span class="ename">{e.name || 'Untitled episode'}
                    {#if e === next}{' '}<span class="tag">Up next</span>{/if}
                    {#if EPISODE_TYPE[e.episode_type]}{' '}<span class="badge">{EPISODE_TYPE[e.episode_type]}</span>{/if}
                  </span>
                  <span class="hint">
                    {#if !e.air_date}No air date yet{:else if isAired}{dayLabel(e.air_date)} {e.air_date.slice(0, 4)}{:else}<span class="new">Airs {dayLabel(e.air_date)} {e.air_date.slice(0, 4)}</span>{/if}
                    {#if e.runtime}{' · '}{e.runtime} min{/if}
                  </span>
                </div>
                <span class="when">
                  {#if e.watch_count}
                    {#if e.watch_count > 1}<span class="times num" title="Watched {e.watch_count} times"><Icon name="repeat" size={12} />{e.watch_count}</span>{/if}
                    {#if e.last_watched_at}<span class="hint">{sinceDay(e.last_watched_at)}</span>{/if}
                  {/if}
                </span>
                <details class="emenu">
                  <summary aria-label="More for {epCode(e.season_number, e.episode_number)}" title="More"><Icon name="more" size={15} /></summary>
                  <div class="pop">
                    {#if isAired}<button type="button" onclick={(ev) => { shut(ev); markUpTo(e) }}><Icon name="check" size={14} />Watched up to here</button>{/if}
                    <button type="button" onclick={(ev) => { shut(ev); logEp(e) }}><Icon name="plus" size={14} />Log with date and rating…</button>
                    {#if e.watch_count}<button type="button" class="danger" onclick={(ev) => { shut(ev); unmark(e) }}><Icon name="trash" size={14} />Mark unwatched</button>{/if}
                  </div>
                </details>
              </li>
            {/each}
          </ol>
        </div>
      {:else}
        <p class="hint">TMDB lists no episodes for this show yet.</p>
      {/if}
    </section>
  {/if}
</div>

<style>
  .page { padding: 20px var(--gutter) 48px; display: grid; gap: 18px; }
  .hero { display: grid; grid-template-columns: 150px minmax(0, 1fr) minmax(280px, 380px); gap: 24px; align-items: start; }
  .hbody { display: grid; gap: 8px; align-content: start; min-width: 0; }
  .crumb { margin: 0; color: var(--text-muted); font-size: 13px; }
  .crumb a { color: inherit; }
  h1 { margin: 0; font: 700 36px/1.08 var(--font-display); letter-spacing: -0.02em; }
  .sub { margin: 0; color: var(--text-muted); font-size: 14.5px; }
  .hactions { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-top: 6px; }
  .numbers { display: grid; grid-template-columns: 1fr 1fr; gap: 1px; margin: 0; border-radius: 16px; overflow: hidden; background: var(--border-subtle); }
  .numbers div { display: grid; gap: 3px; padding: 12px 14px; background: var(--bg-surface); }
  .numbers .wide { grid-column: 1 / -1; }
  dt { color: var(--text-muted); font-size: 12.5px; font-weight: 600; }
  dd { margin: 0; font: 700 22px/1.1 var(--font-sans); }
  dd small { color: var(--text-muted); font-size: 15px; }
  .wide dd { font: 600 15px/1.3 var(--font-display); }
  .new { color: var(--delta-debut); font-weight: 600; }

  .map { gap: 6px; }
  .mh { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 16px; margin-bottom: 4px; }
  .legend { display: inline-flex; align-items: center; gap: 6px; color: var(--text-muted); font-size: 12.5px; }
  .legend .cell { margin-left: 8px; }
  .legend .cell:first-child { margin-left: 0; }
  .mh .btn-secondary { margin-left: auto; }
  .mrow { display: grid; grid-template-columns: 36px 1fr 52px; align-items: center; gap: 10px; }
  .mlabel { height: 24px; border: 0; border-radius: 6px; background: none; color: var(--text-muted); font: 700 13px/1 var(--font-sans); cursor: pointer; }
  .mlabel:hover, .mlabel.on { background: var(--bg-elevated); color: var(--text-primary); }
  .cells { display: flex; flex-wrap: wrap; gap: 3px; }
  .cell { display: inline-block; width: 14px; height: 14px; border-radius: 3px; }
  .cell.watched { background: var(--accent-yours); }
  .cell.aired { background: var(--bg-elevated); box-shadow: inset 0 0 0 1px var(--border-strong); }
  .cell.future { background: transparent; box-shadow: inset 0 0 0 1px var(--border-subtle); background-image: repeating-linear-gradient(135deg, var(--border-subtle) 0 1.5px, transparent 1.5px 4px); }
  .cell.next { box-shadow: inset 0 0 0 2px var(--accent-yours); }
  .mcount { color: var(--text-muted); font-size: 12.5px; text-align: right; }

  .seasons { display: grid; gap: 12px; }
  .tabs { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
  .tabs button { display: inline-flex; align-items: center; gap: 8px; height: 34px; padding: 0 14px; border: 0; border-radius: 17px; background: var(--bg-surface); color: var(--text-muted); font: 600 14px/1 var(--font-sans); cursor: pointer; }
  .tabs button[aria-pressed='true'] { background: var(--accent-primary); color: var(--accent-on); }
  .tc { font-size: 12px; opacity: 0.8; display: inline-flex; }
  .sp { display: inline-flex; align-items: center; gap: 6px; margin-left: auto; color: var(--text-muted); font-size: 13.5px; cursor: pointer; }

  .season { padding: 16px 10px 8px; }
  .seh { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between; gap: 10px; padding: 0 10px 10px; }
  .seh h2 { margin: 0 0 2px; font: 700 20px/1.2 var(--font-display); }
  .seh-actions { display: flex; flex-wrap: wrap; gap: 8px; }
  .sm { height: 32px; padding: 0 14px; font-size: 13.5px; gap: 6px; white-space: nowrap; }
  .quiet { border: 0; border-radius: 16px; background: none; color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .quiet:hover { color: var(--delta-down); background: var(--bg-elevated); }

  .eps { display: grid; margin: 0; padding: 0; list-style: none; }
  .eps li { display: grid; grid-template-columns: 30px 28px minmax(0, 1fr) auto 32px; align-items: center; gap: 12px; padding: 9px 10px; border-radius: 10px; }
  .eps li:hover { background: var(--bg-elevated); }
  .eps li.next { background: color-mix(in srgb, var(--accent-yours) 9%, transparent); }
  .eps li.future .ename { color: var(--text-muted); }
  .tick { display: grid; place-items: center; width: 26px; height: 26px; padding: 0; border: 0; border-radius: 50%; background: none; box-shadow: inset 0 0 0 1.5px var(--border-strong); color: var(--accent-on); cursor: pointer; }
  .tick:hover:not(:disabled) { box-shadow: inset 0 0 0 2px var(--accent-yours); }
  .tick[aria-pressed='true'] { background: var(--accent-yours); box-shadow: none; color: #1a0d05; }
  .tick:disabled { cursor: default; opacity: 0.4; }
  .no { color: var(--text-muted); font-weight: 700; text-align: right; }
  .ebody { display: grid; gap: 2px; min-width: 0; }
  .ename { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 600 15px/1.3 var(--font-display); }
  .tag { color: var(--accent-yours); font: 700 11.5px/1 var(--font-sans); text-transform: uppercase; letter-spacing: 0.05em; }
  .badge { display: inline-block; padding: 1px 7px; border-radius: 9px; background: var(--bg-elevated); font: 700 11.5px/1.5 var(--font-sans); }
  .when { display: inline-flex; align-items: center; gap: 8px; }
  .times { display: inline-flex; align-items: center; gap: 3px; color: var(--accent-yours); font-size: 12.5px; font-weight: 700; }

  .emenu { position: relative; }
  .emenu summary { display: grid; place-items: center; width: 30px; height: 30px; border-radius: 50%; color: var(--text-muted); cursor: pointer; list-style: none; }
  .emenu summary::-webkit-details-marker { display: none; }
  .emenu summary:hover, .emenu[open] summary { background: var(--bg-surface); color: var(--text-primary); }
  .emenu summary:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 2px; }
  .pop { position: absolute; right: 0; top: 34px; z-index: 20; display: grid; gap: 1px; min-width: 230px; padding: 6px; border-radius: 12px; background: var(--bg-surface); box-shadow: 0 18px 50px -12px rgb(0 0 0 / 0.55), 0 0 0 1px var(--border-subtle); }
  .pop button { display: flex; align-items: center; gap: 10px; padding: 8px; border: 0; border-radius: 8px; background: none; color: var(--text-primary); text-align: left; font: 500 14px/1.2 var(--font-sans); cursor: pointer; }
  .pop button:hover, .pop button:focus-visible { background: var(--bg-elevated); outline: none; }
  .pop .danger { color: var(--delta-down); }

  @media (max-width: 1100px) {
    .hero { grid-template-columns: 120px minmax(0, 1fr); }
    .numbers { grid-column: 1 / -1; grid-template-columns: repeat(4, 1fr); }
  }
  @media (max-width: 600px) {
    .hero { grid-template-columns: 96px minmax(0, 1fr); gap: 14px; }
    h1 { font-size: 26px; }
    .numbers { grid-template-columns: 1fr 1fr; }
    .eps li { grid-template-columns: 28px minmax(0, 1fr) auto 30px; gap: 10px; }
    .eps .no { display: none; }
    .sp { margin-left: 0; }
  }
</style>
