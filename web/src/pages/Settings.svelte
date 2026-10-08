<script>
  import { getSettings, updateSettings, listApiKeys, createApiKey, renameApiKey, rotateApiKey, deleteApiKey } from '../lib/api.js'
  import { relativeTime, dateTime } from '../lib/format.js'
  import { toast } from '../lib/toast.svelte.js'
  import { confirm } from '../lib/confirm.svelte.js'
  import Icon from '../components/Icon.svelte'
  import Modal from '../components/Modal.svelte'

  let settings = $state(null)
  let rateLimit = $state(0)
  let keys = $state([])
  let error = $state('')
  let newName = $state('')
  let creating = $state(false)
  let revealed = $state(null) // { name, key, rotated }
  let renaming = $state(null) // { id, name }

  async function load() {
    try {
      const [s, k] = await Promise.all([getSettings(), listApiKeys()])
      settings = s
      rateLimit = s.rate_limit_per_minute
      keys = k
      error = ''
    } catch (e) {
      error = e.message
    }
  }
  $effect(() => { load() })

  async function saveSettings(patch) {
    try {
      settings = await updateSettings(patch)
      rateLimit = settings.rate_limit_per_minute
      toast.success('Settings saved')
    } catch (e) {
      toast.error(e.message)
    }
  }

  async function toggleRequire() {
    if (settings.require_api_key) {
      const ok = await confirm({
        title: 'Make the public API open?',
        message: 'Anyone who can reach the server will be able to call the public API without a key. Requests are still rate limited per IP.',
        confirmLabel: 'Allow without key',
        danger: true,
      })
      if (!ok) return
    }
    saveSettings({ require_api_key: !settings.require_api_key })
  }

  async function create() {
    creating = true
    try {
      const k = await createApiKey(newName)
      revealed = { name: k.name, key: k.key, rotated: false }
      newName = ''
      load()
    } catch (e) {
      toast.error(e.message)
    } finally {
      creating = false
    }
  }

  async function rotate(k) {
    const ok = await confirm({
      title: `Rotate "${k.name}"?`,
      message: 'A new key is generated and the current one stops working immediately. Update every client that uses it.',
      confirmLabel: 'Rotate key',
      danger: true,
    })
    if (!ok) return
    try {
      const r = await rotateApiKey(k.id)
      revealed = { name: r.name, key: r.key, rotated: true }
      load()
    } catch (e) {
      toast.error(e.message)
    }
  }

  async function revoke(k) {
    const ok = await confirm({
      title: `Revoke "${k.name}"?`,
      message: 'The key stops working immediately and is deleted. This cannot be undone.',
      confirmLabel: 'Revoke key',
      danger: true,
    })
    if (!ok) return
    try {
      await deleteApiKey(k.id)
      toast.success(`"${k.name}" revoked`)
      load()
    } catch (e) {
      toast.error(e.message)
    }
  }

  async function saveRename() {
    try {
      await renameApiKey(renaming.id, renaming.name)
      renaming = null
      load()
    } catch (e) {
      toast.error(e.message)
    }
  }

  async function copy(text) {
    try {
      await navigator.clipboard.writeText(text)
      toast.success('Copied to clipboard')
    } catch {
      toast.error('Copy failed — select and copy it manually')
    }
  }

  const origin = window.location.origin
</script>

<div class="page">
  <div class="page-header">
    <div>
      <h1 class="page-title">Settings & API keys</h1>
      <p class="page-desc">Control who can use the public API (<span class="mono">/api/v1</span>, everything except <span class="mono">/api/v1/admin</span>).</p>
    </div>
  </div>

  {#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

  {#if settings}
    {#if settings.require_api_key && keys.length === 0}
      <div class="alert alert-warning"><Icon name="alert" /><div>The public API requires a key but none exist, so every public request is rejected. Create a key below.</div></div>
    {/if}

    <div class="card">
      <div class="card-header"><div class="card-title">Public API access</div></div>
      <div class="card-body stack" style="gap:16px">
        <div class="row" style="gap:14px">
          <button class="switch" class:on={settings.require_api_key} role="switch" aria-checked={settings.require_api_key} aria-label="Require API key" onclick={toggleRequire}></button>
          <div>
            <div style="font-weight:550">Require an API key</div>
            <div class="small muted">Clients send it in the <span class="mono">X-API-Key</span> header (or <span class="mono">Authorization: Bearer …</span>). Turn off only for local development.</div>
          </div>
        </div>
        <div class="row" style="gap:14px;flex-wrap:wrap">
          <label class="row">
            <span style="font-weight:550">Rate limit</span>
            <input class="input" type="number" min="0" style="width:110px" bind:value={rateLimit} />
            <span class="muted">requests / minute per key</span>
          </label>
          <button class="btn btn-sm" onclick={() => saveSettings({ rate_limit_per_minute: Number(rateLimit) })} disabled={Number(rateLimit) === settings.rate_limit_per_minute}>Save</button>
          <span class="small muted">0 turns it off. Without keys, the limit applies per client IP. Over the limit, clients get HTTP 429.</span>
        </div>
        <div class="alert alert-info small" style="margin:0">
          <Icon name="alert" size={14} />
          <div>The admin console and <span class="mono">/api/v1/admin</span> are not protected by API keys. Expose only the public API to the internet (e.g. block <span class="mono">/api/v1/admin</span> and <span class="mono">/</span> at your reverse proxy).</div>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="card-header">
        <div class="card-title">API keys</div>
        <form class="row" onsubmit={(e) => { e.preventDefault(); create() }}>
          <input class="input" placeholder="Key name, e.g. tides" bind:value={newName} maxlength="100" />
          <button class="btn btn-primary" type="submit" disabled={!newName.trim() || creating}><Icon name="key" size={14} />Generate key</button>
        </form>
      </div>
      <div class="table-wrap">
        <table class="table">
          <thead><tr><th>Name</th><th>Key</th><th>Created</th><th>Rotated</th><th>Last used</th><th></th></tr></thead>
          <tbody>
            {#each keys as k (k.id)}
              <tr>
                <td>
                  {#if renaming?.id === k.id}
                    <form class="row" onsubmit={(e) => { e.preventDefault(); saveRename() }}>
                      <input class="input" bind:value={renaming.name} maxlength="100" />
                      <button class="btn btn-sm btn-primary" type="submit">Save</button>
                      <button class="btn btn-sm" type="button" onclick={() => (renaming = null)}>Cancel</button>
                    </form>
                  {:else}
                    <span class="cell-title">{k.name}</span>
                  {/if}
                </td>
                <td class="mono">{k.prefix}</td>
                <td class="nowrap" title={dateTime(k.created_at)}>{relativeTime(k.created_at)}</td>
                <td class="nowrap" title={k.rotated_at ? dateTime(k.rotated_at) : ''}>{k.rotated_at ? relativeTime(k.rotated_at) : '—'}</td>
                <td class="nowrap" title={k.last_used_at ? dateTime(k.last_used_at) : ''}>{k.last_used_at ? relativeTime(k.last_used_at) : 'Never'}</td>
                <td class="right nowrap">
                  <button class="btn btn-sm" onclick={() => (renaming = { id: k.id, name: k.name })}><Icon name="edit" size={12} />Rename</button>
                  <button class="btn btn-sm" onclick={() => rotate(k)}><Icon name="refresh" size={12} />Rotate</button>
                  <button class="btn btn-sm btn-danger" onclick={() => revoke(k)}><Icon name="trash" size={12} />Revoke</button>
                </td>
              </tr>
            {:else}
              <tr><td colspan="6"><div class="empty"><div class="empty-title">No API keys</div>Generate one for each client (e.g. one per app) so you can rotate or revoke them separately.</div></td></tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
</div>

{#if revealed}
  <Modal title={revealed.rotated ? `New key for "${revealed.name}"` : `API key "${revealed.name}" created`} onclose={() => (revealed = null)}>
    <div class="alert alert-warning small"><Icon name="alert" size={14} />Copy this key now. It is not stored and will not be shown again.</div>
    <div class="secret">{revealed.key}</div>
    <div class="row" style="margin-top:10px"><button class="btn" onclick={() => copy(revealed.key)}><Icon name="copy" size={14} />Copy key</button></div>
    <div class="section-title" style="margin-top:18px">Example</div>
    <div class="secret" style="border-style:solid;border-color:var(--border)">curl -H "X-API-Key: {revealed.key}" {origin}/api/v1/top10/movies/US</div>
    {#snippet footer()}<button class="btn btn-primary" onclick={() => (revealed = null)}>Done</button>{/snippet}
  </Modal>
{/if}
