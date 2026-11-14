import Storage from 'expo-sqlite/kv-store';
import { useSyncExternalStore } from 'react';

import { apiRequest } from '@/api/client';
import { apiFetch, currentSession, subscribeSession } from '@/auth/session';
import { queryClient } from '@/query/client';
import { refreshWidgets } from '@/widgets/sync';

import { Outbox, type OutboxItem, type OutboxKind, type OutboxOp } from './outbox';
import { SQLiteStore } from './sqlite-store';

export type { OutboxItem, OutboxKind, OutboxOp };

async function send(op: OutboxOp) {
  const res = await apiFetch(apiRequest(op.method, op.path, op.body));
  let error: string | undefined;
  if (!res.ok) {
    try {
      error = ((await res.json()) as { error?: string }).error;
    } catch {
      // No JSON body: the status says enough.
    }
  }
  return { status: res.status, error };
}

export const outbox = new Outbox(new SQLiteStore(), send, Date.now, () => {
  // Something reached the server: let every screen refetch what it shows.
  void queryClient.invalidateQueries();
});

/** Sends what is queued, when signed in. Safe to call often. */
export function flushOutbox() {
  if (!currentSession()) return Promise.resolve();
  return outbox.flush().then((r) => {
    // A log or a tick changes what the widgets show.
    if (r.sent) void refreshWidgets({ force: true });
  });
}

/** Queue a write and try to send it right away. */
export function enqueue(op: OutboxOp) {
  outbox.enqueue(op);
  void flushOutbox();
}

// Queued writes belong to the account that made them: when a different
// user signs in, they are dropped; the same user gets them sent.
const OWNER_KEY = 'outbox.owner';
subscribeSession(() => {
  const s = currentSession();
  if (!s) return;
  const owner = `${s.server}#${s.user.id}`;
  let previous: string | null = null;
  try {
    previous = Storage.getItemSync(OWNER_KEY);
    if (previous !== owner) Storage.setItemSync(OWNER_KEY, owner);
  } catch {
    // Without the owner record, keep the queue rather than lose writes.
  }
  if (previous && previous !== owner) outbox.clear();
  void flushOutbox();
});

export function useOutbox(): OutboxItem[] {
  return useSyncExternalStore(outbox.subscribe, outbox.items);
}
