<!-- Sign in. Accounts are created by an admin in the console; there is no self sign-up. -->
<script>
  import { signIn } from '../lib/session.svelte.js'

  let username = $state('')
  let password = $state('')
  let busy = $state(false)
  let error = $state('')

  async function submit(e) {
    e.preventDefault()
    busy = true
    error = ''
    try {
      await signIn(username.trim(), password)
    } catch (err) {
      error = err.status === 401 ? 'That username and password don’t match an account.' : err.message
      password = ''
    } finally {
      busy = false
    }
  }
</script>

<main class="wrap">
  <form class="card" onsubmit={submit} aria-labelledby="signin-h">
    <p class="brand">metareel</p>
    <h1 id="signin-h">Sign in</h1>
    <p class="lede">Charts from every service, and your own watchlist and diary alongside them.</p>
    <label for="u">Username</label>
    <input id="u" autocomplete="username" bind:value={username} required />
    <label for="p">Password</label>
    <input id="p" type="password" autocomplete="current-password" bind:value={password} required />
    {#if error}<p class="err" role="alert">{error}</p>{/if}
    <button class="btn-primary" type="submit" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</button>
    <p class="fine">No account? Ask whoever runs this metareel to create one in the admin console.</p>
  </form>
</main>

<style>
  .wrap { min-height: 100vh; display: grid; place-items: center; padding: 24px 16px; }
  .card { width: min(400px, 100%); display: grid; gap: 8px; padding: 32px; border-radius: 22px; background: var(--bg-surface); }
  .brand { margin: 0 0 12px; font: 700 22px/1 var(--font-display); }
  h1 { margin: 0; font: 700 32px/1.1 var(--font-display); }
  .lede { margin: 4px 0 16px; color: var(--text-muted); font: 400 16px/23px var(--font-display); }
  label { margin-top: 6px; color: var(--text-muted); font-size: 13px; font-weight: 600; }
  input { padding: 11px 13px; border: 1px solid var(--border-subtle); border-radius: 11px; background: var(--bg-canvas); color: var(--text-primary); font: 400 15px/1.3 var(--font-sans); }
  .btn-primary { justify-content: center; margin-top: 14px; height: 44px; border-radius: 22px; }
  .err { margin: 6px 0 0; color: var(--delta-down); }
  .fine { margin: 12px 0 0; color: var(--text-muted); font-size: 13px; }
</style>
