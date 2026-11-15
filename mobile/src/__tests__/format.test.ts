jest.mock('@/auth/session', () => ({ api: {} }));

// eslint-disable-next-line import/first -- the mock above must come first
import { currencyFor, dayHeading, endedAt, episodeCode, formatMoney, starText, toMinor, tmdbImage } from '@/lib/format';

test('money stays in minor units', () => {
  expect(toMinor('12.50', 'USD')).toBe(1250);
  expect(toMinor('1,200', 'JPY')).toBe(1200);
  expect(toMinor('', 'USD')).toBeNull();
  expect(toMinor('-3', 'USD')).toBeUndefined();
  expect(toMinor('abc', 'USD')).toBeUndefined();
  expect(formatMoney(1250, 'USD')).toMatch(/12\.50/);
  expect(formatMoney(null, 'USD')).toBe('');
  expect(currencyFor('IN')).toBe('INR');
  expect(currencyFor('DE')).toBe('EUR');
  expect(currencyFor(undefined)).toBe('USD');
});

test('quick-log times', () => {
  const now = new Date(2026, 10, 14, 23, 30);
  expect(endedAt('now', now)).toEqual({ ended_at: now.toISOString(), time_precision: 'minute' });
  expect(new Date(endedAt('hour', now).ended_at).getHours()).toBe(22);
  const today = endedAt('today', now);
  expect(today.time_precision).toBe('day');
  expect(new Date(today.ended_at).getDate()).toBe(14);
  expect(new Date(endedAt('yesterday', now).ended_at).getDate()).toBe(13);
});

test('labels', () => {
  const today = new Date(2026, 10, 14);
  expect(dayHeading('2026-11-14', today)).toBe('Today');
  expect(dayHeading('2026-11-13', today)).toBe('Yesterday');
  expect(dayHeading('2025-11-13', today)).toMatch(/2025/);
  expect(starText(7)).toBe('★★★½');
  expect(starText(10)).toBe('★★★★★');
  expect(starText(null)).toBe('');
  expect(episodeCode(1, 3)).toBe('S01 E03');
  expect(tmdbImage('/a.jpg', 'w92')).toBe('https://image.tmdb.org/t/p/w92/a.jpg');
  expect(tmdbImage(null)).toBeNull();
});

test('search results alternate films and series', () => {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { interleaveKinds } = require('@/data/queries') as typeof import('@/data/queries');
  const r = interleaveKinds([{ tmdb_kind: 'movie', n: 1 }, { tmdb_kind: 'movie', n: 2 }, { tmdb_kind: 'tv', n: 3 }]);
  expect(r.map((x) => x.n)).toEqual([1, 3, 2]);
});
