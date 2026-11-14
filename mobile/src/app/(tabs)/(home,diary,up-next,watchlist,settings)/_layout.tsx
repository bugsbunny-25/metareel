import { Stack } from 'expo-router';

import { useTheme } from '@/theme';

// Each tab is its own stack, so a title, show or watch opened from a tab
// keeps the tab bar (or rail) on screen and Back returns within the tab.
// The detail screens are shared routes, one copy per tab group; anchor is
// each group's root screen.
export const unstable_settings = {
  anchor: 'index',
  diary: { anchor: 'diary' },
  'up-next': { anchor: 'up-next' },
  watchlist: { anchor: 'watchlist' },
  settings: { anchor: 'settings' },
};

const ROOTS = new Set(['index', 'diary', 'up-next', 'watchlist', 'settings']);

export default function TabStack() {
  const { colors } = useTheme();
  return (
    <Stack
      screenOptions={({ route }) => ({
        headerShown: !ROOTS.has(route.name),
        headerTitle: '',
        headerShadowVisible: false,
        headerBackButtonDisplayMode: 'minimal',
        headerStyle: { backgroundColor: colors.bgCanvas },
        headerTintColor: colors.textPrimary,
        headerTitleStyle: { fontFamily: 'SourceSerif4_700Bold', color: colors.textPrimary },
        contentStyle: { backgroundColor: colors.bgCanvas },
      })}
    />
  );
}
