import { Image } from 'expo-image';
import { type Href, Link } from 'expo-router';
import type { ReactNode } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

import { useTop10Of } from '@/data/top10now';
import { tmdbImage } from '@/lib/format';
import { onArtwork, useTheme } from '@/theme';

import { Icon } from './Icon';
import { Text } from './Text';
import { Top10Badge } from './Top10Badge';

// The pieces the TV and Movies tabs are built from: an artwork-led card for
// the list view, a poster tile for the grid, a countdown and a tick.

/** A row card: the poster flush on the left, text in the middle, an action on the right. */
export function MediaCard({
  poster,
  title,
  kind,
  tmdbId,
  onPress,
  selected,
  right,
  children,
  accessibilityLabel,
}: {
  poster?: string | null;
  title: string;
  onPress?: () => void;
  selected?: boolean;
  right?: ReactNode;
  children: ReactNode;
  accessibilityLabel?: string;
  /** With tmdbId: the Top 10 badge when the title charts today. */
  kind?: string | null;
  tmdbId?: string | null;
}) {
  const { colors, radius } = useTheme();
  const top = useTop10Of(kind, tmdbId);
  const uri = tmdbImage(poster, 'w185');
  return (
    <Pressable
      onPress={onPress}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel}
      accessibilityState={selected ? { selected: true } : undefined}
      style={({ pressed }) => [
        styles.card,
        {
          backgroundColor: selected ? colors.bgElevated : colors.bgSurface,
          borderRadius: radius.panel,
          borderCurve: 'continuous',
          borderColor: selected ? colors.borderStrong : 'transparent',
          opacity: pressed ? 0.8 : 1,
        },
      ]}
    >
      <View style={[styles.poster, { backgroundColor: colors.bgElevated }]}>
        {uri ? (
          <Image source={{ uri }} style={StyleSheet.absoluteFill} contentFit="cover" transition={150} accessible={false} />
        ) : (
          <Text variant="caption" style={{ textAlign: 'center' }}>
            {initials(title)}
          </Text>
        )}
        {top ? (
          <View style={styles.top10}>
            <Top10Badge size={20} />
          </View>
        ) : null}
      </View>
      <View style={styles.body}>{children}</View>
      {right ? <View style={styles.right}>{right}</View> : null}
    </Pressable>
  );
}

/** A poster in the grid, with a badge on it, an optional progress bar and the name under it. */
export function PosterTile({
  poster,
  title,
  width,
  kind,
  tmdbId,
  badge,
  badgeSide = 'left',
  percent,
  caption,
  onPress,
  href,
  selected,
}: {
  poster?: string | null;
  title: string;
  width: number;
  badge?: string | null;
  badgeSide?: 'left' | 'right';
  /** 0-100; a bar under the poster when given. */
  percent?: number | null;
  caption?: string | null;
  onPress?: () => void;
  /** Where the tile goes, as a link: on iOS a long press previews it. Use instead of onPress. */
  href?: Href;
  selected?: boolean;
  /** With tmdbId: the Top 10 badge when the title charts today. */
  kind?: string | null;
  tmdbId?: string | null;
}) {
  const { colors } = useTheme();
  const top = useTop10Of(kind, tmdbId);
  const uri = tmdbImage(poster, width > 120 ? 'w342' : 'w185');
  const tile = (
    <Pressable
      onPress={onPress}
      accessibilityRole={href ? 'link' : 'button'}
      accessibilityLabel={[title, badge, caption].filter(Boolean).join(', ')}
      accessibilityState={selected ? { selected: true } : undefined}
      style={({ pressed }) => [{ width, gap: 6, opacity: pressed ? 0.8 : 1 }]}
    >
      <View
        style={[
          styles.tile,
          {
            width,
            height: Math.round(width * 1.5),
            backgroundColor: colors.bgElevated,
            borderColor: selected ? colors.textPrimary : colors.posterHairline,
            borderWidth: selected ? 2 : StyleSheet.hairlineWidth,
          },
        ]}
      >
        {uri ? (
          <Image source={{ uri }} style={StyleSheet.absoluteFill} contentFit="cover" transition={150} accessible={false} />
        ) : (
          <Text variant="caption" style={{ textAlign: 'center', padding: 6 }}>
            {title}
          </Text>
        )}
        {top ? (
          <View style={styles.top10}>
            <Top10Badge size={width < 110 ? 22 : 28} />
          </View>
        ) : null}
        {badge ? (
          <View style={[styles.badge, badgeSide === 'left' ? { left: 6 } : { right: 6 }]}>
            <Text variant="label" num style={styles.badgeText}>
              {badge}
            </Text>
          </View>
        ) : null}
      </View>
      {percent != null ? <ProgressBar percent={percent} /> : null}
      <View>
        <Text variant="label" numberOfLines={1}>
          {title}
        </Text>
        {caption ? (
          <Text variant="caption" numberOfLines={1} style={{ color: colors.accentYours }}>
            {caption}
          </Text>
        ) : null}
      </View>
    </Pressable>
  );
  if (!href) return tile;
  return (
    <Link href={href} asChild>
      <Link.Trigger>{tile}</Link.Trigger>
      <Link.Preview />
    </Link>
  );
}

export function ProgressBar({ percent }: { percent: number }) {
  const { colors } = useTheme();
  const p = Math.max(0, Math.min(100, percent));
  return (
    <View style={[styles.bar, { backgroundColor: colors.bgElevated }]} accessible accessibilityLabel={`${Math.round(p)}% watched`}>
      <View style={{ width: `${p}%`, height: '100%', backgroundColor: p >= 100 ? colors.deltaUp : colors.accentYours, borderRadius: 2, borderCurve: 'continuous' }} />
    </View>
  );
}

/** Days to go, large, with the unit under it. */
export function Countdown({ days }: { days: number }) {
  const { colors } = useTheme();
  const big = days <= 0 ? 'Today' : days === 1 ? '1' : String(days);
  const unit = days <= 0 ? '' : days === 1 ? 'day' : 'days';
  return (
    <View style={{ alignItems: 'flex-end', minWidth: 52 }} accessible accessibilityLabel={days <= 0 ? 'Today' : `in ${days} ${unit}`}>
      <Text variant={days <= 0 ? 'heading' : 'title'} num style={{ fontSize: days <= 0 ? 18 : 28, lineHeight: 32 }}>
        {big}
      </Text>
      {unit ? (
        <Text variant="caption" style={{ textTransform: 'uppercase', letterSpacing: 0.6, color: colors.textMuted }}>
          {unit}
        </Text>
      ) : null}
    </View>
  );
}

/** The round "mark watched" button. */
export function TickButton({ label, pending, onPress }: { label: string; pending?: boolean; onPress: () => void }) {
  const { colors } = useTheme();
  return (
    <Pressable
      disabled={pending}
      onPress={onPress}
      accessibilityRole="button"
      accessibilityLabel={label}
      hitSlop={8}
      style={({ pressed }) => [styles.tick, { backgroundColor: pending ? colors.accentYours : colors.accentPrimary, opacity: pressed ? 0.8 : 1 }]}
    >
      <Icon name="check" size={22} color={colors.accentOn} />
    </Pressable>
  );
}

/** A small uppercase pill (Premiere, Finale). */
export function Badge({ text }: { text: string }) {
  const { colors } = useTheme();
  return (
    <View style={[styles.pill, { backgroundColor: colors.accentPrimary }]}>
      <Text variant="caption" style={{ color: colors.accentOn, fontSize: 11, letterSpacing: 0.6, textTransform: 'uppercase' }}>
        {text}
      </Text>
    </View>
  );
}

function initials(title: string) {
  return title
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0])
    .join('');
}

const styles = StyleSheet.create({
  card: { flexDirection: 'row', alignItems: 'stretch', overflow: 'hidden', minHeight: 108, borderWidth: 1 },
  poster: { width: 72, alignItems: 'center', justifyContent: 'center', overflow: 'hidden' },
  body: { flex: 1, paddingVertical: 12, paddingHorizontal: 14, gap: 4, justifyContent: 'center' },
  right: { paddingRight: 14, justifyContent: 'center' },
  tile: { borderRadius: 10, borderCurve: 'continuous', overflow: 'hidden', alignItems: 'center', justifyContent: 'center' },
  top10: { position: 'absolute', top: 5, right: 5 },
  badge: { position: 'absolute', bottom: 6, backgroundColor: 'rgba(11,11,12,0.78)', borderRadius: 8, borderCurve: 'continuous', paddingHorizontal: 7, paddingVertical: 3 },
  badgeText: { color: onArtwork, fontSize: 12.5 },
  bar: { height: 4, borderRadius: 2, overflow: 'hidden' },
  tick: { width: 44, height: 44, borderRadius: 22, alignItems: 'center', justifyContent: 'center' },
  pill: { borderRadius: 6, borderCurve: 'continuous', paddingHorizontal: 6, paddingVertical: 2, alignSelf: 'flex-start' },
});
