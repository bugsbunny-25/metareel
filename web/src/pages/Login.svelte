<script>
  import Icon from '../components/Icon.svelte'
  import { auth, signIn, signOut } from '../lib/auth.svelte.js'
  import { route, navigate, href } from '../lib/router.svelte.js'

  let username = $state('')
  let password = $state('')
  let busy = $state(false)
  let error = $state('')

  // Signed in, but the console needs the admin role.
  const noAccess = $derived(auth.user && auth.user.role !== 'admin')

  async function submit(e) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      await signIn(username.trim(), password)
      password = ''
      // Opened from the "Sign in" link: go back to where the user was.
      if (route.path === '/login') navigate(route.query.next || '/')
    } catch (err) {
      error = err.message
    } finally {
      busy = false
    }
  }

  async function switchUser() {
    await signOut()
    username = ''
  }
</script>

<div class="login-wrap">
  <div class="card login-card">
    <div class="brand login-brand">
      <div class="brand-mark">m</div>
      <div>
        <div class="brand-name">metareel</div>
        <div class="brand-sub">Streaming charts admin</div>
      </div>
    </div>

    {#if noAccess}
      <div class="login-title">No access</div>
      <p class="small muted">
        You're signed in as <strong>{auth.user.username}</strong> ({auth.user.role}). The admin console needs an account with the
        admin role; ask an admin to change your role.
      </p>
      <button class="btn" onclick={switchUser}><Icon name="logOut" size={14} />Sign in as someone else</button>
    {:else}
      <div class="login-title">Sign in</div>
      {#if auth.loginRequired}
        <p class="small muted">This server requires an admin account to use the console.</p>
      {/if}
      {#if error}<div class="alert alert-error small"><Icon name="alert" size={14} />{error}</div>{/if}
      <form onsubmit={submit}>
        <label class="field">
          <span class="field-label">Username</span>
          <!-- svelte-ignore a11y_autofocus -->
          <input class="input" bind:value={username} autocomplete="username" autocapitalize="none" spellcheck="false" required autofocus />
        </label>
        <label class="field">
          <span class="field-label">Password</span>
          <input class="input" type="password" bind:value={password} autocomplete="current-password" required />
        </label>
        <button class="btn btn-primary login-submit" type="submit" disabled={busy || !username.trim() || !password}>
          {#if busy}<span class="spinner"></span>{:else}<Icon name="lock" size={14} />{/if}Sign in
        </button>
      </form>
      {#if !auth.loginRequired}
        <p class="small muted login-foot"><a href={href(route.query.next || '/')}>Back to the console</a> (signing in is optional on this server)</p>
      {/if}
    {/if}
  </div>
</div>
