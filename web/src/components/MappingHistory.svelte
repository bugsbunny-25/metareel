<script>
  // History of mapping changes of one title (subject: "flixpatrol:12" /
  // "netflix:34") or of one TMDB title (tmdb: "movie:550"). Change `version`
  // to reload it after an edit.
  import { listMappingEvents } from '../lib/api.js'
  import { href } from '../lib/router.svelte.js'
  import { relativeTime, dateTime, dateTimeUTC } from '../lib/format.js'

  let { subject = '', tmdb = '', version = 0, limit = 20 } = $props()

  const ACTIONS = {
    match: ['matched', 'badge-success', 'Automatic match'],
    manual: ['fixed by hand', 'badge-accent', 'Set in the admin UI / API'],
    adopt: ['shared', 'badge-info', 'Took a linked title\'s mapping'],
    remap: ['moved', 'badge-accent', 'Moved with everything on its TMDB title'],
    clear: ['cleared', 'badge-warning', 'Match forgotten by hand'],
    unshare: ['unshared', 'badge-warning', 'A linked title\'s mapping it had taken went away: back to its own match, or unmapped'],
    requeue: ['rematch', '', 'Queued for matching again'],
  }

  let events = $state(null)
  let error = $state('')

  $effect(() => {
    const q = { limit }
    if (subject) q.subject = subject
    if (tmdb) q.tmdb = tmdb
    void version
    let cancelled = false
    error = ''
    listMappingEvents(q)
      .then((e) => { if (!cancelled) events = e })
      .catch((e) => { if (!cancelled) error = e.message })
    return () => { cancelled = true }
  })

  const refLabel = (kind, id) => (id ? `${kind === 'tv' ? 'TV' : 'movie'} ${id}` : 'unmapped')
  const subjectHref = (e) => e.subject_type === 'netflix' ? href('/netflix', { open: e.subject_id }) : href('/titles', { open: e.subject_id })
</script>

{#if error}
  <div class="alert alert-error small">{error}</div>
{:else if events === null}
  <p class="muted small">Loading history…</p>
{:else if events.length === 0}
  <p class="muted small">No mapping changes recorded yet (history starts with this version).</p>
{:else}
  <ul class="history">
    {#each events as e (e.id)}
      {@const a = ACTIONS[e.action] ?? [e.action, '', '']}
      <li>
        <div class="row small">
          <span class="badge {a[1]}" title={a[2]}>{a[0]}</span>
          {#if tmdb}
            <a href={subjectHref(e)}>{e.subject_name || `#${e.subject_id}`}</a>
            <span class="muted">({e.subject_type === 'netflix' ? 'Netflix' : 'FlixPatrol'})</span>
          {/if}
          <span class="mono">
            {#if e.from_tmdb_id !== e.to_tmdb_id || e.from_tmdb_kind !== e.to_tmdb_kind}
              {refLabel(e.from_tmdb_kind, e.from_tmdb_id)} →
            {/if}
            {#if e.to_tmdb_id}
              <a href={href('/mapping', { open: `${e.to_tmdb_kind}:${e.to_tmdb_id}` })}>{refLabel(e.to_tmdb_kind, e.to_tmdb_id)}</a>
            {:else}unmapped{/if}
          </span>
        </div>
        <div class="small muted">
          <span title="{dateTime(e.created_at)} · {dateTimeUTC(e.created_at)}">{relativeTime(e.created_at)}</span>
          {' · by '}<span class="mono">{e.actor}</span>{#if e.source}{' · via '}<span class="mono">{e.source}</span>{/if}
        </div>
      </li>
    {/each}
  </ul>
{/if}

<style>
  .history { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 8px; }
  .history li { padding-bottom: 8px; border-bottom: 1px solid var(--border); }
  .history li:last-child { border-bottom: 0; }
</style>
