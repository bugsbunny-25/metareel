<!-- Title page. A hero tinted from the poster, then the attention chart, timeline, In numbers and
     current positions. A title that never charted keeps the hero, details, In numbers and where to
     watch, and loses every chart panel. Sections that failed to load say so in place. -->
<script>
  import Poster from '../components/Poster.svelte'
  import Icon from '../components/Icon.svelte'
  import { api } from '../lib/api.js'
  import { library } from '../lib/library.svelte.js'
  import { scope, countryName, providerName } from '../lib/scope.svelte.js'
  import { sampleColor, seriesPath, compact, dayMonth } from '../lib/viz.js'
  import { stars } from '../lib/rows.js'
  import { toast } from '../lib/toast.svelte.js'

  let { kind, id, onlog } = $props()

  let loading = $state(true)
  let error = $state('')
  let d = $state(null)
  let tint = $state(null)

  $effect(() => {
    const k = kind, tid = id, country = scope.country
    const ctrl = new AbortController()
    loading = true
    error = ''
    d = null
    tint = null
    load(k, tid, country, ctrl.signal)
      .then((x) => { if (!ctrl.signal.aborted) d = x })
      .catch((err) => { if (err.name !== 'AbortError') error = err.status === 404 ? 'metareel doesn’t know this title.' : err.message })
      .finally(() => { if (!ctrl.signal.aborted) loading = false })
    return () => ctrl.abort()
  })

  async function load(k, tid, country, signal) {
    // A title outside metareel's catalogue (an obscure film you logged) has no overview;
    // /me/titles still returns its TMDB basics, which is enough for the collapsed page.
    const [ov, mine] = await Promise.all([
      api.overview(k, tid, signal).catch((e) => { if (e.status === 404) return null; throw e }),
      api.myTitle(k, tid, signal).catch(() => null),
    ])
    if (!ov && !mine?.title) throw Object.assign(new Error('not found'), { status: 404 })
    const charted = !!ov && (mine?.title?.charted ?? (ov.stats?.appearances ?? 0) > 0)
    const [timeline, history, offerHistory, credits] = await Promise.all([
      charted ? api.timeline(k, tid, country, signal).catch((e) => ({ failed: e.message })) : null,
      charted ? api.history(k, tid, signal).catch((e) => ({ failed: e.message })) : null,
      ov ? api.availabilityHistory(k, tid, signal).catch(() => null) : null,
      // Stored for catalogued titles; fetched live from TMDB for the rest.
      api.credits(k, tid, signal).catch(() => null),
    ])
    return { ov: ov ?? {}, catalogued: !!ov, mine, charted, timeline, history, offerHistory, credits }
  }

  const m = $derived(d?.ov.metadata ?? d?.mine?.title ?? {})
  const errs = $derived(d?.ov.errors ?? {})
  const state = $derived(d ? library.get(kind, id) ?? d.mine?.state ?? null : null)
  $effect(() => { if (m.poster_path) sampleColor(`https://image.tmdb.org/t/p/w154${m.poster_path}`).then((c) => (tint = c)) })

  const LABEL = { season_premiere: 'Season premiere', chart_debut: 'Debut', chart_peak: 'Peak', release: 'Release', netflix_premiere: 'Premiere' }
  const TYPE = { chart_debut: 'Chart', chart_peak: 'Chart', chart_number_one: 'Chart', chart_reentry: 'Chart', season_premiere: 'Release', release: 'Release',
    netflix_premiere: 'Release', rating_changed: 'Rating', pageview_spike: 'Attention', netflix_top10_week: 'Netflix', netflix_engagement: 'Netflix',
    availability_added: 'Streaming', availability_removed: 'Streaming' }

  const W = 640, H = 150
  const wiki = $derived(d?.history?.wikipedia?.points ?? [])
  const s = $derived(wiki.length > 1 ? seriesPath(wiki.map((p) => p.views), W, H, 4) : null)
  const events = $derived(d?.timeline?.events ?? [])
  const marks = $derived(s ? events.filter((e) => LABEL[e.type]).map((e) => ({ ...e, i: wiki.findIndex((p) => p.date === e.date) })).filter((x) => x.i >= 0).slice(0, 3) : [])
  const story = $derived(events.filter((e) => e.type !== 'rating_changed').slice(0, 6))
  const positions = $derived(d?.ov.current_positions ?? [])
  const best = $derived(positions.length ? positions.reduce((a, b) => (b.rank < a.rank ? b : a)) : null)
  const r = $derived(d?.ov.ratings ?? null)
  const tmdbScore = $derived(r?.sources?.find((x) => x.source === 'tmdb')?.value ?? m.vote_average ?? null)
  const here = $derived(d?.ov.availability?.countries?.[scope.country] ?? null)
  const streaming = $derived([...new Set((here?.flatrate ?? []).map((p) => p.provider_name))])
  const lastRemoved = $derived((d?.offerHistory?.events ?? []).filter((e) => e.country === scope.country && e.event === 'removed' && e.monetization === 'flatrate').at(-1) ?? null)
  const nfViews = $derived(d?.ov.netflix?.total_views || 0)

  // Cast & crew (stored once the credits job has run, else live from TMDB). Crew are grouped
  // into the lines a credits block reads as.
  const cast = $derived(d?.credits?.cast ?? [])
  const CREW = { Director: 'Directed by', Creator: 'Created by', Screenplay: 'Written by', Writer: 'Written by', Teleplay: 'Written by', Story: 'Story by',
    Novel: 'Based on the novel by', Characters: 'Characters by', 'Original Music Composer': 'Music by', 'Director of Photography': 'Cinematography by' }
  const crew = $derived.by(() => {
    const g = new Map()
    for (const c of d?.credits?.crew ?? []) {
      const label = CREW[c.job]
      if (!label) continue
      const list = g.get(label) ?? []
      if (!list.some((x) => x.person_id === c.person_id)) list.push(c)
      g.set(label, list)
    }
    const order = [...new Set(Object.values(CREW))]
    return [...g].sort((a, b) => order.indexOf(a[0]) - order.indexOf(b[0]))
  })
  let castAll = $state(false)

  async function toggleWatchlist() {
    try {
      const on = await library.toggleWatchlist(kind, id)
      toast(on ? `Added ${m.title} to your watchlist.` : `Removed ${m.title} from your watchlist.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }
  const log = () => onlog({ kind, id, name: m.title, poster: m.poster_path, season: m.number_of_seasons || 1 })
</script>

{#if error}
  <p class="alert" role="alert">{error}</p>
{:else if loading || !d}
  <div class="page" aria-busy="true" aria-label="Loading"><div class="skeleton" style="height:260px"></div></div>
{:else}
  <article class="page" style:--tint={tint ? `rgb(${tint.join(' ')} / 0.4)` : 'transparent'}>
    <header class="head">
      <div class="art"><Poster path={m.poster_path} seed={id} title={m.title ?? ''} width={d.charted ? 150 : 124} radius={10} /></div>
      <div class="copy">
        <p class="kicker">{kind === 'tv' ? 'Series' : 'Film'}{#if m.genres?.length}{' · '}{m.genres.slice(0, 2).join(', ')}{/if}</p>
        <h1>{m.title}</h1>
        <p class="meta">
          {#if kind === 'tv'}{m.number_of_seasons || '?'} season{m.number_of_seasons === 1 ? '' : 's'}{#if m.number_of_episodes}{' · '}{m.number_of_episodes} episodes{/if}{:else if m.runtime}{Math.floor(m.runtime / 60)} h {m.runtime % 60} min{/if}{#if m.release_date}{' · '}{m.release_date.slice(0, 4)}{/if}
        </p>
        {#if m.overview}<p class="overview">{m.overview}</p>{/if}
        <div class="actions">
          <button class="btn-primary" type="button" onclick={log}><Icon name="plus" size={17} />{state?.watch_count > 0 ? 'Log another watch' : 'Log a watch'}</button>
          <button class="btn-secondary" type="button" aria-pressed={state?.status === 'watchlist'} onclick={toggleWatchlist}>
            <Icon name={state?.status === 'watchlist' ? 'bookmark-fill' : 'bookmark'} size={16} />{state?.status === 'watchlist' ? 'On your Watchlist' : 'Add to Watchlist'}
          </button>
          {#if kind === 'tv'}<a class="btn-secondary" href="#/show/{id}"><Icon name="list" size={16} />Episodes</a>{/if}
          {#if state?.watch_count > 0}
            <span class="yours"><Icon name="check" size={15} />Watched {state.watch_count > 1 ? `${state.watch_count} times` : ''}{#if state.rating}{' · '}<span class="stars" aria-label="rated {state.rating / 2} of 5">{stars(state.rating)}</span>{/if}</span>
          {/if}
        </div>
      </div>
    </header>

    <div class="grid">
      <div class="col">
        {#if d.charted}
          <section class="panel">
            <div class="panel-h"><h2>Attention</h2><span class="hint">Daily Wikipedia pageviews{#if s}{' · '}peak {compact(s.max)} a day{/if}</span></div>
            {#if d.history?.failed}
              <p class="note">Pageviews didn’t load: {d.history.failed}</p>
            {:else if s}
              <svg class="chart" viewBox="0 0 {W} {H + 18}" role="img" aria-label="Daily Wikipedia pageviews over {wiki.length} days, with key events marked">
                <path d={s.area} fill="var(--accent-yours)" opacity="0.14" />
                <path d={s.line} fill="none" stroke="var(--accent-yours)" stroke-width="1.75" stroke-linejoin="round" />
                {#each marks as x}<line x1={s.x(x.i)} x2={s.x(x.i)} y1="4" y2={H - 4} stroke="var(--text-muted)" stroke-dasharray="2 3" />{/each}
                {#if marks.length}
                  <text x={Math.max(...marks.map((x) => s.x(x.i))) - 6} y={H + 14} text-anchor="end" class="ann">{marks.slice().sort((a, b) => a.i - b.i).map((x) => `${LABEL[x.type]} ${dayMonth(x.date)}`).join(' · ')}</text>
                {/if}
              </svg>
              <p class="source">Source: Wikimedia pageviews, via metareel.</p>
            {:else}
              <p class="note">No Wikipedia article is linked to this title yet.</p>
            {/if}
          </section>
          <section class="panel">
            <div class="panel-h"><h2>Timeline</h2><span class="hint">Key events{#if d.timeline?.total}{' · '}{d.timeline.total} in all{/if}</span></div>
            {#if d.timeline?.failed}
              <p class="note">The timeline didn’t load: {d.timeline.failed}</p>
            {:else if story.length}
              <table class="tbl">
                <tbody>
                  {#each story as e}
                    <tr><td class="num muted w-date"><time datetime={e.date}>{dayMonth(e.date)}</time></td><td class="w-type"><span class="pill">{TYPE[e.type] ?? 'Event'}</span></td><td class="wrap">{e.summary}</td></tr>
                  {/each}
                </tbody>
              </table>
            {:else}
              <p class="note">Nothing on the timeline yet.</p>
            {/if}
          </section>
        {:else}
          <section class="panel pad">
            <h2>No chart history</h2>
            <p class="note">This title hasn’t charted in any country metareel tracks, so there is no run, timeline or attention data to show.{#if !d.catalogued}{' '}Its details come from TMDB.{/if}</p>
          </section>
        {/if}
      </div>

      <div class="col">
        <section class="panel pad numbers">
          <h2>In numbers</h2>
          <dl>
            {#if d.charted && best}
              <dt>Best rank now</dt><dd class="num">No.&nbsp;{best.rank} · {best.country}</dd>
            {/if}
            {#if d.charted && d.ov.stats}
              <dt>Peak rank</dt><dd class="num">No.&nbsp;{d.ov.stats.peak_rank ?? '–'}</dd>
              <dt>Days at No. 1</dt><dd class="num">{d.ov.stats.days_at_number_1 ?? 0}</dd>
              <dt>Charted in</dt><dd class="num">{d.ov.stats.countries?.length ?? 0} {d.ov.stats.countries?.length === 1 ? 'country' : 'countries'}</dd>
            {/if}
            {#if nfViews}<dt>Netflix views</dt><dd class="num">{compact(nfViews)}</dd>{/if}
            {#if !d.charted}
              {#if m.release_date}<dt>Released</dt><dd class="num">{m.release_date.slice(0, 4)}</dd>{/if}
              {#if m.runtime}<dt>Runtime</dt><dd class="num">{m.runtime} min</dd>{/if}
              {#if m.genres?.length}<dt>Genres</dt><dd>{m.genres.join(', ')}</dd>{/if}
            {/if}
            {#if errs.ratings}
              <dt>Ratings</dt><dd class="muted">didn’t load</dd>
            {:else}
              {#if r?.imdb?.rating != null}<dt>IMDb</dt><dd class="num" class:stale={r.stale}>{r.imdb.rating.toFixed(1)}{#if r.imdb.votes}{' '}<span class="muted">· {compact(r.imdb.votes)} votes</span>{/if}</dd>{/if}
              {#if r?.rotten_tomatoes?.critics_score != null}<dt>Tomatometer</dt><dd class="num" class:stale={r.stale}>{r.rotten_tomatoes.critics_score}%{#if r.rotten_tomatoes.audience_score != null}{' '}<span class="muted">· audience {r.rotten_tomatoes.audience_score}%</span>{/if}</dd>{/if}
              {#if tmdbScore}<dt>TMDB</dt><dd class="num">{Number(tmdbScore).toFixed(1)}</dd>{/if}
              {#if r?.stale}<dt>Ratings</dt><dd class="stale">not refreshed, last known</dd>{/if}
            {/if}
            <dt>Your status</dt>
            <dd class="you">{state?.status === 'watchlist' ? 'On your Watchlist' : state?.status === 'watching' ? 'Watching' : state?.status === 'completed' ? 'Completed' : state?.watch_count > 0 ? 'Watched' : 'Not tracked'}</dd>
          </dl>
        </section>

        {#if d.charted && positions.length}
          <section class="panel">
            <div class="panel-h"><h2>Current positions</h2></div>
            <table class="tbl">
              <thead><tr><th scope="col">Country</th><th scope="col">Service</th><th scope="col" class="right">Rank</th></tr></thead>
              <tbody>{#each positions as p}<tr><td>{countryName(p.country)}</td><td class="muted">{providerName(p.provider)}</td><td class="right"><span class="rk num" class:one={p.rank === 1}>{p.rank}</span></td></tr>{/each}</tbody>
            </table>
          </section>
        {/if}

        <section class="panel pad">
          <h2>Where to watch · {scope.country}</h2>
          {#if !d.catalogued}
            <p class="note">metareel hasn’t looked up where this streams yet.</p>
          {:else if errs.availability}
            <p class="note">Availability didn’t load.</p>
          {:else if streaming.length}
            <p class="watch">{#each streaming as w}<span class="pill strong">{w}</span>{/each}</p>
          {:else if lastRemoved}
            <p class="left">Left {lastRemoved.provider_name} on {dayMonth(lastRemoved.observed_at.slice(0, 10))}. Not on a subscription service here now.</p>
          {:else}
            <p class="note">Not on a subscription service in {countryName(scope.country)} right now.</p>
          {/if}
          <p class="source">{d.ov.availability?.attribution ?? 'Watch provider data by JustWatch, via TMDB'}</p>
        </section>
      </div>
    </div>

    {#if cast.length || crew.length}
      <section class="panel pad credits" aria-labelledby="credits-h">
        <div class="credits-h"><h2 id="credits-h">Cast &amp; crew</h2>{#if cast.length > 12}<button class="more" onclick={() => (castAll = !castAll)} aria-expanded={castAll}>{castAll ? 'Show fewer' : `All ${cast.length} cast`}</button>{/if}</div>
        {#if cast.length}
          <ul class="cast">
            {#each castAll ? cast : cast.slice(0, 12) as c (c.person_id)}
              <li>
                <a href="#/person/{c.person_id}">
                  {#if c.profile_path}<img src="https://image.tmdb.org/t/p/w185{c.profile_path}" alt="" loading="lazy" />{:else}<span class="face-ph" aria-hidden="true">{c.name.split(/\s+/).slice(0, 2).map((w) => w[0]).join('')}</span>{/if}
                  <b>{c.name}</b>
                  {#if c.character}<span class="as">{c.character}</span>{/if}
                  {#if c.episodes}<span class="as num">{c.episodes} ep{c.episodes === 1 ? '' : 's'}</span>{/if}
                </a>
              </li>
            {/each}
          </ul>
        {/if}
        {#if crew.length}
          <dl class="crew">
            {#each crew as [label, people] (label)}
              <dt>{label}</dt>
              <dd>{#each people.slice(0, 4) as c, i (c.person_id)}{i ? ', ' : ''}<a href="#/person/{c.person_id}">{c.name}</a>{/each}</dd>
            {/each}
          </dl>
        {/if}
      </section>
    {/if}
  </article>
{/if}

<style>
  .page { padding: 20px var(--gutter) 40px; }
  .head { display: flex; gap: 32px; align-items: flex-end; padding: 28px; border-radius: var(--radius-hero); background: linear-gradient(100deg, var(--tint) 0%, transparent 75%), var(--bg-surface); }
  .art { flex: none; box-shadow: var(--shadow-art); border-radius: 10px; }
  .copy { min-width: 0; }
  .kicker { margin: 0 0 6px; color: var(--accent-yours); font-size: 13px; font-weight: 700; letter-spacing: 0.02em; }
  h1 { margin: 0; font: 700 44px/1.05 var(--font-display); letter-spacing: -0.02em; }
  .meta { margin: 8px 0 0; color: var(--text-muted); font-size: 14.5px; }
  .overview { margin: 10px 0 0; max-width: 62ch; font: 400 17px/25px var(--font-display); }
  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: 10px 14px; margin-top: 18px; }
  .yours { display: inline-flex; align-items: center; gap: 7px; color: var(--text-muted); }
  .stars { color: var(--accent-yours); letter-spacing: 0.04em; }

  .grid { display: grid; grid-template-columns: 1.45fr 1fr; gap: 16px; margin-top: 16px; }
  .col { display: grid; gap: 16px; align-content: start; min-width: 0; }
  .chart { display: block; width: 100%; height: auto; padding: 0 12px 2px; }
  .ann { fill: var(--text-primary); font: italic 400 12px var(--font-display); }
  .source { margin: 0; padding: 0 12px; color: var(--text-muted); font-size: 12px; }
  .panel.pad .source { padding: 0; }
  .note { margin: 0; padding: 0 12px 6px; color: var(--text-muted); font: italic 400 15px/22px var(--font-display); }
  .panel.pad .note { padding: 0; }
  .tbl td { height: 42px; padding: 6px 12px; }
  .w-date { width: 52px; white-space: nowrap; font-weight: 600; }
  .w-type { width: 92px; }
  .wrap { line-height: 19px; }
  .pill.strong { color: var(--text-primary); padding: 6px 12px; font-size: 13.5px; }
  .rk { font: 700 17px/1 var(--font-display); }
  .rk.one { color: var(--delta-up); }

  /* In numbers: label left, value right, 2px rule above. */
  .numbers { box-shadow: inset 0 2px 0 var(--text-primary); border-radius: 4px 4px var(--radius-panel) var(--radius-panel); }
  dl { display: grid; grid-template-columns: auto 1fr; gap: 9px 16px; margin: 6px 0 0; font-size: 15px; }
  dt { color: var(--text-muted); }
  dd { margin: 0; font-weight: 700; text-align: right; }
  dd .muted { font-weight: 400; }
  dd.you { color: var(--accent-yours); }
  .stale { color: var(--status-stale); }
  .watch { display: flex; flex-wrap: wrap; gap: 8px; margin: 0; }
  .left { margin: 0; color: var(--delta-down); font-weight: 600; }

  .credits { margin-top: 16px; gap: 14px; }
  .credits-h { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
  .more { padding: 0; border: 0; background: none; color: var(--accent-yours); font: 600 14px/1 var(--font-sans); cursor: pointer; }
  .cast { display: grid; grid-template-columns: repeat(auto-fill, minmax(112px, 1fr)); gap: 18px 14px; margin: 0; padding: 0; list-style: none; }
  .cast a { display: grid; gap: 3px; color: inherit; text-decoration: none; }
  .cast img, .face-ph { width: 100%; aspect-ratio: 1; border-radius: 50%; object-fit: cover; object-position: top; background: var(--bg-elevated); margin-bottom: 5px; }
  .face-ph { display: grid; place-items: center; color: var(--text-muted); font: 700 22px/1 var(--font-display); }
  .cast b { font-size: 14px; line-height: 1.25; }
  .cast a:hover b { text-decoration: underline; text-underline-offset: 3px; }
  .as { color: var(--text-muted); font-size: 12.5px; line-height: 1.3; }
  .crew { grid-template-columns: max-content 1fr; gap: 6px 18px; margin: 4px 0 0; padding-top: 12px; border-top: 1px solid var(--border-subtle); font-size: 14.5px; }
  .crew dd { text-align: left; font-weight: 600; }
  .crew a { color: inherit; text-underline-offset: 3px; text-decoration-color: color-mix(in srgb, currentColor 35%, transparent); }
  .crew a:hover { text-decoration-color: currentColor; }

  @media (max-width: 960px) {
    .grid { grid-template-columns: 1fr; }
  }
  @media (max-width: 640px) {
    .head { flex-direction: column; align-items: flex-start; padding: 20px; gap: 18px; }
    h1 { font-size: 34px; }
  }
</style>
