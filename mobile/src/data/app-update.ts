import { useQuery } from '@tanstack/react-query';
import Constants from 'expo-constants';
import { Platform } from 'react-native';

// Android installs are sideloaded APKs from this repo's GitHub Releases
// (mobile-v* tags). JS changes arrive through EAS Update; a new native build
// needs a new APK, so the app checks for one at most once a day.
const RELEASES = 'https://api.github.com/repos/bugsbunny-25/metareel/releases?per_page=20';

/** "0.10.2" > "0.9.9". */
export function newer(a: string, b: string): boolean {
  const pa = a.split('.').map(Number);
  const pb = b.split('.').map(Number);
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const d = (pa[i] ?? 0) - (pb[i] ?? 0);
    if (d) return d > 0;
  }
  return false;
}

type Release = { tag_name: string; draft: boolean; assets: { name: string; browser_download_url: string }[] };

export function latestApk(releases: Release[], current: string): { version: string; url: string } | null {
  for (const r of releases) {
    const m = /^mobile-v(\d+\.\d+\.\d+)$/.exec(r.tag_name);
    if (!m || r.draft) continue;
    const apk = r.assets.find((a) => a.name.endsWith('.apk'));
    if (apk && newer(m[1], current)) return { version: m[1], url: apk.browser_download_url };
  }
  return null;
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
      return latestApk((await res.json()) as Release[], Constants.expoConfig?.version ?? '0.0.0');
    },
  });
}
