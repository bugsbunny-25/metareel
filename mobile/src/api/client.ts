import createClient from 'openapi-fetch';

import type { Endpoint } from '@/net/endpoint';

import type { paths } from './schema';

// openapi-fetch needs a base URL; the real one is chosen per request (public
// or verified home URL), so requests are built against this placeholder and
// re-targeted in apiFetch.
const PLACEHOLDER = 'https://metareel.invalid';

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

export type ApiDeps = {
  endpoint: Endpoint;
  token: () => string | null;
  /** The server refused the session (expired or revoked). */
  onUnauthorized: () => void;
  fetch?: typeof fetch;
};

/**
 * Sends a request to the current server with the session token. The token
 * is the only credential: cookies are never stored or sent. When the home
 * URL fails at the network level, the request is retried once on the public
 * URL and the home URL is dropped until the next successful probe.
 */
export function makeApiFetch(deps: ApiDeps) {
  const doFetch = deps.fetch ?? fetch;
  return async (input: Request): Promise<Response> => {
    const base = deps.endpoint.base();
    if (!base) throw new ApiError(0, 'Not signed in');
    const path = input.url.startsWith(PLACEHOLDER) ? input.url.slice(PLACEHOLDER.length) : input.url;
    const headers = new Headers(input.headers);
    const token = deps.token();
    if (token) headers.set('Authorization', `Bearer ${token}`);
    const body = input.method === 'GET' || input.method === 'HEAD' ? undefined : await input.text();
    const send = (b: string) => doFetch(b + path, { method: input.method, headers, body, credentials: 'omit' });

    let res: Response;
    try {
      res = await send(base);
    } catch (err) {
      const pub = deps.endpoint.getState().publicUrl;
      if (deps.endpoint.getState().route !== 'home' || !pub) throw err;
      deps.endpoint.homeFailed();
      res = await send(pub);
    }
    if (res.status === 401 && token) deps.onUnauthorized();
    return res;
  };
}

export type ApiFetch = ReturnType<typeof makeApiFetch>;

export function makeApi(apiFetch: ApiFetch) {
  return createClient<paths>({ baseUrl: PLACEHOLDER, fetch: apiFetch });
}

export type Api = ReturnType<typeof makeApi>;

/** A request built for apiFetch from a path ("/api/v1/me/watches") and a JSON body. */
export function apiRequest(method: string, path: string, body?: unknown): Request {
  return new Request(PLACEHOLDER + path, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

/** Unwraps an openapi-fetch result: the data, or an ApiError with the server's message. */
export function unwrap<T>(r: { data?: T; error?: unknown; response: Response }): T {
  if (r.error !== undefined || r.data === undefined) {
    const msg =
      typeof r.error === 'object' && r.error && 'error' in r.error ? String((r.error as { error: unknown }).error) : r.response.statusText;
    throw new ApiError(r.response.status, msg || `HTTP ${r.response.status}`);
  }
  return r.data;
}
