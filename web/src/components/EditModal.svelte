<script>
  import { untrack } from 'svelte'
  import { patchTitle } from '../lib/api.js'

  let { title, onclose, onsave } = $props()

  // untrack: capture the initial prop values as local form state without
  // creating a reactive dependency on the prop itself.
  let tmdb  = $state(untrack(() => title.tmdb_id ?? ''))
  let imdb  = $state(untrack(() => title.imdb_id ?? ''))
  let rtUrl = $state(untrack(() => title.rt_url  ?? ''))
  let saving = $state(false)
  let error  = $state('')

  async function save() {
    saving = true
    error  = ''
    try {
      const updatedFromApi = await patchTitle(title.id, { tmdb_id: tmdb, imdb_id: imdb, rt_url: rtUrl })
      const updatedTitle = {
        ...title,
        tmdb_id: tmdb || null,
        imdb_id: imdb || null,
        rt_url: rtUrl || null,
        ...(updatedFromApi ?? {}),
      }
      onsave(updatedTitle)
    } catch (e) {
      error = e.message
    } finally {
      saving = false
    }
  }

  function onBackdropClick(e) {
    if (e.target === e.currentTarget) onclose()
  }
</script>

<div
  class="modal-backdrop"
  onclick={onBackdropClick}
  onkeydown={(e) => { if (e.key === 'Escape') onclose() }}
  role="dialog"
  aria-modal="true"
  tabindex="-1"
>
  <div class="modal">
    <div class="modal-title">Edit external IDs</div>
    <div class="modal-subtitle">{title.slug} · {title.kind}</div>

    <div class="form-field">
      <label class="form-label" for="tmdb">TMDB ID</label>
      <input id="tmdb" class="form-input" type="text" bind:value={tmdb} placeholder="e.g. 12345" />
    </div>
    <div class="form-field">
      <label class="form-label" for="imdb">IMDb ID</label>
      <input id="imdb" class="form-input" type="text" bind:value={imdb} placeholder="e.g. tt1234567" />
    </div>
    <div class="form-field">
      <label class="form-label" for="rturl">Rotten Tomatoes URL</label>
      <input id="rturl" class="form-input" type="text" bind:value={rtUrl} placeholder="https://www.rottentomatoes.com/m/…" />
    </div>

    {#if error}
      <div class="error-msg">{error}</div>
    {/if}

    <div class="modal-actions">
      <button class="btn" onclick={onclose}>Cancel</button>
      <button class="btn btn-primary" onclick={save} disabled={saving}>
        {saving ? 'Saving…' : 'Save'}
      </button>
    </div>
  </div>
</div>
