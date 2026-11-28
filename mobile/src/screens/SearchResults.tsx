import { useQuery } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { ActivityIndicator, FlatList, type FlatListProps, Pressable, StyleSheet } from 'react-native';

import { unwrap } from '@/api/client';
import type { components } from '@/api/schema';
import { api } from '@/auth/session';
import { Icon } from '@/components/Icon';
import { Text } from '@/components/Text';
import { TitleRow } from '@/components/TitleRow';
import { useMyTitle, useTitleSearch, useWatchlist } from '@/data/queries';
import { DiscoverRails } from '@/screens/DiscoverRails';
import { setWatchlist } from '@/lib/actions';
import { year } from '@/lib/format';
import { lookupQuery, parseShared } from '@/lib/share';
import { useOutbox, watchlistShown } from '@/offline';
import { useTheme } from '@/theme';

type Result = components['schemas']['TitleSearchResult'] & {
  /** From a pasted link or ID: only the kind and ID are known, the row looks the rest up. */
  linked?: boolean;
};

/**
 * Results for a search: a pasted link or ID is looked up, anything else is
 * a title search. Before anything is typed, the Discover rails. Each result
 * has a bookmark to put it on the watchlist (or take it off) without opening
 * it, filled when it's already there. Shared by the Search sheet and iOS's
 * search tab.
 */
export function SearchResults({
  q,
  onPick,
  contentInsetAdjustmentBehavior,
  insetLeft = 0,
  insetRight = 0,
}: {
  q: string;
  onPick: (kind: 'movie' | 'tv', id: string, title?: string, poster?: string) => void;
  contentInsetAdjustmentBehavior?: FlatListProps<Result>['contentInsetAdjustmentBehavior'];
  insetLeft?: number;
  insetRight?: number;
}) {
  const { colors, gutter } = useTheme();
  const parsed = parseShared(q);
  const ref = parsed && parsed.type !== 'search' ? parsed : null;
  const search = useTitleSearch(ref ? '' : q);
  const lookup = useQuery({
    queryKey: ['lookup', q],
    enabled: !!ref && ref.type !== 'tmdb',
    queryFn: async ({ signal }) => unwrap(await api.GET('/api/v1/titles/lookup', { params: { query: lookupQuery(ref!) }, signal })),
  });
  const results: Result[] =
    ref?.type === 'tmdb'
      ? [{ tmdb_kind: ref.kind, tmdb_id: ref.id, title: `TMDB ${ref.kind} ${ref.id}`, linked: true }]
      : ref
        ? (lookup.data?.items ?? []).map((i) => ({ tmdb_kind: i.kind, tmdb_id: i.tmdb_id, title: `Found: ${i.kind} ${i.tmdb_id}`, linked: true }))
        : (search.data?.results ?? []);
  const busy = search.isFetching || lookup.isFetching;
  // What's on the watchlist now: the server's list, then any change still queued.
  const watchlist = useWatchlist();
  const outboxItems = useOutbox();
  const listed = new Set((watchlist.data?.titles ?? []).filter((t) => t?.status === 'watchlist').map((t) => `${t!.tmdb_kind}:${t!.tmdb_id}`));

  return (
    <FlatList
      data={results}
      keyExtractor={(r) => `${r.tmdb_kind}-${r.tmdb_id}`}
      keyboardShouldPersistTaps="handled"
      keyboardDismissMode="on-drag"
      contentInsetAdjustmentBehavior={contentInsetAdjustmentBehavior}
      contentContainerStyle={{ paddingLeft: gutter.compact + insetLeft, paddingRight: gutter.compact + insetRight, paddingBottom: 32 }}
      ListHeaderComponent={
        busy ? (
          <ActivityIndicator color={colors.textMuted} style={{ marginVertical: 8 }} />
        ) : search.isError ? (
          <Text variant="muted">Search needs a connection.</Text>
        ) : !q.trim() ? (
          <DiscoverRails onPick={onPick} />
        ) : undefined
      }
      renderItem={({ item: r }) => {
        const kind = r.tmdb_kind ?? 'movie';
        const id = r.tmdb_id ?? '';
        const row = (shown: Result) => (
          <TitleRow
            poster={shown.poster_path}
            title={shown.title ?? ''}
            kind={kind}
            tmdbId={id}
            subtitle={[kind === 'tv' ? 'Series' : 'Film', year(shown.release_date)].filter(Boolean).join(' · ')}
            onPress={() => onPick(kind, id, shown.title, shown.poster_path)}
            right={
              id ? <WatchlistButton kind={kind} id={id} title={shown.title ?? ''} on={watchlistShown(outboxItems, kind, id, listed.has(`${kind}:${id}`))} /> : null
            }
          />
        );
        return r.linked ? <LinkedRow result={r} render={row} /> : row(r);
      }}
    />
  );
}

/**
 * A result from a pasted link: shows the title's own record (metareel's, or
 * TMDB's on demand) once it loads, the placeholder until then or if it fails.
 */
function LinkedRow({ result, render }: { result: Result; render: (shown: Result) => React.ReactElement }) {
  const mine = useMyTitle(result.tmdb_kind ?? 'movie', result.tmdb_id ?? '');
  const t = mine.data?.title;
  return render(t?.title ? { ...result, title: t.title, poster_path: t.poster_path, release_date: t.release_date } : result);
}

/** Puts a result on the watchlist or takes it off; filled when it's on. */
function WatchlistButton({ kind, id, title, on }: { kind: 'movie' | 'tv'; id: string; title: string; on: boolean }) {
  const { colors } = useTheme();
  return (
    <Pressable
      onPress={() => {
        void Haptics.selectionAsync();
        setWatchlist({ kind, id, title }, !on);
      }}
      hitSlop={8}
      accessibilityRole="button"
      accessibilityLabel={on ? `Remove ${title} from your watchlist` : `Add ${title} to your watchlist`}
      accessibilityState={{ selected: on }}
      style={({ pressed }) => [styles.quick, { backgroundColor: on ? colors.accentYours : colors.bgElevated, opacity: pressed ? 0.7 : 1 }]}
    >
      <Icon name={on ? 'bookmark-fill' : 'bookmark'} size={18} color={on ? colors.accentOn : colors.textPrimary} />
    </Pressable>
  );
}

const styles = StyleSheet.create({
  quick: { width: 40, height: 40, borderRadius: 20, alignItems: 'center', justifyContent: 'center' },
});
