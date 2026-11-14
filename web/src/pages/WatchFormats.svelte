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
    { value: 'disc', label: 'Disc', hint: 'DVD, Blu-ray, 4K UHD Blu-ray (home media)' },
    { value: 'label', label: 'Label', hint: 'Criterion, Arrow Video, Kino Lorber (who released the disc)' },
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

  const blank = () => ({ id: null, slug: '', label: '', category: 'brand', note: '', sort_order: 0, active: true, logo_url: '', logo_display: 'original' })

  // Dark logos vanish on the apps' dark plate. After an upload the logo's
  // brightness is measured here, and a dark one is switched to 'mono' (the
  // apps draw it white; the file is unchanged). The switch can be flipped back.
  async function isDark(src) {
    let url = ''
    try {
      // Measured on a copy: Chrome refuses to read pixels back from an SVG that
      // has a <foreignObject> (Illustrator's "preserve editing" data), so that
      // part, which is never drawn, is cut out first.
      let blob = await fetch(src, { cache: 'no-store' }).then((r) => r.blob())
      if (blob.type === 'image/svg+xml') {
        blob = new Blob([(await blob.text()).replace(/<foreignObject[\s\S]*?<\/foreignObject>/g, '')], { type: 'image/svg+xml' })
      }
      url = URL.createObjectURL(blob)
      // load event + timeout rather than img.decode(), which can wait forever
      // in a background tab; on timeout the logo just keeps its colours.
      const img = new Image()
      await new Promise((resolve, reject) => {
        const t = setTimeout(() => reject(new Error('timeout')), 4000)
        img.onload = () => { clearTimeout(t); resolve() }
        img.onerror = () => { clearTimeout(t); reject(new Error('load failed')) }
        img.src = url
      })
      const w = 240, h = Math.max(1, Math.round(240 * (img.naturalHeight || 1) / (img.naturalWidth || 2)))
      const c = document.createElement('canvas')
      c.width = w
      c.height = h
      const g = c.getContext('2d')
      g.drawImage(img, 0, 0, w, h)
      const px = g.getImageData(0, 0, w, h).data
      // A logo with its own opaque background (no transparent corners) would
      // turn into a solid block if drawn white, so it keeps its colours.
      const corners = [0, (w - 1) * 4, (h - 1) * w * 4, ((h - 1) * w + w - 1) * 4]
      if (corners.every((i) => px[i + 3] > 200)) return false
      // Dark = 40% or more of the visible pixels are dark. An average misses
      // mixed logos (black lettering over a pale bar, a red mark beside
      // black text), whose dark part still vanishes on the plate.
      let n = 0, dark = 0
      for (let i = 0; i < px.length; i += 4) {
        if (px[i + 3] < 40) continue
        n++
        if (0.2126 * px[i] + 0.7152 * px[i + 1] + 0.0722 * px[i + 2] < 60) dark++
      }
      return n > 0 && dark / n >= 0.4
    } catch {
      return false
    } finally {
      if (url) URL.revokeObjectURL(url)
    }
  }
  /** After an upload: switch a dark logo to mono; returns whether it did. */
  async function suggestDisplay(format, src) {
    if (!(await isDark(src))) return false
    if (format.logo_display === 'mono') return true
    await updateWatchFormat(format.id, { logo_display: 'mono' })
    return true
  }

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
        logo_display: editing.logo_display ?? 'original',
      }
      if (editing.id) {
        await updateWatchFormat(editing.id, body)
        toast.success(`Updated ${editing.label}`)
      } else {
        const created = await createWatchFormat({ ...body, slug: editing.slug })
        if (picked) {
          try {
            const up = await uploadWatchFormatLogo(created.id, picked.file)
            if (editing.logo_display !== 'mono' && (await suggestDisplay(created, up.logo_url))) toast.success(`${editing.label}'s logo is dark, so the apps show it white`)
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
      if (await suggestDisplay(updated, updated.logo_url)) {
        editing.logo_display = 'mono'
        toast.success(`Logo saved for ${editing.label}; it's dark, so the apps show it white`)
      } else {
        toast.success(`Logo saved for ${editing.label}`)
      }
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

  // Bulk: drop many files named after slugs (imax.svg, criterion.png) and each
  // goes to its format. Results are listed per file.
  let bulk = $state([]) // [{ name, slug, status: 'ok' | 'error' | 'skip', message }]
  let bulkBusy = $state(false)
  let bulkOver = $state(false)
  async function uploadMany(files) {
    const list = [...(files ?? [])]
    if (!list.length) return
    bulkBusy = true
    bulk = list.map((f) => ({ name: f.name, slug: f.name.replace(/\.[^.]+$/, '').trim().toLowerCase(), status: 'pending', message: '' }))
    for (const [i, file] of list.entries()) {
      const row = bulk[i]
      const format = formats.find((f) => f.slug === row.slug)
      const problem = !format ? 'no format has this slug' : check(file)
      if (problem) {
        bulk[i] = { ...row, status: format ? 'error' : 'skip', message: problem }
        continue
      }
      try {
        const up = await uploadWatchFormatLogo(format.id, file)
        const mono = await suggestDisplay(up, up.logo_url)
        bulk[i] = { ...row, status: 'ok', message: format.label + (mono ? ' (dark: shown white)' : '') }
      } catch (e) {
        bulk[i] = { ...row, status: 'error', message: e.message }
      }
    }
    bulkBusy = false
    const ok = bulk.filter((b) => b.status === 'ok').length
    if (ok) toast.success(`${ok} ${ok === 1 ? 'logo' : 'logos'} uploaded`)
    load()
  }
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
  <div class="card">
    <label
      class="drop bulk"
      class:over={bulkOver}
      ondragover={(e) => { e.preventDefault(); bulkOver = true }}
      ondragleave={() => (bulkOver = false)}
      ondrop={(e) => { e.preventDefault(); bulkOver = false; uploadMany(e.dataTransfer?.files) }}
    >
      <input type="file" multiple accept={LOGO_TYPES.join(',')} disabled={bulkBusy} onchange={(e) => { uploadMany(e.currentTarget.files); e.currentTarget.value = '' }} />
      <span><b>{bulkBusy ? 'Uploading…' : 'Upload logos in bulk'}</b> — drop files named after each format’s slug, e.g. <code>imax.svg</code>, <code>uhd-blu-ray.png</code>, <code>criterion.svg</code></span>
      <span class="muted">Each file replaces that format’s logo. Files that match no slug are skipped.</span>
    </label>
    {#if bulk.length}
      <ul class="bulk-results">
        {#each bulk as b}
          <li class={b.status}>
            <code>{b.name}</code>
            <span>{b.status === 'pending' ? 'waiting…' : b.status === 'ok' ? `✓ ${b.message}` : b.status === 'skip' ? `skipped: ${b.message}` : `✗ ${b.message}`}</span>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
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
                    <span class="thumb" title={f.logo_display === 'mono' ? 'Shown white' : 'Original colours'}><img class:mono={f.logo_display === 'mono'} src={f.logo_url} alt="{f.label} logo" /></span>
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
          <button type="button" class="pv dark" class:chosen={editing.logo_display !== 'mono'} aria-pressed={editing.logo_display !== 'mono'} onclick={() => (editing.logo_display = 'original')}>
            <img src={shown} alt="{editing.label || 'New format'} logo in its original colours" /><span>Original</span>
          </button>
          <button type="button" class="pv dark" class:chosen={editing.logo_display === 'mono'} aria-pressed={editing.logo_display === 'mono'} onclick={() => (editing.logo_display = 'mono')}>
            <img class="mono" src={shown} alt="{editing.label || 'New format'} logo drawn white" /><span>Monochrome (white)</span>
          </button>
          <span class="pv light"><img src={shown} alt="{editing.label || 'New format'} logo on white" /><span>Original on white</span></span>
        </div>
        <div class="muted">{picked ? 'Uploads when you save the format. ' : ''}The apps show it on a dark plate; pick which version (saved with the format). A dark upload is switched to white automatically. The file itself never changes, and other clients get both the file and this choice.</div>
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
  .previews { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 8px; }
  .pv { position: relative; border: 2px solid transparent; cursor: default; }
  button.pv { cursor: pointer; font: inherit; }
  .pv span { position: absolute; left: 0; right: 0; bottom: 4px; font-size: 0.7rem; text-align: center; color: #a1a1a8; }
  .pv.light span { color: #636369; }
  .pv.chosen { border-color: var(--accent); }
  img.mono { filter: brightness(0) invert(1); }
  .pv { display: grid; place-items: center; height: 72px; border-radius: 8px; }
  .pv img { max-height: 28px; max-width: 80%; object-fit: contain; }
  .pv.dark { background: #161618; }
  .pv.light { background: #ffffff; box-shadow: inset 0 0 0 1px #e0e0e4; }
  .drop { position: relative; display: grid; gap: 0.2rem; padding: 0.9rem; border: 1.5px dashed var(--border, #ccc); border-radius: 8px; color: inherit !important; cursor: pointer; }
  .drop.over, .drop:hover { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 6%, transparent); }
  .drop input { position: absolute; inset: 0; opacity: 0; cursor: pointer; }
  .remove { justify-self: start; }
  .bulk { margin: 0; }
  .bulk-results { list-style: none; margin: 0.75rem 0 0; padding: 0; display: grid; gap: 0.25rem; font-size: 0.85rem; }
  .bulk-results li { display: flex; gap: 0.75rem; }
  .bulk-results .ok span { color: var(--success, #1b7530); }
  .bulk-results .error span { color: var(--danger, #c4281c); }
  .bulk-results .skip span { color: var(--muted); }
</style>
