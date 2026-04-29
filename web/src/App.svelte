<script>
  import TitlesTab from './components/TitlesTab.svelte'
  import Top10Tab from './components/Top10Tab.svelte'
  import SchedulesTab from './components/SchedulesTab.svelte'

  let activeTab = $state('titles')
</script>

<header class="app-header">
  <span class="app-logo">metareel</span>
  <nav class="app-tabs">
    <button
      class="tab-btn"
      class:active={activeTab === 'titles'}
      onclick={() => (activeTab = 'titles')}
    >Titles</button>
    <button
      class="tab-btn"
      class:active={activeTab === 'top10'}
      onclick={() => (activeTab = 'top10')}
    >Top 10</button>
    <button
      class="tab-btn"
      class:active={activeTab === 'schedules'}
      onclick={() => (activeTab = 'schedules')}
    >Schedules</button>
  </nav>
</header>

<main class="tab-content">
  {#if activeTab === 'titles'}
    <TitlesTab />
  {:else if activeTab === 'top10'}
    <Top10Tab />
  {:else}
    <SchedulesTab />
  {/if}
</main>
