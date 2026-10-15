<script>
  // Page title, a one-line explanation for first-time readers and actions.
  import InfoTip from './InfoTip.svelte'

  let { title, desc = '', info = '', actions } = $props()
</script>

<div class="page-header">
  <div>
    <h1 class="page-title">{title}{#if info}<InfoTip text={info} label="About this page" size={15} />{/if}</h1>
    {#if desc}<p class="page-desc">{desc}</p>{/if}
  </div>
  {#if actions}<div class="page-actions">{@render actions()}</div>{/if}
</div>
