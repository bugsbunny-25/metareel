/**
 * The offline outbox (docs/mobile-app-plan.md, section 3). Writes that are
 * safe to repeat are queued here and sent in order: a watch log carries a
 * client_key the server dedupes on, marking episodes skips ones already
 * watched, and watchlist add/remove are idempotent PUT/DELETE. Network
 * errors, 401, 408, 429 and 5xx are retried with backoff and hold back the rest
 * of the queue (order matters: you log a film, then rate the next); any
 * other 4xx stops that item and shows it as "couldn't sync".
 *
 * The queue logic is separate from its storage (SQLite in the app, memory
 * in tests).
 */

export type OutboxKind = 'watch' | 'episodes' | 'watchlist';

export type OutboxOp = {
  kind: OutboxKind;
  method: 'POST' | 'PUT' | 'DELETE';
  path: string;
  body?: unknown;
  /** What the user sees: "Dune: Part Two", "Severance S2 E3". */
  label: string;
  /** Display data for pending rows (poster, title, ended_at…). */
  meta?: Record<string, unknown>;
};

export type OutboxItem = OutboxOp & {
  id: number;
  createdAt: number;
  attempts: number;
  nextAttemptAt: number;
  lastError: string | null;
  status: 'pending' | 'failed';
};

export interface OutboxStore {
  insert(item: Omit<OutboxItem, 'id'>): number;
  all(): OutboxItem[];
  update(id: number, patch: Partial<Omit<OutboxItem, 'id'>>): void;
  remove(id: number): void;
  clear(): void;
}

/** What sending one item produced: an HTTP status, or a thrown network error. */
export type Send = (op: OutboxOp) => Promise<{ status: number; error?: string }>;

export type Outcome = 'done' | 'retry' | 'reject';

export const NO_CONNECTION = 'No connection; it is sent when you are back online.';

export function classify(status: number): Outcome {
  if (status >= 200 && status < 300) return 'done';
  // 401: the session ended; keep the item for after the next sign-in.
  if (status === 401 || status === 408 || status === 429 || status >= 500 || status === 0) return 'retry';
  return 'reject';
}

/** 15 s, 30 s, 1 min … capped at 30 min. */
export function backoffMs(attempts: number): number {
  return Math.min(15_000 * 2 ** Math.max(0, attempts - 1), 30 * 60_000);
}

export class Outbox {
  private listeners = new Set<() => void>();
  private snapshot: OutboxItem[];
  private running: Promise<FlushResult> | null = null;

  constructor(
    private store: OutboxStore,
    private send: Send,
    private now: () => number = Date.now,
    private onDone: (op: OutboxOp) => void = () => {},
  ) {
    this.snapshot = store.all();
  }

  items = (): OutboxItem[] => this.snapshot;

  subscribe = (l: () => void) => {
    this.listeners.add(l);
    return () => {
      this.listeners.delete(l);
    };
  };

  private changed() {
    this.snapshot = this.store.all();
    this.listeners.forEach((l) => l());
  }

  enqueue(op: OutboxOp): number {
    const id = this.store.insert({
      ...op,
      createdAt: this.now(),
      attempts: 0,
      nextAttemptAt: 0,
      lastError: null,
      status: 'pending',
    });
    this.changed();
    return id;
  }

  /** Sends what is due, in order. Concurrent calls share one run. */
  flush(): Promise<FlushResult> {
    if (!this.running) {
      this.running = this.run().finally(() => {
        this.running = null;
      });
    }
    return this.running;
  }

  private async run(): Promise<FlushResult> {
    const result: FlushResult = { sent: 0, failed: 0, waiting: 0 };
    for (const item of this.store.all()) {
      if (item.status !== 'pending') continue;
      if (item.nextAttemptAt > this.now()) {
        result.waiting = this.pendingCount();
        return result; // keep the order: nothing after a waiting item goes first
      }
      let outcome: Outcome;
      let error: string | undefined;
      try {
        const res = await this.send(item);
        outcome = classify(res.status);
        error = res.error ?? (outcome === 'done' ? undefined : `HTTP ${res.status}`);
      } catch {
        // A thrown fetch is the network (offline, server unreachable);
        // the platform's error text means nothing to the user.
        outcome = 'retry';
        error = NO_CONNECTION;
      }
      if (outcome === 'done') {
        this.store.remove(item.id);
        result.sent++;
        this.onDone(item);
        this.changed();
        continue;
      }
      const attempts = item.attempts + 1;
      if (outcome === 'reject') {
        this.store.update(item.id, { status: 'failed', attempts, lastError: error ?? null });
        result.failed++;
        this.changed();
        continue;
      }
      this.store.update(item.id, { attempts, lastError: error ?? null, nextAttemptAt: this.now() + backoffMs(attempts) });
      this.changed();
      result.waiting = this.pendingCount();
      return result;
    }
    result.waiting = this.pendingCount();
    return result;
  }

  private pendingCount() {
    return this.store.all().filter((i) => i.status === 'pending').length;
  }

  /** Try a failed or waiting item again now. */
  retry(id: number) {
    this.store.update(id, { status: 'pending', nextAttemptAt: 0 });
    this.changed();
    return this.flush();
  }

  discard(id: number) {
    this.store.remove(id);
    this.changed();
  }

  /** Forget everything (sign-out: queued writes belong to that account). */
  clear() {
    this.store.clear();
    this.changed();
  }
}

export type FlushResult = { sent: number; failed: number; waiting: number };

/** An OutboxStore in memory (tests). */
export class MemoryStore implements OutboxStore {
  private rows: OutboxItem[] = [];
  private next = 1;
  insert(item: Omit<OutboxItem, 'id'>) {
    const id = this.next++;
    this.rows.push({ ...item, id });
    return id;
  }
  all() {
    return this.rows.map((r) => ({ ...r }));
  }
  update(id: number, patch: Partial<Omit<OutboxItem, 'id'>>) {
    this.rows = this.rows.map((r) => (r.id === id ? { ...r, ...patch } : r));
  }
  remove(id: number) {
    this.rows = this.rows.filter((r) => r.id !== id);
  }
  clear() {
    this.rows = [];
  }
}
