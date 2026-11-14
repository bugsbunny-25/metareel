import { router } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, FlatList, Pressable, ScrollView, StyleSheet, View } from 'react-native';

import type { components } from '@/api/schema';
import { Button } from '@/components/Button';
import { Chip } from '@/components/Chip';
import { ConnectionNote } from '@/components/ConnectionNote';
import { Icon } from '@/components/Icon';
import { OutboxBanner } from '@/components/OutboxBanner';
import { Poster } from '@/components/Poster';
import { Panel, Screen } from '@/components/Screen';
import { Text } from '@/components/Text';
import { TitleRow } from '@/components/TitleRow';
import { useDiary, usePreferences, useShowQueue, useTop10 } from '@/data/queries';
import { changeText, type LibraryState, libraryMark, movement, providerName, setScope, useChartReference, useLibrary, useScope } from '@/data/scope';
import { useWindowClass } from '@/layout/window-class';
import Svg, { Text as SvgText } from 'react-native-svg';
import { markEpisode, openTitle } from '@/lib/actions';
import { episodeCode, starText, tmdbKind } from '@/lib/format';
import { useTheme } from '@/theme';

type ChartItem = components['schemas']['Top10Result'];

/** Tonight: log a film, what to watch next, today's Top 10, your last logs. */
export default function Tonight() {
  const { colors } = useTheme();
  const prefs = usePreferences();
  const { country, provider } = useScope({ country: prefs.data?.country, provider: prefs.data?.provider });
  const top10 = useTop10(country, provider);
  const yours = useLibrary().data;
  const [picking, setPicking] = useState(false);
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
        <Pressable
          onPress={() => setPicking((p) => !p)}
          accessibilityRole="button"
          accessibilityState={{ expanded: picking }}
          accessibilityHint="Choose the country and service"
          style={styles.scopeHead}
        >
          <Text variant="heading">Top 10 films</Text>
          <View style={[styles.scopePill, { backgroundColor: colors.bgElevated }]}>
            <Text variant="label" style={{ fontSize: 13 }}>
              {providerName(provider)} · {country} ▾
            </Text>
          </View>
        </Pressable>
        {picking ? <ScopePicker country={country} provider={provider} /> : null}
        {top10.isPending ? (
          <ActivityIndicator color={colors.textMuted} />
        ) : top10.data?.items?.length ? (
          <FlatList
            horizontal
            data={top10.data.items}
            keyExtractor={(i) => `${i.rank}-${i.slug}`}
            showsHorizontalScrollIndicator={false}
            contentContainerStyle={{ gap: 4, paddingTop: 6, paddingBottom: 12 }}
            renderItem={({ item }) => <ChartTile item={item} state={item.tmdb_id ? yours?.get(`${tmdbKind(item.kind)}:${item.tmdb_id}`) : undefined} />}
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

/** Country and service chips for the Top 10 (the web's per-view ScopePicker). */
function ScopePicker({ country, provider }: { country: string; provider: string }) {
  const ref = useChartReference();
  if (!ref.data) return <ActivityIndicator />;
  const services = ref.data.providers.filter((p) => p.slug && p.countries?.includes(country));
  return (
    <View style={{ gap: 8 }}>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
        {services.map((p) => (
          <Chip key={p.slug} label={providerName(p.slug!)} selected={p.slug === provider} onPress={() => setScope({ provider: p.slug })} />
        ))}
      </ScrollView>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
        {ref.data.countries.map((c) => (
          <Chip
            key={c.code}
            label={c.name ?? c.code!}
            selected={c.code === country}
            onPress={() => {
              // Keep the service if it charts there, else take the first that does.
              const keep = ref.data!.providers.find((p) => p.slug === provider)?.countries?.includes(c.code!);
              setScope({ country: c.code, provider: keep ? provider : ref.data!.providers.find((p) => p.countries?.includes(c.code!))?.slug });
            }}
          />
        ))}
      </ScrollView>
    </View>
  );
}

/**
 * One chart entry, drawn like the web's Home rail: a large outlined numeral
 * (Source Sans ExtraBold, stroked in the border colour, filled with the
 * canvas) with the poster overlapping it, and your titles marked by an
 * orange badge on the poster.
 */
function ChartTile({ item, state }: { item: ChartItem; state?: LibraryState }) {
  const { colors, fonts } = useTheme();
  const wide = useWindowClass() !== 'compact';
  const mark = libraryMark(state);
  const m = movement(item);
  return (
    <Pressable
      onPress={() => item.tmdb_id && openTitle({ kind: tmdbKind(item.kind), id: item.tmdb_id, title: item.name })}
      disabled={!item.tmdb_id}
      accessibilityRole="button"
      accessibilityLabel={`Number ${item.rank}: ${item.name}${
        state?.status === 'watchlist' ? ', on your watchlist' : (state?.watch_count ?? 0) > 0 ? ', watched' : mark ? ', in your library' : ''
      }, ${changeText(m)}`}
      style={styles.tile}
    >
      <Numeral value={item.rank} size={wide ? 100 : 76} fill={colors.bgCanvas} stroke={colors.borderStrong} font={fonts.numeral} />
      <View style={styles.tilePoster}>
        <Poster path={item.poster_path} title={item.name} width={92} />
        {mark ? (
          <View style={[styles.mark, { backgroundColor: colors.accentYours }]}>
            <Icon name={mark} size={12} color={colors.bgCanvas} />
          </View>
        ) : null}
      </View>
    </Pressable>
  );
}

/** An outlined numeral, clipped to its 0.78em line like the web's .big. */
function Numeral({ value, size, fill, stroke, font }: { value: number; size: number; fill: string; stroke: string; font: string }) {
  const digits = String(value).length;
  const width = Math.ceil(size * (0.52 * digits - 0.06 * (digits - 1))) + 4;
  const height = Math.round(size * 0.78) + 2;
  return (
    <Svg width={width} height={height} style={{ marginRight: -14 }} accessible={false}>
      <SvgText
        x={width - 2}
        y={height - 3}
        textAnchor="end"
        fontFamily={font}
        fontSize={size}
        letterSpacing={-0.06 * size}
        fill={fill}
        stroke={stroke}
        strokeWidth={2}
      >
        {String(value)}
      </SvgText>
    </Svg>
  );
}

const styles = StyleSheet.create({
  round: { width: 40, height: 40, borderRadius: 20, alignItems: 'center', justifyContent: 'center' },
  tick: { width: 44, height: 44, alignItems: 'center', justifyContent: 'center' },
  tile: { flexDirection: 'row', alignItems: 'flex-end' },
  tilePoster: { zIndex: 1 },
  mark: { position: 'absolute', top: 6, right: 6, width: 22, height: 22, borderRadius: 11, alignItems: 'center', justifyContent: 'center' },
  scopeHead: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 8, flexWrap: 'wrap' },
  scopePill: { minHeight: 32, paddingHorizontal: 12, borderRadius: 16, justifyContent: 'center' },
});
