/**
 * Server addresses as the user types them. The public server must be HTTPS
 * (the sign-in token travels over it); development builds may use HTTP for a
 * local test server. The home URL is plain HTTP by design, but it only ever
 * gets the token after the probe proves it is the same server.
 */
export class URLError extends Error {}

export function normalizeServerURL(input: string, opts: { allowHttp: boolean }): string {
  const raw = input.trim();
  if (!raw) throw new URLError('Enter the server address.');
  const withScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(raw) ? raw : `https://${raw}`;
  let u: URL;
  try {
    u = new URL(withScheme);
  } catch {
    throw new URLError('That is not a valid address.');
  }
  if (u.protocol !== 'https:' && !(opts.allowHttp && u.protocol === 'http:')) {
    throw new URLError(opts.allowHttp ? 'Use an http:// or https:// address.' : 'The server address must start with https://.');
  }
  if (u.username || u.password) throw new URLError('Leave the user name out of the address.');
  return `${u.protocol}//${u.host}${u.pathname.replace(/\/+$/, '')}`;
}

/** The optional home-network address: http(s), and not the public server. */
export function normalizeHomeURL(input: string, server: string): string | null {
  if (!input.trim()) return null;
  const home = normalizeServerURL(input.includes('://') ? input : `http://${input.trim()}`, { allowHttp: true });
  if (home === server) throw new URLError('The home address is the same as the server address.');
  return home;
}
