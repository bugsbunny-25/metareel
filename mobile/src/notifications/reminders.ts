import * as Notifications from 'expo-notifications';
import { AndroidImportance } from 'expo-notifications';
import Storage from 'expo-sqlite/kv-store';
import { useSyncExternalStore } from 'react';

import { unwrap } from '@/api/client';
import { api, currentSession, subscribeSession } from '@/auth/session';

import { planReminders, REMINDER_DAYS, REMINDER_PREFIX } from './plan';

// New-episode reminders (phase 6a) are local notifications: the app plans
// the next two weeks from /me/upcoming and keeps the OS schedule in step on
// launch, on foreground and from the background task. Nothing is sent from
// the server.

export const EPISODE_CHANNEL = 'episodes';
const KEY = 'notify.episodes';
const listeners = new Set<() => void>();
let enabled = read();

function read(): boolean {
  try {
    return Storage.getItemSync(KEY) === 'on';
  } catch {
    return false;
  }
}

export function useEpisodeReminders(): boolean {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => enabled,
  );
}

export type PermissionResult = 'granted' | 'denied' | 'blocked';

/** Android shows the permission prompt only once a channel exists. */
async function ensureChannel() {
  if (process.env.EXPO_OS !== 'android') return;
  await Notifications.setNotificationChannelAsync(EPISODE_CHANNEL, {
    name: 'New episodes',
    description: 'A reminder on the day a new episode of a show you follow airs',
    importance: AndroidImportance.DEFAULT,
  });
}

/**
 * Turns reminders on (asking for permission when needed) or off. Returns the
 * permission outcome; "blocked" means the OS won't ask again, so the user has
 * to allow notifications in the system settings.
 */
export async function setEpisodeReminders(on: boolean): Promise<PermissionResult> {
  let result: PermissionResult = 'granted';
  if (on) {
    await ensureChannel();
    let perm = await Notifications.getPermissionsAsync();
    if (!perm.granted && perm.canAskAgain) {
      perm = await Notifications.requestPermissionsAsync({ ios: { allowAlert: true, allowSound: true, allowBadge: false } });
    }
    if (!perm.granted) result = perm.canAskAgain ? 'denied' : 'blocked';
  }
  enabled = on && result === 'granted';
  try {
    Storage.setItemSync(KEY, enabled ? 'on' : 'off');
  } catch {
    // Unsaved, it lasts until the app restarts.
  }
  listeners.forEach((l) => l());
  await syncEpisodeReminders({ force: true });
  return result;
}

/** What is scheduled now, read back from the OS after each sync. */
export type ReminderStatus = { count: number; next: { title: string; at: Date } | null };
let status: ReminderStatus = { count: 0, next: null };
const statusListeners = new Set<() => void>();

export function useReminderStatus(): ReminderStatus {
  return useSyncExternalStore(
    (l) => {
      statusListeners.add(l);
      return () => statusListeners.delete(l);
    },
    () => status,
  );
}

async function readStatus() {
  const all = await scheduledReminders();
  let next: ReminderStatus['next'] = null;
  for (const n of all) {
    // The trigger comes back in a platform-specific shape, so the time is
    // read from the data the reminder was scheduled with.
    const raw = n.content.data?.at;
    const at = typeof raw === 'string' ? new Date(raw) : null;
    if (at && !Number.isNaN(at.getTime()) && (!next || at < next.at)) next = { title: n.content.title ?? '', at };
  }
  status = { count: all.length, next };
  statusListeners.forEach((l) => l());
  return all.length;
}

let running: Promise<number> | null = null;
let lastSync = 0;
const MIN_INTERVAL = 15 * 60 * 1000;

/**
 * Brings the scheduled reminders in line with the shows' upcoming episodes.
 * Returns how many are scheduled. Throttled to once every 15 minutes unless
 * forced; failures leave the existing schedule alone.
 */
export function syncEpisodeReminders(opts: { force?: boolean } = {}): Promise<number> {
  if (running) return running;
  if (!opts.force && Date.now() - lastSync < MIN_INTERVAL) return Promise.resolve(-1);
  running = run()
    .then((n) => {
      lastSync = Date.now();
      return n;
    })
    .catch(() => -1)
    .finally(() => {
      running = null;
    });
  return running;
}

async function scheduledReminders() {
  return (await Notifications.getAllScheduledNotificationsAsync()).filter((n) => n.identifier.startsWith(REMINDER_PREFIX));
}

export async function cancelEpisodeReminders() {
  for (const n of await scheduledReminders()) {
    await Notifications.cancelScheduledNotificationAsync(n.identifier);
  }
  await readStatus();
}

async function run(): Promise<number> {
  const perm = await Notifications.getPermissionsAsync();
  if (!enabled || !perm.granted || !currentSession()) {
    await cancelEpisodeReminders();
    return 0;
  }
  await ensureChannel();
  const r = unwrap(await api.GET('/api/v1/me/upcoming', { params: { query: { days: REMINDER_DAYS + 1 } } }));
  const want = new Map(planReminders(r.episodes ?? [], new Date()).map((p) => [p.id, p]));
  for (const n of await scheduledReminders()) {
    const p = want.get(n.identifier);
    if (p && p.title === n.content.title && p.body === n.content.body && n.content.data?.at === p.at.toISOString()) {
      want.delete(n.identifier);
    } else {
      await Notifications.cancelScheduledNotificationAsync(n.identifier);
    }
  }
  for (const p of want.values()) {
    await Notifications.scheduleNotificationAsync({
      identifier: p.id,
      content: { title: p.title, body: p.body, data: { url: p.url, at: p.at.toISOString() } },
      trigger: { type: Notifications.SchedulableTriggerInputTypes.DATE, date: p.at, channelId: EPISODE_CHANNEL },
    });
  }
  return readStatus();
}

// Signing out removes this account's reminders from the device.
subscribeSession(() => {
  if (!currentSession()) void cancelEpisodeReminders().catch(() => undefined);
});
