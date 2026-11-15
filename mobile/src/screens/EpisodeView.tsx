import { Image } from 'expo-image';
import { router } from 'expo-router';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, View } from 'react-native';

import { Button } from '@/components/Button';
import { Badge } from '@/components/Media';
import { RatingLogo } from '@/components/RatingLogo';
import { Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { episodeBadge } from '@/data/collections';
import { useEpisode } from '@/data/queries';
import { useDetailScroll } from '@/layout/root-header';
import { useWindowClass } from '@/layout/window-class';
import { markEpisode, openLog, openTitle } from '@/lib/actions';
import { episodeCode, runtimeText, starText, tmdbImage } from '@/lib/format';
import { useOutbox } from '@/offline';
import { useTheme } from '@/theme';

import { watchedText } from './WatchView';

/**
 * One episode: its still, the show, name, air date, runtime and TMDB score,
 * the synopsis, who directed and wrote it, its guest cast, and your watches
 * of it, with Log and Mark watched.
 */
export function EpisodeView({
  show,
  season,
  episode,
  embedded,
  onOpenShow,
}: {
  show: string;
  season: number;
  episode: number;
  embedded?: boolean;
  /** Beside a list: show the whole series in the same pane instead of pushing it. */
  onOpenShow?: () => void;
}) {
  const { colors, gutter, radius } = useTheme();
  const detail = useDetailScroll(embedded);
  const wide = useWindowClass() !== 'compact';
  const q = useEpisode(show, season, episode);
  const pending = useOutbox().some(
    (i) => i.kind === 'episodes' && i.status === 'pending' && i.meta?.show === show && i.meta?.season === season && i.meta?.episode === episode,
  );
  const d = q.data;
  const viewShow = () => (onOpenShow ? onOpenShow() : openTitle({ kind: 'tv', id: show, title: d?.show?.title }));
  const e = d?.episode;
  const title = d?.show?.title ?? '';
  const today = new Date().toISOString().slice(0, 10);
  const aired = !!e?.air_date && e.air_date <= today;
  const watched = (d?.watches?.length ?? 0) > 0;
  const still = tmdbImage(e?.still_path, 'w780');
  const badge = e ? episodeBadge(e) : null;
  const meta = e
    ? [
        e.air_date ? new Date(`${e.air_date}T12:00:00`).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' }) : 'Air date TBA',
        runtimeText(e.runtime),
      ]
        .filter(Boolean)
        .join(' · ')
    : '';

  return (
    <ScrollView
      style={{ backgroundColor: colors.bgCanvas }}
      contentInsetAdjustmentBehavior={detail.contentInsetAdjustmentBehavior}
      contentContainerStyle={{
        padding: gutter.compact,
        paddingTop: detail.paddingTop ?? gutter.compact,
        gap: 16,
        paddingBottom: 40,
        width: '100%',
        maxWidth: 760,
        alignSelf: 'center',
      }}
    >
      {q.isPending ? <ActivityIndicator color={colors.textMuted} /> : null}
      {q.isError ? <Text variant="muted">Couldn&apos;t load this episode. It needs a connection.</Text> : null}
      {d && e ? (
        <>
          <View style={[styles.still, { borderRadius: radius.hero, backgroundColor: colors.bgSurface }]}>
            {still ? <Image source={{ uri: still }} style={StyleSheet.absoluteFill} contentFit="cover" transition={150} accessible={false} /> : null}
            <View style={[StyleSheet.absoluteFill, { backgroundColor: 'rgba(11,11,12,0.35)' }]} />
            <View style={styles.stillText}>
              <Pressable onPress={viewShow} accessibilityRole="link" hitSlop={6}>
                <Text variant="caption" style={{ color: colors.accentYours, textTransform: 'uppercase', letterSpacing: 0.6 }}>
                  {title} ›
                </Text>
              </Pressable>
              <Text variant="caption" num style={{ color: '#f5f5f7' }}>
                {episodeCode(season, episode)}
              </Text>
              <Text variant={wide ? 'display' : 'title'} accessibilityRole="header" style={{ color: '#f5f5f7' }}>
                {e.name || `Episode ${episode}`}
              </Text>
            </View>
          </View>

          <View style={{ gap: 10 }}>
            <View style={styles.metaRow}>
              {badge ? <Badge text={badge} /> : null}
              <Text variant="muted" num style={{ flexShrink: 1 }}>
                {meta}
              </Text>
              {e.vote_count ? (
                <View style={styles.score} accessible accessibilityLabel={`TMDB ${e.vote_average?.toFixed(1)}`}>
                  <RatingLogo source="tmdb" height={15} />
                  <Text variant="label" num>
                    {e.vote_average?.toFixed(1)}
                  </Text>
                </View>
              ) : null}
            </View>
            {!d.live ? <Text variant="caption">Showing what&apos;s saved; TMDB couldn&apos;t be reached for the rest.</Text> : null}
            <View style={styles.actions}>
              {aired ? (
                <>
                  <Button title="Log" onPress={() => openLog({ kind: 'tv', id: show, title, poster: d.show?.poster_path, season, episode })} />
                  <Button
                    kind="quiet"
                    title={pending ? 'Marked' : watched ? `Watched ${d.watches!.length}×` : 'Mark watched'}
                    onPress={() => (pending || watched ? undefined : markEpisode(show, title, season, episode))}
                  />
                </>
              ) : null}
              <Button kind="quiet" title="View whole series" onPress={viewShow} />
            </View>
          </View>

          {e.overview ? (
            <Panel>
              <Text>{e.overview}</Text>
            </Panel>
          ) : null}

          {e.directors?.length || e.writers?.length ? (
            <Panel>
              {e.directors?.length ? <Fact label={e.directors.length > 1 ? 'Directors' : 'Director'} value={e.directors.join(', ')} /> : null}
              {e.writers?.length ? <Fact label={e.writers.length > 1 ? 'Writers' : 'Writer'} value={e.writers.join(', ')} /> : null}
            </Panel>
          ) : null}

          {e.guest_stars?.length ? (
            <View style={{ gap: 10 }}>
              <Text variant="caption" style={styles.eyebrow}>
                Guest stars
              </Text>
              <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 12 }}>
                {e.guest_stars.map((g) => (
                  <View key={g.id} style={{ width: 84, gap: 4 }} accessible accessibilityLabel={`${g.name}${g.character ? ` as ${g.character}` : ''}`}>
                    <View style={[styles.face, { backgroundColor: colors.bgElevated }]}>
                      {g.profile_path ? (
                        <Image source={{ uri: tmdbImage(g.profile_path, 'w185')! }} style={StyleSheet.absoluteFill} contentFit="cover" accessible={false} />
                      ) : (
                        <Text variant="caption">
                          {(g.name ?? '')
                            .split(' ')
                            .map((w) => w[0])
                            .join('')
                            .slice(0, 2)}
                        </Text>
                      )}
                    </View>
                    <Text variant="label" numberOfLines={2} style={{ fontSize: 13, lineHeight: 17 }}>
                      {g.name}
                    </Text>
                    {g.character ? (
                      <Text variant="caption" numberOfLines={2}>
                        {g.character}
                      </Text>
                    ) : null}
                  </View>
                ))}
              </ScrollView>
            </View>
          ) : null}

          {d.watches?.length ? (
            <Panel title="Your watches">
              {d.watches.map((w) => (
                <Pressable
                  key={w.id}
                  onPress={() => router.push({ pathname: '/watch/[id]', params: { id: String(w.id) } })}
                  accessibilityRole="button"
                  style={({ pressed }) => [styles.watch, { opacity: pressed ? 0.7 : 1 }]}
                >
                  <Text style={{ flex: 1 }}>{watchedText(w)}</Text>
                  {w.rating ? <Text style={{ color: colors.accentYours }}>{starText(w.rating)}</Text> : null}
                  {w.liked ? <Text style={{ color: colors.accentYours }}>♥</Text> : null}
                </Pressable>
              ))}
            </Panel>
          ) : null}
        </>
      ) : null}
    </ScrollView>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.fact}>
      <Text variant="caption" style={{ width: 90 }}>
        {label}
      </Text>
      <Text style={{ flex: 1 }}>{value}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  still: { aspectRatio: 16 / 9, overflow: 'hidden', justifyContent: 'flex-end' },
  stillText: { padding: 16, gap: 2 },
  score: { flexDirection: 'row', alignItems: 'center', gap: 4 },
  metaRow: { flexDirection: 'row', alignItems: 'center', gap: 8, flexWrap: 'wrap' },
  actions: { flexDirection: 'row', flexWrap: 'wrap', gap: 8 },
  eyebrow: { textTransform: 'uppercase', letterSpacing: 1.2 },
  face: { width: 84, height: 112, borderRadius: 10, overflow: 'hidden', alignItems: 'center', justifyContent: 'center' },
  watch: { flexDirection: 'row', alignItems: 'center', gap: 10, minHeight: 36 },
  fact: { flexDirection: 'row', gap: 12, alignItems: 'baseline' },
});
