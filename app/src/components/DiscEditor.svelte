<!-- Add a disc to your collection, or edit a release. Adding starts from the barcode: a release
     metareel knows is one click (add another copy); a product the barcode service knows is a
     prefilled draft with TMDB matches, collections for box sets and retailer pictures to keep;
     otherwise you fill it in. The copy's own fields (price, retailer, date, condition) are yours.
     request: { mode: 'add', barcode? } | { mode: 'release', release } -->
<script>
  import { untrack } from 'svelte'
  import ReleaseFields from './ReleaseFields.svelte'
  import DiscCase from './DiscCase.svelte'
  import Poster from './Poster.svelte'
  import { api } from '../lib/api.js'
  import { toast } from '../lib/toast.svelte.js'
  import { toMinor, currencyFor, recall, remember } from '../lib/money.js'
  import { scope } from '../lib/scope.svelte.js'
  import { CONDITIONS, IMAGE_KINDS, blankRelease, releaseInput, label } from '../lib/discs.js'

  let { request = $bindable(null), onsaved = () => {} } = $props()

  let dialog = $state(null)
  let step = $state('scan') // scan | known | form
  let barcode = $state('')
  let looking = $state(false)
  let lookup = $state(null)
  let release = $state(blankRelease())
  let picks = $state([]) // retailer pictures: { url, kind } (kind '' = don't keep)
  let copy = $state(blankCopy())
  let busy = $state(false)
  let err = $state('')
  let barcodeInput = $state(null)

  function blankCopy() {
    return { price: '', currency: recall('disc.currency', currencyFor(scope.country)), retailer: '', purchased_on: '', condition: 'new', notes: '' }
  }

  // Opening resets the dialog. Only the request and the element are dependencies; the
  // reset itself reads state (the barcode, the scope's country) that must not re-run it.
  $effect(() => {
    const req = request
    if (!req || !dialog) return
    untrack(() => open(req))
  })

  function open(req) {
    err = ''
    lookup = null
    picks = []
    copy = blankCopy()
    if (req.mode === 'release') {
      release = releaseInput(req.release)
      step = 'form'
    } else {
      release = blankRelease()
      barcode = req.barcode ?? ''
      step = 'scan'
      if (barcode) look()
    }
    if (!dialog.open) dialog.showModal()
    // showModal focuses the first control (the close button); start on the barcode.
    if (req.mode !== 'release') requestAnimationFrame(() => barcodeInput?.focus())
  }

  function close() {
    dialog?.close()
  }

  async function look(e) {
    e?.preventDefault()
    if (!barcode.trim()) return
    looking = true
    err = ''
    try {
      lookup = await api.lookupDisc(barcode.trim())
      if (lookup.status === 'known') {
        step = 'known'
        return
      }
      const sg = lookup.suggestion
      release = blankRelease({ barcode: lookup.barcode })
      if (sg) {
        Object.assign(release, {
          title: sg.title || sg.product_title, edition: sg.edition, media_format: sg.media_format || 'bluray',
          packaging: sg.packaging, has_slipcover: sg.has_slipcover, studio: sg.studio, label: sg.label,
        })
        // The best candidate goes in; the rest stay one click away.
        const first = lookup.candidates[0]
        if (first && !sg.box_set) release.titles.push({ ...first })
        if (first?.tmdb_kind === 'tv' && sg.seasons.length) {
          release.titles = sg.seasons.map((n) => ({ ...first, season_number: n }))
        }
        picks = sg.images.map((url, i) => ({ url, kind: i === 0 ? 'front' : '' }))
      }
      step = 'form'
    } catch (ex) {
      err = ex.message
    } finally {
      looking = false
    }
  }

  function manual() {
    release = blankRelease({ barcode: barcode.trim() })
    step = 'form'
  }

  function copyBody() {
    const body = { retailer: copy.retailer.trim(), purchased_on: copy.purchased_on, condition: copy.condition, notes: copy.notes.trim() }
    const cur = copy.currency.trim().toUpperCase()
    const minor = toMinor(copy.price, cur || 'USD')
    if (minor === undefined) throw new Error('The price isn’t a number.')
    if (minor !== null) {
      if (!cur) throw new Error('A price needs its currency.')
      body.price_minor = minor
      body.currency = cur
      remember('disc.currency', cur)
    }
    return body
  }

  async function addKnown() {
    busy = true
    err = ''
    try {
      const saved = await api.addDisc({ ...copyBody(), release_id: lookup.release.id })
      toast(`Added another copy of ${saved.release.title}`)
      close()
      onsaved(saved)
    } catch (ex) {
      err = ex.message
    } finally {
      busy = false
    }
  }

  // The form's titles carry display fields; the API wants refs.
  function clean(r) {
    const out = { ...r, titles: r.titles.map((t) => ({ tmdb_kind: t.tmdb_kind, tmdb_id: t.tmdb_id, ...(t.season_number != null ? { season_number: t.season_number } : {}) })) }
    out.discs = r.discs.map((d) => ({ ...d, runtime_minutes: +d.runtime_minutes || 0 }))
    out.parts = r.parts.map(clean)
    if (!r.parts.length) delete out.parts
    return out
  }

  async function save(e) {
    e.preventDefault()
    busy = true
    err = ''
    try {
      if (request.mode === 'release') {
        const out = await api.updateDiscRelease(request.release.id, clean($state.snapshot(release)))
        toast('Release saved')
        close()
        onsaved(out)
      } else {
        const body = { ...copyBody(), release: clean($state.snapshot(release)) }
        body.import_images = picks.filter((p) => p.kind).map((p) => ({ url: p.url, kind: p.kind }))
        const saved = await api.addDisc(body)
        const missing = body.import_images.length - saved.release.images.length
        toast(missing > 0 ? `Added ${saved.release.title}; ${missing} picture${missing === 1 ? '' : 's'} couldn’t be fetched` : `Added ${saved.release.title}`)
        close()
        onsaved(saved)
      }
    } catch (ex) {
      err = ex.message
    } finally {
      busy = false
    }
  }

  const STATUS = {
    not_found: 'The barcode service doesn’t know this one. Fill it in below; the next person to scan it gets your release.',
    rate_limited: 'The free barcode lookup is used up for today. Fill it in, or try again tomorrow.',
    unavailable: 'Barcode lookups are off or failed. Fill it in below.',
  }
</script>

<dialog bind:this={dialog} onclose={() => (request = null)} aria-labelledby="disc-h">
  {#if request}
    <form onsubmit={step === 'scan' ? look : step === 'known' ? (e) => (e.preventDefault(), addKnown()) : save}>
      <header>
        <div>
          <p class="eyebrow">{request.mode === 'release' ? 'Edit release' : 'Add to your collection'}</p>
          <h2 id="disc-h">{request.mode === 'release' ? request.release.title : step === 'known' ? lookup.release.title : release.title || 'A disc'}</h2>
        </div>
        <button type="button" class="close" aria-label="Close" onclick={close}>×</button>
      </header>

      <div class="body">
        {#if step === 'scan'}
          <p class="lead">Type the barcode under the case’s back cover (UPC or EAN). The mobile app can scan it with the camera.</p>
          <div class="scan">
            <input bind:value={barcode} inputmode="numeric" autocomplete="off" placeholder="e.g. 883929640010" aria-label="Barcode" bind:this={barcodeInput} />
            <button class="btn-primary" disabled={looking || !barcode.trim()}>{looking ? 'Looking…' : 'Look up'}</button>
          </div>
          <button type="button" class="link" onclick={manual}>No barcode? Enter it by hand</button>
        {:else if step === 'known'}
          <div class="known">
            <DiscCase release={lookup.release} width={120} eager />
            <div>
              <p><b>{lookup.release.title}</b>{lookup.release.edition ? ` · ${lookup.release.edition}` : ''}</p>
              <p class="muted">{label('format', lookup.release.media_format)} · {lookup.release.disc_count} disc{lookup.release.disc_count === 1 ? '' : 's'}{lookup.release.studio ? ` · ${lookup.release.studio}` : ''}</p>
              {#if lookup.owned_copy_ids.length}<p class="yours">You have {lookup.owned_copy_ids.length === 1 ? 'a copy' : `${lookup.owned_copy_ids.length} copies`} already. <a href="#/collection/{lookup.owned_copy_ids[0]}" onclick={close}>Open it</a></p>{/if}
            </div>
          </div>
        {:else}
          {#if lookup && STATUS[lookup.status]}<p class="note">{STATUS[lookup.status]}</p>{/if}
          {#if lookup?.suggestion}
            <p class="note">From <b>UPCitemdb</b>: “{lookup.suggestion.product_title}”. Check the details; retailer listings are often vague about editions and discs.</p>
            {#if lookup.candidates.length > 1 || lookup.collections.length}
              <div class="cands">
                <span class="lbl">Matches on TMDB</span>
                <div class="chips">
                  {#each lookup.collections as c (c.id)}
                    <button type="button" class="pick" onclick={() => { for (const p of c.parts) if (!release.titles.some((t) => t.tmdb_id === p.tmdb_id)) release.titles.push({ ...p }) }}>+ {c.name} ({c.parts.length})</button>
                  {/each}
                  {#each lookup.candidates.slice(0, 6) as c (c.tmdb_kind + c.tmdb_id)}
                    <button type="button" class="pick" aria-pressed={release.titles.some((t) => t.tmdb_kind === c.tmdb_kind && t.tmdb_id === c.tmdb_id)}
                      onclick={() => { const i = release.titles.findIndex((t) => t.tmdb_kind === c.tmdb_kind && t.tmdb_id === c.tmdb_id); if (i >= 0) release.titles.splice(i, 1); else release.titles.push({ ...c }) }}>
                      <Poster path={c.poster_path} seed={c.tmdb_id} title={c.title} width={18} radius={2} size="w92" />{c.title}{c.year ? ` (${c.year})` : ''}
                    </button>
                  {/each}
                </div>
              </div>
            {/if}
            {#if picks.length}
              <div class="cands">
                <span class="lbl">Retailer pictures <span class="muted">· keep the ones that show this edition</span></span>
                <div class="picks">
                  {#each picks as p (p.url)}
                    <figure class:on={p.kind}>
                      <img src={p.url} alt="" loading="lazy" referrerpolicy="no-referrer" />
                      <select bind:value={p.kind} aria-label="Use this picture as">
                        <option value="">Don’t keep</option>
                        {#each IMAGE_KINDS as [v, l]}<option value={v}>{l}</option>{/each}
                      </select>
                    </figure>
                  {/each}
                </div>
              </div>
            {/if}
          {/if}
          <ReleaseFields bind:release />
        {/if}

        {#if request.mode === 'add' && step !== 'scan'}
          <fieldset class="copy">
            <legend>Your copy</legend>
            <div class="row money">
              <label>Price<input bind:value={copy.price} inputmode="decimal" placeholder="24.99" /></label>
              <label>Currency<input class="cc" bind:value={copy.currency} maxlength="3" /></label>
              <label>Retailer<input bind:value={copy.retailer} maxlength="120" placeholder="Amazon, Best Buy, HMV…" /></label>
            </div>
            <div class="row two">
              <label>Bought on<input type="date" bind:value={copy.purchased_on} /></label>
              <label>Condition<select bind:value={copy.condition}>{#each CONDITIONS as [v, l]}<option value={v}>{l}</option>{/each}</select></label>
            </div>
            <label>Notes<input bind:value={copy.notes} maxlength="4000" placeholder="Signed, missing slipcover, gift…" /></label>
          </fieldset>
        {/if}
      </div>

      <footer>
        {#if err}<p class="err" role="alert">{err}</p>{/if}
        {#if step === 'form' && request.mode === 'add'}<button type="button" class="btn-secondary" onclick={() => (step = 'scan')}>Back</button>{/if}
        <button type="button" class="btn-secondary" onclick={close}>Cancel</button>
        {#if step === 'known'}
          <button class="btn-primary" disabled={busy}>{busy ? 'Adding…' : 'Add another copy'}</button>
        {:else if step === 'form'}
          <button class="btn-primary" disabled={busy || !release.title.trim()}>{busy ? 'Saving…' : request.mode === 'release' ? 'Save release' : 'Add to collection'}</button>
        {/if}
      </footer>
    </form>
  {/if}
</dialog>

<style>
  dialog { width: min(760px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); padding: 0; border: 0; border-radius: 20px; background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 30px 80px -20px rgb(0 0 0 / 0.6); overflow: hidden; }
  dialog[open] { display: flex; }
  dialog::backdrop { background: rgb(0 0 0 / 0.55); }
  form { display: flex; flex-direction: column; width: 100%; max-height: calc(100dvh - 32px); }
  header { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 20px 22px 14px; border-bottom: 1px solid var(--border-subtle); }
  .eyebrow { margin: 0 0 2px; color: var(--text-muted); font-size: 12px; font-weight: 600; letter-spacing: 0.05em; text-transform: uppercase; }
  h2 { margin: 0; font: 700 22px/1.15 var(--font-display); }
  .close { width: 34px; height: 34px; border: 0; border-radius: 50%; background: var(--bg-elevated); color: var(--text-primary); font-size: 20px; cursor: pointer; }
  .body { display: grid; gap: 16px; padding: 16px 22px 20px; overflow-y: auto; overscroll-behavior: contain; }
  .lead { margin: 0; color: var(--text-muted); }
  .scan { display: flex; gap: 10px; }
  .scan input { flex: 1; font-size: 20px; letter-spacing: 0.06em; font-variant-numeric: tabular-nums; }
  label { display: grid; gap: 6px; font-size: 13px; font-weight: 600; color: var(--text-muted); }
  input, select { min-width: 0; padding: 9px 12px; border: 1px solid var(--border-subtle); border-radius: 10px; background: var(--bg-canvas); color: var(--text-primary); font: 400 15px/1.3 var(--font-sans); }
  .cc { text-transform: uppercase; }
  .known { display: flex; gap: 18px; align-items: center; }
  .known p { margin: 0 0 4px; }
  .muted { color: var(--text-muted); font-weight: 400; }
  .yours { color: var(--accent-yours); font-weight: 600; }
  .note { margin: 0; padding: 10px 14px; border-radius: 12px; background: var(--bg-elevated); font-size: 14px; line-height: 1.45; }
  .cands { display: grid; gap: 8px; }
  .lbl { font-size: 13px; font-weight: 600; color: var(--text-muted); }
  .chips { display: flex; flex-wrap: wrap; gap: 6px; }
  .pick { display: inline-flex; align-items: center; gap: 6px; height: 30px; padding: 0 11px 0 5px; border: 1px solid var(--border-subtle); border-radius: 15px; background: var(--bg-surface); color: var(--text-primary); font: 500 13px/1 var(--font-sans); cursor: pointer; }
  .pick[aria-pressed='true'] { border-color: var(--accent-yours); background: color-mix(in srgb, var(--accent-yours) 16%, var(--bg-surface)); font-weight: 700; }
  .picks { display: flex; gap: 10px; overflow-x: auto; padding-bottom: 4px; }
  figure { display: grid; gap: 6px; flex: none; width: 112px; margin: 0; opacity: 0.55; }
  figure.on { opacity: 1; }
  figure img { width: 112px; height: 140px; object-fit: contain; border-radius: 8px; background: #fff; }
  figure select { padding: 5px 6px; font-size: 12.5px; }
  .copy { display: grid; gap: 12px; margin: 0; padding: 14px; border: 1px solid var(--border-subtle); border-radius: 14px; }
  legend { padding: 0 4px; color: var(--text-primary); font: 700 14px/1 var(--font-display); }
  .row { display: grid; gap: 12px; }
  .row.two { grid-template-columns: 1fr 1fr; }
  .row.money { grid-template-columns: 110px 80px 1fr; }
  .link { justify-self: start; padding: 0; border: 0; background: none; color: var(--text-primary); font: 600 14px/1.3 var(--font-sans); text-decoration: underline; text-underline-offset: 3px; cursor: pointer; }
  footer { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 10px; padding: 14px 22px; border-top: 1px solid var(--border-subtle); }
  .err { flex: 1 1 100%; margin: 0; color: var(--delta-down); font-size: 13.5px; }
  @media (max-width: 560px) {
    .row.two { grid-template-columns: 1fr; }
    .row.money { grid-template-columns: 1fr 80px; }
    .row.money label:last-child { grid-column: 1 / -1; }
    .body { padding: 14px 16px 18px; }
    header, footer { padding-inline: 16px; }
    .known { flex-direction: column; align-items: start; }
  }
</style>
