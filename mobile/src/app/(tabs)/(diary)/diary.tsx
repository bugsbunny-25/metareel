import { router } from 'expo-router';
import { useMemo, useState } from 'react';
import { ActivityIndicator, RefreshControl, SectionList, StyleSheet, View } from 'react-native';

import type { components } from '@/api/schema';
import { OutboxBanner } from '@/components/OutboxBanner';
import { Placeholder } from '@/components/Placeholder';
import { Text } from '@/components/Text';
import { TitleRow } from '@/components/TitleRow';
import { diaryMonths, recentDistinct } from '@/data/collections';
import { useDiary, useRatingStats, useRecentWatches, useStatsSummary } from '@/data/queries';
import { ListDetail, useTwoPane } from '@/layout/ListDetail';
import { RootActions, RootTitle, searchAction, useRootScroll } from '@/layout/root-header';
import { useOutbox } from '@/offline';
import { MonthBand, RatingsChart, RecentRow, TimelineRow, TotalRow } from '@/screens/diary-parts';
import { WatchView } from '@/screens/WatchView';
import { useTheme } from '@/theme';

type Entry = components['schemas']['WatchEntry'];

/**
 * Your diary: the latest films and shows as posters, how you rate, your
 * totals, then every watch by month with its day, newest first. Logs still
 * waiting to sync sit on top.
 */
export default function Diary() {
  const { colors, gutter } = useTheme();
  const scroll = useRootScroll();
  const diary = useDiary();
  const films = useRecentWatches('movie', 20);
  const shows = useRecentWatches('tv', 40);
  const ratings = useRatingStats();
  const thisYear = `${new Date().getFullYear()}-01-01`;
  const all = useStatsSummary();
  const year = useStatsSummary(thisYear);
  const pending = useOutbox().filter((i) => i.kind === 'watch' && i.status === 'pending');
  const twoPane = useTwoPane();
  const [selected, setSelected] = useState<number | null>(null);
  const [width, setWidth] = useState(0);

  const sections = useMemo(() => diaryMonths((diary.data?.pages ?? []).flatMap((p) => p.watches ?? [])), [diary.data]);
  const open = (w: Entry) => (twoPane ? setSelected(w.id!) : router.push({ pathname: '/watch/[id]', params: { id: String(w.id) } }));
  const refresh = () => void Promise.all([diary.refetch(), films.refetch(), shows.refetch(), ratings.refetch(), all.refetch(), year.refetch()]);
  const pad = gutter.compact;

  const list = (
    <SectionList
      style={{ backgroundColor: colors.bgCanvas }}
      contentInsetAdjustmentBehavior={scroll.contentInsetAdjustmentBehavior}
      contentContainerStyle={{ paddingTop: scroll.paddingTop, paddingHorizontal: pad, paddingBottom: 32, width: '100%', maxWidth: 760, alignSelf: 'center' }}
      sections={sections}
      keyExtractor={(w) => String(w.id)}
      stickySectionHeadersEnabled={false}
      refreshControl={<RefreshControl refreshing={diary.isRefetching && !diary.isFetchingNextPage} onRefresh={refresh} tintColor={colors.textMuted} />}
      onEndReached={() => diary.hasNextPage && !diary.isFetchingNextPage && void diary.fetchNextPage()}
      ListHeaderComponent={
        <View style={{ gap: 18, marginBottom: 12 }} onLayout={(e) => setWidth(Math.round(e.nativeEvent.layout.width))}>
          <View style={styles.head}>
            <RootTitle>Diary</RootTitle>
            <RootActions actions={[searchAction]} />
          </View>
          <OutboxBanner kinds={['watch']} />
          {pending.map((p) => (
            <TitleRow key={`p${p.id}`} poster={p.meta?.poster as string} title={p.label} subtitle="Waiting to sync" />
          ))}
          <RecentRow title="Recent films" watches={recentDistinct(films.data?.watches ?? [], 4)} width={width} onOpen={open} />
          <RecentRow title="Recent shows" watches={recentDistinct(shows.data?.watches ?? [], 4)} width={width} onOpen={open} />
          {ratings.data?.rated ? <RatingsChart buckets={ratings.data.distribution ?? []} rated={ratings.data.rated} average={ratings.data.average} /> : null}
          {all.data?.totals ? (
            <View>
              <TotalRow label="Films" total={all.data.totals.movie_watches} thisYear={year.data?.totals?.movie_watches} />
              <TotalRow label="Episodes" total={all.data.totals.episode_watches} thisYear={year.data?.totals?.episode_watches} />
              <TotalRow
                label="Hours"
                total={Math.round(all.data.totals.hours ?? 0)}
                thisYear={year.data?.totals ? Math.round(year.data.totals.hours ?? 0) : undefined}
              />
            </View>
          ) : null}
        </View>
      }
      renderSectionHeader={({ section }) => <MonthBand title={section.title} inset={pad} />}
      renderItem={({ item: w, index }) => <TimelineRow w={w} first={index === 0} selected={twoPane && selected === w.id} onPress={() => open(w)} />}
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

const styles = StyleSheet.create({ head: { flexDirection: 'row', alignItems: 'center', gap: 12 } });
