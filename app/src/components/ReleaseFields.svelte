<!-- The fields of a release: what is printed on it, the TMDB titles on it, each disc's specs,
     the special features and, for a box set, its parts (each a release of its own, drawn
     with this same component one level down). Edits the bound object in place. -->
<script>
  import ReleaseFields from './ReleaseFields.svelte'
  import FormatBadge from './FormatBadge.svelte'
  import Poster from './Poster.svelte'
  import { api } from '../lib/api.js'
  import { loadFormats } from '../lib/formats.js'
  import { MEDIA_FORMATS, DISC_FORMATS, PACKAGINGS, CAPACITIES, CODECS, RESOLUTIONS, HDR, AUDIO, blankDisc, blankRelease } from '../lib/discs.js'

  let { release = $bindable(), depth = 0, idp = 'r' } = $props()

  let labels = $state([])
  $effect(() => {
    if (depth === 0) loadFormats().then((all) => (labels = all.filter((f) => f.category === 'label'))).catch(() => {})
  })

  // Titles: search TMDB, or pull a whole collection in for a box set.
  let q = $state('')
  let hits = $state([])
  let searching = $state(false)
  let searchErr = $state('')
  let collections = $state([])
  async function search(kind) {
    const query = q.trim()
    if (!query) return
    searching = true
    searchErr = ''
    try {
      if (kind === 'collection') {
        hits = []
        collections = (await api.discCollections(query)).results
        if (!collections.length) searchErr = 'No TMDB collection by that name.'
      } else {
        collections = []
        hits = (await api.searchTMDB(query)).results ?? []
        if (!hits.length) searchErr = 'Nothing on TMDB by that name.'
      }
    } catch (err) {
      searchErr = err.message
    } finally {
      searching = false
    }
  }
  const has = (t) => release.titles.some((x) => x.tmdb_kind === t.tmdb_kind && x.tmdb_id === t.tmdb_id && (x.season_number ?? null) === (t.season_number ?? null))
  function addTitle(t) {
    const ref = { tmdb_kind: t.tmdb_kind, tmdb_id: t.tmdb_id, title: t.title, year: t.year, poster_path: t.poster_path }
    if (!has(ref)) release.titles.push(ref)
  }
  function addCollection(c) {
    for (const p of c.parts) addTitle(p)
    collections = []
    q = ''
  }
  function onSearchKey(e) {
    if (e.key === 'Enter') {
      e.preventDefault()
      search('')
    }
  }

  // Special features: one per line, "Title — 12 min" or "Title — 12 min — disc 2".
  const featureText = (fs) => fs.map((f) => [f.title, f.minutes ? `${f.minutes} min` : '', f.disc ? `disc ${f.disc}` : ''].filter(Boolean).join(' — ')).join('\n')
  let features = $state(featureText(release.special_features))
  function parseFeatures(text) {
    release.special_features = text.split('\n').map((l) => l.trim()).filter(Boolean).map((line) => {
      const f = { title: line, minutes: 0, disc: 0 }
      const disc = line.match(/\s*[—–-]\s*disc\s*(\d+)\s*$/i)
      if (disc) (f.disc = +disc[1]), (line = line.slice(0, disc.index))
      const min = line.match(/\s*[—–-]\s*(\d+)\s*min(utes)?\s*$/i)
      if (min) (f.minutes = +min[1]), (line = line.slice(0, min.index))
      f.title = line.trim()
      return f
    })
  }

  const subsText = (d) => d.subtitles.join(', ')
  const setSubs = (d, text) => (d.subtitles = text.split(',').map((s) => s.trim()).filter(Boolean))
  function toggleHDR(d, h) {
    d.hdr = d.hdr.includes(h) ? d.hdr.filter((x) => x !== h) : [...d.hdr, h]
  }
  // Which disc panels are open: a new disc opens, stored ones start closed. Kept apart
  // from the disc so filling a field never folds the panel away.
  let openDisc = $state({})
  function addDisc() {
    release.discs.push(blankDisc(release.media_format === 'other' ? 'bluray' : release.media_format))
    openDisc[release.discs.length - 1] = true
  }
  function removeDisc(i) {
    release.discs.splice(i, 1)
    openDisc = {}
  }
  let openPart = $state({})
  function addPart() {
    release.parts.push(blankRelease({ media_format: release.media_format }))
    openPart[release.parts.length - 1] = true
  }
  const titleName = (t) => `${t.title ?? t.tmdb_id}${t.year ? ` (${t.year})` : ''}${t.season_number != null ? ` · Season ${t.season_number}` : ''}`
</script>

<div class="fields">
  <div class="row two">
    <label>Title as printed<input bind:value={release.title} required maxlength="300" placeholder="The Dark Knight Trilogy" /></label>
    <label>Edition<input bind:value={release.edition} maxlength="120" placeholder="Steelbook, Collector’s Edition…" /></label>
  </div>
  <div class="row three">
    <label>Format<select bind:value={release.media_format}>{#each MEDIA_FORMATS as [v, l]}<option value={v}>{l}</option>{/each}</select></label>
    <label>Packaging<select bind:value={release.packaging}>{#each PACKAGINGS as [v, l]}<option value={v}>{l}</option>{/each}</select></label>
    <label class="check inline"><input type="checkbox" bind:checked={release.has_slipcover} />Slipcover</label>
  </div>
  <div class="row three">
    <label>Barcode<input bind:value={release.barcode} inputmode="numeric" maxlength="20" placeholder="UPC / EAN" /></label>
    <label>Region<input bind:value={release.region} maxlength="12" placeholder="A, B, free…" /></label>
    <label>Release date<input type="date" bind:value={release.release_date} /></label>
  </div>
  {#if depth === 0}
    <div class="row three">
      <label>Studio / distributor<input bind:value={release.studio} maxlength="120" /></label>
      <label>Country<input class="cc" bind:value={release.country} maxlength="2" placeholder="US" /></label>
      <label>Label
        <select bind:value={release.label}>
          <option value="">—</option>
          {#each labels as f}<option value={f.slug}>{f.label}</option>{/each}
        </select>
      </label>
    </div>
    {#if release.label && labels.find((f) => f.slug === release.label)}<span class="plate"><FormatBadge format={labels.find((f) => f.slug === release.label)} height={14} /></span>{/if}
  {/if}

  <section class="sub" aria-labelledby="{idp}-titles">
    <h4 id="{idp}-titles">What’s on it <span class="muted">· linked to TMDB</span></h4>
    {#if release.titles.length}
      <ul class="titles">
        {#each release.titles as t, i (t.tmdb_kind + t.tmdb_id + (t.season_number ?? ''))}
          <li>
            <Poster path={t.poster_path} seed={t.tmdb_id} title={t.title ?? ''} width={28} radius={3} size="w92" />
            <span class="tn">{titleName(t)}</span>
            {#if t.tmdb_kind === 'tv'}
              <label class="season">Season<input type="number" min="0" max="1000" value={t.season_number ?? ''} oninput={(e) => (t.season_number = e.currentTarget.value === '' ? undefined : +e.currentTarget.value)} placeholder="all" /></label>
            {/if}
            <button type="button" class="x" aria-label="Remove {t.title}" onclick={() => release.titles.splice(i, 1)}>×</button>
          </li>
        {/each}
      </ul>
    {/if}
    <div class="search">
      <input bind:value={q} placeholder="Search TMDB for a film or show" onkeydown={onSearchKey} aria-label="Search TMDB" />
      <button type="button" class="btn-secondary sm" disabled={searching} onclick={() => search('')}>Search</button>
      {#if depth === 0}<button type="button" class="btn-secondary sm" disabled={searching} onclick={() => search('collection')} title="Find a TMDB collection and add all its films">Collection</button>{/if}
    </div>
    {#if searchErr}<p class="hint">{searchErr}</p>{/if}
    {#if hits.length}
      <ul class="hits">
        {#each hits.slice(0, 8) as h (h.tmdb_kind + h.tmdb_id)}
          <li><button type="button" disabled={has(h)} onclick={() => addTitle(h)}>
            <Poster path={h.poster_path} seed={h.tmdb_id} title={h.title} width={24} radius={3} size="w92" />
            <span>{h.title}{h.year ? ` (${h.year})` : ''} <span class="muted">{h.tmdb_kind === 'tv' ? 'Series' : 'Film'}</span></span>
          </button></li>
        {/each}
      </ul>
    {/if}
    {#each collections as c (c.id)}
      <div class="collection">
        <span><b>{c.name}</b> <span class="muted">· {c.parts.length} films: {c.parts.map((p) => p.title).join(', ')}</span></span>
        <button type="button" class="btn-secondary sm" onclick={() => addCollection(c)}>Add all</button>
      </div>
    {/each}
  </section>

  <section class="sub" aria-labelledby="{idp}-discs">
    <div class="sh"><h4 id="{idp}-discs">Discs <span class="muted">· {release.discs.length}</span></h4><button type="button" class="btn-secondary sm" onclick={addDisc}>Add disc</button></div>
    {#each release.discs as d, i}
      <details class="disc" open={!!openDisc[i]} ontoggle={(e) => (openDisc[i] = e.currentTarget.open)}>
        <summary><b>Disc {i + 1}</b> <span class="muted">{DISC_FORMATS.find(([v]) => v === d.format)?.[1]}{d.name ? ` · ${d.name}` : ''}</span></summary>
        <div class="disc-body">
          <div class="row three">
            <label>Format<select bind:value={d.format}>{#each DISC_FORMATS as [v, l]}<option value={v}>{l}</option>{/each}</select></label>
            <label>Disc type<select bind:value={d.capacity}>{#each CAPACITIES as [v, l]}<option value={v}>{l}</option>{/each}</select></label>
            <label>Name<input bind:value={d.name} maxlength="120" placeholder="Feature film, Bonus disc" /></label>
          </div>
          {#if release.titles.length > 1}
            <label>Holds
              <select value={d.title_index ?? ''} onchange={(e) => (d.title_index = e.currentTarget.value === '' ? undefined : +e.currentTarget.value)}>
                <option value="">—</option>
                {#each release.titles as t, ti}<option value={ti}>{titleName(t)}</option>{/each}
              </select>
            </label>
          {/if}
          <div class="row three">
            <label>Video codec<select bind:value={d.video_codec}>{#each CODECS as [v, l]}<option value={v}>{l}</option>{/each}</select></label>
            <label>Resolution<select bind:value={d.resolution}>{#each RESOLUTIONS as [v, l]}<option value={v}>{l}</option>{/each}</select></label>
            <label>Aspect ratio<input bind:value={d.aspect_ratio} placeholder="2.39:1" maxlength="8" /></label>
          </div>
          <div class="chips" role="group" aria-label="HDR">
            {#each HDR as [v, l]}<button type="button" class="pick" aria-pressed={d.hdr.includes(v)} onclick={() => toggleHDR(d, v)}>{l}</button>{/each}
            <label class="check"><input type="checkbox" bind:checked={d.variable_aspect} />Aspect ratio varies (IMAX)</label>
          </div>
          <div class="row three">
            <label>Region<input bind:value={d.region} maxlength="12" placeholder="A / free" /></label>
            <label>Runtime (min)<input type="number" min="0" max="2000" bind:value={d.runtime_minutes} /></label>
            {#if release.titles.some((t) => t.tmdb_kind === 'tv')}
              <label>Season<input type="number" min="0" value={d.season_number ?? ''} oninput={(e) => (d.season_number = e.currentTarget.value === '' ? undefined : +e.currentTarget.value)} /></label>
            {/if}
          </div>
          <div class="tracks">
            <span class="lbl">Audio</span>
            {#each d.audio as a, ai}
              <div class="track">
                <input bind:value={a.language} placeholder="English" maxlength="40" aria-label="Language" />
                <select bind:value={a.format} aria-label="Audio format">{#each AUDIO as [v, l]}<option value={v}>{l}</option>{/each}</select>
                <input class="ch" bind:value={a.channels} placeholder="7.1" maxlength="8" aria-label="Channels" />
                <input bind:value={a.description} placeholder="Commentary…" maxlength="200" aria-label="Description" />
                <button type="button" class="x" aria-label="Remove track" onclick={() => d.audio.splice(ai, 1)}>×</button>
              </div>
            {/each}
            <button type="button" class="link" onclick={() => d.audio.push({ language: d.audio.at(-1)?.language ?? 'English', format: 'dolby_truehd', channels: '', description: '' })}>+ Add audio track</button>
          </div>
          <label>Subtitles <span class="muted">(comma separated)</span><input value={subsText(d)} onchange={(e) => setSubs(d, e.currentTarget.value)} placeholder="English SDH, French, Spanish" /></label>
          <label>Notes<input bind:value={d.notes} maxlength="2000" /></label>
          <button type="button" class="link danger" onclick={() => removeDisc(i)}>Remove disc {i + 1}</button>
        </div>
      </details>
    {/each}
  </section>

  <section class="sub" aria-labelledby="{idp}-features">
    <h4 id="{idp}-features">Special features</h4>
    <textarea rows="4" bind:value={features} onchange={() => parseFeatures(features)} placeholder={'One per line, e.g.\nGotham Uncovered — 64 min — disc 2\nCommentary with the director'}></textarea>
  </section>

  <label>Notes<textarea rows="2" bind:value={release.notes} maxlength="4000"></textarea></label>

  {#if depth === 0}
    <section class="sub" aria-labelledby="{idp}-parts">
      <div class="sh">
        <h4 id="{idp}-parts">Box set parts <span class="muted">· {release.parts.length || 'none'}</span></h4>
        <button type="button" class="btn-secondary sm" onclick={addPart}>Add part</button>
      </div>
      <p class="hint">For a box set whose pieces are cases of their own (three Steelbooks in an outer box), add each as a part with its own discs and pictures. A simple multi-film set only needs its titles above.</p>
      {#each release.parts as p, i}
        <details class="part" open={!!openPart[i]} ontoggle={(e) => (openPart[i] = e.currentTarget.open)}>
          <summary><b>Part {i + 1}</b> <span class="muted">{p.title || 'untitled'}</span></summary>
          <ReleaseFields bind:release={release.parts[i]} depth={1} idp="{idp}-p{i}" />
          <button type="button" class="link danger" onclick={() => { release.parts.splice(i, 1); openPart = {} }}>Remove part {i + 1}</button>
        </details>
      {/each}
    </section>
  {/if}
</div>

<style>
  .fields { display: grid; gap: 14px; }
  .row { display: grid; gap: 12px; }
  .row.two { grid-template-columns: 1fr 1fr; }
  .row.three { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  label { display: grid; gap: 6px; font-size: 13px; font-weight: 600; color: var(--text-muted); }
  input, textarea, select { min-width: 0; padding: 9px 12px; border: 1px solid var(--border-subtle); border-radius: 10px; background: var(--bg-canvas); color: var(--text-primary); font: 400 15px/1.3 var(--font-sans); }
  textarea { resize: vertical; }
  input:focus-visible, textarea:focus-visible, select:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 0; }
  .cc { text-transform: uppercase; }
  .check { display: flex; align-items: center; gap: 8px; color: var(--text-primary); font-weight: 400; font-size: 14px; }
  .check.inline { align-self: end; height: 40px; }
  .check input { width: 16px; height: 16px; accent-color: var(--accent-yours); }
  .plate { justify-self: start; display: inline-flex; padding: 5px 8px; border-radius: 8px; background: #161618; }
  .sub { display: grid; gap: 10px; padding: 14px; border-radius: 14px; background: var(--bg-canvas); }
  .sub input, .sub select, .sub textarea { background: var(--bg-surface); }
  h4 { margin: 0; font: 700 15px/1.2 var(--font-display); color: var(--text-primary); }
  .muted { color: var(--text-muted); font-weight: 400; }
  .sh { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
  .hint { margin: 0; color: var(--text-muted); font-size: 12.5px; line-height: 1.4; }
  .titles, .hits { display: grid; gap: 6px; margin: 0; padding: 0; list-style: none; }
  .titles li { display: flex; align-items: center; gap: 10px; }
  .tn { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-primary); font-weight: 600; }
  .season { display: flex; align-items: center; gap: 6px; font-weight: 400; }
  .season input { width: 72px; padding: 5px 8px; }
  .search { display: flex; gap: 8px; }
  .search input { flex: 1; }
  .hits button { display: flex; align-items: center; gap: 10px; width: 100%; padding: 6px 8px; border: 0; border-radius: 8px; background: none; color: var(--text-primary); font: 500 14px/1.2 var(--font-sans); text-align: left; cursor: pointer; }
  .hits button:hover:not(:disabled) { background: var(--bg-elevated); }
  .hits button:disabled { opacity: 0.45; cursor: default; }
  .collection { display: flex; align-items: center; justify-content: space-between; gap: 12px; font-size: 14px; }
  .disc, .part { border-radius: 12px; background: var(--bg-surface); }
  .part { padding: 0 12px 12px; background: var(--bg-surface); }
  .part :global(.sub) { background: var(--bg-canvas); }
  summary { padding: 10px 12px; cursor: pointer; font-size: 14px; color: var(--text-primary); }
  .part summary { padding-inline: 0; }
  .disc-body { display: grid; gap: 12px; padding: 0 12px 12px; }
  .disc-body input, .disc-body select { background: var(--bg-canvas); }
  .chips { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 8px; }
  .pick { height: 30px; padding: 0 11px; border: 1px solid var(--border-subtle); border-radius: 15px; background: var(--bg-surface); color: var(--text-primary); font: 500 13px/1 var(--font-sans); cursor: pointer; }
  .pick[aria-pressed='true'] { border-color: var(--accent-yours); background: color-mix(in srgb, var(--accent-yours) 16%, var(--bg-surface)); font-weight: 700; }
  .tracks { display: grid; gap: 6px; }
  .lbl { font-size: 13px; font-weight: 600; color: var(--text-muted); }
  .track { display: grid; grid-template-columns: 1fr 1.4fr 64px 1.4fr 28px; gap: 6px; }
  .track input, .track select { padding: 7px 9px; font-size: 14px; }
  .x { width: 28px; height: 28px; border: 0; border-radius: 50%; background: var(--bg-elevated); color: var(--text-muted); font-size: 16px; line-height: 1; cursor: pointer; align-self: center; }
  .x:hover { color: var(--text-primary); }
  .link { justify-self: start; padding: 0; border: 0; background: none; color: var(--text-primary); font: 600 13.5px/1.3 var(--font-sans); text-decoration: underline; text-underline-offset: 3px; cursor: pointer; }
  .link.danger { color: var(--delta-down); }
  .sm { height: 32px; padding: 0 12px; font-size: 13.5px; }
  @media (max-width: 560px) {
    .row.two, .row.three { grid-template-columns: 1fr; }
    .track { grid-template-columns: 1fr 1fr; }
    .track .x { justify-self: end; }
  }
</style>
