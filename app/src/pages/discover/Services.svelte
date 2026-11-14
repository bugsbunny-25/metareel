<!-- Which services to pay for: the rotation endpoint ranks each subscription service in the
     country on what charted, what's arriving, which shows have new episodes coming and how
     many top-rated titles it has, then recommends which to pay for over the window. The
     services you pay for are a preference (saved to your account), so the advice becomes
     keep / pause / add. -->
<script>
  import { prefs, updatePrefs } from '../../lib/prefs.svelte.js'
  import { setQuery } from '../../lib/router.svelte.js'
  import { inCountry, scope } from '../../lib/scope.svelte.js'
  import { toast } from '../../lib/toast.svelte.js'
  import { dayLabel } from '../../lib/dates.js'

  let { rotation, error, slots, days } = $props()

  const mine = $derived(new Set(prefs.values.services ?? []))
  let saving = $state(false)
  async function toggleMine(s) {
    const next = mine.has(s.provider_id) ? [...mine].filter((id) => id !== s.provider_id) : [...mine, s.provider_id]
    saving = true
    try {
      await updatePrefs({ services: next })
    } catch (err) {
      toast(err.message, 'error')
    } finally {
      saving = false
    }
  }

  const REC = {
    subscribe: ['Subscribe', 'go'], keep: ['Keep', 'go'], add: ['Add', 'new'], pause: ['Pause', 'stale'], skip: ['Skip', 'off'],
  }
  // What a score is made of (see the API): chart points, 10 per show with episodes coming,
  // 3 per arrival, 1 per top-rated title.
  const PARTS = [['chart', 'Chart points'], ['shows', 'Shows with new episodes ×10'], ['arrivals', 'Arrivals ×3'], ['rated', 'Top rated ×1']]
  const parts = (s) => ({ chart: s.chart_points, shows: s.upcoming_shows * 10, arrivals: s.arrivals * 3, rated: s.top_rated })
  const services = $derived(rotation?.services ?? [])
  const max = $derived(Math.max(1, ...services.map((s) => s.score)))
  const payFor = $derived(services.filter((s) => ['subscribe', 'keep', 'add'].includes(s.recommendation)))
  const pause = $derived(services.filter((s) => s.recommendation === 'pause'))
  const n = (v) => v.toLocaleString()
  const pick = (patch) => setQuery(patch, { replace: true })
</script>

<div class="filters">
  <div class="seg" role="group" aria-label="How many services to pay for">
    {#each [1, 2, 3, 4] as v}<button aria-pressed={slots === v} onclick={() => pick({ slots: v === 2 ? '' : v })}>{v} {v === 1 ? 'service' : 'services'}</button>{/each}
  </div>
  <div class="seg" role="group" aria-label="Window">
    {#each [30, 60, 90] as v}<button aria-pressed={days === v} onclick={() => pick({ days: v === 30 ? '' : v })}>{v} days</button>{/each}
  </div>
</div>

{#if error}
  <p class="alert" role="alert">Couldn’t rank the services: {error}</p>
{:else if !rotation}
  <div class="skeleton" style="height:480px" aria-busy="true" aria-label="Loading"></div>
{:else if !services.length}
  <section class="empty">
    <h2>No services to rank in {inCountry(scope.country)}</h2>
    <p>Ranking needs availability data for the country; metareel collects it as it maps the charts.</p>
  </section>
{:else}
  <section class="verdict panel pad" aria-label="Recommendation">
    <p class="big">
      {#if payFor.length}
        Pay for <b>{payFor.map((s) => s.provider_name).join(' and ')}</b> until {dayLabel(rotation.upcoming_to)}.
      {:else}
        Nothing stands out for the next {days} days.
      {/if}
      {#if pause.length}{' '}Pause <b class="stale">{pause.map((s) => s.provider_name).join(' and ')}</b>.{/if}
    </p>
    <p class="hint">
      Based on charts from {dayLabel(rotation.past_from)} to {dayLabel(rotation.past_to)}{rotation.chart_basis === 'all_countries' ? ' in every country (there are none here)' : ` in ${inCountry(scope.country)}`},
      what arrived and left, and episodes airing until {dayLabel(rotation.upcoming_to)}.
    </p>
    <div class="mine">
      <span class="lbl">You pay for</span>
      {#each services as s (s.provider_id)}
        <button class="chip" aria-pressed={mine.has(s.provider_id)} disabled={saving} onclick={() => toggleMine(s)}>{s.provider_name}</button>
      {/each}
      <span class="hint">{mine.size ? 'Saved to your account; the advice becomes keep, pause or add.' : 'Pick yours to get keep, pause or add instead.'}</span>
    </div>
  </section>

  <div class="legend" aria-hidden="true">
    {#each PARTS as [k, l]}<span><i class="p-{k}"></i>{l}</span>{/each}
  </div>

  <ol class="services">
    {#each services as s, i (s.provider_id)}
      {@const p = parts(s)}
      {@const [label, tone] = REC[s.recommendation] ?? [s.recommendation, 'off']}
      <li class:yours={mine.has(s.provider_id)}>
        <span class="rk num">{i + 1}</span>
        <div class="body">
          <div class="top">
            <b class="name">{s.provider_name}</b>
            <span class="rec {tone}">{label}</span>
            {#if mine.has(s.provider_id)}<span class="you">You pay for this</span>{/if}
            <span class="score num" title="Score">{n(s.score)}</span>
          </div>
          <span class="stack" role="img" aria-label="Score {s.score}: {s.chart_points} chart points, {s.upcoming_shows} shows with new episodes, {s.arrivals} arrivals, {s.top_rated} top-rated titles">
            {#each PARTS as [k]}{#if p[k]}<i class="p-{k}" style:width="{(p[k] / max) * 100}%"></i>{/if}{/each}
          </span>
          <p class="facts">
            <span><b class="num">{n(s.chart_points)}</b> chart points from {n(s.charting)} titles</span>
            <span><b class="num">{s.upcoming_shows}</b> shows with new episodes</span>
            <span><b class="num up">+{s.arrivals}</b> / <b class="num down">−{s.departures}</b> titles in the window</span>
            <span><b class="num">{n(s.top_rated)}</b> rated 7.5+ of {n(s.titles)}</span>
          </p>
          {#if s.highlights?.length}
            <p class="hl">
              {#each s.highlights.slice(0, 5) as h (h.kind + h.tmdb_id + h.detail)}
                <a href="#/title/{h.kind}/{h.tmdb_id}"><b>{h.title}</b>{' '}<span class="hint">{h.detail}</span></a>
              {/each}
            </p>
          {/if}
        </div>
      </li>
    {/each}
  </ol>
{/if}

<style>
  .filters { display: flex; flex-wrap: wrap; gap: 10px 14px; }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { height: 30px; padding: 0 12px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .seg button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }
  .hint { color: var(--text-muted); font-size: 13px; }

  .verdict { gap: 10px; }
  .big { margin: 0; font: 600 22px/1.3 var(--font-display); }
  .big b { color: var(--delta-up); }
  .big b.stale { color: var(--status-stale); }
  .verdict .hint { margin: 0; }
  .mine { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; padding-top: 10px; border-top: 1px solid var(--border-subtle); }
  .mine .lbl { margin-right: 4px; font-size: 13px; font-weight: 700; }
  .chip { height: 30px; padding: 0 12px; border: 1px solid var(--border-subtle); border-radius: 15px; background: none; color: var(--text-primary); font: 500 13.5px/1 var(--font-sans); cursor: pointer; }
  .chip[aria-pressed='true'] { border-color: var(--accent-yours); background: color-mix(in srgb, var(--accent-yours) 16%, transparent); color: var(--accent-yours); font-weight: 700; }
  .chip:disabled { opacity: 0.6; }

  .legend { display: flex; flex-wrap: wrap; gap: 6px 16px; color: var(--text-muted); font-size: 12.5px; }
  .legend span { display: inline-flex; align-items: center; gap: 6px; }
  .legend i { display: inline-block; width: 12px; height: 10px; border-radius: 2px; }
  .p-chart { background: color-mix(in srgb, var(--text-primary) 85%, var(--bg-surface)); }
  .p-shows { background: color-mix(in srgb, var(--text-primary) 55%, var(--bg-surface)); }
  .p-arrivals { background: color-mix(in srgb, var(--text-primary) 35%, var(--bg-surface)); }
  .p-rated { background: color-mix(in srgb, var(--text-primary) 18%, var(--bg-surface)); }

  .services { display: grid; gap: 10px; margin: 0; padding: 0; list-style: none; }
  .services li { display: grid; grid-template-columns: 30px minmax(0, 1fr); gap: 14px; padding: 16px 18px; border-radius: 16px; background: var(--bg-surface); }
  .services li.yours { box-shadow: inset 3px 0 0 var(--accent-yours); }
  .rk { color: var(--text-muted); font: 700 22px/1.2 var(--font-display); text-align: right; }
  .body { display: grid; gap: 8px; min-width: 0; }
  .top { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 10px; }
  .name { font: 700 19px/1.2 var(--font-display); }
  .rec { padding: 3px 9px; border-radius: 10px; font-size: 12px; font-weight: 700; letter-spacing: 0.02em; text-transform: uppercase; }
  .rec.go { background: color-mix(in srgb, var(--delta-up) 18%, transparent); color: var(--delta-up); }
  .rec.new { background: color-mix(in srgb, var(--delta-debut) 18%, transparent); color: var(--delta-debut); }
  .rec.stale { background: color-mix(in srgb, var(--status-stale) 20%, transparent); color: var(--status-stale); }
  .rec.off { background: var(--bg-elevated); color: var(--text-muted); }
  .you { color: var(--accent-yours); font-size: 12.5px; font-weight: 700; }
  .score { margin-left: auto; font-size: 18px; font-weight: 700; }
  .stack { display: flex; height: 10px; border-radius: 5px; background: var(--bg-elevated); overflow: hidden; }
  .stack i { display: block; height: 100%; }
  .facts { display: flex; flex-wrap: wrap; gap: 4px 18px; margin: 0; color: var(--text-muted); font-size: 13px; }
  .facts b { color: var(--text-primary); }
  .facts .up { color: var(--delta-up); }
  .facts .down { color: var(--delta-down); }
  .hl { display: flex; flex-wrap: wrap; gap: 6px; margin: 0; }
  .hl a { padding: 5px 10px; border-radius: 10px; background: var(--bg-elevated); color: inherit; font-size: 13px; text-decoration: none; }
  .hl a:hover { background: color-mix(in srgb, var(--text-primary) 10%, var(--bg-elevated)); }
  .hl b { font-weight: 600; }
  .empty { padding: 30px 0; }
  .empty h2 { margin: 0 0 6px; font: 700 22px/1.2 var(--font-display); }
  .empty p { color: var(--text-muted); max-width: 60ch; }
  @media (max-width: 600px) {
    .services li { grid-template-columns: 1fr; padding: 14px; }
    .rk { display: none; }
    .big { font-size: 19px; }
  }
</style>
