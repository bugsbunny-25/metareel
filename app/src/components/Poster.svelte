<script>
  // Shared by every direction: 2:3 lock, inner hairline, deterministic placeholder.
  import { posterSrc } from '../lib/rows.js';

  let { path = null, title, seed = title, width = 40, size = 'w342', radius = 2 } = $props();

  const src = $derived(posterSrc(path, size));
  let failedSrc = $state(null); // a 404 falls back to the same placeholder as a null path

  // Same seed → same stripe angle and initials on every render and device. No Math.random.
  const hash = $derived([...String(seed)].reduce((a, c) => (a * 31 + c.charCodeAt(0)) >>> 0, 7));
  const angle = $derived(20 + (hash % 6) * 25);
  const initials = $derived(
    title.split(/\s+/).filter((w) => /\p{L}/u.test(w[0] ?? '')).slice(0, 2).map((w) => w[0]).join('').toUpperCase()
  );
</script>

<span class="poster" style:width="{width}px" style:--r="{radius}px">
  {#if src && src !== failedSrc}
    <img {src} alt="" loading="lazy" decoding="async" onerror={() => (failedSrc = src)} />
  {:else}
    <span class="ph" style:--a="{angle}deg" aria-hidden="true">{initials}</span>
  {/if}
</span>

<style>
  .poster {
    position: relative;
    display: block;
    flex: none;
    max-width: 100%;
    aspect-ratio: 2 / 3;
    overflow: hidden;
    border-radius: var(--r);
    background: var(--bg-elevated);
    container-type: inline-size;
  }
  /* Hairline sits above the image so light posters keep an edge on light canvases. */
  .poster::after {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: inherit;
    box-shadow: inset 0 0 0 1px var(--poster-hairline);
    pointer-events: none;
  }
  img {
    display: block;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .ph {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    background: repeating-linear-gradient(var(--a), var(--bg-elevated) 0 5px, var(--bg-surface) 5px 10px);
    color: var(--text-muted);
    font: 600 30cqi/1 var(--font-mono);
    letter-spacing: 0.02em;
  }
</style>
