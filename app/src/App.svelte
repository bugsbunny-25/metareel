<!-- App shell: top bar (brand, primary nav, search, theme, account), the routed page,
     the "Log a watch" dialog and toasts. Signed out, it shows the sign-in screen instead. -->
<script>
  import { untrack } from 'svelte'
  import { route, navigate } from './lib/router.svelte.js'
  import { session, loadSession, signOut } from './lib/session.svelte.js'
  import { loadReference } from './lib/scope.svelte.js'
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
  import LazyPage from './components/LazyPage.svelte'
  import { prefetchPages } from './lib/pages.svelte.js'

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
      prefetchPages()
      // Preferences after the reference lists, so a saved country/service pair is checked against them.
      Promise.all([loadReference().then(loadPrefs), library.load()]).catch((err) => (bootError = err.message))
    })
  })

  const titleMatch = $derived(route.path.match(/^\/title\/(movie|tv)\/(\d+)$/))
  const showMatch = $derived(route.path.match(/^\/show\/(\d+)$/))
  const personMatch = $derived(route.path.match(/^\/person\/(\d+)$/))
  const episodeMatch = $derived(route.path.match(/^\/episode\/(\d+)\/(\d+)\/(\d+)$/))
  const watchMatch = $derived(route.path.match(/^\/watch\/(\d+)$/))
  const discMatch = $derived(route.path.match(/^\/collection\/(\d+)$/))
  const PAGES = { '/': 'home', '/charts': 'charts', '/discover': 'discover', '/tv': 'tv', '/movies': 'movies', '/watchlist': 'watchlist', '/library': 'library', '/diary': 'diary', '/you': 'you', '/you/stats': 'statsDetailed', '/stats': 'stats', '/settings': 'settings', '/import': 'import', '/collection': 'collection' }
  const page = $derived(PAGES[route.path] ?? (titleMatch ? 'title' : showMatch ? 'show' : personMatch ? 'person' : episodeMatch ? 'episode' : watchMatch ? 'watch' : discMatch ? 'disc' : 'missing'))
  // Up next became TV and Movies (as on mobile); old links still land on the right one.
  $effect(() => {
    if (route.path === '/up-next') navigate(route.query.tab === 'movies' ? '/movies' : '/tv')
  })
  // Nav groups: the pages each tab owns.
  const TAB_OF = { show: 'tv', episode: 'tv', watch: 'diary', statsDetailed: 'you', stats: 'you', settings: 'you', library: 'you', watchlist: 'movies', disc: 'collection' }
  const tab = $derived(TAB_OF[page] ?? page)

  $effect(() => {
    const NAMES = { charts: 'Top 10', title: 'Title', tv: 'TV', movies: 'Movies', show: 'Episodes', episode: 'Episode', watch: 'Watch', person: 'Person', discover: 'Discover', watchlist: 'Watchlist', library: 'Library', diary: 'Diary', you: 'You', statsDetailed: 'Detailed stats', stats: 'Year in review', settings: 'Settings', import: 'Import', collection: 'Collection', disc: 'Disc', missing: 'Not found' }
    document.title = NAMES[page] ? `${NAMES[page]} · metareel` : 'metareel'
  })
  // A new page starts at the top (the hash router would otherwise keep the last page's scroll).
  // Only the path counts: filters and tabs in the query keep your place.
  let lastPath = route.path
  $effect(() => {
    const p = route.path
    if (p !== lastPath) {
      lastPath = p
      window.scrollTo(0, 0)
    }
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
      <a href="#/tv" aria-current={tab === 'tv' ? 'page' : undefined}>TV</a>
      <a href="#/movies" aria-current={tab === 'movies' ? 'page' : undefined}>Movies</a>
      <a href="#/diary" aria-current={tab === 'diary' ? 'page' : undefined}>Diary</a>
      <a href="#/collection" aria-current={tab === 'collection' ? 'page' : undefined}>Collection</a>
      <a href="#/you" aria-current={tab === 'you' ? 'page' : undefined}>You</a>
    </nav>
    <div class="tools">
      <button class="search" type="button" onclick={() => (searchOpen = true)} aria-keyshortcuts={mac ? 'Meta+K' : 'Control+K'}>
        <Icon name="search" size={15} /><span class="label">Search</span><kbd>{mac ? '⌘' : 'Ctrl'} K</kbd>
      </button>
      <button class="icon-btn" type="button" onclick={toggleTheme} aria-label="Switch to {theme.effective === 'light' ? 'dark' : 'light'} mode" title="Switch to {theme.effective === 'light' ? 'dark' : 'light'} mode">
        {theme.effective === 'light' ? '☾' : '☀'}
      </button>
      <details class="account">
        <summary aria-label="Account: {session.user.username}"><span class="avatar" aria-hidden="true">{session.user.username.slice(0, 1).toUpperCase()}</span></summary>
        <div class="menu">
          <p class="who">Signed in as <b>{session.user.username}</b></p>
          <a href="#/you" onclick={(e) => e.currentTarget.closest('details')?.removeAttribute('open')}>You</a>
          <a href="#/settings" onclick={(e) => e.currentTarget.closest('details')?.removeAttribute('open')}>Settings</a>
          <a href="#/import" onclick={(e) => e.currentTarget.closest('details')?.removeAttribute('open')}>Import history</a>
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
      <LazyPage name="charts" {onlog} />
    {:else if page === 'watchlist'}
      <LazyPage name="watchlist" {onlog} />
    {:else if page === 'library'}
      <LazyPage name="library" {onlog} />
    {:else if page === 'discover'}
      <LazyPage name="discover" {onlog} />
    {:else if page === 'settings'}
      <LazyPage name="settings" {onconfirm} />
    {:else if page === 'import'}
      <LazyPage name="import" {onconfirm} />
    {:else if page === 'collection'}
      <LazyPage name="collection" />
    {:else if page === 'disc'}
      {#key route.path}<LazyPage name="disc" id={discMatch[1]} {onconfirm} />{/key}
    {:else if page === 'tv'}
      <LazyPage name="tv" {onlog} />
    {:else if page === 'movies'}
      <LazyPage name="movies" {onlog} />
    {:else if page === 'you'}
      <LazyPage name="you" />
    {:else if page === 'statsDetailed'}
      <LazyPage name="statsDetailed" />
    {:else if page === 'episode'}
      {#key route.path}<LazyPage name="episode" show={episodeMatch[1]} season={+episodeMatch[2]} episode={+episodeMatch[3]} {onlog} />{/key}
    {:else if page === 'watch'}
      {#key route.path}<LazyPage name="watch" id={watchMatch[1]} {onlog} {onconfirm} />{/key}
    {:else if page === 'show'}
      {#key route.path}<LazyPage name="show" id={showMatch[1]} {onlog} {onconfirm} />{/key}
    {:else if page === 'person'}
      {#key route.path}<LazyPage name="person" id={personMatch[1]} />{/key}
    {:else if page === 'stats'}
      <LazyPage name="stats" />
    {:else if page === 'diary'}
      <LazyPage name="diary" {onlog} {onconfirm} />
    {:else if page === 'title'}
      {#key route.path}<LazyPage name="title" kind={titleMatch[1]} id={titleMatch[2]} {onlog} />{/key}
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
    /* Brand and tools share the first row; the nav gets a full row of its own. */
    .bar { gap: 8px 14px; height: auto; min-height: 56px; flex-wrap: wrap; padding-block: 10px 6px; }
    .tools { margin-left: auto; order: 2; }
    .primary { order: 3; flex-basis: 100%; }
    .search .label, .search kbd { display: none; }
    .primary { max-width: 100%; gap: 14px; overflow-x: auto; white-space: nowrap; scrollbar-width: none; padding-right: 20px; mask-image: linear-gradient(to right, #000 calc(100% - 28px), transparent); }
    .search { width: 32px; padding: 0; justify-content: center; }
  }
  @media (max-width: 440px) {
    .primary { gap: 11px; font-size: 14px; padding-right: 0; mask-image: none; }
    .primary .sep { display: none; }
  }
</style>
