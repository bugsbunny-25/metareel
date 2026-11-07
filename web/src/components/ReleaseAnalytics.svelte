<script>
  // Release patterns, spread lag and release windows for the Insights
  // analytics tab; loads its own data from the tab's filters.
  import { getAnalytics } from '../lib/api.js'
  import { number } from '../lib/format.js'
  import { countryName, providerLabel } from '../lib/constants.js'
  import Icon from './Icon.svelte'
  import InfoTip from './InfoTip.svelte'
  import HBars from './HBars.svelte'

  let { range = '30', country = '', provider = '', kind = '' } = $props()

  let data = $state(null)
  let error = $state('')

  function span(days) {
    const to = new Date()
    const from = new Date(Date.now() - (Number(days) - 1) * 864e5)
    return { from: from.toISOString().slice(0, 10), to: to.toISOString().slice(0, 10) }
  }

  $effect(() => {
    const r = span(range)
    const spread = span(Math.min(Number(range), 180)) // the endpoint's maximum
    const patterns = span(Math.max(Number(range), 365)) // premieres: a year by default
    const q = { country, provider }
    data = null
    error = ''
    let cancelled = false
    Promise.allSettled([
      getAnalytics('release-patterns', { ...patterns, ...q }),
      getAnalytics('spread-lag', { ...spread, ...q, kind }),
      getAnalytics('release-windows', { ...r, country: country || 'US' }),
    ]).then(([p, s, w]) => {
      if (cancelled) return
      data = { patterns: p.value ?? null, spread: s.value ?? null, windows: w.value ?? null }
      const failed = [p, s, w].filter((x) => x.status === 'rejected')
      error = failed.length ? failed.map((x) => x.reason.message).join('; ') : ''
    })
    return () => { cancelled = true }
  })

  const PATTERN = { binge: 'All at once', batch: 'In batches', weekly: 'Weekly', daily: 'Daily', irregular: 'Irregular' }
  const pct = (v) => (v == null ? '—' : `${Math.round(v * 100)}%`)
  const d = (v) => (v == null ? '—' : `${v} d`)
</script>

{#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

{#if !data}
  <div class="skeleton" style="height:200px;margin-top:16px"></div>
{:else}
  <div class="grid grid-2" style="margin-top:16px">
    {#if data.patterns}
      {@const p = data.patterns}
      <div class="card">
        <div class="card-header">
          <div>
            <div class="card-title">Weekly vs all at once <InfoTip text="TV seasons by how their episodes came out (TMDB air dates), and how they did on each chart over {p.window_days} days after the premiere. Premieres in the last year." /></div>
            <div class="card-sub">{p.seasons.length} seasons with a finished window · {p.excluded.in_progress} still running</div>
          </div>
        </div>
        <div class="card-body">
          {#if p.patterns.length}
            <div class="table-wrap">
              <table class="table">
                <thead><tr><th>Release</th><th class="right">Seasons</th><th class="right">Days in Top 10</th><th class="right">Last day charting</th><th class="right">Still on day 28</th></tr></thead>
                <tbody>
                  {#each p.patterns as r (r.pattern)}
                    <tr><td>{PATTERN[r.pattern] ?? r.pattern}</td><td class="right">{r.seasons}</td><td class="right">{r.median_days_in_top10}</td><td class="right">{r.median_last_day}</td><td class="right">{pct(r.share_still_charting_day_28)}</td></tr>
                  {/each}
                </tbody>
              </table>
            </div>
            <p class="field-hint">Medians per chart run (season × chart). Needs TMDB episodes (TMDB episodes job).</p>
          {:else}<div class="empty small">No season has a finished window yet (needs TMDB episodes).</div>{/if}
        </div>
      </div>
    {/if}

    {#if data.windows}
      {@const w = data.windows}
      <div class="card">
        <div class="card-header">
          <div>
            <div class="card-title">Theaters to home <InfoTip text="Movies released in theaters in {countryName(w.country)} in the range: days until their digital (PVOD) release and until they were seen streaming (an offer arriving, or their first chart day). TMDB release dates." /></div>
            <div class="card-sub">{w.movies} movies in theaters in {countryName(w.country)}</div>
          </div>
        </div>
        <div class="card-body">
          {#if w.movies}
            <div class="table-wrap">
              <table class="table">
                <thead><tr><th>Window</th><th class="right">Movies</th><th class="right">Median</th><th class="right">≤ 17 d</th><th class="right">≤ 45 d</th><th class="right">Unknown</th></tr></thead>
                <tbody>
                  {#each [['Theaters → digital', w.theatrical_to_digital], ['Theaters → streaming', w.theatrical_to_streaming], ['Digital → streaming', w.digital_to_streaming]] as [label, s] (label)}
                    <tr><td>{label}</td><td class="right">{s.titles}</td><td class="right">{d(s.median_days)}</td><td class="right">{s.within_days['17']}</td><td class="right">{s.within_days['45']}</td><td class="right muted">{s.missing}</td></tr>
                  {/each}
                </tbody>
              </table>
            </div>
            {#if w.streaming_by_provider.length}
              <div class="section-title">Theaters → streaming by first service</div>
              <HBars color="var(--c3)" items={w.streaming_by_provider.slice(0, 6).map((s) => ({ key: s.provider + s.basis, label: providerLabel(s.provider), value: s.lag.median_days, display: `${s.lag.median_days} d`, title: `${s.lag.titles} movies` }))} />
            {/if}
          {:else}<div class="empty small">No theatrical releases in the range (needs TMDB release dates).</div>{/if}
        </div>
      </div>
    {/if}

    {#if data.spread}
      {@const s = data.spread}
      <div class="card">
        <div class="card-header">
          <div>
            <div class="card-title">How titles spread <InfoTip text="Days after a title first charts in one country until it charts in another, for debuts in the last {Math.min(Number(range), 180)} days. A country counts only with 30 days of stored charts before the debut." /></div>
            <div class="card-sub">{s.titles} debuts · {s.multi_country} reached 2+ countries · {s.simultaneous} the same day</div>
          </div>
        </div>
        <div class="card-body">
          {#if s.countries.length}
            <p class="small">Median arrival after the first country: <b>{d(s.summary.median_days)}</b> ({s.summary.titles} arrivals)</p>
            <div class="table-wrap">
              <table class="table">
                <thead><tr><th>Country</th><th class="right">Debuted here</th><th class="right">Arrived later</th><th class="right">Median lag</th><th class="right">Never within {s.window} d</th></tr></thead>
                <tbody>
                  {#each s.countries as c (c.country)}
                    <tr><td>{countryName(c.country)}</td><td class="right">{c.origin}</td><td class="right">{c.arrived}</td><td class="right">{d(c.arrival_lag.median_days)}</td><td class="right">{c.not_reached}</td></tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {:else}<div class="empty small">Needs charts from two or more countries with 30 days of history.</div>{/if}
        </div>
      </div>
    {/if}
  </div>
{/if}
