<script>
  import { getSchedule, listSchedules, runScheduleNow } from '../lib/api.js'
  import ScheduleModal from './ScheduleModal.svelte'

  let items    = $state([])
  let loading  = $state(false)
  let error    = $state('')
  let editing  = $state(null)
  let creating = $state(false)
  let openingEditId = $state(null)
  // track per-row "run now" status: id → 'loading' | 'ok' | 'error'
  let runStatus = $state({})

  async function load() {
    loading = true
    error = ''
    try {
      items = await listSchedules()
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  $effect(() => { load() })

  async function onRunNow(id) {
    runStatus = { ...runStatus, [id]: 'loading' }
    try {
      await runScheduleNow(id)
      runStatus = { ...runStatus, [id]: 'ok' }
      setTimeout(() => { runStatus = { ...runStatus, [id]: null } }, 3000)
    } catch (e) {
      runStatus = { ...runStatus, [id]: 'error' }
      setTimeout(() => { runStatus = { ...runStatus, [id]: null } }, 3000)
    }
  }

  async function onEdit(id) {
    openingEditId = id
    error = ''
    try {
      editing = await getSchedule(id)
    } catch (e) {
      error = e.message
    } finally {
      openingEditId = null
    }
  }

  function onSaved(savedSchedule) {
    editing = null
    creating = false
    if (savedSchedule?.id) {
      const idx = items.findIndex((item) => item.id === savedSchedule.id)
      if (idx >= 0) {
        items = [
          ...items.slice(0, idx),
          savedSchedule,
          ...items.slice(idx + 1),
        ]
      } else {
        items = [savedSchedule, ...items]
      }
    }
    load()
  }
</script>

<div class="toolbar">
  <span class="toolbar-title">Task Schedules</span>
  <button class="btn btn-primary" onclick={() => (creating = true)}>+ New Schedule</button>
</div>

{#if error}
  <div class="state-msg error-msg">{error}</div>
{:else if loading && items.length === 0}
  <div class="state-msg">Loading…</div>
{:else if items.length === 0}
  <div class="state-msg">No schedules found. Create one to get started.</div>
{:else}
  <table class="data-table">
    <thead>
      <tr>
        <th>ID</th>
        <th>Name</th>
        <th>Type</th>
        <th>Enabled</th>
        <th>Run Times (UTC)</th>
        <th></th>
        <th></th>
      </tr>
    </thead>
    <tbody>
      {#each items as sched (sched.id)}
        <tr>
          <td class="cell-mono">{sched.id}</td>
          <td>{sched.name}</td>
          <td class="cell-mono">{sched.task_type}</td>
          <td>
            {#if sched.enabled}
              <span class="badge badge-enabled">active</span>
            {:else}
              <span class="badge badge-disabled">off</span>
            {/if}
          </td>
          <td class="cell-mono">{(sched.utc_runtimes ?? []).join(', ')}</td>
          <td>
            <button class="btn btn-sm" onclick={() => onEdit(sched.id)} disabled={openingEditId === sched.id}>
              {openingEditId === sched.id ? 'Loading…' : 'Edit'}
            </button>
          </td>
          <td>
            {#if runStatus[sched.id] === 'loading'}
              <span class="cell-mono" style="color:var(--text-muted)">running…</span>
            {:else if runStatus[sched.id] === 'ok'}
              <span class="cell-mono" style="color:var(--accent)">enqueued ✓</span>
            {:else if runStatus[sched.id] === 'error'}
              <span class="cell-mono" style="color:var(--danger)">failed</span>
            {:else}
              <button class="btn btn-sm" onclick={() => onRunNow(sched.id)}
                disabled={!sched.enabled}>Run now</button>
            {/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}

{#if editing}
  <ScheduleModal
    schedule={editing}
    onclose={() => (editing = null)}
    onsave={onSaved}
  />
{/if}

{#if creating}
  <ScheduleModal
    schedule={null}
    onclose={() => (creating = false)}
    onsave={onSaved}
  />
{/if}
