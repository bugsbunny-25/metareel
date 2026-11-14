<!-- Settings: your cinema subscriptions (AMC Stubs A-List, Regal Unlimited, ODEON Limitless, ...).
     A plan has a price per month or year and the days it ran; watches logged at its chain are
     marked "covered by" it, and Stats weighs what it cost against what those tickets were worth. -->
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
  let form = $state(null) // { id?, plan, chain, price, currency, period, started_on, ended_on, note }
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
    form = { id: null, plan: '', chain: '', price: '', currency: cur, period: 'month', started_on: today, ended_on: '', note: '' }
  }
  function edit(s) {
    form = { id: s.id, plan: s.plan, chain: s.chain ?? '', price: fromMinor(s.price_minor, s.currency), currency: s.currency, period: s.period,
      started_on: s.started_on, ended_on: s.ended_on ?? '', note: s.note ?? '' }
  }
  // Picking a well-known plan fills its chain and currency.
  function pickPlan() {
    const p = plans.find((x) => x.plan.toLowerCase() === form.plan.trim().toLowerCase())
    if (p) {
      if (!form.chain) form.chain = p.chain
      form.currency = p.currency
    }
  }
  async function save(e) {
    e.preventDefault()
    const price = toMinor(form.price, form.currency.trim().toUpperCase())
    if (price == null || price === undefined) {
      toast('Enter what the plan costs, like 23.99.', 'error')
      return
    }
    const body = { plan: form.plan, chain: form.chain || null, price_minor: price, currency: form.currency, period: form.period,
      started_on: form.started_on, ended_on: form.ended_on || null, note: form.note || null }
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
    try {
      await api.updateSubscription(s.id, { ...s, ended_on: today })
      toast(`Ended ${s.plan} today.`)
      load()
    } catch (err) {
      toast(err.message, 'error')
    }
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
          <li>
            <div class="pl">
              <b>{s.plan}</b>
              <span class="hint">{fmt(s.price_minor, s.currency)} a {s.period}{#if s.chain}{' · '}{s.chain}{/if} · since {s.started_on}{#if s.ended_on}{' · '}{active ? 'ends' : 'ended'} {s.ended_on}{/if}</span>
            </div>
            <span class="pill" class:on={active}>{active ? 'Active' : 'Ended'}</span>
            <div class="acts">
              <button type="button" class="btn-secondary sm" onclick={() => edit(s)}>Edit</button>
              {#if active && !s.ended_on}<button type="button" class="btn-secondary sm" onclick={() => end(s)}>End today</button>{/if}
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
        <label>Price<input inputmode="decimal" bind:value={form.price} required placeholder="23.99" /></label>
        <label>Currency<input class="cc" bind:value={form.currency} maxlength="3" required /></label>
        <label>Billed
          <select bind:value={form.period}><option value="month">Monthly</option><option value="year">Yearly</option></select>
        </label>
        <label>Started<input type="date" bind:value={form.started_on} required /></label>
        <label>Ended<input type="date" bind:value={form.ended_on} min={form.started_on} /></label>
        <label class="wide">Note<input bind:value={form.note} maxlength="500" placeholder="e.g. 3 movies a week, IMAX included" /></label>
        <p class="hint wide" id="chain-hint">Watches at cinemas whose chain or name contains this start out covered by the plan. The price is what you pay each period, including tax.</p>
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
  .foot { display: flex; justify-content: flex-end; gap: 10px; }
  @media (max-width: 640px) {
    .plans li { grid-template-columns: minmax(0, 1fr) auto; }
    .acts { grid-column: 1 / -1; justify-content: flex-start; }
    .pf { grid-template-columns: 1fr 1fr; }
  }
</style>
