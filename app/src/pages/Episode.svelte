<!-- One episode (#/episode/:show/:season/:episode), from /me/shows/:id/episodes/:season/:episode:
     its still with the show, name, air date, runtime and TMDB score, Log, Mark watched and View
     whole series, the synopsis, who directed and wrote it, its guest cast, and your watches of it.
     Previous and next episode links come from the show's episode list. TMDB is asked live; when it
     can't be reached the stored episode is shown and the page says so. -->
<script>
  import Icon from '../components/Icon.svelte'
  import Poster from '../components/Poster.svelte'
  import RatingLogo from '../components/RatingLogo.svelte'
  import { api } from '../lib/api.js'
  import { posterSrc, stars, starsLabel } from '../lib/rows.js'
  import { localDay, dayLabel, clock } from '../lib/dates.js'
  import { markWatched, epCode, EPISODE_TYPE } from '../lib/shows.js'
  import { toast } from '../lib/toast.svelte.js'

  let { show, season, episode, onlog } = $props()

  let data = $state(null)
  let list = $state(null)
  let error = $state('')
  let busy = $state(false)
  let nonce = $state(0)

  $effect(() => {
    nonce
    const ctrl = new AbortController()
    api.episode(show, season, episode, ctrl.signal)
      .then((d) => { data = d; error = '' })
      .catch((err) => { if (err.name !== 'AbortError') error = err.status === 404 ? 'TMDB doesn’t know this episode.' : err.message })
    return () => ctrl.abort()
  })
  // The show's episodes, for previous / next.
  $effect(() => {
    const ctrl = new AbortController()
    api.show(show, season === 0, ctrl.signal).then((d) => (list = d)).catch(() => {})
    return () => ctrl.abort()
  })
  const reload = () => nonce++

  const e = $derived(data?.episode ?? null)
  const title = $derived(data?.show?.title ?? '')
  const today = localDay()
  const aired = $derived(!!e?.air_date && e.air_date <= today)
  const watches = $derived(data?.watches ?? [])
  const still = $derived(e?.still_path ? `https://image.tmdb.org/t/p/w1280${e.still_path}` : null)
  const badge = $derived(e ? (e.episode_type === 'finale' ? 'Finale' : EPISODE_TYPE[e.episode_type] ?? (e.episode_number === 1 ? (e.season_number === 1 ? 'Series premiere' : 'Season premiere') : null)) : null)
  const meta = $derived(e ? [
    e.air_date ? `${new Date(`${e.air_date}T12:00:00`).toLocaleDateString(undefined, { day: 'numeric', month: 'long', year: 'numeric' })}` : 'Air date TBA',
    e.runtime ? `${e.runtime} min` : '',
  ].filter(Boolean).join(' · ') : '')

  const flat = $derived((list?.seasons ?? []).flatMap((s) => s.episodes))
  const at = $derived(flat.findIndex((x) => x.season_number === season && x.episode_number === episode))
  const prev = $derived(at > 0 ? flat[at - 1] : null)
  const next = $derived(at >= 0 && at < flat.length - 1 ? flat[at + 1] : null)
  const epHref = (x) => `#/episode/${show}/${x.season_number}/${x.episode_number}`

  async function mark() {
    busy = true
    try {
      await markWatched(show, { season, episode, precision: 'minute' })
      toast(`${title}: ${epCode(season, episode)} watched.`)
      reload()
    } catch (err) {
      toast(err.message, 'error')
    } finally {
      busy = false
    }
  }
  const log = () => onlog({ kind: 'tv', id: show, name: title, poster: data?.show?.poster_path, season, episode, onsaved: reload })
  const initials = (n) => (n ?? '').split(' ').map((w) => w[0]).join('').slice(0, 2)
  const watchedText = (w) => w.time_precision === 'minute' ? `${dayLabel(w.watched_on)} ${w.watched_on.slice(0, 4)}, ${clock(w.ended_at)}` : w.time_precision === 'day' ? `${dayLabel(w.watched_on)} ${w.watched_on.slice(0, 4)}` : w.time_precision === 'month' ? new Date(`${w.watched_on}T12:00:00`).toLocaleDateString(undefined, { month: 'long', year: 'numeric' }) : w.watched_on.slice(0, 4)
</script>

<div class="page">
  {#if error}
    <p class="alert" role="alert">Couldn’t load this episode: {error}</p>
  {:else if !data}
    <div class="skeleton" style="height:420px" aria-busy="true" aria-label="Loading"></div>
  {:else if e}
    <!-- Without a still (often the case before an episode airs) the header is compact, with the show's poster. -->
    <header class="hero" class:bare={!still} style:background-image={still ? `url(${still})` : undefined}>
      {#if still}<div class="scrim"></div>{:else}<a href="#/show/{show}" tabindex="-1" aria-hidden="true"><Poster path={data.show?.poster_path ?? null} seed={show} {title} width={96} radius={8} size="w185" /></a>{/if}
      <div class="htext">
        <a class="show" href="#/show/{show}">{title} ›</a>
        <p class="code num">{epCode(season, episode)}</p>
        <h1>{e.name || `Episode ${episode}`}</h1>
      </div>
    </header>

    <div class="cols">
      <div class="main">
        <div class="meta">
          {#if badge}<span class="badge">{badge}</span>{/if}
          <span class="hint num">{meta}</span>
          {#if e.vote_count}<span class="score num" title="TMDB {e.vote_average?.toFixed(1)} from {e.vote_count} votes"><RatingLogo source="tmdb" height={15} />{e.vote_average?.toFixed(1)}</span>{/if}
        </div>
        {#if !data.live}<p class="hint">Showing what’s saved; TMDB couldn’t be reached for the rest.</p>{/if}
        <div class="actions">
          {#if aired}
            <button class="btn-primary" onclick={log}><Icon name="plus" size={16} />Log</button>
            <button class="btn-secondary" disabled={busy || watches.length > 0} onclick={mark}>
              <Icon name="check" size={16} />{watches.length ? `Watched ${watches.length}×` : 'Mark watched'}
            </button>
          {/if}
          <a class="btn-secondary" href="#/show/{show}?season={season}">View whole series</a>
          <a class="btn-secondary" href="#/title/tv/{show}">Title page</a>
        </div>

        {#if e.overview}<p class="overview">{e.overview}</p>{/if}

        {#if e.directors?.length || e.writers?.length}
          <dl class="facts">
            {#if e.directors?.length}<div><dt>{e.directors.length > 1 ? 'Directors' : 'Director'}</dt><dd>{e.directors.join(', ')}</dd></div>{/if}
            {#if e.writers?.length}<div><dt>{e.writers.length > 1 ? 'Writers' : 'Writer'}</dt><dd>{e.writers.join(', ')}</dd></div>{/if}
          </dl>
        {/if}

        {#if e.guest_stars?.length}
          <section aria-labelledby="gs-h">
            <h2 id="gs-h" class="eyebrow">Guest stars</h2>
            <ul class="cast">
              {#each e.guest_stars as g (g.id)}
                <li>
                  <a href="#/person/{g.id}">
                    <span class="face">{#if g.profile_path}<img src={posterSrc(g.profile_path, 'w185')} alt="" loading="lazy" />{:else}<span>{initials(g.name)}</span>{/if}</span>
                    <b>{g.name}</b>
                    {#if g.character}<span class="hint">{g.character}</span>{/if}
                  </a>
                </li>
              {/each}
            </ul>
          </section>
        {/if}
      </div>

      <aside class="side">
        <section class="panel pad">
          <h2>Your watches</h2>
          {#if watches.length}
            <ul class="watches">
              {#each watches as w (w.id)}
                <li>
                  <a href="#/watch/{w.id}">{watchedText(w)}</a>
                  {#if w.rating}<span class="stars" aria-label={starsLabel(w.rating)}>{stars(w.rating)}</span>{/if}
                  {#if w.liked}<span class="liked" aria-label="Liked"><Icon name="heart-fill" size={14} /></span>{/if}
                </li>
              {/each}
            </ul>
          {:else}
            <p class="hint">{aired ? 'Not watched yet.' : `Airs ${e.air_date ? dayLabel(e.air_date) : 'on a date not announced yet'}.`}</p>
          {/if}
        </section>
        {#if prev || next}
          <nav class="step" aria-label="Other episodes">
            {#if prev}<a class="panel pad" href={epHref(prev)}><span class="hint"><Icon name="chevron-left" size={14} />Previous</span><b>{epCode(prev.season_number, prev.episode_number)} · {prev.name || 'Untitled'}</b></a>{/if}
            {#if next}<a class="panel pad" href={epHref(next)}><span class="hint">Next<Icon name="chevron" size={14} /></span><b>{epCode(next.season_number, next.episode_number)} · {next.name || 'Untitled'}</b></a>{/if}
          </nav>
        {/if}
      </aside>
    </div>
  {/if}
</div>

<style>
  .page { padding: 20px var(--gutter) 48px; display: grid; gap: 20px; }
  .hero { position: relative; display: flex; align-items: flex-end; height: clamp(240px, 34vw, 460px); border-radius: 20px; overflow: hidden; background: var(--bg-surface) center / cover no-repeat; }
  .scrim { position: absolute; inset: 0; background: linear-gradient(to top, rgb(11 11 12 / 0.92), rgb(11 11 12 / 0.25) 60%, transparent); }
  .hero.bare { height: auto; align-items: center; gap: 4px; padding: 16px; }
  .hero.bare .htext { padding: 8px 12px; color: var(--text-primary); }
  .htext { position: relative; display: grid; gap: 4px; padding: 26px 28px; color: #f5f5f7; }
  .show { color: var(--accent-yours); font: 700 13px/1 var(--font-sans); letter-spacing: 0.08em; text-transform: uppercase; text-decoration: none; }
  .show:hover { text-decoration: underline; }
  .code { margin: 4px 0 0; font-weight: 700; opacity: 0.85; }
  h1 { margin: 0; font: 700 40px/1.08 var(--font-display); letter-spacing: -0.02em; }

  .cols { display: grid; grid-template-columns: minmax(0, 1fr) 340px; gap: 28px; align-items: start; }
  .main { display: grid; gap: 16px; min-width: 0; }
  .main > * { min-width: 0; }
  .meta { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; }
  .badge { padding: 2px 9px; border-radius: 10px; background: var(--bg-elevated); font-size: 12px; font-weight: 700; }
  .meta .hint { font-size: 14.5px; }
  .score { display: inline-flex; align-items: center; gap: 5px; font-size: 14.5px; font-weight: 600; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; }
  .actions a, .actions button { gap: 6px; text-decoration: none; }
  .overview { margin: 0; max-width: 72ch; font: 400 18px/1.55 var(--font-display); }
  .facts { display: grid; gap: 8px; margin: 0; }
  .facts div { display: grid; grid-template-columns: 96px 1fr; gap: 12px; }
  dt { color: var(--text-muted); font-size: 13px; font-weight: 600; padding-top: 2px; }
  dd { margin: 0; }
  .eyebrow { margin: 0 0 10px; color: var(--text-muted); font: 700 12.5px/1 var(--font-sans); letter-spacing: 0.1em; text-transform: uppercase; }
  .cast { display: grid; grid-template-columns: repeat(auto-fill, minmax(100px, 1fr)); gap: 16px 14px; margin: 0; padding: 0; list-style: none; }
  .cast li { min-width: 0; }
  .cast a { display: grid; gap: 4px; color: inherit; text-decoration: none; }
  .cast a:hover b { text-decoration: underline; }
  .face { display: grid; place-items: center; width: 100%; aspect-ratio: 3 / 4; border-radius: 10px; overflow: hidden; background: var(--bg-elevated); color: var(--text-muted); font-weight: 700; }
  .face img { width: 100%; height: 100%; object-fit: cover; }
  .cast b { font: 600 13.5px/1.25 var(--font-sans); }
  .cast .hint { font-size: 12.5px; }

  .side { display: grid; gap: 12px; }
  .watches { display: grid; gap: 6px; margin: 0; padding: 0; list-style: none; }
  .watches li { display: flex; align-items: center; gap: 10px; min-height: 30px; }
  .watches a { flex: 1; color: inherit; }
  .stars { color: var(--accent-yours); }
  .liked { color: var(--delta-down); display: grid; }
  .step { display: grid; gap: 8px; }
  .step a { gap: 4px; color: inherit; text-decoration: none; }
  .step a:hover { background: var(--bg-elevated); }
  .step .hint { display: inline-flex; align-items: center; gap: 4px; }
  .step b { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 600 15px/1.3 var(--font-display); }

  @media (max-width: 600px) {
    .cast { display: flex; overflow-x: auto; padding-bottom: 8px; }
    .cast li { flex: none; width: 100px; }
  }
  @media (max-width: 960px) {
    .cols { grid-template-columns: minmax(0, 1fr); }
    h1 { font-size: 30px; }
  }
</style>
