import { Pressable, StyleSheet, View } from 'react-native';

import type { components } from '@/api/schema';
import { Poster } from '@/components/Poster';
import { Text } from '@/components/Text';
import { episodeCode, starText, year } from '@/lib/format';
import { useTheme } from '@/theme';

// The diary's pieces: the recent posters, the ratings chart, the totals and
// one timeline row with its day box.

type Watch = components['schemas']['WatchEntry'];
type Bucket = components['schemas']['RatingBucket'];

/** Four posters in a row, each with its stars (and episode for a show). */
export function RecentRow({ title, watches, width, onOpen }: { title: string; watches: Watch[]; width: number; onOpen: (w: Watch) => void }) {
  const { colors } = useTheme();
  const gap = 10;
  const w = Math.floor((width - gap * 3) / 4);
  if (!watches.length || w <= 0) return null;
  return (
    <View style={{ gap: 10 }}>
      <Text variant="caption" style={styles.eyebrow}>
        {title}
      </Text>
      <View style={{ flexDirection: 'row', gap }}>
        {watches.map((x) => (
          <Pressable
            key={x.id}
            onPress={() => onOpen(x)}
            accessibilityRole="button"
            accessibilityLabel={`${x.title}${x.subject_kind === 'episode' ? `, ${episodeCode(x.season_number, x.episode_number)}` : ''}${x.rating ? `, ${x.rating / 2} stars` : ''}`}
            style={({ pressed }) => ({ width: w, gap: 4, opacity: pressed ? 0.8 : 1 })}
          >
            <Poster path={x.poster_path} title={x.title} width={w} kind={x.tmdb_kind} tmdbId={x.tmdb_id} />
            <Text variant="caption" numberOfLines={1} num style={{ color: x.rating ? colors.accentYours : colors.textMuted }}>
              {x.subject_kind === 'episode' ? `${episodeCode(x.season_number, x.episode_number)}  ` : ''}
              {starText(x.rating)}
              {x.liked ? ' ♥' : ''}
            </Text>
          </Pressable>
        ))}
      </View>
    </View>
  );
}

/** Half-star buckets as bars, one star on the left and five on the right. */
export function RatingsChart({ buckets, rated, average }: { buckets: Bucket[]; rated: number; average?: number }) {
  const { colors } = useTheme();
  const counts = Array.from({ length: 10 }, (_, i) => buckets.find((b) => b.rating === i + 1)?.watches ?? 0);
  const max = Math.max(1, ...counts);
  const H = 72;
  return (
    <View style={{ gap: 8 }} accessible accessibilityLabel={`Ratings: ${rated} rated${average ? `, average ${average.toFixed(1)} out of 10` : ''}`}>
      <View style={styles.chartHead}>
        <Text variant="caption" style={styles.eyebrow}>
          Ratings
        </Text>
        <Text variant="caption" num>
          {rated} rated{average ? ` · average ${starText(Math.round(average))}` : ''}
        </Text>
      </View>
      <View style={styles.chartRow}>
        <Text variant="caption" style={{ color: colors.accentYours }}>
          ★
        </Text>
        <View style={[styles.bars, { height: H }]}>
          {counts.map((n, i) => (
            <View key={i} style={styles.barSlot}>
              <View
                style={{
                  height: Math.max(2, Math.round((n / max) * H)),
                  backgroundColor: n ? colors.borderStrong : colors.borderSubtle,
                  borderTopLeftRadius: 2,
                  borderTopRightRadius: 2,
                }}
              />
            </View>
          ))}
        </View>
        <Text variant="caption" style={{ color: colors.accentYours }}>
          ★★★★★
        </Text>
      </View>
    </View>
  );
}

/** "Films  358 / 127 this year" rows. */
export function TotalRow({ label, total, thisYear }: { label: string; total?: number; thisYear?: number }) {
  const { colors } = useTheme();
  return (
    <View style={[styles.total, { borderTopColor: colors.borderSubtle }]}>
      <Text style={{ flex: 1, fontSize: 17 }}>{label}</Text>
      <Text variant="muted" num style={{ fontSize: 16 }}>
        {total ?? '–'}
        {thisYear != null ? ` / ${thisYear} this year` : ''}
      </Text>
    </View>
  );
}

/** A month heading across the whole list. */
export function MonthBand({ title, inset }: { title: string; inset: number }) {
  const { colors } = useTheme();
  return (
    <View style={[styles.band, { backgroundColor: colors.bgSurface, marginHorizontal: -inset, paddingHorizontal: inset }]}>
      <Text variant="caption" style={[styles.eyebrow, { color: colors.textMuted }]}>
        {title}
      </Text>
    </View>
  );
}

/** One diary entry: the day in a box, the poster, the title and year, the stars. */
export function TimelineRow({ w, selected, first, onPress }: { w: Watch; selected?: boolean; first?: boolean; onPress: () => void }) {
  const { colors } = useTheme();
  const day = Number((w.watched_on ?? '').slice(8, 10)) || '';
  const extra = [
    w.subject_kind === 'episode' ? episodeCode(w.season_number, w.episode_number) : null,
    w.venue_name_at_watch ?? w.provider_name,
    w.is_rewatch ? 'Rewatch' : null,
  ]
    .filter(Boolean)
    .join(' · ');
  return (
    <Pressable
      onPress={onPress}
      accessibilityRole="button"
      accessibilityState={selected ? { selected: true } : undefined}
      accessibilityLabel={`${w.watched_on}, ${w.title}${extra ? `, ${extra}` : ''}${w.rating ? `, ${w.rating / 2} stars` : ''}`}
      style={({ pressed }) => [styles.row, { opacity: pressed ? 0.75 : 1, backgroundColor: selected ? colors.bgElevated : 'transparent' }]}
    >
      <View style={[styles.dayBox, { borderColor: colors.borderSubtle }]}>
        <Text num style={{ fontSize: 20, color: colors.textMuted }}>
          {day}
        </Text>
      </View>
      <Poster path={w.poster_path} title={w.title} width={40} kind={w.tmdb_kind} tmdbId={w.tmdb_id} />
      <View style={[styles.rowText, !first && { borderTopColor: colors.borderSubtle, borderTopWidth: StyleSheet.hairlineWidth }]}>
        <Text variant="label" numberOfLines={2} style={{ fontSize: 16, lineHeight: 21 }}>
          {w.title}
          <Text variant="muted" num style={{ fontSize: 13 }}>
            {year(w.release_date) ? `  ${year(w.release_date)}` : ''}
          </Text>
        </Text>
        {w.rating || w.liked || extra ? (
          <Text variant="muted" numberOfLines={1} style={{ fontSize: 13 }}>
            {w.rating ? <Text style={{ color: colors.accentYours, fontSize: 13 }}>{starText(w.rating)} </Text> : null}
            {w.liked ? <Text style={{ color: colors.accentYours, fontSize: 13 }}>♥ </Text> : null}
            {extra}
          </Text>
        ) : null}
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  eyebrow: { textTransform: 'uppercase', letterSpacing: 1.2, fontSize: 12.5 },
  chartHead: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'baseline' },
  chartRow: { flexDirection: 'row', alignItems: 'flex-end', gap: 8 },
  bars: { flex: 1, flexDirection: 'row', alignItems: 'flex-end', gap: 3 },
  barSlot: { flex: 1, justifyContent: 'flex-end' },
  total: { flexDirection: 'row', alignItems: 'center', minHeight: 52, borderTopWidth: StyleSheet.hairlineWidth },
  band: { paddingVertical: 10, marginTop: 8, marginBottom: 4 },
  row: { flexDirection: 'row', alignItems: 'center', gap: 12, paddingVertical: 6, borderRadius: 10, borderCurve: 'continuous' },
  dayBox: { width: 48, height: 48, borderWidth: 1, borderRadius: 6, borderCurve: 'continuous', alignItems: 'center', justifyContent: 'center' },
  rowText: { flex: 1, alignSelf: 'stretch', justifyContent: 'center', gap: 2, paddingVertical: 6 },
});
