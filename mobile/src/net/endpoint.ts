import { useSyncExternalStore } from 'react';

/**
 * Which server address requests go to. The public HTTPS URL always works;
 * the optional home URL (plain HTTP on the home network) is used only after
 * the probe proves it is the same server, and is dropped the moment a
 * request to it fails at the network level.
 */
export type Route = 'public' | 'home';

export type EndpointState = {
  publicUrl: string | null;
  homeUrl: string | null;
  route: Route;
  /** When the home URL was last checked (ms since epoch), 0 if never. */
  checkedAt: number;
};

export class Endpoint {
  private state: EndpointState = { publicUrl: null, homeUrl: null, route: 'public', checkedAt: 0 };
  private listeners = new Set<() => void>();
  private inflight: Promise<void> | null = null;
  private generation = 0;

  constructor(private verify: (homeUrl: string) => Promise<boolean>) {}

  getState = (): EndpointState => this.state;

  subscribe = (l: () => void) => {
    this.listeners.add(l);
    return () => {
      this.listeners.delete(l);
    };
  };

  private set(next: Partial<EndpointState>) {
    this.state = { ...this.state, ...next };
    this.listeners.forEach((l) => l());
  }

  /** Point at a signed-in server (or nothing, when signed out). */
  configure(publicUrl: string | null, homeUrl: string | null) {
    this.generation++;
    this.inflight = null;
    this.set({ publicUrl, homeUrl, route: 'public', checkedAt: 0 });
    if (homeUrl) void this.refresh();
  }

  /** Re-check the home URL (app foreground, network change). */
  refresh(): Promise<void> {
    const { homeUrl } = this.state;
    if (!homeUrl) return Promise.resolve();
    if (this.inflight) return this.inflight;
    const gen = this.generation;
    this.inflight = this.verify(homeUrl)
      .catch(() => false)
      .then((ok) => {
        if (gen !== this.generation) return; // reconfigured meanwhile
        this.set({ route: ok ? 'home' : 'public', checkedAt: Date.now() });
      })
      .finally(() => {
        if (gen === this.generation) this.inflight = null;
      });
    return this.inflight;
  }

  /** A request to the home URL failed at the network level: stop using it. */
  homeFailed() {
    if (this.state.route === 'home') this.set({ route: 'public' });
  }

  /** The base URL to send a request to, or null when signed out. */
  base(): string | null {
    const s = this.state;
    if (s.route === 'home' && s.homeUrl) return s.homeUrl;
    return s.publicUrl;
  }
}

export function useEndpointState(endpoint: Endpoint): EndpointState {
  return useSyncExternalStore(endpoint.subscribe, endpoint.getState);
}
