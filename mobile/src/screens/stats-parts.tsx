import { Image } from 'expo-image';
import { type ReactNode, useState } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

import type { components } from '@/api/schema';
import { Icon, type IconName } from '@/components/Icon';
import { PillFill } from '@/components/PillFill';
import { Text } from '@/components/Text';
import { tmdbImage } from '@/lib/format';
import { useTheme } from '@/theme';

// The pieces the profile and the detailed statistics are built from: stat
// tiles, a months/days/hours duration, horizontal bars, a bar chart, ranked
// lists and people. Screening Room: big tabular numerals on the surface step,
// the accent only for what is yours.

type Person = components['schemas']['PersonStat'];

/** Minutes as months (30 days), days and hours. */
export function splitDuration(minutes: number): { months: number; days: number; hours: number } {
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);
  return { months: Math.floor(days / 30), days: days % 30, hours: hours % 24 };
}

/** "3d 4h" style, for small spaces. */
export function shortDuration(minutes: number): string {
  if (minutes > 0 && minutes < 60) return `${minutes}m`;
  const { months, days, hours } = splitDuration(minutes);
  const parts = [months ? `${months}mo` : '', days ? `${days}d` : '', hours || (!months && !days) ? `${hours}h` : ''];
  return parts.filter(Boolean).join(' ');
}

export const hours = (minutes: number) => `${Math.round(minutes / 60).toLocaleString()} h`;

/** A section heading with an optional "see all" chevron. */
export function SectionHead({ title, onMore, extra }: { title: string; onMore?: () => void; extra?: string }) {
  const { colors } = useTheme();
  const body = (
    <View style={styles.head}>
      <Text variant="title" style={{ flex: 1 }} accessibilityRole="header">
        {title}
        {extra ? (
          <Text variant="muted" num>
            {'  '}
            {extra}
          </Text>
        ) : null}
      </Text>
      {onMore ? <Icon name="chevron" size={22} color={colors.accentYours} /> : null}
    </View>
  );
  return onMore ? (
    <Pressable onPress={onMore} accessibilityRole="button" accessibilityHint="Shows more" hitSlop={6}>
      {body}
    </Pressable>
  ) : (
    body
  );
}

/** A number with what it counts, on a rounded tile. */
export function StatTile({ value, label, icon, onPress, wide }: { value: string; label: string; icon?: IconName; onPress?: () => void; wide?: boolean }) {
  const { colors, radius } = useTheme();
  return (
    <Pressable
      onPress={onPress}
      disabled={!onPress}
      accessibilityRole={onPress ? 'button' : undefined}
      accessibilityLabel={`${value} ${label}`}
      style={({ pressed }) => [
        styles.tile,
        wide && { minWidth: 160 },
        { backgroundColor: colors.bgSurface, borderRadius: radius.panel, opacity: pressed ? 0.8 : 1 },
      ]}
    >
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: 4 }}>
        {icon ? <Icon name={icon} size={24} color={colors.accentYours} /> : null}
        <Text variant="display" num style={{ fontSize: 30, lineHeight: 36 }}>
          {value}
        </Text>
      </View>
      <Text variant="muted" numberOfLines={1}>
        {label}
      </Text>
    </Pressable>
  );
}

/** Months, days and hours, each a big numeral with its unit. */
export function Duration({ minutes, size = 30 }: { minutes: number; size?: number }) {
  const { months, days, hours: h } = splitDuration(minutes);
  const part = (n: number, unit: string) => (
    <View style={styles.durPart} key={unit}>
      <Text variant="display" num style={{ fontSize: size, lineHeight: size + 6 }}>
        {n}
      </Text>
      <Text variant="caption" style={styles.unit}>
        {unit}
      </Text>
    </View>
  );
  return (
    <View style={styles.duration} accessible accessibilityLabel={`${months} months, ${days} days, ${h} hours`}>
      {months ? part(months, months === 1 ? 'month' : 'months') : null}
      {months || days ? part(days, days === 1 ? 'day' : 'days') : null}
      {part(h, h === 1 ? 'hour' : 'hours')}
    </View>
  );
}

/** A card on the surface step. */
export function Card({
  title,
  children,
  right,
  flex,
}: {
  title?: string;
  children: ReactNode;
  right?: ReactNode;
  /** In a row: share it equally. */ flex?: boolean;
}) {
  const { colors, radius } = useTheme();
  return (
    <View style={[styles.card, flex && { flex: 1, flexBasis: 0, minWidth: 0 }, { backgroundColor: colors.bgSurface, borderRadius: radius.panel }]}>
      {title || right ? (
        <View style={styles.cardHead}>
          {title ? (
            <Text variant="heading" style={{ flex: 1 }}>
              {title}
            </Text>
          ) : null}
          {right}
        </View>
      ) : null}
      {children}
    </View>
  );
}

/** Labelled horizontal bars, longest first as given. */
export function BarList({ rows, format = (n) => n.toLocaleString() }: { rows: { label: string; value: number }[]; format?: (n: number) => string }) {
  const { colors } = useTheme();
  const max = Math.max(1, ...rows.map((r) => r.value));
  return (
    <View style={{ gap: 10 }}>
      {rows.map((r) => (
        <View key={r.label} style={styles.barRow} accessible accessibilityLabel={`${r.label}: ${format(r.value)}`}>
          <Text numberOfLines={1} style={{ width: 132 }}>
            {r.label}
          </Text>
          <View style={styles.barTrack}>
            <View
              style={{
                width: `${Math.max(2, (r.value / max) * 100)}%`,
                height: 10,
                borderRadius: 5,
                backgroundColor: r.value ? colors.accentYours : colors.bgElevated,
              }}
            />
          </View>
          <Text variant="muted" num style={{ width: 52, textAlign: 'right' }}>
            {format(r.value)}
          </Text>
        </View>
      ))}
    </View>
  );
}

/** Vertical bars with labels under them; tapping one shows its value. */
export function BarChart({ bars, format, height = 140 }: { bars: { label: string; value: number }[]; format: (n: number) => string; height?: number }) {
  const { colors } = useTheme();
  const [picked, setPicked] = useState<number | null>(null);
  const max = Math.max(1, ...bars.map((b) => b.value));
  const shown = picked ?? bars.findLastIndex((b) => b.value > 0);
  return (
    <View style={{ gap: 6 }}>
      <Text variant="caption" num>
        {shown >= 0 && bars[shown] ? `${bars[shown].label}: ${format(bars[shown].value)}` : 'Tap a bar for details'}
      </Text>
      <View style={[styles.chart, { height }]}>
        {bars.map((b, i) => (
          <Pressable
            key={`${b.label}-${i}`}
            onPress={() => setPicked(i)}
            accessibilityRole="button"
            accessibilityLabel={`${b.label}: ${format(b.value)}`}
            style={styles.chartSlot}
          >
            <View
              style={{
                height: Math.max(3, Math.round((b.value / max) * (height - 4))),
                borderRadius: 4,
                backgroundColor: i === shown ? colors.accentYours : b.value ? colors.borderStrong : colors.bgElevated,
              }}
            />
          </Pressable>
        ))}
      </View>
      <View style={styles.chartLabels}>
        {bars.map((b, i) => (
          <Text key={`${b.label}-l${i}`} variant="caption" numberOfLines={1} style={{ flex: 1, textAlign: 'center', fontSize: 10.5 }}>
            {bars.length > 8 && i % 2 === 1 ? '' : b.label}
          </Text>
        ))}
      </View>
    </View>
  );
}

/** A numbered list: rank, name, value. */
export function RankList({ rows }: { rows: { key: string; title: string; detail?: string; value: string; onPress?: () => void }[] }) {
  const { colors } = useTheme();
  return (
    <View style={{ gap: 4 }}>
      {rows.map((r, i) => (
        <Pressable key={r.key} onPress={r.onPress} disabled={!r.onPress} style={styles.rank} accessibilityRole={r.onPress ? 'button' : undefined}>
          <Text variant="heading" num style={{ width: 26, color: colors.accentYours }}>
            {i + 1}
          </Text>
          <View style={{ flex: 1 }}>
            <Text numberOfLines={1}>{r.title}</Text>
            {r.detail ? (
              <Text variant="caption" numberOfLines={1}>
                {r.detail}
              </Text>
            ) : null}
          </View>
          <Text variant="muted" num>
            {r.value}
          </Text>
        </Pressable>
      ))}
    </View>
  );
}

/** People with their photos, ranked. */
export function PeopleList({ people, unit }: { people: Person[]; unit: (n: number) => string }) {
  const { colors } = useTheme();
  return (
    <View style={{ gap: 8 }}>
      {people.map((p, i) => (
        <View key={p.person_id} style={styles.rank} accessible accessibilityLabel={`${p.name}, ${unit(p.watches ?? 0)}`}>
          <Text variant="heading" num style={{ width: 26, color: colors.accentYours }}>
            {i + 1}
          </Text>
          <View style={[styles.face, { backgroundColor: colors.bgElevated }]}>
            {p.profile_path ? <Image source={{ uri: tmdbImage(p.profile_path, 'w185')! }} style={StyleSheet.absoluteFill} contentFit="cover" /> : null}
          </View>
          <Text style={{ flex: 1 }} numberOfLines={1}>
            {p.name}
          </Text>
          <Text variant="muted" num>
            {unit(p.watches ?? 0)}
          </Text>
        </View>
      ))}
    </View>
  );
}

/** A small one-of-several switch (Week / Months / Year). */
export function MiniSegment<T extends string>({ options, value, onChange }: { options: [T, string][]; value: T; onChange: (v: T) => void }) {
  const { colors } = useTheme();
  return (
    <View style={[styles.mini, { backgroundColor: colors.bgElevated }]} accessibilityRole="tablist">
      {options.map(([v, label]) => (
        <Pressable
          key={v}
          onPress={() => onChange(v)}
          accessibilityRole="tab"
          accessibilityState={{ selected: v === value }}
          style={styles.miniItem}
        >
          <PillFill on={v === value} color={colors.bgCanvas} radius={8} />
          <Text variant="label" style={{ color: v === value ? colors.textPrimary : colors.textMuted, fontSize: 13.5 }}>
            {label}
          </Text>
        </Pressable>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  head: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  tile: { paddingVertical: 14, paddingHorizontal: 16, gap: 2, minWidth: 120, alignItems: 'center' },
  duration: { flexDirection: 'row', alignItems: 'baseline', gap: 14, flexWrap: 'wrap' },
  durPart: { flexDirection: 'row', alignItems: 'baseline', gap: 4 },
  unit: { textTransform: 'uppercase', letterSpacing: 0.6 },
  card: { padding: 16, gap: 12 },
  cardHead: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  barRow: { flexDirection: 'row', alignItems: 'center', gap: 10 },
  barTrack: { flex: 1, justifyContent: 'center' },
  chart: { flexDirection: 'row', alignItems: 'flex-end', gap: 4 },
  chartSlot: { flex: 1, justifyContent: 'flex-end', height: '100%' },
  chartLabels: { flexDirection: 'row', gap: 4 },
  rank: { flexDirection: 'row', alignItems: 'center', gap: 10, minHeight: 40 },
  face: { width: 40, height: 40, borderRadius: 20, overflow: 'hidden' },
  mini: { flexDirection: 'row', borderRadius: 10, padding: 3 },
  miniItem: { flex: 1, alignItems: 'center', paddingVertical: 7, borderRadius: 8 },
});
