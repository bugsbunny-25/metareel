import type { paths } from '@/api/schema';
import { formatMoney } from '@/lib/format';

// What the widgets show, worked out in the app from GET /me/glance (widget
// code runs in an isolated runtime: no fetching, no app state, plain props).

export type UpNextProps = {
  state: 'ok' | 'empty' | 'signedOut';
  /** Up to three rows: a show and its next episode. */
  items: { title: string; line: string }[];
  /** The next episode airing soon, e.g. "Grey's Anatomy · Thu 19 Nov". */
  airing: string;
  url: string;
};

export type BreakEvenProps = {
  state: 'ok' | 'empty' | 'signedOut';
  plan: string;
  /** e.g. "3 of 4 films this month". */
  progressLabel: string;
  /** 0..1 of the break-even count this month. */
  progress: number;
  /** e.g. "Saved $42.10 so far" or "Paid $12.00 more than tickets". */
  balance: string;
  url: string;
};

export type Glance = paths['/api/v1/me/glance']['get']['responses'][200]['content']['application/json'];

export const SIGNED_OUT_UP_NEXT: UpNextProps = { state: 'signedOut', items: [], airing: '', url: 'metareel:///' };
export const SIGNED_OUT_BREAK_EVEN: BreakEvenProps = {
  state: 'signedOut',
  plan: '',
  progressLabel: '',
  progress: 0,
  balance: '',
  url: 'metareel:///',
};

const code = (s?: number, e?: number) => `S${s ?? 0}E${e ?? 0}`;

function day(date: string | null | undefined): string {
  const m = date && /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
  if (!m) return '';
  return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3])).toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short' });
}


export function upNextProps(g: Glance): UpNextProps {
  const items = (g.up_next ?? []).slice(0, 3).map((e) => ({
    title: e.show_title ?? '',
    line: `${code(e.season_number, e.episode_number)}${e.name ? ` · ${e.name}` : ''}`,
  }));
  const a = g.next_airing;
  const airing = a ? `${a.show_title ?? ''} ${code(a.season_number, a.episode_number)} · ${day(a.air_date)}` : '';
  const first = g.up_next?.[0]?.tmdb_id ?? a?.tmdb_id;
  return {
    state: items.length || airing ? 'ok' : 'empty',
    items,
    airing,
    url: first ? `metareel:///show/${first}` : 'metareel:///tv',
  };
}

export function breakEvenProps(g: Glance): BreakEvenProps {
  const s = g.subscription;
  if (!s) return { state: 'empty', plan: '', progressLabel: '', progress: 0, balance: '', url: 'metareel:///subscriptions' };
  const watches = s.watches_this_month ?? 0;
  const target = s.break_even_watches ?? 0;
  const saved = s.saved_minor ?? 0;
  const currency = s.currency ?? 'USD';
  return {
    state: 'ok',
    plan: s.plan ?? '',
    progressLabel: target ? `${watches} of ${target} films this month` : `${watches} film${watches === 1 ? '' : 's'} this month`,
    progress: target ? Math.min(1, watches / target) : 0,
    balance: saved >= 0 ? `Saved ${formatMoney(saved, currency)} so far` : `${formatMoney(-saved, currency)} more than tickets so far`,
    url: 'metareel:///subscriptions',
  };
}
