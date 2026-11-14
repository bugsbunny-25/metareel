import { useDetailScroll } from '@/layout/root-header';
import { useMutation } from '@tanstack/react-query';

import { useState } from 'react';
import { ActivityIndicator, Alert, Pressable, ScrollView, StyleSheet, View } from 'react-native';

import { unwrap } from '@/api/client';
import { api } from '@/auth/session';
import { Button } from '@/components/Button';
import { Chip } from '@/components/Chip';
import { Icon } from '@/components/Icon';
import { Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { useMyTitle, usePreferences, useShow, useTitleOverview } from '@/data/queries';
import { markEpisode, openEpisode, openLog, setWatchlist } from '@/lib/actions';
import { episodeCode, year } from '@/lib/format';
import { useOutbox } from '@/offline';
import { queryClient } from '@/query/client';
import { useTheme } from '@/theme';

import { About, Charting, Hero, Scores, WhereToWatch } from './title-parts';

/**
 * A show: the same hero, scores, where to watch, charts and facts as a
 * film's page, plus your progress, season tabs and episodes to tick.
 */
export function ShowView({
  id,
  embedded,
  onOpenEpisode,
}: {
  id: string;
  embedded?: boolean;
  /** Beside a list: show an episode in the same pane instead of pushing it. */
  onOpenEpisode?: (season: number, episode: number) => void;
}) {
  const { colors, radius, gutter } = useTheme();
  const detail = useDetailScroll(embedded);
  const show = useShow(id);
  const queued = useOutbox().filter((i) => i.kind === 'episodes' && i.meta?.show === id && i.status === 'pending');
  const seasons = (show.data?.seasons ?? []).filter((s) => (s.season_number ?? 0) > 0 || (show.data?.seasons ?? []).length === 1);
  const [picked, setPicked] = useState<number | null>(null);
  const current = picked ?? firstUnfinished(seasons) ?? seasons[0]?.season_number ?? 1;
  const season = seasons.find((s) => s.season_number === current);
  const title = show.data?.title?.title ?? '';
  const today = new Date().toISOString().slice(0, 10);
  const prefs = usePreferences();
  const country = prefs.data?.country;
  const overview = useTitleOverview('tv', id, country);
  const mine = useMyTitle('tv', id);
  const m = overview.data?.metadata;
  const poster = m?.poster_path ?? show.data?.title?.poster_path;
  const first = year(m?.release_date ?? show.data?.title?.release_date);
  const years = first ? (m?.status === 'Ended' || m?.status === 'Canceled' ? `${first}–${year(m.last_air_date) || ''}` : `${first}–`) : '';
  const heroMeta = [
    years,
    m?.number_of_seasons ? `${m.number_of_seasons} ${m.number_of_seasons === 1 ? 'season' : 'seasons'}` : '',
    m?.networks?.[0] ?? '',
    (m?.genres ?? []).slice(0, 2).join(', '),
  ]
    .filter(Boolean)
    .join(' · ');
  // The first aired episode not seen yet, for "Log S01 E02".
  const next = seasons.flatMap((s) => s.episodes ?? []).find((e) => (e.season_number ?? 0) > 0 && !(e.watch_count ?? 0) && !!e.air_date && e.air_date <= today);

  const unmark = useMutation({
    mutationFn: async (e: { season: number; episode: number }) =>
      unwrap(
        await api.DELETE('/api/v1/me/shows/{id}/watched', {
          params: { path: { id }, query: { scope: 'episode', season: e.season, episode: e.episode } },
        }),
      ),
    onSuccess: () => void queryClient.invalidateQueries(),
    onError: () => Alert.alert("Couldn't unmark", 'Unmarking needs a connection. Try again when you are online.'),
  });

  const isQueued = (s: number, e: number) =>
    queued.some((q) => q.meta?.season === s && (q.meta?.episode === e || (q.meta?.scope === 'up_to' && Number(q.meta?.episode) >= e)));

  return (
    <ScrollView
      style={{ backgroundColor: colors.bgCanvas }}
      contentInsetAdjustmentBehavior={detail.contentInsetAdjustmentBehavior}
      contentContainerStyle={{ padding: gutter.compact, paddingTop: detail.paddingTop ?? gutter.compact, gap: 16, paddingBottom: 40 }}
    >
      {show.isPending ? <ActivityIndicator color={colors.textMuted} /> : null}
      {show.data ? (
        <>
          <Hero poster={poster} title={title} tint={mine.data?.title?.poster_color} meta={heroMeta}>
            <Text variant="muted" num>
              {show.data.progress?.watched_episodes ?? 0} of {show.data.progress?.aired_episodes ?? 0} aired episodes
            </Text>
            <Progress percent={show.data.progress?.percent ?? 0} />
            <View style={{ flexDirection: 'row', flexWrap: 'wrap', gap: 8, marginTop: 4 }}>
              {next ? (
                <Button
                  title={`Log ${episodeCode(next.season_number, next.episode_number)}`}
                  onPress={() => openLog({ kind: 'tv', id, title, poster, season: next.season_number, episode: next.episode_number })}
                />
              ) : null}
              <Button
                kind="quiet"
                title={show.data.state?.status === 'watchlist' ? 'On watchlist' : 'Watchlist'}
                onPress={() => setWatchlist({ kind: 'tv', id, title }, show.data?.state?.status !== 'watchlist')}
              />
            </View>
          </Hero>
          <Scores overview={overview.data} />
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
            {seasons.map((s) => (
              <Chip
                key={s.season_number}
                label={`Season ${s.season_number}`}
                selected={s.season_number === current}
                onPress={() => setPicked(s.season_number!)}
              />
            ))}
          </ScrollView>
          {season ? (
            <Panel>
              {(season.episodes ?? []).map((e) => {
                const watched = (e.watch_count ?? 0) > 0;
                const pending = isQueued(e.season_number!, e.episode_number!);
                const aired = !!e.air_date && e.air_date <= today;
                return (
                  <View key={e.episode_number} style={styles.episode}>
                    <Pressable
                      style={{ flex: 1, gap: 2 }}
                      onPress={() =>
                        onOpenEpisode ? onOpenEpisode(e.season_number!, e.episode_number!) : openEpisode(id, e.season_number!, e.episode_number!)
                      }
                      onLongPress={() =>
                        Alert.alert(`Mark up to ${episodeCode(e.season_number, e.episode_number)}?`, 'Every aired episode before it is marked watched too.', [
                          { text: 'Cancel', style: 'cancel' },
                          { text: 'Mark', onPress: () => markEpisode(id, title, e.season_number!, e.episode_number!, 'up_to') },
                        ])
                      }
                      accessibilityHint="Opens the episode; long-press marks up to here"
                    >
                      <Text variant="label" numberOfLines={1}>
                        <Text variant="label" num style={{ color: colors.textMuted }}>
                          {e.episode_number}.{' '}
                        </Text>
                        {e.name}
                      </Text>
                      <Text variant="caption">{aired ? (e.air_date ?? '') : e.air_date ? `Airs ${e.air_date}` : 'Not aired'}</Text>
                    </Pressable>
                    <Pressable
                      disabled={!aired || pending}
                      onPress={() =>
                        watched
                          ? Alert.alert(`Unmark ${episodeCode(e.season_number, e.episode_number)}?`, 'Its watch is deleted for good.', [
                              { text: 'Cancel', style: 'cancel' },
                              { text: 'Unmark', style: 'destructive', onPress: () => unmark.mutate({ season: e.season_number!, episode: e.episode_number! }) },
                            ])
                          : markEpisode(id, title, e.season_number!, e.episode_number!)
                      }
                      accessibilityRole="checkbox"
                      accessibilityState={{ checked: watched || pending, disabled: !aired }}
                      accessibilityLabel={`${episodeCode(e.season_number, e.episode_number)} watched`}
                      hitSlop={8}
                      style={[styles.tick, { opacity: aired ? 1 : 0.3 }]}
                    >
                      <Icon name="check-circle" size={28} color={watched ? colors.deltaUp : pending ? colors.accentYours : colors.borderStrong} />
                    </Pressable>
                  </View>
                );
              })}
            </Panel>
          ) : null}
          <WhereToWatch overview={overview.data} country={country} />
          <Charting overview={overview.data} />
          <About metadata={overview.data?.metadata} kind="tv" />
        </>
      ) : null}
    </ScrollView>
  );
}

function firstUnfinished(seasons: { season_number?: number; watched_episodes?: number; total_episodes?: number }[]) {
  return seasons.find((s) => (s.watched_episodes ?? 0) < (s.total_episodes ?? 0))?.season_number;
}

function Progress({ percent }: { percent: number }) {
  const { colors } = useTheme();
  return (
    <View style={[styles.bar, { backgroundColor: colors.bgElevated }]} accessible accessibilityLabel={`${Math.round(percent)}% watched`}>
      <View style={{ width: `${Math.min(100, percent)}%`, height: '100%', backgroundColor: colors.accentYours, borderRadius: 3 }} />
    </View>
  );
}

const styles = StyleSheet.create({
  episode: { flexDirection: 'row', alignItems: 'center', gap: 12, paddingVertical: 6 },
  tick: { width: 44, height: 44, alignItems: 'center', justifyContent: 'center' },
  bar: { height: 6, borderRadius: 3, overflow: 'hidden' },
});
