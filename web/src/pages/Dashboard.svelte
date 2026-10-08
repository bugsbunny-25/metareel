<script>
  import { getStats, listRuns, runScheduleNow } from '../lib/api.js'
  import { href } from '../lib/router.svelte.js'
  import { relativeTime, dateTime, dateTimeUTC, dateOnly, number, shortError } from '../lib/format.js'
  import { toast } from '../lib/toast.svelte.js'
  import Icon from '../components/Icon.svelte'
  import RunStatus from '../components/RunStatus.svelte'

  let stats = $state(null)
  let failures = $state([])
  let error = $state('')
  let loading = $state(false)
  let running = $state({})

  async function load() {
    loading = true
    try {
      const [s, f] = await Promise.all([getStats(), listRuns({ status: 'failed', limit: 6 })])
      stats = s
      failures = f.items
      error = ''
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  $effect(() => {
    load()
    const t = setInterval(load, 30000)
    return () => clearInterval(t)
  })

  const scheduleName = (id) => stats?.schedule_health.find((h) => h.schedule_id === id)?.name ?? `#${id}`

  function runsSummary(counts) {
    const ok = counts?.succeeded ?? 0
    const failed = counts?.failed ?? 0
    const total = ok + failed
    return { ok, failed, total, rate: total ? Math.round((ok / total) * 100) : null }
  }
  const day = $derived(runsSummary(stats?.runs.last_24h))
  const week = $derived(runsSummary(stats?.runs.last_7d))
  const coverage = $derived(
    stats && stats.titles.total ? Math.round(((stats.titles.total - stats.titles.missing_tmdb) / stats.titles.total) * 100) : null
  )

  // A schedule is stale if enabled and it has not succeeded in over a day.
  const stale = (h) => h.enabled && (!h.last_success_at || Date.now() - Date.parse(h.last_success_at) > 26 * 3600 * 1000)

  async function runNow(h) {
    running = { ...running, [h.schedule_id]: true }
    try {
      await runScheduleNow(h.schedule_id)
      toast.success(`"${h.name}" queued — follow it in Job runs`)
      setTimeout(load, 1500)
    } catch (e) {
      toast.error(e.message)
    } finally {
      running = { ...running, [h.schedule_id]: false }
    }
  }
</script>

<div class="page">
  <div class="page-header">
    <div>
      <h1 class="page-title">Overview</h1>
      <p class="page-desc">Scraper health and data coverage at a glance.</p>
    </div>
    <div class="page-actions">
      {#if stats}<span class="muted small">Updated {relativeTime(stats.generated_at)}</span>{/if}
      <button class="btn" onclick={load} disabled={loading}><Icon name="refresh" />Refresh</button>
    </div>
  </div>

  {#if error}
    <div class="alert alert-error"><Icon name="alert" />{error}</div>
  {/if}

  {#if !stats}
    <div class="grid grid-stats">
      {#each [1, 2, 3, 4] as i (i)}<div class="card stat"><div class="skeleton" style="width:60%"></div><div class="skeleton" style="height:26px;margin-top:10px"></div></div>{/each}
    </div>
  {:else}
    {#if day.failed > 0 && day.ok === 0}
      <div class="alert alert-warning">
        <Icon name="alert" />
        <div>Every job run in the last 24 hours failed. <a href={href('/runs', { status: 'failed' })}>Review the failures</a>.</div>
      </div>
    {/if}

    <div class="grid grid-stats">
      <a class="card stat" href={href('/runs')}>
        <div class="stat-label"><Icon name="list" size={14} />Job success, 24h</div>
        <div class="stat-value">{day.rate === null ? '—' : `${day.rate}%`}</div>
        <div class="stat-foot">{day.ok} succeeded · <span class:move-down={day.failed > 0}>{day.failed} failed</span></div>
        <div class="meter"><span style="width:{day.rate ?? 0}%;background:{day.rate !== null && day.rate < 50 ? 'var(--danger)' : 'var(--success)'}"></span></div>
      </a>
      <a class="card stat" href={href('/runs', { status: 'started' })}>
        <div class="stat-label"><Icon name="play" size={14} />Running now</div>
        <div class="stat-value">{stats.runs.running}</div>
        <div class="stat-foot">7 days: {week.ok} ok · {week.failed} failed</div>
      </a>
      <a class="card stat" href={href('/titles')}>
        <div class="stat-label"><Icon name="film" size={14} />Titles mapped to TMDB</div>
        <div class="stat-value">{coverage === null ? '—' : `${coverage}%`}</div>
        <div class="stat-foot">{number(stats.titles.total - stats.titles.missing_tmdb)} of {number(stats.titles.total)} titles</div>
        <div class="meter"><span style="width:{coverage ?? 0}%"></span></div>
      </a>
      <a class="card stat" href={href('/top10')}>
        <div class="stat-label"><Icon name="chart" size={14} />Latest chart</div>
        <div class="stat-value" style="font-size:18px;margin-top:8px">{dateOnly(stats.rankings.latest_date)}</div>
        <div class="stat-foot">{number(stats.rankings.total)} rankings stored</div>
      </a>
    </div>

    <div class="card">
      <div class="card-header">
        <div class="card-title">Schedules</div>
        <a class="btn btn-sm" href={href('/schedules')}>Manage</a>
      </div>
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr><th>Schedule</th><th>Last run</th><th>Last success</th><th>Next run</th><th></th></tr>
          </thead>
          <tbody>
            {#each stats.schedule_health as h (h.schedule_id)}
              <tr>
                <td>
                  <a class="cell-title" href={href('/runs', { schedule: h.schedule_id })}>{h.name}</a>
                  <div class="cell-sub">{h.enabled ? `Daily at ${h.utc_runtimes.join(', ')} UTC` : 'Disabled'}</div>
                </td>
                <td>
                  {#if h.last_run}
                    <a href={href('/runs', { open: h.last_run.id })} class="row" style="color:inherit" title={dateTime(h.last_run.started_at)}>
                      <RunStatus status={h.last_run.status} />
                      <span class="muted small">{relativeTime(h.last_run.started_at)}</span>
                    </a>
                  {:else}
                    <RunStatus status={null} />
                  {/if}
                </td>
                <td title={h.last_success_at ? `${dateTime(h.last_success_at)} · ${dateTimeUTC(h.last_success_at)}` : ''}>
                  {#if stale(h)}
                    <span class="badge badge-warning"><Icon name="alert" size={12} />{h.last_success_at ? relativeTime(h.last_success_at) : 'never'}</span>
                  {:else}
                    {relativeTime(h.last_success_at)}
                  {/if}
                </td>
                <td title={h.next_run_at ? `${dateTime(h.next_run_at)} · ${dateTimeUTC(h.next_run_at)}` : ''}>
                  {h.next_run_at ? relativeTime(h.next_run_at) : '—'}
                </td>
                <td class="right">
                  <button class="btn btn-sm" onclick={() => runNow(h)} disabled={!h.enabled || running[h.schedule_id]}>
                    <Icon name="play" size={12} />Run now
                  </button>
                </td>
              </tr>
            {:else}
              <tr><td colspan="5" class="empty">No schedules yet. <a href={href('/schedules')}>Create one</a>.</td></tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>

    <div class="grid grid-2" style="margin-top:16px">
      <div class="card">
        <div class="card-header">
          <div class="card-title">Recent failures</div>
          <a class="btn btn-sm" href={href('/runs', { status: 'failed' })}>All failures</a>
        </div>
        {#if failures.length === 0}
          <div class="empty"><div class="empty-title">No failed runs</div>All recent runs succeeded.</div>
        {:else}
          <div class="card-body stack" style="gap:10px">
            {#each failures as r (r.id)}
              <a href={href('/runs', { open: r.id })} style="color:inherit;text-decoration:none">
                <div class="row small">
                  <strong>{scheduleName(r.schedule_id)}</strong>
                  <span class="muted">attempt {r.retry_count + 1}/{r.max_retry + 1}</span>
                  <span class="grow"></span>
                  <span class="muted" title={dateTime(r.started_at)}>{relativeTime(r.started_at)}</span>
                </div>
                <div class="small" style="color:var(--danger)">{shortError(r.error_message, 140)}</div>
              </a>
            {/each}
          </div>
        {/if}
      </div>

      <div class="card">
        <div class="card-header"><div class="card-title">Mapping coverage</div></div>
        <div class="card-body stack" style="gap:12px">
          {#each [['tmdb', 'Missing TMDB ID', stats.titles.missing_tmdb], ['imdb', 'Missing IMDb ID', stats.titles.missing_imdb], ['rt', 'Missing Rotten Tomatoes', stats.titles.missing_rt]] as [key, label, n] (key)}
            <a class="row" href={href('/titles', { missing: key })} style="color:inherit;text-decoration:none">
              <span class="grow">{label}</span>
              <span class="badge {n ? 'badge-warning' : 'badge-success'}">{number(n)}</span>
              <Icon name="chevronRight" size={14} />
            </a>
          {/each}
          <p class="muted small">Open a title to search JustWatch, TMDB and Rotten Tomatoes for the right match and apply it in one click.</p>
        </div>
      </div>
    </div>
  {/if}
</div>
