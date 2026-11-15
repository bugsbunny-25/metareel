import { router } from 'expo-router';
import { TabList, Tabs, TabSlot, TabTrigger, type TabTriggerSlotProps } from 'expo-router/ui';
import type { PropsWithChildren } from 'react';
import { Pressable, StyleSheet, View, type ViewProps } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Icon, type IconName } from '@/components/Icon';
import { Text } from '@/components/Text';
import { useWindowClass } from '@/layout/window-class';
import { useTheme } from '@/theme';

const TABS: { name: string; href: '/' | '/diary' | '/tv' | '/movies' | '/you'; label: string; icon: IconName }[] = [
  { name: '(home)', href: '/', label: 'Tonight', icon: 'tonight' },
  { name: '(tv)', href: '/tv', label: 'TV', icon: 'tv' },
  { name: '(movies)', href: '/movies', label: 'Movies', icon: 'film' },
  { name: '(diary)', href: '/diary', label: 'Diary', icon: 'diary' },
  { name: '(you)', href: '/you', label: 'You', icon: 'user' },
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
          {/* On a wide window search sits in the rail with the tabs (as iOS's search
              tab does), below them; a phone keeps the button beside each title. */}
          {rail ? <RailSearch /> : null}
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
      {/* Material 3's active indicator: a full-radius pill, 64x32 in a bar, 56x32 in a rail. */}
      <View style={[styles.pill, rail ? styles.pillRail : styles.pillBar, isFocused && { backgroundColor: colors.bgElevated }]}>
        <Icon name={icon} size={24} color={color} />
      </View>
      <Text variant="caption" style={{ color, fontSize: 12 }} numberOfLines={1}>
        {label}
      </Text>
    </Pressable>
  );
}

function RailSearch() {
  const { colors } = useTheme();
  return (
    <Pressable
      onPress={() => router.push('/search')}
      accessibilityRole="button"
      accessibilityLabel="Search"
      style={({ pressed }) => [styles.railItem, styles.railSearch, { opacity: pressed ? 0.7 : 1 }]}
    >
      <View style={[styles.pill, styles.pillRail, { backgroundColor: colors.bgElevated }]}>
        <Icon name="search" size={24} color={colors.textPrimary} />
      </View>
      <Text variant="caption" style={{ color: colors.textMuted, fontSize: 12 }} numberOfLines={1}>
        Search
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  railSearch: { marginTop: 12 },
  bar: { paddingTop: 6 },
  barDir: { flexDirection: 'row', alignItems: 'stretch', justifyContent: 'space-around' },
  railDir: { flexDirection: 'column', alignItems: 'center', justifyContent: 'flex-start' },
  barItem: { flex: 1, flexDirection: 'column', alignItems: 'center', gap: 4, minHeight: 56, paddingHorizontal: 0 },
  rail: { width: 88, alignItems: 'center', gap: 12 },
  railItem: { flexDirection: 'column', alignItems: 'center', gap: 4, minHeight: 56, width: 80, paddingHorizontal: 0 },
  pill: { height: 32, borderRadius: 16, alignItems: 'center', justifyContent: 'center' },
  pillBar: { width: 64 },
  pillRail: { width: 56 },
});
