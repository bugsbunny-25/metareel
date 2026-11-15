import { useQuery } from '@tanstack/react-query';
import Storage from 'expo-sqlite/kv-store';
import { useSyncExternalStore } from 'react';

import { unwrap } from '@/api/client';
import { api } from '@/auth/session';

// The chart's country and service, picked per view like the web app's
// ScopePicker and remembered on this phone; until you pick, your server-side
// preferences (else US / Netflix) apply.
const KEY = 'chart.scope';
export type Scope = { country: string; provider: string };

const listeners = new Set<() => void>();
let saved: Partial<Scope> = read();

function read(): Partial<Scope> {
  try {
    return JSON.parse(Storage.getItemSync(KEY) ?? '{}') as Partial<Scope>;
  } catch {
    return {};
  }
}

export function setScope(patch: Partial<Scope>) {
  saved = { ...saved, ...patch };
  try {
    Storage.setItemSync(KEY, JSON.stringify(saved));
  } catch {
    // Unsaved: lasts until the app restarts.
  }
  listeners.forEach((l) => l());
}

export function useScope(defaults: Partial<Scope>): Scope {
  const s = useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => saved,
  );
  return { country: s.country || defaults.country || 'US', provider: s.provider || defaults.provider || 'netflix' };
}

const NAMES: Record<string, string> = {
  netflix: 'Netflix', 'amazon-prime': 'Prime Video', 'apple-tv': 'Apple TV+', disney: 'Disney+', 'hbo-max': 'HBO Max',
  hulu: 'Hulu', 'paramount-plus': 'Paramount+', peacock: 'Peacock',
};
export const providerName = (slug: string) => NAMES[slug] ?? slug.replace(/-/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());

/** Countries that have charts, and each service's countries (for the pickers). */
export function useChartReference() {
  return useQuery({
    queryKey: ['chart-reference'],
    staleTime: 24 * 60 * 60_000,
    queryFn: async ({ signal }) => {
      const [countries, providers] = await Promise.all([
        api.GET('/api/v1/countries', { signal }).then(unwrap),
        api.GET('/api/v1/providers', { signal }).then(unwrap),
      ]);
      const charted = new Set(providers.flatMap((p) => p.countries ?? []));
      return { countries: countries.filter((c) => c.code && charted.has(c.code)), providers };
    },
  });
}

export type LibraryState = { watch_count?: number; favorite?: boolean; status?: string | null };

/** Your titles that count as "yours" in charts (the web's isYours), by "kind:id". */
export function useLibrary() {
  return useQuery({
    queryKey: ['library'],
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/me/titles', { params: { query: { limit: 200 } }, signal })),
    select: (d) => {
      const yours = new Map<string, LibraryState>();
      for (const t of d.titles ?? []) {
        if (t && isYours(t)) yours.set(`${t.tmdb_kind}:${t.tmdb_id}`, t);
      }
      return yours;
    },
  });
}

/**
 * A library entry makes a title yours (orange in charts): anything you watch,
 * watched, want, finished, paused, dropped or favourited; only "Not
 * interested" without watches is not.
 */
export function isYours(s: { watch_count?: number; favorite?: boolean; status?: string | null }): boolean {
  return (s.watch_count ?? 0) > 0 || !!s.favorite || (!!s.status && s.status !== 'not_interested');
}

// Arrow glyphs with the text-presentation selector (U+FE0E) so they take the
// movement colour instead of drawing as emoji tiles (the web's rows.js).
export const UP = '▲';
export const DOWN = '▼';

export type Move = { type: 'up' | 'down' | 'same' | 'debut' | 'reentry'; by: number };

/** previous_rank is null for debuts and re-entries; days_in_top10 > 1 means it was here before. */
export function movement(i: { rank: number; previous_rank: number | null; days_in_top10: number }): Move {
  if (i.previous_rank == null) return { type: i.days_in_top10 <= 1 ? 'debut' : 'reentry', by: 0 };
  const by = i.previous_rank - i.rank;
  return { type: by > 0 ? 'up' : by < 0 ? 'down' : 'same', by: Math.abs(by) };
}

export function changeText(m: Move): string {
  switch (m.type) {
    case 'up':
      return `${UP} ${m.by}`;
    case 'down':
      return `${DOWN} ${m.by}`;
    case 'same':
      return '–';
    case 'debut':
      return 'New';
    default:
      return 'Back';
  }
}

/** The badge a rail poster shows for your title, like the web's Home rail. */
export function libraryMark(s?: LibraryState | null): 'bookmark-fill' | 'check' | 'heart-fill' | 'list' | null {
  if (!s || !isYours(s)) return null;
  if (s.status === 'watchlist') return 'bookmark-fill';
  if ((s.watch_count ?? 0) > 0 || s.status === 'completed') return 'check';
  return s.favorite ? 'heart-fill' : 'list';
}
