<!-- Per-title menu: set the status, favourite, log a watch, or clear the title's state.
     A disclosure (details/summary) holding real buttons: no custom focus trap needed, and
     Escape or picking an option closes it. -->
<script>
  import Icon from './Icon.svelte'
  import { library } from '../lib/library.svelte.js'
  import { toast } from '../lib/toast.svelte.js'
  import { STATUSES } from '../lib/status.js'

  /** t: a library entry (tmdb_kind, tmdb_id, title, status, favorite, watch_count, poster_path) */
  let { t, onlog, align = 'right' } = $props()

  let details
  const shut = () => details?.removeAttribute('open')

  async function setStatus(status) {
    shut()
    try {
      await library.setState(t.tmdb_kind, t.tmdb_id, { status })
      toast(`${t.title}: ${STATUSES.find(([v]) => v === status)?.[1] ?? 'No status'}.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }
  async function favourite() {
    shut()
    const was = t.favorite // t updates as soon as the store does, so read it first
    try {
      await library.setState(t.tmdb_kind, t.tmdb_id, { favorite: !was })
      toast(was ? `Removed ${t.title} from favourites.` : `Added ${t.title} to favourites.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }
  async function clear() {
    shut()
    const { title, watch_count } = t
    try {
      await library.clear(t.tmdb_kind, t.tmdb_id)
      toast(watch_count > 0 ? `Cleared ${title}'s status. Your watches are kept.` : `Removed ${title} from your library.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }
  function log() {
    shut()
    onlog({ kind: t.tmdb_kind, id: t.tmdb_id, name: t.title, poster: t.poster_path, season: 1 })
  }
  function onkeydown(e) {
    if (e.key === 'Escape' && details?.open) { e.preventDefault(); shut(); details.querySelector('summary')?.focus() }
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<details class="menu" bind:this={details} {onkeydown}>
  <summary aria-label="Options for {t.title}" title="Options"><Icon name="more" size={16} /></summary>
  <div class="pop" class:left={align === 'left'}>
    <p class="h">Status</p>
    {#each STATUSES as [v, label]}
      <button type="button" aria-pressed={t.status === v} onclick={() => setStatus(v)}>
        <span class="dot" class:on={t.status === v} aria-hidden="true"></span>{label}
      </button>
    {/each}
    <hr />
    <button type="button" onclick={favourite}><Icon name={t.favorite ? 'heart-fill' : 'heart'} size={14} />{t.favorite ? 'Remove from favourites' : 'Add to favourites'}</button>
    <button type="button" onclick={log}><Icon name="plus" size={14} />Log a watch</button>
    {#if t.status || t.favorite}
      <button type="button" class="quiet" onclick={clear}>{t.watch_count > 0 ? 'Clear status' : 'Remove from library'}</button>
    {/if}
  </div>
</details>

<style>
  .menu { position: relative; }
  summary { display: grid; place-items: center; width: 32px; height: 32px; border-radius: 50%; background: var(--bg-elevated); color: var(--text-primary); cursor: pointer; list-style: none; }
  summary::-webkit-details-marker { display: none; }
  summary:hover, .menu[open] summary { background: var(--accent-primary); color: var(--accent-on); }
  summary:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 2px; }
  .pop { position: absolute; right: 0; top: 38px; z-index: 20; display: grid; gap: 1px; min-width: 220px; padding: 8px; border-radius: 14px; background: var(--bg-surface); box-shadow: 0 18px 50px -12px rgb(0 0 0 / 0.55), 0 0 0 1px var(--border-subtle); }
  .pop.left { right: auto; left: 0; }
  .h { margin: 2px 8px 4px; color: var(--text-muted); font: 700 11px/1 var(--font-sans); letter-spacing: 0.06em; text-transform: uppercase; }
  button { display: flex; align-items: center; gap: 10px; padding: 8px; border: 0; border-radius: 8px; background: none; color: var(--text-primary); text-align: left; font: 500 14px/1.2 var(--font-sans); cursor: pointer; }
  button:hover, button:focus-visible { background: var(--bg-elevated); outline: none; }
  button[aria-pressed='true'] { font-weight: 700; }
  .dot { width: 10px; height: 10px; border-radius: 50%; box-shadow: inset 0 0 0 1.5px var(--border-strong); }
  .dot.on { background: var(--accent-yours); box-shadow: none; }
  .quiet { color: var(--text-muted); }
  hr { width: 100%; margin: 4px 0; border: 0; border-top: 1px solid var(--border-subtle); }
</style>
