import { BarlowCondensed_800ExtraBold } from '@expo-google-fonts/barlow-condensed';
import { SourceSans3_400Regular, SourceSans3_600SemiBold, SourceSans3_700Bold, SourceSans3_800ExtraBold } from '@expo-google-fonts/source-sans-3';
import { SourceSerif4_600SemiBold, SourceSerif4_700Bold, useFonts } from '@expo-google-fonts/source-serif-4';
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client';
import { Stack } from 'expo-router';
import * as SplashScreen from 'expo-splash-screen';
import { StatusBar } from 'expo-status-bar';
import * as SystemUI from 'expo-system-ui';
import { useEffect } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { endpoint, SessionProvider, useSession } from '@/auth/session';
import { useEndpointRefresh } from '@/net/use-endpoint-refresh';
import { useSideInsets } from '@/layout/root-header';
import { useEpisodeReminderSync } from '@/notifications/use-reminders';
import { useOutboxSync } from '@/offline/use-sync';
import { persister, persistMaxAge, queryClient } from '@/query/client';
import { useWindowClass } from '@/layout/window-class';
import { useTheme } from '@/theme';

void SplashScreen.preventAutoHideAsync();

export default function RootLayout() {
  const [fontsLoaded] = useFonts({
    SourceSerif4_600SemiBold,
    SourceSerif4_700Bold,
    SourceSans3_400Regular,
    SourceSans3_600SemiBold,
    SourceSans3_700Bold,
    SourceSans3_800ExtraBold,
    BarlowCondensed_800ExtraBold,
    // Age ratings only (CertificationBadge), as in tides.
    MotionPictureRatings: require('../../assets/fonts/motion_picture_ratings.ttf'),
  });
  return (
    <SafeAreaProvider>
      <SessionProvider>
        <PersistQueryClientProvider client={queryClient} persistOptions={{ persister, maxAge: persistMaxAge }}>
          <Root fontsLoaded={fontsLoaded} />
        </PersistQueryClientProvider>
      </SessionProvider>
    </SafeAreaProvider>
  );
}

function Root({ fontsLoaded }: { fontsLoaded: boolean }) {
  const session = useSession();
  const { scheme, colors } = useTheme();
  const ready = fontsLoaded && session.status !== 'loading';
  // The log sheet: a bottom sheet on phones, a side panel beside the content
  // on unfolded foldables and tablets (drawn by the log screen itself).
  const sidePanel = useWindowClass() !== 'compact';
  const side = useSideInsets();
  useEndpointRefresh(endpoint);
  useOutboxSync(session.status === 'signedIn');
  useEpisodeReminderSync(session.status === 'signedIn');

  useEffect(() => {
    // The window background shows while a foldable unfolds or a window
    // resizes; keep it the canvas colour instead of white.
    void SystemUI.setBackgroundColorAsync(colors.bgCanvas);
  }, [colors.bgCanvas]);
  useEffect(() => {
    if (ready) SplashScreen.hide();
  }, [ready]);

  if (!ready) return null;
  const signedIn = session.status === 'signedIn';
  return (
    <>
      <StatusBar style={scheme === 'dark' ? 'light' : 'dark'} />
      <Stack screenOptions={{ headerShown: false, contentStyle: { backgroundColor: colors.bgCanvas } }}>
        <Stack.Protected guard={signedIn}>
          <Stack.Screen name="(tabs)" />
          {['search', 'share'].map((name) => (
            <Stack.Screen
              key={name}
              name={name}
              options={{
                headerShown: true,
                headerTitle: '',
                headerShadowVisible: false,
                headerBackButtonDisplayMode: 'minimal',
                headerStyle: { backgroundColor: colors.bgCanvas },
                headerTintColor: colors.textPrimary,
                headerTitleStyle: { fontFamily: 'SourceSerif4_700Bold', color: colors.textPrimary },
                contentStyle: { backgroundColor: colors.bgCanvas, ...side },
              }}
            />
          ))}
          <Stack.Screen
            name="log"
            options={
              sidePanel
                ? { presentation: 'transparentModal', animation: 'fade', contentStyle: { backgroundColor: 'transparent' } }
                : {
                    presentation: 'formSheet',
                    sheetAllowedDetents: [0.75, 1],
                    sheetGrabberVisible: true,
                    sheetCornerRadius: 20,
                    contentStyle: { backgroundColor: colors.bgSurface },
                  }
            }
          />
          <Stack.Screen name="scan-disc" options={{ presentation: 'fullScreenModal', contentStyle: { backgroundColor: colors.bgCanvas } }} />
        </Stack.Protected>
        <Stack.Protected guard={!signedIn}>
          <Stack.Screen name="sign-in" />
        </Stack.Protected>
      </Stack>
    </>
  );
}
