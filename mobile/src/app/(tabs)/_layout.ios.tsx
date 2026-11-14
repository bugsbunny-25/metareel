import { NativeTabs } from 'expo-router/native-tabs';

import { useTheme } from '@/theme';

/**
 * iOS uses the system tab bar (UITabBarController), so it is placed the way
 * the platform expects: at the bottom on iPhone, as a sidebar or top bar on
 * iPad, and in the vertical control strip beside the content on iPhone
 * Duo's outer display and on its inner display in landscape. Android keeps
 * the custom bar / rail in _layout.tsx.
 */
export default function TabsLayout() {
  const { colors } = useTheme();
  return (
    <NativeTabs tintColor={colors.textPrimary}>
      <NativeTabs.Trigger name="(home)">
        <NativeTabs.Trigger.Label>Tonight</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf={{ default: 'moon', selected: 'moon.fill' }} />
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="(tv)">
        <NativeTabs.Trigger.Label>TV</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf={{ default: 'tv', selected: 'tv.fill' }} />
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="(movies)">
        <NativeTabs.Trigger.Label>Movies</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf={{ default: 'film', selected: 'film.fill' }} />
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="(diary)">
        <NativeTabs.Trigger.Label>Diary</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf={{ default: 'book.closed', selected: 'book.closed.fill' }} />
      </NativeTabs.Trigger>
      <NativeTabs.Trigger name="(you)">
        <NativeTabs.Trigger.Label>You</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon sf={{ default: 'person.crop.circle', selected: 'person.crop.circle.fill' }} />
      </NativeTabs.Trigger>
    </NativeTabs>
  );
}
