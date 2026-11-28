import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, FlatList, Pressable, RefreshControl, ScrollView, StyleSheet, useWindowDimensions, View } from 'react-native';

import { Button } from '@/components/Button';
import { Chip } from '@/components/Chip';
import { Panel } from '@/components/Screen';
import { Text } from '@/components/Text';
import { useDiscs } from '@/discs/data';
import { DiscCase, DiscSpine } from '@/discs/parts';
import { CASE, type DiscCopy, HIGHLIGHT, isBoxSet, label, MEDIA_FORMATS, type MediaFormat } from '@/discs/vocab';
import { chunk } from '@/lib/chunk';
import { formatMoney } from '@/lib/format';
import { tabBodyScroll } from '@/layout/TabbedRoot';
import { usePullToRefresh } from '@/lib/refresh';
import { StatTile } from '@/screens/stats-parts';
import { useTheme } from '@/theme';

/**
 * Your library as it stands in a cabinet: one shelf per format, spines in
 * title order. Tap a spine to pull it out (its cover, edition, picture and
 * sound, what you paid); tap again or Open for everything about it: a box
 * set lists its discs, each disc its film or show. Cases shows the fronts
 * instead. Scan adds a disc from its barcode. You's Library page, and
 * /collection on its own (standalone, with its own title).
 */
export function LibraryView({ standalone }: { standalone?: boolean }) {
  const { colors, gutter, radius } = useTheme();
  const discs = useDiscs();
  const pull = usePullToRefresh(() => discs.refetch());
  const { width } = useWindowDimensions();
  const [view, setView] = useState<'shelf' | 'cases'>('shelf');
  const [format, setFormat] = useState<MediaFormat | ''>('');
  const [picked, setPicked] = useState<number | null>(null);

  const items = discs.data?.items ?? [];
  const totals = discs.data?.totals;
  const rows = items.filter((c) => !format || c.release?.media_format === format || c.release?.parts.some((p) => p.media_format === format));
  const shelves = MEDIA_FORMATS.map(
    ([f, l]) => [f, l, rows.filter((c) => c.release?.media_format === f).sort((a, b) => (a.release?.title ?? '').localeCompare(b.release?.title ?? ''))] as const,
  ).filter(([, , cs]) => cs.length);
  const formats = MEDIA_FORMATS.filter(([f]) => totals?.by_format[f]);
  const spend = totals?.spend[0];
  const pickedCopy = items.find((c) => c.id === picked);
  const cols = Math.max(3, Math.floor((Math.min(width, 760) - gutter.compact * 2 + 12) / 124));
  const caseW = Math.floor((Math.min(width, 760) - gutter.compact * 2 - (cols - 1) * 12) / cols);
  // Cases is a virtualized grid (rows of cols), so a big collection only
  // draws the covers on screen; everything above it is the list's header.
  const caseRows = view === 'cases' ? chunk(rows.filter((c) => c.release), cols) : [];

  function tap(c: DiscCopy) {
    if (picked === c.id) router.push({ pathname: '/disc/[id]', params: { id: String(c.id) } });
    else setPicked(c.id);
  }

  return (
    <FlatList
      style={{ backgroundColor: colors.bgCanvas }}
      contentInsetAdjustmentBehavior={standalone ? 'automatic' : tabBodyScroll.contentInsetAdjustmentBehavior}
      contentContainerStyle={{
        padding: gutter.compact,
        paddingTop: standalone ? gutter.compact : tabBodyScroll.paddingTop,
        paddingBottom: 40,
      }}
      refreshControl={<RefreshControl refreshing={pull.refreshing} onRefresh={pull.onRefresh} tintColor={colors.textMuted} />}
      data={caseRows}
      keyExtractor={(row) => row.map((c) => c.id).join('|')}
      ItemSeparatorComponent={RowGap}
      ListHeaderComponent={
        <>
          {standalone ? <Stack.Screen options={{ headerTitle: 'Library' }} /> : null}
          <View style={[styles.column, { paddingBottom: caseRows.length ? 16 : 0 }]}>
            <Button title="Scan a barcode" onPress={() => router.push('/scan-disc')} />

            {discs.isPending ? <ActivityIndicator color={colors.textMuted} /> : null}
            {discs.error ? (
              <Panel>
                <Text variant="muted">Your collection didn’t load: {discs.error.message}</Text>
              </Panel>
            ) : null}
            {discs.data && !items.length ? (
              <Panel title="Your shelf is empty">
                <Text variant="muted">
                  Scan the barcode on the back of a DVD, Blu-ray or 4K disc: metareel looks the release up, links it to its films and keeps what you paid and where.
                  Box sets, Steelbooks and slipcovers included.
                </Text>
              </Panel>
            ) : null}

            {totals && items.length ? (
              <>
                <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 10 }}>
                  <StatTile value={String(totals.copies)} label={`Releases · ${totals.discs} discs`} />
                  <StatTile value={spend ? formatMoney(spend.minor, spend.currency) : '–'} label={spend ? `Spent · ${spend.copies} priced` : 'Add prices'} wide />
                  <StatTile value={String(totals.titles)} label="Films & shows" />
                  <StatTile value={String(totals.box_sets)} label="Box sets" />
                </ScrollView>

                <View
                  style={[
                    styles.mix,
                    {
                      backgroundColor: colors.bgSurface,
                      borderRadius: radius.panel,
                      borderCurve: 'continuous',
                    },
                  ]}
                >
                  <View style={styles.bar}>
                    {formats.map(([f]) => (
                      <View
                        key={f}
                        style={{
                          flex: totals.by_format[f],
                          backgroundColor: CASE[f].chart,
                        }}
                      />
                    ))}
                  </View>
                  <View style={styles.legend}>
                    {formats.map(([f, l]) => (
                      <Text key={f} variant="muted" num>
                        <Text style={{ color: CASE[f].chart }}>■ </Text>
                        {l} {totals.by_format[f]}
                      </Text>
                    ))}
                  </View>
                </View>

                <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
                  <Chip label="Everything" selected={!format} onPress={() => setFormat('')} />
                  {formats.map(([f, l]) => (
                    <Chip key={f} label={l} selected={format === f} onPress={() => setFormat(format === f ? '' : f)} />
                  ))}
                  <View style={{ width: 8 }} />
                  <Chip label="Shelf" selected={view === 'shelf'} onPress={() => setView('shelf')} />
                  <Chip label="Cases" selected={view === 'cases'} onPress={() => setView('cases')} />
                </ScrollView>
              </>
            ) : null}

            {view === 'shelf' && items.length ? (
              <View
                style={[
                  styles.cabinet,
                  {
                    backgroundColor: colors.bgSurface,
                    borderRadius: radius.panel,
                    borderCurve: 'continuous',
                  },
                ]}
              >
                {shelves.map(([f, l, cs]) => (
                  <View key={f} style={{ gap: 8 }}>
                    <Text variant="label">
                      {l} <Text variant="muted">{cs.length}</Text>
                    </Text>
                    <FlatList
                      horizontal
                      showsHorizontalScrollIndicator={false}
                      contentContainerStyle={styles.spines}
                      data={cs.filter((c) => c.release)}
                      keyExtractor={(c) => String(c.id)}
                      extraData={picked}
                      renderItem={({ item: c }) => (
                        <Pressable
                          onPress={() => tap(c)}
                          accessibilityRole="button"
                          accessibilityLabel={`${c.release!.title}${c.release!.edition ? `, ${c.release!.edition}` : ''}`}
                          accessibilityHint={picked === c.id ? 'Opens it' : 'Pulls it off the shelf'}
                        >
                          <DiscSpine release={c.release!} height={176} lifted={picked === c.id} />
                        </Pressable>
                      )}
                    />
                    <View style={[styles.plank, { backgroundColor: colors.bgElevated }]} />
                    {pickedCopy?.release && cs.some((c) => c.id === picked) ? <PulledOut copy={pickedCopy} /> : null}
                  </View>
                ))}
              </View>
            ) : null}
          </View>
        </>
      }
      renderItem={({ item: row }) => (
        <View style={[styles.column, styles.caseRow]}>
          {row.map((c) => (
            <Pressable
              key={c.id}
              style={{ width: caseW, gap: 4 }}
              onPress={() =>
                router.push({
                  pathname: '/disc/[id]',
                  params: { id: String(c.id) },
                })
              }
              accessibilityRole="button"
            >
              <DiscCase release={c.release!} width={caseW} />
              <Text variant="label" numberOfLines={1} style={{ fontSize: 14 }}>
                {c.release!.title}
              </Text>
              <Text variant="caption" numberOfLines={1}>
                {label(c.release!.media_format)}
                {c.release!.edition ? ` · ${c.release!.edition}` : ''}
              </Text>
            </Pressable>
          ))}
        </View>
      )}
    />
  );
}

const RowGap = () => <View style={{ height: 12 }} />;

/** A spine pulled off the shelf: its cover and the essentials. */
function PulledOut({ copy }: { copy: DiscCopy }) {
  const { colors, radius } = useTheme();
  const r = copy.release!;
  return (
    <Pressable
      onPress={() => router.push({ pathname: '/disc/[id]', params: { id: String(copy.id) } })}
      accessibilityRole="button"
      style={[styles.pulled, { backgroundColor: colors.bgElevated, borderRadius: radius.panel, borderCurve: 'continuous' }]}
    >
      <DiscCase release={r} width={104} />
      <View style={{ flex: 1, gap: 4 }}>
        <Text variant="caption">
          {label(r.media_format)}
          {isBoxSet(r) ? ` · ${r.disc_count} discs` : ''}
        </Text>
        <Text variant="heading" numberOfLines={2}>
          {r.title}
        </Text>
        {r.edition ? (
          <Text variant="muted" numberOfLines={1}>
            {r.edition}
          </Text>
        ) : null}
        {r.highlights.length ? (
          <View style={styles.badges}>
            {r.highlights.slice(0, 3).map((h) => (
              <Text key={h} style={[styles.badge, { backgroundColor: colors.bgSurface }]}>
                {HIGHLIGHT[h]}
              </Text>
            ))}
          </View>
        ) : null}
        <Text variant="muted" num numberOfLines={1}>
          {[copy.price_minor != null ? formatMoney(copy.price_minor, copy.currency) : '', copy.retailer].filter(Boolean).join(' · ') || 'Open for details'}
        </Text>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  column: { gap: 16, width: '100%', maxWidth: 760, alignSelf: 'center' },
  mix: { padding: 14, gap: 10 },
  bar: {
    flexDirection: 'row',
    height: 12,
    borderRadius: 6,
    overflow: 'hidden',
    gap: 2,
  },
  legend: { flexDirection: 'row', flexWrap: 'wrap', columnGap: 14, rowGap: 4 },
  cabinet: { padding: 14, gap: 22 },
  spines: {
    alignItems: 'flex-end',
    gap: 3,
    paddingTop: 16,
    paddingHorizontal: 2,
  },
  plank: { height: 10, borderRadius: 3, borderCurve: 'continuous', marginTop: -8, marginHorizontal: -6 },
  pulled: { flexDirection: 'row', gap: 14, padding: 12, marginTop: 4 },
  badges: { flexDirection: 'row', flexWrap: 'wrap', gap: 4 },
  badge: {
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 5,
    borderCurve: 'continuous',
    overflow: 'hidden',
    fontSize: 11,
    fontWeight: '700',
  },
  caseRow: { flexDirection: 'row', gap: 12 },
});
