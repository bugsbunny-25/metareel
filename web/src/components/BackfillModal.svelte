<script>
  import { backfill } from '../lib/api.js'
  import { toast } from '../lib/toast.svelte.js'
  import Modal from './Modal.svelte'
  import Icon from './Icon.svelte'

  let { schedule, onclose } = $props()

  const today = new Date().toISOString().slice(0, 10)
  const weekAgo = new Date(Date.now() - 7 * 864e5).toISOString().slice(0, 10)
  let from = $state(weekAgo)
  let to = $state(today)
  let force = $state(false)
  let saving = $state(false)
  let error = $state('')

  const days = $derived(from && to ? Math.round((Date.parse(to) - Date.parse(from)) / 864e5) + 1 : 0)
  const pages = $derived(days * (schedule.flixpatrol_targets?.length ?? 0))

  async function submit() {
    error = ''
    if (days < 1) return (error = '"To" must be on or after "From".')
    if (days > 60) return (error = 'At most 60 days per backfill.')
    saving = true
    try {
      await backfill({ schedule_id: schedule.id, from, to, force })
      toast.success(`Backfill of ${days} day(s) queued for "${schedule.name}"`)
      onclose()
    } catch (e) {
      error = e.message
    } finally {
      saving = false
    }
  }
</script>

<Modal title={`Backfill "${schedule.name}"`} {onclose}>
  <p class="muted small" style="margin-top:0">
    Scrapes every target's chart for each date in the range as one job run. Charts already stored are skipped unless you re-scrape them.
  </p>
  <div class="field-row">
    <label class="field"><span class="field-label">From</span><input class="input" type="date" bind:value={from} max={to} /></label>
    <label class="field"><span class="field-label">To</span><input class="input" type="date" bind:value={to} max={today} /></label>
  </div>
  <label class="checkbox"><input type="checkbox" bind:checked={force} />Re-scrape charts already stored</label>
  <p class="field-hint">Up to {pages} FlixPatrol page(s), paced by the schedule's request delay.</p>
  {#if error}<div class="alert alert-error small"><Icon name="alert" size={14} />{error}</div>{/if}
  {#snippet footer()}
    <button class="btn" onclick={onclose}>Cancel</button>
    <button class="btn btn-primary" onclick={submit} disabled={saving}>{saving ? 'Queuing…' : 'Queue backfill'}</button>
  {/snippet}
</Modal>
