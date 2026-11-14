import type { components } from '@/api/schema';

// Grouping for the TV and Movies tabs ("To watch" and "Upcoming"). Pure, so
// the day and month buckets are tested without a device.

type Show = components['schemas']['ShowQueueItem'];
type Episode = components['schemas']['UpcomingEpisode'];
type Tracked = NonNullable<components['schemas']['TrackedTitleState']>;

export type Group<T> = { key: string; title: string; data: T[]; collapsed?: boolean };

/** Today as YYYY-MM-DD in the device's zone. */
export function localDay(d = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** Whole days from today to a YYYY-MM-DD date (0 = today, negative = past). */
export function daysUntil(date: string, today = localDay()): number {
  const ms = Date.parse(`${date.slice(0, 10)}T00:00:00Z`) - Date.parse(`${today}T00:00:00Z`);
  return Math.round(ms / 86_400_000);
}

/** "Today", "Tomorrow", "3 days"… for a countdown badge. */
export function countdownText(days: number): string {
  if (days <= 0) return 'Today';
  if (days === 1) return 'Tomorrow';
  return `${days} days`;
}

/** The show stages, in the order To watch lists them. */
const STAGES: { stage: NonNullable<Show['stage']>; title: string; collapsed?: boolean }[] = [
  { stage: 'watching', title: 'Continue watching' },
  { stage: 'not_started', title: 'Not started' },
  { stage: 'stalled', title: "Haven't watched in a while", collapsed: true },
  { stage: 'on_hold', title: 'On hold', collapsed: true },
];

/** Shows with an aired episode to watch, by stage. */
export function tvToWatch(shows: Show[]): Group<Show>[] {
  return STAGES.map((s) => ({
    key: s.stage,
    title: s.title,
    collapsed: s.collapsed,
    data: shows.filter((x) => x.stage === s.stage && x.next_episode),
  })).filter((g) => g.data.length);
}

/** A heading for a day: Today, Tomorrow, a weekday this week, else the month. */
export function dayGroup(date: string, today = localDay()): { key: string; title: string } {
  const days = daysUntil(date, today);
  if (days <= 0) return { key: 'today', title: 'Today' };
  if (days === 1) return { key: 'tomorrow', title: 'Tomorrow' };
  if (days < 7) {
    return { key: date, title: new Date(`${date}T12:00:00`).toLocaleDateString(undefined, { weekday: 'long' }) };
  }
  return monthGroup(date);
}

export function monthGroup(date: string): { key: string; title: string } {
  return { key: date.slice(0, 7), title: new Date(`${date.slice(0, 7)}-15T12:00:00`).toLocaleDateString(undefined, { month: 'long', year: 'numeric' }) };
}

/** Buckets items (already sorted by date) under consecutive headings. */
function bucket<T>(items: T[], dateOf: (t: T) => string, head: (date: string) => { key: string; title: string }): Group<T>[] {
  const out: Group<T>[] = [];
  for (const it of items) {
    const h = head(dateOf(it));
    const last = out[out.length - 1];
    if (last && last.key === h.key) last.data.push(it);
    else out.push({ ...h, data: [it] });
  }
  return out;
}

/** Episodes still to air, by day this week and by month after. */
export function tvUpcoming(episodes: Episode[], today = localDay()): Group<Episode>[] {
  const dated = episodes.filter((e) => e.air_date).sort((a, b) => (a.air_date! < b.air_date! ? -1 : a.air_date! > b.air_date! ? 1 : 0));
  return bucket(
    dated,
    (e) => e.air_date!,
    (d) => dayGroup(d, today),
  );
}

/** The next episode of each show, soonest first (the grid shows one poster per show). */
export function nextPerShow(episodes: Episode[]): Episode[] {
  const seen = new Set<string>();
  const out: Episode[] = [];
  for (const e of [...episodes].filter((x) => x.air_date).sort((a, b) => (a.air_date! < b.air_date! ? -1 : 1))) {
    if (seen.has(e.tmdb_id ?? '')) continue;
    seen.add(e.tmdb_id ?? '');
    out.push(e);
  }
  return out;
}

/**
 * Watchlist films split by release: out already (or undated) is To watch,
 * keeping your watchlist order; still to come is Upcoming, by month.
 */
export function moviesSplit(titles: (Tracked | null)[], today = localDay()): { toWatch: Tracked[]; upcoming: Group<Tracked>[] } {
  const films = titles.filter((t): t is Tracked => !!t && t.tmdb_kind === 'movie' && t.status === 'watchlist');
  const future = (t: Tracked) => !!t.release_date && daysUntil(t.release_date, today) > 0;
  const upcoming = films.filter(future).sort((a, b) => (a.release_date! < b.release_date! ? -1 : 1));
  return { toWatch: films.filter((t) => !future(t)), upcoming: bucket(upcoming, (t) => t.release_date!, monthGroup) };
}

/** A badge for an episode worth calling out. */
export function episodeBadge(e: { episode_number?: number; episode_type?: string | null }): 'Premiere' | 'Finale' | null {
  if (e.episode_type === 'finale') return 'Finale';
  if (e.episode_number === 1) return 'Premiere';
  return null;
}

/** "2h 05m" style total for a summary line. */
export function hoursText(minutes: number): string {
  if (minutes <= 0) return '0h';
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return h ? `${h}h${m ? ` ${String(m).padStart(2, '0')}m` : ''}` : `${m}m`;
}

type Watch = components['schemas']['WatchEntry'];

/** The latest watch of each title, newest first, up to n (a rewatch doesn't repeat a poster). */
export function recentDistinct(watches: Watch[], n: number): Watch[] {
  const seen = new Set<string>();
  const out: Watch[] = [];
  for (const w of watches) {
    const key = `${w.tmdb_kind}:${w.tmdb_id}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(w);
    if (out.length === n) break;
  }
  return out;
}

/** Diary watches (newest first) under month headings. */
export function diaryMonths(watches: Watch[]): Group<Watch>[] {
  return bucket(
    watches.filter((w) => w.watched_on),
    (w) => w.watched_on!,
    monthGroup,
  );
}
