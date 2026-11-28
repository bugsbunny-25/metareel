import * as Notifications from 'expo-notifications';
import { AndroidImportance } from 'expo-notifications';
import Storage from 'expo-sqlite/kv-store';
import { useSyncExternalStore } from 'react';

import type { NowShowingProps } from './props';

// "Now showing": a film you're watching right now, shown on the lock screen
// (iOS Live Activity) or as an ongoing notification (Android) until you log
// it or stop it. Started from the log sheet; "Log it" reopens the sheet,
// whose draft (venue, formats...) was saved when it started, so it comes
// back pre-filled with the end time "just now". Local only, no server.

export type NowShowingSession = {
  kind: 'movie' | 'tv';
  id: string;
  title: string;
  poster: string;
  venue: string;
  startMs: number;
  /** Minutes; null when unknown (assumed two hours). */
  runtime: number | null;
};

const KEY = 'nowshowing.session';
const NOTIFICATION_ID = 'now-showing';
const CHANNEL = 'now-showing';
const DEFAULT_RUNTIME = 120;
/** A session is dropped this long after its expected end. */
const EXPIRE_AFTER_MS = 3 * 60 * 60 * 1000;

const listeners = new Set<() => void>();
let current: NowShowingSession | null = read();

function read(): NowShowingSession | null {
  try {
    const raw = Storage.getItemSync(KEY);
    return raw ? (JSON.parse(raw) as NowShowingSession) : null;
  } catch {
    return null;
  }
}

function save(s: NowShowingSession | null) {
  current = s;
  try {
    if (s) Storage.setItemSync(KEY, JSON.stringify(s));
    else Storage.removeItemSync(KEY);
  } catch {
    // Kept in memory only.
  }
  listeners.forEach((l) => l());
}

export function useNowShowing(): NowShowingSession | null {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => current,
  );
}

export function nowShowing(): NowShowingSession | null {
  return current;
}

export function endMs(s: NowShowingSession): number {
  return s.startMs + (s.runtime ?? DEFAULT_RUNTIME) * 60 * 1000;
}

const time = (ms: number) => new Date(ms).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' });

/** The log sheet for this title (its saved draft fills in the rest). */
export function logUrl(s: NowShowingSession): string {
  const q = new URLSearchParams({ kind: s.kind, id: s.id, title: s.title, poster: s.poster });
  return `metareel:///log?${q.toString()}`;
}

export function nowShowingProps(s: NowShowingSession, now = Date.now()): NowShowingProps {
  const end = endMs(s);
  return {
    title: s.title,
    venue: s.venue,
    startMs: s.startMs,
    endMs: end,
    startLabel: `Started ${time(s.startMs)}`,
    endLabel: `${s.runtime ? 'Ends' : 'Ends about'} ${time(end)}`,
    progress: Math.max(0, Math.min(1, (now - s.startMs) / Math.max(1, end - s.startMs))),
    logUrl: logUrl(s),
  };
}

// The native parts are loaded lazily: they need the widgets module in the
// build (a dev build made before it was added would fail at import).
async function liveActivity() {
  return (await import('./NowShowingActivity')).default;
}
async function notification() {
  return (await import('./NowShowingNotification')).default;
}

async function show(s: NowShowingSession) {
  const props = nowShowingProps(s);
  if (process.env.EXPO_OS === 'ios') {
    const factory = await liveActivity();
    const existing = factory.getInstances();
    if (existing.length) {
      await Promise.all(existing.map((a) => a.update(props)));
    } else {
      factory.start(props, props.logUrl, new Date(endMs(s) + EXPIRE_AFTER_MS));
    }
    return;
  }
  await Notifications.setNotificationChannelAsync(CHANNEL, {
    name: 'Now showing',
    description: 'The film you are watching, until you log it',
    importance: AndroidImportance.LOW,
  });
  const factory = await notification();
  const options = { channelId: CHANNEL, title: s.title, text: `${props.startLabel} · ${props.endLabel}`, ongoing: true, url: props.logUrl };
  const active = await factory.get(NOTIFICATION_ID);
  if (active) await active.update(props, options);
  else await factory.post(props, { id: NOTIFICATION_ID, ...options });
}

/** Starts (or replaces) "Now showing". */
export async function startNowShowing(s: Omit<NowShowingSession, 'startMs'> & { startMs?: number }) {
  const session: NowShowingSession = { ...s, startMs: s.startMs ?? Date.now() };
  save(session);
  await show(session).catch(() => undefined);
}

/** Ends "Now showing" (logged, stopped by hand, or long over). */
export async function stopNowShowing() {
  save(null);
  try {
    if (process.env.EXPO_OS === 'ios') {
      const factory = await liveActivity();
      await Promise.all(factory.getInstances().map((a) => a.end('immediate')));
    } else {
      await (await notification()).cancelAll();
    }
  } catch {
    // Nothing showing.
  }
}

/**
 * On foreground: ends a session that is long over, else refreshes the
 * Android notification's progress (the Live Activity counts by itself).
 */
export async function refreshNowShowing() {
  const s = current;
  if (!s) return;
  if (Date.now() > endMs(s) + EXPIRE_AFTER_MS) {
    await stopNowShowing();
    return;
  }
  if (process.env.EXPO_OS === 'android') await show(s).catch(() => undefined);
}

/** Logging the title ends its "Now showing". */
export function loggedTitle(kind: string, id: string) {
  if (current && current.kind === kind && current.id === id) void stopNowShowing();
}
