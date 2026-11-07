<script>
  import PageHeader from '../components/PageHeader.svelte'
  import Icon from '../components/Icon.svelte'
  import Modal from '../components/Modal.svelte'
  import { listUsers, createUser, updateUser, deleteUser, logoutUser, listSessions, revokeSession } from '../lib/api.js'
  import { auth } from '../lib/auth.svelte.js'
  import { relativeTime, dateTime } from '../lib/format.js'
  import { toast } from '../lib/toast.svelte.js'
  import { confirm } from '../lib/confirm.svelte.js'

  let users = $state(null)
  let sessions = $state([])
  let error = $state('')
  let busy = $state('') // key of the action in progress
  let adding = $state(null) // { username, role, password, repeat }
  let resetting = $state(null) // { username, password, repeat }

  async function load() {
    try {
      const [u, s] = await Promise.all([listUsers(), listSessions()])
      users = u
      sessions = s
      error = ''
    } catch (e) {
      error = e.message
    }
  }
  $effect(() => { load() })

  const isMe = (username) => auth.user && auth.user.username.toLowerCase() === username.toLowerCase()
  const sessionCount = (username) => sessions.filter((s) => s.username === username).length

  // run performs an action with feedback; it reports whether it succeeded.
  async function run(key, fn, success) {
    busy = key
    try {
      await fn()
      if (success) toast.success(success)
      load()
      return true
    } catch (e) {
      toast.error(e.message)
      return false
    } finally {
      busy = ''
    }
  }

  function passwordProblem(f) {
    if (f.password.length < 10) return 'At least 10 characters'
    if (f.password !== f.repeat) return 'The passwords do not match'
    return ''
  }

  async function add(e) {
    e.preventDefault()
    const f = adding
    if (await run('add', () => createUser({ username: f.username.trim(), role: f.role, password: f.password }), `User "${f.username.trim()}" created`)) adding = null
  }

  async function changeRole(u, select) {
    const role = select.value
    if (role === u.role) return
    if (role !== 'admin') {
      const ok = await confirm({
        title: `Make ${u.username} a reader?`,
        message: 'Readers can use the public API but not the admin console. Their open console sessions stop working.',
        confirmLabel: 'Make reader',
        danger: true,
      })
      if (!ok) {
        select.value = u.role
        return
      }
    }
    if (!(await run(`role:${u.username}`, () => updateUser(u.username, { role }), `${u.username} is now ${role === 'admin' ? 'an admin' : 'a reader'}`))) select.value = u.role
  }

  async function toggleDisabled(u) {
    if (!u.disabled) {
      const ok = await confirm({
        title: `Disable ${u.username}?`,
        message: 'They are signed out everywhere and cannot sign in until enabled again.',
        confirmLabel: 'Disable',
        danger: true,
      })
      if (!ok) return
    }
    run(`dis:${u.username}`, () => updateUser(u.username, { disabled: !u.disabled }), u.disabled ? `${u.username} enabled` : `${u.username} disabled`)
  }

  async function signOutUser(u) {
    const ok = await confirm({
      title: isMe(u.username) ? 'Sign out your other sessions?' : `Sign ${u.username} out everywhere?`,
      message: isMe(u.username)
        ? 'Every session of yours except this one ends.'
        : 'Every session of theirs ends; they can sign in again.',
      confirmLabel: 'Sign out',
    })
    if (!ok) return
    busy = `out:${u.username}`
    try {
      const r = await logoutUser(u.username)
      toast.success(`${r.sessions_ended} session${r.sessions_ended === 1 ? '' : 's'} ended`)
      await load()
    } catch (e) {
      toast.error(e.message)
    } finally {
      busy = ''
    }
  }

  async function remove(u) {
    const ok = await confirm({
      title: `Delete ${u.username}?`,
      message: 'The account and all its sessions are deleted. This cannot be undone.',
      confirmLabel: 'Delete user',
      danger: true,
    })
    if (!ok) return
    run(`del:${u.username}`, () => deleteUser(u.username), `${u.username} deleted`)
  }

  async function savePassword(e) {
    e.preventDefault()
    const f = resetting
    const ok = await run('pw', () => updateUser(f.username, { password: f.password }),
      isMe(f.username) ? 'Password changed; your other sessions ended' : `Password changed; ${f.username} was signed out`)
    if (ok) resetting = null
  }

  async function revoke(s) {
    const ok = await confirm({
      title: s.current ? 'End this session?' : `End ${s.username}'s session?`,
      message: s.current ? 'You will be signed out of this browser.' : 'That browser or client is signed out at once.',
      confirmLabel: 'End session',
      danger: true,
    })
    if (!ok) return
    run(`ses:${s.id}`, () => revokeSession(s.id), 'Session ended')
  }

  // "Chrome on macOS" from a user agent; the raw string is in the tooltip.
  function device(ua) {
    if (!ua) return 'Unknown client'
    const browser =
      /Edg\//.test(ua) ? 'Edge' : /Firefox\//.test(ua) ? 'Firefox' : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari'
        : (ua.split(/[/ ]/)[0] || 'Client')
    const os = /Mac OS X|Macintosh/.test(ua) ? 'macOS' : /Windows/.test(ua) ? 'Windows' : /Android/.test(ua) ? 'Android'
      : /iPhone|iPad/.test(ua) ? 'iOS' : /Linux/.test(ua) ? 'Linux' : ''
    return os ? `${browser} on ${os}` : browser
  }
</script>

<div class="page">
  <PageHeader
    title="Users & sessions"
    desc="Who can sign in to the admin console and the public API, and where they are signed in."
    info="Admins can use the admin console and the public API; readers only the public API (with a session token instead of an API key). Passwords are stored as argon2id hashes."
  >
    {#snippet actions()}
      <button class="btn btn-primary" onclick={() => (adding = { username: '', role: 'reader', password: '', repeat: '' })}><Icon name="plus" size={14} />Add user</button>
    {/snippet}
  </PageHeader>

  {#if auth.loginRequired}
    <div class="alert alert-info small"><Icon name="lock" size={14} /><div>The admin console requires an admin sign-in (<span class="mono">AUTH_REQUIRE_ADMIN_LOGIN</span> is on).</div></div>
  {:else}
    <div class="alert alert-warning small"><Icon name="alert" size={14} /><div>Signing in is optional: anyone who can reach this console can use it. Once an admin exists, set <span class="mono">AUTH_REQUIRE_ADMIN_LOGIN=true</span> and restart to require an admin sign-in.</div></div>
  {/if}

  {#if error}<div class="alert alert-error"><Icon name="alert" />{error}</div>{/if}

  <div class="card">
    <div class="card-header"><div class="card-title">Users</div></div>
    <div class="table-wrap">
      <table class="table">
        <thead><tr><th>Username</th><th>Role</th><th>Status</th><th class="right">Sessions</th><th>Last sign-in</th><th>Created</th><th></th></tr></thead>
        <tbody>
          {#if users === null && !error}
            {#each [1, 2] as i (i)}<tr><td colspan="7"><div class="skeleton"></div></td></tr>{/each}
          {:else if users}
            {#each users as u (u.id)}
              {@const me = isMe(u.username)}
              <tr>
                <td class="nowrap"><span class="cell-title">{u.username}</span>{#if me} <span class="badge badge-accent">you</span>{/if}</td>
                <td>
                  <select class="select" value={u.role} aria-label="Role of {u.username}" disabled={me || busy !== ''} title={me ? "You can't change your own role" : ''}
                    onchange={(e) => changeRole(u, e.currentTarget)}>
                    <option value="admin">Admin</option>
                    <option value="reader">Reader</option>
                  </select>
                </td>
                <td>{#if u.disabled}<span class="badge badge-danger">Disabled</span>{:else}<span class="badge badge-success">Active</span>{/if}</td>
                <td class="right">{sessionCount(u.username)}</td>
                <td class="nowrap" title={u.last_login_at ? dateTime(u.last_login_at) : ''}>{u.last_login_at ? relativeTime(u.last_login_at) : 'Never'}</td>
                <td class="nowrap" title={dateTime(u.created_at)}>{relativeTime(u.created_at)}</td>
                <td class="right nowrap">
                  <button class="btn btn-sm" disabled={busy !== ''} title={me ? 'Change your password' : `Set a new password for ${u.username}`} onclick={() => (resetting = { username: u.username, password: '', repeat: '' })}><Icon name="key" size={12} />Password</button>
                  <button class="btn btn-sm" disabled={busy !== '' || sessionCount(u.username) <= (me ? 1 : 0)} title={me ? 'End your other sessions' : `End every session of ${u.username}`} onclick={() => signOutUser(u)}><Icon name="logOut" size={12} />Sign out</button>
                  {#if !me}
                    <button class="btn btn-sm" disabled={busy !== ''} onclick={() => toggleDisabled(u)}>{u.disabled ? 'Enable' : 'Disable'}</button>
                    <button class="btn btn-sm btn-danger btn-icon" disabled={busy !== ''} onclick={() => remove(u)} aria-label="Delete {u.username}" title="Delete {u.username}"><Icon name="trash" size={12} /></button>
                  {/if}
                </td>
              </tr>
            {:else}
              <tr><td colspan="7"><div class="empty"><div class="empty-title">No users yet</div>Add an admin, then you can require sign-in for this console.</div></td></tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>
  </div>

  <div class="card">
    <div class="card-header"><div class="card-title">Active sessions</div></div>
    <div class="table-wrap">
      <table class="table">
        <thead><tr><th>User</th><th>Client</th><th>IP</th><th>Signed in</th><th>Last active</th><th>Expires</th><th></th></tr></thead>
        <tbody>
          {#each sessions as s (s.id)}
            <tr>
              <td class="nowrap"><span class="cell-title">{s.username}</span>{#if s.current} <span class="badge badge-accent">this session</span>{/if}</td>
              <td title={s.user_agent}>{device(s.user_agent)}</td>
              <td class="mono">{s.ip || '—'}</td>
              <td class="nowrap" title={dateTime(s.created_at)}>{relativeTime(s.created_at)}</td>
              <td class="nowrap" title={dateTime(s.last_seen_at)}>{relativeTime(s.last_seen_at)}</td>
              <td class="nowrap" title={dateTime(s.expires_at)}>{relativeTime(s.expires_at)}</td>
              <td class="right"><button class="btn btn-sm btn-danger" disabled={busy !== ''} onclick={() => revoke(s)} aria-label="End session {s.id} of {s.username}">End</button></td>
            </tr>
          {:else}
            <tr><td colspan="7"><div class="empty"><div class="empty-title">No active sessions</div>Sessions appear here when someone signs in to the console or the API.</div></td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
</div>

{#if adding}
  <Modal title="Add user" onclose={() => (adding = null)}>
    <form id="add-user" onsubmit={add}>
      <label class="field">
        <span class="field-label">Username</span>
        <input class="input" bind:value={adding.username} autocomplete="off" autocapitalize="none" spellcheck="false" maxlength="64" required />
        <span class="field-hint">2-64 letters, digits, '.', '_' or '-'.</span>
      </label>
      <label class="field">
        <span class="field-label">Role</span>
        <select class="select" bind:value={adding.role}>
          <option value="reader">Reader: public API only</option>
          <option value="admin">Admin: admin console and public API</option>
        </select>
      </label>
      <div class="field-row">
        <label class="field"><span class="field-label">Password</span><input class="input" type="password" bind:value={adding.password} autocomplete="new-password" required /></label>
        <label class="field"><span class="field-label">Repeat password</span><input class="input" type="password" bind:value={adding.repeat} autocomplete="new-password" required /></label>
      </div>
      {#if adding.password && passwordProblem(adding)}<div class="field-hint">{passwordProblem(adding)}</div>{/if}
    </form>
    {#snippet footer()}
      <button class="btn" onclick={() => (adding = null)}>Cancel</button>
      <button class="btn btn-primary" type="submit" form="add-user" disabled={busy !== '' || !adding.username.trim() || !!passwordProblem(adding)}>Create user</button>
    {/snippet}
  </Modal>
{/if}

{#if resetting}
  <Modal title={isMe(resetting.username) ? 'Change your password' : `New password for ${resetting.username}`} onclose={() => (resetting = null)}>
    <form id="reset-pw" onsubmit={savePassword}>
      <p class="small muted" style="margin-top:0">
        {isMe(resetting.username) ? 'Your other sessions end; this one stays signed in.' : `${resetting.username} is signed out everywhere and signs in with the new password.`}
      </p>
      <div class="field-row">
        <label class="field"><span class="field-label">New password</span><input class="input" type="password" bind:value={resetting.password} autocomplete="new-password" required /></label>
        <label class="field"><span class="field-label">Repeat password</span><input class="input" type="password" bind:value={resetting.repeat} autocomplete="new-password" required /></label>
      </div>
      {#if resetting.password && passwordProblem(resetting)}<div class="field-hint">{passwordProblem(resetting)}</div>{/if}
    </form>
    {#snippet footer()}
      <button class="btn" onclick={() => (resetting = null)}>Cancel</button>
      <button class="btn btn-primary" type="submit" form="reset-pw" disabled={busy !== '' || !!passwordProblem(resetting)}>Save password</button>
    {/snippet}
  </Modal>
{/if}
