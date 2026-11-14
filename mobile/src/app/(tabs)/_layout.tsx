import { TabList, Tabs, TabSlot, TabTrigger, type TabTriggerSlotProps } from 'expo-router/ui';
import type { PropsWithChildren } from 'react';
import { Pressable, StyleSheet, View, type ViewProps } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Icon, type IconName } from '@/components/Icon';
import { Text } from '@/components/Text';
import { useWindowClass } from '@/layout/window-class';
import { useTheme } from '@/theme';

const TABS: { name: string; href: '/' | '/diary' | '/up-next' | '/watchlist' | '/settings'; label: string; icon: IconName }[] = [
  { name: '(home)', href: '/', label: 'Tonight', icon: 'tonight' },
  { name: '(diary)', href: '/diary', label: 'Diary', icon: 'diary' },
  { name: '(up-next)', href: '/up-next', label: 'Up next', icon: 'up-next' },
  { name: '(watchlist)', href: '/watchlist', label: 'Watchlist', icon: 'bookmark' },
  { name: '(settings)', href: '/settings', label: 'Settings', icon: 'settings' },
];

/**
 * One navigator for every window size: a bottom bar on compact windows and
 * a navigation rail on medium and expanded ones. Only the flex direction
 * changes when a foldable unfolds or a window resizes, so the screens (and
 * whatever was half-typed in them) are never remounted.
 */
export default function TabsLayout() {
  const rail = useWindowClass() !== 'compact';
  const { colors } = useTheme();
  return (
    <Tabs style={{ flex: 1, flexDirection: rail ? 'row' : 'column-reverse', backgroundColor: colors.bgCanvas }}>
      <TabList asChild>
        <NavBar rail={rail}>
          {TABS.map((t) => (
            <TabTrigger key={t.name} name={t.name} href={t.href} asChild>
              <NavItem rail={rail} label={t.label} icon={t.icon} />
            </TabTrigger>
          ))}
        </NavBar>
      </TabList>
      <TabSlot style={{ flex: 1 }} />
    </Tabs>
  );
}

function NavBar({ rail, children, style, ...rest }: PropsWithChildren<ViewProps & { rail: boolean }>) {
  const { colors } = useTheme();
  const insets = useSafeAreaInsets();
  return (
    <View
      accessibilityRole="tablist"
      style={[
        rail
          ? [styles.rail, { paddingTop: insets.top + 16, paddingLeft: insets.left, paddingBottom: insets.bottom }]
          : [styles.bar, { paddingBottom: Math.max(insets.bottom, 8), paddingLeft: insets.left, paddingRight: insets.right }],
        style, // TabList's defaults first, so the bar / rail layout wins
        { backgroundColor: colors.bgSurface },
        rail ? styles.railDir : styles.barDir,
      ]}
      {...rest}
    >
      {children}
    </View>
  );
}

function NavItem({
  rail,
  label,
  icon,
  isFocused,
  ref: _ref, // the trigger's ref targets a View; the Pressable doesn't need it
  style,
  ...rest
}: TabTriggerSlotProps & { rail: boolean; label: string; icon: IconName }) {
  const { colors } = useTheme();
  const color = isFocused ? colors.textPrimary : colors.textMuted;
  return (
    <Pressable
      accessibilityRole="tab"
      accessibilityState={{ selected: !!isFocused }}
      accessibilityLabel={label}
      {...rest}
      // TabTrigger's default style first, so the item layout wins.
      style={[typeof style === 'function' ? undefined : style, rail ? styles.railItem : styles.barItem]}
    >
      <View style={[styles.pill, isFocused && { backgroundColor: colors.bgElevated }]}>
        <Icon name={icon} size={22} color={color} />
      </View>
      <Text variant="caption" style={{ color, fontSize: 12 }} numberOfLines={1}>
        {label}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  bar: { paddingTop: 6 },
  barDir: { flexDirection: 'row', alignItems: 'stretch', justifyContent: 'space-around' },
  railDir: { flexDirection: 'column', alignItems: 'center', justifyContent: 'flex-start' },
  barItem: { flex: 1, flexDirection: 'column', alignItems: 'center', gap: 2, minHeight: 52, paddingHorizontal: 0 },
  rail: { width: 88, alignItems: 'center', gap: 12 },
  railItem: { flexDirection: 'column', alignItems: 'center', gap: 4, minHeight: 56, width: 80, paddingHorizontal: 0 },
  pill: { width: 56, height: 30, borderRadius: 15, alignItems: 'center', justifyContent: 'center' },
});
