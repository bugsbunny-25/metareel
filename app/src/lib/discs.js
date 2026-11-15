// Physical media: labels for the API's keys, the look of each format's case, and helpers
// for picking a release's artwork. Keys match the server's vocabularies (service/discs.go).

export const MEDIA_FORMATS = [
  ['uhd_bluray', '4K Ultra HD'],
  ['bluray', 'Blu-ray'],
  ['bluray_3d', 'Blu-ray 3D'],
  ['dvd', 'DVD'],
  ['hd_dvd', 'HD DVD'],
  ['other', 'Other'],
]
export const DISC_FORMATS = [...MEDIA_FORMATS.slice(0, 5), ['cd', 'CD'], ['other', 'Other']]
export const PACKAGINGS = [
  ['', '—'], ['keepcase', 'Keepcase'], ['steelbook', 'Steelbook'], ['digipak', 'Digipak'], ['digibook', 'Digibook'],
  ['mediabook', 'Mediabook'], ['box_set', 'Box set'], ['other', 'Other'],
]
export const CAPACITIES = [['', '—'], ['bd100', 'BD-100'], ['bd66', 'BD-66'], ['bd50', 'BD-50'], ['bd25', 'BD-25'], ['dvd9', 'DVD-9'], ['dvd5', 'DVD-5'], ['hd_dvd30', 'HD DVD-30'], ['hd_dvd15', 'HD DVD-15'], ['cd', 'CD']]
export const CODECS = [['', '—'], ['hevc', 'HEVC / H.265'], ['avc', 'AVC / H.264'], ['vc1', 'VC-1'], ['mpeg2', 'MPEG-2'], ['other', 'Other']]
export const RESOLUTIONS = [['', '—'], ['2160p', '2160p'], ['1080p', '1080p'], ['1080i', '1080i'], ['720p', '720p'], ['576p', '576p'], ['576i', '576i'], ['480p', '480p'], ['480i', '480i']]
export const HDR = [['dolby_vision', 'Dolby Vision'], ['hdr10_plus', 'HDR10+'], ['hdr10', 'HDR10'], ['hlg', 'HLG']]
export const AUDIO = [
  ['dolby_atmos', 'Dolby Atmos'], ['dts_x', 'DTS:X'], ['auro_3d', 'Auro-3D'], ['dolby_truehd', 'Dolby TrueHD'],
  ['dts_hd_ma', 'DTS-HD Master Audio'], ['dts_hd_hr', 'DTS-HD High Res'], ['lpcm', 'LPCM'], ['dolby_digital_plus', 'Dolby Digital Plus'],
  ['dts', 'DTS'], ['dolby_digital', 'Dolby Digital'], ['aac', 'AAC'], ['mpeg', 'MPEG'], ['other', 'Other'],
]
export const CONDITIONS = [['', '—'], ['sealed', 'Sealed'], ['new', 'New'], ['used', 'Used']]
export const IMAGE_KINDS = [
  ['front', 'Front'], ['back', 'Back'], ['slip_front', 'Slipcover front'], ['slip_back', 'Slipcover back'],
  ['spine', 'Spine'], ['disc', 'Disc'], ['booklet', 'Booklet'], ['inside', 'Inside'], ['other', 'Other'],
]

const map = (pairs) => Object.fromEntries(pairs)
const L = {
  format: map(DISC_FORMATS), packaging: map(PACKAGINGS), capacity: map(CAPACITIES), codec: map(CODECS),
  hdr: map(HDR), audio: map(AUDIO), condition: map(CONDITIONS), kind: map(IMAGE_KINDS),
}
export const label = (vocab, key) => (key ? L[vocab][key] ?? key : '')
/** Shelf badges: HDR and immersive audio. */
export const HIGHLIGHT_LABEL = { dolby_vision: 'Dolby Vision', hdr10_plus: 'HDR10+', hdr10: 'HDR10', hlg: 'HLG', dolby_atmos: 'Atmos', dts_x: 'DTS:X', auro_3d: 'Auro-3D' }

// The case each format comes in: the band printed across the top of the art and the
// plastic's colour. These are the formats' own identity colours (like a provider's logo),
// not theme colours, so they stay the same in light and dark mode.
export const CASE = {
  // chart: the format in charts, where a black band would vanish on the dark surface.
  uhd_bluray: { band: '#0d0d0f', text: '#f5f5f7', mark: '4K ULTRA HD', ratio: 0.8, chart: '#a8a8b0' },
  bluray: { band: '#0b5fd1', text: '#ffffff', mark: 'BLU-RAY', ratio: 0.8, chart: '#2f7ae5' },
  bluray_3d: { band: '#0b5fd1', text: '#ffffff', mark: 'BLU-RAY 3D', ratio: 0.8, chart: '#7aa7f0' },
  hd_dvd: { band: '#a3161d', text: '#ffffff', mark: 'HD DVD', ratio: 0.8, chart: '#c8323a' },
  dvd: { band: '#1b1b1d', text: '#d9d9de', mark: 'DVD', ratio: 0.71, chart: '#64646c' },
  other: { band: '#3a3a40', text: '#ffffff', mark: '', ratio: 0.75, chart: '#8a7a5a' },
}
export const caseOf = (format) => CASE[format] ?? CASE.other

/** All of a release's images, its parts' included, with the release they belong to. */
export function allImages(release) {
  if (!release) return []
  return [
    ...release.images.map((im) => ({ ...im, releaseId: release.id, partTitle: '' })),
    ...release.parts.flatMap((p) => p.images.map((im) => ({ ...im, releaseId: p.id, partTitle: p.title }))),
  ]
}

/** The best picture of a kind: the release's own, else its first part's. */
export function imageOf(release, kind) {
  if (!release) return null
  return release.images.find((i) => i.kind === kind) ?? release.parts.map((p) => p.images.find((i) => i.kind === kind)).find(Boolean) ?? null
}

/** The front cover's thumbnail, else the first title's TMDB poster (path, for <Poster>). */
export function coverOf(release) {
  const front = imageOf(release, 'front') ?? imageOf(release, 'slip_front')
  if (front) return { src: front.thumb_url, full: front.url }
  const t = release?.titles[0] ?? release?.parts.find((p) => p.titles.length)?.titles[0]
  return t?.poster_path ? { poster: t.poster_path, title: t } : null
}

/** Every TMDB title on a release and its parts, in order, without repeats. */
export function titlesOf(release) {
  const seen = new Set()
  const out = []
  for (const t of [...(release?.titles ?? []), ...(release?.parts ?? []).flatMap((p) => p.titles)]) {
    const k = `${t.tmdb_kind}/${t.tmdb_id}/${t.season_number ?? ''}`
    if (!seen.has(k)) seen.add(k), out.push(t)
  }
  return out
}

export const isBoxSet = (r) => !!r && (r.parts.length > 0 || r.packaging === 'box_set' || r.titles.length > 1)

/** "2160p HEVC · Dolby Vision, HDR10 · 2.39:1" for a disc. */
export function videoLine(d) {
  const parts = [[d.resolution, d.video_codec ? label('codec', d.video_codec).split(' / ')[0] : ''].filter(Boolean).join(' ')]
  if (d.hdr?.length) parts.push(d.hdr.map((h) => label('hdr', h)).join(', '))
  if (d.aspect_ratio) parts.push(d.aspect_ratio + (d.variable_aspect ? ' (varies)' : ''))
  return parts.filter(Boolean).join(' · ')
}

/** "English Dolby Atmos 7.1" for an audio track. */
export const audioLine = (a) => [a.language, label('audio', a.format), a.channels].filter(Boolean).join(' ') + (a.description ? ` — ${a.description}` : '')

/** An empty release form. */
export const blankRelease = (over = {}) => ({
  barcode: '', title: '', edition: '', media_format: 'bluray', packaging: '', has_slipcover: false, region: '', country: '',
  studio: '', label: '', release_date: '', discs: [], special_features: [], notes: '', titles: [], parts: [], ...over,
})
export const blankDisc = (format = 'bluray') => ({ format, capacity: '', name: '', video_codec: '', resolution: '', hdr: [], aspect_ratio: '', variable_aspect: false, region: '', runtime_minutes: 0, audio: [], subtitles: [], notes: '' })

/** A release as the API returned it, turned back into what PUT accepts. */
export function releaseInput(r) {
  return {
    id: r.parent_id ? r.id : undefined,
    barcode: r.barcode ?? '', title: r.title, edition: r.edition, media_format: r.media_format, packaging: r.packaging,
    has_slipcover: r.has_slipcover, region: r.region, country: r.country ?? '', studio: r.studio, label: r.label ?? '',
    release_date: r.release_date ?? '', notes: r.notes ?? '',
    discs: r.discs.map((d) => ({ ...blankDisc(), ...d, hdr: [...(d.hdr ?? [])], audio: (d.audio ?? []).map((a) => ({ ...a })), subtitles: [...(d.subtitles ?? [])] })),
    special_features: r.special_features.map((f) => ({ ...f })),
    titles: r.titles.map((t) => ({ ...t })),
    parts: r.parts.map(releaseInput),
  }
}
