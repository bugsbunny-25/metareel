<!-- Top 10 row. Artwork-led, 64px: serif titles, change in coloured words, a 14-day trend,
     days, IMDb, RT and origin. Your titles get an orange wash and trend line. -->
<script>
  import { chartRow, moveLabel, stars, starsLabel, shortDate } from '../lib/rows.js'
  import Poster from './Poster.svelte'
  import Icon from './Icon.svelte'
  import Spark from './Spark.svelte'

  let { entry, onlog, onwatchlist } = $props()

  const r = $derived(chartRow(entry))
  const onList = $derived(r.mark.kind === 'watchlist')
  const mine = $derived(r.mark.kind !== 'none' || entry.state?.status === 'watching')
  const score = (key) => r.scores.find((s) => s.key === key)
  const owner = (o) => (o ? o.charAt(0).toUpperCase() + o.slice(1) : '')
  const change = $derived(
    { up: `Up ${r.move.by}`, down: `Down ${r.move.by}`, same: 'No change', debut: 'New', reentry: 'Back' }[r.move.type]
  )
  const meta = $derived(r.kind === 'tv' ? (r.season ? `TV · Season ${r.season}` : 'TV') : 'Film')
</script>

<tr class:mine>
  <td class="rank num">{r.rank}</td>
  <td class="move"><span class="chg {r.move.type}" aria-hidden="true">{change}</span><span class="sr">{moveLabel(r.move, r.prev)}</span></td>
  <td class="title">
    <svelte:element this={r.href ? 'a' : 'span'} href={r.href} class="link">
      <Poster path={r.poster} seed={r.seed} title={r.name} width={36} radius={5} size="w185" />
      <span class="text">
        <span class="name">{r.name}</span>
        <span class="meta">
          {meta}{#if r.offChart}<span class="note">{' · '}on the {r.category === 'tv_shows' ? 'TV' : 'movie'} chart</span>{/if}
          {#each r.flags as f}<span class="flag {f.type}">{' · '}{f.type === 'arrived' ? 'New on' : 'Left'} {f.provider} <time datetime={f.date}>{shortDate(f.date)}</time></span>{/each}
        </span>
      </span>
    </svelte:element>
  </td>
  <td class="trend hn" class:hl={mine}><Spark ranks={entry.hist} width={76} height={22} label="Rank over the last 14 days" /></td>
  <td class="num right hn">{r.days}</td>
  <td class="num right hn" class:stale={r.stale}>{score('imdb')?.value ?? '–'}</td>
  <td class="num right hn" class:stale={r.stale}>{score('rt') ? `${score('rt').value}%` : '–'}</td>
  <td class="origin hn" title={r.origin ? `${r.origin.owner} · ${r.origin.reason}` : 'No TMDB metadata yet'}>
    {#if r.origin?.key === 'original'}<span class="pill strong">{owner(r.origin.owner) || 'Service'} original</span>
    {:else if r.origin?.key === 'licensed'}<span class="pill">Licensed{r.origin.owner ? ` · ${owner(r.origin.owner)}` : ''}</span>
    {:else if r.origin?.key === 'in_house'}<span class="pill">In-house</span>
    {:else}<span class="muted">–</span>{/if}
  </td>
  <td class="me">
    <span class="actions">
      {#if r.mark.kind === 'watched'}
        <span class="seen" aria-label="Watched{r.mark.count > 1 ? ` ${r.mark.count} times` : ''}, {starsLabel(r.mark.rating)}">
          <Icon name="check" size={13} /><span aria-hidden="true">{stars(r.mark.rating) || 'Watched'}</span>
        </span>
      {:else if r.href}
        <button class="round" class:on={onList} aria-pressed={onList} title={onList ? 'On your watchlist' : 'Add to watchlist'}
          aria-label="{onList ? 'Remove' : 'Add'} {r.name} {onList ? 'from' : 'to'} watchlist" onclick={() => onwatchlist(entry)}>
          <Icon name={onList ? 'bookmark-fill' : 'bookmark'} size={15} />
        </button>
      {/if}
      {#if r.href}
        <button class="round" title="Log a watch" aria-label="Log a watch of {r.name}" onclick={() => onlog(entry)}><Icon name="plus" size={15} /></button>
      {/if}
    </span>
  </td>
</tr>

<style>
  /* Your titles: a quiet orange wash across the row. */
  tr.mine td { background: color-mix(in srgb, var(--accent-yours) 9%, transparent); }
  tr.mine td:first-child { border-radius: 12px 0 0 12px; }
  tr.mine td:last-child { border-radius: 0 12px 12px 0; }
  td { height: 64px; padding: 0 10px; vertical-align: middle; color: var(--text-primary); font: 400 14px/20px var(--font-sans); white-space: nowrap; }
  tr:hover td { background: var(--bg-elevated); }
  tr:hover td:first-child { border-radius: 12px 0 0 12px; }
  tr:hover td:last-child { border-radius: 0 12px 12px 0; }
  tr:not(:first-child) td:not(.rank):not(.move) { box-shadow: inset 0 1px 0 var(--border-subtle); }
  .right { text-align: right; }

  .rank { width: 36px; padding-right: 2px; text-align: right; color: var(--text-muted); font: 700 21px/1 var(--font-display); }
  tr.mine .rank { color: var(--accent-yours); }
  .move { width: 82px; }

  .title { width: 100%; max-width: 0; }
  .link { display: flex; align-items: center; gap: 12px; min-width: 0; color: inherit; text-decoration: none; }
  .text { display: grid; min-width: 0; }
  .name { overflow: hidden; text-overflow: ellipsis; font: 600 16px/21px var(--font-display); }
  a.link:hover .name { text-decoration: underline; text-underline-offset: 3px; text-decoration-thickness: 1px; }
  .meta { overflow: hidden; text-overflow: ellipsis; color: var(--text-muted); font-size: 12.5px; line-height: 17px; }
  .note { color: var(--text-primary); }
  .flag.left { color: var(--delta-down); }
  .flag.arrived { color: var(--delta-up); }

  .trend { width: 88px; color: var(--text-muted); }
  .trend.hl { color: var(--accent-yours); }
  .stale { color: var(--status-stale); }
  .pill.strong { color: var(--text-primary); }
  tr:hover .pill, tr.mine .pill { background: var(--bg-surface); }

  .me { width: 1%; }
  .actions { display: flex; gap: 6px; justify-content: flex-end; align-items: center; }
  .round {
    display: grid; place-items: center; width: 32px; height: 32px; border: 0; border-radius: 50%;
    background: var(--bg-elevated); color: var(--text-primary); cursor: pointer; transition: transform 0.12s ease;
  }
  tr:hover .round { background: var(--bg-surface); }
  .round:hover { transform: scale(1.06); }
  .round.on { background: var(--accent-yours); color: var(--bg-canvas); }
  .seen { display: inline-flex; align-items: center; gap: 5px; padding-right: 2px; color: var(--accent-yours); font-size: 13px; letter-spacing: 0.04em; }

  @container top10 (max-width: 560px) { td { padding-inline: 6px; } .move { width: 64px; } }
  .link:focus-visible, button:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 2px; }
</style>
