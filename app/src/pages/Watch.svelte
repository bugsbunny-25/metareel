<!-- One watch (#/watch/:id), from /me/watches/:id: what, when (in the zone it was filed in, as
     precisely as it was logged), where, and every detail recorded with it, formats and discs as
     their logos. Edit opens the log dialog; Delete asks first and goes back to the diary. -->
<script>
  import Poster from '../components/Poster.svelte'
  import Icon from '../components/Icon.svelte'
  import FormatBadge from '../components/FormatBadge.svelte'
  import { api } from '../lib/api.js'
  import { library } from '../lib/library.svelte.js'
  import { navigate } from '../lib/router.svelte.js'
  import { stars, starsLabel } from '../lib/rows.js'
  import { fromUTC } from '../lib/dates.js'
  import { epCode } from '../lib/shows.js'
  import { formatMoney } from '../lib/money.js'
  import { toast } from '../lib/toast.svelte.js'
  import { SCREENINGS, PLANS, OTHER_MEDIA, DIMENSIONS, RESOLUTIONS, HDRS, labelOf } from '../lib/watch-fields.js'

  let { id, onlog, onconfirm } = $props()

  let w = $state(null)
  let meta = $state(null)
  let subs = $state([])
  let error = $state('')
  let nonce = $state(0)

  $effect(() => {
    nonce
    const ctrl = new AbortController()
    api.watch(id, ctrl.signal)
      .then((r) => {
        w = r.watch ?? r
        error = ''
        // One watch carries no title or poster; the library has them, else ask for the title.
        const t = library.get(w.tmdb_kind, w.tmdb_id)
        if (t) meta = { title: t.title, poster: t.poster_path, release: t.release_date }
        else api.myTitle(w.tmdb_kind, w.tmdb_id, ctrl.signal).then((m) => (meta = { title: m.title?.title, poster: m.title?.poster_path, release: m.title?.release_date })).catch(() => {})
      })
      .catch((err) => { if (err.name !== 'AbortError') error = err.status === 404 ? 'This watch doesn’t exist any more.' : err.message })
    api.subscriptions(ctrl.signal).then((r) => (subs = r.subscriptions ?? [])).catch(() => {})
    return () => ctrl.abort()
  })

  const title = $derived(w?.title ?? meta?.title ?? '')
  const poster = $derived(w?.poster_path ?? meta?.poster ?? null)
  const episode = $derived(w?.subject_kind === 'episode')
  const titleHref = $derived(!w ? '' : episode ? `#/episode/${w.tmdb_id}/${w.season_number}/${w.episode_number}` : `#/title/${w.tmdb_kind}/${w.tmdb_id}`)

  function watchedText(e) {
    if (e.time_precision === 'year') return e.watched_on.slice(0, 4)
    if (e.time_precision === 'month') return new Date(`${e.watched_on}T12:00:00`).toLocaleDateString(undefined, { month: 'long', year: 'numeric' })
    if (e.time_precision === 'day') return new Date(`${e.watched_on}T12:00:00`).toLocaleDateString(undefined, { dateStyle: 'full' })
    const d = fromUTC(e.ended_at)
    if (!d) return ''
    const opts = { dateStyle: 'full', timeStyle: 'short' }
    try {
      return d.toLocaleString(undefined, e.tz ? { ...opts, timeZone: e.tz, timeZoneName: e.tz !== Intl.DateTimeFormat().resolvedOptions().timeZone ? 'short' : undefined } : opts)
    } catch {
      return d.toLocaleString(undefined, opts)
    }
  }

  const rows = $derived.by(() => {
    if (!w) return []
    const money = (m) => (m != null ? formatMoney(m, w.currency) : '')
    const plan = w.subscription_id ? subs.find((s) => s.id === w.subscription_id)?.plan ?? 'A cinema plan' : null
    const place = [w.venue_name_at_watch, w.venue_city_at_watch].filter(Boolean).join(', ') || (w.venue_kind === 'physical' ? 'At home' : w.venue_kind === 'other' ? 'Elsewhere' : '')
    const formats = w.formats ?? []
    const screening = w.screening_type && w.screening_type !== 'regular' ? [labelOf(SCREENINGS, w.screening_type), w.event_name].filter(Boolean).join(' · ') : ''
    const picture = [labelOf(DIMENSIONS, w.dimension), labelOf(RESOLUTIONS, w.resolution), w.hdr && w.hdr !== 'none' ? labelOf(HDRS, w.hdr) : '', w.frame_rate ? `${w.frame_rate} fps` : ''].filter(Boolean).join(' · ')
    const aspect = [w.content_aspect_ratio_text, w.content_aspect_variable ? 'variable' : ''].filter(Boolean).join(', ')
    const languages = [w.audio_language, w.subtitle_language ? `${w.subtitle_language} subtitles` : ''].filter(Boolean).join(' · ')
    const screen = w.screen_width || w.screen_height ? [w.screen_width && w.screen_height ? `${w.screen_width} × ${w.screen_height} ${w.screen_dim_unit ?? ''}` : '', w.screen_aspect_ratio ? `${w.screen_aspect_ratio}:1` : ''].filter(Boolean).join(' · ') : w.screen_aspect_ratio ? `${w.screen_aspect_ratio}:1` : ''
    return [
      ['Watched', watchedText(w)],
      w.rating && ['Rating', stars(w.rating), 'accent', starsLabel(w.rating)],
      w.liked && ['Liked', '♥', 'accent'],
      w.is_rewatch && ['Rewatch', `Watch ${w.watch_number}`],
      place && ['Where', place],
      w.screen_name_at_watch && ['Screen', w.screen_name_at_watch],
      w.seat && ['Seat', w.seat],
      screening && ['Screening', screening],
      w.with_qa && ['Q&A', 'With a Q&A or introduction'],
      ['Format', formats.filter((f) => f.category !== 'disc' && f.category !== 'label'), 'logos'],
      w.provider_name && [w.venue_kind === 'broadcast' ? 'Channel' : 'Service', w.provider_name],
      w.monetization && ['Plan', labelOf(PLANS, w.monetization)],
      ['Disc', formats.filter((f) => f.category === 'disc'), 'named'],
      !formats.some((f) => f.category === 'disc') && w.media && ['Media', labelOf(OTHER_MEDIA, w.media)],
      ['Label', formats.filter((f) => f.category === 'label'), 'named'],
      w.edition && ['Edition', w.edition],
      w.device && ['Device', w.device],
      plan && ['Covered by', plan, 'accent'],
      w.face_value_minor != null && ['Face value', money(w.face_value_minor)],
      w.price_minor != null && [plan ? 'Paid on top' : 'Paid', money(w.price_minor)],
      w.fees_minor && ['Booking fees', money(w.fees_minor)],
      w.concessions_minor && ['Snacks', money(w.concessions_minor)],
      w.tickets_count > 1 && ['Tickets', String(w.tickets_count)],
      w.booking_source && ['Booked through', w.booking_source],
      picture && ['Picture', picture],
      w.audio_format && ['Sound', w.audio_format],
      languages && ['Language', languages],
      aspect && ['Aspect ratio', aspect],
      screen && ['Screen size', screen],
      w.tags?.length && ['Tags', w.tags.map((t) => `#${t}`).join(' ')],
    ].filter((r) => r && (!Array.isArray(r[1]) || r[1].length))
  })

  function edit() {
    onlog({ kind: w.tmdb_kind, id: w.tmdb_id, name: title, poster, watch: w, onsaved: () => nonce++ })
  }
  function remove() {
    onconfirm({
      title: 'Delete this watch?',
      body: `${title}${episode ? ` ${epCode(w.season_number, w.episode_number)}` : ''}, ${watchedText(w)}. This can’t be undone; the title’s rating and watch count are recalculated.`,
      confirm: 'Delete watch',
      danger: true,
      run: async () => {
        await api.deleteWatch(w.id)
        await library.refresh(w.tmdb_kind, w.tmdb_id).catch(() => {})
        toast('Watch deleted.')
        navigate('/diary')
      },
    })
  }
</script>

<div class="page">
  {#if error}
    <p class="alert" role="alert">Couldn’t load this watch: {error}</p>
  {:else if !w}
    <div class="skeleton" style="height:360px" aria-busy="true" aria-label="Loading"></div>
  {:else}
    <p class="crumb"><a href="#/diary">Diary</a> / Watch</p>
    <header class="head">
      <a href={titleHref} tabindex="-1" aria-hidden="true"><Poster path={poster} seed={w.tmdb_id} {title} kind={w.tmdb_kind} tmdbId={w.tmdb_id} width={110} radius={10} size="w342" /></a>
      <div class="hb">
        <h1><a href={titleHref}>{title || 'Untitled'}</a></h1>
        <p class="hint">{#if episode}{epCode(w.season_number, w.episode_number)}{:else}Film{#if meta?.release}{' · '}{meta.release.slice(0, 4)}{/if}{/if}</p>
        <div class="actions">
          <button class="btn-secondary" onclick={edit}><Icon name="pencil" size={15} />Edit</button>
          <button class="btn-secondary danger" onclick={remove}><Icon name="trash" size={15} />Delete</button>
          {#if episode}<a class="btn-secondary" href="#/show/{w.tmdb_id}?season={w.season_number}">View whole series</a>{/if}
        </div>
      </div>
    </header>

    <dl class="panel pad facts">
      {#each rows as [label, value, style, aria]}
        <div>
          <dt>{label}</dt>
          {#if style === 'logos' || style === 'named'}
            <dd class="badges">
              {#each value as f (f.slug)}<span class="fmt"><FormatBadge format={f} height={16} />{#if style === 'named' && f.logo_url}<span>{f.label}</span>{/if}</span>{/each}
            </dd>
          {:else}
            <dd class:accent={style === 'accent'} aria-label={aria}>{value}</dd>
          {/if}
        </div>
      {/each}
    </dl>
    {#if w.notes}<blockquote class="notes">{w.notes}</blockquote>{/if}
  {/if}
</div>

<style>
  .page { padding: 20px var(--gutter) 48px; display: grid; gap: 18px; max-width: 860px; }
  .crumb { margin: 0; color: var(--text-muted); font-size: 13px; }
  .crumb a { color: inherit; }
  .head { display: flex; gap: 20px; align-items: flex-end; }
  .hb { display: grid; gap: 6px; min-width: 0; }
  h1 { margin: 0; font: 700 32px/1.1 var(--font-display); letter-spacing: -0.02em; }
  h1 a { color: inherit; text-decoration: none; }
  h1 a:hover { text-decoration: underline; text-underline-offset: 4px; }
  .hb .hint { margin: 0; font-size: 14.5px; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 4px; }
  .actions button, .actions a { gap: 6px; text-decoration: none; }
  .danger:hover { color: var(--delta-down); }
  .facts { gap: 0; margin: 0; padding: 6px 20px; }
  .facts div { display: grid; grid-template-columns: 150px 1fr; gap: 16px; align-items: center; min-height: 42px; padding: 8px 0; box-shadow: inset 0 -1px 0 var(--border-subtle); }
  .facts div:last-child { box-shadow: none; }
  dt { color: var(--text-muted); font-size: 13.5px; font-weight: 600; }
  dd { margin: 0; font-size: 15px; }
  dd.accent { color: var(--accent-yours); font-weight: 600; }
  .badges { display: flex; flex-wrap: wrap; gap: 8px; }
  .fmt { display: inline-flex; align-items: center; gap: 8px; }
  .notes { margin: 0; padding: 4px 0 4px 16px; border-left: 3px solid var(--accent-yours); font: italic 400 17px/1.55 var(--font-display); white-space: pre-wrap; }
  @media (max-width: 560px) {
    .facts div { grid-template-columns: 110px 1fr; }
    h1 { font-size: 26px; }
  }
</style>
