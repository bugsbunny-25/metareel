import { useInfiniteQuery, useQuery } from '@tanstack/react-query';

import { ApiError, unwrap } from '@/api/client';
import { api } from '@/auth/session';

type Kind = 'movie' | 'tv';

/** Up next: one episode per show in progress. */
export function useUpNext(limit = 20) {
  return useQuery({
    queryKey: ['up-next', limit],
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/up-next', { params: { query: { limit } }, signal })),
  });
}

/** The account and its sessions (Settings). */
export function useAccount() {
  return useQuery({
    queryKey: ['account'],
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/account', { signal })),
  });
}

/** Server-side preferences: country, chart service, currency, services. */
export function usePreferences() {
  return useQuery({
    queryKey: ['preferences'],
    staleTime: 10 * 60_000,
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/preferences', { signal })),
    select: (d) => d.preferences ?? {},
  });
}

/** Everything about a title (metadata, ratings, where to watch, chart positions). */
export function useTitleOverview(kind: Kind, id: string, country?: string) {
  return useQuery({
    queryKey: ['title', kind, id, country ?? ''],
    // 404: metareel has no catalogue entry (never charted); the screen then
    // shows what /me/titles knows, like the web app.
    retry: (n, err) => !(err instanceof ApiError && err.status === 404) && n < 2,
    queryFn: async ({ signal }) =>
      unwrap(
        await api.GET('/api/v1/titles/tmdb/{kind}/{id}', {
          params: { path: { kind, id }, query: { country, include: 'metadata,ratings,availability,current' } },
          signal,
        }),
      ),
  });
}

/** Your state for a title (watchlist, watches) and its cached snapshot with poster colour. */
export function useMyTitle(kind: Kind, id: string, enabled = true) {
  return useQuery({
    queryKey: ['my-title', kind, id],
    enabled: enabled && !!id,
    queryFn: async ({ signal }) =>
      unwrap(await api.GET('/api/v1/me/titles/{kind}/{id}', { params: { path: { kind, id } }, signal })),
  });
}

export function useWatchlist() {
  return useQuery({
    queryKey: ['watchlist'],
    queryFn: async ({ signal }) =>
      unwrap(await api.GET('/api/v1/me/watchlist', { params: { query: { limit: 200, sort: 'position_asc' } }, signal })),
  });
}

/** The diary, newest first, cursor-paged. */
export function useDiary(limit = 40) {
  return useInfiniteQuery({
    queryKey: ['diary', limit],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) =>
      unwrap(await api.GET('/api/v1/me/watches', { params: { query: { limit, cursor: pageParam } }, signal })),
    getNextPageParam: (last) => last.next_cursor || undefined,
  });
}

export function useWatch(id: number) {
  return useQuery({
    queryKey: ['watch', id],
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/watches/{id}', { params: { path: { id } }, signal })),
  });
}

/** Every show sorted into stages (watching, stalled, not started…). */
export function useShowQueue() {
  return useQuery({
    queryKey: ['shows'],
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/shows', { signal })),
  });
}

export function useShow(id: string) {
  return useQuery({
    queryKey: ['show', id],
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/shows/{id}', { params: { path: { id } }, signal })),
  });
}

export function useSubscriptions() {
  return useQuery({
    queryKey: ['subscriptions'],
    staleTime: 10 * 60_000,
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/subscriptions', { signal })),
  });
}

export function useSavings() {
  return useQuery({
    queryKey: ['subscription-savings'],
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/subscriptions/savings', { signal })),
  });
}

/** Premium formats a watch can carry (IMAX, Dolby Atmos…). */
export function useFormats() {
  return useQuery({
    queryKey: ['formats'],
    staleTime: 24 * 60 * 60_000,
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/formats', { signal })),
  });
}

/** Today's Top 10 movies for a country and service. */
export function useTop10(country: string, provider: string) {
  return useQuery({
    queryKey: ['top10', 'movies', country, provider],
    enabled: !!country && !!provider,
    queryFn: async ({ signal }) =>
      unwrap(await api.GET('/api/v1/top10/movies/{country}/{provider}', { params: { path: { country, provider } }, signal })),
  });
}

/** TMDB title search (anything, charted or not). */
export function useTitleSearch(q: string) {
  return useQuery({
    queryKey: ['search', q, 'mixed'],
    enabled: q.trim().length >= 2,
    staleTime: 5 * 60_000,
    // The server lists every film hit before the series; interleave them so a
    // show isn't pushed out by twenty films of the same name.
    queryFn: async ({ signal }) => {
      const data = unwrap(await api.GET('/api/v1/me/titles/search', { params: { query: { q: q.trim(), limit: 40 } }, signal }));
      return { ...data, results: interleaveKinds(data.results ?? []) };
    },
  });
}

/** Films and series alternately, each in the server's order. */
export function interleaveKinds<T extends { tmdb_kind?: string }>(items: T[]): T[] {
  const movies = items.filter((i) => i.tmdb_kind !== 'tv');
  const shows = items.filter((i) => i.tmdb_kind === 'tv');
  const out: T[] = [];
  for (let i = 0; i < Math.max(movies.length, shows.length); i++) {
    if (movies[i]) out.push(movies[i]);
    if (shows[i]) out.push(shows[i]);
  }
  return out;
}
