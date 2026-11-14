<!-- Discover (#/discover?tab=new|calendar|gems|elsewhere|leaderboard|netflix|trends|services), for
     the country (and, where a tab is per service, the service) in the top bar. The rotation
     response is loaded here and shared: its services group offers into the service you'd pay
     for, on every tab. Each tab keeps its own filters in the query; switching tabs clears them. -->
<script>
  import Gems from './discover/Gems.svelte'
  import Elsewhere from './discover/Elsewhere.svelte'
  import Services from './discover/Services.svelte'
  import Releases from './discover/Releases.svelte'
  import Calendar from './discover/Calendar.svelte'
  import Leaderboards from './discover/Leaderboards.svelte'
  import NetflixTop10 from './discover/NetflixTop10.svelte'
  import Trends from './discover/Trends.svelte'
  import '../discover.css'
  import { api } from '../lib/api.js'
  import { scope, inCountry, providerName } from '../lib/scope.svelte.js'
  import { prefs } from '../lib/prefs.svelte.js'
  import { route, setQuery } from '../lib/router.svelte.js'
  import { familyMap } from '../lib/services.js'

  let { onlog } = $props()

  const TABS = [['new', 'New & upcoming'], ['calendar', 'Episode calendar'], ['gems', 'Hidden gems'], ['elsewhere', 'Popular elsewhere'],
    ['leaderboard', 'Leaderboards'], ['netflix', 'Netflix Top 10'], ['trends', 'Trends'], ['services', 'Which services to pay for']]
  const tab = $derived(TABS.some(([t]) => t === route.query.tab) ? route.query.tab : 'new')
  // Every tab's own query keys, cleared on a tab switch.
  const TAB_KEYS = ['slots', 'days', 'kind', 'rating', 'seen', 'service', 'when', 'only', 'period', 'where', 'cat', 'nf', 'week', 'range', 'svc']
  const SUBS = {
    new: () => `What ${providerName(scope.provider)} added in ${inCountry(scope.country)} lately, and what it has announced.`,
    calendar: () => `New episodes of the shows streaming in ${inCountry(scope.country)}, day by day.`,
    leaderboard: () => 'The titles that held the Top 10 longest and highest.',
    netflix: () => 'Netflix’s own weekly Top 10, with views, and its all-time most popular.',
    trends: () => `What the charts in ${inCountry(scope.country)} say about genres, staying power and originals.`,
  }
  const sub = $derived(SUBS[tab]?.() ?? `What’s worth watching in ${inCountry(scope.country)} that the charts don’t show you.`)
  function choose(t) {
    setQuery({ tab: t === 'new' ? '' : t, ...Object.fromEntries(TAB_KEYS.map((k) => [k, ''])) })
  }
  const slots = $derived([1, 2, 3, 4].includes(+route.query.slots) ? +route.query.slots : 2)
  const days = $derived([30, 60, 90].includes(+route.query.days) ? +route.query.days : 30)

  let rotation = $state(null)
  let rotationError = $state('')
  $effect(() => {
    const cc = scope.country
    const subscribed = (prefs.values.services ?? []).join(',')
    const params = { slots, days, subscribed: subscribed || undefined }
    const ctrl = new AbortController()
    rotationError = ''
    api.rotation(cc, params, ctrl.signal)
      .then((r) => (rotation = r))
      .catch((err) => { if (err.name !== 'AbortError') rotationError = err.message })
    return () => ctrl.abort()
  })
  const families = $derived(familyMap(rotation?.services))
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Discover</h1>
      <p class="sub">{sub}</p>
    </div>
    <div class="tabs" role="tablist" aria-label="Discover">
      {#each TABS as [t, label]}
        <button role="tab" aria-selected={tab === t} onclick={() => choose(t)}>{label}</button>
      {/each}
    </div>
  </header>

  {#if tab === 'new'}
    <Releases country={scope.country} provider={scope.provider} />
  {:else if tab === 'calendar'}
    <Calendar country={scope.country} {families} />
  {:else if tab === 'gems'}
    <Gems country={scope.country} {families} {onlog} />
  {:else if tab === 'elsewhere'}
    <Elsewhere country={scope.country} {families} />
  {:else if tab === 'leaderboard'}
    <Leaderboards country={scope.country} provider={scope.provider} />
  {:else if tab === 'netflix'}
    <NetflixTop10 country={scope.country} />
  {:else if tab === 'trends'}
    <Trends country={scope.country} provider={scope.provider} />
  {:else}
    <Services {rotation} error={rotationError} {slots} {days} />
  {/if}
</div>

<style>
  .page { display: grid; gap: 20px; padding: 20px var(--gutter) 48px; }
  .head { display: grid; gap: 14px; }
  h1 { margin: 0; font: 700 32px/1.1 var(--font-display); letter-spacing: -0.02em; }
  .sub { margin: 4px 0 0; color: var(--text-muted); font-size: 15px; }
  .tabs { display: flex; flex-wrap: wrap; gap: 6px; }
  .tabs button { flex: none; height: 36px; padding: 0 16px; white-space: nowrap; border: 0; border-radius: 18px; background: var(--bg-surface); color: var(--text-muted); font: 600 14px/1 var(--font-sans); cursor: pointer; }
  .tabs button:hover { color: var(--text-primary); }
  .tabs button[aria-selected='true'] { background: var(--accent-primary); color: var(--accent-on); }
  /* Eight tabs: one scrolling row on phones instead of four wrapped ones. */
  @media (max-width: 640px) {
    .tabs { flex-wrap: nowrap; overflow-x: auto; scrollbar-width: none; margin: 0 calc(var(--gutter) * -1); padding: 0 var(--gutter); }
    .tabs::-webkit-scrollbar { display: none; }
  }
</style>
