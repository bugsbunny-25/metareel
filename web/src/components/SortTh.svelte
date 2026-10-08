<script>
  import Icon from './Icon.svelte'

  // sort is "field" (ascending) or "-field" (descending). Clicking a column
  // sorts by it using firstDir, clicking again flips the direction.
  let { label, field, sort, onsort, firstDir = 'asc', align = 'left', title = '' } = $props()

  const active = $derived(sort === field || sort === `-${field}`)
  const desc = $derived(sort === `-${field}`)

  function click() {
    if (!active) onsort(firstDir === 'desc' ? `-${field}` : field)
    else onsort(desc ? field : `-${field}`)
  }
</script>

<th class:right={align === 'right'} aria-sort={active ? (desc ? 'descending' : 'ascending') : 'none'} {title}>
  <button class="th-sort" class:active onclick={click}>
    {label}
    {#if active}
      <Icon name={desc ? 'arrowDown' : 'arrowUp'} size={12} />
    {:else}
      <span class="muted" style="opacity:.5"><Icon name="sort" size={12} /></span>
    {/if}
  </button>
</th>
