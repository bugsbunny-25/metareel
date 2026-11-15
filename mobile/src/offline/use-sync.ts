import * as Network from 'expo-network';
import { useEffect } from 'react';
import { AppState } from 'react-native';

import { registerBackgroundSync } from './background';
import { flushOutbox } from './index';

/** Sends queued writes on launch, on foreground and when the network returns. */
export function useOutboxSync(signedIn: boolean) {
  useEffect(() => {
    if (!signedIn) return;
    void flushOutbox();
    void registerBackgroundSync();
    const app = AppState.addEventListener('change', (s) => {
      if (s === 'active') void flushOutbox();
    });
    const net = Network.addNetworkStateListener((s) => {
      if (s.isConnected) void flushOutbox();
    });
    return () => {
      app.remove();
      net.remove();
    };
  }, [signedIn]);
}
