import { useQuery } from '@tanstack/react-query';
import { ActivityIndicator, FlatList, type FlatListProps } from 'react-native';

import { unwrap } from '@/api/client';
import type { components } from '@/api/schema';
import { api } from '@/auth/session';
import { Text } from '@/components/Text';
import { TitleRow } from '@/components/TitleRow';
import { useTitleSearch } from '@/data/queries';
import { year } from '@/lib/format';
import { lookupQuery, parseShared } from '@/lib/share';
import { useTheme } from '@/theme';

type Result = components['schemas']['TitleSearchResult'];

/**
 * Results for a search: a pasted link or ID is looked up, anything else is
 * a title search. Shared by the Search sheet and iOS's search tab.
 */
export function SearchResults({
  q,
  onPick,
  contentInsetAdjustmentBehavior,
  insetLeft = 0,
  insetRight = 0,
  empty,
}: {
  q: string;
  onPick: (kind: 'movie' | 'tv', id: string, title?: string, poster?: string) => void;
  contentInsetAdjustmentBehavior?: FlatListProps<Result>['contentInsetAdjustmentBehavior'];
  insetLeft?: number;
  insetRight?: number;
  empty?: string;
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
        ) : !q.trim() && empty ? (
          <Text variant="muted" style={{ marginTop: 12 }}>
            {empty}
          </Text>
        ) : undefined
      }
      renderItem={({ item: r }) => (
        <TitleRow
          poster={r.poster_path}
          title={r.title ?? ''}
          subtitle={[r.tmdb_kind === 'tv' ? 'Series' : 'Film', year(r.release_date)].filter(Boolean).join(' · ')}
          onPress={() => onPick(r.tmdb_kind ?? 'movie', r.tmdb_id ?? '', r.title, r.poster_path)}
        />
      )}
    />
  );
}
