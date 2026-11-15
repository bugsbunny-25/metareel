<!-- A row of posters that scrolls sideways: each links somewhere, with an optional caption and
     a progress bar. items: [{ key, href, poster, seed, title, caption?, percent? }] -->
<script>
  import Poster from './Poster.svelte'

  let { items = [], width = 132, label = '' } = $props()
</script>

<ul class="shelf" aria-label={label}>
  {#each items as it (it.key)}
    <li style:width="{width}px">
      <a href={it.href} title={it.title}>
        <Poster path={it.poster} seed={it.seed} title={it.title} {width} radius={10} size="w342" />
        {#if it.percent != null}<span class="bar" aria-hidden="true"><i style:width="{Math.min(100, it.percent)}%"></i></span>{/if}
        <span class="name">{it.title}</span>
        {#if it.caption}<span class="cap">{it.caption}</span>{/if}
      </a>
    </li>
  {/each}
</ul>

<style>
  .shelf { display: flex; gap: 14px; margin: 0; padding: 2px 0 8px; list-style: none; overflow-x: auto; scrollbar-width: thin; }
  li { flex: none; }
  a { display: grid; gap: 5px; color: inherit; text-decoration: none; }
  a:hover .name { text-decoration: underline; text-underline-offset: 3px; }
  .bar { height: 4px; border-radius: 2px; background: var(--bg-elevated); overflow: hidden; }
  .bar i { display: block; height: 100%; background: var(--accent-yours); }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 600 14px/1.25 var(--font-display); }
  .cap { color: var(--accent-yours); font-size: 13px; letter-spacing: 0.04em; }
</style>
