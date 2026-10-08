<script>
  import { route, href } from './lib/router.svelte.js'
  import { getStats } from './lib/api.js'
  import Icon from './components/Icon.svelte'
  import Toasts from './components/Toasts.svelte'
  import ConfirmDialog from './components/ConfirmDialog.svelte'
  import Dashboard from './pages/Dashboard.svelte'
  import Top10 from './pages/Top10.svelte'
  import Titles from './pages/Titles.svelte'
  import Releases from './pages/Releases.svelte'
  import Schedules from './pages/Schedules.svelte'
  import Runs from './pages/Runs.svelte'
  import Settings from './pages/Settings.svelte'

  const NAV = [
    { section: 'Data' },
    { path: '/', label: 'Overview', icon: 'dashboard', page: Dashboard },
    { path: '/top10', label: 'Top 10 charts', icon: 'chart', page: Top10 },
    { path: '/titles', label: 'Titles & mapping', icon: 'film', page: Titles, badge: 'missing' },
    { path: '/releases', label: 'New & upcoming', icon: 'calendar', page: Releases },
    { section: 'Jobs' },
    { path: '/schedules', label: 'Schedules', icon: 'clock', page: Schedules },
    { path: '/runs', label: 'Job runs', icon: 'list', page: Runs, badge: 'running' },
    { section: 'System' },
    { path: '/settings', label: 'Settings & API keys', icon: 'settings', page: Settings },
  ]

  const current = $derived(NAV.find((n) => n.path === route.path) ?? NAV[1])
  const Page = $derived(current.page)

  // Sidebar badges, refreshed every minute.
  let stats = $state(null)
  async function loadStats() {
    try {
      stats = await getStats()
    } catch {
      // badges are optional
    }
  }
  $effect(() => {
    loadStats()
    const t = setInterval(loadStats, 60000)
    return () => clearInterval(t)
  })

  function badge(kind) {
    if (!stats) return null
    if (kind === 'running') return stats.runs.running || null
    if (kind === 'missing') return stats.titles.missing_tmdb || null
    return null
  }

  // Theme: follows the OS unless toggled; the choice is remembered.
  let theme = $state(readTheme())
  function readTheme() {
    try { return localStorage.getItem('metareel.theme') } catch { return null }
  }
  $effect(() => {
    if (theme) document.documentElement.dataset.theme = theme
    else delete document.documentElement.dataset.theme
  })
  function toggleTheme() {
    const dark = theme ? theme === 'dark' : !window.matchMedia('(prefers-color-scheme: light)').matches
    theme = dark ? 'light' : 'dark'
    try { localStorage.setItem('metareel.theme', theme) } catch { /* private mode */ }
  }

  $effect(() => {
    document.title = `${current.label} · metareel admin`
  })
</script>

<div class="shell">
  <aside class="sidebar">
    <div class="brand">
      <div class="brand-mark">m</div>
      <div>
        <div class="brand-name">metareel</div>
        <div class="brand-sub">Admin console</div>
      </div>
    </div>
    <nav class="nav" aria-label="Main">
      {#each NAV as item, i (item.path ?? `s${i}`)}
        {#if item.section}
          <div class="nav-section">{item.section}</div>
        {:else}
          <a class="nav-link" class:active={current === item} href={href(item.path)} title={item.label}>
            <Icon name={item.icon} />
            <span>{item.label}</span>
            {#if item.badge && badge(item.badge)}
              <span class="count badge {item.badge === 'running' ? 'badge-accent' : 'badge-warning'}">{badge(item.badge)}</span>
            {/if}
          </a>
        {/if}
      {/each}
    </nav>
    <div class="sidebar-foot">
      <a class="nav-link" href="/docs" target="_blank" rel="noopener" title="Public API docs"><Icon name="book" /><span>Public API docs</span></a>
      <a class="nav-link" href="/docs/admin" target="_blank" rel="noopener" title="Admin API docs"><Icon name="book" /><span>Admin API docs</span></a>
      <button class="nav-link" style="border:0;background:none;cursor:pointer;width:100%" onclick={toggleTheme} title="Toggle theme">
        <Icon name="moon" /><span>Toggle theme</span>
      </button>
    </div>
  </aside>

  <main class="main">
    {#key route.path}
      <Page />
    {/key}
  </main>
</div>

<Toasts />
<ConfirmDialog />
