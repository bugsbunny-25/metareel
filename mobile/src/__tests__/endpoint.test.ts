import { makeApiFetch } from '@/api/client';
import { Endpoint } from '@/net/endpoint';

const PUB = 'https://metareel.example.com';
const HOME = 'http://homelab.local:5080';

test('the home URL is used only once verified', async () => {
  let ok = false;
  const ep = new Endpoint(async () => ok);
  ep.configure(PUB, HOME);
  expect(ep.base()).toBe(PUB);
  await ep.refresh();
  expect(ep.base()).toBe(PUB);
  ok = true;
  await ep.refresh();
  expect(ep.base()).toBe(HOME);
  ep.homeFailed();
  expect(ep.base()).toBe(PUB);
});

test('a stale check does not override a newer configuration', async () => {
  let release: (v: boolean) => void = () => {};
  const ep = new Endpoint(() => new Promise<boolean>((r) => (release = r)));
  ep.configure(PUB, HOME);
  const pending = ep.refresh();
  ep.configure(null, null); // signed out meanwhile
  release(true);
  await pending;
  expect(ep.base()).toBeNull();
});

function request(path: string, init?: RequestInit) {
  return new Request(`https://metareel.invalid${path}`, init);
}

test('requests carry the bearer token and no cookies, to the current base', async () => {
  const ep = new Endpoint(async () => true);
  ep.configure(PUB, null);
  const fetch = jest.fn(async (_u: RequestInfo | URL, _i?: RequestInit) => new Response('{}'));
  const apiFetch = makeApiFetch({ endpoint: ep, token: () => 'mrs_tok', onUnauthorized: jest.fn(), fetch });
  await apiFetch(request('/api/v1/me/watches', { method: 'POST', body: '{"a":1}' }));
  const [url, init] = fetch.mock.calls[0];
  expect(url).toBe(`${PUB}/api/v1/me/watches`);
  expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer mrs_tok');
  expect(init?.credentials).toBe('omit');
  expect(init?.body).toBe('{"a":1}');
});

test('a network failure on the home URL retries once on the public URL', async () => {
  const ep = new Endpoint(async () => true);
  ep.configure(PUB, HOME);
  await ep.refresh();
  const fetch = jest.fn(async (u: RequestInfo | URL) => {
    if (String(u).startsWith(HOME)) throw new TypeError('Network request failed');
    return new Response('{}');
  });
  const apiFetch = makeApiFetch({ endpoint: ep, token: () => 't', onUnauthorized: jest.fn(), fetch });
  const res = await apiFetch(request('/api/v1/me/up-next'));
  expect(res.status).toBe(200);
  expect(fetch.mock.calls.map((c) => c[0])).toEqual([`${HOME}/api/v1/me/up-next`, `${PUB}/api/v1/me/up-next`]);
  expect(ep.getState().route).toBe('public');
});

test('a 401 ends the session locally', async () => {
  const ep = new Endpoint(async () => false);
  ep.configure(PUB, null);
  const onUnauthorized = jest.fn();
  const apiFetch = makeApiFetch({ endpoint: ep, token: () => 't', onUnauthorized, fetch: async () => new Response('{}', { status: 401 }) });
  await apiFetch(request('/api/v1/me/up-next'));
  expect(onUnauthorized).toHaveBeenCalledTimes(1);
});
