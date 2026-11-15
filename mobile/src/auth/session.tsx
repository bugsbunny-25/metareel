import * as Crypto from 'expo-crypto';
import * as Device from 'expo-device';
import * as SecureStore from 'expo-secure-store';
import { createContext, type PropsWithChildren, useContext, useEffect, useMemo, useSyncExternalStore } from 'react';

import { makeApi, makeApiFetch } from '@/api/client';
import { Endpoint } from '@/net/endpoint';
import { ensureLocalNetworkAccess } from '@/net/local-network';
import { clearQueryCache } from '@/query/client';

import { verifyHomeServer } from './probe';

/** What the app keeps after signing in (in the Keychain / Keystore). */
export type StoredSession = {
  server: string;
  home: string | null;
  token: string;
  sessionId: number;
  probeKey: string;
  expiresAt: string;
  user: { id: number; username: string; role: string };
};

export type SessionState =
  | { status: 'loading' }
  | { status: 'signedOut'; notice?: string }
  | { status: 'signedIn'; session: StoredSession };

const STORE_KEY = 'metareel.session';

// --- the session store (outside React, so the API client can read the token)

let state: SessionState = { status: 'loading' };
const listeners = new Set<() => void>();
function setState(next: SessionState) {
  state = next;
  listeners.forEach((l) => l());
}
const current = () => (state.status === 'signedIn' ? state.session : null);

export const endpoint = new Endpoint((homeUrl) => {
  const s = current();
  if (!s) return Promise.resolve(false);
  return verifyHomeServer(homeUrl, s.sessionId, s.probeKey, {
    fetch,
    nonce: () => Array.from(Crypto.getRandomBytes(16), (b) => b.toString(16).padStart(2, '0')).join(''),
  });
});

/** Sends a request to the current server with the session token (see api/client). */
export const apiFetch = makeApiFetch({
  endpoint,
  token: () => current()?.token ?? null,
  onUnauthorized: () => {
    void signOutLocally('Your session ended. Sign in again.');
  },
});

export const api = makeApi(apiFetch);

/** The signed-in session, or null (for code outside React). */
export const currentSession = current;

/** Hears every session change (sign-in, sign-out, home URL). */
export function subscribeSession(l: () => void) {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

async function persist(s: StoredSession | null) {
  if (s) await SecureStore.setItemAsync(STORE_KEY, JSON.stringify(s));
  else await SecureStore.deleteItemAsync(STORE_KEY);
}

export async function restoreSession() {
  try {
    const raw = await SecureStore.getItemAsync(STORE_KEY);
    const s = raw ? (JSON.parse(raw) as StoredSession) : null;
    if (s && new Date(s.expiresAt).getTime() > Date.now()) {
      setState({ status: 'signedIn', session: s });
      endpoint.configure(s.server, s.home);
      return;
    }
  } catch {
    // An unreadable session is the same as none.
  }
  setState({ status: 'signedOut' });
}

export class SignInError extends Error {}

export async function signIn(input: { server: string; home: string | null; username: string; password: string }) {
  let res: Response;
  try {
    res = await fetch(`${input.server}/api/v1/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'omit',
      body: JSON.stringify({
        username: input.username.trim(),
        password: input.password,
        device_name: Device.modelName ?? Device.deviceName ?? 'Phone',
        remember_device: true,
      }),
    });
  } catch {
    throw new SignInError("Couldn't reach the server. Check the address and your connection.");
  }
  if (res.status === 401) throw new SignInError('Wrong username or password.');
  if (res.status === 429) throw new SignInError('Too many attempts. Wait a minute and try again.');
  if (!res.ok) throw new SignInError(`The server answered ${res.status}. Is this a metareel server?`);
  const body = (await res.json()) as {
    token: string;
    expires_at: string;
    session_id: number;
    probe_key: string;
    user: StoredSession['user'];
  };
  if (!body.token || !body.session_id || !body.probe_key) {
    throw new SignInError('This server is too old for the app. Update metareel first.');
  }
  const s: StoredSession = {
    server: input.server,
    home: input.home,
    token: body.token,
    sessionId: body.session_id,
    probeKey: body.probe_key,
    expiresAt: body.expires_at,
    user: body.user,
  };
  await persist(s);
  setState({ status: 'signedIn', session: s });
  if (s.home) await ensureLocalNetworkAccess();
  endpoint.configure(s.server, s.home);
}

/** Changes (or clears) the home-network address of the signed-in session. */
export async function setHomeURL(home: string | null) {
  const s = current();
  if (!s) return;
  const next = { ...s, home };
  if (home) await ensureLocalNetworkAccess();
  await persist(next);
  setState({ status: 'signedIn', session: next });
  endpoint.configure(next.server, next.home);
}

async function signOutLocally(notice?: string) {
  if (state.status !== 'signedIn') return;
  setState({ status: 'signedOut', notice });
  endpoint.configure(null, null);
  await Promise.allSettled([persist(null), clearQueryCache()]);
}

/** Ends the session on the server (best effort) and forgets it here. */
export async function signOut() {
  if (current()) {
    const ctrl = new AbortController();
    const t = setTimeout(() => ctrl.abort(), 3000);
    await api.POST('/api/v1/auth/logout', { signal: ctrl.signal }).catch(() => undefined);
    clearTimeout(t);
  }
  await signOutLocally();
}

// --- React access

const SessionContext = createContext<SessionState>({ status: 'loading' });

export function SessionProvider({ children }: PropsWithChildren) {
  const value = useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => state,
  );
  useEffect(() => {
    if (state.status === 'loading') void restoreSession();
  }, []);
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionState {
  return useContext(SessionContext);
}

/** The signed-in session; only for screens behind the sign-in guard. */
export function useSignedIn(): StoredSession {
  const s = useSession();
  return useMemo(() => {
    if (s.status !== 'signedIn') throw new Error('useSignedIn outside the signed-in screens');
    return s.session;
  }, [s]);
}
