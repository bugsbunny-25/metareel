import { useQuery } from '@tanstack/react-query';

import { unwrap } from '@/api/client';
import { api } from '@/auth/session';

import { usePreferences } from './queries';
import { useScope } from './scope';

// Which titles are in a Top 10 right now in your country, for the Top 10
// badge: one /top10/{country} (every service's latest charts) per country,
// cached for half an hour and matched locally (the web's top10now). A plain
// object, not a Map: the query cache is persisted for offline use as JSON.

export type Top10Place = { rank: number; provider: string; category: 'movies' | 'tv_shows' };

export function useTop10Country(country: string) {
  return useQuery({
    queryKey: ['top10-country', 2, country],
    enabled: !!country,
    staleTime: 30 * 60_000,
    gcTime: 60 * 60_000,
    queryFn: async ({ signal }) => {
      const r = unwrap(await api.GET('/api/v1/top10/{country}', { params: { path: { country } }, signal }));
      const best: Record<string, Top10Place> = {};
      for (const [category, list] of [
        ['movies', r.movies ?? []],
        ['tv_shows', r.tv_shows ?? []],
      ] as const) {
        for (const e of list) {
          if (!e.tmdb_id || e.rank == null) continue;
          const key = `${e.kind === 'tv_show' ? 'tv' : 'movie'}:${e.tmdb_id}`;
          const prev = best[key];
          if (!prev || e.rank < prev.rank) best[key] = { rank: e.rank, provider: e.provider ?? '', category };
        }
      }
      return best;
    },
  });
}

/** The title's best place in a Top 10 in your country today, or null. */
export function useTop10Of(kind?: string | null, id?: string | null): Top10Place | null {
  const prefs = usePreferences();
  const { country } = useScope({ country: prefs.data?.country, provider: prefs.data?.provider });
  const q = useTop10Country(country);
  if (!kind || !id) return null;
  const hit = q.data?.[`${kind === 'tv_show' ? 'tv' : kind}:${id}`];
  return hit && typeof hit === 'object' && 'rank' in hit ? hit : null;
}
