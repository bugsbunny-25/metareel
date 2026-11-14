<!-- "Log a watch" and "Edit watch" dialog. Shows need a season and episode (one episode per
     watch). The time goes out as a UTC instant with the browser's time zone (or as the start of
     a day, month or year when that's all you remember), so the server files it under the right
     local day. "Where" opens the fields for that kind of watch: a cinema, its screen, formats and
     tickets; a streaming service and plan; a disc. Picture and sound details are optional and
     tucked away. New watches carry a client key, so a double submit is harmless. An edit replaces
     the whole watch (PUT), so every field the watch has is sent back, not just the visible ones. -->
<script>
  import { api } from '../lib/api.js'
  import { library } from '../lib/library.svelte.js'
  import { scope } from '../lib/scope.svelte.js'
  import { prefs } from '../lib/prefs.svelte.js'
  import { toast } from '../lib/toast.svelte.js'
  import { localDay, fromUTC, zonedInput, zonedToDate, browserZone } from '../lib/dates.js'
  import { toMinor, fromMinor, currencyFor, remember, recall } from '../lib/money.js'
  import RatingInput from './RatingInput.svelte'
  import Poster from './Poster.svelte'
  import VenuePicker from './VenuePicker.svelte'
  import FormatPicker from './FormatPicker.svelte'

  /**
   * target: null when closed, else { kind: 'movie'|'tv', id, name, poster?, season?, episode?,
   *   watch?: an existing WatchEntry to edit, onsaved?: (entry) => void }
   */
  let { target = $bindable(null) } = $props()

  const PLACES = [['', 'Not saying'], ['theater', 'Cinema'], ['streaming', 'Streaming'], ['physical', 'Disc or file'], ['broadcast', 'TV broadcast'], ['other', 'Elsewhere']]
  const PRECISIONS = [['minute', 'Exact time'], ['day', 'Day'], ['month', 'Month'], ['year', 'Year']]
  const SCREENINGS = [['', 'Regular show'], ['preview', 'Preview or early access'], ['premiere', 'Premiere'], ['festival', 'Festival'], ['press', 'Press screening'], ['rerelease', 'Re-release'], ['marathon', 'Marathon']]
  const PLANS = [['flatrate', 'Subscription'], ['free', 'Free'], ['ads', 'Free with ads'], ['rent', 'Rented'], ['buy', 'Bought']]
  const MEDIA = [['', 'Not saying'], ['bluray_4k', '4K Blu-ray'], ['bluray', 'Blu-ray'], ['dvd', 'DVD'], ['vhs', 'VHS'], ['digital_file', 'Digital file'], ['35mm', '35mm print'], ['70mm', '70mm print']]
  const DIMENSIONS = [['', '–'], ['2d', '2D'], ['3d', '3D'], ['3d_hfr', '3D HFR']]
  const RESOLUTIONS = [['', '–'], ['sd', 'SD'], ['hd', 'HD 720p'], ['fhd', 'Full HD 1080p'], ['4k', '4K'], ['8k', '8K']]
  const HDRS = [['', '–'], ['none', 'No HDR'], ['hdr10', 'HDR10'], ['hdr10_plus', 'HDR10+'], ['dolby_vision', 'Dolby Vision'], ['hlg', 'HLG']]
  const DEVICES = ['TV', 'Projector', 'Laptop', 'Tablet', 'Phone']
  const AUDIO = ['Dolby Atmos', 'DTS:X', 'Dolby TrueHD', '7.1', '5.1', 'Stereo']
  const ASPECTS = ['1.43:1', '1.66:1', '1.78:1', '1.85:1', '1.90:1', '2.00:1', '2.20:1', '2.39:1', '2.76:1', '4:3']

  let dialog
  let busy = $state(false)
  let error = $state('')
  let known = $state([])
  let services = $state([])
  let key = ''
  // Editing: the diary list carries a slim watch, so the full one is fetched before Save is allowed
  // (a PUT replaces everything, and anything not sent back would be wiped).
  let loadingFull = $state(false)
  let incomplete = $state(false)
  // Every field, in one object so a reset is one assignment.
  let f = $state(blank())

  const editing = $derived(!!target?.watch)
  // Times are read and shown in the cinema's zone when there is one, else the zone the watch
  // was logged in (on edit), else this browser's. The same zone files the watch under a day.
  // An edit keeps the zone the watch was filed in unless you pick a different cinema, so an
  // untouched save never moves it.
  const zone = $derived((f.place === 'theater' && f.venue?.timezone && f.venue.id !== f.origVenueId && f.venue.timezone) || f.zone || browserZone())
  const awayZone = $derived(zone !== browserZone() ? zone : '')
  const money = $derived(f.place === 'theater' || (f.place === 'streaming' && (f.monetization === 'rent' || f.monetization === 'buy')))

  function blank() {
    return {
      season: 1, episode: 1,
      zone: browserZone(), origVenueId: null, precision: 'minute', minute: zonedInput(new Date(), browserZone()), day: localDay(), month: localDay().slice(0, 7), year: new Date().getFullYear(),
      rating: null, liked: false, notes: '', tagText: '',
      place: '', venue: null, screenId: null, seat: '', screening: '', eventName: '', withQA: false, formats: [],
      currency: '', tickets: '', price: '', fees: '', snacks: '', booking: '',
      provider: '', providerId: null, providerOriginal: '', monetization: 'flatrate', device: '', media: '', edition: '', channel: '',
      dimension: '', resolution: '', hdr: '', audioFormat: '', frameRate: '', audioLang: '', subLang: '',
      aspect: '', aspectVariable: false, screenAspect: '', screenW: '', screenH: '', dimUnit: prefs.values.screen_unit ?? 'm',
    }
  }

  function fromWatch(w) {
    const x = blank()
    const cur = w.currency ?? ''
    Object.assign(x, {
      season: w.season_number ?? 1, episode: w.episode_number ?? 1,
      zone: w.tz ?? browserZone(), origVenueId: w.venue_id ?? null, precision: w.time_precision ?? 'minute',
      minute: zonedInput(fromUTC(w.ended_at) ?? new Date(), w.tz ?? browserZone()),
      day: w.watched_on, month: w.watched_on.slice(0, 7), year: Number(w.watched_on.slice(0, 4)),
      rating: w.rating ?? null, liked: !!w.liked, notes: w.notes ?? '', tagText: (w.tags ?? []).join(', '),
      place: w.venue_kind ?? '',
      venue: w.venue_id ? { id: w.venue_id, name: w.venue_name_at_watch ?? 'Cinema', city: w.venue_city_at_watch ?? null } : null,
      screenId: w.screen_id ?? null, seat: w.seat ?? '', screening: w.screening_type === 'regular' ? '' : (w.screening_type ?? ''),
      eventName: w.event_name ?? '', withQA: !!w.with_qa, formats: (w.formats ?? []).map((ff) => ff.slug),
      currency: cur, tickets: w.tickets_count ?? '', price: fromMinor(w.price_minor, cur), fees: fromMinor(w.fees_minor, cur),
      snacks: fromMinor(w.concessions_minor, cur), booking: w.booking_source ?? '',
      monetization: w.monetization ?? 'flatrate', device: w.device ?? '', media: w.media ?? '', edition: w.edition ?? '',
      dimension: w.dimension ?? '', resolution: w.resolution ?? '', hdr: w.hdr ?? '', audioFormat: w.audio_format ?? '',
      frameRate: w.frame_rate ?? '', audioLang: w.audio_language ?? '', subLang: w.subtitle_language ?? '',
      aspect: w.content_aspect_ratio_text ?? '', aspectVariable: !!w.content_aspect_variable,
      screenAspect: w.screen_aspect_ratio ? String(w.screen_aspect_ratio) : '',
      screenW: w.screen_id ? '' : (w.screen_width ?? ''), screenH: w.screen_id ? '' : (w.screen_height ?? ''), dimUnit: w.screen_dim_unit ?? prefs.values.screen_unit ?? 'm',
    })
    // The service or channel is one stored name; which field shows it depends on the place.
    if (x.place === 'broadcast') x.channel = w.provider_name ?? ''
    else x.provider = w.provider_name ?? ''
    x.providerId = w.provider_id ?? null
    x.providerOriginal = w.provider_name ?? ''
    return x
  }

  const pictureSet = (x) => !!(x.dimension || x.resolution || x.hdr || x.audioFormat || x.frameRate || x.audioLang || x.subLang || x.aspect || x.screenAspect || x.screenW || x.screenH)
  let pictureOpen = $state(false)

  $effect(() => {
    if (!dialog) return
    if (target && !dialog.open) {
      const w = target.watch
      f = w ? fromWatch(w) : blank()
      if (!w) {
        f.season = target.season ?? 1
        f.episode = target.episode ?? 1
      }
      // Your saved currency, else the one you used last time, else the country's.
      if (!f.currency) f.currency = prefs.values.currency ?? recall('currency') ?? currencyFor(scope.country)
      pictureOpen = pictureSet(f)
      error = ''
      incomplete = false
      key = crypto.randomUUID()
      dialog.showModal()
      if (w) {
        loadingFull = true
        api.watch(w.id)
          .then((full) => {
            if (target?.watch?.id !== w.id) return
            f = fromWatch(full.watch ?? full)
            pictureOpen = pictureSet(f)
          })
          .catch((err) => {
            incomplete = true
            error = `Couldn’t load every detail of this watch (${err.message}), so it can’t be saved without losing some.`
          })
          .finally(() => (loadingFull = false))
      }
      api.tags().then((r) => (known = (r.tags ?? []).map((t) => t.name))).catch(() => {})
    } else if (!target && dialog.open) {
      dialog.close()
    }
  })
  // Streaming service names for the country, once per dialog, when streaming is picked.
  $effect(() => {
    if (f.place !== 'streaming' || services.length || !target) return
    api.services(scope.country).then((r) => (services = (r.services ?? []).map((s) => s.name).filter(Boolean))).catch(() => {})
  })

  const tags = () => [...new Set(f.tagText.split(',').map((t) => t.trim()).filter(Boolean))].slice(0, 20)
  const text = (v) => (String(v ?? '').trim() || null)
  const num = (v, name) => {
    const s = String(v ?? '').trim()
    if (!s) return null
    const n = Number(s)
    if (!Number.isFinite(n) || n <= 0) throw new Error(`${name} must be a positive number.`)
    return n
  }
  const minor = (v, name) => {
    const m = toMinor(v, f.currency)
    if (m === undefined) throw new Error(`${name} must be an amount like 12.50.`)
    return m
  }

  function endedAt() {
    // An imprecise watch is stored as the start of its day, month or year in your zone; noon
    // local keeps the instant inside the right day whatever the offset.
    switch (f.precision) {
      case 'day': return zonedToDate(`${f.day}T12:00`, zone).toISOString()
      case 'month': return zonedToDate(`${f.month}-01T12:00`, zone).toISOString()
      case 'year': return zonedToDate(`${f.year}-01-01T12:00`, zone).toISOString()
      default: return zonedToDate(f.minute, zone).toISOString()
    }
  }

  function payload() {
    const p = f.place
    const out = {
      tmdb_kind: target.kind, tmdb_id: target.id,
      ended_at: endedAt(), time_precision: f.precision, tz: zone,
      rating: f.rating, liked: f.liked, notes: text(f.notes), tags: tags(), venue_kind: p || null, formats: [],
      // Picture and sound apply wherever you watched.
      dimension: f.dimension || null, resolution: f.resolution || null, hdr: f.hdr || null,
      audio_format: text(f.audioFormat), frame_rate: num(f.frameRate, 'Frame rate'),
      audio_language: text(f.audioLang), subtitle_language: text(f.subLang),
      content_aspect_ratio: text(f.aspect), content_aspect_variable: f.aspectVariable,
      screen_aspect_ratio: text(f.screenAspect),
    }
    const w = num(f.screenW, 'Screen width'), h = num(f.screenH, 'Screen height')
    if (w || h) Object.assign(out, { screen_width: w, screen_height: h, screen_dim_unit: f.dimUnit })

    if (p === 'theater') {
      Object.assign(out, {
        venue_id: f.venue?.id ?? null, screen_id: f.venue ? f.screenId : null, seat: text(f.seat),
        screening_type: f.screening || null, event_name: f.screening ? text(f.eventName) : null, with_qa: f.withQA,
        formats: f.formats, booking_source: text(f.booking), tickets_count: num(f.tickets, 'Tickets'),
      })
    }
    if (p === 'streaming') {
      const name = text(f.provider)
      Object.assign(out, {
        provider_name: name, monetization: f.monetization || null, device: text(f.device),
        // Keep a stored service id only while the name it belongs to is unchanged.
        provider_id: name && name === f.providerOriginal ? f.providerId : null,
      })
    }
    if (p === 'physical') Object.assign(out, { media: f.media || null, edition: text(f.edition), device: text(f.device) })
    if (p === 'broadcast') Object.assign(out, { provider_name: text(f.channel), device: text(f.device) })
    if (money) {
      const amounts = {
        price_minor: minor(f.price, p === 'theater' ? 'Tickets cost' : 'Price'),
        fees_minor: p === 'theater' ? minor(f.fees, 'Fees') : null,
        concessions_minor: p === 'theater' ? minor(f.snacks, 'Snacks') : null,
      }
      Object.assign(out, amounts)
      if (Object.values(amounts).some((v) => v != null)) {
        if (!/^[A-Za-z]{3}$/.test(f.currency.trim())) throw new Error('Pick a currency for the amounts, e.g. USD.')
        out.currency = f.currency.trim().toUpperCase()
      }
    }
    if (target.kind === 'tv') Object.assign(out, { season_number: Number(f.season), episode_number: Number(f.episode) }) // ignored on edit
    return out
  }

  function rememberChoices(input) {
    if (input.currency) remember('currency', input.currency)
    if (f.place === 'theater' && f.venue) {
      const recent = recall('venues', []).filter((v) => v.id !== f.venue.id)
      remember('venues', [{ id: f.venue.id, name: f.venue.name, city: f.venue.city, timezone: f.venue.timezone ?? null }, ...recent].slice(0, 5))
    }
  }

  async function submit(e) {
    e.preventDefault()
    error = ''
    let input
    try {
      input = payload()
    } catch (err) {
      error = err.message
      return
    }
    busy = true
    try {
      const entry = editing ? await api.updateWatch(target.watch.id, input) : await api.logWatch({ ...input, client_key: key })
      rememberChoices(input)
      await library.refresh(target.kind, target.id).catch(() => {})
      const ep = target.kind === 'tv' ? ` S${f.season} E${f.episode}` : ''
      toast(editing ? `Saved your watch of ${target.name}${ep}.` : `Logged ${target.name}${ep}.`)
      target.onsaved?.(entry)
      target = null
    } catch (err) {
      error = err.message
    } finally {
      busy = false
    }
  }
</script>

<dialog bind:this={dialog} onclose={() => (target = null)} aria-labelledby="log-h">
  {#if target}
    <form onsubmit={submit}>
      <header>
        <Poster path={target.poster} seed={target.id} title={target.name} width={56} radius={6} size="w185" />
        <div>
          <p class="eyebrow">{editing ? 'Edit watch' : 'Log a watch'}</p>
          <h2 id="log-h">{target.name}</h2>
        </div>
      </header>

      <div class="body">
        {#if target.kind === 'tv' && editing}
          <!-- The server keeps a watch's title and episode on edit; to change them, delete and log again. -->
          <p class="fixed">Season {f.season} · Episode {f.episode}</p>
        {:else if target.kind === 'tv'}
          <div class="row two keep">
            <label>Season<input type="number" min="0" bind:value={f.season} required /></label>
            <label>Episode<input type="number" min="1" bind:value={f.episode} required /></label>
          </div>
        {/if}

        <section aria-labelledby="sec-when">
          <div class="sh">
            <h3 id="sec-when">When</h3>
            <div class="seg" role="group" aria-label="How precisely">
              {#each PRECISIONS as [v, l]}<button type="button" aria-pressed={f.precision === v} onclick={() => (f.precision = v)}>{l}</button>{/each}
            </div>
          </div>
          {#if f.precision === 'minute'}
            <label>Finished watching<input type="datetime-local" bind:value={f.minute} required max={zonedInput(new Date(), zone)} /></label>
          {:else if f.precision === 'day'}
            <label>Day<input type="date" bind:value={f.day} required max={localDay()} /></label>
          {:else if f.precision === 'month'}
            <label>Month<input type="month" bind:value={f.month} required max={localDay().slice(0, 7)} /></label>
          {:else}
            <label>Year<input type="number" bind:value={f.year} required min="1900" max={new Date().getFullYear()} /></label>
          {/if}
          {#if awayZone}<p class="hint">In {awayZone.replace(/_/g, ' ')} time{f.place === 'theater' && f.venue?.timezone ? ', the cinema’s' : ''}; the watch is filed under that day.</p>{/if}
        </section>

        <section aria-labelledby="sec-take">
          <h3 id="sec-take">Your take</h3>
          <div class="take">
            <RatingInput bind:value={f.rating} />
            <label class="check"><input type="checkbox" bind:checked={f.liked} /> Liked it</label>
          </div>
        </section>

        <section aria-labelledby="sec-where">
          <h3 id="sec-where">Where</h3>
          <div class="places" role="group" aria-label="Where you watched">
            {#each PLACES as [v, l]}<button type="button" class="chip" aria-pressed={f.place === v} onclick={() => (f.place = v)}>{l}</button>{/each}
          </div>

          {#if f.place === 'theater'}
            <div class="ctx">
              <div class="field"><span class="lbl">Cinema</span><VenuePicker bind:venue={f.venue} bind:screenId={f.screenId} country={scope.country} /></div>
              <div class="row two">
                <label>Seat<input bind:value={f.seat} placeholder="e.g. H12" maxlength="40" /></label>
                <label>Screening<select bind:value={f.screening}>{#each SCREENINGS as [v, l]}<option value={v}>{l}</option>{/each}</select></label>
              </div>
              {#if f.screening}
                <label>Event<input bind:value={f.eventName} placeholder="e.g. MAMI Film Festival" maxlength="120" /></label>
              {/if}
              <label class="check"><input type="checkbox" bind:checked={f.withQA} /> With a Q&amp;A or introduction</label>
              <div class="field"><span class="lbl">Formats</span><FormatPicker bind:selected={f.formats} /></div>
            </div>
          {:else if f.place === 'streaming'}
            <div class="ctx">
              <div class="row two">
                <label>Service
                  <input bind:value={f.provider} list="log-services" placeholder="e.g. Netflix" maxlength="80" />
                  <datalist id="log-services">{#each services as s}<option value={s}></option>{/each}</datalist>
                </label>
                <label>Plan<select bind:value={f.monetization}>{#each PLANS as [v, l]}<option value={v}>{l}</option>{/each}</select></label>
              </div>
              <label>Device<input bind:value={f.device} list="log-devices" placeholder="e.g. TV" maxlength="60" /></label>
            </div>
          {:else if f.place === 'physical'}
            <div class="ctx">
              <div class="row two">
                <label>Media<select bind:value={f.media}>{#each MEDIA as [v, l]}<option value={v}>{l}</option>{/each}</select></label>
                <label>Edition<input bind:value={f.edition} placeholder="e.g. Director's cut" maxlength="80" /></label>
              </div>
              <label>Device<input bind:value={f.device} list="log-devices" placeholder="e.g. Projector" maxlength="60" /></label>
            </div>
          {:else if f.place === 'broadcast'}
            <div class="ctx">
              <div class="row two">
                <label>Channel<input bind:value={f.channel} placeholder="e.g. BBC One" maxlength="80" /></label>
                <label>Device<input bind:value={f.device} list="log-devices" placeholder="e.g. TV" maxlength="60" /></label>
              </div>
            </div>
          {/if}
          <datalist id="log-devices">{#each DEVICES as d}<option value={d}></option>{/each}</datalist>

          {#if money}
            <fieldset class="money">
              <legend>{f.place === 'theater' ? 'Tickets' : f.monetization === 'rent' ? 'Rental' : 'Purchase'}</legend>
              <div class="row money-row" class:three={f.place === 'theater'}>
                {#if f.place === 'theater'}<label>Tickets<input type="number" min="1" bind:value={f.tickets} placeholder="1" /></label>{/if}
                <label>{f.place === 'theater' ? 'Tickets cost' : 'Price'}<input inputmode="decimal" bind:value={f.price} placeholder="0.00" /></label>
                <label>Currency<input bind:value={f.currency} maxlength="3" class="cc" aria-describedby="cur-hint" /></label>
              </div>
              {#if f.place === 'theater'}
                <div class="row two">
                  <label>Booking fees<input inputmode="decimal" bind:value={f.fees} placeholder="0.00" /></label>
                  <label>Snacks<input inputmode="decimal" bind:value={f.snacks} placeholder="0.00" /></label>
                </div>
                <label>Booked through<input bind:value={f.booking} placeholder="e.g. Box office, BookMyShow" maxlength="80" /></label>
              {/if}
              <p class="hint" id="cur-hint">Totals, in one currency. Stats never convert between currencies.</p>
            </fieldset>
          {/if}
        </section>

        <details class="picture" bind:open={pictureOpen}>
          <summary>Picture and sound <span class="hint">optional</span></summary>
          <div class="ctx">
            <div class="row three">
              <label>3D<select bind:value={f.dimension}>{#each DIMENSIONS as [v, l]}<option value={v}>{l}</option>{/each}</select></label>
              <label>Resolution<select bind:value={f.resolution}>{#each RESOLUTIONS as [v, l]}<option value={v}>{l}</option>{/each}</select></label>
              <label>HDR<select bind:value={f.hdr}>{#each HDRS as [v, l]}<option value={v}>{l}</option>{/each}</select></label>
            </div>
            <div class="row two">
              <label>Audio<input bind:value={f.audioFormat} list="log-audio" placeholder="e.g. Dolby Atmos" maxlength="60" /></label>
              <label>Frame rate<input inputmode="decimal" bind:value={f.frameRate} placeholder="e.g. 24" /></label>
            </div>
            <datalist id="log-audio">{#each AUDIO as a}<option value={a}></option>{/each}</datalist>
            <div class="row two">
              <label>Audio language<input bind:value={f.audioLang} placeholder="e.g. Hindi" maxlength="40" /></label>
              <label>Subtitles<input bind:value={f.subLang} placeholder="e.g. English" maxlength="40" /></label>
            </div>
            <div class="row two">
              <label>Film's aspect ratio<input bind:value={f.aspect} list="log-aspects" placeholder="e.g. 2.39:1" maxlength="12" /></label>
              <label>Screen's aspect ratio<input bind:value={f.screenAspect} list="log-aspects" placeholder="e.g. 1.90:1" maxlength="12" /></label>
            </div>
            <datalist id="log-aspects">{#each ASPECTS as a}<option value={a}></option>{/each}</datalist>
            <label class="check"><input type="checkbox" bind:checked={f.aspectVariable} /> The aspect ratio changes during the film</label>
            <div class="row size">
              <label>Screen width<input inputmode="decimal" bind:value={f.screenW} /></label>
              <label>Height<input inputmode="decimal" bind:value={f.screenH} /></label>
              <label>Unit<select bind:value={f.dimUnit}><option value="m">m</option><option value="ft">ft</option></select></label>
            </div>
            {#if f.place === 'theater' && f.screenId}<p class="hint">Left empty, the screen's own size and shape are used.</p>{/if}
          </div>
        </details>

        <label>Tags <span class="muted">(comma separated)</span>
          <input bind:value={f.tagText} list="known-tags" placeholder="with friends, rainy day" maxlength="600" />
          <datalist id="known-tags">{#each known as t}<option value={t}></option>{/each}</datalist>
        </label>
        <label>Notes <span class="muted">(only you see these)</span><textarea rows="3" bind:value={f.notes} maxlength="2000"></textarea></label>
      </div>

      <footer>
        {#if error}<p class="err" role="alert">{error}</p>{/if}
        <button type="button" class="btn-secondary" onclick={() => (target = null)}>Cancel</button>
        <button type="submit" class="btn-primary" disabled={busy || loadingFull || incomplete}>{busy ? 'Saving…' : loadingFull ? 'Loading…' : editing ? 'Save changes' : 'Log watch'}</button>
      </footer>
    </form>
  {/if}
</dialog>

<style>
  dialog { width: min(600px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); padding: 0; border: 0; border-radius: 20px; background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 30px 80px -20px rgb(0 0 0 / 0.6); overflow: hidden; }
  dialog[open] { display: flex; }
  dialog::backdrop { background: rgb(0 0 0 / 0.55); }
  form { display: flex; flex-direction: column; width: 100%; max-height: calc(100dvh - 32px); }
  header { display: flex; gap: 14px; align-items: center; padding: 20px 22px 14px; border-bottom: 1px solid var(--border-subtle); }
  .body { display: grid; gap: 18px; padding: 16px 22px 20px; overflow-y: auto; overscroll-behavior: contain; }
  .eyebrow { margin: 0 0 2px; color: var(--text-muted); font-size: 12px; font-weight: 600; letter-spacing: 0.05em; text-transform: uppercase; }
  h2 { margin: 0; font: 700 22px/1.15 var(--font-display); }
  section { display: grid; gap: 10px; }
  h3 { margin: 0; font: 700 16px/1.2 var(--font-display); }
  .sh { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; }
  label, .field { display: grid; gap: 6px; font-size: 13px; font-weight: 600; color: var(--text-muted); }
  .lbl { font-size: 13px; }
  .row { display: grid; gap: 12px; }
  .row.two { grid-template-columns: 1fr 1fr; }
  .row.three { grid-template-columns: repeat(3, 1fr); }
  .money-row { grid-template-columns: 1fr 96px; }
  .money-row.three { grid-template-columns: 90px 1fr 96px; }
  .row.size { grid-template-columns: 1fr 1fr 80px; }
  input, textarea, select {
    min-width: 0; padding: 9px 12px; border: 1px solid var(--border-subtle); border-radius: 10px; background: var(--bg-canvas);
    color: var(--text-primary); font: 400 15px/1.3 var(--font-sans);
  }
  input:focus-visible, textarea:focus-visible, select:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 0; }
  .cc { text-transform: uppercase; }
  .check { display: flex; align-items: center; gap: 8px; color: var(--text-primary); font-weight: 400; font-size: 14px; }
  .check input { width: 16px; height: 16px; accent-color: var(--accent-yours); }
  .take { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 10px 20px; }
  .seg { display: inline-flex; padding: 2px; border-radius: 9px; background: var(--bg-elevated); }
  .seg button { height: 28px; padding: 0 11px; border: 0; border-radius: 7px; background: none; color: var(--text-muted); font: 600 13px/1 var(--font-sans); cursor: pointer; }
  .seg button[aria-pressed='true'] { background: var(--bg-surface); color: var(--text-primary); box-shadow: 0 1px 2px rgb(0 0 0 / 0.25); }
  .places { display: flex; flex-wrap: wrap; gap: 6px; }
  .chip { height: 32px; padding: 0 13px; border: 1px solid var(--border-subtle); border-radius: 16px; background: none; color: var(--text-primary); font: 500 14px/1 var(--font-sans); cursor: pointer; }
  .chip:hover { border-color: var(--border-strong); }
  .chip[aria-pressed='true'] { border-color: var(--accent-primary); background: var(--accent-primary); color: var(--accent-on); font-weight: 600; }
  .ctx { display: grid; gap: 12px; padding: 14px; border-radius: 14px; background: var(--bg-canvas); }
  .ctx input, .ctx select, .ctx :global(input), .ctx :global(select) { background: var(--bg-surface); }
  .money { display: grid; gap: 10px; margin: 0; padding: 14px; border: 1px solid var(--border-subtle); border-radius: 14px; }
  legend { padding: 0 4px; color: var(--text-primary); font: 700 14px/1 var(--font-display); }
  .picture summary { display: flex; align-items: baseline; gap: 8px; padding: 4px 0; font: 700 16px/1.2 var(--font-display); cursor: pointer; list-style: none; }
  .picture summary::-webkit-details-marker { display: none; }
  .picture summary::before { content: '›'; display: inline-block; width: 12px; color: var(--text-muted); transition: transform 0.15s ease; }
  .picture[open] summary::before { transform: rotate(90deg); }
  .picture .ctx { margin-top: 8px; }
  .hint { margin: 0; color: var(--text-muted); font: 400 12.5px/1.4 var(--font-sans); }
  .muted { font-weight: 400; }
  .err { flex: 1 1 100%; margin: 0; color: var(--delta-down); font-size: 13.5px; }
  .fixed { margin: -4px 0 0; color: var(--text-muted); font-weight: 600; }
  footer { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 10px; padding: 14px 22px; border-top: 1px solid var(--border-subtle); background: var(--bg-surface); }
  @media (max-width: 480px) {
    .row.two:not(.keep), .row.three { grid-template-columns: 1fr; }
    .money-row.three { grid-template-columns: 70px 1fr 80px; }
    .body { padding: 14px 16px 18px; }
    header, footer { padding-inline: 16px; }
  }
</style>
