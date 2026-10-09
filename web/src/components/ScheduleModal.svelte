<script>
  import { untrack } from 'svelte'
  import { createSchedule, updateSchedule } from '../lib/api.js'
  import { COUNTRIES, PROVIDERS, providerLabel, countryName } from '../lib/constants.js'
  import { utcTimeToLocal } from '../lib/format.js'
  import { toast } from '../lib/toast.svelte.js'
  import Modal from './Modal.svelte'
  import Icon from './Icon.svelte'

  // taskTypes: [{type, label, description, has_targets}] from /task-types.
  let { schedule, taskTypes = [], onclose, onsaved } = $props()

  const isEdit = $derived(schedule != null)
  const initial = untrack(() => schedule)

  let taskType = $state(initial?.task_type ?? 'flixpatrol.top10.scrape')
  let name = $state(initial?.name ?? '')
  let enabled = $state(initial?.enabled ?? true)
  let runtimes = $state([...(initial?.utc_runtimes ?? [])])
  let newTime = $state('')
  let maxRetries = $state(initial?.max_retries ?? 2)
  let delay = $state(initial?.request_delay_seconds ?? 10)
  let robots = $state(initial?.respect_robots ?? true)
  let userAgent = $state(initial?.user_agent ?? 'metareel-flixpatrol-bot/0.1')
  let backfillDays = $state(initial?.backfill_days ?? 3)
  let recheckHours = $state(initial?.recheck_hours ?? 6)
  let targets = $state([...(initial?.flixpatrol_targets ?? [])])
  let country = $state('US')
  let provider = $state('netflix')
  let saving = $state(false)
  let error = $state('')

  const info = $derived(taskTypes.find((t) => t.type === taskType))
  const isScrape = $derived(taskType === 'flixpatrol.top10.scrape')
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
    if (targets.some((x) => tkey(x) === tkey(t))) {
      error = `${countryName(country)} · ${providerLabel(provider)} is already a target.`
      return
    }
    targets = [...targets, t]
    error = ''
  }

  async function save() {
    error = ''
    if (!name.trim()) return (error = 'Name is required.')
    if (runtimes.length === 0) return (error = 'Add at least one run time.')
    if (isScrape && targets.length === 0) return (error = 'Add at least one country / provider target.')
    saving = true
    const body = {
      name: name.trim(), enabled, utc_runtimes: runtimes, max_retries: Number(maxRetries),
      request_delay_seconds: Number(delay), respect_robots: robots, user_agent: userAgent,
    }
    if (isScrape) Object.assign(body, { backfill_days: Number(backfillDays), recheck_hours: Number(recheckHours) })
    try {
      if (isEdit) {
        await updateSchedule(schedule.id, isScrape ? { ...body, flixpatrol_targets: targets } : body)
      } else {
        await createSchedule({ ...body, task_type: taskType, task_details: isScrape ? { targets } : undefined })
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
      <span class="field-label">Job</span>
      <select class="select" bind:value={taskType} disabled={isEdit}>
        {#each taskTypes as t (t.type)}<option value={t.type}>{t.label}</option>{/each}
        {#if !taskTypes.length}<option value={taskType}>{taskType}</option>{/if}
      </select>
    </label>
    <label class="field">
      <span class="field-label">Name</span>
      <input class="input" bind:value={name} placeholder="e.g. us-daily" />
    </label>
    <div class="field">
      <span class="field-label">Status</span>
      <label class="checkbox" style="height:32px"><input type="checkbox" bind:checked={enabled} />Enabled</label>
    </div>
  </div>
  {#if info}<p class="field-hint" style="margin:-6px 0 12px">{info.description}</p>{/if}

  <div class="field">
    <span class="field-label">Run times (UTC)</span>
    <div class="row" style="flex-wrap:wrap">
      {#each runtimes as rt (rt)}
        <span class="chip"><span class="mono">{rt}</span><span class="muted small">{utcTimeToLocal(rt)} local</span>
          <button onclick={() => (runtimes = runtimes.filter((x) => x !== rt))} aria-label="Remove {rt}"><Icon name="x" size={11} /></button></span>
      {/each}
      <input class="input mono" style="width:90px" bind:value={newTime} placeholder="HH:MM" aria-label="New run time (UTC)" onkeydown={(e) => e.key === 'Enter' && addTime()} />
      <button class="btn btn-sm" onclick={addTime}><Icon name="plus" size={12} />Add</button>
    </div>
    {#if isScrape}<span class="field-hint">FlixPatrol publishes the day's chart around 12:00 UTC; earlier runs fetch the previous day.</span>{/if}
  </div>

  <div class="field-row">
    <label class="field">
      <span class="field-label">Max retries</span>
      <input class="input" type="number" min="0" bind:value={maxRetries} />
    </label>
    {#if isScrape || taskType === 'titles.enrich'}
      <label class="field">
        <span class="field-label">Delay between FlixPatrol pages (s)</span>
        <input class="input" type="number" min="0" bind:value={delay} />
      </label>
    {/if}
    {#if isScrape}
      <div class="field">
        <span class="field-label">robots.txt</span>
        <label class="checkbox" style="height:32px"><input type="checkbox" bind:checked={robots} />Respect</label>
      </div>
    {/if}
  </div>

  {#if isScrape}
    <div class="field-row">
      <label class="field">
        <span class="field-label">Fill gaps from the last (days)</span>
        <input class="input" type="number" min="0" max="60" bind:value={backfillDays} />
        <span class="field-hint">Also scrapes missing charts from this many previous days (0 = off).</span>
      </label>
      <label class="field">
        <span class="field-label">Re-check unpublished charts for (hours)</span>
        <input class="input" type="number" min="0" max="23" bind:value={recheckHours} />
        <span class="field-hint">When a chart still matches the previous day's, check again hourly before saving it (0 = save at once).</span>
      </label>
    </div>

    <label class="field">
      <span class="field-label">User agent</span>
      <input class="input mono" bind:value={userAgent} />
    </label>

    <div class="field">
      <span class="field-label">Targets</span>
      <div class="row" style="flex-wrap:wrap;gap:6px;margin-bottom:8px">
        {#each targets as t (tkey(t))}
          <span class="chip">{countryName(t.country_code)} · {providerLabel(t.provider_slug)}
            <button onclick={() => (targets = targets.filter((x) => tkey(x) !== tkey(t)))} aria-label="Remove {countryName(t.country_code)} {providerLabel(t.provider_slug)}"><Icon name="x" size={11} /></button></span>
        {/each}
        {#if !targets.length}<span class="muted small">No targets yet.</span>{/if}
      </div>
      <div class="row">
        <select class="select grow" bind:value={country} aria-label="Country">{#each COUNTRIES as c (c.code)}<option value={c.code}>{c.name}</option>{/each}</select>
        <select class="select grow" bind:value={provider} aria-label="Provider">{#each PROVIDERS as p (p.value)}<option value={p.value}>{p.label}</option>{/each}</select>
        <button class="btn" onclick={addTarget}><Icon name="plus" size={14} />Add target</button>
      </div>
    </div>
  {/if}

  {#if error}<div class="alert alert-error small"><Icon name="alert" size={14} />{error}</div>{/if}

  {#snippet footer()}
    <button class="btn" onclick={onclose}>Cancel</button>
    <button class="btn btn-primary" onclick={save} disabled={saving}>{saving ? 'Saving…' : isEdit ? 'Save changes' : 'Create schedule'}</button>
  {/snippet}
</Modal>
