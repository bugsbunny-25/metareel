import * as Location from 'expo-location';
import Storage from 'expo-sqlite/kv-store';

import { unwrap } from '@/api/client';
import { api } from '@/auth/session';

export type NearbyVenue = {
  id: number;
  name: string;
  chain?: string;
  city?: string;
  country?: string;
  timezone?: string;
  latitude?: number;
  longitude?: number;
  distance_m: number;
  your_watches: number;
  screen_count?: number;
};

// "Which cinema am I in?": nearest cinemas from the phone's position. The
// last answer (30 km around where it was asked) is kept on the phone, so in a
// basement with GPS but no data the list is ranked locally instead. The
// position goes to your own server only, is not stored there, and is not
// kept here beyond that cached list.
const CACHE_KEY = 'nearby.cache';
const RADIUS_KM = 30;

type Cache = { lat: number; lon: number; at: number; venues: NearbyVenue[] };

export function haversineM(lat1: number, lon1: number, lat2: number, lon2: number): number {
  const rad = Math.PI / 180;
  const dLat = (lat2 - lat1) * rad;
  const dLon = (lon2 - lon1) * rad;
  const a = Math.sin(dLat / 2) ** 2 + Math.cos(lat1 * rad) * Math.cos(lat2 * rad) * Math.sin(dLon / 2) ** 2;
  return 2 * 6371000 * Math.asin(Math.min(1, Math.sqrt(a)));
}

/** Ranks cached venues from a new position, like the server does (yours count 250 m closer). */
export function rankLocally(venues: NearbyVenue[], lat: number, lon: number, limit: number): NearbyVenue[] {
  return venues
    .filter((v) => v.latitude != null && v.longitude != null)
    .map((v) => ({ ...v, distance_m: Math.round(haversineM(lat, lon, v.latitude!, v.longitude!)) }))
    .sort((a, b) => a.distance_m - (a.your_watches ? 250 : 0) - (b.distance_m - (b.your_watches ? 250 : 0)) || a.distance_m - b.distance_m)
    .slice(0, limit);
}

export type NearbyResult =
  | { status: 'ok'; venues: NearbyVenue[]; offline: boolean }
  | { status: 'denied' }
  | { status: 'unavailable'; message: string };

export async function findNearby(limit = 8): Promise<NearbyResult> {
  const perm = await Location.requestForegroundPermissionsAsync();
  if (!perm.granted) return { status: 'denied' };
  let pos = await Location.getLastKnownPositionAsync({ maxAge: 5 * 60_000, requiredAccuracy: 500 });
  // Balanced (network-assisted) first; without Google's location service on
  // Android that fails, so fall back to GPS alone.
  for (const accuracy of [Location.Accuracy.Balanced, Location.Accuracy.High]) {
    if (pos) break;
    try {
      pos = await Location.getCurrentPositionAsync({ accuracy, mayShowUserSettingsDialog: false });
    } catch {
      // try the next
    }
  }
  if (!pos) return { status: 'unavailable', message: "Couldn't get your position." };
  const { latitude: lat, longitude: lon } = pos.coords;
  try {
    const data = unwrap(
      await api.GET('/api/v1/me/venues/nearby', {
        params: { query: { lat, lon, radius_km: RADIUS_KM, limit: 100 } },
      }),
    );
    const venues = (data.venues ?? []) as NearbyVenue[];
    try {
      Storage.setItemSync(CACHE_KEY, JSON.stringify({ lat, lon, at: Date.now(), venues } satisfies Cache));
    } catch {
      // Without a cache the list simply needs the network next time.
    }
    return { status: 'ok', venues: venues.slice(0, limit), offline: false };
  } catch {
    const raw = (() => {
      try {
        return Storage.getItemSync(CACHE_KEY);
      } catch {
        return null;
      }
    })();
    if (!raw) return { status: 'unavailable', message: "You're offline and no cinemas are saved yet. Search by name instead." };
    const cache = JSON.parse(raw) as Cache;
    if (haversineM(lat, lon, cache.lat, cache.lon) > RADIUS_KM * 1000) {
      return { status: 'unavailable', message: "You're offline and far from the saved cinemas. Search by name instead." };
    }
    return { status: 'ok', venues: rankLocally(cache.venues, lat, lon, limit), offline: true };
  }
}
