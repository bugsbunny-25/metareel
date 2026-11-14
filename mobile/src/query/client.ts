import { createAsyncStoragePersister } from '@tanstack/query-async-storage-persister';
import { focusManager, onlineManager, QueryClient } from '@tanstack/react-query';
import * as Network from 'expo-network';
import Storage from 'expo-sqlite/kv-store';
import { AppState, Platform } from 'react-native';

const DAY = 24 * 60 * 60 * 1000;

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      gcTime: 7 * DAY, // kept (and persisted) so the app opens with data offline
      retry: 2,
      networkMode: 'offlineFirst',
    },
  },
});

/** Persists the query cache in SQLite, so screens have data in a cinema basement. */
export const persister = createAsyncStoragePersister({
  storage: Storage,
  key: 'metareel.query-cache',
  throttleTime: 2000,
});

export const persistMaxAge = 7 * DAY;

// React Native has no window focus/online events: wire TanStack Query to the
// app state and the network state instead.
onlineManager.setEventListener((setOnline) => {
  const sub = Network.addNetworkStateListener((s) => setOnline(s.isConnected !== false));
  return () => sub.remove();
});
focusManager.setEventListener((handleFocus) => {
  if (Platform.OS === 'web') return;
  const sub = AppState.addEventListener('change', (s) => handleFocus(s === 'active'));
  return () => sub.remove();
});

/** Forgets every cached response (sign-out). */
export async function clearQueryCache() {
  queryClient.clear();
  await persister.removeClient();
}
