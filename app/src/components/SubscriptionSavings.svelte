<!-- Stats: does your cinema subscription pay off? Each plan's covered tickets at face value
     against what it has cost (billing periods plus anything paid on top), month by month, and the
     break-even. For chains you paid at without a plan, what a plan would have had to cost.
     All time, not the year picked above: a plan runs across years. -->
<script>
  import { api } from '../lib/api.js'
  import { fractionDigits } from '../lib/money.js'

  let data = $state(null)
  let error = $state('')
  $effect(() => {
    const ctrl = new AbortController()
    api.subscriptionSavings(ctrl.signal)
      .then((d) => (data = d))
      .catch((err) => { if (err.name !== 'AbortError') error = err.message })
    return () => ctrl.abort()
  })

  const money = (minor, cur, sign = false) => {
    const v = minor / 10 ** fractionDigits(cur)
    try {
      return new Intl.NumberFormat(undefined, { style: 'currency', currency: cur, maximumFractionDigits: Math.abs(v) >= 100 ? 0 : fractionDigits(cur), signDisplay: sign ? 'exceptZero' : 'auto' }).format(v)
    } catch {
      return `${v.toFixed(2)} ${cur}`
    }
  }
  const MON = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
  const monthLabel = (m) => `${MON[+m.slice(5, 7) - 1]}${m.slice(5, 7) === '01' ? ` ’${m.slice(2, 4)}` : ''}`
  const subs = $derived(data?.subscriptions ?? [])
  const whatIf = $derived((data?.what_if ?? []).filter((w) => !w.have_subscription))
  const scale = (s) => Math.max(1, ...s.months.map((m) => Math.max(m.value_minor, m.cost_minor)))
</script>

{#if error}
  <section class="panel pad"><div class="ph"><h2>Cinema subscriptions</h2></div><p class="hint">Savings didn’t load: {error}</p></section>
{:else if data && (subs.length || whatIf.length)}
  <section class="panel pad" aria-labelledby="sav-h">
    <div class="ph"><h2 id="sav-h">Cinema subscriptions</h2><span class="hint">All time · covered tickets at face value against what the plan cost</span></div>
    {#each subs as s (s.id)}
      {@const ahead = s.saved_minor >= 0}
      <article class="plan">
        <div class="top">
          <div>
            <p class="name">{s.plan} <span class="pill" class:on={s.active}>{s.active ? 'Active' : 'Ended'}</span></p>
            <p class="hint">{money(s.price_minor, s.currency)} a {s.period} · {s.bills} {s.bills === 1 ? 'bill' : 'bills'} since {s.started_on}</p>
          </div>
          <div class="verdict" class:ahead class:behind={!ahead}>
            <b class="num">{money(Math.abs(s.saved_minor), s.currency)}</b>
            <span>{ahead ? 'saved' : 'more than buying tickets'}</span>
          </div>
        </div>
        <dl class="facts">
          <div><dt>Covered</dt><dd class="num">{s.watches} {s.watches === 1 ? 'watch' : 'watches'}</dd></div>
          <div><dt>Worth</dt><dd class="num">{money(s.value_minor, s.currency)}</dd></div>
          <div><dt>Plan cost</dt><dd class="num">{money(s.paid_minor, s.currency)}</dd></div>
          {#if s.extras_minor}<div><dt>Paid on top</dt><dd class="num">{money(s.extras_minor, s.currency)}</dd></div>{/if}
          {#if s.break_even_watches}<div><dt>Pays off at</dt><dd class="num">{s.break_even_watches} a {s.period}</dd></div>{/if}
          <div><dt>You average</dt><dd class="num">{s.watches_per_period.toFixed(1)} a {s.period}</dd></div>
        </dl>
        {#if s.months.length > 1}
          {@const max = scale(s)}
          <div class="bars" role="img" aria-label="Value of covered tickets against plan cost, month by month">
            {#each s.months as m (m.month)}
              <div class="mo" title="{m.month}: {m.watches} watches worth {money(m.value_minor, s.currency)}, cost {money(m.cost_minor, s.currency)} ({money(m.net_minor, s.currency, true)})">
                <div class="pair">
                  <i class="v" style:height="{(m.value_minor / max) * 100}%"></i>
                  <i class="c" style:height="{(m.cost_minor / max) * 100}%"></i>
                </div>
                <span class="ml">{monthLabel(m.month)}</span>
              </div>
            {/each}
          </div>
          <p class="legend hint"><i class="v"></i>Tickets’ worth <i class="c"></i>Plan and extras</p>
        {/if}
        {#if s.estimated || s.unvalued || s.other_currency}
          <p class="hint note">
            {#if s.estimated}{s.estimated} valued at your usual {s.chain || ''} ticket ({money(s.usual_ticket_minor ?? 0, s.currency)}) because no face value was logged.{/if}
            {#if s.unvalued}{' '}{s.unvalued} without a face value or a paid ticket to go by: not counted.{/if}
            {#if s.other_currency}{' '}{s.other_currency} in another currency: not counted.{/if}
          </p>
        {/if}
      </article>
    {/each}

    {#if whatIf.length}
      <div class="whatif">
        <p class="sub-h">Would a plan pay off?</p>
        <ul>
          {#each whatIf as w (w.plan)}
            <li>
              <b>{w.chain}</b>
              <span>You paid {money(w.spent_minor, w.currency)} for {w.watches} {w.watches === 1 ? 'ticket' : 'tickets'} in the last 12 months, {money(w.per_month_minor, w.currency)} a month. {w.plan} pays off if it costs less than that a month.</span>
            </li>
          {/each}
        </ul>
      </div>
    {/if}
  </section>
{/if}

<style>
  .ph { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 12px; }
  .ph h2 { margin: 0; font: 700 18px/1.25 var(--font-display); }
  .hint { color: var(--text-muted); font-size: 13px; margin: 0; }
  .plan { display: grid; gap: 12px; padding: 14px 0; border-top: 1px solid var(--border-subtle); }
  .plan:first-of-type { border-top: 0; padding-top: 6px; }
  .top { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: flex-start; gap: 12px; }
  .name { margin: 0 0 2px; font: 700 17px/1.3 var(--font-display); }
  .pill { margin-left: 6px; vertical-align: 2px; }
  .pill.on { color: var(--accent-yours); }
  .verdict { display: grid; justify-items: end; }
  .verdict b { font: 700 30px/1.05 var(--font-sans); letter-spacing: -0.03em; }
  .verdict span { color: var(--text-muted); font-size: 13px; }
  .verdict.ahead b { color: var(--delta-up); }
  .verdict.behind b { color: var(--delta-down); }
  .facts { display: grid; grid-template-columns: repeat(auto-fill, minmax(120px, 1fr)); gap: 8px 16px; margin: 0; }
  .facts div { display: grid; gap: 2px; }
  dt { color: var(--text-muted); font-size: 12.5px; }
  dd { margin: 0; font-weight: 700; font-size: 15px; }
  .bars { display: flex; align-items: flex-end; gap: 6px; height: 120px; overflow-x: auto; padding-bottom: 2px; }
  .mo { display: grid; grid-template-rows: 1fr auto; gap: 4px; flex: 1 0 34px; height: 100%; }
  .pair { display: flex; align-items: flex-end; justify-content: center; gap: 2px; min-height: 0; }
  .pair i { display: block; width: 10px; min-height: 1px; border-radius: 3px 3px 0 0; }
  i.v { background: var(--delta-up); }
  i.c { background: var(--text-muted); opacity: 0.55; }
  .ml { color: var(--text-muted); font-size: 11px; text-align: center; white-space: nowrap; }
  .legend { display: flex; align-items: center; gap: 6px; }
  .legend i { display: inline-block; width: 10px; height: 10px; border-radius: 2px; }
  .legend i.c { margin-left: 10px; }
  .note { line-height: 1.45; }
  .whatif { padding-top: 12px; border-top: 1px solid var(--border-subtle); }
  .sub-h { margin: 0 0 6px; color: var(--text-muted); font: 700 12px/1 var(--font-sans); letter-spacing: 0.06em; text-transform: uppercase; }
  .whatif ul { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }
  .whatif li { display: grid; gap: 2px; font-size: 14px; }
  .whatif li span { color: var(--text-muted); }
</style>
