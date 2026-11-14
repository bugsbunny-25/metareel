<!-- Diary. Every watch, newest local day first, grouped by month and day (watched_on, never
     ended_at). Above it: this year's totals and a year heatmap; clicking a day filters to it.
     Filters live in the URL: ?kind=movie|tv&rated=1&day=YYYY-MM-DD. -->
<script>
  import Poster from '../components/Poster.svelte'
  import Icon from '../components/Icon.svelte'
  import FormatBadge from '../components/FormatBadge.svelte'
  import YearHeatmap from '../components/YearHeatmap.svelte'
  import { api } from '../lib/api.js'
  import { library } from '../lib/library.svelte.js'
  import { route, setQuery } from '../lib/router.svelte.js'
  import { stars, starsLabel } from '../lib/rows.js'
  import { localDay, addDays, monthLabel, weekdayLabel, dayLabel, clock } from '../lib/dates.js'
  import { toast } from '../lib/toast.svelte.js'

  let { onlog, onconfirm } = $props()

  const VENUE = { streaming: 'Streaming', theater: 'Cinema', physical: 'Physical media', broadcast: 'TV broadcast', other: 'Elsewhere' }
  const kind = $derived(route.query.kind === 'movie' || route.query.kind === 'tv' ? route.query.kind : '')
  const rated = $derived(route.query.rated === '1')
  const day = $derived(/^\d{4}-\d{2}-\d{2}$/.test(route.query.day ?? '') ? route.query.day : '')

  // Year overview: independent of the filters.
  const today = localDay()
  const yearStart = `${today.slice(0, 4)}-01-01`
  let summary = $state(null)
  let calendar = $state(null)
  $effect(() => {
    const ctrl = new AbortController()
    Promise.all([api.statsSummary(yearStart, today, ctrl.signal), api.statsCalendar(addDays(today, -370), today, ctrl.signal)])
      .then(([s, c]) => { summary = s; calendar = c })
      .catch(() => {}) // the diary still works without the overview
    return () => ctrl.abort()
  })

  // Entries, cursor-paged.
  let entries = $state([])
  let cursor = $state(null)
  let loading = $state(true)
  let more = $state(false)
  let error = $state('')

  $effect(() => {
    const params = { kind, rated, from: day || undefined, to: day || undefined }
    const ctrl = new AbortController()
    loading = true
    error = ''
    api.diary(params, ctrl.signal)
      .then((r) => { entries = r.watches ?? []; cursor = r.next_cursor ?? null })
      .catch((err) => { if (err.name !== 'AbortError') error = err.message })
      .finally(() => { if (!ctrl.signal.aborted) loading = false })
    return () => ctrl.abort()
  })

  async function loadMore() {
    more = true
    try {
      const r = await api.diary({ kind, rated, from: day || undefined, to: day || undefined, cursor })
      entries = [...entries, ...(r.watches ?? [])]
      cursor = r.next_cursor ?? null
    } catch (err) {
      toast(err.message, 'error')
    } finally {
      more = false
    }
  }

  // Month → day → watches, preserving the server's newest-first order.
  const groups = $derived.by(() => {
    const months = []
    for (const w of entries) {
      const mk = w.watched_on.slice(0, 7)
      let m = months.at(-1)
      if (!m || m.key !== mk) months.push((m = { key: mk, label: monthLabel(w.watched_on), days: [] }))
      let d = m.days.at(-1)
      if (!d || d.day !== w.watched_on) m.days.push((d = { day: w.watched_on, watches: [] }))
      d.watches.push(w)
    }
    return months
  })

  const totals = $derived(summary?.totals ?? null)
  const streaks = $derived(calendar?.streaks ?? null)
  const titleHref = (w) => `#/title/${w.tmdb_kind}/${w.tmdb_id}`

  function edit(w) {
    onlog({
      kind: w.tmdb_kind, id: w.tmdb_id, name: w.title, poster: w.poster_path, watch: w,
      onsaved: (saved) => {
        // A changed time can move the watch to another day: re-sort locally by (day, time).
        entries = entries.map((x) => (x.id === saved.id ? { ...x, ...saved } : x))
          .sort((a, b) => b.watched_on.localeCompare(a.watched_on) || b.ended_at.localeCompare(a.ended_at) || b.id - a.id)
      },
    })
  }

  function remove(w) {
    onconfirm({
      title: 'Delete this watch?',
      body: `${w.title}${w.subject_kind === 'episode' ? ` S${w.season_number} E${w.episode_number}` : ''} on ${dayLabel(w.watched_on)}. This can’t be undone; the title’s rating and watch count are recalculated.`,
      confirm: 'Delete watch',
      danger: true,
      run: async () => {
        await api.deleteWatch(w.id)
        entries = entries.filter((x) => x.id !== w.id)
        await library.refresh(w.tmdb_kind, w.tmdb_id).catch(() => {})
        toast('Watch deleted.')
      },
    })
  }
</script>

<div class="page">
  <header class="head">
    <h1>Diary</h1>
    <div class="seg" role="group" aria-label="What to show">
      <button aria-pressed={kind === ''} onclick={() => setQuery({ kind: '' })}>Everything</button>
      <button aria-pressed={kind === 'movie'} onclick={() => setQuery({ kind: 'movie' })}>Films</button>
      <button aria-pressed={kind === 'tv'} onclick={() => setQuery({ kind: 'tv' })}>Shows</button>
    </div>
    <button class="toggle" aria-pressed={rated} onclick={() => setQuery({ rated: rated ? '' : '1' })}>Rated only</button>
    {#if day}<span class="chip">{dayLabel(day)} <button aria-label="Show every day" onclick={() => setQuery({ day: '' })}>×</button></span>{/if}
  </header>

  {#if totals}
    <section class="kpis" aria-label="This year">
      <div class="kpi"><span class="k-label">Watches in {today.slice(0, 4)}</span><b class="num">{totals.watches}</b><span class="k-sub">{totals.movie_watches} films · {totals.episode_watches} episodes</span></div>
      <div class="kpi"><span class="k-label">Hours</span><b class="num">{Math.round(totals.hours ?? 0)}</b><span class="k-sub">over {totals.days} day{totals.days === 1 ? '' : 's'} with a watch</span></div>
      <div class="kpi"><span class="k-label">Titles</span><b class="num">{totals.distinct_titles}</b><span class="k-sub">{totals.rewatches} rewatch{totals.rewatches === 1 ? '' : 'es'} · {totals.liked} liked</span></div>
      <div class="kpi"><span class="k-label">Current streak</span><b class="num">{streaks?.current ?? 0}<span class="of">{' '}day{streaks?.current === 1 ? '' : 's'}</span></b><span class="k-sub">Longest {streaks?.longest ?? 0} day{streaks?.longest === 1 ? '' : 's'}</span></div>
    </section>
  {/if}

  {#if calendar}
    <section class="panel pad heat">
      <h2>The last year</h2>
      <!-- Picking a day shows everything from it, so the other filters are cleared. -->
      <YearHeatmap days={calendar.days ?? []} to={today} onpick={(d) => setQuery({ day: d, kind: '', rated: '' })} />
    </section>
  {/if}

  {#if error}
    <p class="alert" role="alert">Couldn’t load your diary: {error}</p>
  {:else if loading}
    <div class="skeleton" style="height:320px;margin-top:24px" aria-busy="true" aria-label="Loading"></div>
  {:else if !entries.length}
    <section class="empty">
      <h2>{day || kind || rated ? 'Nothing matches' : 'Your diary is empty'}</h2>
      <p>{day || kind || rated ? 'Try another filter.' : 'Log a watch from any title, chart row or the Home page, and it appears here under the day you watched it.'}</p>
    </section>
  {:else}
    {#each groups as m (m.key)}
      <section class="month" aria-labelledby="m-{m.key}">
        <h2 id="m-{m.key}" class="month-h">{m.label}</h2>
        {#each m.days as d (d.day)}
          <div class="day">
            <div class="date" aria-hidden="true"><b class="num">{+d.day.slice(8)}</b><span>{weekdayLabel(d.day)}</span></div>
            <ul class="watches" aria-label={dayLabel(d.day)}>
              {#each d.watches as w (w.id)}
                <li class="watch">
                  <a class="art" href={titleHref(w)} tabindex="-1" aria-hidden="true"><Poster path={w.poster_path} seed={w.tmdb_id} title={w.title ?? ''} width={44} radius={5} size="w185" /></a>
                  <div class="body">
                    <a class="name" href={titleHref(w)}>{w.title}</a>
                    <p class="meta">
                      {#if w.subject_kind === 'episode'}Season {w.season_number} · Episode {w.episode_number}{:else}Film{#if w.release_date}{' · '}{w.release_date.slice(0, 4)}{/if}{/if}
                      {#if w.time_precision === 'minute'}{' · '}<time datetime={w.ended_at}>{clock(w.ended_at)}</time>{/if}
                    </p>
                    {#if w.is_rewatch || w.venue_kind || w.tags?.length || w.formats?.length}
                      <p class="pills">
                        {#if w.is_rewatch}<span class="pill rewatch"><Icon name="repeat" size={12} />Watch {w.watch_number}</span>{/if}
                        {#if w.venue_kind}<span class="pill">{VENUE[w.venue_kind] ?? w.venue_kind}</span>{/if}
                        {#each w.formats ?? [] as f (f.slug)}<FormatBadge format={f} />{/each}
                        {#each w.tags ?? [] as t}<span class="pill tag">#{t}</span>{/each}
                      </p>
                    {/if}
                    {#if w.notes}<p class="notes">{w.notes}</p>{/if}
                  </div>
                  <div class="right">
                    {#if w.rating}<span class="stars" aria-label={starsLabel(w.rating)}>{stars(w.rating)}</span>{/if}
                    {#if w.liked}<span class="liked" aria-label="Liked"><Icon name="heart-fill" size={15} /></span>{/if}
                    <button class="icon" aria-label="Edit this watch of {w.title}" title="Edit" onclick={() => edit(w)}><Icon name="pencil" size={15} /></button>
                    <button class="icon" aria-label="Delete this watch of {w.title}" title="Delete" onclick={() => remove(w)}><Icon name="trash" size={15} /></button>
                  </div>
                </li>
              {/each}
            </ul>
          </div>
        {/each}
      </section>
    {/each}
    {#if cursor}
      <p class="more"><button class="btn-secondary" disabled={more} onclick={loadMore}>{more ? 'Loading…' : 'Show older watches'}</button></p>
    {/if}
  {/if}
</div>

<style>
  .page { padding: 20px var(--gutter) 48px; }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: 12px 14px; margin-bottom: 16px; }
  h1 { margin: 0 8px 0 0; font: 700 32px/1.1 var(--font-display); letter-spacing: -0.02em; }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { height: 30px; padding: 0 14px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .seg button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }
  .toggle { height: 32px; padding: 0 14px; border: 0; border-radius: 16px; background: var(--bg-elevated); color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .toggle[aria-pressed='true'] { background: var(--accent-yours); color: var(--bg-canvas); }
  .chip { display: inline-flex; align-items: center; gap: 6px; height: 32px; padding: 0 6px 0 14px; border-radius: 16px; background: var(--bg-surface); font-weight: 600; }
  .chip button { width: 22px; height: 22px; border: 0; border-radius: 50%; background: var(--bg-elevated); color: var(--text-primary); cursor: pointer; }

  .kpis { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; }
  .kpi { display: grid; gap: 4px; padding: 16px 18px; border-radius: 16px; background: var(--bg-surface); }
  .k-label { color: var(--text-muted); font-size: 13px; font-weight: 600; }
  .kpi b { font: 700 32px/1.05 var(--font-sans); letter-spacing: -0.03em; }
  .of { color: var(--text-muted); font-size: 15px; font-weight: 600; letter-spacing: 0; }
  .k-sub { color: var(--text-muted); font-size: 12.5px; }
  .heat { margin-top: 12px; }

  .month { margin-top: 32px; }
  .month-h { margin: 0 0 6px; padding-bottom: 8px; border-bottom: 2px solid var(--text-primary); font: 700 24px/1.2 var(--font-display); letter-spacing: -0.01em; }
  .day { display: grid; grid-template-columns: 64px 1fr; gap: 12px; padding: 14px 0; box-shadow: inset 0 -1px 0 var(--border-subtle); }
  .date { display: grid; align-content: start; justify-items: center; padding-top: 2px; }
  .date b { font: 700 30px/1 var(--font-display); }
  .date span { color: var(--text-muted); font-size: 12.5px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; }
  .watches { display: grid; gap: 10px; margin: 0; padding: 0; list-style: none; }
  .watch { display: grid; grid-template-columns: auto 1fr auto; gap: 14px; align-items: start; padding: 8px 10px; border-radius: 14px; }
  .watch:hover { background: var(--bg-surface); }
  .art { display: block; }
  .body { min-width: 0; display: grid; gap: 4px; }
  .name { font: 600 17px/1.2 var(--font-display); text-decoration: none; }
  .name:hover { text-decoration: underline; text-underline-offset: 3px; }
  .meta { margin: 0; color: var(--text-muted); font-size: 13px; }
  .pills { display: flex; flex-wrap: wrap; gap: 6px; margin: 2px 0 0; }
  .pill { display: inline-flex; align-items: center; gap: 4px; }
  .pill.tag { color: var(--text-primary); }
  .pill.rewatch { color: var(--accent-yours); }
  .notes { margin: 2px 0 0; max-width: 70ch; color: var(--text-muted); font: italic 400 15px/22px var(--font-display); display: -webkit-box; -webkit-line-clamp: 3; line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; }
  .right { display: flex; align-items: center; gap: 8px; }
  .stars { color: var(--accent-yours); font-size: 15px; letter-spacing: 0.04em; }
  .liked { color: var(--delta-down); display: grid; }
  .icon { display: grid; place-items: center; width: 32px; height: 32px; border: 0; border-radius: 50%; background: none; color: var(--text-muted); cursor: pointer; opacity: 0.0; transition: opacity 0.12s ease; }
  .watch:hover .icon, .icon:focus-visible { opacity: 1; }
  .icon:hover { background: var(--bg-elevated); color: var(--text-primary); }
  .more { margin: 24px 0 0; text-align: center; }
  .empty { padding: 40px 0; }
  .empty h2 { margin: 0 0 6px; font: 700 24px/1.2 var(--font-display); }
  .empty p { color: var(--text-muted); max-width: 60ch; }

  @media (hover: none) { .icon { opacity: 1; } }
  @media (max-width: 900px) { .kpis { grid-template-columns: 1fr 1fr; } }
  @media (max-width: 560px) {
    .day { grid-template-columns: 44px 1fr; }
    .watch { grid-template-columns: auto 1fr; }
    .right { grid-column: 2; flex-wrap: wrap; }
    .icon { opacity: 1; }
  }
</style>
