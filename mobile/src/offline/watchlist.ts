import type { OutboxItem } from './outbox';

/**
 * Whether a title shows as on the watchlist: the last queued change for it
 * when one is waiting to be sent, else what the server said.
 */
export function watchlistShown(items: OutboxItem[], kind: string, id: string, server: boolean): boolean {
  const queued = items.filter((i) => i.kind === 'watchlist' && i.meta?.kind === kind && i.meta?.id === id && i.status === 'pending');
  return queued.length ? !!queued[queued.length - 1].meta?.on : server;
}
