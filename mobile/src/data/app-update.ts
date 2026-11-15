import { useQuery } from '@tanstack/react-query';
import Constants from 'expo-constants';
import { Platform } from 'react-native';

import { parseVersion, versionCode } from '../../version.ts';

// Android installs are sideloaded APKs attached to this repo's GitHub Releases
// (v* tags, the same version as the server). JS changes arrive through EAS
// Update; a new native build needs a new APK, so the app checks for one at
// most once a day. A release is newer when its build number (version.ts) is higher.
const RELEASES = 'https://api.github.com/repos/bugsbunny-25/metareel/releases?per_page=20';

type Release = { tag_name: string; draft: boolean; assets: { name: string; browser_download_url: string }[] };

/** This build's version (app.config.ts, from package.json). */
export function appVersion(): string {
  return Constants.expoConfig?.version ?? '0.0.0';
}

function code(v: string): number | null {
  try {
    parseVersion(v);
    return versionCode(v);
  } catch {
    return null; // a tag outside the scheme (the old -alpha ones)
  }
}

export function latestApk(releases: Release[], current: string): { version: string; url: string } | null {
  const mine = code(current) ?? 0;
  let best: { version: string; url: string; code: number } | null = null;
  for (const r of releases) {
    const c = code(r.tag_name);
    const apk = r.assets.find((a) => a.name.endsWith('.apk'));
    if (r.draft || c === null || !apk || c <= mine || (best && c <= best.code)) continue;
    best = { version: r.tag_name.replace(/^v/, ''), url: apk.browser_download_url, code: c };
  }
  return best && { version: best.version, url: best.url };
}

export function useAppUpdate() {
  return useQuery({
    queryKey: ['app-update'],
    enabled: Platform.OS === 'android',
    staleTime: 24 * 60 * 60_000,
    retry: false,
    queryFn: async ({ signal }) => {
      const res = await fetch(RELEASES, { signal, credentials: 'omit', headers: { Accept: 'application/vnd.github+json' } });
      if (!res.ok) return null;
      return latestApk((await res.json()) as Release[], appVersion());
    },
  });
}
