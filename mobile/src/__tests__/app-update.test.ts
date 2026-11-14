jest.mock('expo-constants', () => ({}));

// eslint-disable-next-line import/first -- the mock above must come first
import { latestApk, newer } from '@/data/app-update';

test('versions compare numerically', () => {
  expect(newer('0.10.0', '0.9.9')).toBe(true);
  expect(newer('0.1.0', '0.1.0')).toBe(false);
  expect(newer('0.1', '0.1.1')).toBe(false);
});

test('the newest mobile APK release is offered, other tags ignored', () => {
  const asset = (n: string) => ({ name: n, browser_download_url: `https://example.invalid/${n}` });
  const releases = [
    { tag_name: 'v2.0.0', draft: false, assets: [asset('server.tar.gz')] },
    { tag_name: 'mobile-v0.3.0', draft: true, assets: [asset('metareel-v0.3.0.apk')] },
    { tag_name: 'mobile-v0.2.0', draft: false, assets: [asset('metareel-v0.2.0.apk')] },
    { tag_name: 'mobile-v0.1.0', draft: false, assets: [asset('metareel-v0.1.0.apk')] },
  ];
  expect(latestApk(releases, '0.1.0')).toEqual({ version: '0.2.0', url: 'https://example.invalid/metareel-v0.2.0.apk' });
  expect(latestApk(releases, '0.2.0')).toBeNull();
});
