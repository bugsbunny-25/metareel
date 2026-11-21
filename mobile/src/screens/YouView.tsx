import { router } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, Pressable, RefreshControl, ScrollView, StyleSheet, View } from 'react-native';

import { useSignedIn } from '@/auth/session';
import { PosterTile, ProgressBar } from '@/components/Media';
import { Text } from '@/components/Text';
import { recentDistinct } from '@/data/collections';
import { useAccount, useDetailedStats, useRecentWatches, useSavings, useShowQueue } from '@/data/queries';
import { tabBodyScroll } from '@/layout/TabbedRoot';
import { useWindowClass } from '@/layout/window-class';
import { openTitle } from '@/lib/actions';
import { formatMoney } from '@/lib/format';
import { usePullToRefresh } from '@/lib/refresh';
import { BarList, Card, SectionHead, shortDuration, StatTile } from '@/screens/stats-parts';
import { useTheme } from '@/theme';

/**
 * You's first page: who you are, your statistics at a glance (streak, shows,
 * episodes, films), this year, whether your cinema plans pay off, your
 * genres, and your shows and films as posters. Library has its own tab.
 */
export function YouView({ onOpen }: { onOpen: (page: 'diary') => void }) {
  const { colors, gutter } = useTheme();
  const pad = useWindowClass() === 'compact' ? gutter.compact : 24;
  const session = useSignedIn();
  const account = useAccount();
  const stats = useDetailedStats('');
  const savings = useSavings();
  const queue = useShowQueue();
  const films = useRecentWatches('movie', 60);
  const pull = usePullToRefresh(() =>
    Promise.all([stats.refetch(), savings.refetch(), queue.refetch(), films.refetch(), account.refetch()]),
  );
  const [width, setWidth] = useState(0);
  const s = stats.data;
  const name = account.data?.user?.username ?? session.user.username;
  const since = account.data?.user?.created_at ? new Date(account.data.user.created_at).toLocaleDateString(undefined, { month: 'long', year: 'numeric' }) : '';

  const myShows = [...(queue.data?.shows ?? [])]
    .filter((x) => (x.watched_episodes ?? 0) > 0)
    .sort((a, b) => (b.last_watched_at ?? '').localeCompare(a.last_watched_at ?? ''))
    .slice(0, 12);
  const myFilms = recentDistinct(films.data?.watches ?? [], 12);
  const plans = (savings.data?.subscriptions ?? []).filter((p) => p.active);
  const tileW = Math.max(104, Math.min(150, Math.floor((width - 3 * 12) / 3.4)));

  return (
    <ScrollView
      style={{ backgroundColor: colors.bgCanvas }}
      contentInsetAdjustmentBehavior={tabBodyScroll.contentInsetAdjustmentBehavior}
      contentContainerStyle={[styles.content, { paddingTop: tabBodyScroll.paddingTop, paddingHorizontal: pad }]}
      refreshControl={<RefreshControl refreshing={pull.refreshing} onRefresh={pull.onRefresh} tintColor={colors.textMuted} />}
    >
      <View style={styles.column}>
      <View style={styles.profile} onLayout={(e) => setWidth(e.nativeEvent.layout.width)}>
        <View style={[styles.avatar, { backgroundColor: colors.bgElevated, borderColor: colors.accentYours }]}>
          <Text variant="display" style={{ fontSize: 34 }}>
            {name.slice(0, 1).toUpperCase()}
          </Text>
        </View>
        <View style={{ flex: 1, gap: 2 }}>
          <Text variant="display" numberOfLines={1}>
            {name}
          </Text>
          <Text variant="muted" numberOfLines={1}>
            {since ? `Watching since ${since}` : session.server.replace(/^https?:\/\//, '')}
          </Text>
        </View>
      </View>

      <SectionHead title="Statistics" onMore={() => router.push('/stats')} />
      {stats.isPending ? <ActivityIndicator color={colors.textMuted} /> : null}
      {s ? (
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 10 }} style={{ marginHorizontal: -gutter.compact }}>
          <View style={{ width: gutter.compact - 10 }} />
          <StatTile icon="flame" value={String(s.streak?.current ?? 0)} label="Day streak" onPress={() => router.push('/stats')} />
          <StatTile value={(s.shows ?? 0).toLocaleString()} label="Shows" onPress={() => router.push({ pathname: '/stats', params: { kind: 'tv' } })} />
          <StatTile
            wide
            value={(s.episodes ?? 0).toLocaleString()}
            label="Watched episodes"
            onPress={() => router.push({ pathname: '/stats', params: { kind: 'tv' } })}
          />
          <StatTile value={(s.films ?? 0).toLocaleString()} label="Movies" onPress={() => router.push({ pathname: '/stats', params: { kind: 'movie' } })} />
          <View style={{ width: gutter.compact - 10 }} />
        </ScrollView>
      ) : null}

      {s ? (
        <>
          <SectionHead title="This year" />
          <View style={styles.tiles}>
            <StatTile value={(s.this_year_episodes ?? 0).toLocaleString()} label="Episodes" />
            <StatTile value={(s.this_year_movies ?? 0).toLocaleString()} label="Movies" />
            <StatTile value={shortDuration(s.this_year?.minutes ?? 0)} label="Watch time" />
          </View>
        </>
      ) : null}

      {plans.length ? (
        <>
          <SectionHead title="Cinema plans" onMore={() => router.push('/subscriptions')} />
          {plans.map((p) => {
            const month = p.months?.[p.months.length - 1];
            const need = p.break_even_watches ?? 0;
            const done = month?.watches ?? 0;
            const saved = p.saved_minor ?? 0;
            return (
              <Pressable key={p.id} onPress={() => router.push('/subscriptions')} accessibilityRole="button">
                <Card
                  title={p.plan}
                  right={
                    <Text variant="muted">
                      {formatMoney(p.price_minor, p.currency)} / {p.period}
                    </Text>
                  }
                >
                  {need ? (
                    <View style={{ gap: 6 }}>
                      <Text num>
                        <Text variant="label" num>
                          {done} of {need}
                        </Text>{' '}
                        films to break even this {p.period}
                        {done >= need ? <Text style={{ color: colors.deltaUp }}> · paid off</Text> : null}
                      </Text>
                      <ProgressBar percent={(done / need) * 100} />
                    </View>
                  ) : null}
                  <Text variant="muted" num>
                    {saved >= 0 ? 'Saved ' : 'Down '}
                    <Text variant="label" num style={{ color: saved >= 0 ? colors.deltaUp : colors.deltaDown }}>
                      {formatMoney(Math.abs(saved), p.currency)}
                    </Text>{' '}
                    over {p.watches ?? 0} covered {p.watches === 1 ? 'film' : 'films'}
                  </Text>
                </Card>
              </Pressable>
            );
          })}
        </>
      ) : null}

      {s?.genres?.length ? (
        <>
          <SectionHead title="Your genres" />
          <BarList rows={s.genres.slice(0, 6).map((g) => ({ label: g.label ?? '', value: g.count ?? 0 }))} />
        </>
      ) : null}

      {myShows.length ? (
        <>
          <SectionHead title="My shows" onMore={() => router.push('/tv')} />
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 12 }} style={{ marginHorizontal: -gutter.compact }}>
            <View style={{ width: gutter.compact - 12 }} />
            {myShows.map((x) => (
              <PosterTile
                key={x.tmdb_id}
                width={tileW}
                poster={x.poster_path}
                title={x.title ?? ''}
                kind="tv"
                tmdbId={x.tmdb_id}
                percent={x.percent ?? 0}
                onPress={() => openTitle({ kind: 'tv', id: x.tmdb_id ?? '', title: x.title })}
              />
            ))}
            <View style={{ width: gutter.compact - 12 }} />
          </ScrollView>
        </>
      ) : null}

      {myFilms.length ? (
        <>
          <SectionHead title="My movies" onMore={() => onOpen('diary')} />
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 12 }} style={{ marginHorizontal: -gutter.compact }}>
            <View style={{ width: gutter.compact - 12 }} />
            {myFilms.map((w) => (
              <PosterTile
                key={w.id}
                width={tileW}
                poster={w.poster_path}
                title={w.title ?? ''}
                kind="movie"
                tmdbId={w.tmdb_id}
                caption={w.rating ? '★'.repeat(Math.floor(w.rating / 2)) + (w.rating % 2 ? '½' : '') : null}
                onPress={() => openTitle({ kind: 'movie', id: w.tmdb_id ?? '', title: w.title })}
              />
            ))}
            <View style={{ width: gutter.compact - 12 }} />
          </ScrollView>
        </>
      ) : null}
      </View>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  content: { paddingBottom: 32 },
  column: { gap: 16, width: '100%', maxWidth: 760, alignSelf: 'center' },
  profile: { flexDirection: 'row', alignItems: 'center', gap: 16, paddingVertical: 4 },
  avatar: { width: 72, height: 72, borderRadius: 36, borderWidth: 2, alignItems: 'center', justifyContent: 'center' },
  tiles: { flexDirection: 'row', flexWrap: 'wrap', gap: 10 },
});
