import { backoffMs, classify, MemoryStore, NO_CONNECTION, Outbox, type OutboxItem, type OutboxOp } from '@/offline/outbox';
import { watchlistShown } from '@/offline/watchlist';

const op = (label: string): OutboxOp => ({ kind: 'watch', method: 'POST', path: '/api/v1/me/watches', body: { label }, label });

test('statuses are classified', () => {
  expect(classify(201)).toBe('done');
  expect(classify(200)).toBe('done');
  expect(classify(500)).toBe('retry');
  expect(classify(503)).toBe('retry');
  expect(classify(429)).toBe('retry');
  expect(classify(408)).toBe('retry');
  expect(classify(401)).toBe('retry'); // kept for after the next sign-in
  expect(classify(400)).toBe('reject');
  expect(classify(404)).toBe('reject');
  expect(backoffMs(1)).toBe(15_000);
  expect(backoffMs(3)).toBe(60_000);
  expect(backoffMs(50)).toBe(30 * 60_000);
});

test('items go out in order and leave the queue', async () => {
  const sent: string[] = [];
  const done: string[] = [];
  const box = new Outbox(new MemoryStore(), async (o) => (sent.push(o.label), { status: 201 }), Date.now, (o) => done.push(o.label));
  box.enqueue(op('a'));
  box.enqueue(op('b'));
  expect(box.items()).toHaveLength(2);
  await expect(box.flush()).resolves.toEqual({ sent: 2, failed: 0, waiting: 0 });
  expect(sent).toEqual(['a', 'b']);
  expect(done).toEqual(['a', 'b']);
  expect(box.items()).toHaveLength(0);
});

test('offline holds the queue in order and retries after the backoff', async () => {
  let online = false;
  let t = 1_000;
  const sent: string[] = [];
  const box = new Outbox(
    new MemoryStore(),
    async (o) => {
      if (!online) throw new TypeError('Network request failed');
      sent.push(o.label);
      return { status: 201 };
    },
    () => t,
  );
  box.enqueue(op('a'));
  box.enqueue(op('b'));
  await expect(box.flush()).resolves.toEqual({ sent: 0, failed: 0, waiting: 2 });
  expect(box.items()[0]).toMatchObject({ attempts: 1, lastError: NO_CONNECTION, nextAttemptAt: 16_000 });
  expect(box.items()[1].attempts).toBe(0); // b was never tried before a

  online = true;
  await box.flush(); // a is still backing off
  expect(sent).toEqual([]);
  t = 16_000;
  await box.flush();
  expect(sent).toEqual(['a', 'b']);
});

test('a rejected item stops, the rest still go, and it can be retried or discarded', async () => {
  let reject = true;
  const box = new Outbox(new MemoryStore(), async (o) =>
    o.label === 'bad' && reject ? { status: 400, error: 'tmdb_id must be a TMDB numeric id' } : { status: 201 },
  );
  box.enqueue(op('bad'));
  const good = box.enqueue(op('good'));
  await expect(box.flush()).resolves.toEqual({ sent: 1, failed: 1, waiting: 0 });
  const [failed] = box.items();
  expect(failed).toMatchObject({ label: 'bad', status: 'failed', lastError: 'tmdb_id must be a TMDB numeric id' });
  expect(box.items().some((i) => i.id === good)).toBe(false);

  reject = false;
  await box.retry(failed.id);
  expect(box.items()).toHaveLength(0);

  box.enqueue(op('bad'));
  reject = true;
  await box.flush();
  box.discard(box.items()[0].id);
  expect(box.items()).toHaveLength(0);
});

test('concurrent flushes share one run, and listeners hear changes', async () => {
  let calls = 0;
  const box = new Outbox(new MemoryStore(), async () => {
    calls++;
    await new Promise((r) => setTimeout(r, 5));
    return { status: 201 };
  });
  const heard = jest.fn();
  box.subscribe(heard);
  box.enqueue(op('a'));
  await Promise.all([box.flush(), box.flush()]);
  expect(calls).toBe(1);
  expect(heard).toHaveBeenCalled();
  box.enqueue(op('b'));
  box.clear();
  expect(box.items()).toHaveLength(0);
});

describe('watchlistShown', () => {
  const item = (on: boolean, status: 'pending' | 'failed' = 'pending') =>
    ({ id: 1, kind: 'watchlist', status, meta: { kind: 'movie', id: '9', on } }) as unknown as OutboxItem;

  it('uses the server value when nothing is queued for the title', () => {
    expect(watchlistShown([], 'movie', '9', true)).toBe(true);
    expect(watchlistShown([item(false)], 'tv', '9', true)).toBe(true); // another kind
  });

  it('uses the latest queued change over the server value', () => {
    expect(watchlistShown([item(true)], 'movie', '9', false)).toBe(true);
    expect(watchlistShown([item(true), item(false)], 'movie', '9', true)).toBe(false);
  });

  it('ignores failed items', () => {
    expect(watchlistShown([item(true, 'failed')], 'movie', '9', false)).toBe(false);
  });
});
