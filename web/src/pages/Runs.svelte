<script>
  import { listRuns, listSchedules, listTaskTypes } from '../lib/api.js'
  import { route, setQuery, intParam } from '../lib/router.svelte.js'
  import { relativeTime, dateTime, dateTimeUTC, duration, shortError } from '../lib/format.js'
  import Icon from '../components/Icon.svelte'
  import Pagination from '../components/Pagination.svelte'
  import RunStatus from '../components/RunStatus.svelte'
  import RunDrawer from './RunDrawer.svelte'

  const status = $derived(route.query.status ?? '')
  const schedule = $derived(route.query.schedule ?? '')
  const taskType = $derived(route.query.type ?? '')
  const page = $derived(intParam(route.query.page, 1))
  const size = $derived(intParam(route.query.size, 50))
  const openId = $derived(route.query.open ? Number(route.query.open) : null)

  let schedules = $state([])
  let taskTypes = $state([])
  let data = $state({ items: [], total: 0 })
  let loading = $state(false)
  let error = $state('')
  let autoRefresh = $state(true)
  let tick = $state(0)

  $effect(() => {
    listSchedules().then((s) => (schedules = s)).catch(() => {})
    listTaskTypes().then((t) => (taskTypes = t)).catch(() => {})
  })

  $effect(() => {
    const params = { status, schedule_id: schedule, task_type: taskType, limit: size, offset: (page - 1) * size }
    tick
    loading = true
    let cancelled = false
    listRuns(params)
      .then((d) => { if (!cancelled) { data = d; error = '' } })
      .catch((e) => { if (!cancelled) error = e.message })
      .finally(() => { if (!cancelled) loading = false })
    return () => { cancelled = true }
  })

  $effect(() => {
    if (!autoRefresh) return
    const t = setInterval(() => tick++, 10000)
    return () => clearInterval(t)
  })

  const typeLabel = (t) => taskTypes.find((x) => x.type === t)?.label ?? t
  const scheduleName = (id, type) => (id == null ? `Ad hoc · ${typeLabel(type)}` : schedules.find((s) => s.id === id)?.name ?? `Schedule #${id}`)
  const set = (patch) => setQuery({ ...patch, page: '' })
</script>

<div class="page">
  <div class="page-header">
    <div>
      <h1 class="page-title">Job runs</h1>
      <p class="page-desc">Every job attempt with its outcome and logs. Retries of the same job are separate attempts; a partial scrape run failed some charts and is retried for those.</p>
    </div>
    <div class="page-actions">
      <label class="checkbox small"><input type="checkbox" bind:checked={autoRefresh} />Auto-refresh</label>
      <button class="btn" onclick={() => tick++} disabled={loading}><Icon name="refresh" />Refresh</button>
    </div>
  </div>

  <div class="toolbar">
    <div class="segmented" role="group" aria-label="Status">
      {#each [['', 'All'], ['started', 'Running'], ['succeeded', 'Succeeded'], ['partial', 'Partial'], ['failed', 'Failed']] as [value, label] (value)}
        <button class:active={status === value} onclick={() => set({ status: value })}>{label}</button>
      {/each}
    </div>
    <select class="select" value={schedule} onchange={(e) => set({ schedule: e.currentTarget.value })} aria-label="Schedule">
      <option value="">All schedules</option>
      {#each schedules as s (s.id)}<option value={String(s.id)}>{s.name}</option>{/each}
    </select>
    <select class="select" value={taskType} onchange={(e) => set({ type: e.currentTarget.value })} aria-label="Job type">
      <option value="">All jobs</option>
      {#each taskTypes as t (t.type)}<option value={t.type}>{t.label}</option>{/each}
    </select>
    <span class="spacer"></span>
    {#if loading}<span class="spinner"></span>{/if}
  </div>

  {#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

  <div class="card">
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr><th>Run</th><th>Schedule</th><th>Status</th><th>Charts</th><th>Attempt</th><th>Started</th><th class="right">Duration</th><th>Error</th></tr>
        </thead>
        <tbody>
          {#each data.items as r (r.id)}
            <tr class="clickable" class:selected={openId === r.id} onclick={() => setQuery({ open: r.id })}>
              <td class="mono">#{r.id}</td>
              <td class="nowrap">{scheduleName(r.schedule_id, r.task_type)}</td>
              <td><RunStatus status={r.status} /></td>
              <td class="nowrap small">
                {#if r.targets_total}
                  <span title="saved / skipped (stored or not published yet) / failed">{r.targets_succeeded} saved{#if r.targets_skipped} · {r.targets_skipped} skipped{/if}{#if r.targets_failed} · <span style="color:var(--danger)">{r.targets_failed} failed</span>{/if}</span>
                {:else}<span class="muted">—</span>{/if}
              </td>
              <td class="nowrap muted">{r.retry_count + 1} / {r.max_retry + 1}</td>
              <td class="nowrap" title="{dateTime(r.started_at)} · {dateTimeUTC(r.started_at)}">{relativeTime(r.started_at)}</td>
              <td class="right nowrap mono">{r.error_message?.startsWith('interrupted') ? '—' : duration(r.started_at, r.finished_at)}</td>
              <td class="small" style="color:var(--danger);max-width:420px">{shortError(r.error_message, 110)}</td>
            </tr>
          {:else}
            <tr><td colspan="8">
              <div class="empty">{#if loading}Loading…{:else}<div class="empty-title">No runs</div>Nothing matches these filters yet.{/if}</div>
            </td></tr>
          {/each}
        </tbody>
      </table>
    </div>
    <Pagination {page} {size} total={data.total} onchange={(p, s) => setQuery({ page: p === 1 ? '' : p, size: s === 50 ? '' : s })} />
  </div>
</div>

{#if openId}
  <RunDrawer id={openId} {scheduleName} onclose={() => setQuery({ open: '' })} onchange={() => tick++} />
{/if}
