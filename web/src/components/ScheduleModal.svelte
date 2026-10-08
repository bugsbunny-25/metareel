<script>
  import { untrack } from 'svelte'
  import { createSchedule, updateSchedule, addScheduleTargets } from '../lib/api.js'
  import { COUNTRIES, PROVIDERS, providerLabel, countryName } from '../lib/constants.js'
  import { utcTimeToLocal } from '../lib/format.js'
  import { toast } from '../lib/toast.svelte.js'
  import Modal from './Modal.svelte'
  import Icon from './Icon.svelte'

  let { schedule, onclose, onsaved } = $props()

  const isEdit = $derived(schedule != null)
  const initial = untrack(() => schedule)

  let name = $state(initial?.name ?? '')
  let enabled = $state(initial?.enabled ?? true)
  let runtimes = $state([...(initial?.utc_runtimes ?? [])])
  let newTime = $state('')
  let maxRetries = $state(initial?.max_retries ?? 2)
  let delay = $state(initial?.request_delay_seconds ?? 5)
  let robots = $state(initial?.respect_robots ?? false)
  let userAgent = $state(initial?.user_agent ?? 'metareel-flixpatrol-bot/0.1')
  const existingTargets = initial?.flixpatrol_targets ?? []
  let newTargets = $state([])
  let country = $state('US')
  let provider = $state('netflix')
  let saving = $state(false)
  let error = $state('')

  const tkey = (t) => `${t.country_code}/${t.provider_slug}`

  function addTime() {
    if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(newTime)) {
      error = 'Run time must be HH:MM (24-hour, UTC).'
      return
    }
    if (!runtimes.includes(newTime)) runtimes = [...runtimes, newTime].sort()
    newTime = ''
    error = ''
  }

  function addTarget() {
    const t = { country_code: country, provider_slug: provider }
    if ([...existingTargets, ...newTargets].some((x) => tkey(x) === tkey(t))) {
      error = `${countryName(country)} · ${providerLabel(provider)} is already a target.`
      return
    }
    newTargets = [...newTargets, t]
    error = ''
  }

  async function save() {
    error = ''
    if (!name.trim()) return (error = 'Name is required.')
    if (runtimes.length === 0) return (error = 'Add at least one run time.')
    if (!isEdit && newTargets.length === 0) return (error = 'Add at least one country / provider target.')
    saving = true
    const body = {
      name: name.trim(), enabled, utc_runtimes: runtimes, max_retries: Number(maxRetries),
      request_delay_seconds: Number(delay), respect_robots: robots, user_agent: userAgent,
    }
    try {
      if (isEdit) {
        await updateSchedule(schedule.id, body)
        if (newTargets.length) await addScheduleTargets(schedule.id, newTargets)
      } else {
        await createSchedule({ ...body, task_type: 'flixpatrol.top10.scrape', task_details: { targets: newTargets } })
      }
      toast.success(`Schedule "${body.name}" ${isEdit ? 'updated' : 'created'}`)
      onsaved()
    } catch (e) {
      error = e.message
    } finally {
      saving = false
    }
  }
</script>

<Modal title={isEdit ? `Edit "${schedule.name}"` : 'New schedule'} {onclose} large>
  <div class="field-row">
    <label class="field">
      <span class="field-label">Name</span>
      <input class="input" bind:value={name} placeholder="e.g. us-daily" />
    </label>
    <div class="field">
      <span class="field-label">Status</span>
      <label class="checkbox" style="height:32px"><input type="checkbox" bind:checked={enabled} />Enabled</label>
    </div>
  </div>

  <div class="field">
    <span class="field-label">Run times (UTC)</span>
    <div class="row" style="flex-wrap:wrap">
      {#each runtimes as rt (rt)}
        <span class="chip"><span class="mono">{rt}</span><span class="muted small">{utcTimeToLocal(rt)} local</span>
          <button onclick={() => (runtimes = runtimes.filter((x) => x !== rt))} aria-label="Remove {rt}"><Icon name="x" size={11} /></button></span>
      {/each}
      <input class="input mono" style="width:90px" bind:value={newTime} placeholder="HH:MM" onkeydown={(e) => e.key === 'Enter' && addTime()} />
      <button class="btn btn-sm" onclick={addTime}><Icon name="plus" size={12} />Add</button>
    </div>
    <span class="field-hint">FlixPatrol publishes the day's chart around 12:00 UTC; earlier runs fetch the previous day.</span>
  </div>

  <div class="field-row">
    <label class="field">
      <span class="field-label">Max retries per target</span>
      <input class="input" type="number" min="0" bind:value={maxRetries} />
    </label>
    <label class="field">
      <span class="field-label">Delay between requests (s)</span>
      <input class="input" type="number" min="0" bind:value={delay} />
    </label>
    <div class="field">
      <span class="field-label">robots.txt</span>
      <label class="checkbox" style="height:32px"><input type="checkbox" bind:checked={robots} />Respect</label>
    </div>
  </div>

  <label class="field">
    <span class="field-label">User agent</span>
    <input class="input mono" bind:value={userAgent} />
  </label>

  <div class="field">
    <span class="field-label">Targets</span>
    <div class="row" style="flex-wrap:wrap;gap:6px;margin-bottom:8px">
      {#each existingTargets as t (tkey(t))}<span class="chip">{countryName(t.country_code)} · {providerLabel(t.provider_slug)}</span>{/each}
      {#each newTargets as t (tkey(t))}
        <span class="chip" style="background:var(--accent-soft)">{countryName(t.country_code)} · {providerLabel(t.provider_slug)}
          <button onclick={() => (newTargets = newTargets.filter((x) => tkey(x) !== tkey(t)))} aria-label="Remove"><Icon name="x" size={11} /></button></span>
      {/each}
      {#if !existingTargets.length && !newTargets.length}<span class="muted small">No targets yet.</span>{/if}
    </div>
    <div class="row">
      <select class="select grow" bind:value={country}>{#each COUNTRIES as c (c.code)}<option value={c.code}>{c.name}</option>{/each}</select>
      <select class="select grow" bind:value={provider}>{#each PROVIDERS as p (p.value)}<option value={p.value}>{p.label}</option>{/each}</select>
      <button class="btn" onclick={addTarget}><Icon name="plus" size={14} />Add target</button>
    </div>
    {#if isEdit}<span class="field-hint">Existing targets can't be removed here yet; new ones are added when you save.</span>{/if}
  </div>

  {#if error}<div class="alert alert-error small"><Icon name="alert" size={14} />{error}</div>{/if}

  {#snippet footer()}
    <button class="btn" onclick={onclose}>Cancel</button>
    <button class="btn btn-primary" onclick={save} disabled={saving}>{saving ? 'Saving…' : isEdit ? 'Save changes' : 'Create schedule'}</button>
  {/snippet}
</Modal>
