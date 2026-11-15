jest.mock('expo-constants', () => ({}));

// eslint-disable-next-line import/first -- the mock above must come first
import { latestApk } from '@/data/app-update';

const asset = (n: string) => ({ name: n, browser_download_url: `https://example.invalid/${n}` });

test('the newest release with an APK is offered; drafts, APK-less and other tags are ignored', () => {
  const releases = [
    { tag_name: 'v0.7.0', draft: true, assets: [asset('metareel-v0.7.0.apk')] },
    { tag_name: 'v0.6.1', draft: false, assets: [asset('metareel-v0.6.1.sbom.cdx.json')] },
    { tag_name: 'v0.6.0', draft: false, assets: [asset('metareel-v0.6.0.apk')] },
    { tag_name: 'v0.10.0-alpha', draft: false, assets: [asset('metareel-v0.10.0-alpha.apk')] },
    { tag_name: 'v0.5.0', draft: false, assets: [asset('metareel-v0.5.0.apk')] },
  ];
  expect(latestApk(releases, '0.5.0')).toEqual({ version: '0.6.0', url: 'https://example.invalid/metareel-v0.6.0.apk' });
  expect(latestApk(releases, '0.6.0')).toBeNull();
});
