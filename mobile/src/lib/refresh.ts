import { useCallback, useEffect, useRef, useState } from 'react';

// How long the pull-to-refresh spinner shows at least, and at most.
const MIN_MS = 400;
const MAX_MS = 15_000;

/**
 * Pull to refresh that always ends. The spinner is this hook's own state, not
 * a query's isRefetching: iOS only stops its native spinner when `refreshing`
 * goes true and then false, and a query flag can skip that (a refetch paused
 * while offline never starts; one answered within a frame never shows), which
 * left spinners stuck. It waits for every refetch, at most MAX_MS.
 */
export function usePullToRefresh(refetch: () => Promise<unknown> | unknown): { refreshing: boolean; onRefresh: () => void } {
  const [refreshing, setRefreshing] = useState(false);
  const live = useRef(true);
  useEffect(() => {
    live.current = true;
    return () => {
      live.current = false;
    };
  }, []);
  const onRefresh = useCallback(() => {
    setRefreshing(true);
    const started = Date.now();
    const done = Promise.resolve()
      .then(refetch)
      .catch(() => undefined);
    const timeout = new Promise((resolve) => setTimeout(resolve, MAX_MS));
    void Promise.race([done, timeout]).then(() => {
      const wait = Math.max(0, MIN_MS - (Date.now() - started));
      setTimeout(() => {
        if (live.current) setRefreshing(false);
      }, wait);
    });
  }, [refetch]);
  return { refreshing, onRefresh };
}
