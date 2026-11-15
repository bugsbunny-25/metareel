<!-- Detailed statistics (#/you/stats?kind=tv|movie), all time, from /me/stats/detailed: time spent,
     counts, rewatches, the titles you spent most time on, where your shows stand, what's left and
     when you'd catch up, upcoming watch time, this week and month, trends, the day of the week,
     ratings, genres, actors and directors, and binges. Shows or Movies is remembered in this
     browser (the mobile app's "stats.kind"); a kind in the URL wins. -->
<script>
  import Icon from '../components/Icon.svelte'
  import Columns from '../components/Columns.svelte'
  import BarList from '../components/BarList.svelte'
  import Duration from '../components/Duration.svelte'
  import Poster from '../components/Poster.svelte'
  import SubscriptionSavings from '../components/SubscriptionSavings.svelte'
  import { api } from '../lib/api.js'
  import { route, setQuery } from '../lib/router.svelte.js'
  import { remembered } from '../lib/remember.svelte.js'
  import { posterSrc, stars } from '../lib/rows.js'
  import { dayLabel } from '../lib/dates.js'
  import { shortDuration, hoursText, plural } from '../lib/duration.js'

  const saved = remembered('stats.kind', 'tv', ['tv', 'movie'])
  const kind = $derived(route.query.kind === 'tv' || route.query.kind === 'movie' ? route.query.kind : saved.value)
  const tv = $derived(kind === 'tv')
  function pick(k) {
    saved.value = k
    setQuery({ kind: k }, { replace: true })
  }

  let data = $state({})
  let error = $state('')
  $effect(() => {
    const k = kind
    if (data[k]) return
    const ctrl = new AbortController()
    error = ''
    api.statsDetailed(k, ctrl.signal)
      .then((d) => (data = { ...data, [k]: d }))
      .catch((err) => { if (err.name !== 'AbortError') error = err.message })
    return () => ctrl.abort()
  })
  const s = $derived(data[kind] ?? null)

  let period = $state('weeks')
  let measure = $state('time')
  const noun = (n) => (tv ? (n === 1 ? 'episode' : 'episodes') : n === 1 ? 'movie' : 'movies')
  const hrs = (min) => Math.round(((min ?? 0) / 60) * 10) / 10
  const MON = new Intl.DateTimeFormat(undefined, { month: 'short' })
  const monthOf = (ym) => MON.format(new Date(`${ym}-15T12:00:00`))
  const label = (p) => (period === 'weeks' ? dayLabel(p.label) : period === 'months' ? `${monthOf(p.label)}${p.label.endsWith('-01') ? ` ’${p.label.slice(2, 4)}` : ''}` : p.label)
  const point = (p, name) => {
    const value = measure === 'time' ? hrs(p.minutes) : p.watches
    return { label: name, value, title: `${name}: ${p.watches} ${noun(p.watches)} · ${hoursText(p.minutes)}` }
  }
  const trend = $derived(((s?.trend?.[period]) ?? []).map((p) => point(p, label(p))))
  const weekdays = $derived.by(() => {
    // Monday first; the API lists Sunday first.
    const d = s?.by_weekday ?? []
    return [...d.slice(1), ...d.slice(0, 1)].map((p) => point(p, p.label.slice(0, 3)))
  })
  const unit = $derived(measure === 'time' ? ' h' : '')
  const upcoming = $derived.by(() => {
    const m = s?.upcoming_by_month ?? []
    const timed = m.some((x) => x.minutes)
    // Runtimes of unaired episodes are often unknown yet: count them instead.
    return {
      timed,
      items: m.map((x) => ({ label: monthOf(x.label), value: timed ? hrs(x.minutes) : x.watches, title: `${monthOf(x.label)}: ${plural(x.watches, 'episode')}${x.minutes ? ` · ${hoursText(x.minutes)}` : ''}` })),
      any: m.some((x) => x.watches),
    }
  })
  const ratings = $derived.by(() => {
    const d = s?.ratings?.distribution ?? {}
    return Array.from({ length: 10 }, (_, i) => ({ label: `${Math.floor((i + 1) / 2) || ''}${(i + 1) % 2 ? '½' : ''}★`, value: d[i + 1] ?? 0, title: `${(i + 1) / 2} ★: ${d[i + 1] ?? 0}` }))
  })
  const titleHref = (t) => (tv ? `#/show/${t.tmdb_id}` : `#/title/movie/${t.tmdb_id}`)
</script>

<div class="page">
  <header class="head">
    <p class="crumb"><a href="#/you">You</a> /</p>
    <h1>Detailed stats</h1>
    <div class="tabs" role="tablist" aria-label="Shows or movies">
      <button role="tab" aria-selected={tv} onclick={() => pick('tv')}>Shows</button>
      <button role="tab" aria-selected={!tv} onclick={() => pick('movie')}>Movies</button>
    </div>
  </header>

  {#if error}<p class="alert" role="alert">Couldn’t load your statistics: {error}</p>{/if}

  {#if !s && !error}
    <div class="skeleton" style="height:520px" aria-busy="true" aria-label="Loading"></div>
  {:else if s}
    <div class="grid">
      <section class="panel pad wide">
        <h2>{tv ? 'Time spent watching series' : 'Time spent watching movies'}</h2>
        <Duration minutes={s.minutes ?? 0} size={40} />
      </section>

      <section class="panel pad"><h2>{tv ? 'Watched episodes' : 'Movies watched'}</h2><b class="big num">{((tv ? s.episodes : s.movies) ?? 0).toLocaleString()}</b></section>
      <section class="panel pad"><h2>{tv ? 'Rewatched episodes' : 'Rewatches'}</h2><b class="big num">{(s.rewatches ?? 0).toLocaleString()}</b></section>
      <section class="panel pad"><h2>{tv ? 'Shows' : 'Films'}</h2><b class="big num">{((tv ? s.shows : s.films) ?? 0).toLocaleString()}</b></section>
      <section class="panel pad">
        <h2>Streak</h2>
        <b class="big num"><span class="flame"><Icon name="flame" size={26} /></span>{s.streak?.current ?? 0}<small>{s.streak?.current === 1 ? ' day' : ' days'} · best {s.streak?.longest ?? 0}</small></b>
      </section>

      {#if s.top_titles?.length}
        <section class="panel pad tall">
          <h2>{tv ? 'Most-watched series' : 'Most-watched movies'}</h2>
          <ol class="rank">
            {#each s.top_titles as t, i (t.tmdb_kind + t.tmdb_id)}
              <li>
                <span class="no num">{i + 1}</span>
                <Poster path={t.poster_path ?? null} seed={t.tmdb_id} title={t.title} width={36} radius={4} size="w92" />
                <span class="rb"><a href={titleHref(t)}>{t.title}</a><span class="hint">{tv ? plural(t.watches, 'episode') : t.watches > 1 ? `${t.watches} times` : 'Once'}</span></span>
                <span class="val num">{hoursText(t.minutes)}</span>
              </li>
            {/each}
          </ol>
        </section>
      {/if}

      {#if tv && s.shows_by_stage?.length}
        <section class="panel pad tall">
          <h2>Breakdown by status</h2>
          <BarList items={s.shows_by_stage.map((x) => ({ label: x.label, value: x.count }))} />
        </section>
      {/if}

      {#if tv && s.minutes_left != null}
        <section class="panel pad">
          <h2>Time left to watch</h2>
          <Duration minutes={s.minutes_left} size={28} />
          <span class="hint">Aired episodes you haven’t seen of the shows you’re on</span>
        </section>
      {/if}
      {#if tv && s.catch_up_date}
        <section class="panel pad">
          <h2>Catch-up date</h2>
          <b class="date">{new Date(`${s.catch_up_date}T12:00:00`).toLocaleDateString(undefined, { day: 'numeric', month: 'long', year: 'numeric' })}</b>
          <span class="hint">At your pace of the last 8 weeks ({shortDuration(s.pace_minutes_per_week ?? 0)} a week)</span>
        </section>
      {/if}

      {#if tv && upcoming.any}
        <section class="panel pad wide">
          <div class="ph"><h2>Upcoming watch time</h2><span class="hint">{upcoming.timed ? 'Hours of episodes airing per month' : 'Episodes airing per month; runtimes aren’t announced yet'}</span></div>
          <Columns items={upcoming.items} height={110} unit={upcoming.timed ? ' h' : ''} label="Upcoming watch time, next six months" />
        </section>
      {/if}

      <section class="panel pad">
        <h2>This week</h2>
        <b class="mid num">{plural(s.this_week?.watches ?? 0, noun(1), noun(2))}</b>
        <span class="hint num">{shortDuration(s.this_week?.minutes ?? 0)} · since Monday</span>
      </section>
      <section class="panel pad">
        <h2>This month</h2>
        <b class="mid num">{plural(s.this_month?.watches ?? 0, noun(1), noun(2))}</b>
        <span class="hint num">{shortDuration(s.this_month?.minutes ?? 0)}</span>
      </section>
      <section class="panel pad">
        <h2>This year</h2>
        <b class="mid num">{plural(s.this_year?.watches ?? 0, noun(1), noun(2))}</b>
        <span class="hint num">{shortDuration(s.this_year?.minutes ?? 0)}</span>
      </section>

      <section class="panel pad wide">
        <div class="ph">
          <h2>Trend</h2>
          <div class="seg" role="group" aria-label="Measure">
            <button aria-pressed={measure === 'time'} onclick={() => (measure = 'time')}>Watch time</button>
            <button aria-pressed={measure === 'count'} onclick={() => (measure = 'count')}>{tv ? 'Episodes' : 'Movies'}</button>
          </div>
          <div class="seg" role="group" aria-label="Period">
            <button aria-pressed={period === 'weeks'} onclick={() => (period = 'weeks')}>Weeks</button>
            <button aria-pressed={period === 'months'} onclick={() => (period = 'months')}>Months</button>
            <button aria-pressed={period === 'years'} onclick={() => (period = 'years')}>Years</button>
          </div>
        </div>
        <Columns items={trend} height={150} {unit} every={period === 'weeks' ? 2 : 1} highlight={trend.length - 1} label="{measure === 'time' ? 'Hours' : tv ? 'Episodes' : 'Movies'} per {period.slice(0, -1)}" />
      </section>

      <section class="panel pad half">
        <h2>Day of the week</h2>
        <Columns items={weekdays} height={110} {unit} label="{measure === 'time' ? 'Hours' : tv ? 'Episodes' : 'Movies'} by weekday" />
      </section>

      <section class="panel pad half">
        <div class="ph">
          <h2>Ratings</h2>
          {#if s.ratings?.rated}<span class="hint">Average <span class="stars">{stars(Math.round(s.ratings.average ?? 0))}</span> ({((s.ratings.average ?? 0) / 2).toFixed(1)} of 5) · {plural(s.ratings.rated, 'rated watch', 'rated watches')}</span>{/if}
        </div>
        {#if s.ratings?.rated}
          <Columns items={ratings} height={110} label="Your ratings" />
        {:else}
          <p class="hint">No ratings yet.</p>
        {/if}
      </section>

      {#if s.genres?.length}
        <section class="panel pad half">
          <h2>Genres</h2>
          <BarList items={s.genres.map((g) => ({ label: g.label, value: g.count }))} />
        </section>
      {/if}

      {#each [['Most-watched actors', s.actors, tv ? 'episode' : 'viewing'], ['Most-watched directors', s.directors, 'viewing']] as [heading, people, unitName]}
        {#if people?.length}
          <section class="panel pad half">
            <h2>{heading}</h2>
            <ol class="rank">
              {#each people as p, i (p.person_id)}
                <li>
                  <span class="no num">{i + 1}</span>
                  <span class="face">{#if p.profile_path}<img src={posterSrc(p.profile_path, 'w185')} alt="" loading="lazy" />{/if}</span>
                  <span class="rb"><a href="#/person/{p.person_id}">{p.name}</a></span>
                  <span class="val num">{plural(p.watches, unitName)}</span>
                </li>
              {/each}
            </ol>
          </section>
        {/if}
      {/each}

      {#if tv && s.binges?.length}
        <section class="panel pad half">
          <h2>Biggest binges</h2>
          <ol class="rank">
            {#each s.binges as b, i (b.tmdb_id + b.day)}
              <li>
                <span class="no num">{i + 1}</span>
                <Poster path={b.poster_path ?? null} seed={b.tmdb_id} title={b.title} width={36} radius={4} size="w92" />
                <span class="rb"><a href="#/show/{b.tmdb_id}">{b.title}</a><span class="hint">{new Date(`${b.day}T12:00:00`).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })} · {hoursText(b.minutes)}</span></span>
                <span class="val num">{b.episodes} ep.</span>
              </li>
            {/each}
          </ol>
        </section>
      {/if}
    </div>

    {#if !tv}<SubscriptionSavings />{/if}
  {/if}
</div>

<style>
  .page { padding: 20px var(--gutter) 48px; display: grid; gap: 18px; max-width: 1320px; }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 16px; }
  .crumb { margin: 0; color: var(--text-muted); font-size: 14px; }
  .crumb a { color: inherit; }
  h1 { margin: 0 8px 0 0; font: 700 32px/1.1 var(--font-display); letter-spacing: -0.02em; }
  .tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--border-subtle); }
  .tabs button { height: 38px; padding: 0 18px; border: 0; border-bottom: 2px solid transparent; margin-bottom: -1px; background: none; color: var(--text-muted); font: 700 14px/1 var(--font-sans); letter-spacing: 0.05em; text-transform: uppercase; cursor: pointer; }
  .tabs button[aria-selected='true'] { color: var(--text-primary); border-bottom-color: var(--text-primary); }

  .grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); grid-auto-flow: dense; gap: 12px; }
  .grid > section { gap: 10px; }
  .wide { grid-column: 1 / -1; }
  .half { grid-column: span 2; }
  .tall { grid-column: span 2; grid-row: span 2; }
  .ph { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 14px; }
  .ph h2 { margin-right: auto; }
  .big { display: inline-flex; align-items: baseline; gap: 4px; font: 700 38px/1.05 var(--font-sans); letter-spacing: -0.03em; }
  .big small { color: var(--text-muted); font-size: 15px; font-weight: 600; letter-spacing: 0; }
  .mid { font: 700 24px/1.1 var(--font-sans); letter-spacing: -0.02em; }
  .flame { align-self: center; color: var(--accent-yours); display: inline-flex; }
  .date { color: var(--accent-yours); font: 700 26px/1.15 var(--font-display); }
  .stars { color: var(--accent-yours); }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { height: 28px; padding: 0 12px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13px/1 var(--font-sans); cursor: pointer; }
  .seg button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }

  .rank { display: grid; gap: 6px; margin: 0; padding: 0; list-style: none; }
  .rank li { display: grid; grid-template-columns: 22px auto minmax(0, 1fr) auto; align-items: center; gap: 12px; min-height: 44px; }
  .no { color: var(--accent-yours); font-weight: 700; font-size: 15px; }
  .rb { display: grid; gap: 1px; min-width: 0; }
  .rb a { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: inherit; font: 600 15px/1.3 var(--font-display); text-decoration: none; }
  .rb a:hover { text-decoration: underline; text-underline-offset: 3px; }
  .val { color: var(--text-muted); font-size: 14px; font-weight: 600; white-space: nowrap; }
  .face { display: block; width: 40px; height: 40px; border-radius: 50%; overflow: hidden; background: var(--bg-elevated); }
  .face img { width: 100%; height: 100%; object-fit: cover; display: block; }

  @media (max-width: 1100px) {
    .grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .half, .tall { grid-column: 1 / -1; grid-row: auto; }
  }
  @media (max-width: 520px) {
    .big { font-size: 30px; }
  }
</style>
