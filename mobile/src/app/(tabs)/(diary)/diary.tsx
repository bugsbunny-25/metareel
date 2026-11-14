import { router } from 'expo-router';
import { useMemo, useState } from 'react';
import { ActivityIndicator, RefreshControl, SectionList, StyleSheet, View } from 'react-native';

import type { components } from '@/api/schema';
import { OutboxBanner } from '@/components/OutboxBanner';
import { Text } from '@/components/Text';
import { TitleRow } from '@/components/TitleRow';
import { Placeholder } from '@/components/Placeholder';
import { useDiary } from '@/data/queries';
import { ListDetail, useTwoPane } from '@/layout/ListDetail';
import { dayHeading, episodeCode, starText } from '@/lib/format';
import { useOutbox } from '@/offline';
import { WatchView } from '@/screens/WatchView';
import { searchAction } from '@/layout/CollectionScreen';
import { RootActions, RootTitle, useRootScroll } from '@/layout/root-header';
import { useTheme } from '@/theme';

type Entry = components['schemas']['WatchEntry'];

/** Your watches, newest day first; logs still waiting to sync sit on top. */
export default function Diary() {
  const { colors, gutter } = useTheme();
  const scroll = useRootScroll();
  const diary = useDiary();
  const pending = useOutbox().filter((i) => i.kind === 'watch' && i.status === 'pending');
  const twoPane = useTwoPane();
  const [selected, setSelected] = useState<number | null>(null);

  const sections = useMemo(() => {
    const byDay = new Map<string, Entry[]>();
    for (const page of diary.data?.pages ?? []) {
      for (const w of page.watches ?? []) {
        const day = w.watched_on ?? '';
        byDay.set(day, [...(byDay.get(day) ?? []), w]);
      }
    }
    return [...byDay.entries()].map(([day, data]) => ({ title: dayHeading(day), data }));
  }, [diary.data]);

  const list = (
    <SectionList
      style={{ backgroundColor: colors.bgCanvas }}
      contentInsetAdjustmentBehavior={scroll.contentInsetAdjustmentBehavior}
      contentContainerStyle={{
        paddingTop: scroll.paddingTop,
        paddingHorizontal: gutter.compact,
        paddingBottom: 32,
        width: '100%',
        maxWidth: 760,
        alignSelf: 'center',
      }}
      sections={sections}
      keyExtractor={(w) => String(w.id)}
      stickySectionHeadersEnabled={false}
      refreshControl={
        <RefreshControl refreshing={diary.isRefetching && !diary.isFetchingNextPage} onRefresh={() => void diary.refetch()} tintColor={colors.textMuted} />
      }
      onEndReached={() => diary.hasNextPage && !diary.isFetchingNextPage && void diary.fetchNextPage()}
      ListHeaderComponent={
        <View style={{ gap: 12, marginBottom: 8 }}>
          <View style={styles.head}>
            <RootTitle>Diary</RootTitle>
            <RootActions actions={[searchAction]} />
          </View>
          <OutboxBanner kinds={['watch']} />
          {pending.map((p) => (
            <TitleRow key={`p${p.id}`} poster={p.meta?.poster as string} title={p.label} subtitle="Waiting to sync" />
          ))}
        </View>
      }
      renderSectionHeader={({ section }) => (
        <Text variant="caption" style={styles.day}>
          {section.title}
        </Text>
      )}
      renderItem={({ item: w }) => (
        <TitleRow
          poster={w.poster_path}
          title={w.title ?? ''}
          subtitle={
            <Text variant="muted" numberOfLines={1}>
              {[
                w.subject_kind === 'episode' ? episodeCode(w.season_number, w.episode_number) : null,
                w.venue_name_at_watch ?? w.provider_name,
                w.is_rewatch ? 'rewatch' : null,
              ]
                .filter(Boolean)
                .join(' · ')}
              {w.rating ? <Text variant="muted" style={{ color: colors.accentYours }}>{`  ${starText(w.rating)}`}</Text> : null}
            </Text>
          }
          selected={twoPane && selected === w.id}
          onPress={() => (twoPane ? setSelected(w.id!) : router.push({ pathname: '/watch/[id]', params: { id: String(w.id) } }))}
        />
      )}
      ListEmptyComponent={
        diary.isPending ? (
          <ActivityIndicator color={colors.textMuted} />
        ) : diary.isError ? (
          <Text variant="muted">Couldn&apos;t load your diary. Pull to try again.</Text>
        ) : (
          <Text variant="muted">Nothing logged yet. Log a film from Tonight.</Text>
        )
      }
      ListFooterComponent={diary.isFetchingNextPage ? <ActivityIndicator color={colors.textMuted} /> : undefined}
    />
  );
  return (
    <ListDetail
      list={list}
      detail={selected ? <WatchView key={selected} id={String(selected)} embedded /> : null}
      placeholder={<Placeholder text="Pick a watch to see it here." />}
    />
  );
}

const styles = StyleSheet.create({ day: { marginTop: 14, marginBottom: 4 }, head: { flexDirection: 'row', alignItems: 'center', gap: 12 } });
