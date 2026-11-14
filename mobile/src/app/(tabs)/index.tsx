import { router } from 'expo-router';
import { ActivityIndicator, FlatList, Pressable, StyleSheet, View } from 'react-native';

import type { components } from '@/api/schema';
import { Button } from '@/components/Button';
import { ConnectionNote } from '@/components/ConnectionNote';
import { Icon } from '@/components/Icon';
import { OutboxBanner } from '@/components/OutboxBanner';
import { Poster } from '@/components/Poster';
import { Panel, Screen } from '@/components/Screen';
import { Text } from '@/components/Text';
import { TitleRow } from '@/components/TitleRow';
import { useDiary, usePreferences, useShowQueue, useTop10 } from '@/data/queries';
import { markEpisode, openTitle } from '@/lib/actions';
import { episodeCode, starText, tmdbKind } from '@/lib/format';
import { useTheme } from '@/theme';

type ChartItem = components['schemas']['Top10Result'];

/** Tonight: log a film, what to watch next, today's Top 10, your last logs. */
export default function Tonight() {
  const { colors } = useTheme();
  const prefs = usePreferences();
  const country = prefs.data?.country || 'US';
  const provider = prefs.data?.provider || 'netflix';
  const top10 = useTop10(country, provider);
  const queue = useShowQueue();
  const diary = useDiary(5);
  const recent = diary.data?.pages[0]?.watches?.slice(0, 5) ?? [];
  const watching = (queue.data?.shows ?? []).filter((s) => s.stage === 'watching' && s.next_episode).slice(0, 4);

  return (
    <Screen
      title="Tonight"
      refreshing={queue.isRefetching}
      onRefresh={() => void Promise.all([queue.refetch(), top10.refetch(), diary.refetch()])}
      right={
        <Pressable onPress={() => router.push('/search')} accessibilityRole="button" accessibilityLabel="Search" style={[styles.round, { backgroundColor: colors.bgElevated }]}>
          <Icon name="search" color={colors.textPrimary} />
        </Pressable>
      }
    >
      <ConnectionNote />
      <Button title="Log a film" onPress={() => router.push({ pathname: '/search', params: { mode: 'log' } })} />
      <OutboxBanner />

      {watching.length ? (
        <Panel title="Up next">
          {watching.map((s) => {
            const ep = s.next_episode!;
            return (
              <TitleRow
                key={s.tmdb_id}
                poster={s.poster_path}
                title={s.title ?? ''}
                subtitle={`${episodeCode(ep.season_number, ep.episode_number)}${ep.name ? ` · ${ep.name}` : ''}`}
                onPress={() => openTitle({ kind: 'tv', id: s.tmdb_id ?? '' })}
                right={
                  <Pressable
                    onPress={() => markEpisode(s.tmdb_id ?? '', s.title ?? '', ep.season_number!, ep.episode_number!)}
                    accessibilityRole="button"
                    accessibilityLabel={`Mark ${episodeCode(ep.season_number, ep.episode_number)} watched`}
                    hitSlop={8}
                    style={styles.tick}
                  >
                    <Icon name="check-circle" size={28} color={colors.textMuted} />
                  </Pressable>
                }
              />
            );
          })}
        </Panel>
      ) : null}

      <View style={{ gap: 8 }}>
        <Text variant="heading">
          Top 10 films · <Text variant="heading" style={{ color: colors.textMuted }}>{provider} {country}</Text>
        </Text>
        {top10.isPending ? (
          <ActivityIndicator color={colors.textMuted} />
        ) : top10.data?.items?.length ? (
          <FlatList
            horizontal
            data={top10.data.items}
            keyExtractor={(i) => `${i.rank}-${i.slug}`}
            showsHorizontalScrollIndicator={false}
            contentContainerStyle={{ gap: 4 }}
            renderItem={({ item }) => <ChartTile item={item} />}
          />
        ) : (
          <Text variant="muted">No chart for this service today.</Text>
        )}
      </View>

      {recent.length ? (
        <Panel title="Recently logged">
          {recent.map((w) => (
            <TitleRow
              key={w.id}
              poster={w.poster_path}
              title={w.title ?? ''}
              subtitle={
                <Text variant="muted" numberOfLines={1}>
                  {w.subject_kind === 'episode' ? `${episodeCode(w.season_number, w.episode_number)}  ` : ''}
                  <Text variant="muted" style={{ color: colors.accentYours }}>
                    {starText(w.rating)}
                  </Text>
                </Text>
              }
              onPress={() => router.push({ pathname: '/watch/[id]', params: { id: String(w.id) } })}
            />
          ))}
        </Panel>
      ) : null}
    </Screen>
  );
}

/** One chart entry: a large numeral beside the poster, movement under it. */
function ChartTile({ item }: { item: ChartItem }) {
  const { colors, fonts } = useTheme();
  const moved = item.previous_rank == null ? (item.days_in_top10 <= 1 ? 'new' : 're') : item.previous_rank - item.rank;
  const move =
    moved === 'new'
      ? { text: 'NEW', color: colors.deltaDebut }
      : moved === 're'
        ? { text: 'BACK', color: colors.deltaDebut }
        : moved > 0
          ? { text: `▲${moved}`, color: colors.deltaUp }
          : moved < 0
            ? { text: `▼${-moved}`, color: colors.deltaDown }
            : { text: '–', color: colors.textMuted };
  return (
    <Pressable
      onPress={() => item.tmdb_id && openTitle({ kind: tmdbKind(item.kind), id: item.tmdb_id, title: item.name })}
      disabled={!item.tmdb_id}
      accessibilityRole="button"
      accessibilityLabel={`Number ${item.rank}, ${item.name}`}
      style={styles.tile}
    >
      <Text style={{ fontFamily: fonts.display, fontSize: 64, lineHeight: 70, color: colors.textPrimary, marginRight: -10 }} num>
        {item.rank}
      </Text>
      <View style={{ gap: 4, width: 92 }}>
        <Poster path={item.poster_path} title={item.name} width={92} />
        <Text variant="caption" style={{ color: move.color }} num>
          {move.text}
        </Text>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  round: { width: 40, height: 40, borderRadius: 20, alignItems: 'center', justifyContent: 'center' },
  tick: { width: 44, height: 44, alignItems: 'center', justifyContent: 'center' },
  tile: { flexDirection: 'row', alignItems: 'flex-end', paddingRight: 8 },
});
