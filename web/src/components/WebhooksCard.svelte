<script>
  // Outbound webhooks: subscriptions, test pings and recent deliveries.
  import { listWebhooks, createWebhook, updateWebhook, deleteWebhook, rotateWebhookSecret, testWebhook, listWebhookDeliveries, getWebhookDelivery, retryWebhookDelivery } from '../lib/api.js'
  import { relativeTime, dateTime, number } from '../lib/format.js'
  import { toast } from '../lib/toast.svelte.js'
  import { confirm } from '../lib/confirm.svelte.js'
  import Icon from './Icon.svelte'
  import Modal from './Modal.svelte'
  import InfoTip from './InfoTip.svelte'

  const EVENT_LABELS = {
    'chart.updated': 'Chart stored or changed',
    'title.entered': 'Title entered a chart',
    'title.number_one': 'New #1 on a chart',
    'availability.added': 'Title arrived on a service',
    'availability.removed': 'Title left a service',
    'title.breakout': 'Title rising in several countries',
  }

  let hooks = $state([])
  let eventTypes = $state([])
  let error = $state('')
  let editing = $state(null) // { id?, name, url, all, events: {type: bool}, enabled }
  let saving = $state(false)
  let revealed = $state(null) // { name, secret, rotated }
  let openId = $state(null) // webhook whose deliveries are shown
  let deliveries = $state([])
  let payload = $state(null) // { id, json }
  let busy = $state({})

  async function load() {
    try {
      const r = await listWebhooks()
      hooks = r.items
      eventTypes = r.event_types
      error = ''
    } catch (e) {
      error = e.message
    }
  }
  $effect(() => { load() })

  function startCreate() {
    editing = { name: '', url: '', all: true, events: Object.fromEntries(eventTypes.map((t) => [t, false])), enabled: true }
  }
  function startEdit(h) {
    const all = h.events.includes('*')
    editing = { id: h.id, name: h.name, url: h.url, all, events: Object.fromEntries(eventTypes.map((t) => [t, all || h.events.includes(t)])), enabled: h.enabled }
  }
  const chosen = (ed) => (ed.all ? ['*'] : eventTypes.filter((t) => ed.events[t]))

  async function save() {
    saving = true
    try {
      const body = { name: editing.name, url: editing.url, events: chosen(editing), enabled: editing.enabled }
      if (editing.id) {
        await updateWebhook(editing.id, body)
        toast.success('Webhook saved')
      } else {
        const w = await createWebhook(body)
        revealed = { name: w.name, secret: w.secret, rotated: false }
      }
      editing = null
      load()
    } catch (e) {
      toast.error(e.message)
    } finally {
      saving = false
    }
  }

  async function toggle(h) {
    try {
      await updateWebhook(h.id, { enabled: !h.enabled })
      load()
    } catch (e) {
      toast.error(e.message)
    }
  }

  async function test(h) {
    busy = { ...busy, [h.id]: true }
    try {
      const r = await testWebhook(h.id)
      if (r.delivery.status === 'delivered') toast.success(`Ping delivered (HTTP ${r.status_code}, ${r.duration_ms} ms)`)
      else toast.error(`Ping failed: ${r.error}`)
      load()
      if (openId === h.id) showDeliveries(h.id)
    } catch (e) {
      toast.error(e.message)
    } finally {
      busy = { ...busy, [h.id]: false }
    }
  }

  async function rotate(h) {
    const ok = await confirm({ title: `New secret for "${h.name}"?`, message: 'The current secret stops working at once: update the receiver to verify with the new one.', confirmLabel: 'Rotate secret', danger: true })
    if (!ok) return
    try {
      const r = await rotateWebhookSecret(h.id)
      revealed = { name: r.name, secret: r.secret, rotated: true }
    } catch (e) {
      toast.error(e.message)
    }
  }

  async function remove(h) {
    const ok = await confirm({ title: `Delete "${h.name}"?`, message: 'Pending deliveries are dropped and its delivery log is deleted.', confirmLabel: 'Delete webhook', danger: true })
    if (!ok) return
    try {
      await deleteWebhook(h.id)
      if (openId === h.id) openId = null
      toast.success(`"${h.name}" deleted`)
      load()
    } catch (e) {
      toast.error(e.message)
    }
  }

  async function showDeliveries(id) {
    openId = id
    try {
      deliveries = (await listWebhookDeliveries(id, { limit: 25 })).items
    } catch (e) {
      toast.error(e.message)
    }
  }

  async function retry(d) {
    try {
      await retryWebhookDelivery(d.id)
      toast.success('Queued again; sent within 30 seconds')
      showDeliveries(d.webhook_id)
    } catch (e) {
      toast.error(e.message)
    }
  }

  async function viewPayload(d) {
    try {
      payload = { id: d.id, json: JSON.stringify(await getWebhookDelivery(d.id), null, 2) }
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

  const statusClass = (s) => (s === 'delivered' ? 'badge-success' : s === 'failed' ? 'badge-danger' : 'badge-warning')
  const host = (u) => { try { return new URL(u).host } catch { return u } }
  // Delivery log times are UTC "YYYY-MM-DD HH:MM:SS".
  const utc = (s) => (s ? s.replace(' ', 'T') + 'Z' : null)
</script>

<div class="card">
  <div class="card-header">
    <div class="card-title">Webhooks <InfoTip text="metareel POSTs a signed JSON event to each URL when a chart is stored or changes, a title enters a chart or reaches #1, or a title arrives on or leaves a service. Failed deliveries are retried for about 15 hours." /></div>
    <button class="btn btn-primary" onclick={startCreate} disabled={!eventTypes.length}><Icon name="plus" size={14} />Add webhook</button>
  </div>
  {#if error}<div class="card-body"><div class="alert alert-error"><Icon name="alert" />{error}</div></div>{/if}
  <div class="table-wrap">
    <table class="table">
      <thead><tr><th>Name</th><th>Receiver</th><th>Events</th><th>Last delivery</th><th class="right">Pending</th><th class="right">Failed</th><th>On</th><th></th></tr></thead>
      <tbody>
        {#each hooks as h (h.id)}
          <tr class:selected={openId === h.id}>
            <td><span class="cell-title">{h.name}</span></td>
            <td class="mono small truncate" style="max-width:220px" title={h.url}>{host(h.url)}</td>
            <td class="small">{h.events.includes('*') ? 'All events' : h.events.join(', ')}</td>
            <td class="nowrap small">
              {#if h.last_delivery_at}
                <span class="badge {statusClass(h.last_status)}">{h.last_status}</span>
                <span title={dateTime(h.last_delivery_at)}>{relativeTime(h.last_delivery_at)}</span>
                {#if h.last_status === 'failed' && h.last_error}<div class="muted truncate" style="max-width:240px" title={h.last_error}>{h.last_error}</div>{/if}
              {:else}<span class="muted">Never</span>{/if}
            </td>
            <td class="right">{number(h.deliveries.pending ?? 0)}</td>
            <td class="right">{number(h.deliveries.failed ?? 0)}</td>
            <td><button class="switch" class:on={h.enabled} role="switch" aria-checked={h.enabled} aria-label="Enabled" onclick={() => toggle(h)}></button></td>
            <td class="right nowrap">
              <button class="btn btn-sm" onclick={() => test(h)} disabled={busy[h.id]}><Icon name="play" size={12} />{busy[h.id] ? 'Sending…' : 'Test'}</button>
              <button class="btn btn-sm" onclick={() => (openId === h.id ? (openId = null) : showDeliveries(h.id))}>{openId === h.id ? 'Hide log' : 'Log'}</button>
              <button class="btn btn-sm" onclick={() => startEdit(h)}><Icon name="edit" size={12} />Edit</button>
              <button class="btn btn-sm" onclick={() => rotate(h)}><Icon name="refresh" size={12} />Secret</button>
              <button class="btn btn-sm btn-danger" onclick={() => remove(h)} aria-label="Delete {h.name}"><Icon name="trash" size={12} /></button>
            </td>
          </tr>
          {#if openId === h.id}
            <tr>
              <td colspan="8" style="background:var(--surface-2)">
                <div class="section-title">Latest deliveries</div>
                <table class="table">
                  <thead><tr><th>Event</th><th>Status</th><th class="right">Attempts</th><th>HTTP</th><th>Queued</th><th>Next try / delivered</th><th></th></tr></thead>
                  <tbody>
                    {#each deliveries as d (d.id)}
                      <tr>
                        <td class="mono small">{d.event_type}</td>
                        <td><span class="badge {statusClass(d.status)}">{d.status}</span>{#if d.last_error}<div class="muted small truncate" style="max-width:260px" title={d.last_error}>{d.last_error}</div>{/if}</td>
                        <td class="right">{d.attempts}</td>
                        <td class="small">{d.last_status_code ?? '—'}</td>
                        <td class="nowrap small" title={dateTime(utc(d.created_at))}>{relativeTime(utc(d.created_at))}</td>
                        <td class="nowrap small">{#if d.status === 'pending'}{relativeTime(utc(d.next_attempt_at))}{:else if d.delivered_at}{relativeTime(utc(d.delivered_at))}{:else}—{/if}</td>
                        <td class="right nowrap">
                          <button class="btn btn-sm" onclick={() => viewPayload(d)}>Body</button>
                          {#if d.status !== 'pending'}<button class="btn btn-sm" onclick={() => retry(d)}>Send again</button>{/if}
                        </td>
                      </tr>
                    {:else}
                      <tr><td colspan="7"><div class="empty">No deliveries yet. Events are queued as charts and availability change; use Test to send a ping now.</div></td></tr>
                    {/each}
                  </tbody>
                </table>
              </td>
            </tr>
          {/if}
        {:else}
          <tr><td colspan="8"><div class="empty"><div class="empty-title">No webhooks</div>Add one so a client (e.g. tides) is told about chart and availability changes instead of polling.</div></td></tr>
        {/each}
      </tbody>
    </table>
  </div>
</div>

{#if editing}
  <Modal title={editing.id ? `Edit "${editing.name}"` : 'Add webhook'} onclose={() => (editing = null)}>
    <form class="stack" style="gap:14px" onsubmit={(e) => { e.preventDefault(); save() }}>
      <label class="stack" style="gap:4px"><span style="font-weight:550">Name</span><input class="input" bind:value={editing.name} maxlength="100" placeholder="tides" /></label>
      <label class="stack" style="gap:4px"><span style="font-weight:550">URL</span><input class="input mono" bind:value={editing.url} placeholder="https://tides.example/hooks/metareel" /></label>
      <fieldset class="stack" style="gap:6px;border:0;padding:0;margin:0">
        <legend style="font-weight:550;margin-bottom:6px">Events</legend>
        <label class="row"><input type="checkbox" bind:checked={editing.all} /> All events (including ones added later)</label>
        {#each eventTypes as t (t)}
          <label class="row" style="padding-left:18px"><input type="checkbox" bind:checked={editing.events[t]} disabled={editing.all} /> {EVENT_LABELS[t] ?? t} <span class="mono muted small">{t}</span></label>
        {/each}
      </fieldset>
      <label class="row"><input type="checkbox" bind:checked={editing.enabled} /> Enabled</label>
    </form>
    {#snippet footer()}
      <button class="btn" onclick={() => (editing = null)}>Cancel</button>
      <button class="btn btn-primary" onclick={save} disabled={saving || !editing.name.trim() || !editing.url.trim() || !chosen(editing).length}>{saving ? 'Saving…' : editing.id ? 'Save' : 'Add webhook'}</button>
    {/snippet}
  </Modal>
{/if}

{#if revealed}
  <Modal title={revealed.rotated ? `New secret for "${revealed.name}"` : `Webhook "${revealed.name}" added`} onclose={() => (revealed = null)}>
    <div class="alert alert-warning small"><Icon name="alert" size={14} />Copy the signing secret now. It is not shown again (rotate it to get a new one).</div>
    <div class="secret">{revealed.secret}</div>
    <div class="row" style="margin-top:10px"><button class="btn" onclick={() => copy(revealed.secret)}><Icon name="copy" size={14} />Copy secret</button></div>
    <div class="section-title" style="margin-top:18px">Verifying deliveries</div>
    <p class="small muted">Each POST carries <span class="mono">X-Metareel-Signature: t=&lt;unix time&gt;,v1=&lt;hex&gt;</span>, where the hex is HMAC-SHA256 of <span class="mono">&lt;t&gt;.&lt;raw body&gt;</span> keyed with this secret. Reject old timestamps, and use <span class="mono">X-Metareel-Event-Id</span> to ignore repeats.</p>
    {#snippet footer()}<button class="btn btn-primary" onclick={() => (revealed = null)}>Done</button>{/snippet}
  </Modal>
{/if}

{#if payload}
  <Modal title={`Delivery #${payload.id}`} onclose={() => (payload = null)}>
    <pre class="secret" style="max-height:60vh;overflow:auto;white-space:pre-wrap">{payload.json}</pre>
    {#snippet footer()}<button class="btn" onclick={() => copy(payload.json)}><Icon name="copy" size={14} />Copy</button><button class="btn btn-primary" onclick={() => (payload = null)}>Close</button>{/snippet}
  </Modal>
{/if}
