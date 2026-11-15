jest.mock('expo-sqlite/kv-store', () => ({ getItemSync: () => null, setItemSync: () => {} }));
jest.mock('@/auth/session', () => ({ api: {} }));

// eslint-disable-next-line import/first -- the mocks above must come first
import { changeText, isYours, movement, providerName } from '@/data/scope';

test('chart movement reads like the web', () => {
  expect(changeText(movement({ rank: 2, previous_rank: 5, days_in_top10: 4 }))).toBe('▲ 3');
  expect(changeText(movement({ rank: 6, previous_rank: 5, days_in_top10: 4 }))).toBe('▼ 1');
  expect(changeText(movement({ rank: 5, previous_rank: 5, days_in_top10: 4 }))).toBe('–');
  expect(changeText(movement({ rank: 1, previous_rank: null, days_in_top10: 1 }))).toBe('New');
  expect(changeText(movement({ rank: 9, previous_rank: null, days_in_top10: 7 }))).toBe('Back');
});

test('what counts as yours', () => {
  expect(isYours({ watch_count: 1 })).toBe(true);
  expect(isYours({ favorite: true })).toBe(true);
  expect(isYours({ status: 'watchlist' })).toBe(true);
  expect(isYours({ status: 'not_interested' })).toBe(false);
  expect(isYours({ status: 'not_interested', watch_count: 2 })).toBe(true);
  expect(isYours({})).toBe(false);
  expect(providerName('amazon-prime')).toBe('Prime Video');
  expect(providerName('crunchy-roll')).toBe('Crunchy Roll');
});
