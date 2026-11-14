import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Image } from 'expo-image';

import type { ReactNode } from 'react';
import { ActivityIndicator, Linking, Pressable, ScrollView, StyleSheet, View } from 'react-native';

import type { components } from '@/api/schema';
import { Button } from '@/components/Button';
import { Poster } from '@/components/Poster';
import { RatingLogo } from '@/components/RatingLogo';
import { Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { useMyTitle, usePreferences, useTitleOverview } from '@/data/queries';
import { providerName } from '@/data/scope';
import { useWindowClass } from '@/layout/window-class';
import { openLog, setWatchlist } from '@/lib/actions';
import { runtimeText, starText, tmdbImage, year } from '@/lib/format';
import { useOutbox } from '@/offline';
import { useTheme } from '@/theme';

type Provider = components['schemas']['WatchProvider'];

/**
 * A title, lean: an artwork-tinted hero, scores, where to watch here, where
 * it charts today, and your watches. Attention charts and the timeline stay
 * on the web.
 */
export function TitleView({ kind: k, id, embedded }: { kind: string; id: string; embedded?: boolean }) {
  const kind = k === 'tv' ? 'tv' : 'movie';
  const { colors, radius, gutter } = useTheme();
  const insets = useSafeAreaInsets();
  const wide = useWindowClass() !== 'compact';
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
  const tint = snap?.poster_color;

  const avail = overview.data?.availability?.countries?.[country ?? ''] ?? Object.values(overview.data?.availability?.countries ?? {})[0];
  const streams: Provider[] = [...(avail?.flatrate ?? []), ...(avail?.free ?? []), ...(avail?.ads ?? [])];
  const r = overview.data?.ratings;

  return (
    <ScrollView style={{ backgroundColor: colors.bgCanvas }} contentContainerStyle={{ padding: gutter.compact, paddingTop: embedded ? insets.top + 12 : gutter.compact, gap: 16, paddingBottom: 40 }}>
      <View style={[styles.hero, wide && styles.heroWide, { backgroundColor: colors.bgSurface, borderRadius: radius.hero }]}>
        {tint ? <View style={[StyleSheet.absoluteFill, { backgroundColor: tint, opacity: 0.4, borderRadius: radius.hero }]} /> : null}
        <Poster path={poster} title={title} width={wide ? 180 : 132} />
        <View style={styles.heroText}>
          <Text variant="display" accessibilityRole="header">
            {title || ' '}
          </Text>
          <Text variant="muted" num>
            {[year(m?.release_date ?? snap?.release_date), runtimeText(m?.runtime ?? snap?.runtime), (m?.genres ?? []).slice(0, 2).join(', ')]
              .filter(Boolean)
              .join(' · ')}
          </Text>
          <View style={styles.actions}>
            {kind === 'movie' ? <Button title="Log" onPress={() => openLog({ kind, id, title, poster })} /> : null}
            <Button
              kind="quiet"
              title={onWatchlist ? 'On watchlist' : 'Watchlist'}
              onPress={() => setWatchlist({ kind, id, title }, !onWatchlist)}
            />
          </View>
        </View>
      </View>

      {overview.isLoading ? <ActivityIndicator color={colors.textMuted} /> : null}

      {r && (r.imdb || r.rotten_tomatoes || m?.vote_average) ? (
        <View style={styles.scores}>
          {r.imdb ? <Score label="IMDb" value={r.imdb.rating.toFixed(1)} logo={<RatingLogo source="imdb" />} /> : null}
          {r.rotten_tomatoes?.critics_score != null ? (
            <Score
              label={r.rotten_tomatoes.certified_fresh ? 'Tomatometer, Certified Fresh' : 'Tomatometer'}
              value={`${r.rotten_tomatoes.critics_score}%`}
              logo={<RatingLogo source="rt" rotten={r.rotten_tomatoes.critics_score < 60} certified={!!r.rotten_tomatoes.certified_fresh} />}
            />
          ) : null}
          {r.rotten_tomatoes?.audience_score != null ? (
            <Score label="Popcornmeter" value={`${r.rotten_tomatoes.audience_score}%`} logo={<RatingLogo source="rt-audience" rotten={r.rotten_tomatoes.audience_score < 60} />} />
          ) : null}
          {m?.vote_average ? <Score label="TMDB" value={m.vote_average.toFixed(1)} logo={<RatingLogo source="tmdb" />} /> : null}
        </View>
      ) : null}

      {state?.watch_count ? (
        <Panel title="Your watches">
          <Text>
            <Text num>{state.watch_count}</Text> {state.watch_count === 1 ? 'watch' : 'watches'}
            {state.rating ? <Text style={{ color: colors.accentYours }}>{`  ${starText(state.rating)}`}</Text> : null}
          </Text>
          {state.last_watched_at ? <Text variant="muted">Last on {new Date(state.last_watched_at).toLocaleDateString()}</Text> : null}
        </Panel>
      ) : null}

      {streams.length ? (
        <Panel title={`Where to watch${country ? ` in ${country}` : ''}`}>
          <View style={styles.providers}>
            {streams.map((p) => (
              <Pressable
                key={`${p.provider_id}-${p.monetization}`}
                accessibilityRole={p.url || p.link ? 'link' : undefined}
                accessibilityLabel={`${p.provider_name}${p.monetization === 'flatrate' ? '' : `, ${p.monetization}`}`}
                onPress={() => (p.url || p.link ? void Linking.openURL((p.url || p.link)!) : undefined)}
                style={styles.provider}
              >
                {p.logo_path ? (
                  <Image source={{ uri: tmdbImage(p.logo_path, 'w92')! }} style={[styles.logo, { borderColor: colors.posterHairline }]} />
                ) : (
                  <View style={[styles.logo, { backgroundColor: colors.bgElevated }]} />
                )}
                <Text variant="caption" numberOfLines={1} style={{ maxWidth: 72, textAlign: 'center' }}>
                  {p.provider_name}
                </Text>
              </Pressable>
            ))}
          </View>
        </Panel>
      ) : null}

      {overview.data?.current_positions?.length ? (
        <Panel title="Charting today">
          {overview.data.current_positions.slice(0, 6).map((c) => (
            <Text key={`${c.country}-${c.provider}-${c.category}`}>
              <Text variant="label" num>
                #{c.rank}
              </Text>{' '}
              {providerName(c.provider ?? '')} · {c.country}
            </Text>
          ))}
        </Panel>
      ) : null}

      {m?.overview ? (
        <Panel>
          <Text>{m.overview}</Text>
        </Panel>
      ) : null}
    </ScrollView>
  );
}

function Score({ label, value, logo }: { label: string; value: string; logo: ReactNode }) {
  const { colors, radius } = useTheme();
  return (
    <View style={[styles.score, { backgroundColor: colors.bgSurface, borderRadius: radius.panel }]} accessible accessibilityLabel={`${label} ${value}`}>
      {logo}
      <Text variant="title" num numberOfLines={1} adjustsFontSizeToFit>
        {value}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  hero: { flexDirection: 'row', gap: 16, padding: 16, alignItems: 'flex-end', overflow: 'hidden' },
  heroWide: { padding: 24, gap: 24 },
  heroText: { flex: 1, gap: 6 },
  actions: { flexDirection: 'row', flexWrap: 'wrap', gap: 8, marginTop: 8 },
  // One row of equal boxes, however many sources there are (up to four).
  scores: { flexDirection: 'row', gap: 8 },
  score: { flex: 1, minWidth: 0, paddingVertical: 12, paddingHorizontal: 12, gap: 6, alignItems: 'flex-start' },
  providers: { flexDirection: 'row', flexWrap: 'wrap', gap: 14 },
  provider: { alignItems: 'center', gap: 4 },
  logo: { width: 48, height: 48, borderRadius: 12, borderWidth: StyleSheet.hairlineWidth },
});
