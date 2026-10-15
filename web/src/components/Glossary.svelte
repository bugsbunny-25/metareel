<script>
  import Icon from './Icon.svelte'
  import { GLOSSARY } from '../lib/glossary.js'

  let { onclose } = $props()
  let filter = $state('')
  let input = $state()
  $effect(() => { input?.focus() })

  const groups = $derived.by(() => {
    const f = filter.trim().toLowerCase()
    if (!f) return GLOSSARY
    return GLOSSARY.map((g) => ({ ...g, terms: g.terms.filter(([k, v]) => k.toLowerCase().includes(f) || v.toLowerCase().includes(f)) })).filter((g) => g.terms.length)
  })

  function onkeydown(e) {
    if (e.key === 'Escape') onclose()
  }
</script>

<svelte:window {onkeydown} />

<div class="overlay" onclick={onclose} role="presentation"></div>
<div class="drawer drawer-sm" role="dialog" aria-modal="true" aria-labelledby="glossary-title" tabindex="-1">
  <div class="drawer-header">
    <div class="grow">
      <div class="drawer-title" id="glossary-title">Glossary</div>
      <div class="muted small">What the terms and numbers in this console mean.</div>
    </div>
    <button class="btn btn-ghost btn-icon" onclick={onclose} title="Close (Esc)" aria-label="Close glossary"><Icon name="x" /></button>
  </div>
  <div class="drawer-body">
    <div class="search" style="margin-bottom:18px">
      <span class="icon"><Icon name="search" size={14} /></span>
      <input bind:this={input} class="input" style="width:100%" type="search" placeholder="Find a term" bind:value={filter} aria-label="Find a term" />
    </div>
    {#each groups as g (g.group)}
      <div class="glossary-group">{g.group}</div>
      <dl class="glossary">
        {#each g.terms as [term, def] (term)}<dt>{term}</dt><dd>{def}</dd>{/each}
      </dl>
    {:else}
      <div class="empty">No term matches “{filter}”.</div>
    {/each}
  </div>
</div>
