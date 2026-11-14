import type { components } from '@/api/schema';

type UpcomingEpisode = components['schemas']['UpcomingEpisode'];

/** One reminder: a show's episodes airing on one day. */
export type Reminder = {
  /** Stable per show and day, so a re-plan keeps what is already scheduled. */
  id: string;
  at: Date;
  title: string;
  body: string;
  url: string;
};

/** Episode reminders go out at this local hour on the air date (TMDB has dates, not times). */
export const REMINDER_HOUR = 18;
/** How far ahead reminders are scheduled; refreshed on every sync. */
export const REMINDER_DAYS = 14;
/** iOS keeps at most 64 pending local notifications per app; leave room. */
export const MAX_REMINDERS = 48;

export const REMINDER_PREFIX = 'episode:';

/**
 * Plans the reminders for the episodes that air in the next two weeks: one
 * per show and day (a season dropped at once is one reminder), at
 * REMINDER_HOUR local time, soonest first. Days already past that hour are
 * left out.
 */
export function planReminders(episodes: UpcomingEpisode[], now: Date): Reminder[] {
  const until = new Date(now);
  until.setDate(until.getDate() + REMINDER_DAYS);
  const byShowDay = new Map<string, UpcomingEpisode[]>();
  for (const e of episodes) {
    if (!e.air_date || !e.tmdb_id) continue;
    const key = `${e.tmdb_id}:${e.air_date}`;
    const list = byShowDay.get(key);
    if (list) list.push(e);
    else byShowDay.set(key, [e]);
  }
  const out: Reminder[] = [];
  for (const [key, eps] of byShowDay) {
    const at = localAt(eps[0].air_date!, REMINDER_HOUR);
    if (!at || at <= now || at > until) continue;
    eps.sort((a, b) => (a.season_number ?? 0) - (b.season_number ?? 0) || (a.episode_number ?? 0) - (b.episode_number ?? 0));
    const show = eps[0].show_title || 'A show you follow';
    out.push({
      id: REMINDER_PREFIX + key,
      at,
      title: show,
      body: describe(eps),
      url: `/show/${eps[0].tmdb_id}`,
    });
  }
  out.sort((a, b) => a.at.getTime() - b.at.getTime() || a.id.localeCompare(b.id));
  return out.slice(0, MAX_REMINDERS);
}

function describe(eps: UpcomingEpisode[]): string {
  const first = eps[0];
  const code = (e: UpcomingEpisode) => `S${e.season_number ?? 0}E${e.episode_number ?? 0}`;
  if (eps.length === 1) {
    const what = first.episode_type === 'finale' ? 'Finale' : first.episode_number === 1 ? 'Premiere' : 'New episode';
    return `${what} today: ${code(first)}${first.name ? ` · ${first.name}` : ''}`;
  }
  const last = eps[eps.length - 1];
  return `${eps.length} new episodes today: ${code(first)}–${code(last)}`;
}

/** YYYY-MM-DD at the given local hour, or null for a bad date. */
function localAt(date: string, hour: number): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
  if (!m) return null;
  const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]), hour, 0, 0, 0);
  return Number.isNaN(d.getTime()) ? null : d;
}
