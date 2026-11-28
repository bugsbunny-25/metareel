import { Stack } from 'expo-router';

import { nativeRootHeader, useSideInsets } from '@/layout/root-header';
import { useTheme } from '@/theme';

// Each tab is its own stack, so a title, show or watch opened from a tab
// keeps the tab bar (or rail) on screen and Back returns within the tab.
// The detail screens are shared routes, one copy per tab group; anchor is
// each group's root screen.
export const unstable_settings = {
  anchor: 'index',
  tv: { anchor: 'tv' },
  movies: { anchor: 'movies' },
  you: { anchor: 'you' },
  search: { anchor: 'find' },
};

const ROOT_TITLES: Record<string, string> = {
  index: 'Tonight',
  tv: 'TV',
  movies: 'Movies',
  you: 'You',
  find: 'Search',
};

// Roots with tabs pinned under a standard (opaque) bar: TV and Movies' To
// watch / Upcoming, You's You / Diary / Library.
const PINNED = new Set(['tv', 'movies', 'you']);

export default function TabStack() {
  const { colors } = useTheme();
  const side = useSideInsets();
  return (
    <Stack
      screenOptions={({ route }) => {
        const root = ROOT_TITLES[route.name];
        // On iOS a root's title and actions live in the system bar with a
        // large title (see layout/root-header); Android roots draw their own.
        // TV and Movies pin a To watch / Upcoming switch under a standard
        // (opaque) bar, so their lists start below it.
        if (root && nativeRootHeader && PINNED.has(route.name)) {
          return {
            headerShown: true,
            title: root,
            headerLargeTitleEnabled: false,
            headerTransparent: false,
            headerShadowVisible: false,
            headerStyle: { backgroundColor: colors.bgCanvas },
            headerTintColor: colors.textPrimary,
            headerTitleStyle: { fontFamily: 'SourceSerif4_700Bold', color: colors.textPrimary },
            contentStyle: { backgroundColor: colors.bgCanvas },
          };
        }
        if (root && nativeRootHeader) {
          return {
            headerShown: true,
            title: root,
            headerLargeTitleEnabled: true,
            headerTransparent: true,
            headerShadowVisible: false,
            headerLargeTitleShadowVisible: false,
            headerTintColor: colors.textPrimary,
            headerTitleStyle: {
              fontFamily: 'SourceSerif4_700Bold',
              color: colors.textPrimary,
            },
            headerLargeTitleStyle: {
              fontFamily: 'SourceSerif4_700Bold',
              color: colors.textPrimary,
            },
            contentStyle: { backgroundColor: colors.bgCanvas },
          };
        }
        return {
          headerShown: !root,
          headerTitle: '',
          headerShadowVisible: false,
          headerBackButtonDisplayMode: 'minimal',
          headerStyle: { backgroundColor: colors.bgCanvas },
          headerTintColor: colors.textPrimary,
          headerTitleStyle: {
            fontFamily: 'SourceSerif4_700Bold',
            color: colors.textPrimary,
          },
          contentStyle: { backgroundColor: colors.bgCanvas, ...side },
        };
      }}
    />
  );
}
