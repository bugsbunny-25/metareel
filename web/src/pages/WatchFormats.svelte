<script>
  import PageHeader from '../components/PageHeader.svelte'
  import Modal from '../components/Modal.svelte'
  import Icon from '../components/Icon.svelte'
  import { listWatchFormats, createWatchFormat, updateWatchFormat } from '../lib/api.js'
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
  const categoryLabel = (v) => CATEGORIES.find((c) => c.value === v)?.label ?? v

  let formats = $state([])
  let loading = $state(true)
  let error = $state('')
  let editing = $state(null) // the format being edited, or a blank new one
  let saving = $state(false)

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

  const blank = () => ({ id: null, slug: '', label: '', category: 'brand', note: '', sort_order: 0, active: true })

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
        await createWatchFormat({ ...body, slug: editing.slug })
        toast.success(`Added ${editing.label}`)
      }
      editing = null
      load()
    } catch (e) {
      toast.error(e.message)
    } finally {
      saving = false
    }
  }

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
</script>

<PageHeader
  title="Premium formats"
  desc="The formats a watch can be logged in. One screening can carry several, because a brand, a projection, a sound system and a motion rig are separate things."
  info="Formats are shared with everyone who logs watches. Retiring one keeps it on the watches that already have it, which is why there is no delete."
>
  {#snippet actions()}
    <button class="btn btn-primary" onclick={() => (editing = blank())}>
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
  {#each CATEGORIES as cat}
    {@const rows = formats.filter((f) => f.category === cat.value)}
    {#if rows.length}
      <div class="card">
        <h3>{cat.label} <span class="muted">— {cat.hint}</span></h3>
        <table class="table">
          <thead>
            <tr>
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
                  <button class="btn btn-sm" onclick={() => (editing = { ...f, note: f.note ?? '' })}>Edit</button>
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
  <Modal title={editing.id ? `Edit ${editing.label}` : 'Add a format'} onclose={() => (editing = null)}>
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
    {#snippet footer()}
      <button class="btn" onclick={() => (editing = null)}>Cancel</button>
      <button class="btn btn-primary" onclick={save} disabled={saving || !editing.label}>
        {saving ? 'Saving…' : 'Save'}
      </button>
    {/snippet}
  </Modal>
{/if}

<style>
  h3 { margin: 0 0 0.75rem; font-size: 0.95rem; }
  .muted { color: var(--muted); font-weight: 400; font-size: 0.85rem; }
  .dim td { opacity: 0.55; }
  .right { text-align: right; white-space: nowrap; }
  .form-row { margin-bottom: 0.9rem; display: grid; gap: 0.3rem; }
  .form-row label { font-size: 0.85rem; color: var(--muted); }
</style>
