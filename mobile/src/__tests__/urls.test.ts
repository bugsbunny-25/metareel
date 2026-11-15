import { normalizeHomeURL, normalizeServerURL, URLError } from '@/auth/urls';

test('server addresses', () => {
  expect(normalizeServerURL('metareel.example.com', { allowHttp: false })).toBe('https://metareel.example.com');
  expect(normalizeServerURL(' https://metareel.example.com/ ', { allowHttp: false })).toBe('https://metareel.example.com');
  expect(normalizeServerURL('https://example.com/metareel//', { allowHttp: false })).toBe('https://example.com/metareel');
  expect(() => normalizeServerURL('http://metareel.example.com', { allowHttp: false })).toThrow(URLError);
  expect(normalizeServerURL('http://localhost:18080', { allowHttp: true })).toBe('http://localhost:18080');
  expect(() => normalizeServerURL('', { allowHttp: false })).toThrow(URLError);
  expect(() => normalizeServerURL('ftp://x', { allowHttp: true })).toThrow(URLError);
  expect(() => normalizeServerURL('https://me:pw@example.com', { allowHttp: false })).toThrow(URLError);
});

test('home addresses default to http and must differ from the server', () => {
  expect(normalizeHomeURL('', 'https://a.example')).toBeNull();
  expect(normalizeHomeURL('homelab.local:5080', 'https://a.example')).toBe('http://homelab.local:5080');
  expect(normalizeHomeURL('http://192.168.1.20:5080/', 'https://a.example')).toBe('http://192.168.1.20:5080');
  expect(() => normalizeHomeURL('https://a.example', 'https://a.example')).toThrow(URLError);
});
