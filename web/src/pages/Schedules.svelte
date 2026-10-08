<script>
  import { listSchedules, getSchedule, updateSchedule, runScheduleNow, getStats } from '../lib/api.js'
  import { href } from '../lib/router.svelte.js'
  import { relativeTime, dateTime, dateTimeUTC, utcTimeToLocal } from '../lib/format.js'
  import { providerLabel } from '../lib/constants.js'
  import { toast } from '../lib/toast.svelte.js'
  import Icon from '../components/Icon.svelte'
  import RunStatus from '../components/RunStatus.svelte'
  import ScheduleModal from '../components/ScheduleModal.svelte'

  let schedules = $state([])
  let health = $state({})
  let loading = $state(false)
  let error = $state('')
  let editing = $state(null) // schedule object, or {} for a new one
  let busy = $state({})

  async function load() {
    loading = true
    try {
      const list = await listSchedules()
      const [details, stats] = await Promise.all([Promise.all(list.map((s) => getSchedule(s.id))), getStats().catch(() => null)])
      schedules = details
      health = Object.fromEntries((stats?.schedule_health ?? []).map((h) => [h.schedule_id, h]))
      error = ''
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  $effect(() => { load() })

  function payload(s, patch = {}) {
    return {
      name: s.name, enabled: s.enabled, utc_runtimes: s.utc_runtimes, max_retries: s.max_retries,
      request_delay_seconds: s.request_delay_seconds, respect_robots: s.respect_robots, user_agent: s.user_agent, ...patch,
    }
  }

  async function toggle(s) {
    busy = { ...busy, [s.id]: true }
    try {
      const updated = await updateSchedule(s.id, payload(s, { enabled: !s.enabled }))
      schedules = schedules.map((x) => (x.id === s.id ? { ...x, ...updated } : x))
      toast.success(`"${s.name}" ${updated.enabled ? 'enabled' : 'disabled'}`)
      load()
    } catch (e) {
      toast.error(e.message)
    } finally {
      busy = { ...busy, [s.id]: false }
    }
  }

  async function runNow(s) {
    busy = { ...busy, [s.id]: true }
    try {
      await runScheduleNow(s.id)
      toast.success(`"${s.name}" queued`)
    } catch (e) {
      toast.error(e.message)
    } finally {
      busy = { ...busy, [s.id]: false }
    }
  }
</script>

<div class="page">
  <div class="page-header">
    <div>
      <h1 class="page-title">Schedules</h1>
      <p class="page-desc">When each FlixPatrol scrape runs and which countries and providers it covers. Times are UTC.</p>
    </div>
    <div class="page-actions">
      <button class="btn" onclick={load} disabled={loading}><Icon name="refresh" />Refresh</button>
      <button class="btn btn-primary" onclick={() => (editing = {})}><Icon name="plus" />New schedule</button>
    </div>
  </div>

  {#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

  <div class="card">
    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr><th>Enabled</th><th>Schedule</th><th>Runs at</th><th>Next run</th><th>Last run</th><th></th></tr>
        </thead>
        <tbody>
          {#each schedules as s (s.id)}
            {@const h = health[s.id]}
            <tr>
              <td>
                <button class="switch" class:on={s.enabled} role="switch" aria-checked={s.enabled} aria-label="Enabled" disabled={busy[s.id]} onclick={() => toggle(s)}></button>
              </td>
              <td>
                <div class="cell-title">{s.name}</div>
                <div class="row" style="flex-wrap:wrap;gap:4px;margin-top:4px">
                  {#each s.flixpatrol_targets ?? [] as t (t.country_code + t.provider_slug)}
                    <span class="chip">{t.country_code} · {providerLabel(t.provider_slug)}</span>
                  {/each}
                </div>
              </td>
              <td class="nowrap">
                {#each s.utc_runtimes as rt (rt)}
                  <div><span class="mono">{rt}</span> <span class="muted small">({utcTimeToLocal(rt)} local)</span></div>
                {/each}
              </td>
              <td class="nowrap" title={h?.next_run_at ? `${dateTime(h.next_run_at)} · ${dateTimeUTC(h.next_run_at)}` : ''}>
                {s.enabled && h?.next_run_at ? relativeTime(h.next_run_at) : '—'}
              </td>
              <td class="nowrap">
                {#if h?.last_run}
                  <a href={href('/runs', { open: h.last_run.id })} class="row" style="color:inherit">
                    <RunStatus status={h.last_run.status} /><span class="muted small">{relativeTime(h.last_run.started_at)}</span>
                  </a>
                {:else}<span class="muted">Never</span>{/if}
              </td>
              <td class="right nowrap">
                <button class="btn btn-sm" onclick={() => runNow(s)} disabled={!s.enabled || busy[s.id]} title={s.enabled ? 'Run now' : 'Enable to run'}><Icon name="play" size={12} />Run</button>
                <a class="btn btn-sm" href={href('/runs', { schedule: s.id })}><Icon name="list" size={12} />Runs</a>
                <button class="btn btn-sm" onclick={() => (editing = s)}><Icon name="edit" size={12} />Edit</button>
              </td>
            </tr>
          {:else}
            <tr><td colspan="6"><div class="empty">{#if loading}Loading…{:else}<div class="empty-title">No schedules</div>Create one to start scraping charts.{/if}</div></td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
</div>

{#if editing}
  <ScheduleModal schedule={editing.id ? editing : null} onclose={() => (editing = null)} onsaved={() => { editing = null; load() }} />
{/if}
