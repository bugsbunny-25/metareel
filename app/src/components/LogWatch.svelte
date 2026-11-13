<!-- "Log a watch" dialog. Shows need a season and episode (one episode per watch).
     The time is sent as a UTC instant with the browser's time zone, so the server
     files it under the right local day. A client key makes a double submit harmless. -->
<script>
  import { api } from '../lib/api.js'
  import { library } from '../lib/library.svelte.js'
  import { toast } from '../lib/toast.svelte.js'
  import RatingInput from './RatingInput.svelte'
  import Poster from './Poster.svelte'

  /** target: { kind: 'movie'|'tv', id, name, poster?, season? } or null when closed */
  let { target = $bindable(null) } = $props()

  let dialog
  let when = $state('')
  let rating = $state(null)
  let liked = $state(false)
  let notes = $state('')
  let season = $state(1)
  let episode = $state(1)
  let busy = $state(false)
  let error = $state('')
  let key = ''

  const pad = (n) => String(n).padStart(2, '0')
  const localNow = () => {
    const d = new Date()
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
  }

  $effect(() => {
    if (!dialog) return
    if (target && !dialog.open) {
      when = localNow()
      rating = null
      liked = false
      notes = ''
      season = target.season ?? 1
      episode = 1
      error = ''
      key = crypto.randomUUID()
      dialog.showModal()
    } else if (!target && dialog.open) {
      dialog.close()
    }
  })

  async function submit(e) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      const input = {
        client_key: key,
        tmdb_kind: target.kind,
        tmdb_id: target.id,
        ended_at: new Date(when).toISOString(),
        tz: Intl.DateTimeFormat().resolvedOptions().timeZone,
        rating,
        liked,
        notes: notes.trim() || null,
      }
      if (target.kind === 'tv') Object.assign(input, { season_number: Number(season), episode_number: Number(episode) })
      await api.logWatch(input)
      await library.refresh(target.kind, target.id)
      toast(`Logged ${target.name}${target.kind === 'tv' ? ` S${season} E${episode}` : ''}.`)
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
          <p class="eyebrow">Log a watch</p>
          <h2 id="log-h">{target.name}</h2>
        </div>
      </header>

      {#if target.kind === 'tv'}
        <div class="row two">
          <label>Season<input type="number" min="0" bind:value={season} required /></label>
          <label>Episode<input type="number" min="1" bind:value={episode} required /></label>
        </div>
      {/if}
      <label>Finished watching<input type="datetime-local" bind:value={when} required max={localNow()} /></label>
      <div class="field"><span class="lbl">Rating</span><RatingInput bind:value={rating} /></div>
      <label class="check"><input type="checkbox" bind:checked={liked} /> Liked it</label>
      <label>Notes <span class="muted">(only you see these)</span><textarea rows="3" bind:value={notes} maxlength="2000"></textarea></label>

      {#if error}<p class="err" role="alert">{error}</p>{/if}
      <footer>
        <button type="button" class="btn-secondary" onclick={() => (target = null)}>Cancel</button>
        <button type="submit" class="btn-primary" disabled={busy}>{busy ? 'Saving…' : 'Log watch'}</button>
      </footer>
    </form>
  {/if}
</dialog>

<style>
  dialog { width: min(480px, calc(100vw - 32px)); padding: 0; border: 0; border-radius: 20px; background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 30px 80px -20px rgb(0 0 0 / 0.6); }
  dialog::backdrop { background: rgb(0 0 0 / 0.55); }
  form { display: grid; gap: 14px; padding: 22px; }
  header { display: flex; gap: 14px; align-items: center; }
  .eyebrow { margin: 0 0 2px; color: var(--text-muted); font-size: 12px; font-weight: 600; letter-spacing: 0.05em; text-transform: uppercase; }
  h2 { margin: 0; font: 700 22px/1.15 var(--font-display); }
  label, .field { display: grid; gap: 6px; font-size: 13px; font-weight: 600; color: var(--text-muted); }
  .lbl { font-size: 13px; }
  .row.two { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
  input, textarea {
    padding: 9px 12px; border: 1px solid var(--border-subtle); border-radius: 10px; background: var(--bg-canvas);
    color: var(--text-primary); font: 400 15px/1.3 var(--font-sans);
  }
  input:focus-visible, textarea:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 0; }
  .check { display: flex; align-items: center; gap: 8px; color: var(--text-primary); font-weight: 400; font-size: 14px; }
  .check input { width: 16px; height: 16px; accent-color: var(--accent-yours); }
  .muted { font-weight: 400; }
  .err { margin: 0; color: var(--delta-down); }
  footer { display: flex; justify-content: flex-end; gap: 10px; margin-top: 4px; }
</style>
