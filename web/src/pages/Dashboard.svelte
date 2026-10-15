<script>
  import { getStats, listRuns, runScheduleNow, getDataQuality } from '../lib/api.js'
  import { href } from '../lib/router.svelte.js'
  import { relativeTime, dateTime, dateTimeUTC, dateOnly, number, shortError } from '../lib/format.js'
  import { toast } from '../lib/toast.svelte.js'
  import { define } from '../lib/glossary.js'
  import { RUN_SERIES, COVERAGE_SERIES, shortDay, runRows } from '../lib/charts.js'
  import Icon from '../components/Icon.svelte'
  import RunStatus from '../components/RunStatus.svelte'
  import PageHeader from '../components/PageHeader.svelte'
  import Kpi from '../components/Kpi.svelte'
  import ColumnChart from '../components/ColumnChart.svelte'
  import LineChart from '../components/LineChart.svelte'
  import Legend from '../components/Legend.svelte'
  import HBars from '../components/HBars.svelte'
  import InfoTip from '../components/InfoTip.svelte'

  let stats = $state(null)
  let quality = $state(null)
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

  // The data-quality report is heavier; refresh it less often and treat it as optional.
  async function loadQuality() {
    try {
      quality = await getDataQuality()
    } catch {
      quality = null
    }
  }

  $effect(() => {
    load()
    loadQuality()
    const t = setInterval(load, 30000)
    const q = setInterval(loadQuality, 300000)
    return () => { clearInterval(t); clearInterval(q) }
  })

  const scheduleName = (id) => stats?.schedule_health.find((h) => h.schedule_id === id)?.name ?? `#${id}`

  function runsSummary(counts) {
    const ok = (counts?.succeeded ?? 0) + (counts?.partial ?? 0)
    const failed = counts?.failed ?? 0
    const total = ok + failed
    return { ok, failed, partial: counts?.partial ?? 0, total, rate: total ? Math.round((ok / total) * 100) : null }
  }
  const day = $derived(runsSummary(stats?.runs.last_24h))
  const week = $derived(runsSummary(stats?.runs.last_7d))
  const daily = $derived(stats?.runs.daily ?? [])
  const dailyRate = $derived(daily.map((d) => {
    const t = d.succeeded + d.partial + d.failed
    return t ? ((d.succeeded + d.partial) / t) * 100 : null
  }).filter((v) => v !== null))
  const dailyTotal = $derived(daily.map((d) => d.succeeded + d.partial + d.failed + d.started))


  const share = (missing) => (stats?.titles.total ? ((stats.titles.total - missing) / stats.titles.total) * 100 : 0)
  const coverage = $derived(stats?.titles.total ? Math.round(share(stats.titles.missing_tmdb)) : null)
  const idCoverage = $derived(stats ? [
    { key: 'tmdb', label: 'TMDB', missing: stats.titles.missing_tmdb },
    { key: 'imdb', label: 'IMDb', missing: stats.titles.missing_imdb },
    { key: 'rt', label: 'Rotten Tomatoes', missing: stats.titles.missing_rt },
  ].map((c) => ({
    key: c.key,
    label: c.label,
    value: share(c.missing),
    display: `${Math.round(share(c.missing))}%`,
    sub: `${number(c.missing)} missing`,
    href: href('/titles', { missing: c.key }),
    title: `${number(stats.titles.total - c.missing)} of ${number(stats.titles.total)} titles have a ${c.label} ID. Open the ones without.`,
    color: share(c.missing) < 80 ? 'var(--c-partial)' : 'var(--c1)',
  })) : [])

  const coverageWeeks = $derived((quality?.weekly_coverage ?? []).map((w) => ({
    label: w.first_date,
    title: `Week of ${dateOnly(w.first_date)} · ${number(w.entries)} chart entries`,
    mapped: w.entries ? w.mapped / w.entries : null,
    rt: w.entries ? w.with_rt / w.entries : null,
  })))

  // A schedule is stale if enabled and it has not succeeded in over a day.
  const stale = (h) => h.enabled && (!h.last_success_at || Date.now() - Date.parse(h.last_success_at) > 26 * 3600 * 1000)
  const chartAgeDays = $derived(stats?.rankings.latest_date ? Math.floor((Date.now() - Date.parse(`${stats.rankings.latest_date}T00:00:00Z`)) / 864e5) : null)

  // Things that need a person, most urgent first. Each links to where it is fixed.
  const attention = $derived.by(() => {
    if (!stats) return []
    const out = []
    const staleSchedules = stats.schedule_health.filter(stale)
    if (staleSchedules.length) out.push({ tone: 'danger', icon: 'clock', text: `${staleSchedules.length} schedule${staleSchedules.length > 1 ? 's have' : ' has'} not succeeded in over a day`, detail: staleSchedules.map((h) => h.name).join(', '), href: href('/schedules') })
    if (day.failed) out.push({ tone: 'danger', icon: 'alert', text: `${day.failed} failed job run${day.failed > 1 ? 's' : ''} in the last 24 hours`, detail: 'Open a run to read its error and logs', href: href('/runs', { status: 'failed' }) })
    if (quality?.stale_charts?.length) out.push({ tone: 'warning', icon: 'chart', text: `${quality.stale_charts.length} chart${quality.stale_charts.length > 1 ? 's are' : ' is'} behind`, detail: 'The latest stored date is older than expected', href: href('/data-health') })
    if (quality?.chart_gaps?.length) out.push({ tone: 'warning', icon: 'calendar', text: `${quality.chart_gaps.length} chart${quality.chart_gaps.length > 1 ? 's have' : ' has'} missing days`, detail: `In the last ${quality.gap_window_days} days; a backfill can fetch them`, href: href('/data-health') })
    const unmatched = quality?.title_match_status?.unmatched ?? 0
    if (unmatched) out.push({ tone: 'warning', icon: 'link', text: `${number(unmatched)} FlixPatrol title${unmatched > 1 ? 's' : ''} could not be matched`, detail: 'Pick the right TMDB title by hand', href: href('/titles', { match: 'unmatched', sort: '-rankings' }) })
    const nfUnmatched = quality?.netflix_title_match_status?.unmatched ?? 0
    if (nfUnmatched) out.push({ tone: 'warning', icon: 'tv', text: `${number(nfUnmatched)} Netflix title${nfUnmatched > 1 ? 's' : ''} could not be matched`, detail: 'Pick the right TMDB title by hand', href: href('/netflix', { match: 'unmatched' }) })
    return out
  })

  async function runNow(h) {
    running = { ...running, [h.schedule_id]: true }
    try {
      await runScheduleNow(h.schedule_id)
      toast.success(`"${h.name}" queued. Follow it in Job runs.`)
      setTimeout(load, 1500)
    } catch (e) {
      toast.error(e.message)
    } finally {
      running = { ...running, [h.schedule_id]: false }
    }
  }
</script>

<div class="page">
  <PageHeader
    title="Overview"
    desc="Whether the scrapers are running, how fresh the charts are and how much of the data is mapped."
    info="metareel scrapes daily streaming Top 10 charts, links every title to TMDB, IMDb and Rotten Tomatoes, and serves the result through an API. This page shows whether that pipeline is healthy."
  >
    {#snippet actions()}
      {#if stats}<span class="muted small" title={dateTime(stats.generated_at)}>Updated {relativeTime(stats.generated_at)}</span>{/if}
      <button class="btn" onclick={() => { load(); loadQuality() }} disabled={loading}><Icon name="refresh" />Refresh</button>
    {/snippet}
  </PageHeader>

  {#if error}
    <div class="alert alert-error"><Icon name="alert" />{error}</div>
  {/if}

  {#if !stats}
    <div class="grid grid-stats">
      {#each [1, 2, 3, 4] as i (i)}<div class="card stat"><div class="skeleton" style="width:60%"></div><div class="skeleton" style="height:26px;margin-top:10px"></div><div class="skeleton" style="height:28px;margin-top:12px"></div></div>{/each}
    </div>
    <div class="skeleton" style="height:260px;border-radius:var(--radius)"></div>
  {:else}
    {#if day.failed > 0 && day.ok === 0}
      <div class="alert alert-warning">
        <Icon name="alert" />
        <div>Every job run in the last 24 hours failed. <a href={href('/runs', { status: 'failed' })}>Review the failures</a>.</div>
      </div>
    {/if}

    <div class="grid grid-stats">
      <Kpi
        label="Job success, 24h"
        icon="activity"
        value={day.rate === null ? '—' : day.rate}
        unit={day.rate === null ? '' : '%'}
        info="The share of job runs in the last 24 hours that finished. Partial scrapes count as finished. The line shows the daily rate over 14 days."
        href={href('/runs')}
        spark={dailyRate}
        sparkColor={day.rate !== null && day.rate < 80 ? 'var(--c-fail)' : 'var(--c-ok)'}
        tone={day.rate !== null && day.rate < 50 ? 'danger' : ''}
      >
        {#snippet foot()}{day.ok} finished · <span class:move-down={day.failed > 0}>{day.failed} failed</span>{/snippet}
      </Kpi>
      <Kpi
        label="Runs, 7 days"
        icon="list"
        value={number(week.total)}
        info="Job attempts started in the last 7 days. Retries count separately. The line shows runs per day over 14 days."
        href={href('/runs')}
        spark={dailyTotal}
        sparkColor="var(--c6)"
      >
        {#snippet foot()}{stats.runs.running ? `${stats.runs.running} running now` : 'Nothing running now'} · {week.failed} failed{/snippet}
      </Kpi>
      <Kpi
        label="Titles mapped to TMDB"
        icon="link"
        value={coverage === null ? '—' : coverage}
        unit={coverage === null ? '' : '%'}
        info={define('Coverage')}
        href={href('/titles', { missing: 'tmdb' })}
        meter={coverage}
        meterColor={coverage !== null && coverage < 80 ? 'var(--c-partial)' : 'var(--c1)'}
        tone={coverage !== null && coverage < 60 ? 'warning' : ''}
      >
        {#snippet foot()}{number(stats.titles.total - stats.titles.missing_tmdb)} of {number(stats.titles.total)} FlixPatrol titles{/snippet}
      </Kpi>
      <Kpi
        label="Latest chart"
        icon="chart"
        value={stats.rankings.latest_date ? shortDay(stats.rankings.latest_date) : '—'}
        info="The most recent chart date stored. Charts are published daily, so a date more than a day or two old means scrapes are falling behind."
        href={href('/top10')}
        tone={chartAgeDays !== null && chartAgeDays > 2 ? 'warning' : ''}
      >
        {#snippet foot()}
          {#if chartAgeDays !== null}<span class:move-down={chartAgeDays > 2}>{chartAgeDays === 0 ? 'today' : chartAgeDays === 1 ? 'yesterday' : `${chartAgeDays} days ago`}</span> · {/if}{number(stats.rankings.total)} rankings stored
        {/snippet}
      </Kpi>
    </div>

    <div class="grid grid-main">
      <div class="card">
        <div class="card-header">
          <div>
            <div class="card-title">Job runs per day <InfoTip text={define('Job run')} /></div>
            <div class="card-sub">Last 14 days (UTC). Hover a day for its counts.</div>
          </div>
          <Legend series={RUN_SERIES} />
        </div>
        <div class="card-body" style="padding-top:12px">
          <ColumnChart data={runRows(daily)} series={RUN_SERIES} height={200} formatLabel={shortDay} labelEvery={2} ariaLabel="Job runs per day by outcome, last 14 days" />
        </div>
      </div>

      <div class="card">
        <div class="card-header">
          <div>
            <div class="card-title">Needs attention</div>
            <div class="card-sub">Problems, most urgent first</div>
          </div>
          <span class="badge {attention.length ? 'badge-warning' : 'badge-success'}">{attention.length || 'All clear'}</span>
        </div>
        {#if attention.length === 0}
          <div class="empty" style="padding:36px 20px">
            <div class="empty-title"><Icon name="check" /> Nothing to fix</div>
            Schedules are succeeding and charts are up to date.
          </div>
        {:else}
          <div class="attention">
            {#each attention as a (a.text)}
              <a class="attention-item" href={a.href}>
                <span class="attention-icon tone-{a.tone}"><Icon name={a.icon} size={14} /></span>
                <span class="grow" style="min-width:0">
                  <span class="attention-text">{a.text}</span>
                  <span class="cell-sub truncate" style="display:block">{a.detail}</span>
                </span>
                <Icon name="chevronRight" size={14} />
              </a>
            {/each}
          </div>
        {/if}
      </div>
    </div>

    <div class="grid grid-main mt">
      <div class="card">
        <div class="card-header">
          <div>
            <div class="card-title">Mapping coverage over time <InfoTip text="Of all chart entries each week, the share whose title has a TMDB ID or a Rotten Tomatoes ID. A drop usually means new titles are waiting to be matched." /></div>
            <div class="card-sub">Share of weekly chart entries with each ID</div>
          </div>
          <Legend series={COVERAGE_SERIES} />
        </div>
        <div class="card-body" style="padding-top:12px">
          {#if coverageWeeks.length > 1}
            <LineChart data={coverageWeeks} series={COVERAGE_SERIES} percent height={200} formatLabel={shortDay} labelEvery={Math.max(1, Math.ceil(coverageWeeks.length / 6))} ariaLabel="Weekly share of chart entries with a TMDB or Rotten Tomatoes ID" />
          {:else if quality === null}
            <div class="skeleton" style="height:200px"></div>
          {:else}
            <div class="empty">Not enough weeks of charts yet.</div>
          {/if}
        </div>
      </div>

      <div class="card">
        <div class="card-header">
          <div>
            <div class="card-title">IDs per title <InfoTip text={define('Mapping')} /></div>
            <div class="card-sub">FlixPatrol titles with each ID. Click to see the ones without.</div>
          </div>
        </div>
        <div class="card-body">
          <HBars items={idCoverage} max={100} />
          {#if quality?.metadata}
            <div class="divider"></div>
            <dl class="kv small" style="grid-template-columns:1fr auto">
              <dt>TMDB titles with details <InfoTip text="TMDB titles whose genres, release date and poster have been fetched by the metadata job." /></dt>
              <dd class="right num">{number(quality.metadata.with_details)} / {number(quality.metadata.titles)}</dd>
              <dt>IMDb ratings stored</dt>
              <dd class="right num">{number(quality.imdb_ratings)}</dd>
            </dl>
          {/if}
        </div>
      </div>
    </div>

    <div class="card mt">
      <div class="card-header">
        <div>
          <div class="card-title">Schedules <InfoTip text={define('Schedule')} /></div>
          <div class="card-sub">{stats.schedules.enabled} of {stats.schedules.total} enabled. Times are UTC.</div>
        </div>
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
                    <span class="badge badge-warning" title="No success in over a day"><Icon name="alert" size={12} />{h.last_success_at ? relativeTime(h.last_success_at) : 'never'}</span>
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

    <div class="card mt">
      <div class="card-header">
        <div>
          <div class="card-title">Recent failures</div>
          <div class="card-sub">The last failed attempts and why they failed</div>
        </div>
        <a class="btn btn-sm" href={href('/runs', { status: 'failed' })}>All failures</a>
      </div>
      {#if failures.length === 0}
        <div class="empty"><div class="empty-title">No failed runs</div>Recent runs all succeeded.</div>
      {:else}
        <div class="table-wrap">
          <table class="table">
            <tbody>
              {#each failures as r (r.id)}
                <tr class="clickable" onclick={() => (location.hash = href('/runs', { open: r.id }))}>
                  <td style="width:28%"><a class="cell-title" href={href('/runs', { open: r.id })} onclick={(e) => e.stopPropagation()}>{r.schedule_id ? scheduleName(r.schedule_id) : r.task_type}</a><div class="cell-sub">attempt {r.retry_count + 1} of {r.max_retry + 1}</div></td>
                  <td class="small" style="color:var(--danger)">{shortError(r.error_message, 160)}</td>
                  <td class="right muted small nowrap" title={dateTime(r.started_at)}>{relativeTime(r.started_at)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .attention { display: flex; flex-direction: column; }
  .attention-item { display: flex; align-items: center; gap: 12px; padding: 11px 16px; border-bottom: 1px solid var(--border); color: var(--text); }
  .attention-item:last-child { border-bottom: 0; }
  .attention-item:hover { background: var(--surface-2); text-decoration: none; }
  .attention-item :global(.icon:last-child) { color: var(--muted); flex-shrink: 0; }
  .attention-text { font-weight: 550; display: block; }
  .attention-icon { width: 28px; height: 28px; border-radius: 8px; display: grid; place-items: center; flex-shrink: 0; }
  .tone-danger { background: var(--danger-soft); color: var(--danger); }
  .tone-warning { background: var(--warning-soft); color: var(--warning); }
  .divider { height: 1px; background: var(--border); margin: 16px 0 12px; }
</style>
