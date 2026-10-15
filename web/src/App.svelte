<script>
  import { route, href, setQuery } from './lib/router.svelte.js'
  import { getStats } from './lib/api.js'
  import Icon from './components/Icon.svelte'
  import Toasts from './components/Toasts.svelte'
  import ConfirmDialog from './components/ConfirmDialog.svelte'
  import Glossary from './components/Glossary.svelte'
  import Dashboard from './pages/Dashboard.svelte'
  import Top10 from './pages/Top10.svelte'
  import Releases from './pages/Releases.svelte'
  import Schedules from './pages/Schedules.svelte'
  import Runs from './pages/Runs.svelte'
  import Settings from './pages/Settings.svelte'
  import Insights from './pages/Insights.svelte'
  import DataHealth from './pages/DataHealth.svelte'
  import MappingHub from './pages/MappingHub.svelte'

  // paths: every route the item covers (the Mapping hub keeps the old
  // /titles, /netflix and /mapping URLs as its tabs).
  const NAV = [
    { path: '/', label: 'Overview', icon: 'dashboard', page: Dashboard },
    { section: 'Charts' },
    { path: '/top10', label: 'Top 10 charts', icon: 'chart', page: Top10 },
    { path: '/insights', label: 'Insights', icon: 'sparkles', page: Insights },
    { path: '/releases', label: 'New & upcoming', icon: 'calendar', page: Releases },
    { section: 'Data' },
    { path: '/titles', paths: ['/titles', '/netflix', '/mapping'], label: 'Mapping', icon: 'link', page: MappingHub, badge: 'missing' },
    { path: '/data-health', label: 'Data health', icon: 'activity', page: DataHealth },
    { section: 'Operations' },
    { path: '/schedules', label: 'Schedules', icon: 'clock', page: Schedules },
    { path: '/runs', label: 'Job runs', icon: 'list', page: Runs, badge: 'running' },
    { section: 'System' },
    { path: '/settings', label: 'Settings & API keys', icon: 'settings', page: Settings },
  ]

  const covers = (item, path) => (item.paths ?? [item.path]).includes(path)
  const current = $derived(NAV.find((n) => n.page && covers(n, route.path)) ?? NAV[0])
  const section = $derived.by(() => {
    let s = ''
    for (const n of NAV) {
      if (n.section) s = n.section
      else if (n === current) return n.path === '/' ? '' : s
    }
    return ''
  })
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
  const badgeTitle = (kind) => (kind === 'running' ? 'Jobs running now' : 'FlixPatrol titles without a TMDB ID')

  // Theme: follows the OS unless toggled; the choice is remembered.
  let theme = $state(readTheme())
  function readTheme() {
    try { return localStorage.getItem('metareel.theme') } catch { return null }
  }
  $effect(() => {
    if (theme) document.documentElement.dataset.theme = theme
    else delete document.documentElement.dataset.theme
  })
  const isDark = () => (theme ? theme === 'dark' : window.matchMedia('(prefers-color-scheme: dark)').matches)
  let dark = $state(isDark())
  function toggleTheme() {
    theme = isDark() ? 'light' : 'dark'
    dark = theme === 'dark'
    try { localStorage.setItem('metareel.theme', theme) } catch { /* private mode */ }
  }

  const glossaryOpen = $derived(route.query.glossary === '1')

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
        <div class="brand-sub">Streaming charts admin</div>
      </div>
    </div>
    <nav class="nav" aria-label="Main">
      {#each NAV as item, i (item.path ?? `s${i}`)}
        {#if item.section}
          <div class="nav-section">{item.section}</div>
        {:else}
          <a class="nav-link" class:active={current === item} href={href(item.path)} title={item.label} aria-current={current === item ? 'page' : undefined}>
            <Icon name={item.icon} />
            <span>{item.label}</span>
            {#if item.badge && badge(item.badge)}
              <span class="count badge {item.badge === 'running' ? 'badge-accent' : 'badge-warning'}" title={badgeTitle(item.badge)}>{badge(item.badge)}</span>
            {/if}
          </a>
        {/if}
      {/each}
    </nav>
    <div class="sidebar-foot">
      <a class="nav-link" href="/docs" target="_blank" rel="noopener" title="Public API docs"><Icon name="book" /><span>Public API docs</span></a>
      <a class="nav-link" href="/docs/admin" target="_blank" rel="noopener" title="Admin API docs"><Icon name="book" /><span>Admin API docs</span></a>
    </div>
  </aside>

  <main class="main">
    <header class="topbar">
      <div class="crumbs">
        {#if section}<span>{section}</span><Icon name="chevronRight" size={12} />{/if}
        <strong>{current.label}</strong>
      </div>
      <span class="grow"></span>
      {#if stats}
        <a class="badge {stats.runs.running ? 'badge-accent pulse' : ''}" href={href('/runs', { status: 'started' })} title="Jobs running now" style="text-decoration:none">
          <span class="dot"></span>{stats.runs.running ? `${stats.runs.running} running` : 'Idle'}
        </a>
      {/if}
      <button class="btn btn-ghost btn-sm" onclick={() => setQuery({ glossary: '1' })} title="What the terms on these pages mean"><Icon name="help" size={14} />Glossary</button>
      <button class="btn btn-ghost btn-icon btn-sm" onclick={toggleTheme} aria-label={dark ? 'Switch to light theme' : 'Switch to dark theme'} title="Toggle theme">
        <Icon name={dark ? 'sun' : 'moon'} size={14} />
      </button>
    </header>
    {#key route.path}
      <Page />
    {/key}
  </main>
</div>

{#if glossaryOpen}<Glossary onclose={() => setQuery({ glossary: '' })} />{/if}
<Toasts />
<ConfirmDialog />
