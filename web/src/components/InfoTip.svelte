<script>
  // A small (i) that explains a metric or term on hover, focus or tap.
  import Icon from './Icon.svelte'

  let { text, label = 'What is this?', size = 13 } = $props()
  let open = $state(false)
  const id = `tip-${Math.random().toString(36).slice(2, 9)}`

  function onkeydown(e) {
    if (e.key === 'Escape') open = false
  }
</script>

<span class="info">
  <button
    type="button"
    class="info-btn"
    aria-label={label}
    aria-describedby={open ? id : undefined}
    onmouseenter={() => (open = true)}
    onmouseleave={() => (open = false)}
    onfocus={() => (open = true)}
    onblur={() => (open = false)}
    onclick={(e) => { e.preventDefault(); e.stopPropagation(); open = !open }}
    {onkeydown}
  ><Icon name="info" {size} /></button>
  {#if open}<span class="info-pop" role="tooltip" {id}>{text}</span>{/if}
</span>
