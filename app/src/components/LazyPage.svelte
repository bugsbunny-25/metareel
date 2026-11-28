<!-- A page loaded on demand (lib/pages.svelte.js); the rest of the props go to the page. -->
<script>
  import { pageComponent, pageError, retryPage } from '../lib/pages.svelte.js'

  let { name, ...props } = $props()
  const Page = $derived(pageComponent(name))
  const error = $derived(pageError(name))
</script>

{#if Page}
  <Page {...props} />
{:else if error}
  <section class="failed" role="alert">
    <h1>This page didn’t load</h1>
    <p>The connection may have dropped, or metareel was updated since you opened it.</p>
    <div class="actions">
      <button class="btn-primary" type="button" onclick={() => retryPage(name)}>Try again</button>
      <button class="btn-secondary" type="button" onclick={() => location.reload()}>Reload metareel</button>
    </div>
  </section>
{/if}

<style>
  .failed { padding: 60px var(--gutter); }
  .failed h1 { margin: 0 0 8px; font: 700 32px/1.1 var(--font-display); }
  .failed p { margin: 0 0 20px; color: var(--text-muted); }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; }
</style>
