import { createHmac, randomBytes } from 'node:crypto';

import { expectedProof, verifyHomeServer } from '@/auth/probe';

const probeKey = randomBytes(32).toString('hex');
const nonceFn = () => 'abcdef0123456789abcdef0123456789';

// What the Go server computes (internal/service/session_probe.go).
function serverProof(keyHex: string, nonce: string) {
  return createHmac('sha256', Buffer.from(keyHex, 'hex')).update(nonce).digest('hex');
}

function server(answer: (body: { session_id: number; nonce: string }) => Response | Promise<Response>) {
  return jest.fn(async (_url: RequestInfo | URL, init?: RequestInit) => answer(JSON.parse(String(init?.body))));
}

test('expectedProof matches the server HMAC', () => {
  expect(expectedProof(probeKey, 'some-nonce-1234567')).toBe(serverProof(probeKey, 'some-nonce-1234567'));
});

test('the real server is verified, without sending any credentials', async () => {
  const fetch = server((b) => Response.json({ proof: serverProof(probeKey, b.nonce) }));
  await expect(verifyHomeServer('http://homelab.local:5080', 7, probeKey, { fetch, nonce: nonceFn })).resolves.toBe(true);
  const [url, init] = fetch.mock.calls[0];
  expect(url).toBe('http://homelab.local:5080/api/v1/auth/probe');
  expect(JSON.parse(String(init?.body))).toEqual({ session_id: 7, nonce: nonceFn() });
  expect(init?.credentials).toBe('omit');
  expect(JSON.stringify(init?.headers)).not.toMatch(/authorization/i);
});

test('anything else is refused', async () => {
  const other = randomBytes(32).toString('hex');
  const cases: ((b: { nonce: string }) => Response)[] = [
    (b) => Response.json({ proof: serverProof(other, b.nonce) }), // another server
    () => Response.json({ proof: 'not-hex' }),
    () => Response.json({}),
    () => new Response('nope', { status: 500 }),
  ];
  for (const answer of cases) {
    await expect(verifyHomeServer('http://h', 1, probeKey, { fetch: server(answer), nonce: nonceFn })).resolves.toBe(false);
  }
  const offline = jest.fn(async () => {
    throw new TypeError('Network request failed');
  });
  await expect(verifyHomeServer('http://h', 1, probeKey, { fetch: offline, nonce: nonceFn })).resolves.toBe(false);
});

test('a server that never answers times out', async () => {
  const hang = jest.fn(
    (_u: RequestInfo | URL, init?: RequestInit) =>
      new Promise<Response>((_, reject) => init?.signal?.addEventListener('abort', () => reject(new Error('aborted')))),
  );
  await expect(verifyHomeServer('http://h', 1, probeKey, { fetch: hang, nonce: nonceFn, timeoutMs: 20 })).resolves.toBe(false);
});
