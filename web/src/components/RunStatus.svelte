<script>
  let { status } = $props()

  const cls = $derived(
    status === 'succeeded' ? 'badge-success' : status === 'failed' ? 'badge-danger' : status === 'partial' ? 'badge-warning' : status === 'started' ? 'badge-accent pulse' : ''
  )
  const label = $derived(status === 'started' ? 'running' : status ?? 'never run')
</script>

<span class="badge {cls}"><span class="dot"></span>{label}</span>
