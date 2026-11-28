import Constants from 'expo-constants';
import * as Notifications from 'expo-notifications';
import { AndroidImportance } from 'expo-notifications';
import Storage from 'expo-sqlite/kv-store';

import { unwrap } from '@/api/client';
import { api } from '@/auth/session';
import { queryClient } from '@/query/client';

// Watchlist arrivals (phase 6b) come from the server as push notifications
// through Expo's push service. Turning them on here: permission, the
// "arrivals" channel on Android, an Expo push token registered with
// PUT /me/devices (tied to this session, so signing out removes it), and the
// notify_watchlist_arrivals preference, which follows the account.

export const ARRIVALS_CHANNEL = 'arrivals';
const DEVICE_KEY = 'push.device_id';

export type ArrivalsResult = 'on' | 'off' | 'denied' | 'blocked' | 'unavailable' | 'server-off';

async function ensureChannel() {
  if (process.env.EXPO_OS !== 'android') return;
  await Notifications.setNotificationChannelAsync(ARRIVALS_CHANNEL, {
    name: 'Watchlist arrivals',
    description: 'A title on your watchlist arrives on one of your services',
    importance: AndroidImportance.DEFAULT,
  });
}

function projectId(): string | undefined {
  const extra = Constants.expoConfig?.extra as { eas?: { projectId?: string } } | undefined;
  return extra?.eas?.projectId ?? Constants.easConfig?.projectId;
}

/**
 * Registers this device's push token with the server. Returns null when the
 * build can't get a token (no EAS project, no push entitlement or Firebase
 * config, a simulator without push).
 */
export async function registerDevice(): Promise<{ id: number; pushEnabled: boolean } | null> {
  const id = projectId();
  if (!id) return null;
  let token: string;
  try {
    token = (await Notifications.getExpoPushTokenAsync({ projectId: id })).data;
  } catch {
    return null;
  }
  const r = unwrap(
    await api.PUT('/api/v1/me/devices', {
      body: { platform: process.env.EXPO_OS === 'ios' ? 'ios' : 'android', token, app_version: Constants.expoConfig?.version },
    }),
  );
  const deviceId = r.device?.id;
  if (deviceId) {
    try {
      Storage.setItemSync(DEVICE_KEY, String(deviceId));
    } catch {
      // Without it, turning arrivals off still clears the preference.
    }
  }
  return deviceId ? { id: deviceId, pushEnabled: !!r.push_enabled } : null;
}

/** Replaces one preference, keeping the rest (PUT replaces the whole set). */
async function setArrivalsPreference(on: boolean) {
  const current = unwrap(await api.GET('/api/v1/me/preferences')).preferences ?? {};
  const next = { ...current, notify_watchlist_arrivals: on || undefined };
  const saved = unwrap(await api.PUT('/api/v1/me/preferences', { body: next }));
  queryClient.setQueryData(['preferences'], saved);
}

/**
 * Turns watchlist-arrival notifications on or off for the account and this
 * device. "unavailable": this build can't receive push; "server-off": the
 * server doesn't send push (PUSH_ENABLED), though the choice is saved.
 */
export async function setWatchlistArrivals(on: boolean): Promise<ArrivalsResult> {
  if (!on) {
    await setArrivalsPreference(false);
    let deviceId: string | null = null;
    try {
      deviceId = Storage.getItemSync(DEVICE_KEY);
    } catch {
      deviceId = null;
    }
    if (deviceId) {
      await api.DELETE('/api/v1/me/devices/{id}', { params: { path: { id: Number(deviceId) } } }).catch(() => undefined);
      try {
        Storage.removeItemSync(DEVICE_KEY);
      } catch {
        // Nothing to forget.
      }
    }
    return 'off';
  }
  await ensureChannel();
  let perm = await Notifications.getPermissionsAsync();
  if (!perm.granted && perm.canAskAgain) {
    perm = await Notifications.requestPermissionsAsync({ ios: { allowAlert: true, allowSound: true, allowBadge: false } });
  }
  if (!perm.granted) return perm.canAskAgain ? 'denied' : 'blocked';
  const device = await registerDevice();
  if (!device) return 'unavailable';
  await setArrivalsPreference(true);
  return device.pushEnabled ? 'on' : 'server-off';
}

/**
 * Re-registers the token on launch when arrivals are on (tokens can change),
 * quietly: failures are retried on the next launch.
 */
export async function refreshPushRegistration(arrivalsOn: boolean) {
  if (!arrivalsOn) return;
  const perm = await Notifications.getPermissionsAsync().catch(() => null);
  if (!perm?.granted) return;
  await ensureChannel().catch(() => undefined);
  await registerDevice().catch(() => null);
}
