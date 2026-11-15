<!-- Collection: the discs you own. Cases (front art under each format's band), Shelf (spines,
     one shelf per format, as they'd stand in a cabinet) or List (a table with the purchase
     details). The whole collection is loaded once; filters and sorting are instant.
     URL: ?format=uhd_bluray|bluray|...&sort=added|title|purchased|price&view=cases|shelf|list&q= -->
<script>
  import DiscCase from '../components/DiscCase.svelte'
  import DiscSpine from '../components/DiscSpine.svelte'
  import DiscEditor from '../components/DiscEditor.svelte'
  import Icon from '../components/Icon.svelte'
  import { api } from '../lib/api.js'
  import { route, setQuery, navigate } from '../lib/router.svelte.js'
  import { formatMoney } from '../lib/money.js'
  import { MEDIA_FORMATS, CASE, HIGHLIGHT_LABEL, label, titlesOf, isBoxSet } from '../lib/discs.js'
  import { dayLabel } from '../lib/dates.js'

  let data = $state(null)
  let err = $state('')
  let editor = $state(null)

  async function load() {
    try {
      data = await api.discs({ sort: 'added' })
      err = ''
    } catch (ex) {
      err = ex.message
    }
  }
  load()

  const SORTS = [['added', 'Recently added'], ['title', 'Title'], ['purchased', 'Recently bought'], ['price', 'Price']]
  const format = $derived(MEDIA_FORMATS.some(([v]) => v === route.query.format) ? route.query.format : '')
  const sort = $derived(SORTS.some(([v]) => v === route.query.sort) ? route.query.sort : 'added')
  const view = $derived(['shelf', 'list'].includes(route.query.view) ? route.query.view : 'cases')
  let q = $state(route.query.q ?? '')

  const items = $derived(data?.items ?? [])
  const totals = $derived(data?.totals)
  const holds = (c, f) => c.release.media_format === f || c.release.parts.some((p) => p.media_format === f)
  const text = (c) => [c.release.title, c.release.edition, c.release.studio, c.retailer, ...titlesOf(c.release).map((t) => t.title)].join(' ').toLowerCase()
  const rows = $derived(
    items
      .filter((c) => !format || holds(c, format))
      .filter((c) => !q.trim() || text(c).includes(q.trim().toLowerCase()))
      .toSorted((a, b) =>
        sort === 'title' ? a.release.title.localeCompare(b.release.title)
        : sort === 'purchased' ? (b.purchased_on ?? '').localeCompare(a.purchased_on ?? '')
        : sort === 'price' ? (b.price_minor ?? -1) - (a.price_minor ?? -1)
        : b.id - a.id)
  )
  // Shelf view: one shelf per format, best first, spines alphabetical (as you'd file them).
  const shelves = $derived(
    MEDIA_FORMATS.map(([f, l]) => [f, l, rows.filter((c) => c.release.media_format === f).toSorted((a, b) => a.release.title.localeCompare(b.release.title))]).filter(([, , cs]) => cs.length)
  )
  const formatCounts = $derived(MEDIA_FORMATS.map(([f, l]) => [f, l, totals?.by_format[f] ?? 0]).filter(([, , n]) => n))
  const topSpend = $derived(totals?.spend[0])
  const price = (c) => (c.price_minor != null ? formatMoney(c.price_minor, c.currency) : '')

  function saved(copy) {
    load()
    navigate(`/collection/${copy.id}`)
  }
</script>

<div class="page">
  <header class="head">
    <h1>Collection</h1>
    {#if totals}<span class="count num">{totals.copies} release{totals.copies === 1 ? '' : 's'} · {totals.discs} disc{totals.discs === 1 ? '' : 's'}</span>{/if}
    <button class="btn-primary add" onclick={() => (editor = { mode: 'add' })}><Icon name="plus" size={15} />Add a disc</button>
  </header>

  {#if err}
    <p class="alert" role="alert">Your collection didn’t load: {err} <button class="link" onclick={load}>Try again</button></p>
  {:else if !data}
    <div class="skeleton" style="height:420px" aria-busy="true" aria-label="Loading"></div>
  {:else if !items.length}
    <section class="empty">
      <h2>Your shelf is empty</h2>
      <p>Add the DVDs, Blu-rays and 4K discs you own: scan or type a barcode and metareel looks the release up, links it to its films on TMDB and keeps what you paid and where. Box sets, Steelbooks and slipcovers included.</p>
      <button class="btn-primary" onclick={() => (editor = { mode: 'add' })}>Add your first disc</button>
    </section>
  {:else}
    <section class="kpis" aria-label="Your collection at a glance">
      <div class="kpi">
        <span class="k-label">Releases</span><b class="num">{totals.copies}</b>
        <span class="k-sub">{totals.titles} film{totals.titles === 1 ? '' : 's'} and shows · {totals.box_sets} box set{totals.box_sets === 1 ? '' : 's'}</span>
      </div>
      <div class="kpi wide">
        <span class="k-label">Formats</span>
        <span class="stack" role="img" aria-label={formatCounts.map(([, l, n]) => `${l}: ${n}`).join(', ')}>
          {#each formatCounts as [f, , n]}<i style:flex-grow={n} style:background={CASE[f].chart}></i>{/each}
        </span>
        <span class="legend">
          {#each formatCounts as [f, l, n]}<button class="lg" aria-pressed={format === f} onclick={() => setQuery({ format: format === f ? '' : f })}><i style:background={CASE[f].chart}></i>{l} <b class="num">{n}</b></button>{/each}
        </span>
      </div>
      <div class="kpi">
        <span class="k-label">Spent</span>
        {#if topSpend}
          <b class="num">{formatMoney(topSpend.minor, topSpend.currency)}</b>
          <span class="k-sub">{formatMoney(topSpend.avg_minor, topSpend.currency)} a release · {topSpend.copies} priced{#each totals.spend.slice(1) as s}{' · '}{formatMoney(s.minor, s.currency)}{/each}</span>
        {:else}
          <b class="num muted">–</b><span class="k-sub">Add prices to see what your shelf cost</span>
        {/if}
      </div>
      <div class="kpi">
        <span class="k-label">Bought from</span>
        {#if totals.retailers.length}
          <b class="retailer">{totals.retailers[0].retailer}</b>
          <span class="k-sub">{totals.retailers[0].copies} release{totals.retailers[0].copies === 1 ? '' : 's'}{#if totals.retailers[1]}{' · then '}{totals.retailers[1].retailer} ({totals.retailers[1].copies}){/if}</span>
        {:else}
          <b class="num muted">–</b><span class="k-sub">Retailers show up here</span>
        {/if}
      </div>
    </section>

    <div class="bar">
      <nav class="chips" aria-label="Format">
        <button aria-pressed={format === ''} onclick={() => setQuery({ format: '' })}>Everything<span class="n num">{totals.copies}</span></button>
        {#each formatCounts as [f, l, n]}<button aria-pressed={format === f} onclick={() => setQuery({ format: f })}>{l}<span class="n num">{n}</span></button>{/each}
      </nav>
      <input class="filter" type="search" placeholder="Filter" bind:value={q} onchange={() => setQuery({ q: q.trim() }, { replace: true })} aria-label="Filter your collection" />
      <label class="sort">Sort
        <select value={sort} onchange={(e) => setQuery({ sort: e.currentTarget.value === 'added' ? '' : e.currentTarget.value })}>
          {#each SORTS as [v, l]}<option value={v}>{l}</option>{/each}
        </select>
      </label>
      <div class="seg" role="group" aria-label="View">
        <button aria-pressed={view === 'cases'} onclick={() => setQuery({ view: '' })}>Cases</button>
        <button aria-pressed={view === 'shelf'} onclick={() => setQuery({ view: 'shelf' })}>Shelf</button>
        <button aria-pressed={view === 'list'} onclick={() => setQuery({ view: 'list' })}>List</button>
      </div>
    </div>

    {#if !rows.length}
      <p class="none">Nothing here matches. <button class="link" onclick={() => { q = ''; setQuery({ format: '', q: '' }) }}>Show everything</button></p>
    {:else if view === 'cases'}
      <ul class="grid" aria-label="{rows.length} releases">
        {#each rows as c (c.id)}
          <li>
            <a class="card" href="#/collection/{c.id}">
              <span class="lift"><DiscCase release={c.release} width={176} /></span>
              <span class="name">{c.release.title}</span>
              <span class="meta">{label('format', c.release.media_format)}{c.release.edition ? ` · ${c.release.edition}` : ''}{isBoxSet(c.release) ? ` · ${c.release.disc_count} discs` : ''}</span>
              {#if c.release.highlights.length || price(c)}
                <span class="line">
                  {#each c.release.highlights.slice(0, 3) as h}<span class="hl">{HIGHLIGHT_LABEL[h]}</span>{/each}
                  {#if price(c)}<span class="price num">{price(c)}</span>{/if}
                </span>
              {/if}
            </a>
          </li>
        {/each}
      </ul>
    {:else if view === 'shelf'}
      <div class="cabinet">
        {#each shelves as [f, l, cs] (f)}
          <section class="shelf" aria-label="{l}, {cs.length}">
            <h2 class="shelf-h">{l} <span class="num">{cs.length}</span></h2>
            <div class="spines">
              {#each cs as c (c.id)}
                <a class="spine-link" href="#/collection/{c.id}" title="{c.release.title}{c.release.edition ? ` · ${c.release.edition}` : ''}" aria-label={c.release.title}>
                  <DiscSpine release={c.release} height={210} />
                </a>
              {/each}
            </div>
            <div class="plank" aria-hidden="true"></div>
          </section>
        {/each}
      </div>
    {:else}
      <div class="table-wrap">
        <table class="list">
          <caption class="sr">Your collection, {rows.length} releases</caption>
          <thead><tr><th scope="col">Release</th><th scope="col">Format</th><th scope="col" class="right">Discs</th><th scope="col">Picture &amp; sound</th><th scope="col">Bought</th><th scope="col">Retailer</th><th scope="col" class="right">Price</th></tr></thead>
          <tbody>
            {#each rows as c (c.id)}
              <tr>
                <td class="t"><a class="cell" href="#/collection/{c.id}"><DiscCase release={c.release} width={34} /><span class="text"><span class="nm">{c.release.title}</span><span class="meta">{[c.release.edition, label('packaging', c.release.packaging)].filter((x) => x && x !== '—').join(' · ')}</span></span></a></td>
                <td>{label('format', c.release.media_format)}</td>
                <td class="right num">{c.release.disc_count || '–'}</td>
                <td class="muted">{c.release.highlights.map((h) => HIGHLIGHT_LABEL[h]).join(', ') || '–'}</td>
                <td class="muted">{c.purchased_on ? dayLabel(c.purchased_on) : '–'}</td>
                <td>{c.retailer || '–'}</td>
                <td class="right num">{price(c) || '–'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  {/if}
</div>

<DiscEditor bind:request={editor} onsaved={saved} />

<style>
  .page { padding: 20px var(--gutter) 48px; }
  .head { display: flex; flex-wrap: wrap; align-items: baseline; gap: 10px 14px; margin-bottom: 16px; }
  h1 { margin: 0; font: 700 32px/1.1 var(--font-display); letter-spacing: -0.02em; }
  .count { color: var(--text-muted); font-size: 15px; }
  .add { margin-left: auto; align-self: center; }

  .kpis { display: grid; grid-template-columns: 1fr 1.6fr 1fr 1fr; gap: 12px; }
  .kpi { display: grid; gap: 4px; align-content: start; padding: 16px 18px; border-radius: 16px; background: var(--bg-surface); }
  .k-label { color: var(--text-muted); font-size: 13px; font-weight: 600; }
  .kpi b.num { font: 700 32px/1.05 var(--font-sans); letter-spacing: -0.03em; }
  .kpi b.muted { color: var(--text-muted); }
  .retailer { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 700 24px/1.2 var(--font-display); }
  .k-sub { color: var(--text-muted); font-size: 12.5px; }
  .stack { display: flex; gap: 2px; height: 14px; margin: 6px 0 4px; overflow: hidden; border-radius: 7px; }
  .stack i { min-width: 4px; box-shadow: inset 0 0 0 1px var(--poster-hairline); }
  .legend { display: flex; flex-wrap: wrap; gap: 4px 12px; }
  .lg { display: inline-flex; align-items: center; gap: 6px; padding: 2px 0; border: 0; background: none; color: var(--text-muted); font: 500 13px/1.2 var(--font-sans); cursor: pointer; }
  .lg b { color: var(--text-primary); }
  .lg[aria-pressed='true'] { color: var(--text-primary); text-decoration: underline; text-underline-offset: 3px; }
  .lg i { width: 10px; height: 10px; border-radius: 3px; box-shadow: inset 0 0 0 1px var(--poster-hairline); }

  .bar { display: flex; flex-wrap: wrap; align-items: center; gap: 12px 14px; margin: 22px 0 18px; }
  .chips { display: flex; flex-wrap: wrap; gap: 8px; }
  .chips button { display: inline-flex; align-items: center; gap: 8px; height: 34px; padding: 0 8px 0 14px; border: 0; border-radius: 17px; background: var(--bg-surface); color: var(--text-muted); font: 600 14px/1 var(--font-sans); cursor: pointer; }
  .chips button:hover { color: var(--text-primary); }
  .chips button[aria-pressed='true'] { background: var(--accent-primary); color: var(--accent-on); }
  .n { min-width: 22px; padding: 3px 6px; border-radius: 10px; background: var(--bg-elevated); color: var(--text-primary); font-size: 12px; text-align: center; }
  .chips button[aria-pressed='true'] .n { background: color-mix(in srgb, var(--accent-on) 18%, transparent); color: var(--accent-on); }
  .filter { height: 34px; margin-left: auto; padding: 0 14px; border: 0; border-radius: 17px; background: var(--bg-surface); color: var(--text-primary); font: 500 14px/1 var(--font-sans); width: 160px; }
  .sort { display: flex; align-items: center; gap: 8px; color: var(--text-muted); font-size: 13.5px; font-weight: 600; }
  .sort select { height: 32px; padding: 0 12px; border: 0; border-radius: 16px; background: var(--bg-elevated); color: var(--text-primary); font: 600 13.5px/1 var(--font-sans); }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { height: 30px; padding: 0 12px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13.5px/1 var(--font-sans); cursor: pointer; }
  .seg button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }

  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(176px, 1fr)); gap: 28px 22px; margin: 0; padding: 0; list-style: none; }
  .card { display: grid; gap: 4px; align-content: start; color: inherit; text-decoration: none; }
  .lift { display: block; margin-bottom: 8px; transition: transform 0.15s ease; }
  .lift :global(.case) { width: 100% !important; }
  .card:hover .lift { transform: translateY(-3px); }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font: 600 16px/1.25 var(--font-display); }
  .card:hover .name { text-decoration: underline; text-underline-offset: 3px; }
  .meta { color: var(--text-muted); font-size: 12.5px; }
  .line { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 6px; margin-top: 4px; }
  .hl { padding: 2px 6px; border-radius: 5px; background: var(--bg-elevated); color: var(--text-primary); font: 700 11px/1.2 var(--font-sans); }
  .price { margin-left: auto; color: var(--text-muted); font-size: 13px; }

  /* The cabinet: shelves of spines on a plank, like the real thing. */
  .cabinet { display: grid; gap: 28px; padding: 22px 22px 6px; border-radius: var(--radius-panel); background: var(--bg-surface); }
  .shelf-h { display: flex; gap: 8px; margin: 0 0 10px; font: 700 15px/1 var(--font-display); }
  .shelf-h .num { color: var(--text-muted); font: 500 13px/1 var(--font-sans); }
  .spines { display: flex; flex-wrap: wrap; align-items: flex-end; gap: 3px; row-gap: 18px; }
  .spine-link { display: block; transition: transform 0.15s ease; }
  .spine-link:hover, .spine-link:focus-visible { transform: translateY(-10px); }
  .plank { height: 12px; margin: 0 -10px; border-radius: 3px; background: linear-gradient(var(--bg-elevated), color-mix(in srgb, var(--bg-elevated) 60%, #000)); box-shadow: 0 8px 14px -8px rgb(0 0 0 / 0.6); }

  .table-wrap { overflow-x: auto; }
  .list { width: 100%; border-collapse: separate; border-spacing: 0; }
  .list th { height: 34px; padding: 0 12px; color: var(--text-muted); font: 600 12.5px/1 var(--font-sans); text-align: left; white-space: nowrap; }
  .list td { height: 60px; padding: 4px 12px; vertical-align: middle; white-space: nowrap; }
  .list tbody tr:not(:first-child) td { box-shadow: inset 0 1px 0 var(--border-subtle); }
  .list tbody tr:hover td { background: var(--bg-elevated); }
  .right { text-align: right !important; }
  .t { width: 40%; }
  .cell { display: flex; align-items: center; gap: 12px; min-width: 0; color: inherit; text-decoration: none; }
  .text { display: grid; min-width: 0; }
  .nm { overflow: hidden; text-overflow: ellipsis; font: 600 16px/1.25 var(--font-display); }
  .cell:hover .nm { text-decoration: underline; text-underline-offset: 3px; }
  .muted { color: var(--text-muted); }

  .empty { padding: 32px 0; display: grid; gap: 12px; justify-items: start; }
  .empty h2 { margin: 0; font: 700 24px/1.2 var(--font-display); }
  .empty p { margin: 0; color: var(--text-muted); max-width: 62ch; line-height: 1.5; }
  .none { color: var(--text-muted); }
  .link { border: 0; background: none; color: var(--text-primary); text-decoration: underline; cursor: pointer; font: inherit; }

  @media (max-width: 960px) { .kpis { grid-template-columns: 1fr 1fr; } .wide { grid-column: 1 / -1; order: -1; } }
  @media (max-width: 560px) {
    .kpis { gap: 8px; }
    .kpi { padding: 12px; }
    .kpi b.num { font-size: 26px; }
    .grid { grid-template-columns: repeat(auto-fill, minmax(132px, 1fr)); gap: 20px 12px; }
    .filter { margin-left: 0; flex: 1; }
    .cabinet { padding: 16px 12px 4px; }
  }
  @media (prefers-reduced-motion: reduce) { .lift, .spine-link { transition: none; } }
</style>
