import { type Href, router } from 'expo-router';

import { deviceTimeZone, episodeCode } from '@/lib/format';
import { enqueue } from '@/offline';

type Target = { kind: 'movie' | 'tv'; id: string; title?: string; poster?: string | null };

/** Opens the log sheet for a movie or one episode. */
export function openLog(t: Target & { season?: number; episode?: number }) {
  router.push({
    pathname: '/log',
    params: {
      kind: t.kind,
      id: t.id,
      title: t.title ?? '',
      poster: t.poster ?? '',
      ...(t.season != null ? { season: String(t.season), episode: String(t.episode) } : {}),
    },
  });
}

/** A movie's or show's page, as a link target. */
export function titleHref(t: Target): Href {
  return t.kind === 'tv' ? { pathname: '/show/[id]', params: { id: t.id } } : { pathname: '/title/[kind]/[id]', params: { kind: t.kind, id: t.id } };
}

export function openTitle(t: Target) {
  router.push(titleHref(t));
}

/** Opens a person (cast or crew) in the current tab. */
export function openPerson(id: number) {
  router.push({ pathname: '/person/[id]', params: { id: String(id) } });
}

/** Opens one episode's details. */
export function openEpisode(show: string, season: number, episode: number) {
  router.push({ pathname: '/episode/[show]/[season]/[episode]', params: { show, season: String(season), episode: String(episode) } });
}

/** Marks one episode watched now (queued; marking skips episodes already watched). */
export function markEpisode(showId: string, title: string, season: number, episode: number, scope: 'episode' | 'up_to' = 'episode') {
  enqueue({
    kind: 'episodes',
    method: 'POST',
    path: `/api/v1/me/shows/${showId}/watched`,
    body: {
      scope,
      season_number: season,
      episode_number: episode,
      include_specials: false,
      ended_at: new Date().toISOString(),
      tz: deviceTimeZone(),
      time_precision: 'minute',
    },
    label: `${title} ${scope === 'up_to' ? 'up to ' : ''}${episodeCode(season, episode)}`,
    meta: { show: showId, season, episode, scope },
  });
}

/** Adds a title to the watchlist or takes it off (queued; both are idempotent). */
export function setWatchlist(t: Target, on: boolean) {
  enqueue({
    kind: 'watchlist',
    method: on ? 'PUT' : 'DELETE',
    path: `/api/v1/me/watchlist/${t.kind}/${t.id}`,
    body: on ? {} : undefined,
    label: `${on ? 'Add' : 'Remove'} ${t.title ?? 'title'} ${on ? 'to' : 'from'} the watchlist`,
    meta: { kind: t.kind, id: t.id, on },
  });
}
