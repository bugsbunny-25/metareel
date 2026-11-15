jest.mock('expo-location', () => ({}));
jest.mock('expo-sqlite/kv-store', () => ({}));
jest.mock('@/auth/session', () => ({ api: {} }));

// eslint-disable-next-line import/first -- the mocks above must come first
import { haversineM, type NearbyVenue, rankLocally } from '@/data/nearby';

const v = (id: number, lat: number, lon: number, yours = 0): NearbyVenue => ({
  id, name: `v${id}`, latitude: lat, longitude: lon, distance_m: 0, your_watches: yours,
});

test('distances', () => {
  expect(Math.round(haversineM(51.5106, -0.1299, 51.5108, -0.1295) / 10)).toBe(4); // ~36 m
});

test('cached cinemas are re-ranked from a new position, yours slightly favoured', () => {
  const here = { lat: 19.0, lon: 72.8 };
  const list = [v(1, 19.0 + 0.0009, 72.8), v(2, 19.0 - 0.003, 72.8, 2), v(3, 19.2, 72.8), { ...v(4, 0, 0), latitude: undefined }];
  expect(rankLocally(list, here.lat, here.lon, 10).map((x) => x.id)).toEqual([2, 1, 3]);
  expect(rankLocally(list, here.lat, here.lon, 1)).toHaveLength(1);
});
