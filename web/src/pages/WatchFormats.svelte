<script>
  import PageHeader from '../components/PageHeader.svelte'
  import Modal from '../components/Modal.svelte'
  import Icon from '../components/Icon.svelte'
  import { listWatchFormats, createWatchFormat, updateWatchFormat, uploadWatchFormatLogo, deleteWatchFormatLogo } from '../lib/api.js'
  import { toast } from '../lib/toast.svelte.js'
  import { confirm } from '../lib/confirm.svelte.js'

  // The axes a cinema format mixes. Keeping them apart is what makes
  // "all my 70mm screenings" or "IMAX hours" answerable later.
  const CATEGORIES = [
    { value: 'brand', label: 'Brand', hint: 'IMAX, Dolby Cinema, Cinemark XD' },
    { value: 'projection', label: 'Projection', hint: '70mm, 35mm, laser, high frame rate' },
    { value: 'sound', label: 'Sound', hint: 'Dolby Atmos, DTS:X' },
    { value: 'motion', label: 'Motion', hint: '4DX, D-BOX' },
    { value: 'dimension', label: 'Dimension', hint: 'RealD 3D' },
  ]

  // What the server accepts; it checks the bytes itself, this just saves a round trip.
  const LOGO_TYPES = ['image/png', 'image/jpeg', 'image/webp', 'image/gif', 'image/svg+xml']
  const LOGO_MAX = 512 * 1024

  let formats = $state([])
  let loading = $state(true)
  let error = $state('')
  let editing = $state(null) // the format being edited, or a blank new one
  let saving = $state(false)
  // A logo picked in the dialog: uploaded at once for an existing format, after
  // creation for a new one. preview is an object URL for showing it before then.
  let picked = $state(null) // { file, preview }
  let logoBusy = $state(false)
  let dragging = $state(false)

  async function load() {
    loading = true
    try {
      const data = await listWatchFormats()
      formats = data.formats ?? []
      error = ''
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }
  $effect(() => { load() })

  const blank = () => ({ id: null, slug: '', label: '', category: 'brand', note: '', sort_order: 0, active: true, logo_url: '' })

  function open(f) {
    clearPicked()
    editing = f ? { ...f, note: f.note ?? '', logo_url: f.logo_url ?? '' } : blank()
  }
  function close() {
    clearPicked()
    editing = null
  }
  function clearPicked() {
    if (picked?.preview) URL.revokeObjectURL(picked.preview)
    picked = null
  }

  async function save() {
    saving = true
    try {
      const body = {
        label: editing.label,
        category: editing.category,
        note: editing.note || null,
        sort_order: Number(editing.sort_order) || 0,
        active: editing.active,
      }
      if (editing.id) {
        await updateWatchFormat(editing.id, body)
        toast.success(`Updated ${editing.label}`)
      } else {
        const created = await createWatchFormat({ ...body, slug: editing.slug })
        if (picked) {
          try {
            await uploadWatchFormatLogo(created.id, picked.file)
          } catch (e) {
            toast.error(`Added ${editing.label}, but the logo didn't upload: ${e.message}`)
            close()
            load()
            return
          }
        }
        toast.success(`Added ${editing.label}`)
      }
      close()
      load()
    } catch (e) {
      toast.error(e.message)
    } finally {
      saving = false
    }
  }

  function check(file) {
    if (!LOGO_TYPES.includes(file.type)) return 'Use a PNG, JPEG, WebP, GIF or SVG image.'
    if (file.size > LOGO_MAX) return `That file is ${(file.size / 1024).toFixed(0)} KB; logos can be up to 512 KB.`
    return ''
  }

  async function choose(file) {
    if (!file) return
    const problem = check(file)
    if (problem) {
      toast.error(problem)
      return
    }
    clearPicked()
    if (!editing.id) {
      // A new format has no id yet: keep the file and upload it on save.
      picked = { file, preview: URL.createObjectURL(file) }
      return
    }
    logoBusy = true
    try {
      const updated = await uploadWatchFormatLogo(editing.id, file)
      editing.logo_url = updated.logo_url ?? ''
      toast.success(`Logo saved for ${editing.label}`)
      load()
    } catch (e) {
      toast.error(e.message)
    } finally {
      logoBusy = false
    }
  }

  async function removeLogo() {
    if (picked) {
      clearPicked()
      return
    }
    const ok = await confirm({
      title: `Remove ${editing.label}'s logo?`,
      message: 'The apps show the label on its own until another logo is uploaded.',
      confirmLabel: 'Remove logo',
    })
    if (!ok) return
    logoBusy = true
    try {
      await deleteWatchFormatLogo(editing.id)
      editing.logo_url = ''
      toast.success('Logo removed')
      load()
    } catch (e) {
      toast.error(e.message)
    } finally {
      logoBusy = false
    }
  }

  function onDrop(e) {
    e.preventDefault()
    dragging = false
    choose(e.dataTransfer?.files?.[0])
  }

  const shown = $derived(picked?.preview ?? editing?.logo_url ?? '')

  // Formats are retired, never deleted: watches reference them for good.
  async function toggleActive(f) {
    if (f.active) {
      const ok = await confirm({
        title: `Retire ${f.label}?`,
        message: 'It stays on every watch that already carries it, but nobody can pick it for a new one.',
        confirmLabel: 'Retire it',
      })
      if (!ok) return
    }
    try {
      await updateWatchFormat(f.id, { active: !f.active })
      toast.success(f.active ? `${f.label} retired` : `${f.label} is available again`)
      load()
    } catch (e) {
      toast.error(e.message)
    }
  }

  const withLogo = $derived(formats.filter((f) => f.logo_url).length)
</script>

<PageHeader
  title="Premium formats"
  desc="The formats a watch can be logged in, and their logos. One screening can carry several, because a brand, a projection, a sound system and a motion rig are separate things."
  info="Formats are shared with everyone who logs watches. Retiring one keeps it on the watches that already have it, which is why there is no delete. Logos show next to the label in the user app."
>
  {#snippet actions()}
    <button class="btn btn-primary" onclick={() => open(null)}>
      <Icon name="plus" /> Add format
    </button>
  {/snippet}
</PageHeader>

{#if error}
  <div class="alert-error">{error}</div>
{/if}

{#if loading}
  <div class="card">Loading formats…</div>
{:else if formats.length === 0}
  <div class="card">No formats yet. Add the first one.</div>
{:else}
  <p class="muted summary">{formats.length} formats · {withLogo} with a logo · {formats.filter((f) => !f.active).length} retired</p>
  {#each CATEGORIES as cat}
    {@const rows = formats.filter((f) => f.category === cat.value)}
    {#if rows.length}
      <div class="card">
        <h3>{cat.label} <span class="muted">— {cat.hint}</span></h3>
        <table class="table">
          <thead>
            <tr>
              <th class="logo-col">Logo</th>
              <th>Label</th>
              <th>Slug</th>
              <th class="num">Order</th>
              <th>Status</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {#each rows as f (f.id)}
              <tr class:dim={!f.active}>
                <td class="logo-col">
                  {#if f.logo_url}
                    <span class="thumb"><img src={f.logo_url} alt="{f.label} logo" /></span>
                  {:else}
                    <button class="add-logo" onclick={() => open(f)} title="Add a logo for {f.label}">+ Logo</button>
                  {/if}
                </td>
                <td>
                  {f.label}
                  {#if f.note}<div class="muted">{f.note}</div>{/if}
                </td>
                <td><code>{f.slug}</code></td>
                <td class="num">{f.sort_order}</td>
                <td>
                  {#if f.active}
                    <span class="badge badge-success">Available</span>
                  {:else}
                    <span class="badge badge-warning">Retired</span>
                  {/if}
                </td>
                <td class="right">
                  <button class="btn btn-sm" onclick={() => open(f)}>Edit</button>
                  <button class="btn btn-sm" onclick={() => toggleActive(f)}>
                    {f.active ? 'Retire' : 'Restore'}
                  </button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  {/each}
{/if}

{#if editing}
  <Modal title={editing.id ? `Edit ${editing.label}` : 'Add a format'} onclose={close}>
    <div class="form-row">
      <label for="fmt-label">Label</label>
      <input id="fmt-label" class="input" bind:value={editing.label} placeholder="IMAX 70mm" />
    </div>
    {#if !editing.id}
      <div class="form-row">
        <label for="fmt-slug">Slug</label>
        <input id="fmt-slug" class="input" bind:value={editing.slug} placeholder="imax-70mm" />
        <div class="muted">Lowercase words joined by hyphens. It never changes, because watches reference it.</div>
      </div>
    {/if}
    <div class="form-row">
      <label for="fmt-category">Category</label>
      <select id="fmt-category" class="input" bind:value={editing.category}>
        {#each CATEGORIES as c}
          <option value={c.value}>{c.label} — {c.hint}</option>
        {/each}
      </select>
    </div>
    <div class="form-row">
      <label for="fmt-note">Note</label>
      <input id="fmt-note" class="input" bind:value={editing.note} placeholder="Optional" />
    </div>
    <div class="form-row">
      <label for="fmt-order">Sort order</label>
      <input id="fmt-order" class="input num" type="number" bind:value={editing.sort_order} />
    </div>

    <div class="form-row">
      <span class="lbl" id="fmt-logo-h">Logo</span>
      {#if shown}
        <div class="previews" aria-labelledby="fmt-logo-h">
          <span class="pv dark"><img src={shown} alt="{editing.label || 'New format'} logo on a dark background" /></span>
          <span class="pv light"><img src={shown} alt="{editing.label || 'New format'} logo on a light background" /></span>
        </div>
        <div class="muted">{picked ? 'Uploads when you save the format. ' : ''}Left: how the apps show it, on a dark plate in both themes. Right: on white, for clients that don’t add a plate; a white logo disappears there.</div>
      {/if}
      <label
        class="drop"
        class:over={dragging}
        ondragover={(e) => { e.preventDefault(); dragging = true }}
        ondragleave={() => (dragging = false)}
        ondrop={onDrop}
      >
        <input type="file" accept={LOGO_TYPES.join(',')} disabled={logoBusy} onchange={(e) => { choose(e.currentTarget.files?.[0]); e.currentTarget.value = '' }} />
        <span><b>{logoBusy ? 'Uploading…' : shown ? 'Replace the logo' : 'Add a logo'}</b> — drop an image here or click to choose one</span>
        <span class="muted">PNG, SVG, WebP, JPEG or GIF, up to 512 KB and 4096px a side. SVGs may only hold shapes, text, gradients and filters (no styles, scripts or embedded images). A transparent PNG or SVG, wider than it is tall, reads best; it's shown about 20px high.</span>
      </label>
      {#if shown}
        <button class="btn btn-sm remove" onclick={removeLogo} disabled={logoBusy}>{picked ? 'Don’t use this file' : 'Remove logo'}</button>
      {/if}
    </div>
    {#snippet footer()}
      <button class="btn" onclick={close}>Cancel</button>
      <button class="btn btn-primary" onclick={save} disabled={saving || logoBusy || !editing.label}>
        {saving ? 'Saving…' : 'Save'}
      </button>
    {/snippet}
  </Modal>
{/if}

<style>
  h3 { margin: 0 0 0.75rem; font-size: 0.95rem; }
  .muted { color: var(--muted); font-weight: 400; font-size: 0.85rem; }
  .summary { margin: 0 0 0.75rem; }
  .dim td { opacity: 0.55; }
  .right { text-align: right; white-space: nowrap; }
  .form-row { margin-bottom: 0.9rem; display: grid; gap: 0.35rem; }
  .form-row label, .lbl { font-size: 0.85rem; color: var(--muted); }
  .logo-col { width: 110px; }
  .thumb { display: inline-grid; place-items: center; height: 32px; min-width: 56px; padding: 4px 8px; border-radius: 6px; background: #161618; }
  .thumb img { max-height: 22px; max-width: 90px; object-fit: contain; }
  .add-logo { padding: 3px 8px; border: 1px dashed var(--border, #ccc); border-radius: 6px; background: none; color: var(--muted); font-size: 0.8rem; cursor: pointer; }
  .add-logo:hover { color: inherit; border-style: solid; }
  .previews { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
  .pv { display: grid; place-items: center; height: 72px; border-radius: 8px; }
  .pv img { max-height: 28px; max-width: 80%; object-fit: contain; }
  .pv.dark { background: #161618; }
  .pv.light { background: #ffffff; box-shadow: inset 0 0 0 1px #e0e0e4; }
  .drop { position: relative; display: grid; gap: 0.2rem; padding: 0.9rem; border: 1.5px dashed var(--border, #ccc); border-radius: 8px; color: inherit !important; cursor: pointer; }
  .drop.over, .drop:hover { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 6%, transparent); }
  .drop input { position: absolute; inset: 0; opacity: 0; cursor: pointer; }
  .remove { justify-self: start; }
</style>
