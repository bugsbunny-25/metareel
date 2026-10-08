<script>
  import { getRun, getRunLogs, runScheduleNow } from '../lib/api.js'
  import { href } from '../lib/router.svelte.js'
  import { dateTime, dateTimeUTC, duration, shortError } from '../lib/format.js'
  import { toast } from '../lib/toast.svelte.js'
  import Drawer from '../components/Drawer.svelte'
  import Icon from '../components/Icon.svelte'
  import RunStatus from '../components/RunStatus.svelte'

  let { id, scheduleName, onclose, onchange } = $props()

  let run = $state(null)
  let logs = $state([])
  let error = $state('')
  let level = $state('')
  let filter = $state('')
  let showRawError = $state(false)
  let follow = $state(true)
  let logBox = $state()

  async function load() {
    try {
      const [r, l] = await Promise.all([getRun(id), getRunLogs(id)])
      run = r
      logs = l
      error = ''
    } catch (e) {
      error = e.message
    }
  }

  // Poll while the run is in progress.
  $effect(() => {
    id
    run = null
    logs = []
    load()
    const t = setInterval(() => {
      if (!run || run.status === 'started') load()
    }, 2000)
    return () => clearInterval(t)
  })

  $effect(() => {
    logs.length
    if (follow && logBox) queueMicrotask(() => (logBox.scrollTop = logBox.scrollHeight))
  })

  const LEVELS = { debug: 0, info: 1, warn: 2, error: 3 }
  const shown = $derived(
    logs.filter((l) => (!level || LEVELS[l.level] >= LEVELS[level]) && (!filter || l.message.toLowerCase().includes(filter.toLowerCase())))
  )
  const counts = $derived(logs.reduce((acc, l) => ({ ...acc, [l.level]: (acc[l.level] ?? 0) + 1 }), {}))

  function time(ts) {
    const d = new Date(ts)
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleTimeString(undefined, { hour12: false })
  }

  async function copyLogs() {
    const text = logs.map((l) => `${l.created_at} ${l.level.toUpperCase()} ${l.message}`).join('\n')
    try {
      await navigator.clipboard.writeText(text)
      toast.success('Logs copied')
    } catch {
      toast.error('Copy failed')
    }
  }

  async function rerun() {
    try {
      await runScheduleNow(run.schedule_id)
      toast.success('Schedule queued — it will appear at the top of the list')
      onchange?.()
    } catch (e) {
      toast.error(e.message)
    }
  }
</script>

<Drawer {onclose}>
  {#snippet header()}
    <div class="row" style="gap:10px">
      <div class="drawer-title">Run #{id}</div>
      {#if run}<RunStatus status={run.status} />{/if}
    </div>
    {#if run}<div class="small muted" style="margin-top:4px">{scheduleName(run.schedule_id)} · {run.task_type}</div>{/if}
  {/snippet}

  {#if error}
    <div class="alert alert-error"><Icon name="alert" />{error}</div>
  {:else if !run}
    <div class="skeleton" style="height:120px"></div>
  {:else}
    <dl class="kv">
      <dt>Schedule</dt><dd><a href={href('/runs', { schedule: run.schedule_id })}>{scheduleName(run.schedule_id)}</a></dd>
      <dt>Attempt</dt><dd>{run.retry_count + 1} of {run.max_retry + 1}</dd>
      <dt>Started</dt><dd>{dateTime(run.started_at)} <span class="muted small">{dateTimeUTC(run.started_at)}</span></dd>
      <dt>Finished</dt><dd>{run.finished_at ? dateTime(run.finished_at) : '—'}</dd>
      <dt>Duration</dt><dd class="mono">{run.error_message?.startsWith('interrupted') ? 'unknown (interrupted)' : duration(run.started_at, run.finished_at)}{run.status === 'started' ? ' (running)' : ''}</dd>
      {#if run.asynq_task_id}<dt>Task ID</dt><dd class="mono small">{run.asynq_task_id}</dd>{/if}
    </dl>

    {#if run.error_message}
      <div class="section-title row">Error
        <button class="btn btn-ghost btn-sm" onclick={() => (showRawError = !showRawError)}>{showRawError ? 'Summary' : 'Full error'}</button>
      </div>
      <div class="error-box">{showRawError ? run.error_message : shortError(run.error_message, 600)}</div>
    {/if}

    <div class="section-title row" style="margin-top:20px">
      Logs <span class="muted">({logs.length})</span>
      <span class="grow"></span>
    </div>
    <div class="toolbar">
      <div class="segmented" role="group" aria-label="Log level">
        <button class:active={level === ''} onclick={() => (level = '')}>All</button>
        <button class:active={level === 'info'} onclick={() => (level = 'info')}>Info+</button>
        <button class:active={level === 'warn'} onclick={() => (level = 'warn')}>Warn+ {counts.warn ? `(${counts.warn})` : ''}</button>
        <button class:active={level === 'error'} onclick={() => (level = 'error')}>Errors {counts.error ? `(${counts.error})` : ''}</button>
      </div>
      <div class="search grow">
        <span class="icon"><Icon name="search" size={14} /></span>
        <input class="input" style="width:100%" type="search" placeholder="Filter logs" bind:value={filter} />
      </div>
      <label class="checkbox small"><input type="checkbox" bind:checked={follow} />Follow</label>
      <button class="btn btn-sm" onclick={copyLogs} disabled={!logs.length}><Icon name="copy" size={12} />Copy</button>
    </div>
    <div class="logs" bind:this={logBox}>
      {#each shown as l (l.id)}
        <div class="log-line log-{l.level}">
          <span class="log-time" title={dateTime(l.created_at)}>{time(l.created_at)}</span>
          <span class="log-level">{l.level}</span>
          <span class="log-msg">{l.message}</span>
        </div>
      {:else}
        <div class="empty small">{logs.length ? 'No log lines match the filter.' : 'No log lines yet.'}</div>
      {/each}
    </div>
    {#if run.status === 'started'}<p class="small muted row" style="margin-top:8px"><span class="spinner"></span>Live — updating every 2 seconds</p>{/if}
  {/if}

  {#snippet footer()}
    {#if run}
      <button class="btn" onclick={rerun}><Icon name="play" size={14} />Run schedule again</button>
    {/if}
    <button class="btn" onclick={onclose}>Close</button>
  {/snippet}
</Drawer>
