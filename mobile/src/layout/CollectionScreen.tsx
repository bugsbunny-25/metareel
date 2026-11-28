import { type ReactNode, useRef, useState } from 'react';
import { ActivityIndicator, Pressable, RefreshControl, SectionList, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Icon } from '@/components/Icon';
import { Placeholder } from '@/components/Placeholder';
import { Text } from '@/components/Text';
import type { Group } from '@/data/collections';
import { chunk } from '@/lib/chunk';
import { useTheme } from '@/theme';

import { ListDetail } from './ListDetail';
import { nativeRootHeader, type RootAction, RootActions, RootTitle, searchAction } from './root-header';

export { searchAction };

type Segment<S extends string> = { key: S; label: string };

/**
 * The TV and Movies tabs: a To watch / Upcoming switch pinned under the
 * title, a list or poster-grid view of grouped items, collapsible groups, and on wide windows the picked item beside the
 * list. The switch and the actions stay put while the list scrolls, so the
 * view can be changed from anywhere in it.
 */
export function CollectionScreen<T, S extends string>({
  title,
  view,
  onView,
  segments,
  segment,
  onSegment,
  groups,
  itemKey,
  renderRow,
  renderTile,
  summary,
  loading,
  error,
  empty,
  refreshing,
  onRefresh,
  banner,
  detail,
  placeholder,
}: {
  title: string;
  view: 'list' | 'grid';
  onView: (v: 'list' | 'grid') => void;
  segments: Segment<S>[];
  segment: S;
  onSegment: (s: S) => void;
  groups: Group<T>[];
  itemKey: (t: T) => string;
  renderRow: (t: T) => ReactNode;
  renderTile: (t: T, width: number) => ReactNode;
  summary?: string | null;
  loading?: boolean;
  error?: boolean;
  empty: string;
  refreshing?: boolean;
  onRefresh?: () => void;
  banner?: ReactNode;
  detail: ReactNode | null;
  placeholder: string;
}) {
  const { colors, gutter } = useTheme();
  const insets = useSafeAreaInsets();
  const [width, setWidth] = useState(0);
  // Groups that start collapsed ("Haven't watched in a while") until opened.
  const [toggled, setToggled] = useState<Record<string, boolean>>({});
  const isCollapsed = (g: Group<T>) => toggled[g.key] ?? !!g.collapsed;

  // Each segment keeps its own scroll position: the list is remounted per
  // segment (so a switch never lands the new one mid-way down), and goes back
  // to where that segment was left once its content is laid out.
  const listRef = useRef<SectionList<{ key: string; items: T[] }, { key: string; title: string; count: number; collapsible: boolean; data: { key: string; items: T[] }[] }>>(null);
  const offsets = useRef<Partial<Record<S, number>>>({});
  const restoring = useRef<S | null>(null);
  const listHeight = useRef(0);
  const pickSegment = (s: S) => {
    if (s === segment) return;
    restoring.current = s;
    onSegment(s);
  };
  // Called as the remounted list's content grows (rows render in batches):
  // scroll as far toward the saved offset as the content allows, until it
  // fits or the user drags.
  const restoreScroll = (contentHeight: number) => {
    if (restoring.current !== segment) return;
    const y = offsets.current[segment];
    if (y == null || y <= 0) {
      restoring.current = null;
      return;
    }
    if (loading) return; // again when the content arrives
    const max = Math.max(0, contentHeight - listHeight.current);
    // A frame later: Android drops a scroll past the content it has laid out
    // so far. Scroll events stay unrecorded until the last step is sent.
    requestAnimationFrame(() => {
      listRef.current?.getScrollResponder()?.scrollTo({ y: Math.min(y, max), animated: false });
      if (max >= y && restoring.current === segment) restoring.current = null;
    });
  };

  const pad = gutter.compact;
  const gap = 12;
  const inner = Math.max(0, width - pad * 2);
  const cols = Math.max(3, Math.min(8, Math.floor((inner + gap) / (104 + gap))));
  const tileWidth = Math.floor((inner - gap * (cols - 1)) / cols);

  type Row = { key: string; items: T[] };
  // React Compiler memoizes this on what it reads.
  const sections = groups.map((g) => {
    const shown = isCollapsed(g) ? [] : g.data;
    const rows: Row[] =
      view === 'grid'
        ? chunk(shown, cols).map((items) => ({ key: items.map(itemKey).join('|'), items }))
        : shown.map((t) => ({ key: itemKey(t), items: [t] }));
    return { key: g.key, title: g.title, count: g.data.length, collapsible: !!g.collapsed, data: rows };
  });

  const actions: RootAction[] = [
    view === 'grid'
      ? { label: 'List', sf: 'list.bullet', icon: 'list', onPress: () => onView('list') }
      : { label: 'Grid', sf: 'square.grid.2x2', icon: 'grid', onPress: () => onView('grid') },
    searchAction,
  ];

  const list = (
    <View style={{ flex: 1, backgroundColor: colors.bgCanvas }} onLayout={(e) => setWidth(Math.round(e.nativeEvent.layout.width))}>
      <View
        style={{
          paddingTop: nativeRootHeader ? 4 : insets.top + 12,
          paddingHorizontal: pad,
          paddingRight: pad + (nativeRootHeader ? 0 : insets.right),
          gap: 10,
        }}
      >
        {nativeRootHeader ? (
          <RootActions actions={actions} />
        ) : (
          <View style={styles.titleRow}>
            <RootTitle>{title}</RootTitle>
            <RootActions actions={actions} />
          </View>
        )}
        <Segmented segments={segments} value={segment} onChange={pickSegment} />
      </View>
      <SectionList
        key={segment}
        ref={listRef}
        style={{ flex: 1 }}
        onLayout={(e) => (listHeight.current = e.nativeEvent.layout.height)}
        onScroll={(e) => {
          if (restoring.current !== segment) offsets.current[segment] = e.nativeEvent.contentOffset.y;
        }}
        onScrollBeginDrag={() => (restoring.current = null)}
        scrollEventThrottle={32}
        onContentSizeChange={(_, h) => restoreScroll(h)}
        contentInsetAdjustmentBehavior={nativeRootHeader ? 'automatic' : undefined}
        contentContainerStyle={{ paddingHorizontal: pad, paddingRight: pad + (nativeRootHeader ? 0 : insets.right), paddingTop: 8, paddingBottom: 32 }}
        sections={sections}
        keyExtractor={(r) => r.key}
        stickySectionHeadersEnabled={false}
        refreshControl={onRefresh ? <RefreshControl refreshing={!!refreshing} onRefresh={onRefresh} tintColor={colors.textMuted} /> : undefined}
        ListHeaderComponent={
          <View style={{ gap: 10 }}>
            {banner}
            {summary ? (
              <Text variant="muted" num>
                {summary}
              </Text>
            ) : null}
          </View>
        }
        renderSectionHeader={({ section }) => (
          <Pressable
            disabled={!section.collapsible}
            onPress={() => setToggled((t) => ({ ...t, [section.key]: !(t[section.key] ?? true) }))}
            accessibilityRole={section.collapsible ? 'button' : 'header'}
            accessibilityState={section.collapsible ? { expanded: !!section.data.length } : undefined}
            style={styles.sectionHead}
          >
            <View style={[styles.sectionPill, { backgroundColor: colors.bgElevated }]}>
              <Text variant="caption" style={styles.sectionText}>
                {section.title}
              </Text>
              {section.collapsible ? (
                <>
                  <Text variant="caption" num>
                    {section.count}
                  </Text>
                  <Icon name={section.data.length ? 'chevron-down' : 'chevron'} size={14} color={colors.textMuted} />
                </>
              ) : null}
            </View>
          </Pressable>
        )}
        renderItem={({ item: row }) =>
          view === 'grid' ? (
            <View style={[styles.gridRow, { gap }]}>
              {row.items.map((t) => (
                <View key={itemKey(t)}>{renderTile(t, tileWidth)}</View>
              ))}
            </View>
          ) : (
            <View style={styles.listRow}>{renderRow(row.items[0])}</View>
          )
        }
        ListEmptyComponent={
          loading ? (
            <ActivityIndicator color={colors.textMuted} style={{ marginTop: 24 }} />
          ) : (
            <Text variant="muted" style={{ marginTop: 16 }}>
              {error ? "Couldn't load this. Pull to try again." : empty}
            </Text>
          )
        }
      />
    </View>
  );
  return <ListDetail list={list} detail={detail} placeholder={<Placeholder text={placeholder} />} />;
}

/** Tabs pinned under a root's title (TV and Movies' To watch / Upcoming, You's You / Diary / Library). */
export function Segmented<S extends string>({ segments, value, onChange }: { segments: Segment<S>[]; value: S; onChange: (s: S) => void }) {
  const { colors } = useTheme();
  return (
    <View style={[styles.segments, { borderBottomColor: colors.borderSubtle }]} accessibilityRole="tablist">
      {segments.map((s) => {
        const on = s.key === value;
        return (
          <Pressable
            key={s.key}
            onPress={() => onChange(s.key)}
            accessibilityRole="tab"
            accessibilityState={{ selected: on }}
            style={[styles.segment, { borderBottomColor: on ? colors.textPrimary : 'transparent' }]}
          >
            <Text variant="label" style={{ color: on ? colors.textPrimary : colors.textMuted, fontSize: 15, letterSpacing: 0.4, textTransform: 'uppercase' }}>
              {s.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  titleRow: { flexDirection: 'row', alignItems: 'center', gap: 12 },
  segments: { flexDirection: 'row', borderBottomWidth: StyleSheet.hairlineWidth },
  segment: { flex: 1, alignItems: 'center', paddingVertical: 10, borderBottomWidth: 2, marginBottom: -StyleSheet.hairlineWidth },
  sectionHead: { alignItems: 'center', marginTop: 18, marginBottom: 10 },
  sectionPill: { flexDirection: 'row', alignItems: 'center', gap: 6, paddingHorizontal: 14, paddingVertical: 7, borderRadius: 16, borderCurve: 'continuous' },
  sectionText: { textTransform: 'uppercase', letterSpacing: 0.6, fontSize: 12.5 },
  gridRow: { flexDirection: 'row', marginBottom: 18 },
  listRow: { marginBottom: 12, width: '100%', maxWidth: 760, alignSelf: 'center' },
});
