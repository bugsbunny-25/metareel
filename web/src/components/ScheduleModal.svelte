<script>
  import { untrack } from 'svelte'
  import { COUNTRIES, PROVIDERS, createSchedule, updateSchedule } from '../lib/api.js'

  let { schedule, onclose, onsave } = $props()

  const isEdit = $derived(schedule != null)

  let name       = $state(untrack(() => schedule?.name ?? ''))
  let enabled    = $state(untrack(() => schedule?.enabled ?? true))
  let runtimes   = $state(untrack(() => (schedule?.utc_runtimes ?? []).join(', ')))
  let maxRetries = $state(untrack(() => schedule?.max_retries ?? 3))
  let delay      = $state(untrack(() => schedule?.request_delay_seconds ?? 10))
  let robots     = $state(untrack(() => schedule?.respect_robots ?? true))
  let userAgent  = $state(untrack(() => schedule?.user_agent ?? 'metareel-flixpatrol-bot/0.1'))
  let selectedCountry = $state(COUNTRIES[0]?.code ?? '')
  let selectedProvider = $state(
    PROVIDERS.find((provider) => provider.value)?.value ?? ''
  )
  let targets = $state([])

  let saving = $state(false)
  let error  = $state('')

  function parseRuntimes(raw) {
    return raw.split(',').map(s => s.trim()).filter(Boolean)
  }

  function targetKey(target) {
    return `${target.country_code}/${target.provider_slug}`
  }

  function addTarget() {
    const countryCode = selectedCountry.trim().toUpperCase()
    const providerSlug = selectedProvider.trim().toLowerCase()
    if (!countryCode || !providerSlug) {
      error = 'Select both country and provider to add a target.'
      return
    }

    const nextTarget = { country_code: countryCode, provider_slug: providerSlug }
    if (targets.some((target) => targetKey(target) === targetKey(nextTarget))) {
      error = 'That country/provider combination is already added.'
      return
    }

    targets = [...targets, nextTarget]
    error = ''
  }

  function removeTarget(targetToRemove) {
    targets = targets.filter((target) => targetKey(target) !== targetKey(targetToRemove))
  }

  async function save() {
    saving = true
    error  = ''
    try {
      const utcRuntimes = parseRuntimes(runtimes)
      if (utcRuntimes.length === 0) { error = 'At least one run time is required.'; saving = false; return }

      let savedSchedule
      if (isEdit) {
        savedSchedule = await updateSchedule(schedule.id, {
          name, enabled,
          utc_runtimes: utcRuntimes,
          max_retries: Number(maxRetries),
          request_delay_seconds: Number(delay),
          respect_robots: robots,
          user_agent: userAgent,
        })
      } else {
        if (targets.length === 0) { error = 'Add at least one country/provider target.'; saving = false; return }
        savedSchedule = await createSchedule({
          task_type: 'flixpatrol.top10.scrape',
          name, enabled,
          utc_runtimes: utcRuntimes,
          max_retries: Number(maxRetries),
          request_delay_seconds: Number(delay),
          respect_robots: robots,
          user_agent: userAgent,
          task_details: { targets },
        })
      }
      onsave(savedSchedule)
    } catch (e) {
      error = e.message
    } finally {
      saving = false
    }
  }

  function onBackdropClick(e) {
    if (e.target === e.currentTarget) onclose()
  }
</script>

<div
  class="modal-backdrop"
  onclick={onBackdropClick}
  onkeydown={(e) => { if (e.key === 'Escape') onclose() }}
  role="dialog"
  aria-modal="true"
  tabindex="-1"
>
  <div class="modal modal-lg">
    <div class="modal-title">{isEdit ? 'Edit schedule' : 'New schedule'}</div>
    {#if isEdit}
      <div class="modal-subtitle">{schedule.task_type}</div>
    {/if}

    <div class="form-row">
      <div class="form-field" style="flex:1">
        <label class="form-label" for="sched-name">Name</label>
        <input id="sched-name" class="form-input" type="text" bind:value={name} placeholder="My schedule" />
      </div>
      <div class="form-field form-field-inline">
        <label class="form-label" for="sched-enabled">Enabled</label>
        <input id="sched-enabled" type="checkbox" bind:checked={enabled} class="form-checkbox" />
      </div>
    </div>

    <div class="form-field">
      <label class="form-label" for="sched-runtimes">Run times (UTC, comma-separated)</label>
      <input id="sched-runtimes" class="form-input" type="text" bind:value={runtimes} placeholder="09:00, 18:00" />
    </div>

    <div class="form-row">
      <div class="form-field" style="flex:1">
        <label class="form-label" for="sched-retries">Max retries</label>
        <input id="sched-retries" class="form-input" type="number" min="0" bind:value={maxRetries} />
      </div>
      <div class="form-field" style="flex:1">
        <label class="form-label" for="sched-delay">Request delay (s)</label>
        <input id="sched-delay" class="form-input" type="number" min="0" bind:value={delay} />
      </div>
      <div class="form-field form-field-inline">
        <label class="form-label" for="sched-robots">Respect robots</label>
        <input id="sched-robots" type="checkbox" bind:checked={robots} class="form-checkbox" />
      </div>
    </div>

    <div class="form-field">
      <label class="form-label" for="sched-ua">User agent</label>
      <input id="sched-ua" class="form-input" type="text" bind:value={userAgent} />
    </div>

    {#if !isEdit}
      <div class="form-field">
        <div class="form-label">Targets (country + provider)</div>
        <div class="form-row">
          <div class="form-field" style="flex:1">
            <label class="form-label" for="sched-target-country">Country</label>
            <select id="sched-target-country" class="form-input" bind:value={selectedCountry}>
              {#each COUNTRIES as country}
                <option value={country.code}>{country.name} ({country.code})</option>
              {/each}
            </select>
          </div>
          <div class="form-field" style="flex:1">
            <label class="form-label" for="sched-target-provider">Provider</label>
            <select id="sched-target-provider" class="form-input" bind:value={selectedProvider}>
              {#each PROVIDERS.filter((provider) => provider.value) as provider}
                <option value={provider.value}>{provider.label}</option>
              {/each}
            </select>
          </div>
          <div class="form-field" style="justify-content:flex-end;display:flex">
            <button class="btn" type="button" onclick={addTarget}>Add target</button>
          </div>
        </div>
        {#if targets.length === 0}
          <div class="state-msg" style="margin-top:8px">No targets added yet.</div>
        {:else}
          <div style="margin-top:8px">
            {#each targets as target (targetKey(target))}
              <div class="form-row" style="align-items:center; margin-bottom:6px">
                <div class="cell-mono" style="flex:1">{target.country_code}/{target.provider_slug}</div>
                <button class="btn btn-sm" type="button" onclick={() => removeTarget(target)}>Delete</button>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}

    {#if error}
      <div class="error-msg">{error}</div>
    {/if}

    <div class="modal-actions">
      <button class="btn" onclick={onclose}>Cancel</button>
      <button class="btn btn-primary" onclick={save} disabled={saving}>
        {saving ? 'Saving…' : isEdit ? 'Save changes' : 'Create'}
      </button>
    </div>
  </div>
</div>
