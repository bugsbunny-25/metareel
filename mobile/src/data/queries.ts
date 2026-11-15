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
export function usePreferences(enabled = true) {
  return useQuery({
    queryKey: ['preferences'],
    enabled,
    // Short: a change made on another device (the country order) shows up here soon.
    staleTime: 30_000,
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
          params: { path: { kind, id }, query: { country, include: 'metadata,stats,ratings,availability,current' } },
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
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/titles/{kind}/{id}', { params: { path: { kind, id } }, signal })),
  });
}

export function useWatchlist() {
  return useQuery({
    queryKey: ['watchlist'],
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/watchlist', { params: { query: { limit: 200, sort: 'position_asc' } }, signal })),
  });
}

/** The diary, newest first, cursor-paged. */
export function useDiary(limit = 40) {
  return useInfiniteQuery({
    queryKey: ['diary', limit],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) => unwrap(await api.GET('/api/v1/me/watches', { params: { query: { limit, cursor: pageParam } }, signal })),
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
    // Short: a plan added on the web (or from the log sheet's link) shows up soon.
    staleTime: 30_000,
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

/** Today's Top 10 movies or shows for a country and service. */
/** One Top 10: the latest, or a past day (date YYYY-MM-DD). */
export function useTop10(country: string, provider: string, category: 'movies' | 'tv-shows' = 'movies', date?: string) {
  return useQuery({
    queryKey: ['top10', category, country, provider, date ?? ''],
    enabled: !!country && !!provider,
    queryFn: async ({ signal }) => {
      const params = { path: { country, provider }, query: date ? { date } : {} };
      return unwrap(
        category === 'movies'
          ? await api.GET('/api/v1/top10/movies/{country}/{provider}', { params, signal })
          : await api.GET('/api/v1/top10/tv-shows/{country}/{provider}', { params, signal }),
      );
    },
  });
}

/** The days a country and service have charts for, newest first. */
export function useChartDates(country: string, provider: string) {
  return useQuery({
    queryKey: ['chart-dates', country, provider],
    enabled: !!country && !!provider,
    staleTime: 30 * 60_000,
    queryFn: async ({ signal }) =>
      unwrap(await api.GET('/api/v1/charts/{country}/{provider}/dates', { params: { path: { country, provider }, query: { limit: 366 } }, signal })),
  });
}

/** What is trending on TMDB today (the server asks TMDB and keeps it half an hour). */
export function useTrending() {
  return useQuery({
    queryKey: ['trending'],
    staleTime: 30 * 60_000,
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/titles/trending', { signal })),
  });
}

/** Films in cinemas in a country, from TMDB's now playing. */
export function useInCinemas(country: string) {
  return useQuery({
    queryKey: ['in-cinemas', country],
    enabled: !!country,
    staleTime: 30 * 60_000,
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/titles/in-cinemas/{country}', { params: { path: { country } }, signal })),
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

/** Episodes of your shows still to air, soonest first. */
export function useUpcoming(days = 180) {
  return useQuery({
    queryKey: ['upcoming', days],
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/upcoming', { params: { query: { days } }, signal })),
  });
}

/** Streaming services in a country (names for the log sheet's service picker). */
export function useServices(country?: string) {
  return useQuery({
    queryKey: ['services', country ?? ''],
    enabled: !!country,
    staleTime: 24 * 60 * 60_000,
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/services/{country}', { params: { path: { country: country! } }, signal })),
  });
}

/** Your latest watches of one kind (films or episodes), for the diary's recent rows. */
export function useRecentWatches(kind: Kind, limit: number) {
  return useQuery({
    queryKey: ['diary', 'recent', kind, limit],
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/watches', { params: { query: { tmdb_kind: kind, limit } }, signal })),
  });
}

/** How you rate: the half-star distribution. */
export function useRatingStats() {
  return useQuery({
    queryKey: ['stats', 'ratings'],
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/stats/ratings', { signal })),
  });
}

/** Totals over a range (all time without one). */
export function useStatsSummary(from?: string) {
  return useQuery({
    queryKey: ['stats', 'summary', from ?? ''],
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/stats/summary', { params: { query: from ? { from } : {} }, signal })),
  });
}

/** One episode's details and your watches of it. */
export function useEpisode(show: string, season: number, episode: number) {
  return useQuery({
    queryKey: ['episode', show, season, episode],
    enabled: !!show && Number.isFinite(season) && Number.isFinite(episode),
    queryFn: async ({ signal }) =>
      unwrap(await api.GET('/api/v1/me/shows/{id}/episodes/{season}/{episode}', { params: { path: { id: show, season, episode } }, signal })),
  });
}

/** The profile's statistics for shows ('tv'), films ('movie') or both (''). */
export function useDetailedStats(kind: '' | 'tv' | 'movie') {
  return useQuery({
    queryKey: ['stats', 'detailed', kind],
    staleTime: 5 * 60_000,
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/stats/detailed', { params: { query: { kind } }, signal })),
  });
}

/** Where a title streams in one country (Where to watch, for a country other than your default). */
export function useTitleAvailability(kind: Kind, id: string, country: string | undefined, enabled = true) {
  return useQuery({
    queryKey: ['title-availability', kind, id, country ?? ''],
    enabled: enabled && !!id && !!country,
    staleTime: 30 * 60_000,
    queryFn: async ({ signal }) =>
      unwrap(await api.GET('/api/v1/titles/tmdb/{kind}/{id}/availability', { params: { path: { kind, id }, query: { country } }, signal })),
  });
}

/** Every day a title was in a Top 10 (country, service, rank), for its Top 10 graph. */
export function useTitleRankings(kind: 'movie' | 'tv', id: string, enabled = true) {
  return useQuery({
    queryKey: ['title-rankings', kind, id, 30],
    enabled: enabled && !!id,
    staleTime: 30 * 60_000,
    // The Top 10 graph covers the last 30 days at most.
    queryFn: async ({ signal }) =>
      unwrap(await api.GET('/api/v1/titles/tmdb/{kind}/{id}/rankings', { params: { path: { kind, id }, query: { from: localDayDaysAgo(29) } }, signal })),
  });
}

/** YYYY-MM-DD n days before today, in the device's zone. */
function localDayDaysAgo(n: number): string {
  const d = new Date();
  d.setDate(d.getDate() - n);
  const p = (x: number) => String(x).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}
