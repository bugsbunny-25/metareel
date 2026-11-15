import { router, Stack } from 'expo-router';
import type { ComponentProps } from 'react';
import { Platform, Pressable, StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Icon, type IconName } from '@/components/Icon';
import { Text } from '@/components/Text';
import { useWindowClass } from '@/layout/window-class';
import { useTheme } from '@/theme';

/**
 * Tab roots on iOS use the system navigation bar (large title, toolbar
 * buttons), so the platform can place them: at the top on most iPhones, and
 * in the vertical control strip on iPhone Duo with the tab bar. Android draws
 * its own title row.
 */
export const nativeRootHeader = Platform.OS === 'ios';

/** Scroll-view props for a tab root's content. */
export function useRootScroll(): { contentInsetAdjustmentBehavior?: 'automatic'; paddingTop: number; insetRight: number } {
  const insets = useSafeAreaInsets();
  // Vertical scroll views aren't inset sideways by the system, so content
  // adds the right inset itself (iPhone Duo's control strip, a landscape
  // camera cutout).
  return nativeRootHeader
    ? { contentInsetAdjustmentBehavior: 'automatic', paddingTop: 8, insetRight: insets.right }
    : { paddingTop: insets.top + 12, insetRight: insets.right };
}

/**
 * Scroll props for a detail shown beside a list (two panes). On iOS the
 * system insets it below the tab root's bar, which can be the see-through
 * large-title bar it would otherwise sit under (iPhone Duo open, iPad);
 * Android has no bar above it, so it clears the status bar itself.
 */
export function useDetailScroll(embedded?: boolean): { contentInsetAdjustmentBehavior?: 'automatic'; paddingTop?: number } {
  const insets = useSafeAreaInsets();
  if (!embedded) return {};
  return nativeRootHeader ? { contentInsetAdjustmentBehavior: 'automatic', paddingTop: 12 } : { paddingTop: insets.top + 12 };
}

/**
 * Side padding for pushed screens on iOS. On iPhone Duo's outer display (and
 * its inner display in landscape) the status bar, Back button and tab bar sit
 * in a strip on one side; the window's safe area covers it, so content inset
 * by it never runs under those controls. Android's rail already pads for its
 * side, so it gets none.
 */
export function useSideInsets(): { paddingLeft?: number; paddingRight?: number } {
  const insets = useSafeAreaInsets();
  return nativeRootHeader ? { paddingLeft: insets.left, paddingRight: insets.right } : {};
}

/** The screen title: the native bar shows it on iOS. */
export function RootTitle({ children }: { children: string }) {
  if (nativeRootHeader) return null;
  return (
    <Text variant="display" accessibilityRole="header" style={{ flex: 1 }}>
      {children}
    </Text>
  );
}

type SFSymbol = NonNullable<ComponentProps<typeof Stack.Toolbar.Button>['icon']>;

export type RootAction = { label: string; sf: SFSymbol; icon: IconName; onPress: () => void; search?: boolean };

/**
 * Search, beside every tab's title on Android. iOS leaves it out: its search
 * is the system search tab, which sits with the other tabs (and so in the
 * iPhone Duo's control strip, where toolbar buttons stack up beside them).
 */
export const searchAction: RootAction = { label: 'Search', sf: 'magnifyingglass', icon: 'search', onPress: () => router.push('/search'), search: true };

/**
 * A tab root's actions: toolbar buttons (symbol plus title, as the HIG asks)
 * on iOS, round icon buttons beside the title elsewhere.
 */
export function RootActions({ actions }: { actions: RootAction[] }) {
  const { colors } = useTheme();
  // Search lives with the tabs where they can hold it: iOS's search tab, Android's rail.
  const rail = useWindowClass() !== 'compact';
  if (nativeRootHeader || rail) actions = actions.filter((a) => !a.search);
  if (!actions.length) return null;
  if (nativeRootHeader) {
    return (
      <Stack.Toolbar placement="right">
        {actions.map((a) => (
          <Stack.Toolbar.Button key={a.label} icon={a.sf} onPress={a.onPress}>
            {a.label}
          </Stack.Toolbar.Button>
        ))}
      </Stack.Toolbar>
    );
  }
  return (
    <View style={styles.row}>
      {actions.map((a) => (
        <Pressable
          key={a.label}
          onPress={a.onPress}
          accessibilityRole="button"
          accessibilityLabel={a.label}
          style={[styles.round, { backgroundColor: colors.bgElevated }]}
        >
          <Icon name={a.icon} color={colors.textPrimary} />
        </Pressable>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', gap: 8 },
  round: { width: 40, height: 40, borderRadius: 20, alignItems: 'center', justifyContent: 'center' },
});
