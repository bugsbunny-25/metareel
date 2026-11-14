import { useState } from 'react';
import { ActivityIndicator, FlatList, RefreshControl, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { OutboxBanner } from '@/components/OutboxBanner';
import { Placeholder } from '@/components/Placeholder';
import { Text } from '@/components/Text';
import { TitleRow } from '@/components/TitleRow';
import { useWatchlist } from '@/data/queries';
import { ListDetail, useTwoPane } from '@/layout/ListDetail';
import { openTitle } from '@/lib/actions';
import { year } from '@/lib/format';
import { ShowView } from '@/screens/ShowView';
import { TitleView } from '@/screens/TitleView';
import { useTheme } from '@/theme';

export default function Watchlist() {
  const { colors, gutter } = useTheme();
  const insets = useSafeAreaInsets();
  const list = useWatchlist();
  const titles = list.data?.titles ?? [];
  const twoPane = useTwoPane();
  const [selected, setSelected] = useState<{ kind: 'movie' | 'tv'; id: string } | null>(null);
  const listView = (
    <FlatList
      style={{ backgroundColor: colors.bgCanvas }}
      contentContainerStyle={{ paddingTop: insets.top + 12, paddingHorizontal: gutter.compact, paddingBottom: 32, width: '100%', maxWidth: 760, alignSelf: 'center' }}
      data={titles}
      keyExtractor={(t) => `${t?.tmdb_kind}-${t?.tmdb_id}`}
      refreshControl={<RefreshControl refreshing={list.isRefetching} onRefresh={() => void list.refetch()} tintColor={colors.textMuted} />}
      ListHeaderComponent={
        <View style={{ gap: 12, marginBottom: 8 }}>
          <Text variant="display" accessibilityRole="header">
            Watchlist
          </Text>
          <OutboxBanner kinds={['watchlist']} />
        </View>
      }
      renderItem={({ item: t }) => (
        <TitleRow
          poster={t?.poster_path}
          title={t?.title ?? ''}
          subtitle={[t?.tmdb_kind === 'tv' ? 'Series' : 'Film', year(t?.release_date), t?.watchlist_note].filter(Boolean).join(' · ')}
          selected={twoPane && selected?.id === t?.tmdb_id && selected?.kind === t?.tmdb_kind}
          onPress={() => {
            const ref = { kind: t?.tmdb_kind ?? 'movie', id: t?.tmdb_id ?? '' } as const;
            if (twoPane) setSelected(ref);
            else openTitle({ ...ref, title: t?.title });
          }}
        />
      )}
      ListEmptyComponent={
        list.isPending ? <ActivityIndicator color={colors.textMuted} /> : <Text variant="muted">Your watchlist is empty. Add titles from their page.</Text>
      }
    />
  );
  return (
    <ListDetail
      list={listView}
      detail={
        selected ? (
          selected.kind === 'tv' ? (
            <ShowView key={selected.id} id={selected.id} embedded />
          ) : (
            <TitleView key={selected.id} kind="movie" id={selected.id} embedded />
          )
        ) : null
      }
      placeholder={<Placeholder text="Pick a title to see it here." />}
    />
  );
}
