import {
  countdownText,
  daysUntil,
  dayGroup,
  diaryMonths,
  episodeBadge,
  hoursText,
  moviesSplit,
  nextPerShow,
  recentDistinct,
  sortToWatch,
  tvToWatch,
  tvUpcoming,
} from '@/data/collections';

const today = '2026-11-14'; // a Saturday

test('daysUntil counts whole days', () => {
  expect(daysUntil('2026-11-14', today)).toBe(0);
  expect(daysUntil('2026-11-17', today)).toBe(3);
  expect(daysUntil('2026-12-15', today)).toBe(31);
  expect(daysUntil('2026-11-10', today)).toBe(-4);
});

test('countdownText', () => {
  expect(countdownText(0)).toBe('Today');
  expect(countdownText(1)).toBe('Tomorrow');
  expect(countdownText(5)).toBe('5 days');
});

test('dayGroup: weekday inside a week, month after', () => {
  expect(dayGroup('2026-11-15', today).key).toBe('tomorrow');
  expect(dayGroup('2026-11-17', today).key).toBe('2026-11-17');
  expect(dayGroup('2026-11-21', today).key).toBe('2026-11');
  expect(dayGroup('2027-04-08', today).key).toBe('2027-04');
});

test('tvToWatch orders stages and drops shows with nothing aired', () => {
  const ep = { season_number: 1, episode_number: 1 };
  const groups = tvToWatch([
    { tmdb_id: 'a', stage: 'stalled', next_episode: ep },
    { tmdb_id: 'b', stage: 'watching', next_episode: ep },
    { tmdb_id: 'c', stage: 'not_started', next_episode: null },
    { tmdb_id: 'd', stage: 'not_started', next_episode: ep },
    { tmdb_id: 'e', stage: 'new_episodes', next_episode: ep },
  ]);
  expect(groups.map((g) => g.key)).toEqual(['new_episodes', 'watching', 'stalled', 'not_started']);
  expect(groups[3].data.map((s) => s.tmdb_id)).toEqual(['d']);
  expect(groups[2].collapsed).toBe(true);
});

test('tvUpcoming groups consecutive days and months', () => {
  const groups = tvUpcoming(
    [
      { tmdb_id: 'x', air_date: '2026-11-17', episode_number: 6 },
      { tmdb_id: 'x', air_date: '2026-11-17', episode_number: 5 },
      { tmdb_id: 'y', air_date: '2026-11-15', episode_number: 1 },
      { tmdb_id: 'z', air_date: '2026-12-02', episode_number: 2 },
      { tmdb_id: 'z', air_date: null, episode_number: 3 },
    ],
    today,
  );
  expect(groups.map((g) => [g.key, g.data.length])).toEqual([
    ['tomorrow', 1],
    ['2026-11-17', 2],
    ['2026-12', 1],
  ]);
});

test('nextPerShow keeps the soonest episode of each show', () => {
  const out = nextPerShow([
    { tmdb_id: 'x', air_date: '2026-11-20', episode_number: 7 },
    { tmdb_id: 'x', air_date: '2026-11-17', episode_number: 6 },
    { tmdb_id: 'y', air_date: '2026-11-18', episode_number: 1 },
  ]);
  expect(out.map((e) => [e.tmdb_id, e.episode_number])).toEqual([
    ['x', 6],
    ['y', 1],
  ]);
});

test('moviesSplit: released or undated to watch, future by month', () => {
  const { toWatch, upcoming } = moviesSplit(
    [
      { tmdb_kind: 'movie', tmdb_id: '1', status: 'watchlist', release_date: '2026-03-01' },
      { tmdb_kind: 'movie', tmdb_id: '2', status: 'watchlist', release_date: '2026-12-16' },
      { tmdb_kind: 'movie', tmdb_id: '3', status: 'watchlist', release_date: '' },
      { tmdb_kind: 'movie', tmdb_id: '4', status: 'watchlist', release_date: '2026-11-18' },
      { tmdb_kind: 'tv', tmdb_id: '5', status: 'watchlist', release_date: '2027-01-01' },
      { tmdb_kind: 'movie', tmdb_id: '6', status: 'completed', release_date: '2026-01-01' },
    ],
    today,
  );
  expect(toWatch.map((t) => t.tmdb_id)).toEqual(['1', '3']);
  expect(upcoming.map((g) => [g.key, g.data.map((t) => t.tmdb_id)])).toEqual([
    ['2026-11', ['4']],
    ['2026-12', ['2']],
  ]);
});

test('episodeBadge and hoursText', () => {
  expect(episodeBadge({ episode_number: 1 })).toBe('Premiere');
  expect(episodeBadge({ episode_number: 8, episode_type: 'finale' })).toBe('Finale');
  expect(episodeBadge({ episode_number: 3 })).toBeNull();
  expect(hoursText(125)).toBe('2h 05m');
  expect(hoursText(45)).toBe('45m');
  expect(hoursText(120)).toBe('2h');
});

test('recentDistinct and diaryMonths', () => {
  const ws = [
    { id: 1, tmdb_kind: 'movie' as const, tmdb_id: 'a', watched_on: '2026-11-05' },
    { id: 2, tmdb_kind: 'movie' as const, tmdb_id: 'a', watched_on: '2026-10-25' },
    { id: 3, tmdb_kind: 'movie' as const, tmdb_id: 'b', watched_on: '2026-10-23' },
    { id: 4, tmdb_kind: 'movie' as const, tmdb_id: 'c', watched_on: '2026-09-27' },
  ];
  expect(recentDistinct(ws, 2).map((w) => w.id)).toEqual([1, 3]);
  expect(diaryMonths(ws).map((g) => [g.key, g.data.length])).toEqual([
    ['2026-11', 1],
    ['2026-10', 2],
    ['2026-09', 1],
  ]);
});

test('sortToWatch orders by release date with undated last, or keeps the order added', () => {
  const f = (id: string, release_date?: string) => ({ tmdb_kind: 'movie', tmdb_id: id, status: 'watchlist', release_date }) as never;
  const films = [f('a', '2020-05-01'), f('b'), f('c', '2024-01-01'), f('d', '2010-03-03')];
  expect(sortToWatch(films, 'added').map((x) => x.tmdb_id)).toEqual(['a', 'b', 'c', 'd']);
  expect(sortToWatch(films, 'latest').map((x) => x.tmdb_id)).toEqual(['c', 'a', 'd', 'b']);
  expect(sortToWatch(films, 'earliest').map((x) => x.tmdb_id)).toEqual(['d', 'a', 'c', 'b']);
});
