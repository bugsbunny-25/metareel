<script>
  import { getDataQuality, runJob, listTaskTypes } from '../lib/api.js'
  import { href } from '../lib/router.svelte.js'
  import { relativeTime, dateTime, dateOnly, number, links, ID_SOURCES } from '../lib/format.js'
  import { providerLabel, countryName } from '../lib/constants.js'
  import { toast } from '../lib/toast.svelte.js'
  import Icon from '../components/Icon.svelte'
  import PageHeader from '../components/PageHeader.svelte'
  import Kpi from '../components/Kpi.svelte'
  import SegmentBar from '../components/SegmentBar.svelte'
  import LineChart from '../components/LineChart.svelte'
  import Legend from '../components/Legend.svelte'
  import HBars from '../components/HBars.svelte'
  import InfoTip from '../components/InfoTip.svelte'
  import { define } from '../lib/glossary.js'
  import { COVERAGE_SERIES, shortDay } from '../lib/charts.js'

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

  const JOBS = ['titles.enrich', 'titles.metadata', 'netflix.top10.import', 'netflix.titles.match', 'imdb.ratings.import', 'ratings.prewarm', 'maintenance']
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
  const IMPORT_LABEL = {
    'netflix.global': 'Netflix global weekly', 'netflix.countries': 'Netflix per country', 'netflix.most_popular': 'Netflix most popular', 'imdb.ratings': 'IMDb ratings',
  }

  const share = (a, b) => (b ? Math.round((100 * a) / b) : null)
  const matchSegments = (m, path) => [
    { key: 'matched', label: 'Matched automatically', value: m?.matched ?? 0, color: 'var(--c-ok)', href: href(path, { match: 'matched' }) },
    { key: 'manual', label: 'Fixed by hand', value: m?.manual ?? 0, color: 'var(--c1)', href: href(path, { match: 'manual' }) },
    { key: 'pending', label: 'Not tried yet', value: m?.pending ?? 0, color: 'var(--c6)', href: href(path, { match: 'pending' }) },
    { key: 'unmatched', label: 'No confident match', value: m?.unmatched ?? 0, color: 'var(--c-fail)', href: href(path, { match: 'unmatched' }) },
  ]

  // What decided an IMDb-mismatch entry's mapping.
  const EVIDENCE_LABEL = { manual: 'fixed by hand', netflix_id: 'Netflix ID', match: 'automatic match' }
  // Netflix ID disagreements worth a look: not fixed by hand, and not a
  // special / part of a show whose own Netflix title has the TMDB title's ID.
  const suspectNetflixIDs = $derived((report?.netflix_id_mismatches ?? []).filter((m) => m.match_status !== 'manual' && !m.other_title_has_id))
</script>

<div class="page">
  <PageHeader
    title="Data health"
    desc="How complete and up to date the stored data is, and the jobs that fill the gaps."
    info="Matching links titles to TMDB, metadata adds genres and release dates, imports load Netflix and IMDb files, and scrapes keep charts current. Each section shows what is missing and where to fix it."
  >
    {#snippet actions()}
      <button class="btn" onclick={load} disabled={loading}><Icon name="refresh" />Refresh</button>
    {/snippet}
  </PageHeader>

  {#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

  {#if !report}
    <div class="skeleton" style="height:200px"></div>
  {:else}
    {@const tm = report.title_match_status}
    {@const nm = report.netflix_title_match_status}
    {@const ns = report.netflix_season_mapping ?? {}}
    {@const fpShare = share((tm.matched ?? 0) + (tm.manual ?? 0), total(tm))}
    {@const nfShare = share((nm.matched ?? 0) + (nm.manual ?? 0), total(nm))}
    {@const seasonShare = share(total(ns) - (ns.unmapped ?? 0), total(ns))}
    <div class="grid grid-stats">
      <Kpi label="FlixPatrol titles matched" value={fpShare ?? '—'} unit={fpShare === null ? '' : '%'} info={define('Match status')} href={href('/titles', { match: 'unmatched', sort: '-rankings' })} meter={fpShare} meterColor="var(--c1)" tone={fpShare !== null && fpShare < 60 ? 'warning' : ''}>
        {#snippet foot()}{number(tm.unmatched ?? 0)} without a match · {number(tm.pending ?? 0)} waiting{/snippet}
      </Kpi>
      <Kpi label="Netflix titles matched" value={nfShare ?? '—'} unit={nfShare === null ? '' : '%'} info="Titles from Netflix's official Top 10 that have a TMDB ID." href={href('/netflix', { match: 'unmatched' })} meter={nfShare} meterColor="var(--c1)" tone={nfShare !== null && nfShare < 60 ? 'warning' : ''}>
        {#snippet foot()}{number((nm.matched ?? 0) + (nm.manual ?? 0))} of {number(total(nm))} · {number(nm.unmatched ?? 0)} without a match{/snippet}
      </Kpi>
      <Kpi label="Netflix seasons numbered" value={seasonShare ?? '—'} unit={seasonShare === null ? '' : '%'} info={define('Season mapping')} href={href('/netflix', { missing: 'season', kind: 'tv_show' })} meter={seasonShare} meterColor="var(--c2)">
        {#snippet foot()}{number(ns.unmapped ?? 0)} without a number · {number((ns.tmdb_air_date ?? 0) + (ns.number ?? 0))} inferred{/snippet}
      </Kpi>
      <Kpi label="TMDB titles with details" value={number(report.metadata.with_details)} info="TMDB titles whose genres, release date and poster have been fetched by the metadata job." meter={share(report.metadata.with_details, report.metadata.titles)} meterColor="var(--c4)">
        {#snippet foot()}of {number(report.metadata.titles)} · {number(report.imdb_ratings)} IMDb ratings{/snippet}
      </Kpi>
      <Kpi label="Stale charts" value={report.stale_charts.length} info={define('Stale chart')} tone={report.stale_charts.length ? 'warning' : ''}>
        {#snippet foot()}{report.chart_gaps.length} chart{report.chart_gaps.length === 1 ? '' : 's'} with missing days <InfoTip text={define('Chart gap')} />{/snippet}
      </Kpi>
    </div>

    <div class="grid grid-2" style="margin-bottom:16px">
      <div class="card">
        <div class="card-header"><div><div class="card-title">FlixPatrol title matching</div><div class="card-sub">{number(total(tm))} titles by match status. Click a status to list them.</div></div></div>
        <div class="card-body"><SegmentBar segments={matchSegments(tm, '/titles')} label="FlixPatrol titles by match status" /></div>
      </div>
      <div class="card">
        <div class="card-header"><div><div class="card-title">Netflix title matching</div><div class="card-sub">{number(total(nm))} titles by match status. Click a status to list them.</div></div></div>
        <div class="card-body"><SegmentBar segments={matchSegments(nm, '/netflix')} label="Netflix titles by match status" /></div>
      </div>
    </div>

    <div class="grid grid-2">
      <div class="card">
        <div class="card-header"><div><div class="card-title">Jobs</div><div class="card-sub">Run a job now instead of waiting for its schedule</div></div></div>
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
        <div class="card-header"><div><div class="card-title">Imports</div><div class="card-sub">Files loaded from Netflix and IMDb</div></div></div>
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
        <div class="card-header"><div><div class="card-title">Chart freshness</div><div class="card-sub">Each chart's latest date and missing days</div></div></div>
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
        <div class="card-header">
          <div>
            <div class="card-title">Mapping coverage by week <InfoTip text={define('Coverage')} /></div>
            <div class="card-sub">Share of each week's chart entries with an ID</div>
          </div>
          <Legend series={COVERAGE_SERIES} />
        </div>
        <div class="card-body" style="padding-top:12px">
          {#if report.weekly_coverage.length > 1}
            <LineChart
              data={report.weekly_coverage.map((w) => ({ label: w.first_date, title: `Week of ${dateOnly(w.first_date)} · ${number(w.entries)} entries`, mapped: w.entries ? w.mapped / w.entries : null, rt: w.entries ? w.with_rt / w.entries : null }))}
              series={COVERAGE_SERIES}
              percent
              height={190}
              formatLabel={shortDay}
              labelEvery={Math.max(1, Math.ceil(report.weekly_coverage.length / 6))}
              ariaLabel="Weekly share of chart entries with a TMDB or Rotten Tomatoes ID"
            />
          {:else}<div class="empty small">Not enough weeks of charts yet.</div>{/if}
          {#if report.unmatched_by_country.length}
            <div class="section-title" style="margin-top:18px">Unmatched titles charting in the last {report.gap_window_days} days, by country</div>
            <HBars color="var(--c-fail)" items={report.unmatched_by_country.slice(0, 8).map((c) => ({ key: c.country, label: countryName(c.country), value: c.titles }))} />
          {/if}
        </div>
      </div>

      <div class="card">
        <div class="card-header">
          <div class="card-title">IMDb IDs that disagree with TMDB</div>
          <span class="badge {(report.imdb_mismatches ?? []).length ? 'badge-warning' : 'badge-success'}">{(report.imdb_mismatches ?? []).length}</span>
        </div>
        <div class="table-wrap">
          <table class="table">
            <thead><tr><th>Title</th><th>TMDB title</th><th>Match's IMDb</th><th>TMDB title's IMDb</th></tr></thead>
            <tbody>
              {#each report.imdb_mismatches ?? [] as m (m.subject_type + m.subject_id)}
                <tr>
                  <td>
                    <a href={m.subject_type === 'netflix' ? href('/netflix', { open: m.subject_id }) : href('/titles', { open: m.subject_id })}>{m.name}</a>
                    <div class="cell-sub">{m.subject_type === 'netflix' ? 'Netflix' : 'FlixPatrol'} · {EVIDENCE_LABEL[m.evidence_kind] ?? m.evidence_kind}{m.source && m.source !== m.evidence_kind ? ` (${m.source})` : ''}</div>
                  </td>
                  <td class="mono"><a href={href('/mapping', { open: `${m.tmdb_kind}:${m.tmdb_id}` })}>{m.tmdb_kind} {m.tmdb_id}</a></td>
                  <td class="mono">{m.imdb_id}</td>
                  <td class="mono">{m.tmdb_imdb_id}</td>
                </tr>
              {:else}
                <tr><td colspan="4"><div class="empty small">Every mapped title's IMDb ID matches its TMDB title's.</div></td></tr>
              {/each}
            </tbody>
          </table>
        </div>
        {#if (report.imdb_mismatches ?? []).length}
          <p class="field-hint" style="padding:0 16px 12px">The match or fix that decides the title's mapping came with another IMDb ID than its TMDB title has, so one of the two is wrong. A wrong mapping: fix it on the title. A wrong IMDb ID on the TMDB title: enter the right one on the title (it becomes the TMDB title's, set by hand). Saving the mapping by hand confirms it and clears the entry.</p>
        {/if}
      </div>

      <div class="card">
        <div class="card-header">
          <div class="card-title">Netflix IDs that disagree with TMDB</div>
          <span class="badge {suspectNetflixIDs.length ? 'badge-warning' : 'badge-success'}" title="{suspectNetflixIDs.length} to check of {(report.netflix_id_mismatches ?? []).length}">{suspectNetflixIDs.length}</span>
        </div>
        <div class="table-wrap">
          <table class="table">
            <thead><tr><th>Netflix title</th><th>TMDB title</th><th>Title's Netflix ID</th><th>TMDB title's Netflix ID</th><th></th></tr></thead>
            <tbody>
              {#each report.netflix_id_mismatches ?? [] as m (m.netflix_title_id)}
                <tr>
                  <td>
                    <a href={href('/netflix', { open: m.netflix_title_id })}>{m.show_title}</a>
                    <div class="cell-sub">{m.list === 'tv_show' ? 'TV' : 'Films'} list · {m.match_status === 'manual' ? 'fixed by hand' : `via ${m.match_source || m.match_status}`}</div>
                  </td>
                  <td>
                    <a href={href('/mapping', { open: `${m.tmdb_kind}:${m.tmdb_id}` })}>{m.tmdb_title || `TMDB ${m.tmdb_id}`}</a>
                    <div class="cell-sub mono">{m.tmdb_kind} {m.tmdb_id}</div>
                  </td>
                  <td class="mono"><a href={links.netflix(m.netflix_id)} target="_blank" rel="noopener">{m.netflix_id}</a></td>
                  <td class="mono"><a href={links.netflix(m.tmdb_netflix_id)} target="_blank" rel="noopener">{m.tmdb_netflix_id}</a>
                    <div class="cell-sub" title={ID_SOURCES[m.tmdb_netflix_source]?.hint ?? ''}>{ID_SOURCES[m.tmdb_netflix_source]?.label ?? m.tmdb_netflix_source}</div></td>
                  <td class="nowrap">
                    {#if m.other_title_has_id}<span class="badge badge-info" title="Another Netflix title mapped here has the TMDB title's Netflix ID: likely a special or part of that show">same show</span>
                    {:else if m.match_status === 'manual'}<span class="badge badge-accent" title="Fixed by hand: confirmed">fixed by hand</span>
                    {:else}<span class="badge badge-warning" title="Likely matched to a different title of the same name">check mapping</span>{/if}
                  </td>
                </tr>
              {:else}
                <tr><td colspan="5"><div class="empty small">Every Netflix title's Netflix ID matches its TMDB title's.</div></td></tr>
              {/each}
            </tbody>
          </table>
        </div>
        {#if (report.netflix_id_mismatches ?? []).length}
          <p class="field-hint" style="padding:0 16px 12px">The Netflix ID Netflix's own Top 10 search gives the title differs from the one its TMDB title has (from TMDB, Wikidata, a match or by hand). "Check mapping": usually a match to another title of the same name; open it and fix the mapping, then the entry goes. "Same show": a special or season mapped to its show, whose own Netflix title has that ID. Titles fixed by hand are listed last.</p>
        {/if}
      </div>
    </div>
  {/if}
</div>
