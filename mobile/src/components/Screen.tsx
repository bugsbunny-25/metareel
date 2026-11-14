import type { PropsWithChildren } from 'react';
import { RefreshControl, ScrollView, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { nativeRootHeader, type RootAction, RootActions, RootTitle, useRootScroll } from '@/layout/root-header';
import { useWindowClass } from '@/layout/window-class';
import { useTheme } from '@/theme';

import { Text } from './Text';

/**
 * A scrolling screen on the canvas with the gutter for the window class.
 * The tab shell owns the bottom/left insets; screens pad the top.
 */
export function Screen({
  title,
  actions = [],
  children,
  refreshing,
  onRefresh,
}: PropsWithChildren<{
  title: string;
  actions?: RootAction[];
  refreshing?: boolean;
  onRefresh?: () => void;
}>) {
  const { colors, gutter } = useTheme();
  const insets = useSafeAreaInsets();
  const scroll = useRootScroll();
  const wc = useWindowClass();
  const pad = wc === 'compact' ? gutter.compact : 24;
  return (
    <ScrollView
      style={{ backgroundColor: colors.bgCanvas }}
      contentInsetAdjustmentBehavior={scroll.contentInsetAdjustmentBehavior}
      contentContainerStyle={[
        styles.content,
        {
          paddingTop: scroll.paddingTop,
          paddingHorizontal: pad,
          paddingRight: pad + insets.right,
        },
      ]}
      refreshControl={onRefresh ? <RefreshControl refreshing={!!refreshing} onRefresh={onRefresh} tintColor={colors.textMuted} /> : undefined}
    >
      <View style={styles.column}>
        {nativeRootHeader ? (
          <RootActions actions={actions} />
        ) : (
          <View style={styles.header}>
            <RootTitle>{title}</RootTitle>
            <RootActions actions={actions} />
          </View>
        )}
        {children}
      </View>
    </ScrollView>
  );
}

/** A rounded panel on the surface step (no hairlines between sections). */
export function Panel({ children, title }: PropsWithChildren<{ title?: string }>) {
  const { colors, radius } = useTheme();
  return (
    <View style={[styles.panel, { backgroundColor: colors.bgSurface, borderRadius: radius.panel }]}>
      {title ? <Text variant="heading">{title}</Text> : null}
      {children}
    </View>
  );
}

const styles = StyleSheet.create({
  content: { paddingBottom: 32 },
  // Single-column screens stay readable on unfolded and tablet windows;
  // list-detail screens (phase 4) use the width for a second pane instead.
  column: { gap: 16, width: '100%', maxWidth: 760, alignSelf: 'center' },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    marginBottom: 4,
  },
  panel: { padding: 16, gap: 10 },
});
