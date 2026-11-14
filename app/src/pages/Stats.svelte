<!-- Stats. One year at a time (?year=YYYY, default this year), from /me/stats/year and
     /me/stats/ratings. Panels with nothing behind them (people before credits are fetched,
     spend before any ticket prices) don't render, instead of showing empty boxes. -->
<script>
  import Poster from '../components/Poster.svelte'
  import Icon from '../components/Icon.svelte'
  import Columns from '../components/Columns.svelte'
  import BarList from '../components/BarList.svelte'
  import { api } from '../lib/api.js'
  import { library } from '../lib/library.svelte.js'
  import { route, setQuery } from '../lib/router.svelte.js'
  import { dayLabel } from '../lib/dates.js'

  const thisYear = new Date().getFullYear()
  const year = $derived(/^\d{4}$/.test(route.query.year ?? '') && +route.query.year <= thisYear ? +route.query.year : thisYear)

  let loading = $state(true)
  let error = $state('')
  let y = $state(null)
  let r = $state(null)

  $effect(() => {
    const yr = year
    const ctrl = new AbortController()
    loading = true
    error = ''
    Promise.all([api.statsYear(yr, ctrl.signal), api.statsRatings(`${yr}-01-01`, `${yr}-12-31`, ctrl.signal).catch(() => null)])
      .then(([a, b]) => { y = a; r = b })
      .catch((err) => { if (err.name !== 'AbortError') error = err.message })
      .finally(() => { if (!ctrl.signal.aborted) loading = false })
    return () => ctrl.abort()
  })

  const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
  const hours = (min) => Math.round((min ?? 0) / 6) / 10
  const months = $derived.by(() => {
    const by = new Map((y?.by_month ?? []).map((m) => [m.label, m]))
    return MONTHS.map((name, i) => {
      const m = by.get(`${year}-${String(i + 1).padStart(2, '0')}`)
      return { label: name, value: m?.watches ?? 0, title: `${name}: ${m?.watches ?? 0} watches, ${hours(m?.minutes)} h` }
    })
  })
  const nowMonth = $derived(year === thisYear ? new Date().getMonth() : -1)
  // Monday first; the API numbers weekdays from Sunday = 0.
  const weekdays = $derived.by(() => {
    const by = new Map((y?.by_weekday ?? []).map((d) => [String(d.weekday), d.watches]))
    return ['1', '2', '3', '4', '5', '6', '0'].map((d, i) => ({ label: ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'][i], value: by.get(d) ?? 0 }))
  })
  const hoursOfDay = $derived.by(() => {
    const by = new Map((y?.by_hour ?? []).map((h) => [Number(h.label), h.watches]))
    return Array.from({ length: 24 }, (_, h) => ({ label: String(h), value: by.get(h) ?? 0, title: `${String(h).padStart(2, '0')}:00–${String(h).padStart(2, '0')}:59: ${by.get(h) ?? 0}` }))
  })
  const timed = $derived(hoursOfDay.reduce((s, h) => s + h.value, 0))

  const region = new Intl.DisplayNames(undefined, { type: 'region' })
  const language = new Intl.DisplayNames(undefined, { type: 'language' })
  const nameOf = (fn, code) => { try { return fn.of(code) ?? code } catch { return code } }
  const taste = $derived({
    genres: (y?.genres ?? []).slice(0, 8).map((g) => ({ label: g.genre, value: g.titles })),
    decades: [...(y?.decades ?? [])].sort((a, b) => a.decade.localeCompare(b.decade)).map((d) => ({ label: d.decade, value: d.titles })),
    countries: (y?.countries ?? []).slice(0, 6).map((c) => ({ label: nameOf(region, c.country), value: c.titles })),
    languages: (y?.languages ?? []).slice(0, 6).map((l) => ({ label: nameOf(language, l.language), value: l.titles })),
  })
  const VENUE = { streaming: 'Streaming', theater: 'Cinema', physical: 'Physical media', broadcast: 'TV broadcast', other: 'Elsewhere', unspecified: 'Not recorded' }
  const venues = $derived((y?.by_venue ?? []).map((v) => ({ label: VENUE[v.label] ?? v.label, value: v.watches, note: `· ${hours(v.minutes)} h` })))
  const asBar = (f) => ({ label: f.label, value: f.watches, note: `· ${hours(f.minutes)} h`, logo: f.logo_url, mono: f.logo_display === 'mono' })
  const HOME = new Set(['disc', 'label'])
  const formats = $derived((y?.formats ?? []).filter((f) => !HOME.has(f.category)).map(asBar))
  const discs = $derived((y?.formats ?? []).filter((f) => HOME.has(f.category)).map(asBar))
  const people = $derived([
    ['Actors', y?.people?.cast ?? []], ['Directors', y?.people?.directors ?? []], ['Writers', y?.people?.writers ?? []],
  ].filter(([, list]) => list.length))
  const money = (minor, currency) => {
    try {
      const f = new Intl.NumberFormat(undefined, { style: 'currency', currency })
      return f.format(minor / 10 ** f.resolvedOptions().maximumFractionDigits)
    } catch { return `${minor} ${currency}` }
  }

  // Ratings: your distribution, and how far you sit from each crowd.
  const dist = $derived((r?.distribution ?? []).map((b) => ({ label: `${Math.floor(b.stars) || ''}${b.rating % 2 ? '½' : ''}`, value: b.watches, title: `${b.stars} ★: ${b.watches}` })))
  const CROWD = [['tmdb', 'TMDB'], ['imdb', 'IMDb'], ['rotten_tomatoes', 'Rotten Tomatoes']]
  const vs = $derived(CROWD.map(([k, name]) => ({ name, ...(r?.vs_crowd?.[k] ?? { compared: 0 }) })))
  // Per title: you vs the crowd on one 0–10 scale; biggest disagreements first.
  const compare = $derived(
    (r?.titles ?? []).map((t) => {
      const crowd = [t.tmdb_rating, t.imdb_rating, t.rt_rating].filter((v) => v != null)
      const avg = crowd.length ? crowd.reduce((a, b) => a + b, 0) / crowd.length : null
      return { ...t, crowd: avg, gap: avg == null ? 0 : t.user_rating - avg }
    }).filter((t) => t.crowd != null).sort((a, b) => Math.abs(b.gap) - Math.abs(a.gap)).slice(0, 8)
  )
  const pct = (v) => `${(v / 10) * 100}%`

  const totals = $derived(y?.totals ?? null)
  const poster = (t) => library.get(t.tmdb_kind, t.tmdb_id)?.poster_path ?? null
</script>

<div class="page">
  <header class="head">
    <h1>Stats</h1>
    <div class="years" role="group" aria-label="Year">
      <button aria-label="Previous year" onclick={() => setQuery({ year: year - 1 })}>‹</button>
      <span class="yr num" aria-live="polite">{year}</span>
      <button aria-label="Next year" disabled={year >= thisYear} onclick={() => setQuery({ year: year + 1 === thisYear ? '' : year + 1 })}>›</button>
    </div>
  </header>

  {#if error}
    <p class="alert" role="alert">Couldn’t load your stats: {error}</p>
  {:else if loading && !y}
    <div class="skeleton" style="height:480px" aria-busy="true" aria-label="Loading"></div>
  {:else if !totals?.watches}
    <section class="empty">
      <h2>Nothing logged in {year}</h2>
      <p>Your stats fill in as you log watches. Try another year, or log something from the charts.</p>
    </section>
  {:else}
    <div class="stack" aria-busy={loading}>
      <section class="kpis" aria-label="{year} at a glance">
        <div class="kpi"><span class="k-label">Watches</span><b class="num">{totals.watches}</b><span class="k-sub">{totals.movie_watches} films · {totals.episode_watches} episodes</span></div>
        <div class="kpi"><span class="k-label">Hours</span><b class="num">{Math.round(totals.hours)}</b><span class="k-sub">{(totals.hours / Math.max(1, totals.days)).toFixed(1)} h on a day you watched</span></div>
        <div class="kpi"><span class="k-label">Titles</span><b class="num">{totals.distinct_titles}</b><span class="k-sub">{totals.rewatches} rewatch{totals.rewatches === 1 ? '' : 'es'} · {totals.liked} liked</span></div>
        <div class="kpi"><span class="k-label">Days with a watch</span><b class="num">{y.streaks?.days_watched ?? totals.days}</b><span class="k-sub">Longest streak {y.streaks?.longest ?? 0} day{y.streaks?.longest === 1 ? '' : 's'}{#if y.streaks?.longest_ended}, ended {dayLabel(y.streaks.longest_ended)}{/if}</span></div>
      </section>

      <section class="panel pad">
        <div class="ph"><h2>Your {year}</h2><span class="hint">Watches per month · hover for hours</span></div>
        <Columns items={months} height={150} label="Watches per month in {year}" highlight={nowMonth} />
      </section>

      <div class="two">
        <section class="panel pad">
          <div class="ph"><h2>Which days</h2></div>
          <Columns items={weekdays} height={110} label="Watches by weekday" />
        </section>
        <section class="panel pad">
          <div class="ph"><h2>What time</h2><span class="hint">{timed} of {totals.watches} logged to the minute</span></div>
          <Columns items={hoursOfDay} height={110} every={3} label="Watches by the hour they ended, local time" />
        </section>
      </div>

      {#if r?.rated}
        <section class="panel pad">
          <div class="ph"><h2>How you rate</h2><span class="hint">{r.rated} rated watch{r.rated === 1 ? '' : 'es'} · average <b class="avg">{(r.average / 2).toFixed(1)} ★</b></span></div>
          <div class="rate-grid">
            <div>
              <Columns items={dist} height={110} label="Your ratings, by half-star" />
              <ul class="crowd">
                {#each vs as c}
                  <li>
                    <span class="cn">{c.name}</span>
                    {#if c.compared}
                      <b class="num" class:up={c.average_gap > 0} class:down={c.average_gap < 0}>{c.average_gap > 0 ? '+' : ''}{(c.average_gap ?? 0).toFixed(1)}</b>
                      <span class="cs">higher than them on {c.above_the_crowd} of {c.compared}</span>
                    {:else}
                      <span class="cs wide">nothing to compare yet</span>
                    {/if}
                  </li>
                {/each}
              </ul>
            </div>
            {#if compare.length}
              <div>
                <p class="sub-h">You against the crowd <span class="hint">out of 10 · biggest gaps first</span></p>
                <ul class="dumbbell">
                  {#each compare as t (t.tmdb_kind + t.tmdb_id)}
                    <li>
                      <a class="dt" href="#/title/{t.tmdb_kind}/{t.tmdb_id}">{t.title}</a>
                      <span class="scale" role="img" aria-label="You {t.user_rating}, the crowd {t.crowd.toFixed(1)} out of 10">
                        <span class="bar" style:left={pct(Math.min(t.user_rating, t.crowd))} style:width={pct(Math.abs(t.user_rating - t.crowd))}></span>
                        <span class="dot them" style:left={pct(t.crowd)} title="Crowd {t.crowd.toFixed(1)}"></span>
                        <span class="dot you" style:left={pct(t.user_rating)} title="You {t.user_rating}"></span>
                      </span>
                      <span class="gap num" class:up={t.gap > 0} class:down={t.gap < 0}>{t.gap > 0 ? '+' : ''}{t.gap.toFixed(1)}</span>
                    </li>
                  {/each}
                </ul>
                <p class="legend"><span class="dot you"></span>You <span class="dot them"></span>Crowd (TMDB, IMDb and RT averaged)</p>
              </div>
            {/if}
          </div>
        </section>
      {/if}

      <div class="two">
        <section class="panel pad">
          <div class="ph"><h2>Your taste</h2><span class="hint">Titles, by genre</span></div>
          <BarList items={taste.genres} />
        </section>
        <section class="panel pad">
          <div class="ph"><h2>Most watched</h2></div>
          <ol class="most">
            {#each (y.most_watched ?? []).slice(0, 5) as t, i (t.tmdb_kind + t.tmdb_id)}
              <li>
                <span class="rk num">{i + 1}</span>
                <a class="mt" href="#/title/{t.tmdb_kind}/{t.tmdb_id}">
                  <Poster path={poster(t)} seed={t.tmdb_id} title={t.title} width={36} radius={4} size="w92" />
                  <span class="mtx"><span class="nm">{t.title}</span><span class="hint">{t.watches} watch{t.watches === 1 ? '' : 'es'} · {hours(t.minutes)} h{#if t.genres?.length}{' · '}{t.genres.slice(0, 2).join(', ')}{/if}</span></span>
                </a>
              </li>
            {/each}
          </ol>
        </section>
      </div>

      <div class="three">
        {#if taste.decades.length}<section class="panel pad"><div class="ph"><h2>Decades</h2></div><BarList items={taste.decades} /></section>{/if}
        {#if taste.countries.length}<section class="panel pad"><div class="ph"><h2>Countries</h2></div><BarList items={taste.countries} /></section>{/if}
        {#if taste.languages.length}<section class="panel pad"><div class="ph"><h2>Languages</h2></div><BarList items={taste.languages} /></section>{/if}
      </div>

      <div class="two">
        <section class="panel pad">
          <div class="ph"><h2>Where you watched</h2><span class="hint">Watches · hours</span></div>
          <BarList items={venues} />
          {#if formats.length}<p class="sub-h">Formats</p><BarList items={formats} />{/if}
          {#if discs.length}<p class="sub-h">Discs and labels</p><BarList items={discs} />{/if}
        </section>
        {#if y.spend?.length}
          <section class="panel pad">
            <div class="ph"><h2>Spend</h2><span class="hint">Never converted between currencies</span></div>
            <ul class="spend">
              {#each y.spend as c}
                <li><b class="num">{money(c.total_minor, c.currency)}</b><span class="hint">{c.tickets} ticket{c.tickets === 1 ? '' : 's'} · {c.watches} watch{c.watches === 1 ? '' : 'es'}{#if c.concessions_minor}{' · '}{money(c.concessions_minor, c.currency)} on snacks{/if}</span></li>
              {/each}
            </ul>
          </section>
        {/if}
      </div>

      {#if people.length}
        <section class="panel pad">
          <div class="ph"><h2>The people behind it</h2><span class="hint">By titles of yours they worked on</span></div>
          <div class="people">
            {#each people as [role, list]}
              <div>
                <p class="sub-h">{role}</p>
                <ol class="ppl">
                  {#each list.slice(0, 5) as p (p.person_id)}
                    <li>
                      {#if p.profile_path}<img src="https://image.tmdb.org/t/p/w92{p.profile_path}" alt="" loading="lazy" />{:else}<span class="ph-face" aria-hidden="true"><Icon name="flag" size={12} /></span>{/if}
                      <span class="pn">{p.name}</span><span class="hint num">{p.titles}</span>
                    </li>
                  {/each}
                </ol>
              </div>
            {/each}
          </div>
        </section>
      {/if}
    </div>
  {/if}
</div>

<style>
  .page { padding: 20px var(--gutter) 48px; }
  .head { display: flex; align-items: center; gap: 18px; margin-bottom: 16px; }
  h1 { margin: 0; font: 700 32px/1.1 var(--font-display); letter-spacing: -0.02em; }
  .years { display: inline-flex; align-items: center; gap: 4px; padding: 2px; border-radius: 18px; background: var(--bg-elevated); }
  .years button { width: 30px; height: 30px; border: 0; border-radius: 50%; background: none; color: var(--text-primary); font-size: 18px; cursor: pointer; }
  .years button:hover:not(:disabled) { background: var(--bg-surface); }
  .years button:disabled { opacity: 0.35; cursor: default; }
  .yr { min-width: 48px; text-align: center; font: 700 18px/1 var(--font-display); }
  .stack { display: grid; gap: 16px; }

  .kpis { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; }
  .kpi { display: grid; gap: 4px; padding: 16px 18px; border-radius: 16px; background: var(--bg-surface); }
  .k-label { color: var(--text-muted); font-size: 13px; font-weight: 600; }
  .kpi b { font: 700 32px/1.05 var(--font-sans); letter-spacing: -0.03em; }
  .k-sub { color: var(--text-muted); font-size: 12.5px; }

  .ph { display: flex; flex-wrap: wrap; align-items: baseline; gap: 6px 12px; margin-bottom: 12px; }
  .ph h2 { margin: 0; font: 700 19px/1.2 var(--font-display); }
  .avg { color: var(--accent-yours); }
  .sub-h { margin: 14px 0 8px; font: 700 15px/1.2 var(--font-display); }
  .sub-h .hint { font: 400 12.5px/1.3 var(--font-sans); }
  .two { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
  .three { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; }
  .three:empty { display: none; }

  .rate-grid { display: grid; grid-template-columns: 1fr 1.3fr; gap: 28px; }
  .crowd { display: grid; gap: 6px; margin: 16px 0 0; padding: 0; list-style: none; }
  .crowd li { display: grid; grid-template-columns: 130px 56px 1fr; align-items: baseline; gap: 10px; }
  .cn { font-weight: 600; }
  .crowd b { font-size: 17px; }
  .cs { color: var(--text-muted); font-size: 13px; }
  .cs.wide { grid-column: 2 / -1; }
  .up { color: var(--delta-up); }
  .down { color: var(--delta-down); }
  .dumbbell { display: grid; gap: 10px; margin: 0; padding: 0; list-style: none; }
  .dumbbell li { display: grid; grid-template-columns: minmax(90px, 32%) 1fr 44px; align-items: center; gap: 12px; }
  .dt { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 600 14.5px/1.2 var(--font-display); text-decoration: none; }
  .dt:hover { text-decoration: underline; }
  .scale { position: relative; height: 16px; background: repeating-linear-gradient(90deg, var(--border-subtle) 0 1px, transparent 1px 10%); box-shadow: inset 0 -1px 0 var(--border-subtle); }
  .bar { position: absolute; top: 7px; height: 2px; background: var(--border-strong); }
  .dot { position: absolute; top: 3px; width: 10px; height: 10px; margin-left: -5px; border-radius: 50%; }
  .dot.you { background: var(--accent-yours); z-index: 1; }
  .dot.them { background: var(--bg-surface); box-shadow: inset 0 0 0 2px var(--text-muted); }
  .gap { text-align: right; font-weight: 700; font-size: 13.5px; }
  .legend { display: flex; align-items: center; gap: 6px; margin: 10px 0 0; color: var(--text-muted); font-size: 12.5px; }
  .legend .dot { position: static; margin: 0 2px 0 8px; }
  .legend .dot:first-child { margin-left: 0; }

  .most { display: grid; gap: 10px; margin: 0; padding: 0; list-style: none; }
  .most li { display: flex; align-items: center; gap: 12px; }
  .rk { width: 18px; color: var(--text-muted); font: 700 18px/1 var(--font-display); text-align: right; }
  .mt { display: flex; align-items: center; gap: 12px; min-width: 0; color: inherit; text-decoration: none; }
  .mtx { display: grid; min-width: 0; }
  .nm { font: 600 15.5px/1.25 var(--font-display); }
  .mt:hover .nm { text-decoration: underline; text-underline-offset: 3px; }

  .spend { display: grid; gap: 10px; margin: 0; padding: 0; list-style: none; }
  .spend li { display: grid; }
  .spend b { font: 700 26px/1.1 var(--font-sans); }
  .people { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 24px; }
  .ppl { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }
  .ppl li { display: flex; align-items: center; gap: 10px; }
  .ppl img, .ph-face { width: 32px; height: 32px; border-radius: 50%; object-fit: cover; background: var(--bg-elevated); display: grid; place-items: center; color: var(--text-muted); }
  .pn { flex: 1; font-weight: 600; }

  .empty { padding: 40px 0; }
  .empty h2 { margin: 0 0 6px; font: 700 24px/1.2 var(--font-display); }
  .empty p { color: var(--text-muted); max-width: 60ch; }

  @media (max-width: 960px) {
    .kpis { grid-template-columns: 1fr 1fr; }
    .two, .rate-grid { grid-template-columns: 1fr; }
    .three, .people { grid-template-columns: 1fr; }
  }
  @media (max-width: 480px) { .crowd li { grid-template-columns: 1fr auto; } .cs { grid-column: 1 / -1; } }
</style>
