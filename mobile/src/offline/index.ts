import Storage from 'expo-sqlite/kv-store';
import { useSyncExternalStore } from 'react';

import { apiRequest } from '@/api/client';
import { apiFetch, currentSession, subscribeSession } from '@/auth/session';
import { queryClient } from '@/query/client';
import { refreshWidgets } from '@/widgets/sync';

import { Outbox, type OutboxItem, type OutboxKind, type OutboxOp } from './outbox';
import { SQLiteStore } from './sqlite-store';

export type { OutboxItem, OutboxKind, OutboxOp };
export { watchlistShown } from './watchlist';

async function send(op: OutboxOp) {
  const res = await apiFetch(apiRequest(op.method, op.path, op.body));
  if (res.ok && op.kind === 'watchlist') await settleWatchlist(op);
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

/**
 * A watchlist change shows its queued value until it's sent (see
 * watchlistShown). Before the item leaves the queue, refetch what that title's
 * screens read, so the button goes from the queued value straight to the
 * server's instead of flashing back to the old one in between.
 */
async function settleWatchlist(op: OutboxOp) {
  const { kind, id } = (op.meta ?? {}) as { kind?: string; id?: string };
  if (!kind || !id) return;
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ['my-title', kind, id] }),
    kind === 'tv' ? queryClient.invalidateQueries({ queryKey: ['show', id] }) : null,
    queryClient.invalidateQueries({ queryKey: ['watchlist'] }),
  ]).catch(() => undefined);
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
