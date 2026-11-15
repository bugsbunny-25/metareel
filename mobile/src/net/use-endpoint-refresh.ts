import * as Network from 'expo-network';
import { useEffect } from 'react';
import { AppState } from 'react-native';

import type { Endpoint } from './endpoint';

/** Re-checks the home URL when the app comes to the foreground or the network changes. */
export function useEndpointRefresh(endpoint: Endpoint) {
  useEffect(() => {
    const app = AppState.addEventListener('change', (s) => {
      if (s === 'active') void endpoint.refresh();
    });
    const net = Network.addNetworkStateListener(() => void endpoint.refresh());
    return () => {
      app.remove();
      net.remove();
    };
  }, [endpoint]);
}
