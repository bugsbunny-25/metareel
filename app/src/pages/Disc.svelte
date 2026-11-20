<!-- One disc in your collection: its pictures (front, back, slipcover, the discs themselves),
     what is on it (linked to the title pages), every disc's picture and sound, the special
     features and, for a box set, each part. Your copy (price, retailer, date, condition) is
     edited here; the release itself through the editor, since it is shared. -->
<script>
  import DiscCase from '../components/DiscCase.svelte'
  import DiscEditor from '../components/DiscEditor.svelte'
  import FormatBadge from '../components/FormatBadge.svelte'
  import Poster from '../components/Poster.svelte'
  import { api } from '../lib/api.js'
  import { navigate } from '../lib/router.svelte.js'
  import { toast } from '../lib/toast.svelte.js'
  import { loadFormats } from '../lib/formats.js'
  import { formatMoney, fromMinor, toMinor } from '../lib/money.js'
  import { dayLabel } from '../lib/dates.js'
  import { CONDITIONS, IMAGE_KINDS, HIGHLIGHT_LABEL, allImages, label, videoLine, audioLine, isBoxSet } from '../lib/discs.js'

  let { id, onconfirm } = $props()

  let copy = $state(null)
  let err = $state('')
  let editor = $state(null)
  let labels = $state([])
  loadFormats().then((all) => (labels = all.filter((f) => f.category === 'label'))).catch(() => {})

  async function load() {
    try {
      copy = await api.disc(id)
      err = ''
    } catch (ex) {
      err = ex.status === 404 ? 'This disc isn’t in your collection.' : ex.message
    }
  }
  load()

  const r = $derived(copy?.release)
  const images = $derived(allImages(r))
  let shown = $state(null) // the picture in the big frame
  const main = $derived(images.find((i) => i.id === shown) ?? images.find((i) => i.kind === 'front') ?? images[0])
  const labelFormat = $derived(r?.label ? labels.find((f) => f.slug === r.label) : null)

  // Your copy, edited in place.
  let editingCopy = $state(false)
  let cf = $state({})
  function editCopy() {
    cf = { price: fromMinor(copy.price_minor, copy.currency ?? 'USD'), currency: copy.currency ?? '', retailer: copy.retailer, purchased_on: copy.purchased_on ?? '', condition: copy.condition, notes: copy.notes ?? '' }
    editingCopy = true
  }
  let busy = $state(false)
  async function saveCopy(e) {
    e.preventDefault()
    const cur = cf.currency.trim().toUpperCase()
    const minor = toMinor(cf.price, cur || 'USD')
    if (minor === undefined) return toast('The price isn’t a number.', 'error')
    busy = true
    try {
      copy = await api.updateDisc(copy.id, {
        purchased_on: cf.purchased_on, retailer: cf.retailer.trim(), condition: cf.condition, notes: cf.notes.trim(),
        ...(minor !== null ? { price_minor: minor, currency: cur } : {}),
      })
      editingCopy = false
      toast('Saved')
    } catch (ex) {
      toast(ex.message, 'error')
    } finally {
      busy = false
    }
  }

  function remove() {
    onconfirm({
      title: 'Remove from your collection?',
      body: `${r.title}${r.edition ? ` (${r.edition})` : ''} comes off your shelf with what you paid for it. The release itself stays in metareel for the next scan of its barcode.`,
      confirm: 'Remove',
      danger: true,
      run: async () => {
        await api.deleteDisc(copy.id)
        toast(`Removed ${r.title}`)
        navigate('/collection')
      },
    })
  }

  // Photos: pick a kind, then a file; it goes to the release or one of its parts.
  let upKind = $state('front')
  let upTarget = $state(0)
  let uploading = $state(false)
  let fileInput = $state(null)
  async function upload(e) {
    const file = e.currentTarget.files?.[0]
    e.currentTarget.value = ''
    if (!file) return
    uploading = true
    try {
      const im = await api.uploadDiscImage(upTarget || r.id, upKind, file)
      await load()
      shown = im.id
      toast(`${label('kind', upKind)} picture added`)
    } catch (ex) {
      toast(ex.message, 'error')
    } finally {
      uploading = false
    }
  }
  async function dropImage(im) {
    try {
      await api.deleteDiscImage(im.releaseId, im.id)
      if (shown === im.id) shown = null
      await load()
    } catch (ex) {
      toast(ex.message, 'error')
    }
  }
  async function setKind(im, kind) {
    try {
      await api.updateDiscImage(im.releaseId, im.id, { kind, position: im.position })
      await load()
    } catch (ex) {
      toast(ex.message, 'error')
    }
  }

  let lightbox = $state(null)
  const year = (t) => (t.year ? ` (${t.year})` : '')
  const titleHref = (t) => (t.tmdb_kind === 'tv' ? `#/show/${t.tmdb_id}` : `#/title/movie/${t.tmdb_id}`)
  const facts = (x) => [
    ['Format', label('format', x.media_format)],
    ['Packaging', x.packaging ? label('packaging', x.packaging) + (x.has_slipcover ? ' with slipcover' : '') : x.has_slipcover ? 'Slipcover' : ''],
    ['Region', x.region],
    ['Studio', x.studio],
    ['Released', x.release_date ? dayLabel(x.release_date) : ''],
    ['Country', x.country ?? ''],
    ['Barcode', x.barcode ?? ''],
  ].filter(([, v]) => v)
</script>

{#snippet discs(rel)}
  {#if rel.discs.length}
    <ol class="discs">
      {#each rel.discs as d}
        <li class="disc">
          <div class="dh">
            <span class="dn num">{d.number}</span>
            <div>
              <b>{label('format', d.format)}{d.capacity ? ` · ${label('capacity', d.capacity)}` : ''}</b>
              <span class="muted">{[d.name, d.title_index != null && rel.titles[d.title_index] ? rel.titles[d.title_index].title : '', d.season_number != null ? `Season ${d.season_number}${d.episodes ? `, episodes ${d.episodes}` : ''}` : '', d.runtime_minutes ? `${d.runtime_minutes} min` : '', d.region ? `Region ${d.region}` : ''].filter(Boolean).join(' · ')}</span>
            </div>
          </div>
          {#if videoLine(d)}<p class="spec"><span class="k">Video</span>{videoLine(d)}</p>{/if}
          {#if d.audio?.length}<div class="spec"><span class="k">Audio</span><ul>{#each d.audio as a}<li>{audioLine(a)}</li>{/each}</ul></div>{/if}
          {#if d.subtitles?.length}<p class="spec"><span class="k">Subtitles</span>{d.subtitles.join(', ')}</p>{/if}
          {#if d.notes}<p class="spec"><span class="k">Notes</span>{d.notes}</p>{/if}
        </li>
      {/each}
    </ol>
  {/if}
  {#if rel.special_features.length}
    <div class="features">
      <h4>Special features</h4>
      <ul>
        {#each rel.special_features as f}
          <li><span>{f.title}</span><span class="muted num">{[f.minutes ? `${f.minutes} min` : '', f.disc ? `disc ${f.disc}` : ''].filter(Boolean).join(' · ')}</span></li>
        {/each}
      </ul>
    </div>
  {/if}
{/snippet}

<div class="page">
  <a class="back" href="#/collection">← Collection</a>
  {#if err}
    <p class="alert" role="alert">{err}</p>
  {:else if !copy}
    <div class="skeleton" style="height:520px" aria-busy="true" aria-label="Loading"></div>
  {:else}
    <section class="hero">
      <div class="gallery">
        <div class="frame">
          {#if main}
            <button class="big" onclick={() => (lightbox = main)} aria-label="Open the {label('kind', main.kind)} picture"><img src={main.url} alt="{label('kind', main.kind)} of {r.title}" /></button>
          {:else}
            <DiscCase release={r} width={300} eager />
          {/if}
        </div>
        {#if images.length}
          <ul class="thumbs" aria-label="Pictures">
            {#each images as im (im.id)}
              <li>
                <button class:on={main?.id === im.id} onclick={() => (shown = im.id)} title="{label('kind', im.kind)}{im.partTitle ? ` · ${im.partTitle}` : ''}">
                  <img src={im.thumb_url} alt="" loading="lazy" />
                  <span>{label('kind', im.kind)}</span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
        <div class="photo">
          <select bind:value={upKind} aria-label="Picture of">{#each IMAGE_KINDS as [v, l]}<option value={v}>{l}</option>{/each}</select>
          {#if r.parts.length}
            <select bind:value={upTarget} aria-label="For">
              <option value={0}>The box</option>
              {#each r.parts as p}<option value={p.id}>{p.title}</option>{/each}
            </select>
          {/if}
          <button class="btn-secondary sm" disabled={uploading} onclick={() => fileInput.click()}>{uploading ? 'Uploading…' : 'Add a photo'}</button>
          <input bind:this={fileInput} type="file" accept="image/jpeg,image/png,image/webp,image/gif" hidden onchange={upload} />
        </div>
      </div>

      <div class="about">
        <p class="eyebrow">{label('format', r.media_format)}{isBoxSet(r) ? ` · box set of ${r.disc_count} discs` : r.disc_count > 1 ? ` · ${r.disc_count} discs` : ''}</p>
        <h1>{r.title}</h1>
        {#if r.edition}<p class="edition">{r.edition}</p>{/if}
        {#if r.highlights.length || labelFormat}
          <div class="badges">
            {#if labelFormat}<span class="plate"><FormatBadge format={labelFormat} height={14} /></span>{/if}
            {#each r.highlights as h}<span class="hl">{HIGHLIGHT_LABEL[h]}</span>{/each}
          </div>
        {/if}

        {#if r.titles.length}
          <ul class="on-it" aria-label="What’s on it">
            {#each r.titles as t}
              <li><a href={titleHref(t)}><Poster path={t.poster_path} seed={t.tmdb_id} title={t.title} kind={t.tmdb_kind} tmdbId={t.tmdb_id} width={46} radius={4} size="w92" /><span><b>{t.title}</b>{year(t)}{t.season_number != null ? ` · Season ${t.season_number}` : ''}</span></a></li>
            {/each}
          </ul>
        {/if}

        <dl class="facts">
          {#each facts(r) as [k, v]}<div><dt>{k}</dt><dd class:num={k === 'Barcode'}>{v}</dd></div>{/each}
        </dl>
        {#if r.notes}<p class="notes">{r.notes}</p>{/if}

        <div class="actions">
          <button class="btn-secondary" onclick={() => (editor = { mode: 'release', release: r })}>Edit release</button>
          <button class="btn-secondary danger" onclick={remove}>Remove</button>
        </div>
      </div>

      <aside class="mine" aria-labelledby="mine-h">
        <h2 id="mine-h">Your copy</h2>
        {#if editingCopy}
          <form onsubmit={saveCopy}>
            <div class="row"><label>Price<input bind:value={cf.price} inputmode="decimal" /></label><label class="cur">Currency<input class="cc" bind:value={cf.currency} maxlength="3" /></label></div>
            <label>Retailer<input bind:value={cf.retailer} maxlength="120" /></label>
            <label>Bought on<input type="date" bind:value={cf.purchased_on} /></label>
            <label>Condition<select bind:value={cf.condition}>{#each CONDITIONS as [v, l]}<option value={v}>{l}</option>{/each}</select></label>
            <label>Notes<textarea rows="2" bind:value={cf.notes}></textarea></label>
            <div class="row end"><button type="button" class="btn-secondary sm" onclick={() => (editingCopy = false)}>Cancel</button><button class="btn-primary sm" disabled={busy}>Save</button></div>
          </form>
        {:else}
          <p class="paid num">{copy.price_minor != null ? formatMoney(copy.price_minor, copy.currency) : '–'}</p>
          <p class="muted">{[copy.retailer, copy.purchased_on ? dayLabel(copy.purchased_on) : '', label('condition', copy.condition) === '—' ? '' : label('condition', copy.condition)].filter(Boolean).join(' · ') || 'No purchase details yet'}</p>
          {#if copy.notes}<p class="notes">{copy.notes}</p>{/if}
          <button class="btn-secondary sm" onclick={editCopy}>Edit</button>
        {/if}
      </aside>
    </section>

    {#if r.discs.length || r.special_features.length}
      <section class="panel" aria-label="Discs">{@render discs(r)}</section>
    {/if}

    {#each r.parts as p (p.id)}
      <section class="panel part" aria-labelledby="part-{p.id}">
        <div class="part-h">
          <DiscCase release={p} width={92} />
          <div>
            <p class="eyebrow">{label('format', p.media_format)}{p.packaging ? ` · ${label('packaging', p.packaging)}` : ''}</p>
            <h3 id="part-{p.id}">{p.title}</h3>
            {#if p.edition}<p class="edition">{p.edition}</p>{/if}
            {#if p.titles.length}<p class="muted">{#each p.titles as t, i}{i ? ', ' : ''}<a href={titleHref(t)}>{t.title}</a>{/each}</p>{/if}
          </div>
        </div>
        {@render discs(p)}
      </section>
    {/each}
  {/if}
</div>

{#if lightbox}
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="lightbox" onclick={() => (lightbox = null)}>
    <img src={lightbox.url} alt="{label('kind', lightbox.kind)} of {r.title}" />
    <div class="lb-bar" onclick={(e) => e.stopPropagation()}>
      <span>{label('kind', lightbox.kind)}{lightbox.partTitle ? ` · ${lightbox.partTitle}` : ''} · {lightbox.width}×{lightbox.height}</span>
      <select value={lightbox.kind} onchange={(e) => { setKind(lightbox, e.currentTarget.value); lightbox = null }} aria-label="Picture of">{#each IMAGE_KINDS as [v, l]}<option value={v}>{l}</option>{/each}</select>
      <button class="btn-secondary sm danger" onclick={() => { dropImage(lightbox); lightbox = null }}>Delete picture</button>
      <button class="btn-secondary sm" onclick={() => (lightbox = null)}>Close</button>
    </div>
  </div>
{/if}

<svelte:window onkeydown={(e) => e.key === 'Escape' && (lightbox = null)} />
<DiscEditor bind:request={editor} onsaved={load} />

<style>
  .page { padding: 16px var(--gutter) 48px; display: grid; gap: 18px; }
  .back { justify-self: start; color: var(--text-muted); font-size: 14px; font-weight: 600; text-decoration: none; }
  .back:hover { color: var(--text-primary); }
  .hero { display: grid; grid-template-columns: minmax(260px, 380px) 1fr 260px; gap: 28px; align-items: start; }
  .gallery { display: grid; gap: 10px; }
  .frame { display: grid; place-items: center; min-height: 300px; padding: 14px; border-radius: var(--radius-panel); background: var(--bg-surface); }
  .big { padding: 0; border: 0; background: none; cursor: zoom-in; }
  .big img { display: block; max-width: 100%; max-height: 460px; border-radius: 6px; box-shadow: var(--shadow-art); }
  .thumbs { display: flex; gap: 8px; margin: 0; padding: 0 0 4px; overflow-x: auto; list-style: none; }
  .thumbs button { display: grid; gap: 4px; padding: 4px; border: 0; border-radius: 10px; background: none; color: var(--text-muted); font: 600 11px/1.2 var(--font-sans); cursor: pointer; }
  .thumbs button.on { background: var(--bg-surface); color: var(--text-primary); box-shadow: inset 0 0 0 2px var(--accent-yours); }
  .thumbs img { width: 60px; height: 72px; object-fit: cover; border-radius: 5px; }
  .photo { display: flex; flex-wrap: wrap; gap: 8px; }
  .photo select { height: 32px; padding: 0 10px; border: 0; border-radius: 16px; background: var(--bg-elevated); color: var(--text-primary); font: 600 13px/1 var(--font-sans); }

  .about { display: grid; gap: 12px; align-content: start; min-width: 0; }
  .eyebrow { margin: 0; color: var(--text-muted); font-size: 12.5px; font-weight: 700; letter-spacing: 0.05em; text-transform: uppercase; }
  h1 { margin: 0; font: 700 38px/1.08 var(--font-display); letter-spacing: -0.02em; }
  .edition { margin: -4px 0 0; color: var(--text-muted); font: 500 italic 17px/1.3 var(--font-display); }
  .badges { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
  .hl { padding: 4px 8px; border-radius: 6px; background: var(--bg-elevated); font: 700 12px/1.2 var(--font-sans); }
  .plate { display: inline-flex; align-items: center; height: 26px; padding: 0 8px; border-radius: 6px; background: #161618; }
  .on-it { display: grid; gap: 8px; margin: 4px 0; padding: 0; list-style: none; }
  .on-it a { display: flex; align-items: center; gap: 12px; color: inherit; text-decoration: none; font-size: 15px; }
  .on-it a:hover b { text-decoration: underline; text-underline-offset: 3px; }
  .on-it b { font: 600 17px/1.2 var(--font-display); }
  .facts { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px 18px; margin: 4px 0 0; }
  dt { color: var(--text-muted); font-size: 12px; font-weight: 600; }
  dd { margin: 2px 0 0; font-size: 15px; }
  .notes { margin: 0; color: var(--text-muted); white-space: pre-line; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 4px; }
  .danger { color: var(--delta-down) !important; }
  .sm { height: 32px; padding: 0 14px; font-size: 13.5px; }

  .mine { display: grid; gap: 8px; align-content: start; padding: 18px; border-radius: var(--radius-panel); background: var(--bg-surface); }
  .mine h2 { margin: 0; font: 700 16px/1.2 var(--font-display); }
  .paid { margin: 0; color: var(--accent-yours); font: 700 32px/1.05 var(--font-sans); letter-spacing: -0.03em; }
  .mine p { margin: 0; }
  .mine form { display: grid; gap: 10px; }
  .mine label { display: grid; gap: 5px; color: var(--text-muted); font-size: 12.5px; font-weight: 600; }
  .mine input, .mine select, .mine textarea { min-width: 0; padding: 8px 10px; border: 1px solid var(--border-subtle); border-radius: 9px; background: var(--bg-canvas); color: var(--text-primary); font: 400 14.5px/1.3 var(--font-sans); }
  .row { display: flex; gap: 8px; }
  .row label { flex: 1; }
  .row .cur { flex: 0 0 76px; }
  .row.end { justify-content: flex-end; }
  .cc { text-transform: uppercase; }
  .muted { color: var(--text-muted); }

  .panel { display: grid; gap: 16px; padding: 20px; border-radius: var(--radius-panel); background: var(--bg-surface); }
  .part-h { display: flex; gap: 16px; align-items: center; }
  .part-h h3 { margin: 2px 0 0; font: 700 22px/1.15 var(--font-display); }
  .part-h p { margin: 2px 0 0; }
  .part-h a { color: inherit; }
  .discs { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 12px; margin: 0; padding: 0; list-style: none; }
  .disc { display: grid; gap: 8px; align-content: start; padding: 14px; border-radius: 14px; background: var(--bg-canvas); }
  .dh { display: flex; gap: 12px; align-items: center; }
  .dh div { display: grid; }
  .dn { display: grid; place-items: center; width: 34px; height: 34px; border-radius: 50%; background: var(--bg-elevated); font: 700 15px/1 var(--font-sans); }
  .spec { display: grid; grid-template-columns: 76px 1fr; gap: 8px; margin: 0; font-size: 14px; }
  .spec ul { margin: 0; padding: 0; list-style: none; display: grid; gap: 2px; }
  .k { color: var(--text-muted); font-size: 12.5px; font-weight: 600; }
  .features h4 { margin: 0 0 8px; font: 700 16px/1.2 var(--font-display); }
  .features ul { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 4px 24px; margin: 0; padding: 0; list-style: none; }
  .features li { display: flex; justify-content: space-between; gap: 12px; padding: 6px 0; box-shadow: inset 0 -1px 0 var(--border-subtle); font-size: 14.5px; }

  .lightbox { position: fixed; inset: 0; z-index: 50; display: grid; place-items: center; padding: 24px 24px 84px; background: rgb(0 0 0 / 0.86); cursor: zoom-out; }
  .lightbox img { max-width: 100%; max-height: 100%; object-fit: contain; border-radius: 6px; }
  .lb-bar { position: fixed; left: 50%; bottom: 20px; transform: translateX(-50%); display: flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: 10px; padding: 8px 12px; border-radius: 22px; background: var(--bg-surface); color: var(--text-primary); font-size: 13.5px; cursor: default; }
  .lb-bar select { height: 32px; padding: 0 10px; border: 0; border-radius: 16px; background: var(--bg-elevated); color: var(--text-primary); }

  @media (max-width: 1100px) { .hero { grid-template-columns: minmax(240px, 340px) 1fr; } .mine { grid-column: 1 / -1; } }
  @media (max-width: 720px) {
    .hero { grid-template-columns: 1fr; }
    h1 { font-size: 30px; }
    .discs, .features ul { grid-template-columns: 1fr; }
  }
</style>
