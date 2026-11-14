<!-- Settings: your cinema subscriptions (AMC Stubs A-List, Regal Unlimited, ODEON Limitless, ...).
     A plan has runs (cancel and resubscribe, and the gap between isn't billed) and prices (each from
     a date, for when the plan's price changes). Watches logged at its chain are marked "covered by"
     it, and Stats weighs what it cost against what those tickets were worth. -->
<script>
  import { api } from '../lib/api.js'
  import { prefs } from '../lib/prefs.svelte.js'
  import { scope } from '../lib/scope.svelte.js'
  import { toast } from '../lib/toast.svelte.js'
  import { toMinor, fromMinor, currencyFor, fractionDigits } from '../lib/money.js'
  import { localDay } from '../lib/dates.js'

  let { onconfirm } = $props()

  let subs = $state(null)
  let plans = $state([])
  let error = $state('')
  // { id?, plan, chain, currency, period, note, runs: [{ started_on, ended_on }], prices: [{ from, price }] }
  let form = $state(null)
  let saving = $state(false)

  async function load() {
    try {
      const r = await api.subscriptions()
      subs = r.subscriptions ?? []
      plans = r.plans ?? []
      error = ''
    } catch (err) {
      error = err.message
    }
  }
  load()

  const today = localDay()
  const fmt = (minor, cur) => {
    try {
      return new Intl.NumberFormat(undefined, { style: 'currency', currency: cur }).format(minor / 10 ** fractionDigits(cur))
    } catch {
      return `${fromMinor(minor, cur)} ${cur}`
    }
  }
  function add() {
    const cur = prefs.values.currency ?? currencyFor(scope.country)
    form = { id: null, plan: '', chain: '', currency: cur, period: 'month', note: '',
      runs: [{ started_on: today, ended_on: '' }], prices: [{ from: '', price: '' }] }
  }
  const runsOf = (s) => s.periods?.length ? s.periods : [{ started_on: s.started_on, ended_on: s.ended_on }]
  const pricesOf = (s) => s.prices?.length ? s.prices : [{ from: s.started_on, price_minor: s.price_minor }]
  function edit(s) {
    form = { id: s.id, plan: s.plan, chain: s.chain ?? '', currency: s.currency, period: s.period, note: s.note ?? '',
      runs: runsOf(s).map((p) => ({ started_on: p.started_on, ended_on: p.ended_on ?? '' })),
      // The first price applies from the start, so its date isn't edited.
      prices: pricesOf(s).map((p, i) => ({ from: i ? p.from : '', price: fromMinor(p.price_minor, s.currency) })) }
  }
  const lastRun = $derived(form?.runs.at(-1))
  function addRun() {
    // Resubscribing: the last run has to have ended first.
    if (lastRun && !lastRun.ended_on) lastRun.ended_on = today
    form.runs.push({ started_on: lastRun?.ended_on && lastRun.ended_on >= today ? lastRun.ended_on : today, ended_on: '' })
  }
  const addPrice = () => form.prices.push({ from: today, price: form.prices.at(-1)?.price ?? '' })
  // Picking a well-known plan fills its chain and currency.
  function pickPlan() {
    const p = plans.find((x) => x.plan.toLowerCase() === form.plan.trim().toLowerCase())
    if (p) {
      if (!form.chain) form.chain = p.chain
      form.currency = p.currency
    }
  }
  // The API body for a plan, from a stored one with its runs and prices changed.
  const bodyOf = (s, runs, prices) => ({ plan: s.plan, chain: s.chain || null, currency: s.currency, period: s.period, note: s.note || null,
    periods: runs.map((p) => ({ started_on: p.started_on, ended_on: p.ended_on || null })),
    prices: prices.map((p) => ({ from: p.from, price_minor: p.price_minor })) })
  async function save(e) {
    e.preventDefault()
    const cur = form.currency.trim().toUpperCase()
    const prices = []
    for (const p of form.prices) {
      const minor = toMinor(p.price, cur)
      if (minor == null) {
        toast('Enter what the plan costs, like 23.99.', 'error')
        return
      }
      prices.push({ from: p.from || form.runs[0].started_on, price_minor: minor })
    }
    const body = bodyOf({ ...form, currency: cur }, form.runs, prices)
    saving = true
    try {
      if (form.id) await api.updateSubscription(form.id, body)
      else await api.createSubscription(body)
      toast(form.id ? `Saved ${form.plan}.` : `Added ${form.plan}. Watches at ${form.chain || 'its cinemas'} can now be marked as covered.`)
      form = null
      load()
    } catch (err) {
      toast(err.message, 'error')
    } finally {
      saving = false
    }
  }
  async function end(s) {
    const runs = runsOf(s).map((p, i, all) => (i === all.length - 1 ? { ...p, ended_on: today } : p))
    try {
      await api.updateSubscription(s.id, bodyOf(s, runs, pricesOf(s)))
      toast(`Ended ${s.plan} today. Resubscribe any time; the gap won’t count as paid.`)
      load()
    } catch (err) {
      toast(err.message, 'error')
    }
  }
  // Taking an ended plan up again: a new run from today (after the last one's end).
  async function resubscribe(s) {
    const runs = runsOf(s)
    const from = runs.at(-1).ended_on >= today ? null : today
    if (!from) {
      edit(s)
      addRun()
      return
    }
    try {
      await api.updateSubscription(s.id, bodyOf(s, [...runs, { started_on: from, ended_on: null }], pricesOf(s)))
      toast(`Resubscribed to ${s.plan} from today. If the price changed, edit the plan to add it.`)
      load()
    } catch (err) {
      toast(err.message, 'error')
    }
  }
  const was = (s) => {
    const p = pricesOf(s)
    return p.length > 1 ? p.filter((x) => x.from <= today).at(-2) ?? null : null
  }
  function remove(s) {
    onconfirm({
      title: `Delete ${s.plan}?`,
      body: 'Its watches stay in your diary but are no longer marked as covered, and it leaves the savings figures. To keep its history, end it instead.',
      confirm: 'Delete plan',
      danger: true,
      run: async () => {
        await api.deleteSubscription(s.id)
        toast(`Deleted ${s.plan}.`)
        load()
      },
    })
  }
</script>

<section class="panel pad" aria-labelledby="s-plans">
  <div class="sh">
    <h2 id="s-plans">Cinema subscriptions</h2>
    <span class="hint">AMC Stubs A-List, Regal Unlimited, ODEON Limitless… Stats shows whether each one pays off.</span>
    {#if subs && !form}<button type="button" class="btn-secondary sm quiet" onclick={add}>Add a plan</button>{/if}
  </div>
  {#if error}
    <p class="err" role="alert">{error}</p>
  {:else if !subs}
    <div class="skeleton" style="height:60px" aria-busy="true" aria-label="Loading"></div>
  {:else}
    {#if subs.length}
      <ul class="plans">
        {#each subs as s (s.id)}
          {@const active = !s.ended_on || s.ended_on >= today}
          {@const runs = runsOf(s)}
          {@const before = was(s)}
          <li>
            <div class="pl">
              <b>{s.plan}</b>
              <span class="hint">{fmt(s.price_minor, s.currency)} a {s.period}{#if before}{' '}(was {fmt(before.price_minor, s.currency)}){/if}{#if s.chain}{' · '}{s.chain}{/if} · since {s.started_on}{#if s.ended_on}{' · '}{active ? 'ends' : 'ended'} {s.ended_on}{/if}</span>
              {#if runs.length > 1}
                <span class="hint runs">{runs.length} runs: {#each runs as r, i}{i ? ', ' : ''}{r.started_on} – {r.ended_on ?? 'now'}{/each}</span>
              {/if}
            </div>
            <span class="pill" class:on={active}>{active ? 'Active' : 'Ended'}</span>
            <div class="acts">
              <button type="button" class="btn-secondary sm" onclick={() => edit(s)}>Edit</button>
              {#if active && !s.ended_on}<button type="button" class="btn-secondary sm" onclick={() => end(s)}>End today</button>{/if}
              {#if !active}<button type="button" class="btn-secondary sm" onclick={() => resubscribe(s)}>Resubscribe</button>{/if}
              <button type="button" class="btn-secondary sm" onclick={() => remove(s)}>Delete</button>
            </div>
          </li>
        {/each}
      </ul>
    {:else if !form}
      <p class="hint">No plans yet. Add the one you pay for, and log watches as covered by it.</p>
    {/if}

    {#if form}
      <form class="pf" onsubmit={save}>
        <label class="wide">Plan<input bind:value={form.plan} list="known-plans" onchange={pickPlan} required maxlength="100" placeholder="e.g. AMC Stubs A-List" /></label>
        <datalist id="known-plans">{#each plans as p}<option value={p.plan}>{p.chain} · {p.country}</option>{/each}</datalist>
        <label>Chain<input bind:value={form.chain} maxlength="100" placeholder="e.g. AMC" aria-describedby="chain-hint" /></label>
        <label>Currency<input class="cc" bind:value={form.currency} maxlength="3" required /></label>
        <label>Billed
          <select bind:value={form.period}><option value="month">Monthly</option><option value="year">Yearly</option></select>
        </label>

        <fieldset class="wide">
          <legend>When you had it</legend>
          <p class="hint">Cancelled and came back? Add another run; the gap in between isn’t counted as paid.</p>
          {#each form.runs as r, i}
            <div class="row">
              <label>{form.runs.length > 1 ? `Run ${i + 1} started` : 'Started'}<input type="date" bind:value={r.started_on} min={i ? form.runs[i - 1].ended_on : undefined} required /></label>
              <label>Ended<input type="date" bind:value={r.ended_on} min={r.started_on} required={i < form.runs.length - 1} /></label>
              {#if form.runs.length > 1}<button type="button" class="btn-secondary sm x" aria-label="Remove run {i + 1}" onclick={() => form.runs.splice(i, 1)}>Remove</button>{/if}
            </div>
          {/each}
          <button type="button" class="btn-secondary sm add" onclick={addRun}>Add a resubscription</button>
        </fieldset>

        <fieldset class="wide">
          <legend>Price per {form.period}</legend>
          <p class="hint">When the price changed, add it from the date of the first bill at the new price.</p>
          {#each form.prices as p, i}
            <div class="row">
              {#if i}<label>From<input type="date" bind:value={p.from} required /></label>{:else}<span class="from0">{form.prices.length > 1 ? 'At the start' : ' '}</span>{/if}
              <label>Price<input inputmode="decimal" bind:value={p.price} required placeholder="23.99" /></label>
              {#if i}<button type="button" class="btn-secondary sm x" aria-label="Remove the price from {p.from}" onclick={() => form.prices.splice(i, 1)}>Remove</button>{/if}
            </div>
          {/each}
          <button type="button" class="btn-secondary sm add" onclick={addPrice}>Add a price change</button>
        </fieldset>
        <label class="wide">Note<input bind:value={form.note} maxlength="500" placeholder="e.g. 3 movies a week, IMAX included" /></label>
        <p class="hint wide" id="chain-hint">Watches at cinemas whose chain or name contains this start out covered by the plan. Prices are what you pay each period, including tax.</p>
        <div class="foot wide">
          <button type="button" class="btn-secondary sm" onclick={() => (form = null)}>Cancel</button>
          <button type="submit" class="btn-primary sm" disabled={saving}>{saving ? 'Saving…' : form.id ? 'Save' : 'Add plan'}</button>
        </div>
      </form>
    {/if}
  {/if}
</section>

<style>
  .sh { display: flex; flex-wrap: wrap; align-items: baseline; gap: 6px 12px; margin-bottom: 6px; }
  .sh .quiet { margin-left: auto; }
  h2 { margin: 0; font: 700 18px/1.25 var(--font-display); }
  .hint { color: var(--text-muted); font-size: 13px; }
  .sm { height: 34px; padding: 0 14px; font-size: 13.5px; }
  .err { margin: 0; color: var(--delta-down); font-size: 13.5px; }
  .plans { display: grid; gap: 0; margin: 0; padding: 0; list-style: none; }
  .plans li { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; align-items: center; gap: 8px 14px; padding: 12px 0; border-top: 1px solid var(--border-subtle); }
  .plans li:first-child { border-top: 0; }
  .pl { display: grid; gap: 2px; min-width: 0; }
  .pl b { font-size: 15px; }
  .pill.on { color: var(--accent-yours); }
  .acts { display: flex; flex-wrap: wrap; gap: 6px; justify-content: flex-end; }
  .pf { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px 14px; padding-top: 12px; border-top: 1px solid var(--border-subtle); }
  .pf label { display: grid; gap: 6px; color: var(--text-muted); font-size: 13px; font-weight: 600; }
  .pf .wide { grid-column: 1 / -1; }
  .pf .hint { margin: 0; font-weight: 400; }
  input, select { min-width: 0; padding: 9px 12px; border: 1px solid var(--border-subtle); border-radius: 10px; background: var(--bg-canvas); color: var(--text-primary); font: 400 15px/1.3 var(--font-sans); }
  .cc { text-transform: uppercase; }
  .runs { font-size: 12.5px; }
  fieldset { display: grid; gap: 8px; margin: 0; padding: 12px 14px; border: 1px solid var(--border-subtle); border-radius: 12px; }
  legend { padding: 0 6px; color: var(--text-primary); font-size: 13.5px; font-weight: 700; }
  fieldset .hint { margin: 0; }
  .row { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto; align-items: end; gap: 10px; }
  .from0 { align-self: center; color: var(--text-muted); font-size: 13px; }
  .add { justify-self: start; }
  .x { align-self: end; }
  .foot { display: flex; justify-content: flex-end; gap: 10px; }
  @media (max-width: 640px) {
    .plans li { grid-template-columns: minmax(0, 1fr) auto; }
    .acts { grid-column: 1 / -1; justify-content: flex-start; }
    .pf { grid-template-columns: 1fr 1fr; }
    .row { grid-template-columns: 1fr 1fr; }
    .row .x { grid-column: 1 / -1; justify-self: start; }
  }
</style>
