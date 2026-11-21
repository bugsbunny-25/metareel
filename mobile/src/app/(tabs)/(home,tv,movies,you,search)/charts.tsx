import { Stack, useLocalSearchParams } from 'expo-router';
import { useMemo, useState } from 'react';
import { ActivityIndicator, FlatList, Pressable, StyleSheet, View } from 'react-native';

import type { components } from '@/api/schema';
import { DateTimeField } from '@/components/DateTimeField';
import { Icon } from '@/components/Icon';
import { Numeral } from '@/components/Numeral';
import { Poster } from '@/components/Poster';
import { ScopePicker } from '@/components/ScopePicker';
import { Text } from '@/components/Text';
import { useChartDates, usePreferences, useTop10 } from '@/data/queries';
import { changeText, movement, providerName, useScope } from '@/data/scope';
import { openTitle } from '@/lib/actions';
import { tmdbKind } from '@/lib/format';
import { usePersisted } from '@/lib/persisted';
import { MiniSegment } from '@/screens/stats-parts';
import { useTheme } from '@/theme';

type Entry = components['schemas']['Top10Result'];
type Cat = 'movies' | 'tv-shows';

/**
 * Charts (from Tonight's "View all"): a Top 10 for the country and service
 * picked here, films or shows, on the latest day or any past day that has a
 * chart. The service and country are the same scope Tonight uses.
 */
export default function Charts() {
  const params = useLocalSearchParams<{ cat?: string }>();
  const { colors, gutter } = useTheme();
  const prefs = usePreferences();
  const { country, provider } = useScope({ country: prefs.data?.country, provider: prefs.data?.provider });
  const [saved, setSaved] = usePersisted<Cat>('charts.cat', 'movies', ['movies', 'tv-shows']);
  const [cat, setCatState] = useState<Cat>(params.cat === 'movies' || params.cat === 'tv-shows' ? params.cat : saved);
  const setCat = (c: Cat) => {
    setCatState(c);
    setSaved(c);
  };
  // The day picked, for the scope it was picked in: another country or service starts at its latest chart.
  const scopeKey = `${country}|${provider}`;
  const [picked, setPicked] = useState<{ scope: string; date?: string }>({ scope: scopeKey });
  const date = picked.scope === scopeKey ? picked.date : undefined; // undefined: the latest chart
  const setDate = (d?: string) => setPicked({ scope: scopeKey, date: d });
  const [picking, setPicking] = useState(false);

  const dates = useChartDates(country, provider);
  // The days this chart exists, newest first.
  const days = useMemo(() => {
    const want = cat === 'movies' ? 'movies' : 'tv_shows';
    return [...new Set((dates.data?.items ?? []).filter((d) => d.category === want && d.date).map((d) => d.date!))].sort().reverse();
  }, [dates.data, cat]);
  const chart = useTop10(country, provider, cat, date);
  const shown = chart.data?.date ?? date ?? days[0];
  const at = shown ? days.indexOf(shown) : -1;
  const older = at >= 0 && at < days.length - 1 ? days[at + 1] : null;
  const newer = at > 0 ? days[at - 1] : null;
  // A picked day without a chart snaps to the nearest earlier one.
  const pick = (d: string) => {
    const hit = days.find((x) => x <= d) ?? days[days.length - 1];
    setDate(hit === days[0] ? undefined : hit);
  };
  const items = chart.data?.items ?? [];
  const dayText = shown ? new Date(`${shown}T12:00:00`).toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' }) : '';

  return (
    <View style={{ flex: 1, backgroundColor: colors.bgCanvas }}>
      <Stack.Screen options={{ headerTitle: 'Top 10' }} />
      <FlatList
        data={items}
        keyExtractor={(e) => `${e.rank}-${e.slug}`}
        contentInsetAdjustmentBehavior="automatic"
        contentContainerStyle={{ padding: gutter.compact, paddingBottom: 40, gap: 2, width: '100%', maxWidth: 760, alignSelf: 'center' }}
        ListHeaderComponent={
          <View style={{ gap: 12, marginBottom: 8 }}>
            <Pressable onPress={() => setPicking((p) => !p)} accessibilityRole="button" accessibilityState={{ expanded: picking }} style={styles.scope}>
              <Text variant="title">
                {providerName(provider)} · {country}
              </Text>
              <Icon name={picking ? 'chevron-down' : 'chevron'} size={18} color={colors.textMuted} />
            </Pressable>
            {picking ? <ScopePicker country={country} provider={provider} /> : null}
            <MiniSegment
              options={[
                ['movies', 'Movies'],
                ['tv-shows', 'Shows'],
              ]}
              value={cat}
              onChange={(c) => {
                setCat(c);
                setDate(undefined);
              }}
            />
            <View style={styles.dateRow}>
              <StepButton label="Day before" icon="chevron-left" disabled={!older} onPress={() => older && setDate(older)} />
              <View style={{ flex: 1, alignItems: 'center' }}>
                {shown ? (
                  <DateTimeField value={shown} withTime={false} max={new Date()} onChange={pick} />
                ) : (
                  <Text variant="muted">{dayText}</Text>
                )}
              </View>
              <StepButton label="Day after" icon="chevron" disabled={!newer} onPress={() => newer && setDate(newer === days[0] ? undefined : newer)} />
            </View>
            {!date ? (
              <Text variant="caption" style={{ textAlign: 'center' }}>
                Latest chart
              </Text>
            ) : null}
          </View>
        }
        ListEmptyComponent={
          chart.isPending ? (
            <ActivityIndicator color={colors.textMuted} />
          ) : (
            <Text variant="muted">{chart.isError ? 'No chart for this service and day.' : 'Nothing charted.'}</Text>
          )
        }
        renderItem={({ item }) => <ChartRow entry={item} />}
      />
    </View>
  );
}

function StepButton({ label, icon, disabled, onPress }: { label: string; icon: 'chevron' | 'chevron-left'; disabled: boolean; onPress: () => void }) {
  const { colors } = useTheme();
  return (
    <Pressable
      onPress={onPress}
      disabled={disabled}
      accessibilityRole="button"
      accessibilityLabel={label}
      style={[styles.step, { backgroundColor: colors.bgElevated, opacity: disabled ? 0.35 : 1 }]}
    >
      <Icon name={icon} size={20} color={colors.textPrimary} />
    </Pressable>
  );
}

/**
 * The move since the day before (▲ ▼ New), then the rank numeral with the
 * poster over its edge (as on Tonight's rail), the title and days in the Top 10.
 */
function ChartRow({ entry: e }: { entry: Entry }) {
  const { colors } = useTheme();
  const m = movement(e);
  const change = changeText(m);
  // The chart carries the title's stored facts, so no lookup per row.
  const facts = [
    e.year ? String(e.year) : '',
    e.certification ?? '',
    e.season_count ? `${e.season_count} ${e.season_count === 1 ? 'season' : 'seasons'}` : '',
    e.genres?.[0] ?? '',
  ]
    .filter(Boolean)
    .join(' · ');
  const tone = change.startsWith('▲') ? colors.deltaUp : change.startsWith('▼') ? colors.deltaDown : change === 'New' ? colors.deltaDebut : colors.textMuted;
  return (
    <Pressable
      onPress={() => e.tmdb_id && openTitle({ kind: tmdbKind(e.kind), id: e.tmdb_id, title: e.name })}
      disabled={!e.tmdb_id}
      accessibilityRole="button"
      accessibilityLabel={`Number ${e.rank}: ${e.name}, ${change}`}
      style={({ pressed }) => [styles.row, { borderBottomColor: colors.borderSubtle, opacity: pressed ? 0.7 : 1 }]}
    >
      <Text variant="label" num style={[styles.change, { color: tone }]}>
        {change}
      </Text>
      {/* Right-aligned so the posters line up; the poster sits over the numeral's edge. */}
      <View style={styles.pair}>
        <Numeral value={e.rank} size={64} strokeWidth={1.5} overlap={5} />
        <Poster path={e.poster_path} title={e.name} width={44} />
      </View>
      <View style={{ flex: 1, gap: 2 }}>
        <Text variant="label" numberOfLines={2}>
          {e.name}
        </Text>
        {facts ? (
          <Text variant="caption" num numberOfLines={1}>
            {facts}
          </Text>
        ) : null}
        {e.overview ? (
          <Text variant="muted" numberOfLines={2}>
            {e.overview}
          </Text>
        ) : null}
        <Text variant="caption" num>
          {e.days_in_top10} {e.days_in_top10 === 1 ? 'day' : 'days'} in the Top 10
        </Text>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  scope: { flexDirection: 'row', alignItems: 'center', gap: 6, alignSelf: 'flex-start', minHeight: 36 },
  dateRow: { flexDirection: 'row', alignItems: 'center', gap: 10 },
  step: { width: 40, height: 40, borderRadius: 20, alignItems: 'center', justifyContent: 'center' },
  row: { flexDirection: 'row', alignItems: 'center', paddingVertical: 10, borderBottomWidth: StyleSheet.hairlineWidth },
  change: { width: 30, marginRight: 4, fontSize: 13 },
  // As wide as 10 and its poster (64pt numerals): a single digit leaves the least gap.
  pair: { width: 88, marginRight: 12, flexDirection: 'row', alignItems: 'center', justifyContent: 'flex-end' },
});
