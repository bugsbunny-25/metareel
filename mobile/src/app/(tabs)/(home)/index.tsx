import { router } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, FlatList, Pressable, StyleSheet, View } from 'react-native';

import type { components } from '@/api/schema';
import { Button } from '@/components/Button';
import { ConnectionNote } from '@/components/ConnectionNote';
import { Icon } from '@/components/Icon';
import { OutboxBanner } from '@/components/OutboxBanner';
import { PosterTile } from '@/components/Media';
import { Numeral } from '@/components/Numeral';
import { Poster } from '@/components/Poster';
import { RatingLogo } from '@/components/RatingLogo';
import { ScopePicker } from '@/components/ScopePicker';
import { Panel, Screen } from '@/components/Screen';
import { Text } from '@/components/Text';
import { TitleRow } from '@/components/TitleRow';
import { useDiary, useInCinemas, usePreferences, useShowQueue, useTop10, useTrending } from '@/data/queries';
import { changeText, type LibraryState, libraryMark, movement, providerName, useLibrary, useScope } from '@/data/scope';
import { useWindowClass } from '@/layout/window-class';
import { markEpisode, openEpisode, openLog, openTitle } from '@/lib/actions';
import { episodeCode, starText, tmdbKind } from '@/lib/format';
import { searchAction } from '@/layout/root-header';
import { useTheme } from '@/theme';
import { nowShowingProps, stopNowShowing, useNowShowing } from '@/nowshowing';

type ChartItem = components['schemas']['Top10Result'];

/** Tonight: log a film, what to watch next, today's Top 10, your last logs. */
export default function Tonight() {
  const { colors } = useTheme();
  const prefs = usePreferences();
  const { country, provider } = useScope({ country: prefs.data?.country, provider: prefs.data?.provider });
  const top10 = useTop10(country, provider);
  const top10tv = useTop10(country, provider, 'tv-shows');
  const trending = useTrending();
  const cinemas = useInCinemas(country);
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
      onRefresh={() => void Promise.all([queue.refetch(), top10.refetch(), top10tv.refetch(), diary.refetch(), trending.refetch(), cinemas.refetch()])}
      actions={[searchAction]}
    >
      <ConnectionNote />
      <NowShowingCard />
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
                kind="tv"
                tmdbId={s.tmdb_id}
                subtitle={`${episodeCode(ep.season_number, ep.episode_number)}${ep.name ? ` · ${ep.name}` : ''}`}
                onPress={() => openEpisode(s.tmdb_id ?? '', ep.season_number ?? 0, ep.episode_number ?? 1)}
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

      <ListRail title="Trending" hint="Today on TMDB" items={trending.data?.items} loading={trending.isPending} />
      <ListRail title="In cinemas" hint={`Now playing in ${country}`} items={cinemas.data?.items} loading={cinemas.isPending} />

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
        <ChartRail query={top10} yours={yours} empty="No film chart for this service today." />
        <ViewAll label="All of today's Top 10 films" onPress={() => router.push({ pathname: '/charts', params: { cat: 'movies' } })} />
      </View>

      <View style={{ gap: 8 }}>
        <Text variant="heading">Top 10 shows</Text>
        <ChartRail query={top10tv} yours={yours} empty="No show chart for this service today." />
        <ViewAll label="All of today's Top 10 shows" onPress={() => router.push({ pathname: '/charts', params: { cat: 'tv-shows' } })} />
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

type ListItem = components['schemas']['TitleListResponse']['items'][number];

/** A row of posters from a TMDB list (trending, in cinemas), with the Top 10 badge where it applies. */
function ListRail({ title, hint, items, loading }: { title: string; hint: string; items?: ListItem[]; loading: boolean }) {
  const { colors } = useTheme();
  if (!loading && !items?.length) return null;
  return (
    <View style={{ gap: 8 }}>
      <View style={styles.railHead}>
        <Text variant="heading">{title}</Text>
        <Text variant="caption">{hint}</Text>
      </View>
      {loading ? (
        <ActivityIndicator color={colors.textMuted} />
      ) : (
        <FlatList
          horizontal
          data={items}
          keyExtractor={(i) => `${i.tmdb_kind}-${i.tmdb_id}`}
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={{ gap: 12, paddingBottom: 4 }}
          renderItem={({ item }) => (
            <View style={{ width: 112, gap: 4 }}>
              <PosterTile
                width={112}
                poster={item.poster_path}
                title={item.title}
                kind={item.tmdb_kind}
                tmdbId={item.tmdb_id}
                onPress={() => openTitle({ kind: item.tmdb_kind, id: item.tmdb_id, title: item.title })}
              />
              <View style={styles.railMeta}>
                <Text variant="caption" num>
                  {[item.tmdb_kind === 'tv' ? 'Series' : 'Film', (item.release_date ?? '').slice(0, 4)].filter(Boolean).join(' · ')}
                </Text>
                {item.vote_count >= 20 ? (
                  <>
                    <RatingLogo source="tmdb" height={11} />
                    <Text variant="caption" num>
                      {item.vote_average.toFixed(1)}
                    </Text>
                  </>
                ) : null}
              </View>
            </View>
          )}
        />
      )}
    </View>
  );
}

/** "View all", right-aligned under a rail. */
function ViewAll({ label, onPress }: { label: string; onPress: () => void }) {
  const { colors } = useTheme();
  return (
    <Pressable onPress={onPress} accessibilityRole="link" accessibilityLabel={label} hitSlop={8} style={styles.viewAll}>
      <Text variant="label" style={{ color: colors.accentYours }}>
        View all
      </Text>
      <Icon name="chevron" size={16} color={colors.accentYours} />
    </Pressable>
  );
}

/** One Top 10 as a horizontal rail of numbered posters. */
function ChartRail({ query, yours, empty }: { query: ReturnType<typeof useTop10>; yours?: Map<string, LibraryState>; empty: string }) {
  const { colors } = useTheme();
  if (query.isPending) return <ActivityIndicator color={colors.textMuted} />;
  const items = query.data?.items ?? [];
  if (!items.length) return <Text variant="muted">{empty}</Text>;
  return (
    <FlatList
      horizontal
      data={items}
      keyExtractor={(i) => `${i.rank}-${i.slug}`}
      showsHorizontalScrollIndicator={false}
      contentContainerStyle={{ gap: 4, paddingTop: 6, paddingBottom: 12 }}
      renderItem={({ item }) => <ChartTile item={item} state={item.tmdb_id ? yours?.get(`${tmdbKind(item.kind)}:${item.tmdb_id}`) : undefined} />}
    />
  );
}

/**
 * One chart entry, drawn like the web's Home rail: a large outlined numeral
 * (Source Sans ExtraBold, stroked in the border colour, filled with the
 * canvas) with the poster overlapping it, and your titles marked by an
 * orange badge on the poster.
 */
function ChartTile({ item, state }: { item: ChartItem; state?: LibraryState }) {
  const { colors } = useTheme();
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
      <Numeral value={item.rank} size={wide ? 150 : 124} overlap={18} />
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

const styles = StyleSheet.create({
  tick: { width: 44, height: 44, alignItems: 'center', justifyContent: 'center' },
  railHead: { flexDirection: 'row', alignItems: 'baseline', gap: 10, flexWrap: 'wrap' },
  railMeta: { flexDirection: 'row', alignItems: 'center', gap: 4, flexWrap: 'wrap' },
  viewAll: { flexDirection: 'row', alignItems: 'center', gap: 2, alignSelf: 'flex-end', minHeight: 32 },
  tile: { flexDirection: 'row', alignItems: 'flex-end' },
  tilePoster: { zIndex: 1 },
  mark: { position: 'absolute', top: 6, right: 6, width: 22, height: 22, borderRadius: 11, alignItems: 'center', justifyContent: 'center' },
  scopeHead: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 8, flexWrap: 'wrap' },
  scopePill: { minHeight: 32, paddingHorizontal: 12, borderRadius: 16, justifyContent: 'center' },
});

/** The film you're watching right now, with Log it and Stop. */
function NowShowingCard() {
  const s = useNowShowing();
  if (!s) return null;
  const p = nowShowingProps(s);
  return (
    <Panel title="Now showing">
      <TitleRow
        poster={s.poster}
        title={s.title}
        subtitle={[s.venue, p.startLabel, p.endLabel].filter(Boolean).join(' · ')}
        onPress={() => openTitle({ kind: s.kind, id: s.id, title: s.title })}
      />
      <Button title="Log it" onPress={() => openLog({ kind: s.kind, id: s.id, title: s.title, poster: s.poster })} />
      <Button kind="quiet" title="Stop" onPress={() => void stopNowShowing()} />
    </Panel>
  );
}
