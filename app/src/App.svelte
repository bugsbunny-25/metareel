<!-- App shell: top bar (brand, primary nav, scope pickers, theme, account), the routed page,
     the "Log a watch" dialog and toasts. Signed out, it shows the sign-in screen instead. -->
<script>
  import { untrack } from 'svelte'
  import { route } from './lib/router.svelte.js'
  import { session, loadSession, signOut } from './lib/session.svelte.js'
  import { scope, reference, setScope, loadReference, providerName } from './lib/scope.svelte.js'
  import { library } from './lib/library.svelte.js'
  import { loadPrefs } from './lib/prefs.svelte.js'
  import { theme, toggleTheme } from './lib/theme.svelte.js'
  import { toasts } from './lib/toast.svelte.js'
  import LogWatch from './components/LogWatch.svelte'
  import ConfirmDialog from './components/ConfirmDialog.svelte'
  import SearchPalette from './components/SearchPalette.svelte'
  import Icon from './components/Icon.svelte'
  import Login from './pages/Login.svelte'
  import Home from './pages/Home.svelte'
  import Charts from './pages/Charts.svelte'
  import Title from './pages/Title.svelte'
  import Diary from './pages/Diary.svelte'
  import Watchlist from './pages/Watchlist.svelte'
  import Library from './pages/Library.svelte'
  import Stats from './pages/Stats.svelte'
  import UpNext from './pages/UpNext.svelte'
  import Settings from './pages/Settings.svelte'
  import Discover from './pages/Discover.svelte'
  import Show from './pages/Show.svelte'
  import Person from './pages/Person.svelte'

  let logTarget = $state(null)
  let confirmRequest = $state(null)
  let searchOpen = $state(false)

  // ⌘K / Ctrl K anywhere; "/" when not typing in a field.
  function onWindowKey(e) {
    if (!session.user) return
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault()
      searchOpen = !searchOpen
    } else if (e.key === '/' && !searchOpen && !e.target.closest?.('input, textarea, select, [contenteditable="true"], dialog')) {
      e.preventDefault()
      searchOpen = true
    }
  }
  const mac = /Mac|iPhone|iPad/.test(navigator.platform)
  const onconfirm = (r) => (confirmRequest = r)
  let bootError = $state('')
  const onlog = (t) => (logTarget = t)

  loadSession()

  // Once signed in: the pickers' reference lists and the library. Only the user is a
  // dependency; untrack keeps the loaders' own reads from re-running this.
  $effect(() => {
    if (!session.user) return
    untrack(() => {
      bootError = ''
      // Preferences after the reference lists, so a saved country/service pair is checked against them.
      Promise.all([loadReference().then(loadPrefs), library.load()]).catch((err) => (bootError = err.message))
    })
  })

  const titleMatch = $derived(route.path.match(/^\/title\/(movie|tv)\/(\d+)$/))
  const showMatch = $derived(route.path.match(/^\/show\/(\d+)$/))
  const personMatch = $derived(route.path.match(/^\/person\/(\d+)$/))
  const PAGES = { '/': 'home', '/charts': 'charts', '/discover': 'discover', '/up-next': 'upnext', '/watchlist': 'watchlist', '/library': 'library', '/diary': 'diary', '/stats': 'stats', '/settings': 'settings' }
  const page = $derived(PAGES[route.path] ?? (titleMatch ? 'title' : showMatch ? 'show' : personMatch ? 'person' : 'missing'))
  const providers = $derived(reference.providers.filter((p) => p.countries.includes(scope.country)))

  $effect(() => {
    const NAMES = { charts: 'Top 10', title: 'Title', upnext: 'Up next', show: 'Episodes', person: 'Person', discover: 'Discover', watchlist: 'Watchlist', library: 'Library', diary: 'Diary', stats: 'Stats', settings: 'Settings', missing: 'Not found' }
    document.title = NAMES[page] ? `${NAMES[page]} · metareel` : 'metareel'
  })
  // On a phone the nav scrolls sideways; keep the current page's link in view.
  let nav = $state(null)
  $effect(() => {
    page
    nav?.querySelector('[aria-current="page"]')?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
  })
</script>

<svelte:window onkeydown={onWindowKey} />

{#if !session.ready}
  <div class="boot" aria-busy="true" aria-label="Loading"></div>
{:else if !session.user}
  <Login />
{:else}
  <header class="bar">
    <a class="brand" href="#/">metareel</a>
    <nav class="primary" aria-label="Primary" bind:this={nav}>
      <a href="#/" aria-current={page === 'home' ? 'page' : undefined}>Home</a>
      <a href="#/charts" aria-current={page === 'charts' ? 'page' : undefined}>Charts</a>
      <a href="#/discover" aria-current={page === 'discover' ? 'page' : undefined}>Discover</a>
      <span class="sep" aria-hidden="true"></span>
      <a href="#/up-next" aria-current={page === 'upnext' || page === 'show' ? 'page' : undefined}>Up next</a>
      <a href="#/watchlist" aria-current={page === 'watchlist' ? 'page' : undefined}>Watchlist</a>
      <a href="#/library" aria-current={page === 'library' ? 'page' : undefined}>Library</a>
      <a href="#/diary" aria-current={page === 'diary' ? 'page' : undefined}>Diary</a>
      <a href="#/stats" aria-current={page === 'stats' ? 'page' : undefined}>Stats</a>
    </nav>
    <div class="tools">
      <button class="search" type="button" onclick={() => (searchOpen = true)} aria-keyshortcuts={mac ? 'Meta+K' : 'Control+K'}>
        <Icon name="search" size={15} /><span class="label">Search</span><kbd>{mac ? '⌘' : 'Ctrl'} K</kbd>
      </button>
      <label class="pick">
        <span class="sr">Country</span>
        <select value={scope.country} onchange={(e) => setScope({ country: e.currentTarget.value })} disabled={!reference.loaded}>
          {#each reference.countries as c}<option value={c.code}>{c.name}</option>{/each}
          {#if !reference.loaded}<option value={scope.country}>{scope.country}</option>{/if}
        </select>
      </label>
      <label class="pick">
        <span class="sr">Service</span>
        <select value={scope.provider} onchange={(e) => setScope({ provider: e.currentTarget.value })} disabled={!reference.loaded}>
          {#each providers as p}<option value={p.slug}>{providerName(p.slug)}</option>{/each}
          {#if !reference.loaded}<option value={scope.provider}>{providerName(scope.provider)}</option>{/if}
        </select>
      </label>
      <button class="icon-btn" type="button" onclick={toggleTheme} aria-label="Switch to {theme.effective === 'light' ? 'dark' : 'light'} mode" title="Switch to {theme.effective === 'light' ? 'dark' : 'light'} mode">
        {theme.effective === 'light' ? '☾' : '☀'}
      </button>
      <details class="account">
        <summary aria-label="Account: {session.user.username}"><span class="avatar" aria-hidden="true">{session.user.username.slice(0, 1).toUpperCase()}</span></summary>
        <div class="menu">
          <p class="who">Signed in as <b>{session.user.username}</b></p>
          <a href="#/settings" onclick={(e) => e.currentTarget.closest('details')?.removeAttribute('open')}>Settings</a>
          {#if session.user.role === 'admin'}<a href="/admin/">Admin console</a>{/if}
          <button type="button" onclick={signOut}>Sign out</button>
        </div>
      </details>
    </div>
  </header>

  {#if bootError}<p class="alert" role="alert">Some of your data didn’t load: {bootError}</p>{/if}

  <main>
    {#if page === 'home'}
      <Home {onlog} />
    {:else if page === 'charts'}
      <Charts {onlog} />
    {:else if page === 'watchlist'}
      <Watchlist {onlog} />
    {:else if page === 'library'}
      <Library {onlog} />
    {:else if page === 'discover'}
      <Discover {onlog} />
    {:else if page === 'settings'}
      <Settings {onconfirm} />
    {:else if page === 'upnext'}
      <UpNext {onlog} />
    {:else if page === 'show'}
      {#key route.path}<Show id={showMatch[1]} {onlog} {onconfirm} />{/key}
    {:else if page === 'person'}
      {#key route.path}<Person id={personMatch[1]} />{/key}
    {:else if page === 'stats'}
      <Stats />
    {:else if page === 'diary'}
      <Diary {onlog} {onconfirm} />
    {:else if page === 'title'}
      {#key route.path}<Title kind={titleMatch[1]} id={titleMatch[2]} {onlog} />{/key}
    {:else}
      <section class="missing">
        <h1>Page not found</h1>
        <p>There’s nothing at this address. <a href="#/">Go to Home</a>.</p>
      </section>
    {/if}
  </main>

  <LogWatch bind:target={logTarget} />
  <ConfirmDialog bind:request={confirmRequest} />
  <SearchPalette bind:open={searchOpen} {onlog} />
{/if}

<div class="toasts" role="status" aria-live="polite">
  {#each toasts.items as t (t.id)}<p class="toast" class:error={t.kind === 'error'}>{t.text}</p>{/each}
</div>

<style>
  .boot { min-height: 100vh; }
  .bar { position: sticky; top: 0; z-index: 10; display: flex; align-items: center; gap: 26px; height: 64px; padding: 0 var(--gutter); background: color-mix(in srgb, var(--bg-canvas) 92%, transparent); backdrop-filter: saturate(140%) blur(10px); }
  .brand { font: 700 23px/1 var(--font-display); letter-spacing: -0.02em; text-decoration: none; }
  .primary { display: flex; align-items: center; gap: 20px; font-size: 15px; font-weight: 600; }
  .primary .sep { flex: none; width: 1px; height: 16px; background: var(--border-subtle); }
  .primary a { color: var(--text-muted); text-decoration: none; white-space: nowrap; }
  .primary a:hover, .primary a[aria-current='page'] { color: var(--text-primary); }
  .tools { margin-left: auto; display: flex; align-items: center; gap: 8px; }
  .pick select {
    height: 32px; padding: 0 28px 0 12px; border: 0; border-radius: 16px; appearance: none; cursor: pointer;
    background: var(--bg-elevated) url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6'%3E%3Cpath d='M1 1l4 4 4-4' fill='none' stroke='%23a1a1a8' stroke-width='1.5'/%3E%3C/svg%3E") no-repeat right 11px center;
    color: var(--text-primary); font: 600 13px/1 var(--font-sans);
  }
  .search { display: flex; align-items: center; gap: 8px; height: 32px; padding: 0 8px 0 12px; border: 0; border-radius: 16px; background: var(--bg-elevated); color: var(--text-muted); font: 500 13px/1 var(--font-sans); cursor: pointer; }
  .search:hover { color: var(--text-primary); }
  .search .label { min-width: 90px; text-align: left; }
  .search kbd { white-space: nowrap; padding: 3px 6px; border-radius: 5px; background: var(--bg-surface); color: var(--text-muted); font: 600 11px/1 var(--font-sans); }
  .icon-btn { display: grid; place-items: center; width: 32px; height: 32px; border: 0; border-radius: 50%; background: var(--bg-elevated); color: var(--text-primary); cursor: pointer; font-size: 15px; }
  .account { position: relative; }
  .account summary { list-style: none; cursor: pointer; border-radius: 50%; }
  .account summary::-webkit-details-marker { display: none; }
  .avatar { display: grid; place-items: center; width: 32px; height: 32px; border-radius: 50%; background: var(--bg-elevated); font-weight: 700; font-size: 13px; }
  .menu { position: absolute; right: 0; top: 40px; display: grid; gap: 4px; min-width: 210px; padding: 10px; border-radius: 14px; background: var(--bg-surface); box-shadow: 0 18px 50px -12px rgb(0 0 0 / 0.55); }
  .who { margin: 4px 8px 6px; color: var(--text-muted); font-size: 13px; }
  .menu a, .menu button { display: block; padding: 8px; border: 0; border-radius: 8px; background: none; color: var(--text-primary); text-align: left; text-decoration: none; cursor: pointer; font-size: 14px; }
  .menu a:hover, .menu button:hover { background: var(--bg-elevated); }

  .missing { padding: 60px var(--gutter); }
  .missing h1 { margin: 0 0 8px; font: 700 32px/1.1 var(--font-display); }

  .toasts { position: fixed; left: 50%; bottom: 24px; z-index: 50; display: grid; gap: 8px; transform: translateX(-50%); }
  .toast { margin: 0; padding: 10px 16px; border-radius: 12px; background: var(--accent-primary); color: var(--accent-on); font-weight: 600; box-shadow: 0 12px 30px -10px rgb(0 0 0 / 0.5); }
  .toast.error { background: var(--delta-down); color: var(--bg-canvas); }

  @media (max-width: 1180px) and (min-width: 1024px) {
    .bar { gap: 16px; }
    .primary { gap: 14px; }
    .search .label { display: none; }
  }
  @media (max-width: 1023px) {
    .bar { gap: 14px; height: auto; min-height: 56px; flex-wrap: wrap; padding-block: 8px; }
    .tools { margin-left: 0; width: 100%; }
    .search .label, .search kbd { display: none; }
    .primary { max-width: 100%; gap: 14px; overflow-x: auto; white-space: nowrap; scrollbar-width: none; padding-right: 20px; mask-image: linear-gradient(to right, #000 calc(100% - 28px), transparent); }
    .search { width: 32px; padding: 0; justify-content: center; }
  }
</style>
