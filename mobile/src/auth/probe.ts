import { hmac } from '@noble/hashes/hmac.js';
import { sha256 } from '@noble/hashes/sha2.js';
import { bytesToHex, hexToBytes, utf8ToBytes } from '@noble/hashes/utils.js';

/**
 * The local-URL probe (server: POST /api/v1/auth/probe). The server answers
 * proof = HMAC-SHA256(probe_key, nonce); probe_key came back at sign-in over
 * HTTPS. Only a server that holds this session can answer, so the token is
 * never sent to a stranger answering on the home address.
 */
export function expectedProof(probeKeyHex: string, nonce: string): string {
  return bytesToHex(hmac(sha256, hexToBytes(probeKeyHex), utf8ToBytes(nonce)));
}

function sameHex(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return diff === 0;
}

export type ProbeDeps = {
  fetch: typeof fetch;
  /** 16 random bytes as hex (expo-crypto in the app, node:crypto in tests). */
  nonce: () => string;
  timeoutMs?: number;
};

/** True only when the server at homeUrl proves it holds this session. */
export async function verifyHomeServer(
  homeUrl: string,
  sessionId: number,
  probeKeyHex: string,
  deps: ProbeDeps,
): Promise<boolean> {
  const nonce = deps.nonce();
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), deps.timeoutMs ?? 1500);
  try {
    const res = await deps.fetch(`${homeUrl}/api/v1/auth/probe`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ session_id: sessionId, nonce }),
      credentials: 'omit',
      signal: ctrl.signal,
    });
    if (!res.ok) return false;
    const body = (await res.json()) as { proof?: unknown };
    return typeof body.proof === 'string' && sameHex(body.proof, expectedProof(probeKeyHex, nonce));
  } catch {
    return false;
  } finally {
    clearTimeout(timer);
  }
}
