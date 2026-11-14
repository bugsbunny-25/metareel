<!-- Discover (#/discover?tab=gems|elsewhere|services), for the country in the top bar. The
     rotation response is loaded here and shared: its services group offers into the service
     you'd pay for, on every tab. -->
<script>
  import Gems from './discover/Gems.svelte'
  import Elsewhere from './discover/Elsewhere.svelte'
  import Services from './discover/Services.svelte'
  import { api } from '../lib/api.js'
  import { scope, inCountry } from '../lib/scope.svelte.js'
  import { prefs } from '../lib/prefs.svelte.js'
  import { route, setQuery } from '../lib/router.svelte.js'
  import { familyMap } from '../lib/services.js'

  let { onlog } = $props()

  const TABS = [['gems', 'Hidden gems'], ['elsewhere', 'Popular elsewhere'], ['services', 'Which services to pay for']]
  const tab = $derived(TABS.some(([t]) => t === route.query.tab) ? route.query.tab : 'gems')
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
      <p class="sub">What’s worth watching in {inCountry(scope.country)} that the charts don’t show you.</p>
    </div>
    <div class="tabs" role="tablist" aria-label="Discover">
      {#each TABS as [t, label]}
        <button role="tab" aria-selected={tab === t} onclick={() => setQuery({ tab: t === 'gems' ? '' : t, slots: '', days: '' })}>{label}</button>
      {/each}
    </div>
  </header>

  {#if tab === 'gems'}
    <Gems country={scope.country} {families} {onlog} />
  {:else if tab === 'elsewhere'}
    <Elsewhere country={scope.country} {families} />
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
  .tabs button { height: 36px; padding: 0 16px; border: 0; border-radius: 18px; background: var(--bg-surface); color: var(--text-muted); font: 600 14px/1 var(--font-sans); cursor: pointer; }
  .tabs button:hover { color: var(--text-primary); }
  .tabs button[aria-selected='true'] { background: var(--accent-primary); color: var(--accent-on); }
</style>
