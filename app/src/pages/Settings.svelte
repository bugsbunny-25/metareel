<!-- Settings (#/settings): your account, preferences that follow you to every device
     (/me/preferences), this device's theme, your password, and where you're signed in
     (/me/account sessions). -->
<script>
  import { api } from '../lib/api.js'
  import { session, signOut } from '../lib/session.svelte.js'
  import { scope, reference, setScope, providerName } from '../lib/scope.svelte.js'
  import { prefs, savePrefs } from '../lib/prefs.svelte.js'
  import { theme, setTheme } from '../lib/theme.svelte.js'
  import { toast } from '../lib/toast.svelte.js'
  import { fromUTC, sinceDay } from '../lib/dates.js'
  import { currencyFor } from '../lib/money.js'

  let { onconfirm } = $props()

  let account = $state(null)
  let error = $state('')
  let nonce = $state(0)
  $effect(() => {
    nonce
    const ctrl = new AbortController()
    api.account(ctrl.signal)
      .then((a) => { account = a; error = '' })
      .catch((err) => { if (err.name !== 'AbortError') error = err.message })
    return () => ctrl.abort()
  })

  // Preferences form: a draft of the saved values, with this device's scope as the fallback.
  const CURRENCIES = ['USD', 'GBP', 'EUR', 'INR', 'CAD', 'AUD', 'JPY', 'KRW', 'BRL', 'MXN', 'CHF', 'SEK', 'SGD', 'AED']
  let draft = $state(null)
  let saving = $state(false)
  let prefsError = $state('')
  $effect(() => {
    if (prefs.loaded && !draft) draft = fromPrefs(prefs.values)
  })
  function fromPrefs(v) {
    const country = v.country ?? scope.country
    return {
      country, provider: v.provider ?? scope.provider,
      currency: v.currency ?? '', screen_unit: v.screen_unit ?? 'm', stale_days: v.stale_days ?? 30,
    }
  }
  const providersFor = (cc) => reference.providers.filter((p) => p.countries.includes(cc))
  const dirty = $derived(!!draft && JSON.stringify(draft) !== JSON.stringify(fromPrefs(prefs.values)))
  $effect(() => {
    // A service has to chart in the chosen country.
    if (!draft || !reference.loaded) return
    const list = providersFor(draft.country)
    if (list.length && !list.some((p) => p.slug === draft.provider)) draft.provider = list[0].slug
  })

  async function save(e) {
    e.preventDefault()
    prefsError = ''
    const days = Number(draft.stale_days)
    if (!Number.isInteger(days) || days < 7 || days > 365) { prefsError = 'Pick between 7 and 365 days.'; return }
    const cur = draft.currency.trim().toUpperCase()
    if (cur && !/^[A-Z]{3}$/.test(cur)) { prefsError = 'A currency is a 3-letter code, e.g. USD.'; return }
    saving = true
    try {
      const v = await savePrefs({
        country: draft.country, provider: draft.provider, currency: cur || null,
        screen_unit: draft.screen_unit, stale_days: days,
      })
      draft = fromPrefs(v)
      // Saving the default also switches this device to it.
      setScope({ country: v.country, provider: v.provider })
      toast('Preferences saved.')
    } catch (err) {
      prefsError = sentence(err.message)
    } finally {
      saving = false
    }
  }

  // Password
  let pw = $state({ current: '', next: '', again: '' })
  let pwBusy = $state(false)
  let pwError = $state('')
  async function changePassword(e) {
    e.preventDefault()
    pwError = ''
    if (pw.next.length < 10) { pwError = 'The new password needs at least 10 characters.'; return }
    if (pw.next !== pw.again) { pwError = 'The new passwords don’t match.'; return }
    pwBusy = true
    try {
      const r = await api.changePassword(pw.current, pw.next)
      pw = { current: '', next: '', again: '' }
      const n = r.other_sessions_ended ?? 0
      toast(n ? `Password changed. Signed out ${n} other ${n === 1 ? 'session' : 'sessions'}.` : 'Password changed.')
      nonce++
    } catch (err) {
      pwError = sentence(err.message)
    } finally {
      pwBusy = false
    }
  }

  // Sessions
  const others = $derived((account?.sessions ?? []).filter((s) => !s.current))
  async function endSession(s) {
    try {
      await api.endSession(s.id)
      toast(`Signed out ${device(s.user_agent).name}.`)
      nonce++
    } catch (err) {
      toast(err.message, 'error')
    }
  }
  function endOthers() {
    onconfirm({
      title: 'Sign out everywhere else?',
      body: `This signs out ${others.length} other ${others.length === 1 ? 'session' : 'sessions'}, including apps like tides that use your account. You stay signed in here.`,
      confirm: 'Sign out others',
      run: async () => {
        const r = await api.endOtherSessions()
        toast(`Signed out ${r.ended} ${r.ended === 1 ? 'session' : 'sessions'}.`)
        nonce++
      },
    })
  }

  /** "Chrome on macOS" from a user agent; apps that aren't browsers show their own name. */
  function device(ua = '') {
    if (!ua) return { name: 'Unknown device', kind: 'other' }
    const os = /iPhone/.test(ua) ? 'iPhone' : /iPad/.test(ua) ? 'iPad' : /Android/.test(ua) ? 'Android'
      : /CrOS/.test(ua) ? 'ChromeOS' : /Mac OS X|Macintosh/.test(ua) ? 'macOS' : /Windows/.test(ua) ? 'Windows' : /Linux/.test(ua) ? 'Linux' : ''
    const browser = /Edg\//.test(ua) ? 'Edge' : /OPR\//.test(ua) ? 'Opera' : /Firefox\//.test(ua) ? 'Firefox'
      : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) && /Version\//.test(ua) ? 'Safari' : ''
    if (browser) return { name: os ? `${browser} on ${os}` : browser, kind: /iPhone|Android/.test(ua) ? 'phone' : 'computer' }
    return { name: ua.split(/[\s/(]/)[0] || 'Unknown device', kind: 'app' }
  }
  const longDate = (ts) => fromUTC(ts)?.toLocaleDateString(undefined, { day: 'numeric', month: 'long', year: 'numeric' }) ?? ''
  /** Server messages are lower-case fragments; show them as sentences. */
  const sentence = (m) => (m ? m[0].toUpperCase() + m.slice(1) + (/[.!?]$/.test(m) ? '' : '.') : m)
  const THEMES = [['dark', 'Dark'], ['light', 'Light'], ['system', 'Match system']]
</script>

<div class="page">
  <h1>Settings</h1>
  {#if error}<p class="alert" role="alert">Couldn’t load your account: {error}</p>{/if}

  <section class="panel pad acct" aria-labelledby="s-account">
    <h2 id="s-account" class="sr">Account</h2>
    <span class="avatar" aria-hidden="true">{session.user.username.slice(0, 1).toUpperCase()}</span>
    <div class="who">
      <b>{session.user.username}</b>
      <span class="hint">
        {session.user.role === 'admin' ? 'Administrator' : 'Member'}{#if account?.user?.created_at}{' · '}joined {longDate(account.user.created_at)}{/if}
      </span>
    </div>
    <div class="acts">
      {#if session.user.role === 'admin'}<a class="btn-secondary sm" href="/admin/">Admin console</a>{/if}
      <button type="button" class="btn-secondary sm" onclick={signOut}>Sign out</button>
    </div>
  </section>

  <section class="panel pad" aria-labelledby="s-prefs">
    <div class="sh"><h2 id="s-prefs">Preferences</h2><span class="hint">Saved to your account, so they follow you to every device</span></div>
    {#if !draft}
      <div class="skeleton" style="height:180px" aria-busy="true" aria-label="Loading"></div>
    {:else}
      <form class="grid" onsubmit={save}>
        <div class="row">
          <div class="lab"><b>Default charts</b><span class="hint">What a new device opens on. Saving also switches this one.</span></div>
          <div class="ctl two">
            <label><span class="sr">Country</span>
              <select bind:value={draft.country}>{#each reference.countries as c (c.code)}<option value={c.code}>{c.name}</option>{/each}</select>
            </label>
            <label><span class="sr">Service</span>
              <select bind:value={draft.provider}>{#each providersFor(draft.country) as p (p.slug)}<option value={p.slug}>{providerName(p.slug)}</option>{/each}</select>
            </label>
          </div>
        </div>
        <div class="row">
          <div class="lab"><b>Currency</b><span class="hint">Prefilled for tickets and rentals. Empty uses the last one you entered.</span></div>
          <div class="ctl">
            <label><span class="sr">Currency</span>
              <input class="cc" bind:value={draft.currency} list="pref-currencies" maxlength="3" placeholder={currencyFor(draft.country)} />
            </label>
            <datalist id="pref-currencies">{#each CURRENCIES as c}<option value={c}></option>{/each}</datalist>
          </div>
        </div>
        <div class="row">
          <div class="lab"><b>Screen sizes</b><span class="hint">The unit you measure screens in.</span></div>
          <div class="ctl">
            <div class="seg" role="group" aria-label="Screen size unit">
              <button type="button" aria-pressed={draft.screen_unit === 'm'} onclick={() => (draft.screen_unit = 'm')}>Metres</button>
              <button type="button" aria-pressed={draft.screen_unit === 'ft'} onclick={() => (draft.screen_unit = 'ft')}>Feet</button>
            </div>
          </div>
        </div>
        <div class="row">
          <div class="lab"><b>“Haven’t watched in a while”</b><span class="hint">How long a show can sit untouched before Up next moves it out of Continue watching.</span></div>
          <div class="ctl">
            <label class="days"><input type="number" min="7" max="365" bind:value={draft.stale_days} aria-label="Days" /> days</label>
          </div>
        </div>
        <div class="foot">
          {#if prefsError}<p class="err" role="alert">{prefsError}</p>{/if}
          {#if prefs.updatedAt && !dirty}<span class="hint">Saved {sinceDay(prefs.updatedAt)}</span>{/if}
          <button type="button" class="btn-secondary sm" disabled={!dirty || saving} onclick={() => (draft = fromPrefs(prefs.values))}>Reset</button>
          <button type="submit" class="btn-primary sm" disabled={!dirty || saving}>{saving ? 'Saving…' : 'Save'}</button>
        </div>
      </form>
    {/if}
  </section>

  <section class="panel pad" aria-labelledby="s-look">
    <div class="sh"><h2 id="s-look">Appearance</h2><span class="hint">This device only</span></div>
    <div class="row">
      <div class="lab"><b>Theme</b><span class="hint">{theme.mode === 'system' ? `Following your system: ${theme.effective} right now.` : 'Dark is the default; artwork reads best on it.'}</span></div>
      <div class="ctl">
        <div class="seg" role="group" aria-label="Theme">
          {#each THEMES as [v, l]}<button type="button" aria-pressed={theme.mode === v} onclick={() => setTheme(v)}>{l}</button>{/each}
        </div>
      </div>
    </div>
  </section>

  <section class="panel pad" aria-labelledby="s-pw">
    <div class="sh"><h2 id="s-pw">Password</h2><span class="hint">Changing it signs you out everywhere else</span></div>
    <form class="pw" onsubmit={changePassword}>
      <input type="text" autocomplete="username" value={session.user.username} hidden readonly />
      <label>Current password<input type="password" autocomplete="current-password" bind:value={pw.current} required /></label>
      <label>New password<input type="password" autocomplete="new-password" bind:value={pw.next} required minlength="10" aria-describedby="pw-rule" /></label>
      <label>New password again<input type="password" autocomplete="new-password" bind:value={pw.again} required /></label>
      <p class="hint" id="pw-rule">At least 10 characters. A phrase of a few words is easier to remember than symbols.</p>
      <div class="foot">
        {#if pwError}<p class="err" role="alert">{pwError}</p>{/if}
        <button type="submit" class="btn-primary sm" disabled={pwBusy || !pw.current || !pw.next}>{pwBusy ? 'Changing…' : 'Change password'}</button>
      </div>
    </form>
  </section>

  <section class="panel pad" aria-labelledby="s-sessions">
    <div class="sh">
      <h2 id="s-sessions">Where you’re signed in</h2>
      {#if account}<span class="hint">{account.sessions.length} active {account.sessions.length === 1 ? 'session' : 'sessions'}</span>{/if}
      {#if others.length}<button type="button" class="quiet sm" onclick={endOthers}>Sign out everywhere else</button>{/if}
    </div>
    {#if !account}
      <div class="skeleton" style="height:120px" aria-busy="true" aria-label="Loading"></div>
    {:else}
      <ul class="sessions">
        {#each account.sessions as s (s.id)}
          {@const d = device(s.user_agent)}
          <li class:current={s.current}>
            <span class="dev" aria-hidden="true">{d.kind === 'phone' ? '▯' : d.kind === 'app' ? '◇' : '▭'}</span>
            <div class="sbody">
              <b>{d.name}{#if s.current}{' '}<span class="pill here">This device</span>{/if}</b>
              <span class="hint">
                {s.current ? 'Active now' : `Last active ${sinceDay(s.last_seen_at)}`}{' · '}signed in {longDate(s.created_at)}{#if s.ip}{' · '}{s.ip}{/if}
              </span>
            </div>
            {#if s.current}
              <button type="button" class="btn-secondary sm" onclick={signOut}>Sign out</button>
            {:else}
              <button type="button" class="btn-secondary sm" onclick={() => endSession(s)}>Sign out</button>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>
</div>

<style>
  .page { display: grid; gap: 16px; max-width: 860px; padding: 20px var(--gutter) 48px; }
  h1 { margin: 0 0 4px; font: 700 32px/1.1 var(--font-display); letter-spacing: -0.02em; }
  .sh { display: flex; flex-wrap: wrap; align-items: baseline; gap: 6px 12px; margin-bottom: 6px; }
  .sh .quiet { margin-left: auto; }
  .hint { color: var(--text-muted); font-size: 13px; }
  .sm { height: 34px; padding: 0 14px; font-size: 13.5px; }

  .acct { grid-template-columns: auto 1fr auto; align-items: center; gap: 16px; }
  .avatar { display: grid; place-items: center; width: 56px; height: 56px; border-radius: 50%; background: var(--accent-yours); color: #1a0d05; font: 700 24px/1 var(--font-display); }
  .who { display: grid; gap: 2px; }
  .who b { font: 700 22px/1.2 var(--font-display); }
  .acts { display: flex; flex-wrap: wrap; gap: 8px; justify-content: flex-end; }

  .grid { display: grid; gap: 0; }
  .row { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 320px); gap: 8px 24px; align-items: center; padding: 14px 0; border-top: 1px solid var(--border-subtle); }
  .grid .row:first-child { border-top: 0; padding-top: 4px; }
  .lab { display: grid; gap: 3px; }
  .lab b { font-size: 15px; }
  .ctl { display: grid; gap: 8px; justify-items: start; }
  .ctl.two { grid-template-columns: 1fr 1fr; justify-items: stretch; }
  .ctl.two label { display: grid; }
  .ctl.two select { width: 100%; }
  .days { display: inline-flex; align-items: center; gap: 8px; color: var(--text-muted); font-size: 14px; }
  .days input { width: 90px; }
  input, select { min-width: 0; padding: 9px 12px; border: 1px solid var(--border-subtle); border-radius: 10px; background: var(--bg-canvas); color: var(--text-primary); font: 400 15px/1.3 var(--font-sans); }
  input:focus-visible, select:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 0; }
  .cc { width: 110px; text-transform: uppercase; }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { height: 30px; padding: 0 14px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .seg button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }
  .foot { display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: 10px; padding-top: 12px; border-top: 1px solid var(--border-subtle); }
  .err { flex: 1 1 100%; margin: 0; color: var(--delta-down); font-size: 13.5px; }

  .pw { display: grid; gap: 12px; max-width: 420px; }
  .pw label { display: grid; gap: 6px; font-size: 13px; font-weight: 600; color: var(--text-muted); }
  .pw .foot { border-top: 0; padding-top: 0; justify-content: flex-start; }
  .pw .hint { margin: 0; }

  .sessions { display: grid; gap: 2px; margin: 0; padding: 0; list-style: none; }
  .sessions li { display: grid; grid-template-columns: 28px minmax(0, 1fr) auto; align-items: center; gap: 12px; padding: 10px 8px; border-radius: 10px; }
  .sessions li:hover { background: var(--bg-elevated); }
  .dev { color: var(--text-muted); font-size: 20px; text-align: center; }
  .sbody { display: grid; gap: 2px; min-width: 0; }
  .sbody b { font-size: 15px; }
  .sbody .hint { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .here { margin-left: 4px; background: color-mix(in srgb, var(--accent-yours) 18%, transparent); color: var(--accent-yours); }
  .quiet { border: 0; border-radius: 17px; background: none; color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .quiet:hover { background: var(--bg-elevated); color: var(--delta-down); }
  .sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }

  @media (max-width: 640px) {
    .row { grid-template-columns: 1fr; }
    .acct { grid-template-columns: auto 1fr; }
    .acts { grid-column: 1 / -1; justify-content: flex-start; }
    .ctl.two { grid-template-columns: 1fr; }
  }
</style>
