<!-- A poster tile for Discover grids: artwork linking to the title page, the name, a watchlist
     toggle, a meta line and optional extra lines (children). Titles without a TMDB ID (rare in
     JustWatch data) show without a link or toggle. kind: movie | tv. -->
<script>
  import Poster from './Poster.svelte'
  import Icon from './Icon.svelte'
  import { library } from '../lib/library.svelte.js'
  import { toast } from '../lib/toast.svelte.js'

  let { kind, tmdbId = null, title, poster = null, meta = '', badge = '', badgeTone = '', children } = $props()

  const t = $derived(tmdbId ? library.get(kind, tmdbId) : null)
  const on = $derived(t?.status === 'watchlist')
  const seen = $derived(!!t && (t.watch_count > 0 || t.status === 'completed'))
  const href = $derived(tmdbId ? `#/title/${kind}/${tmdbId}` : null)

  async function toggle() {
    try {
      const now = await library.toggleWatchlist(kind, tmdbId)
      toast(now ? `Added ${title} to your watchlist.` : `Removed ${title} from your watchlist.`)
    } catch (err) {
      toast(err.message, 'error')
    }
  }
</script>

<li class="tile">
  <svelte:element this={href ? 'a' : 'div'} class="art" {href}>
    <Poster path={poster} seed={tmdbId ?? title} {title} width={170} radius={10} size="w342" />
    {#if badge}<span class="badge {badgeTone}">{badge}</span>{/if}
    {#if seen}<span class="seen" title="You’ve seen it"><Icon name="check" size={12} /></span>{/if}
  </svelte:element>
  <div class="tt">
    {#if href}<a class="name" {href}>{title}</a>{:else}<span class="name">{title}</span>{/if}
    {#if tmdbId}
      <button class="mark" aria-pressed={on} aria-label="{on ? 'Remove from' : 'Add to'} watchlist: {title}" title={on ? 'On your watchlist' : 'Add to watchlist'} onclick={toggle}>
        <Icon name={on ? 'bookmark-fill' : 'bookmark'} size={15} />
      </button>
    {/if}
  </div>
  {#if meta}<p class="meta">{meta}</p>{/if}
  {@render children?.()}
</li>

<style>
  .tile { display: grid; gap: 5px; align-content: start; min-width: 0; }
  .art { position: relative; display: block; transition: transform 0.15s ease; }
  a.art:hover { transform: translateY(-2px); }
  .art :global(.poster) { width: 100% !important; }
  .badge { position: absolute; left: 8px; bottom: 8px; padding: 3px 7px; border-radius: 8px; background: rgb(0 0 0 / 0.74); color: #fff; font-size: 12px; font-weight: 700; }
  .badge.gold { color: #f5c518; }
  .badge.new { color: var(--delta-debut); }
  .seen { position: absolute; right: 8px; top: 8px; display: grid; place-items: center; width: 22px; height: 22px; border-radius: 50%; background: var(--accent-yours); color: var(--bg-canvas); }
  .tt { display: flex; align-items: flex-start; justify-content: space-between; gap: 6px; margin-top: 4px; }
  .name { font: 700 15.5px/1.25 var(--font-display); color: inherit; text-decoration: none; }
  a.name:hover { text-decoration: underline; text-underline-offset: 3px; }
  .mark { flex: none; display: grid; place-items: center; width: 28px; height: 28px; border: 0; border-radius: 50%; background: var(--bg-elevated); color: var(--text-muted); cursor: pointer; }
  .mark:hover { color: var(--text-primary); }
  .mark[aria-pressed='true'] { background: color-mix(in srgb, var(--accent-yours) 20%, transparent); color: var(--accent-yours); }
  .meta { margin: 0; color: var(--text-muted); font-size: 12.5px; }
</style>
