<!-- Netflix's own Top 10 (#/discover?tab=netflix&nf=country|alltime&week=YYYY-MM-DD): the weekly
     global lists (four categories, with hours and views), the weekly list for the country in the
     top bar (films and shows, ranks only: that is all Netflix publishes per country) and the
     all-time most popular (views in the first 91 days). Each row carries the TMDB poster of the
     title it's mapped to; unmapped titles get a placeholder. -->
<script>
  import Poster from '../../components/Poster.svelte'
  import Icon from '../../components/Icon.svelte'
  import { api } from '../../lib/api.js'
  import { library } from '../../lib/library.svelte.js'
  import { route, setQuery } from '../../lib/router.svelte.js'
  import { inCountry } from '../../lib/scope.svelte.js'
  import { compact } from '../../lib/viz.js'
  import { addDays, dayLabel } from '../../lib/dates.js'
  import { tmdbKind } from '../../lib/rows.js'
  import { toast } from '../../lib/toast.svelte.js'

  let { country } = $props()

  const nf = $derived(['country', 'alltime'].includes(route.query.nf) ? route.query.nf : 'global')
  const week = $derived(/^\d{4}-\d{2}-\d{2}$/.test(route.query.week ?? '') ? route.query.week : '')

  let data = $state(null)
  let error = $state('')
  let latest = $state({}) // view → latest week, to stop "next" there
  $effect(() => {
    const view = nf, w = week, cc = country
    const ctrl = new AbortController()
    data = null
    error = ''
    const req = view === 'alltime' ? api.netflixMostPopular(ctrl.signal) : api.netflixWeek(view === 'country' ? cc : '', w || undefined, ctrl.signal)
    req.then((d) => {
      data = d
      if (view !== 'alltime' && !w && d.week) latest = { ...latest, [`${view}:${cc}`]: d.week }
    }).catch((err) => {
      if (err.name === 'AbortError') return
      error = err.status === 404
        ? view === 'country' ? `Netflix publishes no Top 10 for ${inCountry(cc)} that metareel has${w ? ' for that week' : ''}.` : 'Netflix has no Top 10 for that week.'
        : err.message
    })
    return () => ctrl.abort()
  })

  const items = $derived(data?.items ?? [])
  const cats = $derived.by(() => {
    const m = new Map()
    for (const it of items) {
      if (!m.has(it.category)) m.set(it.category, [])
      m.get(it.category).push(it)
    }
    for (const list of m.values()) list.sort((a, b) => a.rank - b.rank)
    return [...m]
  })
  const kindOf = (it) => tmdbKind(it.kind)
  const name = (it) => (it.season_title && it.season_title !== it.show_title ? `${it.show_title}: ${it.season_title}` : it.show_title)
  const views = (it) => it.views ?? it.views_first_91_days ?? null

  const fresh = $derived(items.filter((it) => it.cumulative_weeks === 1).length)
  const longest = $derived(items.filter((it) => it.cumulative_weeks).reduce((a, b) => (!a || b.cumulative_weeks > a.cumulative_weeks ? b : a), null))
  const biggest = $derived(items.filter((it) => views(it)).reduce((a, b) => (!a || views(b) > views(a) ? b : a), null))
  const hours = $derived(items.reduce((n, it) => n + (it.hours_viewed ?? it.hours_viewed_first_91_days ?? 0), 0))
  const onList = $derived(items.filter((it) => it.tmdb_id && library.get(kindOf(it), it.tmdb_id)?.status === 'watchlist').length)

  const latestWeek = $derived(latest[`${nf}:${country}`] ?? '')
  const shownWeek = $derived(data?.week ?? week)
  const weekRange = (w) => `${dayLabel(addDays(w, -6))} – ${dayLabel(w)}`

  async function toggle(it) {
    try {
      const on = await library.toggleWatchlist(kindOf(it), it.tmdb_id)
      toast(on ? `Added ${it.show_title} to your watchlist.` : `Removed ${it.show_title} from your watchlist.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }
  const pick = (patch) => setQuery(patch, { replace: true })
</script>

<div class="d-filters">
  <div class="d-seg" role="group" aria-label="List">
    <button aria-pressed={nf === 'global'} onclick={() => pick({ nf: '', week: '' })}>Global this week</button>
    <button aria-pressed={nf === 'country'} onclick={() => pick({ nf: 'country', week: '' })}>{inCountry(country)}</button>
    <button aria-pressed={nf === 'alltime'} onclick={() => pick({ nf: 'alltime', week: '' })}>All-time most popular</button>
  </div>
  {#if nf !== 'alltime' && shownWeek}
    <div class="weeknav" role="group" aria-label="Week">
      <button class="nav" aria-label="Previous week" onclick={() => pick({ week: addDays(shownWeek, -7) })}>‹</button>
      <span class="num">Week of {weekRange(shownWeek)}</span>
      <button class="nav" aria-label="Next week" disabled={!week || (latestWeek && shownWeek >= latestWeek)} onclick={() => pick({ week: latestWeek && addDays(shownWeek, 7) >= latestWeek ? '' : addDays(shownWeek, 7) })}>›</button>
      {#if week}<button class="d-link" onclick={() => pick({ week: '' })}>Latest</button>{/if}
    </div>
  {/if}
</div>

{#if error}
  <section class="d-empty"><h2>Nothing to show</h2><p>{error}</p>{#if week}<p><button class="d-link" onclick={() => pick({ week: '' })}>Back to the latest week</button></p>{/if}</section>
{:else if !data}
  <div class="skeleton" style="height:520px" aria-busy="true" aria-label="Loading"></div>
{:else}
  <section class="d-kpis" aria-label="Netflix Top 10 at a glance">
    {#if nf === 'alltime'}
      <div class="d-kpi"><span class="k-label">Most watched ever</span><b class="small" title={biggest ? name(biggest) : ''}>{biggest ? name(biggest) : '–'}</b><span class="k-sub">{biggest ? `${compact(views(biggest))} views in its first 91 days` : ''}</span></div>
      <div class="d-kpi"><span class="k-label">Lists</span><b class="num">{cats.length}</b><span class="k-sub">{items.length} titles in all</span></div>
      <div class="d-kpi"><span class="k-label">Hours viewed</span><b class="num">{compact(hours)}</b><span class="k-sub">by these titles in their first 91 days</span></div>
    {:else}
      <div class="d-kpi"><span class="k-label">New this week</span><b class="num">{fresh}</b><span class="k-sub">of {items.length} entries</span></div>
      <div class="d-kpi"><span class="k-label">Longest run</span><b class="small" title={longest ? name(longest) : ''}>{longest ? name(longest) : '–'}</b><span class="k-sub">{longest ? `${longest.cumulative_weeks} weeks in the Top 10` : ''}</span></div>
      {#if nf === 'global'}
        <div class="d-kpi"><span class="k-label">Biggest this week</span><b class="small" title={biggest ? name(biggest) : ''}>{biggest ? name(biggest) : '–'}</b><span class="k-sub">{biggest ? `${compact(views(biggest))} views` : ''}</span></div>
      {:else}
        <div class="d-kpi"><span class="k-label">Lists</span><b class="num">{cats.length}</b><span class="k-sub">Netflix publishes ranks only per country</span></div>
      {/if}
    {/if}
    <div class="d-kpi yours"><span class="k-label">On your watchlist</span><b class="num">{onList}</b><span class="k-sub">{onList ? 'of these titles' : 'bookmark one below'}</span></div>
  </section>

  <div class="lists" class:two={cats.length <= 2}>
    {#each cats as [cat, list] (cat)}
      <section class="panel pad list">
        <h2>{cat}</h2>
        <ol>
          {#each list as it (`${it.rank}:${it.netflix_title_id}`)}
            {@const k = kindOf(it)}
            {@const t = it.tmdb_id ? library.get(k, it.tmdb_id) : null}
            {@const href = it.tmdb_id ? `#/title/${k}/${it.tmdb_id}` : null}
            <li>
              <span class="rank num" class:one={it.rank === 1}>{it.rank}</span>
              <svelte:element this={href ? 'a' : 'span'} class="pic" {href} tabindex="-1" aria-hidden="true"><Poster path={it.poster_path ?? null} seed={it.tmdb_id ?? it.show_title} title={it.show_title} width={34} radius={4} size="w92" /></svelte:element>
              <div class="txt">
                {#if href}<a class="name" {href}>{name(it)}</a>{:else}<span class="name">{name(it)}</span>{/if}
                <span class="sub">
                  {#if views(it)}<span class="num">{compact(views(it))} views</span>{/if}
                  {#if it.cumulative_weeks === 1}<span class="tag new">New</span>{:else if it.cumulative_weeks}<span class="num">{it.cumulative_weeks} weeks</span>{/if}
                  {#if t}<span class="you"><Icon name="check" size={11} />{t.status === 'watchlist' ? 'Watchlist' : t.watch_count > 0 ? 'Seen' : 'Yours'}</span>{/if}
                </span>
              </div>
              {#if it.tmdb_id}
                <button class="mark" aria-pressed={t?.status === 'watchlist'} aria-label="{t?.status === 'watchlist' ? 'Remove from' : 'Add to'} watchlist: {it.show_title}" onclick={() => toggle(it)}>
                  <Icon name={t?.status === 'watchlist' ? 'bookmark-fill' : 'bookmark'} size={14} />
                </button>
              {/if}
            </li>
          {/each}
        </ol>
      </section>
    {/each}
  </div>
  <p class="d-source">Netflix Top 10 (netflix.com/tudum/top10), imported weekly by metareel. Weeks run Monday to Sunday; views are hours viewed divided by runtime.</p>
{/if}

<style>
  .weeknav { display: inline-flex; align-items: center; gap: 8px; font-size: 14px; font-weight: 600; }
  .nav { display: grid; place-items: center; width: 30px; height: 30px; border: 0; border-radius: 50%; background: var(--bg-elevated); color: var(--text-primary); font: 600 18px/1 var(--font-sans); cursor: pointer; }
  .nav:disabled { opacity: 0.4; cursor: default; }
  .lists { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
  .list h2 { margin: 0 0 4px; }
  ol { display: grid; margin: 0; padding: 0; list-style: none; }
  li { display: grid; grid-template-columns: 26px auto minmax(0, 1fr) auto; align-items: center; gap: 10px; padding: 7px 0; box-shadow: inset 0 -1px 0 var(--border-subtle); }
  li:last-child { box-shadow: none; }
  .rank { font: 700 18px/1 var(--font-display); color: var(--text-muted); text-align: right; }
  .rank.one { color: var(--delta-up); }
  .pic { display: block; }
  .txt { display: grid; gap: 3px; min-width: 0; }
  .name { font: 700 15px/1.25 var(--font-display); color: inherit; text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  a.name:hover { text-decoration: underline; text-underline-offset: 3px; }
  .sub { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 10px; color: var(--text-muted); font-size: 12.5px; }
  .tag { padding: 1px 6px; border-radius: 7px; background: var(--bg-elevated); font-size: 11px; font-weight: 700; }
  .tag.new { color: var(--delta-debut); }
  .you { display: inline-flex; align-items: center; gap: 3px; color: var(--accent-yours); font-weight: 600; }
  .mark { display: grid; place-items: center; width: 28px; height: 28px; border: 0; border-radius: 50%; background: var(--bg-elevated); color: var(--text-muted); cursor: pointer; }
  .mark:hover { color: var(--text-primary); }
  .mark[aria-pressed='true'] { background: color-mix(in srgb, var(--accent-yours) 20%, transparent); color: var(--accent-yours); }
  @media (max-width: 900px) {
    .lists { grid-template-columns: 1fr; }
  }
</style>
