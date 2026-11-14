import { SourceSans3_400Regular, SourceSans3_600SemiBold, SourceSans3_700Bold } from '@expo-google-fonts/source-sans-3';
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
import { persister, persistMaxAge, queryClient } from '@/query/client';
import { useTheme } from '@/theme';

void SplashScreen.preventAutoHideAsync();

export default function RootLayout() {
  const [fontsLoaded] = useFonts({
    SourceSerif4_600SemiBold,
    SourceSerif4_700Bold,
    SourceSans3_400Regular,
    SourceSans3_600SemiBold,
    SourceSans3_700Bold,
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
  useEndpointRefresh(endpoint);

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
        </Stack.Protected>
        <Stack.Protected guard={!signedIn}>
          <Stack.Screen name="sign-in" />
        </Stack.Protected>
      </Stack>
    </>
  );
}
