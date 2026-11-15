import type { components } from '@/api/schema';
import { endedAt, fractionDigits, toMinor } from '@/lib/format';

import { fromUTC, wallTime, zonedToDate } from './zones';

// The log sheet's form, shared by logging and editing a watch (the web app's
// LogWatch dialog). Pure, so the request it builds is tested without a
// device. An edit replaces the whole watch (PUT), so the form holds every
// field a watch has and sends them all back.

type WatchInput = components['schemas']['WatchInput'];
type WatchEntry = components['schemas']['WatchEntry'];

export type Place = '' | 'theater' | 'streaming' | 'physical' | 'broadcast' | 'other';
export type Precision = 'minute' | 'day' | 'month' | 'year';
/** A quick time for a new log, or "exact" for the pickers (always on edit). */
export type Quick = 'now' | 'hour' | 'today' | 'yesterday' | 'exact';
export type Venue = {
  id: number;
  name: string;
  city?: string | null;
  timezone?: string | null;
  chain?: string | null;
  distance_m?: number;
  your_watches?: number;
};

export const FORM_VERSION = 2;

export type LogForm = {
  v: number;
  quick: Quick;
  precision: Precision;
  /** "YYYY-MM-DDTHH:MM" in the watch's zone */
  wall: string;
  day: string;
  month: string;
  year: string;
  /** The zone the watch is filed in (an edit keeps the stored one). */
  zone: string;
  rating: number | null;
  liked: boolean;
  notes: string;
  tags: string;
  place: Place;
  // cinema
  venue: Venue | null;
  screenId: number | null;
  seat: string;
  screening: string;
  eventName: string;
  withQA: boolean;
  formats: string[];
  subId: number | null;
  /** The plan was picked or cleared by hand, so the venue no longer suggests one. */
  subTouched: boolean;
  faceValue: string;
  price: string;
  fees: string;
  snacks: string;
  currency: string;
  tickets: string;
  booking: string;
  // streaming / TV
  provider: string;
  providerId: number | null;
  providerOriginal: string;
  monetization: string;
  device: string;
  channel: string;
  // physical media
  disc: string;
  media: string;
  label: string;
  edition: string;
  // picture and sound
  dimension: string;
  resolution: string;
  hdr: string;
  audioFormat: string;
  frameRate: string;
  audioLang: string;
  subLang: string;
  aspect: string;
  aspectVariable: boolean;
  screenAspect: string;
  screenW: string;
  screenH: string;
  dimUnit: 'm' | 'ft';
  /** What the edit started from, so an untouched time or cinema is sent back as stored. */
  orig: { endedAt: string | null; wall: string; day: string; precision: Precision; venueId: number | null; zone: string } | null;
  savedAt: number;
};

/** The media enum a disc format stands for, so watches keep recording `media` too. */
export const DISC_MEDIA: Record<string, string> = { 'uhd-blu-ray': 'bluray_4k', 'blu-ray': 'bluray', dvd: 'dvd' };
const MEDIA_DISC = Object.fromEntries(Object.entries(DISC_MEDIA).map(([k, v]) => [v, k]));

export const SCREENINGS = [
  ['', 'Regular'],
  ['preview', 'Preview'],
  ['premiere', 'Premiere'],
  ['festival', 'Festival'],
  ['press', 'Press'],
  ['rerelease', 'Re-release'],
  ['marathon', 'Marathon'],
] as const;
export const PLANS = [
  ['flatrate', 'Subscription'],
  ['free', 'Free'],
  ['ads', 'With ads'],
  ['rent', 'Rented'],
  ['buy', 'Bought'],
] as const;
export const OTHER_MEDIA = [
  ['vhs', 'VHS'],
  ['digital_file', 'Digital file'],
  ['35mm', '35mm print'],
  ['70mm', '70mm print'],
] as const;
export const DIMENSIONS = [
  ['2d', '2D'],
  ['3d', '3D'],
  ['3d_hfr', '3D HFR'],
] as const;
export const RESOLUTIONS = [
  ['sd', 'SD'],
  ['hd', '720p'],
  ['fhd', '1080p'],
  ['4k', '4K'],
  ['8k', '8K'],
] as const;
export const HDRS = [
  ['none', 'No HDR'],
  ['hdr10', 'HDR10'],
  ['hdr10_plus', 'HDR10+'],
  ['dolby_vision', 'Dolby Vision'],
  ['hlg', 'HLG'],
] as const;
export const DEVICES = ['TV', 'Projector', 'Laptop', 'Tablet', 'Phone'];
export const AUDIO = ['Dolby Atmos', 'DTS:X', 'Dolby TrueHD', '7.1', '5.1', 'Stereo'];
export const ASPECTS = ['1.43:1', '1.78:1', '1.85:1', '1.90:1', '2.00:1', '2.20:1', '2.39:1', '2.76:1', '4:3'];

const today = (now: Date) => wallTime(now, null).slice(0, 10);

export function blankForm(o: { now?: Date; zone: string; currency?: string; unit?: 'm' | 'ft' }): LogForm {
  const now = o.now ?? new Date();
  const wall = wallTime(now, o.zone);
  return {
    v: FORM_VERSION,
    quick: 'now',
    precision: 'minute',
    wall,
    day: wall.slice(0, 10),
    month: wall.slice(0, 7),
    year: wall.slice(0, 4),
    zone: o.zone,
    rating: null,
    liked: false,
    notes: '',
    tags: '',
    place: '',
    venue: null,
    screenId: null,
    seat: '',
    screening: '',
    eventName: '',
    withQA: false,
    formats: [],
    subId: null,
    subTouched: false,
    faceValue: '',
    price: '',
    fees: '',
    snacks: '',
    currency: o.currency ?? '',
    tickets: '1',
    booking: '',
    provider: '',
    providerId: null,
    providerOriginal: '',
    monetization: 'flatrate',
    device: '',
    channel: '',
    disc: '',
    media: '',
    label: '',
    edition: '',
    dimension: '',
    resolution: '',
    hdr: '',
    audioFormat: '',
    frameRate: '',
    audioLang: '',
    subLang: '',
    aspect: '',
    aspectVariable: false,
    screenAspect: '',
    screenW: '',
    screenH: '',
    dimUnit: o.unit ?? 'm',
    orig: null,
    savedAt: now.getTime(),
  };
}

const fromMinor = (minor: number | null | undefined, currency: string) => {
  if (minor == null) return '';
  const d = fractionDigits(currency || 'USD');
  return (minor / 10 ** d).toFixed(d);
};
const str = (v: unknown) => (v == null ? '' : String(v));

/** A stored watch as the form, every field filled in. */
export function formFromEntry(w: WatchEntry, o: { deviceZone: string; unit?: 'm' | 'ft' }): LogForm {
  const zone = w.tz || o.deviceZone;
  const ended = fromUTC(w.ended_at) ?? new Date();
  const wall = wallTime(ended, zone);
  const watchedOn = w.watched_on || wall.slice(0, 10);
  const cur = w.currency ?? '';
  const precision = (w.time_precision ?? 'minute') as Precision;
  const place = (w.venue_kind ?? '') as Place;
  const formats = w.formats ?? [];
  return {
    ...blankForm({ now: ended, zone, currency: cur, unit: w.screen_dim_unit === 'ft' ? 'ft' : (o.unit ?? 'm') }),
    quick: 'exact',
    precision,
    wall,
    day: watchedOn,
    month: watchedOn.slice(0, 7),
    year: watchedOn.slice(0, 4),
    rating: w.rating ?? null,
    liked: !!w.liked,
    notes: w.notes ?? '',
    tags: (w.tags ?? []).join(', '),
    place,
    venue: w.venue_id ? { id: w.venue_id, name: w.venue_name_at_watch ?? 'Cinema', city: w.venue_city_at_watch ?? null } : null,
    screenId: w.screen_id ?? null,
    seat: w.seat ?? '',
    screening: w.screening_type === 'regular' ? '' : (w.screening_type ?? ''),
    eventName: w.event_name ?? '',
    withQA: !!w.with_qa,
    formats: formats.filter((f) => f.category !== 'disc' && f.category !== 'label').map((f) => f.slug!),
    disc: formats.find((f) => f.category === 'disc')?.slug ?? MEDIA_DISC[w.media ?? ''] ?? '',
    label: formats.find((f) => f.category === 'label')?.slug ?? '',
    media: formats.some((f) => f.category === 'disc') || MEDIA_DISC[w.media ?? ''] ? '' : (w.media ?? ''),
    subId: w.subscription_id ?? null,
    subTouched: true,
    faceValue: fromMinor(w.face_value_minor, cur),
    price: fromMinor(w.price_minor, cur),
    fees: fromMinor(w.fees_minor, cur),
    snacks: fromMinor(w.concessions_minor, cur),
    tickets: str(w.tickets_count),
    booking: w.booking_source ?? '',
    provider: place === 'broadcast' ? '' : (w.provider_name ?? ''),
    channel: place === 'broadcast' ? (w.provider_name ?? '') : '',
    providerId: w.provider_id ?? null,
    providerOriginal: w.provider_name ?? '',
    monetization: w.monetization ?? 'flatrate',
    device: w.device ?? '',
    edition: w.edition ?? '',
    dimension: w.dimension ?? '',
    resolution: w.resolution ?? '',
    hdr: w.hdr ?? '',
    audioFormat: w.audio_format ?? '',
    frameRate: str(w.frame_rate),
    audioLang: w.audio_language ?? '',
    subLang: w.subtitle_language ?? '',
    aspect: w.content_aspect_ratio_text ?? '',
    aspectVariable: !!w.content_aspect_variable,
    screenAspect: w.screen_aspect_ratio ? String(w.screen_aspect_ratio) : '',
    // A screen's own size is the screen's; only a hand-entered size is the watch's.
    screenW: w.screen_id ? '' : str(w.screen_width),
    screenH: w.screen_id ? '' : str(w.screen_height),
    orig: { endedAt: ended.toISOString(), wall, day: watchedOn, precision, venueId: w.venue_id ?? null, zone },
  };
}

/** The zone a watch is filed in: a newly picked cinema's own, else the form's. */
export function zoneOf(f: LogForm): string {
  if (f.place === 'theater' && f.venue?.timezone && f.venue.id !== (f.orig?.venueId ?? null)) return f.venue.timezone;
  return f.zone;
}

/** True when the plan may cover this kind of ticket (anything but a stored edit). */
export const moneyShown = (f: LogForm) => f.place === 'theater' || (f.place === 'streaming' && (f.monetization === 'rent' || f.monetization === 'buy'));

function when(f: LogForm, now: Date): { ended_at: string; time_precision: Precision } {
  if (f.quick !== 'exact') return endedAt(f.quick, now);
  const zone = zoneOf(f);
  // An edit whose time and zone weren't touched keeps its stored instant.
  if (f.orig && f.orig.endedAt && f.precision === f.orig.precision && zone === f.orig.zone) {
    const o = f.orig;
    const same =
      f.precision === 'minute'
        ? f.wall === o.wall
        : f.precision === 'day'
          ? f.day === o.day
          : f.precision === 'month'
            ? f.month === o.day.slice(0, 7)
            : f.year === o.day.slice(0, 4);
    if (same) return { ended_at: f.orig.endedAt, time_precision: f.precision };
  }
  const wall =
    f.precision === 'minute' ? f.wall : f.precision === 'day' ? `${f.day}T12:00` : f.precision === 'month' ? `${f.month}-01T12:00` : `${f.year}-01-01T12:00`;
  if (f.precision === 'month' && !/^\d{4}-\d{2}$/.test(f.month)) throw new Error('Pick the month.');
  if (f.precision === 'year' && !/^\d{4}$/.test(f.year)) throw new Error('The year must be four digits, like 2019.');
  const d = zonedToDate(wall, zone);
  if (Number.isNaN(d.getTime())) throw new Error('Check the date.');
  if (d.getTime() > now.getTime() + 5 * 60_000 && f.precision === 'minute') throw new Error("That's in the future.");
  if (f.precision === 'day' && f.day > today(now)) throw new Error("That's in the future.");
  return { ended_at: d.toISOString(), time_precision: f.precision };
}

const text = (v: string) => v.trim() || null;
function positive(v: string, name: string): number | null {
  const s = v.trim();
  if (!s) return null;
  const n = Number(s);
  if (!Number.isFinite(n) || n <= 0) throw new Error(`${name} must be a positive number.`);
  return n;
}

/**
 * The request body for the form. Every context field is sent, null when it
 * doesn't apply, so changing "where" on an edit clears what the old place
 * had.
 */
export function buildInput(f: LogForm, t: { kind: 'movie' | 'tv'; id: string; season?: number | null; episode?: number | null }, now = new Date()): WatchInput {
  const p = f.place;
  const zone = zoneOf(f);
  const tags = [
    ...new Set(
      f.tags
        .split(',')
        .map((x) => x.trim())
        .filter(Boolean),
    ),
  ].slice(0, 20);
  const out: WatchInput = {
    tmdb_kind: t.kind,
    tmdb_id: t.id,
    ...when(f, now),
    tz: zone,
    rating: f.rating,
    liked: f.liked,
    notes: text(f.notes),
    tags,
    venue_kind: p || null,
    formats: [],
    // Where: everything off unless the place below sets it.
    venue_id: null,
    screen_id: null,
    seat: null,
    screening_type: null,
    event_name: null,
    with_qa: false,
    booking_source: null,
    tickets_count: null,
    subscription_id: null,
    price_minor: null,
    fees_minor: null,
    concessions_minor: null,
    face_value_minor: null,
    currency: null,
    provider_name: null,
    provider_id: null,
    monetization: null,
    device: null,
    media: null,
    edition: null,
    // Picture and sound apply wherever you watched.
    dimension: (f.dimension || null) as WatchInput['dimension'],
    resolution: (f.resolution || null) as WatchInput['resolution'],
    hdr: (f.hdr || null) as WatchInput['hdr'],
    audio_format: text(f.audioFormat),
    frame_rate: positive(f.frameRate, 'Frame rate'),
    audio_language: text(f.audioLang),
    subtitle_language: text(f.subLang),
    content_aspect_ratio: text(f.aspect),
    content_aspect_variable: f.aspectVariable,
    screen_aspect_ratio: text(f.screenAspect),
    screen_width: null,
    screen_height: null,
    screen_dim_unit: null,
  };
  const w = positive(f.screenW, 'Screen width');
  const h = positive(f.screenH, 'Screen height');
  if (w || h) Object.assign(out, { screen_width: w, screen_height: h, screen_dim_unit: f.dimUnit });

  if (p === 'theater') {
    const tickets = f.tickets.trim() ? Number(f.tickets) : null;
    if (tickets != null && (!Number.isInteger(tickets) || tickets < 1)) throw new Error('Tickets must be a whole number.');
    Object.assign(out, {
      venue_id: f.venue?.id ?? null,
      // A screen belongs to its cinema; picking another cinema drops it.
      screen_id: f.venue && f.venue.id === (f.orig?.venueId ?? null) ? f.screenId : null,
      seat: text(f.seat),
      screening_type: f.screening || null,
      event_name: f.screening ? text(f.eventName) : null,
      with_qa: f.withQA,
      formats: f.formats,
      booking_source: text(f.booking),
      tickets_count: tickets,
      subscription_id: f.subId,
    });
  }
  if (p === 'streaming') {
    const name = text(f.provider);
    Object.assign(out, {
      provider_name: name,
      monetization: f.monetization || null,
      device: text(f.device),
      // Keep a stored service id only while the name it belongs to is unchanged.
      provider_id: name && name === f.providerOriginal ? f.providerId : null,
    });
  }
  if (p === 'physical') {
    Object.assign(out, {
      media: f.disc ? (DISC_MEDIA[f.disc] ?? null) : f.media || null,
      edition: text(f.edition),
      device: text(f.device),
      formats: [f.disc, f.label].filter(Boolean),
    });
  }
  if (p === 'broadcast') Object.assign(out, { provider_name: text(f.channel), device: text(f.device) });
  if (moneyShown(f)) {
    const currency = f.currency.trim().toUpperCase();
    const minor = (v: string, name: string) => {
      const m = toMinor(v, currency || 'USD');
      if (m === undefined) throw new Error(`${name} must be an amount like 12.50.`);
      return m;
    };
    const amounts = {
      price_minor: minor(f.price, p === 'theater' ? (f.subId ? 'Paid on top' : 'Paid') : 'Price'),
      fees_minor: p === 'theater' ? minor(f.fees, 'Fees') : null,
      concessions_minor: p === 'theater' ? minor(f.snacks, 'Snacks') : null,
      face_value_minor: p === 'theater' && f.subId ? minor(f.faceValue, 'Face value') : null,
    };
    Object.assign(out, amounts);
    if (Object.values(amounts).some((v) => v != null)) {
      if (!/^[A-Z]{3}$/.test(currency)) throw new Error('Pick a currency for the amounts, e.g. USD.');
      out.currency = currency;
    }
  }
  if (t.kind === 'tv') Object.assign(out, { season_number: t.season ?? null, episode_number: t.episode ?? null });
  return out;
}

/** Whether a plan ran on a day (any of its runs covers it). */
export function planOnDay(
  s: { started_on?: string; ended_on?: string | null; periods?: { started_on: string; ended_on?: string | null }[] },
  day: string,
): boolean {
  const runs = s.periods?.length ? s.periods : [{ started_on: s.started_on ?? '', ended_on: s.ended_on }];
  return runs.some((r) => r.started_on <= day && (!r.ended_on || r.ended_on >= day));
}

/** The plan a cinema suggests: the one plan whose chain is in its name or chain. */
export function suggestPlan<T extends { id?: number; chain?: string | null }>(plans: T[], venue: { name?: string; chain?: string | null } | null): T | null {
  if (!venue) return null;
  const hay = `${venue.chain ?? ''} ${venue.name ?? ''}`.toLowerCase();
  const hits = plans.filter((s) => s.chain && hay.includes(s.chain.toLowerCase()));
  return hits.length === 1 ? hits[0] : null;
}

/** The day the watch falls on (for picking plans that ran then). */
export function formDay(f: LogForm, now = new Date()): string {
  if (f.quick === 'exact') {
    if (f.precision === 'minute') return f.wall.slice(0, 10);
    if (f.precision === 'day') return f.day;
    if (f.precision === 'month') return `${f.month}-01`;
    return `${f.year}-01-01`;
  }
  const d = new Date(endedAt(f.quick, now).ended_at);
  return wallTime(d, null).slice(0, 10);
}
