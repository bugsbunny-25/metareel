import type { components } from '@/api/schema';

export type DiscRelease = components['schemas']['DiscRelease'];
export type DiscCopy = components['schemas']['DiscCopy'];
export type DiscImage = components['schemas']['DiscImage'];
export type DiscLookup = components['schemas']['DiscLookup'];
export type MediaFormat = components['schemas']['DiscMediaFormat'];
export type ImageKind = components['schemas']['DiscImageKind'];
export type Condition = NonNullable<components['schemas']['DiscCopyInput']['condition']>;
export type Packaging = NonNullable<components['schemas']['DiscReleaseInput']['packaging']>;

// Labels for the server's keys (service/discs.go), as the web shows them (app/src/lib/discs.js).
export const MEDIA_FORMATS: [MediaFormat, string][] = [
  ['uhd_bluray', '4K Ultra HD'],
  ['bluray', 'Blu-ray'],
  ['bluray_3d', 'Blu-ray 3D'],
  ['dvd', 'DVD'],
  ['hd_dvd', 'HD DVD'],
  ['other', 'Other'],
];
export const IMAGE_KINDS: [ImageKind, string][] = [
  ['front', 'Front'],
  ['back', 'Back'],
  ['slip_front', 'Slip front'],
  ['slip_back', 'Slip back'],
  ['spine', 'Spine'],
  ['disc', 'Disc'],
  ['booklet', 'Booklet'],
  ['inside', 'Inside'],
  ['other', 'Other'],
];
export const CONDITIONS: [Condition, string][] = [
  ['sealed', 'Sealed'],
  ['new', 'New'],
  ['used', 'Used'],
];

const LABELS: Record<string, string> = {
  ...Object.fromEntries(MEDIA_FORMATS),
  cd: 'CD',
  keepcase: 'Keepcase',
  steelbook: 'Steelbook',
  digipak: 'Digipak',
  digibook: 'Digibook',
  mediabook: 'Mediabook',
  box_set: 'Box set',
  bd100: 'BD-100',
  bd66: 'BD-66',
  bd50: 'BD-50',
  bd25: 'BD-25',
  dvd9: 'DVD-9',
  dvd5: 'DVD-5',
  hd_dvd30: 'HD DVD-30',
  hd_dvd15: 'HD DVD-15',
  hevc: 'HEVC',
  avc: 'AVC',
  vc1: 'VC-1',
  mpeg2: 'MPEG-2',
  dolby_vision: 'Dolby Vision',
  hdr10_plus: 'HDR10+',
  hdr10: 'HDR10',
  hlg: 'HLG',
  dolby_atmos: 'Dolby Atmos',
  dts_x: 'DTS:X',
  auro_3d: 'Auro-3D',
  dolby_truehd: 'Dolby TrueHD',
  dts_hd_ma: 'DTS-HD MA',
  dts_hd_hr: 'DTS-HD HR',
  lpcm: 'LPCM',
  dolby_digital_plus: 'Dolby Digital Plus',
  dts: 'DTS',
  dolby_digital: 'Dolby Digital',
  aac: 'AAC',
  mpeg: 'MPEG',
  sealed: 'Sealed',
  new: 'New',
  used: 'Used',
  ...Object.fromEntries(IMAGE_KINDS),
};
export const label = (key?: string | null) => (key ? (LABELS[key] ?? key) : '');
export const HIGHLIGHT: Record<string, string> = {
  dolby_vision: 'Dolby Vision',
  hdr10_plus: 'HDR10+',
  hdr10: 'HDR10',
  hlg: 'HLG',
  dolby_atmos: 'Atmos',
  dts_x: 'DTS:X',
  auro_3d: 'Auro-3D',
};

// The case each format comes in: the band across the top of the art, the
// plastic, and the case's proportions. These are the formats' own identity
// colours (like a provider's logo), not theme colours, so they stay put in
// light and dark mode; chart is the format in charts, where a black band
// would vanish on the dark surface.
export const CASE: Record<
  string,
  {
    band: string;
    text: string;
    mark: string;
    spine: string;
    ratio: number;
    chart: string;
  }
> = {
  uhd_bluray: {
    band: '#0d0d0f',
    text: '#f5f5f7',
    mark: '4K ULTRA HD',
    spine: '4K',
    ratio: 0.8,
    chart: '#a8a8b0',
  },
  bluray: {
    band: '#0b5fd1',
    text: '#ffffff',
    mark: 'BLU-RAY',
    spine: 'BD',
    ratio: 0.8,
    chart: '#2f7ae5',
  },
  bluray_3d: {
    band: '#0b5fd1',
    text: '#ffffff',
    mark: 'BLU-RAY 3D',
    spine: '3D',
    ratio: 0.8,
    chart: '#7aa7f0',
  },
  hd_dvd: {
    band: '#a3161d',
    text: '#ffffff',
    mark: 'HD DVD',
    spine: 'HD',
    ratio: 0.8,
    chart: '#c8323a',
  },
  dvd: {
    band: '#1b1b1d',
    text: '#d9d9de',
    mark: 'DVD',
    spine: 'DVD',
    ratio: 0.71,
    chart: '#64646c',
  },
  other: {
    band: '#3a3a40',
    text: '#ffffff',
    mark: '',
    spine: '',
    ratio: 0.75,
    chart: '#8a7a5a',
  },
};
export const caseOf = (format?: string | null) => CASE[format ?? 'other'] ?? CASE.other;

/** The best picture of a kind: the release's own, else its first part's. */
export function imageOf(r: DiscRelease | null | undefined, kind: ImageKind): DiscImage | null {
  if (!r) return null;
  return r.images.find((i) => i.kind === kind) ?? r.parts.map((p) => p.images.find((i) => i.kind === kind)).find(Boolean) ?? null;
}

/** Every picture of a release and its parts, with the part it shows. */
export function allImages(r: DiscRelease): (DiscImage & { releaseId: number; part: string })[] {
  return [
    ...r.images.map((im) => ({ ...im, releaseId: r.id, part: '' })),
    ...r.parts.flatMap((p) => p.images.map((im) => ({ ...im, releaseId: p.id, part: p.title }))),
  ];
}

/** The front cover's image, else the first title's TMDB poster path. */
export function coverOf(r: DiscRelease): {
  image?: DiscImage;
  poster?: string;
} {
  const image = imageOf(r, 'front') ?? imageOf(r, 'slip_front');
  if (image) return { image };
  const t = r.titles[0] ?? r.parts.find((p) => p.titles.length)?.titles[0];
  return t?.poster_path ? { poster: t.poster_path } : {};
}

export const isBoxSet = (r: DiscRelease) => r.parts.length > 0 || r.packaging === 'box_set' || r.titles.length > 1;

/** "2160p HEVC · Dolby Vision, HDR10 · 2.39:1" for a disc. */
export function videoLine(d: components['schemas']['DiscSpec']): string {
  const parts = [[d.resolution, label(d.video_codec)].filter(Boolean).join(' ')];
  if (d.hdr?.length) parts.push(d.hdr.map(label).join(', '));
  if (d.aspect_ratio) parts.push(d.aspect_ratio + (d.variable_aspect ? ' (varies)' : ''));
  return parts.filter(Boolean).join(' · ');
}
