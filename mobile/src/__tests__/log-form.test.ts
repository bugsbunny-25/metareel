import type { components } from '@/api/schema';
import { blankForm, buildInput, formDay, formFromEntry, planOnDay, suggestPlan, zoneOf } from '@/log/form';
import { wallTime, zonedToDate } from '@/log/zones';

const now = new Date('2026-11-14T20:00:00Z');

test('zones: wall time round-trips through New York across the DST change', () => {
  expect(zonedToDate('2026-11-14T15:00', 'America/New_York').toISOString()).toBe('2026-11-14T20:00:00.000Z');
  expect(zonedToDate('2026-07-14T15:00', 'America/New_York').toISOString()).toBe('2026-07-14T19:00:00.000Z');
  expect(wallTime(new Date('2026-11-14T20:00:00Z'), 'Asia/Kolkata')).toBe('2026-11-15T01:30');
});

test('a quick new cinema log carries venue, formats, plan and amounts', () => {
  const f = {
    ...blankForm({ now, zone: 'America/New_York', currency: 'USD' }),
    place: 'theater' as const,
    venue: { id: 7, name: 'AMC Lincoln Square', timezone: 'America/New_York' },
    formats: ['imax-70mm'],
    subId: 3,
    faceValue: '21.50',
    price: '5',
    seat: 'H12',
    screening: 'premiere',
    eventName: 'Opening night',
  };
  const b = buildInput(f, { kind: 'movie', id: '603' }, now);
  expect(b).toMatchObject({
    venue_kind: 'theater',
    venue_id: 7,
    screen_id: null,
    formats: ['imax-70mm'],
    subscription_id: 3,
    face_value_minor: 2150,
    price_minor: 500,
    currency: 'USD',
    seat: 'H12',
    screening_type: 'premiere',
    event_name: 'Opening night',
    tickets_count: 1,
    tz: 'America/New_York',
    time_precision: 'minute',
  });
  expect(b.ended_at).toBe(now.toISOString());
});

test('a disc picks its media and carries disc and label formats', () => {
  const f = { ...blankForm({ now, zone: 'UTC' }), place: 'physical' as const, disc: 'uhd-blu-ray', label: 'criterion', edition: "Director's cut" };
  const b = buildInput(f, { kind: 'movie', id: '1' }, now);
  expect(b).toMatchObject({ media: 'bluray_4k', formats: ['uhd-blu-ray', 'criterion'], edition: "Director's cut", venue_id: null });
});

const entry: components['schemas']['WatchEntry'] = {
  id: 9,
  tmdb_kind: 'movie',
  tmdb_id: '603',
  subject_kind: 'movie',
  ended_at: '2026-11-10 03:30:00',
  tz: 'America/Los_Angeles',
  watched_on: '2026-11-09',
  time_precision: 'minute',
  rating: 8,
  liked: true,
  notes: 'Loud',
  tags: ['imax', 'friends'],
  venue_kind: 'theater',
  venue_id: 7,
  venue_name_at_watch: 'AMC Metreon',
  screen_id: 4,
  seat: 'J9',
  subscription_id: 3,
  face_value_minor: 1999,
  price_minor: 250,
  currency: 'USD',
  tickets_count: 2,
  formats: [{ slug: 'imax-70mm', category: 'brand' }],
  dimension: '2d',
  audio_format: 'Dolby Atmos',
};

test('editing without changes sends the watch back as stored', () => {
  const f = formFromEntry(entry, { deviceZone: 'Asia/Kolkata' });
  expect(f.wall).toBe('2026-11-09T19:30');
  const b = buildInput(f, { kind: 'movie', id: '603' }, now);
  expect(b).toMatchObject({
    ended_at: '2026-11-10T03:30:00.000Z',
    tz: 'America/Los_Angeles',
    rating: 8,
    liked: true,
    notes: 'Loud',
    tags: ['imax', 'friends'],
    venue_id: 7,
    screen_id: 4,
    seat: 'J9',
    subscription_id: 3,
    face_value_minor: 1999,
    price_minor: 250,
    currency: 'USD',
    tickets_count: 2,
    formats: ['imax-70mm'],
    dimension: '2d',
    audio_format: 'Dolby Atmos',
  });
});

test('editing the time is read in the watch zone; moving to streaming clears the cinema', () => {
  const f = { ...formFromEntry(entry, { deviceZone: 'Asia/Kolkata' }), wall: '2026-11-09T21:00', place: 'streaming' as const, provider: 'Netflix' };
  const b = buildInput(f, { kind: 'movie', id: '603' }, now);
  expect(b.ended_at).toBe('2026-11-10T05:00:00.000Z');
  expect(b).toMatchObject({
    venue_kind: 'streaming',
    venue_id: null,
    screen_id: null,
    subscription_id: null,
    price_minor: null,
    provider_name: 'Netflix',
    formats: [],
  });
});

test('picking another cinema files the watch in its zone and drops the screen', () => {
  const f = { ...formFromEntry(entry, { deviceZone: 'UTC' }), venue: { id: 8, name: 'Odeon Leicester Square', timezone: 'Europe/London' } };
  expect(zoneOf(f)).toBe('Europe/London');
  const b = buildInput(f, { kind: 'movie', id: '603' }, now);
  expect(b).toMatchObject({ venue_id: 8, screen_id: null, tz: 'Europe/London' });
});

test('validation', () => {
  const base = { ...blankForm({ now, zone: 'UTC' }), place: 'theater' as const };
  expect(() => buildInput({ ...base, price: 'abc' }, { kind: 'movie', id: '1' }, now)).toThrow('Paid must be an amount');
  expect(() => buildInput({ ...base, price: '5', currency: '' }, { kind: 'movie', id: '1' }, now)).toThrow('currency');
  expect(() => buildInput({ ...base, tickets: '1.5' }, { kind: 'movie', id: '1' }, now)).toThrow('Tickets');
  expect(() => buildInput({ ...base, quick: 'exact', wall: '2026-11-20T10:00' }, { kind: 'movie', id: '1' }, now)).toThrow('future');
});

test('plans: on the day and suggested by chain', () => {
  const plans = [
    { id: 1, chain: 'AMC', started_on: '2024-01-01', ended_on: null },
    { id: 2, chain: 'Regal', started_on: '2024-01-01', periods: [{ started_on: '2024-01-01', ended_on: '2024-06-30' }] },
  ];
  expect(plans.filter((p) => planOnDay(p, '2026-11-14')).map((p) => p.id)).toEqual([1]);
  expect(suggestPlan(plans, { name: 'AMC Metreon 16' })?.id).toBe(1);
  expect(suggestPlan(plans, { name: 'Alamo Drafthouse' })).toBeNull();
  expect(formDay({ ...blankForm({ now, zone: 'UTC' }), quick: 'exact', precision: 'day', day: '2026-11-01' })).toBe('2026-11-01');
});
