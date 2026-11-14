<!-- Premium formats for a cinema watch, grouped by axis (a screening can be IMAX *and* Dolby
     Atmos *and* 3D, so these are independent toggles, not one choice). The list comes from
     /me/formats once per page load. -->
<script>
  import { loadFormats } from '../lib/formats.js'

  /** selected: format slugs. */
  let { selected = $bindable([]) } = $props()

  const GROUPS = [['brand', 'Screen'], ['projection', 'Projection'], ['sound', 'Sound'], ['motion', 'Motion'], ['dimension', '3D']]

  let formats = $state(null)
  loadFormats().then((list) => (formats = list)).catch(() => (formats = []))
  const groups = $derived(GROUPS.map(([k, name]) => [name, (formats ?? []).filter((f) => f.category === k)]).filter(([, l]) => l.length))
  const toggle = (slug) => (selected = selected.includes(slug) ? selected.filter((s) => s !== slug) : [...selected, slug])
</script>


{#if formats === null}
  <p class="hint">Loading formats…</p>
{:else}
  <div class="groups">
    {#each groups as [name, list]}
      <div class="g" role="group" aria-label={name}>
        <span class="gl">{name}</span>
        <div class="chips">
          {#each list as f (f.slug)}
            <button type="button" class="chip" aria-pressed={selected.includes(f.slug)} title={f.note ?? f.label} onclick={() => toggle(f.slug)}>{#if f.logo_url}<span class="plate" aria-hidden="true"><img class:mono={f.logo_display === 'mono'} src={f.logo_url} alt="" /></span>{/if}{f.label}</button>
          {/each}
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  .groups { display: grid; gap: 8px; }
  .g { display: grid; grid-template-columns: 82px 1fr; gap: 10px; align-items: start; }
  .gl { padding-top: 6px; color: var(--text-muted); font-size: 12.5px; font-weight: 600; }
  .chips { display: flex; flex-wrap: wrap; gap: 6px; }
  .chip { display: inline-flex; align-items: center; gap: 6px; height: 28px; padding: 0 11px; border: 1px solid var(--border-subtle); border-radius: 14px; background: none; color: var(--text-primary); font: 500 13px/1 var(--font-sans); cursor: pointer; }
  .chip:hover { border-color: var(--border-strong); }
  .chip[aria-pressed='true'] { border-color: var(--accent-yours); background: color-mix(in srgb, var(--accent-yours) 16%, transparent); font-weight: 700; }
  .chip:has(.plate) { padding-left: 4px; }
  .plate { display: inline-flex; align-items: center; height: 20px; padding: 0 5px; border-radius: 10px; background: #161618; }
  .plate img { display: block; height: 12px; max-width: 64px; object-fit: contain; }
  .plate img.mono { filter: brightness(0) invert(1); }
  .hint { margin: 0; color: var(--text-muted); font-size: 13px; }
  @media (max-width: 480px) { .g { grid-template-columns: 1fr; gap: 4px; } .gl { padding-top: 0; } }
</style>
