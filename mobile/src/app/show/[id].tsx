import { useMutation } from '@tanstack/react-query';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, Alert, Pressable, ScrollView, StyleSheet, View } from 'react-native';

import { unwrap } from '@/api/client';
import { api } from '@/auth/session';
import { Button } from '@/components/Button';
import { Chip } from '@/components/Chip';
import { Icon } from '@/components/Icon';
import { Poster } from '@/components/Poster';
import { Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { useShow } from '@/data/queries';
import { markEpisode, openLog, setWatchlist } from '@/lib/actions';
import { episodeCode } from '@/lib/format';
import { useOutbox } from '@/offline';
import { queryClient } from '@/query/client';
import { useTheme } from '@/theme';

/** A show: progress, season tabs and episodes to tick. */
export default function ShowScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { colors, radius, gutter } = useTheme();
  const show = useShow(id);
  const queued = useOutbox().filter((i) => i.kind === 'episodes' && i.meta?.show === id && i.status === 'pending');
  const seasons = (show.data?.seasons ?? []).filter((s) => (s.season_number ?? 0) > 0 || (show.data?.seasons ?? []).length === 1);
  const [picked, setPicked] = useState<number | null>(null);
  const current = picked ?? firstUnfinished(seasons) ?? seasons[0]?.season_number ?? 1;
  const season = seasons.find((s) => s.season_number === current);
  const title = show.data?.title?.title ?? '';
  const today = new Date().toISOString().slice(0, 10);

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
    <ScrollView style={{ backgroundColor: colors.bgCanvas }} contentContainerStyle={{ padding: gutter.compact, gap: 16, paddingBottom: 40 }}>
      <Stack.Screen options={{ title: '' }} />
      {show.isPending ? <ActivityIndicator color={colors.textMuted} /> : null}
      {show.data ? (
        <>
          <View style={[styles.hero, { backgroundColor: colors.bgSurface, borderRadius: radius.hero }]}>
            <Poster path={show.data.title?.poster_path} title={title} width={96} />
            <View style={{ flex: 1, gap: 6 }}>
              <Text variant="title" accessibilityRole="header">
                {title}
              </Text>
              <Text variant="muted" num>
                {show.data.progress?.watched_episodes ?? 0} of {show.data.progress?.aired_episodes ?? 0} aired episodes
              </Text>
              <Progress percent={show.data.progress?.percent ?? 0} />
              <View style={{ flexDirection: 'row', gap: 8, marginTop: 4 }}>
                <Button
                  kind="quiet"
                  title={show.data.state?.status === 'watchlist' ? 'On watchlist' : 'Watchlist'}
                  onPress={() => setWatchlist({ kind: 'tv', id, title }, show.data?.state?.status !== 'watchlist')}
                />
              </View>
            </View>
          </View>
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
            {seasons.map((s) => (
              <Chip key={s.season_number} label={`Season ${s.season_number}`} selected={s.season_number === current} onPress={() => setPicked(s.season_number!)} />
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
                      onPress={() => aired && openLog({ kind: 'tv', id, title, poster: show.data?.title?.poster_path, season: e.season_number, episode: e.episode_number })}
                      onLongPress={() =>
                        Alert.alert(`Mark up to ${episodeCode(e.season_number, e.episode_number)}?`, 'Every aired episode before it is marked watched too.', [
                          { text: 'Cancel', style: 'cancel' },
                          { text: 'Mark', onPress: () => markEpisode(id, title, e.season_number!, e.episode_number!, 'up_to') },
                        ])
                      }
                      accessibilityHint="Opens the log sheet; long-press marks up to here"
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
  hero: { flexDirection: 'row', gap: 16, padding: 16, alignItems: 'center' },
  episode: { flexDirection: 'row', alignItems: 'center', gap: 12, paddingVertical: 6 },
  tick: { width: 44, height: 44, alignItems: 'center', justifyContent: 'center' },
  bar: { height: 6, borderRadius: 3, overflow: 'hidden' },
});
