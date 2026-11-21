import type { ReactNode } from 'react';
import { StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { useWindowClass } from '@/layout/window-class';
import { useTheme } from '@/theme';

import { Segmented } from './CollectionScreen';
import { nativeRootHeader, type RootAction, RootActions, RootTitle } from './root-header';

/**
 * A tab root with its own tabs pinned under the title (You: You / Diary /
 * Library), as TV and Movies pin theirs. Each page brings its own scroll
 * view (see tabBodyScroll), so a page keeps its scroll position and pull to
 * refresh, and the tabs stay put while it scrolls.
 */
export function TabbedRoot<S extends string>({
  title,
  actions,
  segments,
  segment,
  onSegment,
  children,
}: {
  title: string;
  actions: RootAction[];
  segments: { key: S; label: string }[];
  segment: S;
  onSegment: (s: S) => void;
  children: ReactNode;
}) {
  const { colors, gutter } = useTheme();
  const insets = useSafeAreaInsets();
  const pad = useWindowClass() === 'compact' ? gutter.compact : 24;
  return (
    <View style={{ flex: 1, backgroundColor: colors.bgCanvas }}>
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
        <Segmented segments={segments} value={segment} onChange={onSegment} />
      </View>
      <View style={{ flex: 1 }}>{children}</View>
    </View>
  );
}

/** Scroll props for a page under TabbedRoot's tabs: inset for the tab bar on iOS, a gap under the tabs. */
export const tabBodyScroll = {
  contentInsetAdjustmentBehavior: nativeRootHeader ? ('automatic' as const) : undefined,
  paddingTop: 12,
};

const styles = StyleSheet.create({ titleRow: { flexDirection: 'row', alignItems: 'center', gap: 12 } });
