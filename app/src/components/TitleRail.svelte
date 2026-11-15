<!-- A row of posters for a list of titles (trending, in cinemas): artwork with the Top 10
     badge where it applies, the name, and year and TMDB score. Scrolls sideways; wheel and
     arrow buttons page through it on wide screens.
     items: [{ tmdb_kind, tmdb_id, title, poster_path, release_date, vote_average, vote_count }] -->
<script>
  import Poster from './Poster.svelte'
  import RatingLogo from './RatingLogo.svelte'
  import Icon from './Icon.svelte'

  let { items = [], label = '', width = 140 } = $props()
  let rail = $state(null)
  let atStart = $state(true)
  let atEnd = $state(false)
  const onscroll = () => {
    if (!rail) return
    atStart = rail.scrollLeft < 4
    atEnd = rail.scrollLeft + rail.clientWidth >= rail.scrollWidth - 4
  }
  $effect(() => { items; queueMicrotask(onscroll) })
  const page = (dir) => rail?.scrollBy({ left: dir * rail.clientWidth * 0.85, behavior: 'smooth' })
</script>

<div class="wrap">
  <ul class="rail" aria-label={label} bind:this={rail} {onscroll}>
    {#each items as it (it.tmdb_kind + it.tmdb_id)}
      <li style:width="{width}px">
        <a href="#/title/{it.tmdb_kind}/{it.tmdb_id}" title={it.title}>
          <Poster path={it.poster_path ?? null} seed={it.tmdb_id} title={it.title} {width} radius={10} size="w342" kind={it.tmdb_kind} tmdbId={it.tmdb_id} />
          <span class="name">{it.title}</span>
          <span class="meta">
            {it.tmdb_kind === 'tv' ? 'Series' : 'Film'}{#if it.release_date}{' · '}{it.release_date.slice(0, 4)}{/if}
            {#if it.vote_count >= 20}<span class="score num"><RatingLogo source="tmdb" height={12} />{it.vote_average.toFixed(1)}</span>{/if}
          </span>
        </a>
      </li>
    {/each}
  </ul>
  {#if !atStart}<button class="nav prev" type="button" aria-label="Scroll back" onclick={() => page(-1)}><Icon name="chevron-left" size={18} /></button>{/if}
  {#if !atEnd}<button class="nav next" type="button" aria-label="Scroll on" onclick={() => page(1)}><Icon name="chevron" size={18} /></button>{/if}
</div>

<style>
  .wrap { position: relative; }
  .rail { display: flex; gap: 14px; margin: 0; padding: 2px 0 6px; list-style: none; overflow-x: auto; scroll-snap-type: x proximity; scrollbar-width: none; }
  .rail::-webkit-scrollbar { display: none; }
  li { flex: none; scroll-snap-align: start; }
  a { display: grid; gap: 5px; color: inherit; text-decoration: none; }
  a:hover .name { text-decoration: underline; text-underline-offset: 3px; }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 700 15px/1.25 var(--font-display); }
  .meta { display: flex; flex-wrap: wrap; align-items: center; gap: 2px 8px; color: var(--text-muted); font-size: 12.5px; }
  .score { display: inline-flex; align-items: center; gap: 4px; color: var(--text-primary); font-weight: 600; }
  .nav { position: absolute; top: calc(var(--h, 210px) / 2 - 18px); display: grid; place-items: center; width: 36px; height: 36px; border: 0; border-radius: 50%; background: color-mix(in srgb, var(--bg-canvas) 82%, transparent); color: var(--text-primary); box-shadow: 0 4px 14px rgb(0 0 0 / 0.4); cursor: pointer; }
  .nav:hover { background: var(--bg-elevated); }
  .prev { left: -6px; }
  .next { right: -6px; }
  @media (hover: none) { .nav { display: none; } }
</style>
