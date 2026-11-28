import { useQuery } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { ActivityIndicator, FlatList, type FlatListProps, Pressable, StyleSheet } from 'react-native';

import { unwrap } from '@/api/client';
import type { components } from '@/api/schema';
import { api } from '@/auth/session';
import { Icon } from '@/components/Icon';
import { Text } from '@/components/Text';
import { TitleRow } from '@/components/TitleRow';
import { useTitleSearch, useWatchlist } from '@/data/queries';
import { DiscoverRails } from '@/screens/DiscoverRails';
import { setWatchlist } from '@/lib/actions';
import { year } from '@/lib/format';
import { lookupQuery, parseShared } from '@/lib/share';
import { useOutbox, watchlistShown } from '@/offline';
import { useTheme } from '@/theme';

type Result = components['schemas']['TitleSearchResult'];

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
      ? [{ tmdb_kind: ref.kind, tmdb_id: ref.id, title: `TMDB ${ref.kind} ${ref.id}` }]
      : ref
        ? (lookup.data?.items ?? []).map((i) => ({ tmdb_kind: i.kind, tmdb_id: i.tmdb_id, title: `Found: ${i.kind} ${i.tmdb_id}` }))
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
      renderItem={({ item: r }) => (
        <TitleRow
          poster={r.poster_path}
          title={r.title ?? ''}
          kind={r.tmdb_kind}
          tmdbId={r.tmdb_id}
          subtitle={[r.tmdb_kind === 'tv' ? 'Series' : 'Film', year(r.release_date)].filter(Boolean).join(' · ')}
          onPress={() => onPick(r.tmdb_kind ?? 'movie', r.tmdb_id ?? '', r.title, r.poster_path)}
          right={
            r.tmdb_id ? (
              <WatchlistButton
                kind={r.tmdb_kind ?? 'movie'}
                id={r.tmdb_id}
                title={r.title ?? ''}
                on={watchlistShown(outboxItems, r.tmdb_kind ?? 'movie', r.tmdb_id, listed.has(`${r.tmdb_kind ?? 'movie'}:${r.tmdb_id}`))}
              />
            ) : null
          }
        />
      )}
    />
  );
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
