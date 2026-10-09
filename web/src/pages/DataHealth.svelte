<script>
  import { getDataQuality, runJob, listTaskTypes } from '../lib/api.js'
  import { href } from '../lib/router.svelte.js'
  import { relativeTime, dateTime, dateOnly, number } from '../lib/format.js'
  import { providerLabel, countryName } from '../lib/constants.js'
  import { toast } from '../lib/toast.svelte.js'
  import Icon from '../components/Icon.svelte'

  let report = $state(null)
  let taskTypes = $state([])
  let loading = $state(false)
  let error = $state('')
  let busy = $state({})

  async function load() {
    loading = true
    try {
      ;[report, taskTypes] = await Promise.all([getDataQuality(), listTaskTypes().catch(() => [])])
      error = ''
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }
  $effect(() => { load() })

  const JOBS = ['titles.enrich', 'titles.metadata', 'netflix.top10.import', 'imdb.ratings.import', 'ratings.prewarm', 'maintenance']
  const label = (t) => taskTypes.find((x) => x.type === t)?.label ?? t

  async function start(type, force = false) {
    busy = { ...busy, [type]: true }
    try {
      await runJob(type, force ? { force: true } : {})
      toast.success(`${label(type)} queued`)
    } catch (e) {
      toast.error(e.message)
    } finally {
      busy = { ...busy, [type]: false }
    }
  }

  const total = (m) => Object.values(m ?? {}).reduce((a, b) => a + b, 0)
  const pct = (a, b) => (b ? `${Math.round((100 * a) / b)}%` : '—')
  const IMPORT_LABEL = {
    'netflix.global': 'Netflix global weekly', 'netflix.countries': 'Netflix per country', 'netflix.most_popular': 'Netflix most popular', 'imdb.ratings': 'IMDb ratings',
  }
</script>

<div class="page">
  <div class="page-header">
    <div>
      <h1 class="page-title">Data health</h1>
      <p class="page-desc">How complete and fresh the data is: matching coverage, metadata, imports, stale charts and missing dates. Start a job here to fix gaps.</p>
    </div>
    <div class="page-actions">
      <button class="btn" onclick={load} disabled={loading}><Icon name="refresh" />Refresh</button>
    </div>
  </div>

  {#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

  {#if !report}
    <div class="skeleton" style="height:200px"></div>
  {:else}
    {@const tm = report.title_match_status}
    {@const nm = report.netflix_title_match_status}
    <div class="grid grid-stats">
      <a class="card stat" href={href('/titles', { match: 'unmatched', sort: '-rankings' })}>
        <div class="stat-label">FlixPatrol titles matched</div>
        <div class="stat-value">{pct((tm.matched ?? 0) + (tm.manual ?? 0), total(tm))}</div>
        <div class="stat-foot">{number(tm.unmatched ?? 0)} unmatched · {number(tm.pending ?? 0)} pending · {number(tm.manual ?? 0)} by hand</div>
      </a>
      <div class="card stat">
        <div class="stat-label">Netflix titles matched</div>
        <div class="stat-value">{pct(nm.matched ?? 0, total(nm))}</div>
        <div class="stat-foot">{number(nm.matched ?? 0)} of {number(total(nm))} · {number(nm.pending ?? 0)} pending</div>
      </div>
      <div class="card stat">
        <div class="stat-label">TMDB metadata</div>
        <div class="stat-value">{number(report.metadata.with_details)}</div>
        <div class="stat-foot">{number(report.metadata.with_wikidata)} on Wikidata · {number(report.imdb_ratings)} IMDb ratings</div>
      </div>
      <div class="card stat">
        <div class="stat-label">Stale charts</div>
        <div class="stat-value" style:color={report.stale_charts.length ? 'var(--warning)' : ''}>{report.stale_charts.length}</div>
        <div class="stat-foot">{report.chart_gaps.length} chart(s) with missing dates</div>
      </div>
    </div>

    <div class="grid grid-2">
      <div class="card">
        <div class="card-header"><div class="card-title">Jobs</div></div>
        <div class="table-wrap">
          <table class="table">
            <tbody>
              {#each JOBS as t (t)}
                <tr>
                  <td>
                    <div class="cell-title">{label(t)}</div>
                    <div class="small muted">{taskTypes.find((x) => x.type === t)?.description ?? ''}</div>
                  </td>
                  <td class="right nowrap">
                    {#if t.endsWith('.import')}<button class="btn btn-sm" onclick={() => start(t, true)} disabled={busy[t]} title="Re-import even if the file is unchanged">Force</button>{/if}
                    <button class="btn btn-sm" onclick={() => start(t)} disabled={busy[t]}><Icon name="play" size={12} />Run</button>
                    <a class="btn btn-sm" href={href('/runs', { type: t })} aria-label="Runs of {label(t)}"><Icon name="list" size={12} /></a>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>

      <div class="card">
        <div class="card-header"><div class="card-title">Imports</div></div>
        <div class="table-wrap">
          <table class="table">
            <thead><tr><th>File</th><th class="right">Rows</th><th>Imported</th></tr></thead>
            <tbody>
              {#each report.imports as i (i.source)}
                <tr>
                  <td>{IMPORT_LABEL[i.source] ?? i.source}</td>
                  <td class="right">{number(i.rows)}</td>
                  <td class="nowrap" title={dateTime(i.imported_at)}>{relativeTime(i.imported_at)}</td>
                </tr>
              {:else}
                <tr><td colspan="3"><div class="empty small">Nothing imported yet. Run the Netflix and IMDb jobs.</div></td></tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>

      <div class="card">
        <div class="card-header"><div class="card-title">Charts</div></div>
        <div class="table-wrap">
          <table class="table">
            <thead><tr><th>Chart</th><th>Latest date</th><th>Last scraped</th><th>Missing (last {report.gap_window_days} days)</th></tr></thead>
            <tbody>
              {#each report.chart_freshness ?? [] as c (c.country + c.provider)}
                {@const gap = report.chart_gaps.find((g) => g.country === c.country && g.provider === c.provider)}
                {@const stale = report.stale_charts.find((g) => g.country === c.country && g.provider === c.provider)}
                <tr>
                  <td class="nowrap">{providerLabel(c.provider)} · {countryName(c.country)}</td>
                  <td class="nowrap">{dateOnly(c.latest_date)} {#if stale}<span class="badge badge-warning" title="Next chart overdue by {stale.overdue_hours}h">stale</span>{/if}</td>
                  <td class="nowrap muted" title={c.last_scraped_at ? dateTime(c.last_scraped_at) : ''}>{c.last_scraped_at ? relativeTime(c.last_scraped_at) : '—'}</td>
                  <td class="small">{gap ? `${gap.missing_dates.length}: ${gap.missing_dates.slice(0, 4).map((d) => d.slice(5)).join(', ')}${gap.missing_dates.length > 4 ? '…' : ''}` : '—'}</td>
                </tr>
              {:else}
                <tr><td colspan="4"><div class="empty small">No charts stored yet.</div></td></tr>
              {/each}
            </tbody>
          </table>
        </div>
        {#if report.chart_gaps.length}<p class="field-hint" style="padding:0 16px 12px">Fill gaps with a schedule's Backfill button (Schedules), or let each scrape fill its last few days.</p>{/if}
      </div>

      <div class="card">
        <div class="card-header"><div class="card-title">Matching coverage by week</div></div>
        <div class="table-wrap">
          <table class="table">
            <thead><tr><th>Week of</th><th class="right">Chart entries</th><th class="right">TMDB</th><th class="right">Rotten Tomatoes</th></tr></thead>
            <tbody>
              {#each [...report.weekly_coverage].reverse() as w (w.week)}
                <tr><td>{dateOnly(w.first_date)}</td><td class="right">{number(w.entries)}</td><td class="right">{pct(w.mapped, w.entries)}</td><td class="right">{pct(w.with_rt, w.entries)}</td></tr>
              {:else}
                <tr><td colspan="4"><div class="empty small">No charts in the last 12 weeks.</div></td></tr>
              {/each}
            </tbody>
          </table>
        </div>
        {#if report.unmatched_by_country.length}
          <p class="small muted" style="padding:0 16px 12px">Unmatched titles charting in the last {report.gap_window_days} days: {report.unmatched_by_country.map((c) => `${countryName(c.country)} ${c.titles}`).join(' · ')}</p>
        {/if}
      </div>
    </div>
  {/if}
</div>
