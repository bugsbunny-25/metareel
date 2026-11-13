<!-- "Log a watch" and "Edit watch" dialog. Shows need a season and episode (one episode per
     watch). The time goes out as a UTC instant with the browser's time zone, so the server
     files it under the right local day. New watches carry a client key, so a double submit
     is harmless; edits replace the watch (PUT). -->
<script>
  import { api } from '../lib/api.js'
  import { library } from '../lib/library.svelte.js'
  import { toast } from '../lib/toast.svelte.js'
  import { localInput, fromUTC } from '../lib/dates.js'
  import RatingInput from './RatingInput.svelte'
  import Poster from './Poster.svelte'

  /**
   * target: null when closed, else { kind: 'movie'|'tv', id, name, poster?, season?,
   *   watch?: an existing WatchEntry to edit, onsaved?: (entry) => void }
   */
  let { target = $bindable(null) } = $props()

  const VENUES = [
    ['', 'Not saying'], ['streaming', 'Streaming'], ['theater', 'Cinema'], ['physical', 'Disc or file'], ['broadcast', 'TV broadcast'], ['other', 'Somewhere else'],
  ]

  let dialog
  let when = $state('')
  let rating = $state(null)
  let liked = $state(false)
  let notes = $state('')
  let tagText = $state('')
  let venue = $state('')
  let season = $state(1)
  let episode = $state(1)
  let busy = $state(false)
  let error = $state('')
  let known = $state([])
  let key = ''

  const editing = $derived(!!target?.watch)

  $effect(() => {
    if (!dialog) return
    if (target && !dialog.open) {
      const w = target.watch
      when = localInput(w ? fromUTC(w.ended_at) ?? new Date() : new Date())
      rating = w?.rating ?? null
      liked = w?.liked ?? false
      notes = w?.notes ?? ''
      tagText = (w?.tags ?? []).join(', ')
      venue = w?.venue_kind ?? ''
      season = w?.season_number ?? target.season ?? 1
      episode = w?.episode_number ?? 1
      error = ''
      key = crypto.randomUUID()
      dialog.showModal()
      api.tags().then((r) => (known = (r.tags ?? []).map((t) => t.name))).catch(() => {})
    } else if (!target && dialog.open) {
      dialog.close()
    }
  })

  const tags = () => [...new Set(tagText.split(',').map((t) => t.trim()).filter(Boolean))].slice(0, 20)

  async function submit(e) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      const input = {
        tmdb_kind: target.kind,
        tmdb_id: target.id,
        ended_at: new Date(when).toISOString(),
        tz: Intl.DateTimeFormat().resolvedOptions().timeZone,
        rating,
        liked,
        notes: notes.trim() || null,
        tags: tags(),
        venue_kind: venue || null,
      }
      if (target.kind === 'tv') Object.assign(input, { season_number: Number(season), episode_number: Number(episode) }) // ignored on edit
      const entry = editing ? await api.updateWatch(target.watch.id, input) : await api.logWatch({ ...input, client_key: key })
      await library.refresh(target.kind, target.id).catch(() => {})
      const ep = target.kind === 'tv' ? ` S${season} E${episode}` : ''
      toast(editing ? `Saved your watch of ${target.name}${ep}.` : `Logged ${target.name}${ep}.`)
      target.onsaved?.(entry)
      target = null
    } catch (err) {
      error = err.message
    } finally {
      busy = false
    }
  }
</script>

<dialog bind:this={dialog} onclose={() => (target = null)} aria-labelledby="log-h">
  {#if target}
    <form onsubmit={submit}>
      <header>
        <Poster path={target.poster} seed={target.id} title={target.name} width={56} radius={6} size="w185" />
        <div>
          <p class="eyebrow">{editing ? 'Edit watch' : 'Log a watch'}</p>
          <h2 id="log-h">{target.name}</h2>
        </div>
      </header>

      {#if target.kind === 'tv' && editing}
        <!-- The server keeps a watch's title and episode on edit; to change them, delete and log again. -->
        <p class="fixed">Season {season} · Episode {episode}</p>
      {:else if target.kind === 'tv'}
        <div class="row two">
          <label>Season<input type="number" min="0" bind:value={season} required /></label>
          <label>Episode<input type="number" min="1" bind:value={episode} required /></label>
        </div>
      {/if}
      <div class="row two">
        <label>Finished watching<input type="datetime-local" bind:value={when} required max={localInput()} /></label>
        <label>Where
          <select bind:value={venue}>{#each VENUES as [v, l]}<option value={v}>{l}</option>{/each}</select>
        </label>
      </div>
      <div class="field"><span class="lbl">Rating</span><RatingInput bind:value={rating} /></div>
      <label class="check"><input type="checkbox" bind:checked={liked} /> Liked it</label>
      <label>Tags <span class="muted">(comma separated)</span>
        <input bind:value={tagText} list="known-tags" placeholder="cinema, with friends" maxlength="600" />
        <datalist id="known-tags">{#each known as t}<option value={t}></option>{/each}</datalist>
      </label>
      <label>Notes <span class="muted">(only you see these)</span><textarea rows="3" bind:value={notes} maxlength="2000"></textarea></label>

      {#if error}<p class="err" role="alert">{error}</p>{/if}
      <footer>
        <button type="button" class="btn-secondary" onclick={() => (target = null)}>Cancel</button>
        <button type="submit" class="btn-primary" disabled={busy}>{busy ? 'Saving…' : editing ? 'Save changes' : 'Log watch'}</button>
      </footer>
    </form>
  {/if}
</dialog>

<style>
  dialog { width: min(500px, calc(100vw - 32px)); max-height: calc(100vh - 32px); padding: 0; border: 0; border-radius: 20px; background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 30px 80px -20px rgb(0 0 0 / 0.6); }
  dialog::backdrop { background: rgb(0 0 0 / 0.55); }
  form { display: grid; gap: 14px; padding: 22px; }
  header { display: flex; gap: 14px; align-items: center; }
  .eyebrow { margin: 0 0 2px; color: var(--text-muted); font-size: 12px; font-weight: 600; letter-spacing: 0.05em; text-transform: uppercase; }
  h2 { margin: 0; font: 700 22px/1.15 var(--font-display); }
  label, .field { display: grid; gap: 6px; font-size: 13px; font-weight: 600; color: var(--text-muted); }
  .lbl { font-size: 13px; }
  .row.two { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
  input, textarea, select {
    padding: 9px 12px; border: 1px solid var(--border-subtle); border-radius: 10px; background: var(--bg-canvas);
    color: var(--text-primary); font: 400 15px/1.3 var(--font-sans);
  }
  input:focus-visible, textarea:focus-visible, select:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 0; }
  .check { display: flex; align-items: center; gap: 8px; color: var(--text-primary); font-weight: 400; font-size: 14px; }
  .check input { width: 16px; height: 16px; accent-color: var(--accent-yours); }
  .muted { font-weight: 400; }
  .err { margin: 0; color: var(--delta-down); }
  .fixed { margin: -4px 0 0; color: var(--text-muted); font-weight: 600; }
  footer { display: flex; justify-content: flex-end; gap: 10px; margin-top: 4px; }
  @media (max-width: 480px) { .row.two { grid-template-columns: 1fr; } }
</style>
