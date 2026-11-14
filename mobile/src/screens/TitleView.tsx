import { ActivityIndicator, ScrollView, StyleSheet, View } from 'react-native';

import { Button } from '@/components/Button';
import { Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { useMyTitle, usePreferences, useTitleOverview } from '@/data/queries';
import { useDetailScroll } from '@/layout/root-header';
import { openLog, setWatchlist } from '@/lib/actions';
import { runtimeText, starText, year } from '@/lib/format';
import { useOutbox } from '@/offline';
import { useTheme } from '@/theme';

import { About, Charting, Hero, Scores, WhereToWatch } from './title-parts';

/**
 * A film: an artwork-tinted hero, scores, your watches, where to watch here,
 * its charts and the facts. Shows use ShowView, which adds the episodes.
 */
export function TitleView({ kind: k, id, embedded }: { kind: string; id: string; embedded?: boolean }) {
  const kind = k === 'tv' ? 'tv' : 'movie';
  const { colors, gutter } = useTheme();
  const detail = useDetailScroll(embedded);
  const prefs = usePreferences();
  const country = prefs.data?.country;
  const overview = useTitleOverview(kind, id, country);
  const mine = useMyTitle(kind, id);
  const queued = useOutbox().filter((i) => i.kind === 'watchlist' && i.meta?.id === id && i.status === 'pending');

  const m = overview.data?.metadata;
  const snap = mine.data?.title;
  const state = mine.data?.state;
  const title = m?.title ?? snap?.title ?? '';
  const poster = m?.poster_path ?? snap?.poster_path;
  const onWatchlist = queued.length ? !!queued[queued.length - 1].meta?.on : state?.status === 'watchlist';

  return (
    <ScrollView
      style={{ backgroundColor: colors.bgCanvas }}
      contentInsetAdjustmentBehavior={detail.contentInsetAdjustmentBehavior}
      contentContainerStyle={{ padding: gutter.compact, paddingTop: detail.paddingTop ?? gutter.compact, gap: 16, paddingBottom: 40 }}
    >
      <Hero
        poster={poster}
        title={title}
        tint={snap?.poster_color}
        meta={[year(m?.release_date ?? snap?.release_date), runtimeText(m?.runtime ?? snap?.runtime), (m?.genres ?? []).slice(0, 2).join(', ')]
          .filter(Boolean)
          .join(' · ')}
      >
        <View style={styles.actions}>
          {kind === 'movie' ? <Button title="Log" onPress={() => openLog({ kind, id, title, poster })} /> : null}
          <Button kind="quiet" title={onWatchlist ? 'On watchlist' : 'Watchlist'} onPress={() => setWatchlist({ kind, id, title }, !onWatchlist)} />
        </View>
      </Hero>

      {overview.isLoading ? <ActivityIndicator color={colors.textMuted} /> : null}
      <Scores overview={overview.data} />

      {state?.watch_count ? (
        <Panel title="Your watches">
          <Text>
            <Text num>{state.watch_count}</Text> {state.watch_count === 1 ? 'watch' : 'watches'}
            {state.rating ? <Text style={{ color: colors.accentYours }}>{`  ${starText(state.rating)}`}</Text> : null}
          </Text>
          {state.last_watched_at ? <Text variant="muted">Last on {new Date(state.last_watched_at).toLocaleDateString()}</Text> : null}
        </Panel>
      ) : null}

      <WhereToWatch overview={overview.data} country={country} />
      <Charting overview={overview.data} />
      <About metadata={m} kind={kind} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  actions: { flexDirection: 'row', flexWrap: 'wrap', gap: 8, marginTop: 8 },
});
